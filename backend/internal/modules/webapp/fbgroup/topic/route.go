// Package topic — chủ đề phân loại group Facebook.
package topic

import (
	"erpCluvix/internal/modules/webapp/fbgroup/topic/handler"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes gắn /fbgroup/topic/*. Route tĩnh (preview, reclassify) đăng ký TRƯỚC /:id.
func RegisterRoutes(g gin.IRouter, d handler.Deps) {
	h := handler.New(d)
	r := g.Group("/topic")
	{
		r.GET("", h.ListTopic)
		r.POST("", h.CreateTopic)
		r.POST("/preview", h.PreviewTopic)
		r.POST("/reclassify", h.ReclassifyTopic)
		r.GET("/:id", h.GetTopic)
		r.PUT("/:id", h.UpdateTopic)
		r.DELETE("/:id", h.DeleteTopic)
	}
}
