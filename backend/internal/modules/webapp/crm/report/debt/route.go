// Package debt — nhóm báo cáo CÔNG NỢ của CRM (port từ ERP accountant/{customer,order_plan,treatment}_debts,
// 2026-10-17). Endpoint dưới /api/crm/report/debt/*. Báo cáo công nợ trả góp đã gỡ cùng tính năng trả góp.
//
// Khác ERP (cố ý):
//   - 1 thương hiệu/DB ⇒ bỏ mọi vế company_id; phạm vi chỉ còn branch qua tenant.ScopedDBNullBranch.
//   - Bỏ các đường tối ưu cho quy mô ERP (FORCE INDEX có cổng, chia luồng song song theo khoảng id, tra id
//     ngram/search_text): CRM SME không có `customer.search_text`/ApplyFoldedKeyword ⇒ từ khoá = LIKE thường.
//     Ngoại lệ có số đo (bench 982.900 dòng, 2026-10-19): "công nợ theo dịch vụ" dùng lại STRAIGHT_JOIN +
//     FORCE INDEX có cổng theo dải ngày — xem serviceDebtTable.
//   - "Công nợ theo dịch vụ" (ERP: công nợ điều trị) bỏ treatmentCourseLast + completion_percent
//     (CRM không còn treatment_course).
//
// Quy tắc tiền CRM (D20a — đừng đổi ở đây): hoàn tiền = huỷ bớt dịch vụ, công nợ = phải thu − đã thu ròng.
// Các báo cáo dưới tính nợ dịch vụ = phải thu − Σ thu (payment_for 4,5), phải thu = totalEnd + vat_amount
// (bmad/order-vat: totalEnd là tiền TRƯỚC thuế, phiếu thu là tiền CÓ thuế; dòng không VAT ⇒ phải thu = totalEnd
// như trước). Hai cách viết BẰNG NHAU vì phiếu hoàn (payment_for 1) lưu số ÂM đúng bằng −refundServiceUsed:
// (phải thu − refund) − (thu45 − refund) = phải thu − thu45.
package debt

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/internal/deps"
	"erpCluvix/internal/middleware"
	"erpCluvix/pkg/define"
)

// Handler — các báo cáo công nợ.
type Handler struct {
	DB *gorm.DB
}

// Register gắn /report/debt/* vào group /crm.
func Register(rg *gin.RouterGroup, d *deps.Deps) {
	h := &Handler{DB: d.DB}
	// Export (`?_export=1`) chặn thật theo manageCrmReport (S-7) — xem define.PermManageCrmReport.
	g := rg.Group("/report/debt", middleware.RequireExportPerm(d.RbacSvc, define.PermManageCrmReport))
	g.GET("/customer", h.CustomerDebts)
	g.GET("/order-plan", h.OrderPlanDebts)
	g.GET("/service", h.ServiceDebts)
}
