package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/internal/modules/webapp/crm/customer/scope"
	customerDetailSvc "erpCluvix/internal/modules/webapp/crm/customer_detail/service"
	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/handlerutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"
)

// --- Appointment by Customer ---

// GetAppointmentByCustomer GET /crm/appointment/get-by-customer?idCustomer=xxx
// Endpoint cho AppointmentInCustomer - explicit SELECT, không expand/join
func (h *Handler) GetAppointmentByCustomer(c *gin.Context) {
	idCustomer, errParse := strconv.ParseUint(c.Query("idCustomer"), 10, 64)
	// Parse ra SỐ: MySQL ép '12abc' → 12 (warning 1292 bị nuốt) ⇒ URL rác đọc trúng dữ liệu khách khác.
	if errParse != nil || idCustomer == 0 {
		response.Validate(c, i18n.T(c, "missing_id_customer_param"))
		return
	}

	q := tenant.ScopedDBNullBranch(c, h.DB, "appointment").Table("appointment").
		Where("appointment.customer_id = ?", idCustomer)
	// S-10: phạm vi telesale — khách ngoài phạm vi ⇒ rỗng; lỗi đọc phạm vi ⇒ 500 (fail-closed).
	q = scope.WhereCustomerID(c, h.DB, q, "appointment.customer_id")

	q = query.ApplySort(c, q, "appointment.appointment_date DESC", "appointment.appointment_date")

	// SELECT explicit fields - tất cả labels FE lấy từ IndexedDB, không cần join
	q = q.Select(`appointment.id, appointment.code, appointment.customer_id,
		appointment.status, appointment.bookType, appointment.confirmAppointment,
		appointment.appointment_type_id, appointment.appointment_date,
		appointment.schedule_time,
		appointment.content, appointment.contentPrivate,
		appointment.checkin_date, appointment.checkout_date,
		appointment.appointment_number,
		appointment.type_service_id, appointment.service_id,
		appointment.type_service_accepted_id,
		appointment.is_accepted,
		appointment.appointment_reason_id, appointment.noteAppointment,
		appointment.convert_id,
		appointment.revenue_id, appointment.source_id,
		appointment.branch_id,
		appointment.created_by, appointment.created_at,
		appointment.updated_at`)

	var items []map[string]interface{}
	result := pagination.Paginate(c, q, &items)
	c.JSON(http.StatusOK, result)
}

// --- Service Used by Customer ---

// ListServiceUsedCustomer GET /crm/order-service-used/list-service-used-customer?id=xxx
// Endpoint cho OrderPlanInCustomer - explicit SELECT, không expand
func (h *Handler) ListServiceUsedCustomer(c *gin.Context) {
	id, errParse := strconv.ParseUint(c.Query("id"), 10, 64)
	// Parse ra SỐ: MySQL ép '12abc' → 12 (warning 1292 bị nuốt) ⇒ URL rác đọc trúng dữ liệu khách khác.
	if errParse != nil || id == 0 {
		response.Validate(c, i18n.T(c, "param_id_required"))
		return
	}

	q := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_service_used", true).
		Table("order_service_used").
		Where("order_service_used.customer_id = ?", id)
	q = scope.WhereCustomerID(c, h.DB, q, "order_service_used.customer_id") // S-10

	if raw := c.Query("appointment_id"); raw != "" {
		// Parse ra SỐ: MySQL ép '12abc' → 12 (warning 1292 bị nuốt) ⇒ URL rác lọc trúng lịch hẹn khác.
		apptID, errAppt := strconv.ParseUint(raw, 10, 64)
		if errAppt != nil || apptID == 0 {
			response.Validate(c, i18n.T(c, "invalid_request"))
			return
		}
		// Không lọc company trong subquery — outer query đã scope tenant qua
		// ScopedDBNullBranchSkipBranch rồi; lọc company thêm ở đây (đặc biệt khi
		// companyID=0 lúc chưa switch company active) gây bỏ sót treatment_plan
		// có company_id thật, chỉ khớp company_id IS NULL.
		q = q.Where(
			"order_service_used.appointment_id = ? OR (order_service_used.appointment_id IS NULL AND order_service_used.order_plan_id IN (SELECT id FROM order_plan WHERE appointment_id = ?))",
			apptID, apptID,
		)
	}

	// Tie-breaker `id ASC` nằm TRONG defaultOrder, không phải `.Order()` rời phía sau:
	// `ApplySort` tự nối `<bảng gốc>.id DESC` khi defaultOrder chưa có cột đó (query.withTiebreaker,
	// 2026-09-06) ⇒ để rời sẽ ra `created_day DESC, id DESC, id ASC` — MySQL đã xác định thứ tự
	// ngay ở `id DESC` nên vế `id ASC` thành vô nghĩa và dòng cùng đơn bị đảo ngược thứ tự tạo.
	q = query.ApplySort(c, q, "order_service_used.created_day DESC, order_service_used.id ASC", "order_service_used.created_day", "order_service_used.order_plan_id", "order_service_used.id")

	// SELECT explicit fields - chỉ lấy field FE hiển thị + logic cần
	q = q.Select(`order_service_used.id, order_service_used.code,
		order_service_used.customer_id, order_service_used.order_plan_id,
		order_service_used.service_id, order_service_used.listRevenues,
		order_service_used.branch_id, order_service_used.source_id,
		order_service_used.quantity,
		order_service_used.totalEnd, order_service_used.vat_amount, order_service_used.payment,
		order_service_used.refundServiceUsed,
		order_service_used.isLock,
		order_service_used.created_day,
		order_service_used.created_by, order_service_used.created_at,
		order_service_used.updated_by, order_service_used.updated_at`)

	var items []map[string]interface{}
	result := pagination.Paginate(c, q, &items)
	helpers.ParseJSONFieldsForTables(items, "order_service_used")

	if len(items) > 0 {
		planIds := handlerutil.CollectIDs(items, "order_plan_id")
		if len(planIds) > 0 {
			// FK lookup CỐ Ý không lọc chi nhánh — story-36: CrossBranchLookup thay mặc định CompanyOnly.
			handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "order_plan_id", "treatmentPlan",
				"order_plan", "id", "id, code, note, orderPlanType, paymentPlan", planIds)
			for _, item := range items {
				if tp, ok := item["treatmentPlan"].(map[string]interface{}); ok {
					helpers.ParseDecimalFields(tp, "paymentPlan")
				}
			}
		}
		// BE-075: gắn order_type + is_paraclinical từ service → service_type
		if err := attachOrderType(c, h.DB, items); pagination.IsQueryError(err) {
			pagination.FailQuery(c, err)
			return
		}
	}

	c.JSON(http.StatusOK, result)
}

// attachOrderType bổ sung order_type và is_paraclinical vào mỗi order item.
// Batch query service → service_type theo service_id để tránh N+1.
//
// FK lookup CỐ Ý không lọc chi nhánh (dịch vụ dùng chung mọi chi nhánh) — tenant.CrossBranchLookup
// (story-36 thay helper company-only cũ: D15 — 1 thương hiệu/DB, vế company không còn ý nghĩa).
//
// Trả error thay vì nuốt: `.Find` hỏng mà bỏ qua thì MỌI dòng rơi về `order_type = "other"` và
// `is_paraclinical = false` — FE hiển thị sai loại chỉ định mà không có lỗi nào nổi lên.
func attachOrderType(c *gin.Context, db *gorm.DB, items []map[string]interface{}) error {
	serviceIds := handlerutil.CollectIDs(items, "service_id")
	if len(serviceIds) == 0 {
		return nil
	}

	type row struct {
		ServiceID uint   `gorm:"column:service_id"`
		TypeName  string `gorm:"column:type_name"`
	}
	var rows []row
	if err := tenant.CrossBranchLookup(db, "service", "s").
		Table("service s").
		Select("s.id as service_id, st.name as type_name").
		Joins("LEFT JOIN service_type st ON st.id = s.service_type_id").
		Where("s.id IN ?", serviceIds).
		Find(&rows).Error; err != nil {
		return err
	}

	svcMap := make(map[uint]row, len(rows))
	for _, r := range rows {
		svcMap[r.ServiceID] = r
	}

	paraclinicalTypes := map[string]bool{"lab": true, "imaging": true, "function_test": true}

	for _, item := range items {
		sid, _ := item["service_id"].(uint)
		if sid == 0 {
			if v, ok := item["service_id"]; ok {
				switch n := v.(type) {
				case int64:
					sid = uint(n)
				case float64:
					sid = uint(n)
				}
			}
		}
		r, ok := svcMap[sid]
		if !ok {
			item["order_type"] = "other"
			item["is_paraclinical"] = false
			continue
		}
		orderType := deriveOrderType(r.TypeName)
		item["order_type"] = orderType
		item["is_paraclinical"] = paraclinicalTypes[orderType]
	}
	return nil
}

// deriveOrderType: phân loại chỉ định theo keyword trong tên loại dịch vụ (cấu hình loại chỉ định EMR
// trên service_type đã bỏ ở story-12 crm-standalone).
func deriveOrderType(typeName string) string {
	s := strings.ToLower(typeName)
	containsAny := func(keywords ...string) bool {
		for _, kw := range keywords {
			if strings.Contains(s, kw) {
				return true
			}
		}
		return false
	}
	switch {
	case containsAny("xét nghiệm", "sinh hóa", "huyết học", "vi sinh", "miễn dịch", "lab", "test"):
		return "lab"
	case containsAny("chẩn đoán hình ảnh", "cdha", "x quang", "xquang", "siêu âm", "ct scan", "mri", "pet", "ultrasound", "imaging"):
		return "imaging"
	case containsAny("thăm dò", "chức năng", "ecg", "ekg", "eeg", "nội soi", "loãng xương", "spirometry", "endoscopy"):
		return "function_test"
	case containsAny("thủ thuật", "phẫu thuật", "tiểu phẫu", "procedure", "surgery"):
		return "procedure"
	case containsAny("nha khoa", "niềng", "implant", "thẩm mỹ", "vật lý trị liệu", "châm cứu", "treatment", "therapy"):
		return "treatment"
	}
	return "other"
}

// --- Payment by Customer ---

// GetPaymentDetailForCustomer GET /crm/payment/get-payment-detail-for-customer?customer_id=xxx
// Endpoint cho PaymentOfCustomer - explicit SELECT, không expand, chỉ attach relation cần thiết cho FE
// paymentCustomerIdx / paymentForceCustomerIdx — xem ghi chú dưới.
const paymentCustomerIdx = "idx_payment_customer"

// paymentForceCustomerIdx: có ép MySQL đi theo chỉ mục `(customer_id)` hay không.
//
// Không ép thì optimizer quét NGƯỢC `PRIMARY` để cược vào early-exit của `LIMIT 20`. Khách nào có
// id payment CŨ (ngừng giao dịch lâu) là cược thua, đi gần hết bảng. Đo @605.319 payment, khách
// đối kháng 5.000 payment id cũ nhất, bộ đếm SESSION `Handler_read_*`:
//
//	ca                                trước      chỉ mục (auto)   chỉ mục + FORCE
//	1 chi nhánh                       600.677    5.938            358
//	toàn chi nhánh (người xem QL)     600.339    600.339 (!)      20
//	khách THƯỜNG (trần thật 29)       30         30               20
//
// Chỉ mục một mình KHÔNG đủ: ở ca toàn-chi-nhánh optimizer vẫn bỏ nó. Có FORCE thì
// 582–4.921ms -> 0,04–0,58ms.
//
// ⚠ TẮT khi có `appointment_id`: nhánh đó `idx-payment-appointment_id` chọn lọc hơn hẳn
// (20 dòng), FORCE ép bỏ nó ⇒ 4.072 dòng. (Bản thân việc THÊM chỉ mục cũng đưa nhánh này từ 20 lên
// ~1.277 dòng vì optimizer chuyển sang index-merge intersect — 0,05ms -> ~0,9ms, chấp nhận được;
// cột `payment.appointment_id` hiện NULL ở 100% dòng nên nhánh này thực tế chưa ai chạm.)
//
// ⚠ Vì sao FORCE an toàn ở đây mà KHÔNG an toàn ở `note/get-all-note`: predicate là ĐẲNG THỨC trên
// `customer_id`, nên phạm vi chỉ mục bị chặn trên bởi "số payment của MỘT khách" — đúng bằng ngữ
// nghĩa của endpoint. Không có tham số nào (như độ rộng cửa sổ ngày) làm nó phình ra.
//
// ⚠ Tên chỉ mục nằm trong SQL: đổi tên ⇒ MySQL lỗi 1176 (lớn tiếng), và
// `TestPaymentCustomerIdxKhopVoiMigration` đỏ ngay trong repo.
func paymentForceCustomerIdx(c *gin.Context) bool {
	return c.Query("appointment_id") == ""
}

func (h *Handler) GetPaymentDetailForCustomer(c *gin.Context) {
	customerID, errParse := strconv.ParseUint(c.Query("customer_id"), 10, 64)
	// Parse ra SỐ: MySQL ép '12abc' → 12 (warning 1292 bị nuốt) ⇒ URL rác đọc trúng dữ liệu khách khác.
	if errParse != nil || customerID == 0 {
		response.Validate(c, i18n.T(c, "customer_id_required"))
		return
	}

	table := "payment"
	if paymentForceCustomerIdx(c) {
		table = "payment FORCE INDEX (" + paymentCustomerIdx + ")"
	}
	q := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "payment", true).Table(table).
		Where("payment.customer_id = ?", customerID)
	q = scope.WhereCustomerID(c, h.DB, q, "payment.customer_id") // S-10

	if raw := c.Query("appointment_id"); raw != "" {
		// Parse ra SỐ: MySQL ép '12abc' → 12 (warning 1292 bị nuốt) ⇒ URL rác lọc trúng lịch hẹn khác.
		apptID, errAppt := strconv.ParseUint(raw, 10, 64)
		if errAppt != nil || apptID == 0 {
			response.Validate(c, i18n.T(c, "invalid_request"))
			return
		}
		q = q.Where("payment.appointment_id = ?", apptID)
	}

	q = query.ApplySort(c, q, "payment.id DESC", "payment.id")

	// SELECT explicit fields - chỉ lấy field FE hiển thị + field cần cho logic
	q = q.Select(`payment.id, payment.code, payment.customer_id,
		payment.parent_id, payment.serviceUsed_id,
		payment.branch_id,
		payment.method, payment.payment_time, payment.payment_of_date,
		payment.amount, payment.deposit_after, payment.deposit_type,
		payment.payment_for, payment.note,
		payment.created_by, payment.created_at,
		payment.updated_by, payment.updated_at`)

	var items []map[string]interface{}
	result := pagination.Paginate(c, q, &items)

	if len(items) > 0 {
		planIds := handlerutil.CollectIDs(items, "parent_id")
		serviceUsedIds := handlerutil.CollectIDs(items, "serviceUsed_id")

		// FK lookup CỐ Ý không lọc chi nhánh — story-36: CrossBranchLookup thay mặc định CompanyOnly.
		// treatmentPlan: FE dùng treatmentPlan?.code và treatmentPlan?.id
		if len(planIds) > 0 {
			handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "parent_id", "treatmentPlan",
				"order_plan", "id", "id, code", planIds)
		}
		// orderServiceUsed: FE dùng code hiển thị + service_id để lookup tên dịch vụ qua IndexedDB
		if len(serviceUsedIds) > 0 {
			handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "serviceUsed_id", "orderServiceUsed",
				"order_service_used", "id", "id, code, service_id, listRevenues, source_id", serviceUsedIds)
		}
	}

	c.JSON(http.StatusOK, result)
}

// --- Timeline ---

// TimelineCustomer GET /crm/customer/:id/timeline-customer
func (h *Handler) TimelineCustomer(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}

	// Customer info — scope theo tenant (chống IDOR timeline)
	// S-10: kèm phạm vi telesale — khách ngoài phạm vi ⇒ như không tồn tại.
	var customer map[string]interface{}
	if err := scope.WhereCustomerID(c, h.DB, tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer"), "customer.id").
		Select("id, fullname, phone, code, payment_user_id, source_id, created_at, created_day, created_by").
		Where("id = ?", id).Limit(1).Scan(&customer).Error; err != nil || len(customer) == 0 {
		response.Validate(c, i18n.T(c, "customer_not_found"))
		return
	}
	phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), []map[string]interface{}{customer})

	// Enrich customer: paymentUserId, source
	// `user_profile`/`source` KHÔNG nằm trong registry scope nên không có vế scope ở đây — nhưng lỗi
	// SQL thì vẫn phải nổi lên: nuốt nó chỉ làm tên hiển thị rỗng, giống hệt ca "chưa có hồ sơ".
	if puid := customer["payment_user_id"]; puid != nil {
		var name string
		if err := h.DB.Table("user_profile").Select("fullname").Where("user_id = ?", puid).Scan(&name).Error; pagination.IsQueryError(err) {
			pagination.FailQuery(c, err)
			return
		}
		customer["paymentUserId"] = map[string]interface{}{"fullname": name}
	}
	if sid := customer["source_id"]; sid != nil {
		var name string
		if err := h.DB.Table("source").Select("name").Where("id = ?", sid).Scan(&name).Error; pagination.IsQueryError(err) {
			pagination.FailQuery(c, err)
			return
		}
		customer["source"] = map[string]interface{}{"name": name}
	}

	// Khách đã qua scope ở trên; lịch sử của khách CỐ Ý không lọc chi nhánh (story-36:
	// CrossBranchLookup thay helper company-only cũ — D15 bỏ vế company).
	events, err := customerDetailSvc.BuildCustomerTimeline(customerDetailSvc.TimelineDeps{
		DB:              h.DB,
		AppointmentDB:   tenant.CrossBranchLookup(h.DB, "appointment"),
		ServiceDB:       tenant.CrossBranchLookup(h.DB, "service"),
		TreatmentPlanDB: tenant.CrossBranchLookup(h.DB, "order_plan"),
		PaymentDB:       tenant.CrossBranchLookup(h.DB, "payment"),
		ReminderDB:      tenant.CrossBranchLookup(h.DB, "reminder"),
		CustomerID:      id,
		Customer:        customer,
	})
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}

	response.Data(c, gin.H{"timeline": events, "customer": customer})
}
