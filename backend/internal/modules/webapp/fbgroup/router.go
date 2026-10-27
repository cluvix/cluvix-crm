// Package fbgroup — module "Group Facebook": group do NextCore GROUPCRAWL crawl về, phân loại chủ đề,
// quản lý trạng thái làm việc và thống kê (thiết kế: bmad/fb-group/thiet-ke.md, duyệt 2026-10-27).
package fbgroup

import (
	"erpCluvix/internal/deps"
	"erpCluvix/internal/modules/webapp/fbgroup/group"
	groupHandler "erpCluvix/internal/modules/webapp/fbgroup/group/handler"
	"erpCluvix/internal/modules/webapp/fbgroup/ingest"
	ingestHandler "erpCluvix/internal/modules/webapp/fbgroup/ingest/handler"
	"erpCluvix/internal/modules/webapp/fbgroup/stats"
	statsHandler "erpCluvix/internal/modules/webapp/fbgroup/stats/handler"
	"erpCluvix/internal/modules/webapp/fbgroup/topic"
	topicHandler "erpCluvix/internal/modules/webapp/fbgroup/topic/handler"

	"github.com/gin-gonic/gin"
)

// Register gắn route của module:
//   - api (KHÔNG JWT): POST /api/fbgroup/ingest — NextCore đẩy group, xác thực bằng khoá API riêng.
//   - authed (JWT + PermissionEnforcer): còn lại, dưới /api/fbgroup.
func Register(api, authed *gin.RouterGroup, d deps.Deps) {
	db := d.DB
	ingest.RegisterPublic(api, ingestHandler.Deps{DB: db})

	g := authed.Group("/fbgroup")
	group.RegisterRoutes(g, groupHandler.Deps{DB: db})
	topic.RegisterRoutes(g, topicHandler.Deps{DB: db})
	stats.RegisterRoutes(g, statsHandler.Deps{DB: db})
	ingest.RegisterRoutes(g, ingestHandler.Deps{DB: db})
}
