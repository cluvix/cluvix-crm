package handler

import (
	"net/http"
	"strings"

	"erpCluvix/internal/modules/webapp/crm/customer/scope"
	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/handlerutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Danh sách + export đơn hàng cho màn CRM › Quản lý đơn hàng — port từ ERP
// `accountant/order_plan` (GET /accountant/order-plan, 2026-09-14) về `GET /crm/order-plan`.
//
// Khác ERP (cố ý):
//   - 1 thương hiệu/DB: KHÔNG còn cột/bộ lọc `company_id`.
//   - Bảng `order_treatment_plan` ⇒ `order_plan`; dịch vụ con khoá theo `order_service_used.order_plan_id`.
//   - Schema CRM không có `listDoctors`/`listDirectSales`/`serviceCombo_id`/`version` và bảng
//     `coupon_used_by_order_plan` ⇒ bỏ cột/bộ lọc `_listDirectSales`/relation coupon tương ứng.
//   - Từ khoá: LIKE thuần (KHÔNG đường ngram `customer.search_text`, KHÔNG index hint). Vế khách đi
//     qua subquery `customer_id IN (SELECT ...)` thay vì `LEFT JOIN customer` — export
//     (`exportHandlers.ExportOrderPlan`) tự JOIN `customer`, base query mà cũng JOIN thì trùng alias.

const orderPlanTable = "order_plan"

// orderPlanKeywordCustomerCols — cột khách được tìm bằng từ khoá (cùng bộ 5 cột của đường LIKE ERP).
var orderPlanKeywordCustomerCols = []string{"customer.fullname", "customer.code", "customer.phone", "customer.nickname", "customer.other_phone"}

// orderPlanKeywordCond dựng điều kiện từ khoá: mã/ghi chú đơn LIKE, hoặc khách khớp LIKE trên 5 cột.
// Câu khách KHÔNG scope chi nhánh — CỐ Ý (FK lookup, giữ đúng tập kết quả ERP); đơn hàng đã bị
// scope chi nhánh đầy đủ ở truy vấn ngoài.
func orderPlanKeywordCond(kw string) (string, []interface{}) {
	like := "%" + kw + "%"
	custConds := make([]string, len(orderPlanKeywordCustomerCols))
	args := []interface{}{like, like}
	for i, col := range orderPlanKeywordCustomerCols {
		custConds[i] = col + " LIKE ?"
		args = append(args, like)
	}
	cond := "(order_plan.code LIKE ? OR order_plan.note LIKE ? OR order_plan.customer_id IN " +
		"(SELECT customer.id FROM customer WHERE " + strings.Join(custConds, " OR ") + "))"
	return cond, args
}

// orderPlanFilterBase: tenant scope + từ khoá + filters, CHƯA sort/select. Dùng chung cho câu đếm,
// câu trang và export để 3 nơi cùng một bộ lọc.
//
// S-10: phạm vi telesale (khách mình phụ trách) áp qua `order_plan.customer_id IN (khách trong
// phạm vi)`; đọc phạm vi lỗi ⇒ trả error (fail-closed), caller dừng bằng 500.
func (h *Handler) orderPlanFilterBase(c *gin.Context) (*gorm.DB, error) {
	// Danh sách + export TOÀN hệ thống ⇒ GIỮ lọc chi nhánh. Khác GetOrderPlan/sửa/xoá/in theo id (bỏ lọc
	// branch để dùng đơn đã mua ở chi nhánh khác): mở list tổng = lộ doanh thu, khách của chi nhánh khác.
	q := tenant.ScopedDBNullBranch(c, h.DB, orderPlanTable).
		Table(orderPlanTable)

	if kw := strings.TrimSpace(c.Query("_keyword")); kw != "" {
		cond, args := orderPlanKeywordCond(kw)
		q = q.Where(cond, args...)
	}

	q = query.ApplyFilter(c, q, "customer_id", "order_plan.customer_id")
	q = query.ApplyFilter(c, q, "branch_id", "order_plan.branch_id")
	q = query.ApplyFilter(c, q, "orderPlanType", "order_plan.orderPlanType")
	q = query.ApplyFilter(c, q, "source_id", "order_plan.source_id")
	q = query.ApplyFilter(c, q, "created_by", "order_plan.created_by")
	q = query.ApplyFilter(c, q, "status", "order_plan.status")
	q = query.ApplyFilter(c, q, "created_time", "order_plan.created_time")
	q = query.ApplyFilter(c, q, "created_at", "order_plan.created_at")
	return scope.ApplyCustomerID(c, h.DB, q, "order_plan.customer_id")
}

func applyOrderPlanSort(c *gin.Context, q *gorm.DB) *gorm.DB {
	return query.ApplySort(c, q, "order_plan.id DESC", "order_plan.id")
}

// ListOrderPlan GET /crm/order-plan — danh sách đơn hàng (kèm `?_export=1` ⇒ file Excel).
func (h *Handler) ListOrderPlan(c *gin.Context) {
	base, err := h.orderPlanFilterBase(c)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	if c.Query("_export") == "1" {
		exportHandlers.ExportOrderPlan(c, applyOrderPlanSort(c, base))
		return
	}

	// Đếm THẲNG trên bộ lọc — KHÔNG để Paginate bọc câu SELECT có subquery tương quan SUM(totalEnd)
	// (ERP đo @600k đơn: 3.280 → 118ms).
	var total int64
	tx := base.Select("COUNT(*)").Scan(&total)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	pageBase, err := h.orderPlanFilterBase(c)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	q := applyOrderPlanSort(c, pageBase).
		Select(`order_plan.id, order_plan.code, order_plan.orderPlanType,
			order_plan.customer_id, order_plan.appointment_id, order_plan.branch_id,
			COALESCE((SELECT SUM(osu.totalEnd) FROM order_service_used osu WHERE osu.order_plan_id = order_plan.id), 0) AS total,
			COALESCE((SELECT SUM(osu.vat_amount) FROM order_service_used osu WHERE osu.order_plan_id = order_plan.id), 0) AS vat_total,
			order_plan.vat_rate,
			order_plan.discount_other,
			order_plan.paymentPlan, order_plan.debtsPlan,
			order_plan.listRevenues, order_plan.source_id,
			order_plan.note, order_plan.created_time, order_plan.status,
			order_plan.created_by, order_plan.created_at,
			order_plan.updated_by, order_plan.updated_at`)

	var items []map[string]interface{}
	result := pagination.PaginateWithTotal(c, q, &items, total)
	if c.Writer.Written() {
		return // PaginateWithTotal đã trả lỗi query
	}

	for _, item := range items {
		// `total`/`vat_total` là alias (không phải cột của order_plan) ⇒ Paginate không parse decimal hộ.
		helpers.ParseDecimalFields(item, "total", "vat_total")
		// VAT (bmad/order-vat): `total` giữ tiền TRƯỚC thuế; `total_after_vat` = phải thu của đơn.
		item["total_after_vat"] = helpers.ToDecimal(item["total"]).Add(helpers.ToDecimal(item["vat_total"]))
		// FE dùng model.payment (alias cho paymentPlan).
		item["payment"] = item["paymentPlan"]
	}

	if len(items) > 0 {
		if customerIDs := handlerutil.CollectIDs(items, "customer_id"); len(customerIDs) > 0 {
			// FK lookup — CỐ Ý không lọc chi nhánh (AttachRelation = CrossBranchLookupScope).
			handlerutil.AttachRelation(c, h.DB, items, "customer_id", "customer",
				"customer", "id", "id, code, fullname, nickname, phone, avatar, stage_id, province_id", customerIDs)
			phoneutil.MaskNestedCustomerPhone(c, h.DB, items, "customer")
		}
		if ids := handlerutil.CollectIDs(items, "id"); len(ids) > 0 {
			// Dịch vụ con của đơn — CỐ Ý không lọc chi nhánh (giống GetOrderPlan, story-37): dịch vụ trong
			// cùng đơn có thể ở chi nhánh khác; đơn đã qua branch scope ở trên.
			var svcRows []map[string]interface{}
			tx := tenant.CrossBranchLookup(h.DB, "order_service_used").Table("order_service_used").
				Select(`id, order_plan_id, service_id, quantity, unit_price, service_discount,
					discount_other, totalEnd, vat_amount, payment, status, isLock`).
				Where("order_plan_id IN ?", ids).Order("id ASC").Find(&svcRows)
			if pagination.IsQueryError(tx.Error) {
				pagination.FailQuery(c, tx.Error)
				return
			}
			helpers.ParseDecimalFieldsForTables(svcRows, "order_service_used")
			svcMap := map[int64][]map[string]interface{}{}
			for _, r := range svcRows {
				key := helpers.ToInt64(r["order_plan_id"])
				svcMap[key] = append(svcMap[key], r)
			}
			for _, item := range items {
				if rows, ok := svcMap[helpers.ToInt64(item["id"])]; ok {
					item["orderServiceUseds"] = rows
				} else {
					item["orderServiceUseds"] = []map[string]interface{}{}
				}
			}
		}
	}

	c.JSON(http.StatusOK, result)
}
