// Package customer_stage — danh mục bước tiến trình khách hàng (epic customer-stage-pipeline).
package customer_stage

import (
	"erpCluvix/internal/modules/webapp/config/customer_stage/handler"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts /config/customer-stage endpoints.
// `reorder` khai TRƯỚC `/:id` cho rõ ràng (gin tách được static/param nhưng giữ thứ tự dễ đọc).
func RegisterRoutes(cfg gin.IRouter, d handler.Deps) {
	h := handler.New(d)
	cfg.GET("/customer-stage", h.ListCustomerStage)
	cfg.GET("/customer-stage/:id", h.GetCustomerStage)
	cfg.POST("/customer-stage", h.CreateCustomerStage)
	cfg.PUT("/customer-stage/reorder", h.ReorderCustomerStage)
	cfg.PUT("/customer-stage/:id", h.UpdateCustomerStage)
	cfg.PUT("/customer-stage/:id/set-default", h.SetDefaultCustomerStage)
	cfg.DELETE("/customer-stage/:id", h.DeleteCustomerStage)
}
