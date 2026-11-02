// Package customer — module root.
//
// Plan B.2 (2026-05-16): FULL layered (handler + service). Note: TimelineCustomer
// ở customer_detail.go (vẫn trong crm package) → wire riêng.
package customer

import (
	"erpCluvix/internal/middleware"
	"erpCluvix/internal/modules/webapp/crm/customer/handler"
	"erpCluvix/pkg/define"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers /crm/customer/* EXCEPT /:id/timeline-customer.
func RegisterRoutes(crm gin.IRouter, d handler.Deps) {
	h := handler.New(d)

	customer := crm.Group("/customer")
	{
		// `?_export=1` = file Excel trọn danh sách khách ⇒ cần extCustomer (chặn thật, S-7).
		customer.GET("", middleware.RequireExportPerm(d.RbacSvc, define.PermExtCustomer), h.ListCustomer)
		customer.GET("/search-global", h.SearchGlobal)
		customer.GET("/load-by-keyword", h.LoadCustomerByKeyword)
		customer.GET("/get-number-customer-by-phone", h.GetNumberCustomerByPhone)
		customer.GET("/get-number-customer-by-old-code", h.GetNumberCustomerByOldCode)
		customer.GET("/get-customer-by-code", h.GetCustomerByCode)
		customer.GET("/:id", h.GetCustomer)
		customer.POST("", h.CreateCustomer)
		customer.PUT("/:id", h.UpdateCustomer)
		customer.DELETE("/:id", h.DeleteCustomer)
		customer.POST("/:id/add-tag", h.AddTag)
		customer.POST("/:id/remove-tag", h.RemoveTag)
		customer.POST("/:id/update-appointment-not-booked-reasons", h.UpdateAppointmentNotBookedReasons)
		customer.POST("/:id/create-avatar", h.CreateAvatar)
	}
}
