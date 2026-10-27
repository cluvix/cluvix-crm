package router

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"erpCluvix/config"
	"erpCluvix/internal/deps"
	"erpCluvix/internal/middleware"
	"erpCluvix/internal/modules/webapp/auth"
	"erpCluvix/internal/modules/webapp/auth/rbac/seed"
	configModule "erpCluvix/internal/modules/webapp/config"
	"erpCluvix/internal/modules/webapp/crm"
	exportModule "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/internal/modules/webapp/fbgroup"
	"erpCluvix/internal/modules/webapp/history"
	"erpCluvix/internal/modules/webapp/hr"
	importmodule "erpCluvix/internal/modules/webapp/import"
	printModule "erpCluvix/internal/modules/webapp/print"
	"erpCluvix/pkg/logger"
	"erpCluvix/pkg/storage/signedfile"

	"github.com/gin-gonic/gin"
)

func Setup(cfg *config.Config, d deps.Deps) *gin.Engine {
	db := d.DB
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	// Gin default tin MỌI proxy → ClientIP() giả mạo được qua X-Forwarded-For.
	// Chỉ tin proxy trong TRUSTED_PROXIES; rỗng = không tin ai (IP = remote addr).
	// Nền tảng cho rate-limit login, IP binding, security log.
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		logger.L.Error(fmt.Sprintf("SetTrustedProxies failed: %v — fall back to no trusted proxies", err))
		_ = r.SetTrustedProxies(nil)
	}
	r.MaxMultipartMemory = middleware.MaxMultipartBodyBytes
	r.Use(gin.Logger())
	r.Use(gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, err any) {
		logger.L.Error(fmt.Sprintf("PANIC: %v | %s %s\n%s", err, c.Request.Method, c.Request.URL.String(), debug.Stack()))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "Internal Server Error"})
	}))
	r.Use(middleware.RenderErrorGuard())
	r.Use(middleware.RequestID())
	r.Use(middleware.RequestSizeLimit())
	r.Use(middleware.CORS(cfg.IsProduction(), cfg.CORSAllowedOrigins))
	r.Use(middleware.SecurityHeaders())
	// RequestPath set "METHOD /url" vào goroutine-local store để gorm.Trace
	// bake path vào SQL Error log. Defer clear ngay sau khi handler return.
	r.Use(middleware.RequestPath())
	r.Use(middleware.ResponseHeaders(db, d.Cache))

	// /health — xem healthHandler (health.go): phơi trạng thái decimalColumnMap (story-02).
	r.GET("/health", healthHandler)
	// /health/ready — readiness: ping DB + Redis, 503 khi hỏng (O-11). Ngoài /api ⇒ không cần perm map.
	r.GET("/health/ready", readyHandler(newReadyChecks(d)))

	// Static files
	// Kho PUBLIC driver local ⇒ phục vụ static tại STORAGE_PUBLIC_PREFIX. Driver s3/r2 ⇒ file đi
	// thẳng từ CDN/bucket, KHÔNG mount gì. Kho PRIVATE (đính kèm khách hàng) KHÔNG BAO GIỜ được mount —
	// chỉ tải qua GET /api/crm/attachment/:id/download (JWT + quyền + scope).
	mountPublicUploads(r, d.Store)
	// /download/export KHÔNG còn static công khai (S-2/O-6, 2026-10-18): file export chứa PII khách,
	// tải qua GET /api/download/export/:name (nhóm authed, xem exportModule.RegisterDownload).
	// /download/templates giữ công khai: chỉ chứa file mẫu import tĩnh, không có dữ liệu khách.
	r.Static("/download/templates", "./storage/templates")

	api := r.Group("/api")

	// URL ký ngắn hạn kho private driver local — CÔNG KHAI (ngoài authed), xác thực bằng chữ ký HMAC.
	signedfile.Mount(api, d.PrivateStore)

	// Authenticated group
	authed := api.Group("", middleware.JWTAuth(d.TokenSvc), middleware.AuthResponseHeaders(db, d.Cache))
	// PermissionEnforcer (declarative, audit-mode): 1 middleware toàn cục tra route→perm map
	// đã embed, enforce theo P1.2/P1.3. Đặt SAU JWTAuth + AuthResponseHeaders (cần uid + scope).
	// Lỗi parse map = fail-fast lúc boot (artifact hỏng), KHÔNG chạy tiếp không có enforcement
	// (O-9): prod hoặc PERM_ENFORCE=true ⇒ Fatal (dừng tiến trình); chỉ dev audit-mode mới chạy tiếp.
	routePermMap, err := seed.LoadRoutePermMap()
	if err != nil {
		if cfg.IsProduction() || cfg.PermEnforce {
			// panic (không chỉ logger.L.Fatal): Fatal của logger no-op khi zap chưa Init ⇒ sẽ chạy tiếp.
			msg := fmt.Sprintf("LoadRoutePermMap failed: %v — từ chối khởi động thiếu PermissionEnforcer", err)
			logger.L.Error(msg)
			panic(msg)
		}
		logger.L.Error(fmt.Sprintf("LoadRoutePermMap failed: %v", err))
	} else {
		if cfg.IsProduction() && !cfg.PermEnforce {
			logger.L.Error("PERM_ENFORCE=false trên production — RBAC chỉ audit-mode, KHÔNG chặn request thiếu quyền")
		}
		authed.Use(middleware.PermissionEnforcer(d.RbacSvc, routePermMap))
	}

	// Module routes
	auth.Register(api, authed, d)
	crm.Register(authed, d)
	configModule.Register(authed, d)
	hr.Register(authed, d)

	history.Register(authed, d)
	importmodule.Register(authed, d)
	printModule.Register(authed, d)
	// Group Facebook — POST /api/fbgroup/ingest nằm ở `api` (NextCore gọi bằng khoá API, không JWT).
	fbgroup.Register(api, authed, d)
	// Tải file export — CÓ JWT + chỉ file của chính người tạo (S-2/O-6). Thay static công khai cũ.
	exportModule.RegisterDownload(authed)

	return r
}
