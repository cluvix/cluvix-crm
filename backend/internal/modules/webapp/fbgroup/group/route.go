// Package group — danh sách / chi tiết / cập nhật group Facebook.
package group

import (
	"erpCluvix/internal/modules/webapp/fbgroup/group/handler"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes gắn /fbgroup/group/*. Route tĩnh (bulk) đăng ký TRƯỚC /:id.
func RegisterRoutes(g gin.IRouter, d handler.Deps) {
	h := handler.New(d)
	r := g.Group("/group")
	{
		r.GET("", h.ListGroup)
		r.POST("/bulk", h.BulkGroup)
		r.GET("/:id", h.GetGroup)
		r.PUT("/:id", h.UpdateGroup)
	}
}
