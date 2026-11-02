package handler

import (
	"github.com/shopspring/decimal"

	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
)

// Danh sách + export đơn hàng (`ListOrderPlan`, GET /crm/order-plan): xem list.go.

// GetOrderPlan GET /crm/order-plan/:id
func (h *Handler) GetOrderPlan(c *gin.Context) {
	// Parse id MỘT lần ở đầu hàm: trước đây câu đầu dùng c.Param("id") dạng CHUỖI
	// (MySQL ép '12abc' → 12) còn câu sau đã parse — hai câu cùng hàm đọc id khác nhau.
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}

	var item map[string]interface{}
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_plan", true).Table("order_plan").
		Where("id = ?", id).Limit(1).Scan(&item).Error; err != nil {
		response.NotFound(c)
		return
	}
	if len(item) == 0 {
		response.NotFound(c)
		return
	}
	helpers.ParseJSONFieldForTable(item, "order_plan")
	helpers.ParseDecimalFieldsForTables([]map[string]interface{}{item}, "order_plan")

	// Dịch vụ con của đơn — CỐ Ý không lọc chi nhánh (tenant.CrossBranchLookup, story-37): dịch vụ
	// trong cùng đơn có thể ở chi nhánh khác (điều trị chéo chi nhánh); bản thân đơn cũng nạp KHÔNG lọc chi nhánh ở trên.
	var services []map[string]interface{}
	tx := tenant.CrossBranchLookup(h.DB, "order_service_used").Table("order_service_used").
		Where("order_plan_id = ?", id).Find(&services)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	helpers.ParseJSONFieldsForTables(services, "order_service_used")
	helpers.ParseDecimalFieldsForTables(services, "order_service_used")
	item["orderServiceUseds"] = services

	// VAT (bmad/order-vat): `total` giữ tiền TRƯỚC thuế; `total_after_vat` = phải thu của đơn.
	total, vatTotal := sumOrderPlanServices(services)
	item["total"] = total
	item["vat_total"] = vatTotal
	item["total_after_vat"] = total.Add(vatTotal)

	// payment + refundServiceTotal của đơn (đơn đã nạp ở trên, không lọc chi nhánh). Story-37 (D15): bỏ vế
	// company trong subquery — 1 thương hiệu/DB, mọi phiếu/dịch vụ của đơn đều thuộc thương hiệu đó.
	var agg struct {
		Payment            decimal.Decimal `gorm:"column:payment"`
		RefundServiceTotal decimal.Decimal `gorm:"column:refund_service_total"`
	}
	h.DB.Raw(`SELECT
		COALESCE((SELECT SUM(amount) FROM payment WHERE parent_id = ? AND payment_for IN (4, 5)), 0) AS payment,
		COALESCE((SELECT SUM(refundServiceUsed) FROM order_service_used WHERE order_plan_id = ?), 0) AS refund_service_total`,
		id, id).Scan(&agg)
	item["payment"] = agg.Payment
	item["refundServiceTotal"] = agg.RefundServiceTotal

	response.Data(c, item)
}
