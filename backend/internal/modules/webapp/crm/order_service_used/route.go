// Package order_service_used — module root (dòng dịch vụ đơn hàng; URL /crm/order-service-used).
//
// crm-standalone story-14: đổi tên từ module "dịch vụ điều trị" cũ — bỏ các endpoint liệu trình.
//
// Plan B.2 (2026-05-16): Handler-only migrate. Note 2 carve-out endpoint wire
// riêng ở router/crm.go.
package order_service_used

import (
	"erpCluvix/internal/middleware"
	"erpCluvix/internal/modules/webapp/crm/order_service_used/handler"
	"erpCluvix/pkg/define"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers /crm/order-service-used/* EXCEPT
// /list-service-used-customer + /report-service-used.
func RegisterRoutes(crm gin.IRouter, d handler.Deps) {
	h := handler.New(d)

	// Export dòng dịch vụ (kèm SĐT khách, doanh thu) — màn dùng là báo cáo "Dịch vụ sử dụng" (FE gác
	// manageCrmReport) ⇒ chặn thật theo manageCrmReport (S-7).
	exportGuard := middleware.RequireExportPerm(d.RbacSvc, define.PermManageCrmReport)
	ts := crm.Group("/order-service-used")
	{
		ts.GET("", exportGuard, h.ListTreatmentService)
		ts.GET("/service-used-with-appointment", exportGuard, h.ListServiceUsedWithAppointment)
		ts.GET("/service-useds", h.ServiceUseds)
		ts.GET("/service-used-of-customer", h.ServiceUsedOfCustomer)
		ts.GET("/:id", h.GetTreatmentService)
		ts.GET("/:id/get-service-used-of-customer", h.GetServiceUsedOfCustomer)
		ts.POST("/:id/lock-service-used", h.LockServiceUsed)
	}
}
