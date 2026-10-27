// Package stats — thống kê group Facebook.
package stats

import (
	"erpCluvix/internal/modules/webapp/fbgroup/stats/handler"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes gắn /fbgroup/stats/*.
func RegisterRoutes(g gin.IRouter, d handler.Deps) {
	h := handler.New(d)
	r := g.Group("/stats")
	{
		r.GET("/overview", h.Overview)
		r.GET("/branches", h.Branches)
		r.GET("/provinces", h.Provinces)
		r.GET("/keywords", h.Keywords)
	}
}
