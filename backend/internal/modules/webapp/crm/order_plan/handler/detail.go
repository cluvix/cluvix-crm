package handler

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"erpCluvix/pkg/define"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"
)

// orderPlanDetailColumns — cột của ĐƠN HÀNG cho màn chi tiết đơn.
//
// KHÔNG select `total`, `revenue_id`, `doctorRevenue_id`: 3 cột này đã bị DROP
// (migration 20260601221056 + đợt bỏ FK revenue/doctorRevenue, nay nằm trong squash 20260814091602).
// Tổng tiền đơn tính từ tổng `totalEnd` của các dịch vụ con, xem `total` trong response.
const orderPlanDetailColumns = `id, code, old_code, orderPlanType, customer_id, appointment_id,
	branch_id, source_id,
	discount_other, vat_rate, paymentPlan, debtsPlan,
	listRevenues,
	status, note, created_time,
	created_by, created_at, updated_by, updated_at`

// orderPlanServiceColumns — cột của từng DỊCH VỤ CON trong đơn.
//
// ⛔ KHÔNG lấy nhóm hoa hồng (`commission_*_snap`): màn chi tiết đơn không hiển thị hoa hồng, để
// trong response là rò rỉ tỷ lệ hoa hồng cho mọi người xem được đơn.
const orderPlanServiceColumns = `id, code, service_id,
	quantity,
	unit_price, service_discount, discount_other,
	discountServiceLock,
	totalEnd, vat_amount, payment, refundServiceUsed,
	isLock,
	listRevenues`

// GetOrderPlanDetail GET /crm/order-plan/:id/detail
//
// Endpoint RIÊNG cho màn chi tiết đơn hàng — cố ý KHÔNG dùng lại `GET /crm/order-plan/:id`:
//   - endpoint kia đang `SELECT *` cả đơn lẫn dịch vụ con và có ≥6 consumer (form thanh toán đơn,
//     form sửa đơn, đổi dịch vụ, EMR order list, kế toán…); siết cột ở đó là rủi ro mất field âm thầm;
//   - đơn hàng là bản ghi CHA có nhiều dịch vụ con, khác hẳn `order_service_used` (một dịch vụ lẻ) —
//     hai màn cần hai hình dạng dữ liệu khác nhau, gộp chung sẽ kéo theo cột thừa cho cả hai.
//
// Response giữ đúng tên khoá mà FE `OrderPlanService.addLabel` đang dùng
// (`orderServiceUseds`, `total`, `payment`, `refundServiceTotal`). VAT (bmad/order-vat): `total` giữ tiền TRƯỚC
// thuế (Σ totalEnd); thêm `vat_total` (Σ vat_amount) và `total_after_vat` = phải thu của đơn. story-10 crm-standalone: bỏ 2 khoá
// mã giảm giá của đơn + CTV giới thiệu cùng module loyalty.
func (h *Handler) GetOrderPlanDetail(c *gin.Context) {
	// id sai định dạng là input SAI → 422. 404 dành riêng cho id hợp lệ nhưng không tồn tại/ngoài
	// tenant: interceptor FE coi MỌI 404 là lỗi toàn cục (điều hướng + toast toàn app).
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_id"))
		return
	}

	var items []map[string]interface{}
	tx := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_plan", tenant.IsCrossBranchServiceEnabled(c, h.DB)).
		Table("order_plan").
		Select(orderPlanDetailColumns).
		Where("id = ?", id).Limit(1).Find(&items)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	if len(items) == 0 {
		response.NotFound(c)
		return
	}
	helpers.ParseJSONFieldsForTables(items, "order_plan")
	helpers.ParseDecimalFieldsForTables(items, "order_plan")
	item := items[0]

	// Dịch vụ con của đơn — CỐ Ý không lọc chi nhánh (tenant.CrossBranchLookup, story-37): dịch vụ
	// trong cùng đơn có thể thuộc chi nhánh khác (điều trị chéo chi nhánh), lọc theo branch sẽ làm
	// mất dòng của chính đơn đang xem (đơn đã qua branch scope ở trên).
	services := make([]map[string]interface{}, 0)
	txSvc := tenant.CrossBranchLookup(h.DB, "order_service_used").
		Table("order_service_used").
		Select(orderPlanServiceColumns).
		Where("order_plan_id = ?", id).
		Order("id ASC").
		Find(&services)
	if pagination.IsQueryError(txSvc.Error) {
		pagination.FailQuery(c, txSvc.Error)
		return
	}
	helpers.ParseJSONFieldsForTables(services, "order_service_used")
	helpers.ParseDecimalFieldsForTables(services, "order_service_used")
	item["orderServiceUseds"] = services

	total, vatTotal := sumOrderPlanServices(services)
	item["total"] = total
	item["vat_total"] = vatTotal
	item["total_after_vat"] = total.Add(vatTotal)

	if !h.attachOrderPlanMoney(c, item, id) {
		return
	}

	response.Data(c, item)
}

// sumOrderPlanServices — Σ totalEnd (giá trị đơn TRƯỚC thuế) và Σ vat_amount (VAT đơn) của dịch vụ con. Dùng chung
// cho GET /:id và /:id/detail để `total`/`vat_total` hai màn không lệch nhau.
func sumOrderPlanServices(services []map[string]interface{}) (total, vatTotal decimal.Decimal) {
	for _, svc := range services {
		total = total.Add(helpers.ToDecimal(svc["totalEnd"]))
		vatTotal = vatTotal.Add(helpers.ToDecimal(svc["vat_amount"]))
	}
	return total, vatTotal
}

// attachOrderPlanMoney tính tiền đã thu + đã hoàn của đơn (đơn `id` đã qua branch scope ở caller).
//
// Story-37 (E6/D15): bỏ vế company trong subquery — 1 thương hiệu/DB (DB-per-customer), mọi phiếu/
// dịch vụ của đơn đều thuộc thương hiệu đó nên vế company không còn phân tách gì.
func (h *Handler) attachOrderPlanMoney(c *gin.Context, item map[string]interface{}, id uint) bool {
	var agg struct {
		Payment            decimal.Decimal `gorm:"column:payment"`
		RefundServiceTotal decimal.Decimal `gorm:"column:refund_service_total"`
	}

	// Phiếu thu tính vào đơn = thu cho đơn + chuyển cọc sang đơn. Dùng hằng số `pkg/define`, KHÔNG
	// viết 4/5 trần: đổi nghĩa `payment_for` mà sót một chỗ là lệch tiền âm thầm.
	paymentForOrder := fmt.Sprintf("%d, %d", define.PaymentForOrder, define.PaymentForMoveDeposit)
	aggSQL := `SELECT
		COALESCE((SELECT SUM(amount) FROM payment WHERE parent_id = ? AND payment_for IN (` + paymentForOrder + `)), 0) AS payment,
		COALESCE((SELECT SUM(refundServiceUsed) FROM order_service_used WHERE order_plan_id = ?), 0) AS refund_service_total`

	tx := h.DB.Raw(aggSQL, id, id).Scan(&agg)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return false
	}
	item["payment"] = agg.Payment
	item["refundServiceTotal"] = agg.RefundServiceTotal
	return true
}
