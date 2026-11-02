package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	crmService "erpCluvix/internal/modules/webapp/crm/service"
	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/handlerutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func (h *Handler) buildTreatmentServiceQuery(c *gin.Context) *gorm.DB {
	q := tenant.ScopedDBNullBranch(c, h.DB, "order_service_used").
		Table("order_service_used").
		Where("order_service_used.isLock = 1")

	// JOIN order_plan cho filter created_time VÀ orderPlanType.
	// ⚠ orderPlanType trước đây không nằm trong điều kiện này ⇒ lọc riêng orderPlanType mà
	// không kèm created_time là "Unknown column 'order_plan.orderPlanType'".
	if query.GetFilterValue(c, "created_time") != "" || c.Query("filter[created_time][gte]") != "" || c.Query("filter[created_time][lte]") != "" ||
		len(query.GetFilterValues(c, "orderPlanType")) > 0 {
		q = q.Joins("LEFT JOIN order_plan ON order_plan.id = order_service_used.order_plan_id")
	}
	q = query.ApplyFilter(c, q, "created_time", "order_plan.created_time")
	q = query.ApplyFilter(c, q, "created_at", "order_service_used.created_at")

	// Exact filters
	q = query.ApplyFilter(c, q, "appointment_id", "order_service_used.appointment_id")
	q = query.ApplyFilter(c, q, "source_id", "order_service_used.source_id")
	q = query.ApplyFilter(c, q, "branch_id", "order_service_used.branch_id")
	q = query.ApplyFilter(c, q, "orderPlanType", "order_plan.orderPlanType")
	q = query.ApplyFilter(c, q, "created_by", "order_service_used.created_by")
	q = query.ApplyFilter(c, q, "payment_status", "order_service_used.payment_status")

	// _serviceType: via service → service_type
	if vals := query.GetFilterValues(c, "_serviceType"); len(vals) > 0 {
		q = q.Joins("LEFT JOIN service ON service.id = order_service_used.service_id").
			Where("service.service_type_id IN ?", vals)
	}

	// _listRevenues: JSON_CONTAINS OR
	if vals := query.GetFilterValues(c, "_listRevenues"); len(vals) > 0 {
		orClause := ""
		args := make([]interface{}, len(vals))
		for i, v := range vals {
			if i > 0 {
				orClause += " OR "
			}
			orClause += "JSON_CONTAINS(order_service_used.listRevenues, ?)"
			args[i] = v
		}
		q = q.Where(orClause, args...)
	}

	// _groupLocal
	if vals := query.GetFilterValues(c, "_groupLocal"); len(vals) > 0 {
		provinceIds := query.GetProvinceIdsByGroupLocal(h.DB, vals)
		if len(provinceIds) > 0 {
			q = q.Joins("LEFT JOIN customer cgl ON cgl.id = order_service_used.customer_id").
				Where("cgl.province_id IN ?", provinceIds)
		}
	}

	// _groupCountry
	if vals := query.GetFilterValues(c, "_groupCountry"); len(vals) > 0 {
		countryIds := query.GetCountryIdsByGroupCountry(h.DB, vals)
		if len(countryIds) > 0 {
			q = q.Joins("LEFT JOIN customer cgc ON cgc.id = order_service_used.customer_id").
				Where("cgc.country_id IN ?", countryIds)
		}
	}

	// Keyword — ưu tiên đường tra-id-trước (không join gì). Chi tiết + số đo + các biến thể
	// đã loại: list_search.go.
	q, kwFast := h.applyTreatmentServiceKeywordFast(c, q)
	if !kwFast {
		// Đường lui: 3 LEFT JOIN + LIKE '%kw%' trên 7 cột. Chậm nhưng ĐÚNG — chỉ chạy khi
		// vượt trần id hoặc từ khoá quá ngắn để ngram sinh token.
		q = q.Joins("LEFT JOIN customer ON customer.id = order_service_used.customer_id").
			Joins("LEFT JOIN service svc ON svc.id = order_service_used.service_id").
			Joins("LEFT JOIN order_plan otp ON otp.id = order_service_used.order_plan_id")
		kw := query.GetFilterValue(c, "_keyword")
		like := "%" + kw + "%"
		q = q.Where("customer.fullname LIKE ? OR customer.code LIKE ? OR customer.phone LIKE ? OR order_service_used.code LIKE ? OR svc.name LIKE ? OR svc.code LIKE ? OR otp.note LIKE ?",
			like, like, like, like, like, like, like)
	}

	// sourceCustomer_id needs customer join — bỏ qua nếu đường lui từ khoá đã join rồi
	// (join trùng alias ⇒ "Not unique table/alias: 'customer'").
	if query.GetFilterValue(c, "sourceCustomer_id") != "" && kwFast {
		q = q.Joins("LEFT JOIN customer ON customer.id = order_service_used.customer_id")
	}
	q = query.ApplyFilter(c, q, "sourceCustomer_id", "customer.source_id")

	q = query.ApplySort(c, q, "order_service_used.id DESC", "order_service_used.id")
	return q
}

// ListTreatmentService GET /crm/order-service-used
func (h *Handler) ListTreatmentService(c *gin.Context) {
	q := h.buildTreatmentServiceQuery(c)

	if c.Query("_export") == "1" {
		exportHandlers.ExportOrderServiceUsed(c, q)
		return
	}

	// Aggregates — simple raw query with same WHERE conditions
	var agg struct {
		TotalDiscountOther decimal.Decimal `gorm:"column:totalDiscountOther"`
		TotalTotalEnd      decimal.Decimal `gorm:"column:totalTotalEnd"`
		TotalVatAmount     decimal.Decimal `gorm:"column:totalVatAmount"`
		TotalThanhtoan     decimal.Decimal `gorm:"column:totalThanhtoan"`
	}
	h.buildTreatmentServiceQuery(c).
		Select(`COALESCE(SUM(order_service_used.discount_other), 0) as totalDiscountOther,
			COALESCE(SUM(order_service_used.totalEnd), 0) as totalTotalEnd,
			COALESCE(SUM(order_service_used.vat_amount), 0) as totalVatAmount,
			COALESCE(SUM(order_service_used.payment), 0) as totalThanhtoan`).
		Offset(-1).Limit(-1).
		Scan(&agg)

	// Paginate with specific SELECT
	var items []map[string]interface{}
	q = q.Select(`order_service_used.id, order_service_used.code, order_service_used.customer_id,
		order_service_used.appointment_id, order_service_used.branch_id,
		order_service_used.order_plan_id, order_service_used.service_id,
		order_service_used.listRevenues, order_service_used.source_id,
		order_service_used.quantity,
		order_service_used.unit_price,
		order_service_used.service_discount, order_service_used.service_discount_type,
		order_service_used.discount_other,
		order_service_used.totalEnd, order_service_used.vat_amount, order_service_used.payment,
		order_service_used.payment_status, order_service_used.dayComplete,
		order_service_used.created_day,
		order_service_used.isLock,
		order_service_used.created_by, order_service_used.created_at,
		order_service_used.updated_by, order_service_used.updated_at`)
	result := pagination.Paginate(c, q, &items)

	if len(items) > 0 {
		customerIds := handlerutil.CollectIDs(items, "customer_id")
		appointmentIds := handlerutil.CollectIDs(items, "appointment_id")
		treatmentPlanIds := handlerutil.CollectIDs(items, "order_plan_id")

		if len(customerIds) > 0 {
			handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "customer_id", "customer",
				"customer", "id", "id, code, old_code, fullname, nickname, phone, address, year_born, province_id, ward_id, country_id, source_id", customerIds)
		}
		if len(treatmentPlanIds) > 0 {
			handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "order_plan_id", "treatmentPlan",
				"order_plan", "id", "id, code, note, orderPlanType, created_time", treatmentPlanIds)
		}
		if len(appointmentIds) > 0 {
			handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "appointment_id", "appointment",
				"appointment", "id", "id, code, appointment_date, appointment_type_id, status, branch_id", appointmentIds)
		}
	}

	// AttachRelation nest customer.phone vào map con — mask thủ công vì
	// MaskPhoneInItems chỉ xử lý top-level phone.
	if !phoneutil.CanSeePhone(c, h.DB) {
		for _, item := range items {
			if customer, ok := item["customer"].(map[string]interface{}); ok {
				if phone, ok2 := customer["phone"].(string); ok2 {
					customer["phone"] = phoneutil.MaskPhone(phone)
				}
			}
		}
	}

	// customer.ward_name — nest vào map con, không dùng AttachRelation (chỉ attach top-level items)
	// vì ward thuộc về customer (sub-object), không phải order_service_used.
	wardIDs := make([]interface{}, 0)
	seenWard := map[int64]bool{}
	for _, item := range items {
		if customer, ok := item["customer"].(map[string]interface{}); ok {
			if wid := helpers.ToInt64(customer["ward_id"]); wid > 0 && !seenWard[wid] {
				seenWard[wid] = true
				wardIDs = append(wardIDs, wid)
			}
		}
	}
	if len(wardIDs) > 0 {
		var wards []struct {
			ID   int64  `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		h.DB.Table("location_ward").Select("id, name").Where("id IN ?", wardIDs).Find(&wards)
		wardNameByID := make(map[int64]string, len(wards))
		for _, w := range wards {
			wardNameByID[w.ID] = w.Name
		}
		for _, item := range items {
			if customer, ok := item["customer"].(map[string]interface{}); ok {
				if wid := helpers.ToInt64(customer["ward_id"]); wid > 0 {
					customer["ward_name"] = wardNameByID[wid]
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"object":             result,
		"totalDiscountOther": agg.TotalDiscountOther,
		"totalTotalEnd":      agg.TotalTotalEnd,
		"totalVatAmount":     agg.TotalVatAmount,
		"totalThanhtoan":     agg.TotalThanhtoan,
	})
}

// buildServiceUsedWithAppointmentQuery — WHERE/JOIN dùng CHUNG cho cả 3 đường của
// ListServiceUsedWithAppointment: danh sách (+`COUNT(*)` của Paginate), export, và dòng TỔNG.
//
// ⚠ Trước đây dòng tổng tự dựng lại WHERE riêng và chỉ áp `created_day` + `appointment_date`,
// bỏ qua branch_id / source_id / created_by / created_at / sourceCustomer_id / _keyword và cả
// các filter JSON người phụ trách (nay chỉ còn _listRevenues). Hệ quả đo được trên DB clone
// 524.288 dòng, với bộ lọc ngày mặc định của FE (30 ngày): lọc thêm theo bác sĩ #5 ra 284 dòng
// / 430.875.286đ, nhưng dòng tổng vẫn báo 4.402 dòng / 6.678.560.330đ — số của bộ lọc ngày
// KHÔNG có bác sĩ. Lọc theo chi nhánh (258 dòng), nguồn (490), từ khoá (42) đều lệch y hệt.
// Dựng MỘT lần ở đây để filter mới không thể "quên" gắn vào dòng tổng nữa — cùng cách
// `buildTreatmentServiceQuery` đang làm cho endpoint anh em ở trên.
// Đối chiếu sau khi sửa: 5/5 kịch bản trên, dòng tổng khớp đúng số dòng của danh sách.
//
// KHÔNG gắn ORDER BY trong này: dòng tổng là query gộp không GROUP BY, `sql_mode` có
// ONLY_FULL_GROUP_BY. Sort chỉ áp ở đường danh sách/export.
func (h *Handler) buildServiceUsedWithAppointmentQuery(c *gin.Context) *gorm.DB {
	q := tenant.ScopedDBNullBranch(c, h.DB, "order_service_used").
		Table("order_service_used").
		Where("order_service_used.isLock = 1").
		Where("order_service_used.appointment_id IS NOT NULL")

	// JOIN appointment khi cần filter theo appointment_type_id hoặc appointment_date
	needsApptJoin := query.GetFilterValue(c, "appointment_type_id") != "" ||
		c.Query("filter[appointment_type_id][gte]") != "" || c.Query("filter[appointment_type_id][lte]") != "" ||
		c.Query("filter[appointment_date][gte]") != "" || c.Query("filter[appointment_date][lte]") != ""
	if needsApptJoin {
		// Story-37 (E6/D15 — 1 thương hiệu/DB): BỎ vế company trong điều kiện JOIN appointment. Phạm vi
		// dữ liệu vẫn do branch scope của order_service_used ở trên quyết định (không đổi).
		//
		// ⚠ HIỆU NĂNG — vế company từng là chỗ dựa để `appointment.appointment_date BETWEEN ...` dùng
		// được `idx_appointment_date_scope` (index dẫn đầu bằng company_id). Đo cũ trên DB clone
		// 524.288 dòng osu / 300.000 lịch hẹn, lọc ngày hẹn 30 ngày: SUM tổng 0.62-1.33s (không vế)
		// → 0.05-0.13s (có vế). Bỏ vế ⇒ quay về đường chậm tới khi schema mới (E4) có index dẫn đầu
		// bằng `appointment_date` — story-24/25 phải đo lại bằng /check-scale, KHÔNG thêm hint cứng.
		q = q.Joins("LEFT JOIN appointment ON appointment.id = order_service_used.appointment_id")
		q = query.ApplyFilter(c, q, "appointment_type_id", "appointment.appointment_type_id")
		q = query.ApplyFilter(c, q, "appointment_date", "appointment.appointment_date")
	}

	// Range filters
	q = query.ApplyFilter(c, q, "created_day", "order_service_used.created_day")
	q = query.ApplyFilter(c, q, "created_at", "order_service_used.created_at")

	// Exact filters
	q = query.ApplyFilter(c, q, "source_id", "order_service_used.source_id")
	q = query.ApplyFilter(c, q, "branch_id", "order_service_used.branch_id")
	q = query.ApplyFilter(c, q, "created_by", "order_service_used.created_by")

	// sourceCustomer_id: filter theo source_id của customer
	if query.GetFilterValue(c, "sourceCustomer_id") != "" {
		q = q.Joins("LEFT JOIN customer sc ON sc.id = order_service_used.customer_id")
		q = query.ApplyFilter(c, q, "sourceCustomer_id", "sc.source_id")
	}

	// _listRevenues
	if vals := query.GetFilterValues(c, "_listRevenues"); len(vals) > 0 {
		orClause := ""
		args := make([]interface{}, len(vals))
		for i, v := range vals {
			if i > 0 {
				orClause += " OR "
			}
			orClause += "JSON_CONTAINS(order_service_used.listRevenues, ?)"
			args[i] = v
		}
		q = q.Where(orClause, args...)
	}

	// Keyword
	if c.Query("_keyword") != "" {
		q = q.Joins("LEFT JOIN customer ON customer.id = order_service_used.customer_id").
			Joins("LEFT JOIN service svc ON svc.id = order_service_used.service_id")
		like := "%" + c.Query("_keyword") + "%"
		q = q.Where("customer.fullname LIKE ? OR customer.code LIKE ? OR customer.phone LIKE ? OR order_service_used.code LIKE ? OR svc.name LIKE ? OR svc.code LIKE ?",
			like, like, like, like, like, like)
	}

	return q
}

// ListServiceUsedWithAppointment GET /crm/order-service-used/service-used-with-appointment
func (h *Handler) ListServiceUsedWithAppointment(c *gin.Context) {
	q := query.ApplySort(c, h.buildServiceUsedWithAppointmentQuery(c),
		"order_service_used.id DESC", "order_service_used.id")

	if c.Query("_export") == "1" {
		exportHandlers.ExportOrderServiceUsedWithAppt(c, q)
		return
	}

	// Aggregates — CÙNG bộ lọc với danh sách (dựng lại từ cùng builder, không sort).
	var agg struct {
		TotalTotalEnd  decimal.Decimal `gorm:"column:totalTotalEnd"`
		TotalVatAmount decimal.Decimal `gorm:"column:totalVatAmount"`
		TotalThanhtoan decimal.Decimal `gorm:"column:totalThanhtoan"`
	}
	// Lỗi câu tổng KHÔNG được nuốt: `Scan` thất bại để `agg` ở zero-value, FE hiện tổng = 0 —
	// nhìn y hệt một bộ lọc không có dữ liệu, không ai biết là query hỏng.
	aggTx := h.buildServiceUsedWithAppointmentQuery(c).
		Select(`COALESCE(SUM(order_service_used.totalEnd), 0) as totalTotalEnd,
		COALESCE(SUM(order_service_used.vat_amount), 0) as totalVatAmount,
		COALESCE(SUM(order_service_used.payment), 0) as totalThanhtoan`).
		Scan(&agg)
	if pagination.IsQueryError(aggTx.Error) {
		pagination.FailQuery(c, aggTx.Error)
		return
	}

	// Paginate
	var items []map[string]interface{}
	q = q.Select(`order_service_used.id, order_service_used.code, order_service_used.customer_id,
		order_service_used.appointment_id, order_service_used.branch_id,
		order_service_used.order_plan_id, order_service_used.service_id,
		order_service_used.listRevenues, order_service_used.source_id,
		order_service_used.quantity,
		order_service_used.unit_price,
		order_service_used.service_discount, order_service_used.service_discount_type,
		order_service_used.discount_other, order_service_used.totalEnd, order_service_used.vat_amount,
		order_service_used.payment, order_service_used.payment_status, order_service_used.created_day,
		order_service_used.isLock, order_service_used.created_by, order_service_used.created_at,
		order_service_used.updated_by, order_service_used.updated_at`)
	result := pagination.Paginate(c, q, &items)

	// Luôn attach nested relations
	if len(items) > 0 {
		customerIds := handlerutil.CollectIDs(items, "customer_id")
		appointmentIds := handlerutil.CollectIDs(items, "appointment_id")

		if len(customerIds) > 0 {
			handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "customer_id", "customer",
				"customer", "id", "id, code, fullname, nickname, phone, address, gender, avatar, status", customerIds)
		}
		if len(appointmentIds) > 0 {
			handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "appointment_id", "appointment",
				"appointment", "id", "id, code, appointment_date, appointment_type_id, status, branch_id", appointmentIds)
		}
	}

	// AttachRelationScoped nest customer.phone vào map con — mask thủ công vì
	// MaskPhoneInItems chỉ xử lý top-level phone.
	if !phoneutil.CanSeePhone(c, h.DB) {
		for _, item := range items {
			if customer, ok := item["customer"].(map[string]interface{}); ok {
				if phone, ok2 := customer["phone"].(string); ok2 {
					customer["phone"] = phoneutil.MaskPhone(phone)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"object":         result,
		"totalTotalEnd":  agg.TotalTotalEnd,
		"totalVatAmount": agg.TotalVatAmount,
		"totalThanhtoan": agg.TotalThanhtoan,
	})
}

// treatmentServiceDetailColumns — cột cho endpoint chi tiết dòng dịch vụ.
//
// Giới hạn đúng tập 2 consumer đang dùng, KHÔNG trả dư (rule: chỉ select field FE cần):
//   - SlideOver chi tiết dịch vụ điều trị (FE `treatment-service-detail-panel`);
//   - form thanh toán (`form-payment.ts`) — cần `totalEnd`, `vat_amount`, `payment`, `refundServiceUsed`, `created_day`.
//
// ⛔ KHÔNG thêm lại nhóm hoa hồng (`commission_*_snap`): tỷ lệ hoa hồng không hiển thị ở màn nào trong hai màn trên, để trong
// response là rò rỉ cho bất kỳ ai mở được danh sách dịch vụ. Cần hiển thị hoa hồng → làm endpoint
// riêng có gác quyền, đừng nới chuỗi này.
const treatmentServiceDetailColumns = `id, code, customer_id, service_id, branch_id,
	quantity,
	unit_price, service_discount, service_discount_type, discount_other,
	discountServiceLock,
	totalEnd, vat_amount, payment, refundServiceUsed,
	isLock, installment_config, note, note_discount,
	created_day, activated_at, dayComplete, dayReExploitation,
	created_by, created_at, updated_by, updated_at`

// GetTreatmentService GET /crm/order-service-used/:id
func (h *Handler) GetTreatmentService(c *gin.Context) {
	// `ParseID` false = id rỗng/không phải số/bằng 0 ⇒ input SAI, trả 422.
	// KHÔNG dùng 404 ở đây: interceptor FE coi MỌI 404 là lỗi toàn cục (điều hướng + toast toàn app),
	// nên một lần gọi với id lỗi sẽ nổ ra như hệ thống hỏng. 404 chỉ dành cho id hợp lệ nhưng không
	// tồn tại / ngoài tenant — đúng nhánh `len(items) == 0` bên dưới.
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_id"))
		return
	}

	var items []map[string]interface{}
	tx := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_service_used", tenant.IsCrossBranchServiceEnabled(c, h.DB)).
		Table("order_service_used").
		Select(treatmentServiceDetailColumns).
		Where("id = ?", id).Limit(1).Find(&items)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	if len(items) == 0 {
		response.NotFound(c)
		return
	}
	helpers.ParseDecimalFieldsForTables(items, "order_service_used")
	helpers.ParseJSONFieldsForTables(items, "order_service_used")

	response.Data(c, items[0])
}

// attachRelationReverse — story-33: gộp về handlerutil.AttachRelationReverse.

// GetServiceUsedOfCustomer GET /crm/order-service-used/:id/get-service-used-of-customer
func (h *Handler) GetServiceUsedOfCustomer(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	var item map[string]interface{}
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_service_used", tenant.IsCrossBranchServiceEnabled(c, h.DB)).Table("order_service_used").
		Where("id = ?", id).Limit(1).Scan(&item).Error; err != nil {
		response.NotFound(c)
		return
	}
	helpers.ParseJSONFieldForTable(item, "order_service_used")
	// Cột DECIMAL scan vào map là []uint8 → parse để JSON ra number.
	helpers.ParseDecimalFieldsForTables([]map[string]interface{}{item}, "order_service_used")

	// Luôn attach service (scoped theo tenant)
	serviceID := helpers.ToInt64(item["service_id"])
	if serviceID > 0 {
		var svc map[string]interface{}
		tx := tenant.ScopedDBNullBranch(c, h.DB, "service").Table("service").
			Where("id = ?", serviceID).Limit(1).Scan(&svc)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		helpers.ParseJSONFieldForTable(svc, "service")
		helpers.ParseDecimalFieldsForTables([]map[string]interface{}{svc}, "service")
		item["service"] = svc
	}

	response.Data(c, item)
}

// LockServiceUsed POST /crm/order-service-used/:id/lock-service-used
func (h *Handler) LockServiceUsed(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}

	var body struct {
		IsLock    int    `json:"isLock"`
		UpdatedAt *int64 `json:"updated_at"` // khoá lạc quan (D6); nil = client không gửi
	}
	helpers.BindJSON(c, &body)

	// Load current updated_at — scope tenant để chống IDOR
	var current struct {
		UpdatedAt int64 `gorm:"column:updated_at"`
	}
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_service_used", tenant.IsCrossBranchServiceEnabled(c, h.DB)).Table("order_service_used").
		Select("updated_at").Where("id = ?", id).First(&current).Error; err != nil {
		response.NotFound(c)
		return
	}

	// Enforce updated_at required
	if body.UpdatedAt == nil {
		response.Validate(c, i18n.T(c, "version_missing"))
		return
	}
	if *body.UpdatedAt != current.UpdatedAt {
		response.Conflict(c, errs.ErrVersionConflict.Error())
		return
	}

	var item map[string]any
	tx := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_service_used", tenant.IsCrossBranchServiceEnabled(c, h.DB)).Table("order_service_used").
		Where("id = ?", id).Scan(&item)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	if item == nil {
		response.NotFound(c)
		return
	}

	totalEnd := helpers.ToDecimal(item["totalEnd"])
	vatAmount := helpers.ToDecimal(item["vat_amount"])
	payment := helpers.ToDecimal(item["payment"])
	currentIsLock := int(helpers.ToDecimal(item["isLock"]).IntPart())

	updates := map[string]any{}
	msg := ""

	// Logic giống PHP: FE truyền isLock hiện tại, BE làm ngược lại
	// body.isLock == IS_LOCK → mở khoá | body.isLock != IS_LOCK → khoá
	var isLockNew int
	if body.IsLock == define.ServiceUsedIsLock {
		// Đang khoá → mở khoá
		msg = "Mở khoá dịch vụ thành công"
		isLockNew = define.ServiceUsedNotLock
		updates["isLock"] = define.ServiceUsedNotLock
		restored := totalEnd.Add(helpers.ToDecimal(item["discountServiceLock"]))
		updates["totalEnd"] = restored
		updates["discountServiceLock"] = decimal.Zero
		// VAT tính lại theo mức VAT HIỆN TẠI của đơn trên totalEnd khôi phục (bmad/order-vat D7).
		updates["vat_amount"] = crmService.OrderServiceUsedVat(h.DB, int(helpers.ToDecimal(item["order_plan_id"]).IntPart()), restored)
	} else {
		// Chưa khoá → khoá: chốt phải thu = đã thu. Đã thu là tiền CÓ thuế ⇒ chia theo tỷ lệ dòng thành phần
		// trước thuế (totalEnd mới) + phần thuế (vat_amount mới); discountServiceLock = phần trước thuế chưa thu.
		msg = "Khoá dịch vụ thành công"
		isLockNew = define.ServiceUsedIsLock
		updates["isLock"] = define.ServiceUsedIsLock
		lockedTotalEnd, lockedVat := crmService.LockSplit(totalEnd, vatAmount, payment)
		updates["discountServiceLock"] = totalEnd.Sub(lockedTotalEnd)
		updates["totalEnd"] = lockedTotalEnd
		updates["vat_amount"] = lockedVat
	}

	// Guard: nếu trạng thái không đổi → bỏ qua
	if isLockNew == currentIsLock {
		c.JSON(http.StatusOK, "Cập nhật thành công")
		return
	}

	updates["updated_at"] = time.Now().Unix()

	customerId := item["customer_id"]

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		// Khoá khách TRƯỚC (cùng thứ tự với luồng phiếu thu: customer → dịch vụ — crm/service/payment_lock.go)
		// để tổng tiền tính lại bên dưới đọc bản mới nhất (lens-money M-2).
		if customerId != nil {
			var locked []int
			if err := tx.Raw("SELECT id FROM customer WHERE id = ? FOR UPDATE", customerId).Scan(&locked).Error; err != nil {
				return err
			}
		}
		// isLock là tinyint(1), GORM tự map sang bool nên phải dùng Exec để set đúng giá trị số.
		// Khoá lạc quan D6: bump GREATEST(now, updated_at+1) + AND updated_at=? guard chống race window.
		result := tx.Exec("UPDATE order_service_used SET isLock=?, discountServiceLock=?, totalEnd=?, vat_amount=?, updated_at=GREATEST(?, updated_at + 1) WHERE id=? AND updated_at=?",
			isLockNew, updates["discountServiceLock"], updates["totalEnd"], updates["vat_amount"], updates["updated_at"], id, current.UpdatedAt)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errs.ErrVersionConflict
		}

		// --- afterSave logic giống PHP OrderServiceUsed::afterSave ---
		// Khi khoá/mở khoá, totalEnd thay đổi → cần cập nhật Customer: totalRevenue, totalDebt.
		// (crm-standalone story-14: bỏ cập nhật doanh thu buổi điều trị + hoa hồng bác sĩ cùng liệu trình.)

		// Tính lại debtsPlan đơn + totalRevenue/totalDebt khách từ nguồn (lens-money M-14 — bản cũ chỉ tính khách).
		return crmService.RecalcAfterServiceUsedLock(tx,
			int(helpers.ToDecimal(item["order_plan_id"]).IntPart()), int(helpers.ToDecimal(customerId).IntPart()))
	})

	if err != nil {
		response.HandleError(c, err) // auto 409 khi ErrVersionConflict (race window)
		return
	}
	c.JSON(http.StatusOK, msg)
}

// formatVND format số tiền VND
// ServiceUseds GET /crm/order-service-used/service-useds?customerId=X
func (h *Handler) ServiceUseds(c *gin.Context) {
	customerId := c.Query("customerId")
	// FE cũ còn gửi thêm query companyId — bị bỏ qua (story-37); phạm vi theo branch scope bên dưới.
	if customerId == "" {
		c.JSON(http.StatusOK, []any{})
		return
	}

	var rows []map[string]any
	tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_service_used", tenant.IsCrossBranchServiceEnabled(c, h.DB), "osu").Table("order_service_used osu").
		Select("osu.id, osu.service_id, s.name").
		Joins("LEFT JOIN service s ON s.id = osu.service_id").
		Where("osu.customer_id = ? AND osu.isLock = 1", customerId).
		Find(&rows)

	result := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		name, _ := r["name"].(string)
		result = append(result, gin.H{"id": r["service_id"], "name": name})
	}
	c.JSON(http.StatusOK, result)
}

// ServiceUsedOfCustomer GET /crm/order-service-used/service-used-of-customer?idCustomer=X
func (h *Handler) ServiceUsedOfCustomer(c *gin.Context) {
	idCustomer, errParse := strconv.ParseUint(c.Query("idCustomer"), 10, 64)
	// Parse ra SỐ: MySQL ép '12abc' → 12 (warning 1292 bị nuốt) ⇒ URL rác đọc trúng dữ liệu khách khác.
	if errParse != nil || idCustomer == 0 {
		c.JSON(http.StatusOK, []any{})
		return
	}
	var rows []map[string]any
	tx := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_service_used", tenant.IsCrossBranchServiceEnabled(c, h.DB), "osu").Table("order_service_used osu").
		Select("osu.id, s.name as serviceName, osu.order_plan_id, osu.branch_id, "+
			"COALESCE(osu.totalEnd, 0) + osu.vat_amount AS payable, osu.payment, osu.created_day").
		Joins("LEFT JOIN service s ON s.id = osu.service_id").
		// Còn nợ theo số PHẢI THU = totalEnd + VAT (bmad/order-vat D4).
		Where("osu.customer_id = ? AND COALESCE(osu.totalEnd, 0) + osu.vat_amount != osu.payment", idCustomer).
		Order("osu.id DESC").Limit(500).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	helpers.ParseDecimalFieldsForTables(rows, "order_service_used")

	result := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		payable := helpers.ToDecimal(r["payable"])
		payment := helpers.ToDecimal(r["payment"])
		money := payable.Sub(payment)
		svcName, _ := r["serviceName"].(string)
		label := fmt.Sprintf("%s - %s", svcName, helpers.FormatDecimal(h.DB, money))
		result = append(result, gin.H{
			"value": r["id"], "label": label, "number": money,
			"branch_id": r["branch_id"], "parent_id": r["order_plan_id"],
			"created_day": r["created_day"],
		})
	}
	c.JSON(http.StatusOK, result)
}
