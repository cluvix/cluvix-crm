package dashboard

import (
	"erpCluvix/internal/modules/webapp/crm/dashboard/handler"

	"github.com/gin-gonic/gin"
)

// New constructs the dashboard Handler. Exposed so callers can wire individual
// methods into other groups (e.g. router/crm.go wires CrmCompareAppointment
// under /report).
func New(d handler.Deps) *handler.Handler { return handler.New(d) }

func RegisterRoutes(crm gin.IRouter, h *handler.Handler) {
	dashGrp := crm.Group("/crm-dashboard")
	dashGrp.GET("/payment-in-month", h.CrmPaymentInMonth)
	dashGrp.GET("/quick-starts", h.CrmQuickStarts)
	dashGrp.GET("/additional-stats", h.CrmAdditionalStats)
	dashGrp.GET("/compare-appointment", h.CrmCompareAppointment)
	dashGrp.GET("/appointment-accepted-stats", h.CrmAppointmentAcceptedStats)
	dashGrp.GET("/overview-appointment", h.CrmOverviewAppointment)
	// D14/story-15 crm-standalone patch P2 (2026-09-24): bỏ /estimate-price-marketing +
	// /marketing-view — cả 2 phụ thuộc HOÀN TOÀN bảng digital_facebook (marketing, xoá story-07),
	// không phải 1 nhánh nhỏ trong hàm đa năng.
	// 2026-10-19 (dashboard tuỳ biến, user chốt): bỏ tiếp /customer-status-marketing + /overview-online —
	// 2 khối marketing không có trong registry khối CRM.
	dashGrp.GET("/report-statistics", h.CrmReportStatistics)
	dashGrp.GET("/status-customer-overview", h.CrmStatusCustomerOverview)
	dashGrp.GET("/report-customer-by-region", h.CrmReportCustomerByRegion)
	dashGrp.GET("/payment-branch-overview", h.CrmPaymentBranchOverview)
	dashGrp.GET("/overview-service-type-accepted", h.CrmOverviewServiceTypeAccepted)
	dashGrp.GET("/overview-chart-source", h.CrmOverviewChartSource)
	dashGrp.GET("/overview-service-type-interest", h.CrmOverviewServiceTypeInterest)
	dashGrp.GET("/service-used-statistics", h.CrmServiceUsedStatistics)
}
