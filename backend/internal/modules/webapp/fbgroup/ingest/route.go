// Package ingest — NextCore đẩy group Facebook vào CRM + quản lý khoá API + nhập tệp CSV.
package ingest

import (
	"erpCluvix/internal/modules/webapp/fbgroup/ingest/handler"

	"github.com/gin-gonic/gin"
)

// RegisterPublic gắn POST /fbgroup/ingest vào nhóm /api KHÔNG JWT — xác thực bằng khoá API NextCore.
func RegisterPublic(api gin.IRouter, d handler.Deps) {
	h := handler.New(d)
	api.POST("/fbgroup/ingest", h.RequireIngestKey(), h.Ingest)
}

// RegisterRoutes gắn các route cần JWT vào nhóm /fbgroup (đã nằm trong authed).
func RegisterRoutes(g gin.IRouter, d handler.Deps) {
	h := handler.New(d)
	g.GET("/ingest-key", h.GetIngestKey)
	g.POST("/ingest-key", h.CreateIngestKey)
	g.POST("/group/import-csv", h.ImportCSV)
}
