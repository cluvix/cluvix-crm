//go:build integration

// Integration test bố cục dashboard CRM (port ERP epic crm-dashboard-customizable story-04) — DB dùng-một-lần.
// CRM: 1 DB = 1 thương hiệu ⇒ bản "mặc định thương hiệu" (user_id NULL) là duy nhất trong DB; mỗi test dọn
// sạch bảng trước/sau.
package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	authSvc "erpCluvix/internal/modules/webapp/auth/rbac/service"
	layout "erpCluvix/internal/modules/webapp/crm/dashboard_layout"
	"erpCluvix/internal/modules/webapp/crm/dashboard_layout/handler"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

type fx struct{ admin, a, b uint }

var roleSeq int

// grantPerm gán 1 quyền thật cho user qua role riêng (rbac_auth_*), để HasAnyPermStrict đọc ra đúng.
func grantPerm(t *testing.T, userID uint, perm string) {
	t.Helper()
	now := time.Now().Unix()
	roleSeq++
	role := fmt.Sprintf("test_dl_role_%d_%d", now, roleSeq)
	require.NoError(t, testDB.Exec("INSERT IGNORE INTO rbac_auth_item (name, type, created_at, updated_at) VALUES (?, 2, ?, ?)", perm, now, now).Error)
	require.NoError(t, testDB.Exec("INSERT INTO rbac_auth_item (name, type, created_at, updated_at) VALUES (?, 1, ?, ?)", role, now, now).Error)
	require.NoError(t, testDB.Exec("INSERT INTO rbac_auth_item_child (parent, child) VALUES (?, ?)", role, perm).Error)
	require.NoError(t, testDB.Exec("INSERT INTO rbac_auth_assignment (item_name, user_id, created_at) VALUES (?, ?, ?)", role, userID, now).Error)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM rbac_auth_assignment WHERE item_name = ?", role)
		testDB.Exec("DELETE FROM rbac_auth_item_child WHERE parent = ?", role)
		testDB.Exec("DELETE FROM rbac_auth_item WHERE name = ?", role)
	})
}

func setup(t *testing.T) *fx {
	t.Helper()
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	testDB.Exec("DELETE FROM dashboard_layout")
	f := &fx{
		admin: testhelpers.CreateUser(t, testDB, "dla", nil),
		a:     testhelpers.CreateUser(t, testDB, "dlu", nil),
		b:     testhelpers.CreateUser(t, testDB, "dlv", nil),
	}
	grantPerm(t, f.admin, define.PermManageConfig)
	// a, b có quyền xem dashboard nhưng KHÔNG có manageConfig.
	grantPerm(t, f.a, "viewCustomer")
	grantPerm(t, f.b, "viewCustomer")
	t.Cleanup(func() { testDB.Exec("DELETE FROM dashboard_layout") })
	return f
}

func router(uid uint) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", uid)
		c.Next()
	})
	layout.RegisterRoutes(r.Group("/api/crm"), handler.Deps{DB: testDB, RbacSvc: &authSvc.RbacService{DB: testDB}})
	return r
}

type view struct {
	Config          json.RawMessage `json:"config"`
	Source          string          `json:"source"`
	HasBrandDefault bool            `json:"hasBrandDefault"`
	UpdatedAt       int64           `json:"updatedAt"`
}

func call(t *testing.T, r *gin.Engine, method, path, body string) (int, view, string) {
	t.Helper()
	req := httptest.NewRequest(method, "/api/crm/crm-dashboard/layout"+path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out struct {
		Data view `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out.Data, w.Body.String()
}

func cfg(keys ...string) string {
	items := make([]string, 0, len(keys))
	for _, k := range keys {
		items = append(items, fmt.Sprintf(`{"key":%q,"zone":"main","visible":true,"size":"full"}`, k))
	}
	return `{"version":1,"items":[` + strings.Join(items, ",") + `]}`
}

func count(t *testing.T, where string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testDB.Raw("SELECT COUNT(*) FROM dashboard_layout WHERE "+where, args...).Scan(&n).Error)
	return n
}

func TestLayout_UserSaveGetResetIsolation(t *testing.T) {
	f := setup(t)
	ra, rb := router(f.a), router(f.b)

	code, v, body := call(t, ra, http.MethodGet, "", "")
	require.Equal(t, http.StatusOK, code, body)
	require.Equal(t, define.CrmDashboardLayoutSourceSystem, v.Source)
	require.Equal(t, "null", string(v.Config))
	require.False(t, v.HasBrandDefault)

	code, v, body = call(t, ra, http.MethodPut, "", `{"config":`+cfg("quick_stats", "additional_stats")+`}`)
	require.Equal(t, http.StatusOK, code, body)
	require.Equal(t, define.CrmDashboardLayoutSourceUser, v.Source)
	require.Contains(t, string(v.Config), "additional_stats")
	require.NotZero(t, v.UpdatedAt)

	// Lưu lần 2 ⇒ cập nhật đúng dòng cũ, không sinh dòng mới.
	code, _, body = call(t, ra, http.MethodPut, "", `{"config":`+cfg("customer_region")+`}`)
	require.Equal(t, http.StatusOK, code, body)
	require.EqualValues(t, 1, count(t, "1=1"))

	// User B KHÔNG thấy bố cục của A.
	_, v, _ = call(t, rb, http.MethodGet, "", "")
	require.Equal(t, define.CrmDashboardLayoutSourceSystem, v.Source)

	// Khôi phục mặc định ⇒ về system; xoá lần 2 vẫn 200.
	code, v, _ = call(t, ra, http.MethodDelete, "", "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, define.CrmDashboardLayoutSourceSystem, v.Source)
	code, _, _ = call(t, ra, http.MethodDelete, "", "")
	require.Equal(t, http.StatusOK, code)
	require.EqualValues(t, 0, count(t, "1=1"))
}

// Thứ tự ưu tiên user > brand > system; chỉ manageConfig lưu/xoá mặc định thương hiệu.
func TestLayout_BrandDefault(t *testing.T) {
	f := setup(t)
	ra, radmin := router(f.a), router(f.admin)

	// Không manageConfig ⇒ 403 cả PUT lẫn DELETE, không ghi gì.
	code, _, _ := call(t, ra, http.MethodPut, "/brand-default", `{"config":`+cfg("quick_stats")+`}`)
	require.Equal(t, http.StatusForbidden, code)
	code, _, _ = call(t, ra, http.MethodDelete, "/brand-default", "")
	require.Equal(t, http.StatusForbidden, code)
	require.EqualValues(t, 0, count(t, "1=1"))

	code, v, body := call(t, radmin, http.MethodPut, "/brand-default", `{"config":`+cfg("customer_region")+`}`)
	require.Equal(t, http.StatusOK, code, body)
	require.True(t, v.HasBrandDefault)
	require.Equal(t, define.CrmDashboardLayoutSourceBrand, v.Source) // admin chưa có bản riêng

	// User A: chưa có bản riêng ⇒ brand; có bản riêng ⇒ user; khôi phục ⇒ về brand.
	_, v, _ = call(t, ra, http.MethodGet, "", "")
	require.Equal(t, define.CrmDashboardLayoutSourceBrand, v.Source)
	require.True(t, v.HasBrandDefault)
	require.Contains(t, string(v.Config), "customer_region")
	call(t, ra, http.MethodPut, "", `{"config":`+cfg("quick_stats")+`}`)
	_, v, _ = call(t, ra, http.MethodGet, "", "")
	require.Equal(t, define.CrmDashboardLayoutSourceUser, v.Source)
	require.True(t, v.HasBrandDefault)
	_, v, _ = call(t, ra, http.MethodDelete, "", "")
	require.Equal(t, define.CrmDashboardLayoutSourceBrand, v.Source)

	// Lưu mặc định 2 lần ⇒ vẫn 1 dòng (UNIQUE với user_id NULL qua COALESCE).
	call(t, radmin, http.MethodPut, "/brand-default", `{"config":`+cfg("additional_stats")+`}`)
	require.EqualValues(t, 1, count(t, "user_id IS NULL"))

	// Blame: bản mặc định do admin tạo.
	var cb, ub *uint
	require.NoError(t, testDB.Raw("SELECT created_by, updated_by FROM dashboard_layout WHERE user_id IS NULL").Row().Scan(&cb, &ub))
	require.NotNil(t, cb)
	require.Equal(t, f.admin, *cb)
	require.Equal(t, f.admin, *ub)

	// Xoá mặc định thương hiệu KHÔNG xoá bản của user; user không có bản riêng ⇒ về system.
	call(t, ra, http.MethodPut, "", `{"config":`+cfg("quick_stats")+`}`)
	code, _, _ = call(t, radmin, http.MethodDelete, "/brand-default", "")
	require.Equal(t, http.StatusOK, code)
	require.EqualValues(t, 1, count(t, "user_id = ?", f.a))
	_, v, _ = call(t, router(f.b), http.MethodGet, "", "")
	require.Equal(t, define.CrmDashboardLayoutSourceSystem, v.Source)
	require.False(t, v.HasBrandDefault)
}

func TestLayout_Validation(t *testing.T) {
	f := setup(t)
	ra := router(f.a)
	tooMany := make([]string, 0, 51)
	for i := 0; i < 51; i++ {
		tooMany = append(tooMany, `{"key":"quick_stats","zone":"main","visible":true,"size":"full"}`)
	}
	pad := strings.Repeat(" ", 17<<10)
	bad := map[string]string{
		"key lạ":                  `{"config":` + cfg("hacked_widget") + `}`,
		"khối marketing đã bỏ":    `{"config":` + cfg("customer_status_marketing") + `}`,
		"khối ERP không có ở CRM": `{"config":` + cfg("clv_overview") + `}`,
		"trùng key":               `{"config":` + cfg("quick_stats", "quick_stats") + `}`,
		"vùng sai":                `{"config":{"version":1,"items":[{"key":"quick_stats","zone":"left","visible":true,"size":"full"}]}}`,
		"cỡ sai":                  `{"config":{"version":1,"items":[{"key":"quick_stats","zone":"main","visible":true,"size":"third"}]}}`,
		"version sai":             `{"config":{"version":2,"items":[]}}`,
		"thiếu items":             `{"config":{"version":1}}`,
		"field lạ":                `{"config":{"version":1,"items":[],"evil":1}}`,
		"config null":             `{"config":null}`,
		"không phải JSON":         `not-json`,
		"quá 50 mục":              `{"config":{"version":1,"items":[` + strings.Join(tooMany, ",") + `]}}`,
		"quá 16KB":                `{"config":{"version":1,` + pad + `"items":[]}}`,
	}
	for name, body := range bad {
		code, _, resp := call(t, ra, http.MethodPut, "", body)
		require.Equal(t, http.StatusUnprocessableEntity, code, "%s: %s", name, resp)
		require.NotContains(t, resp, "json:", "%s: không được lộ lỗi decode thô", name)
	}
	require.EqualValues(t, 0, count(t, "1=1"))

	// Đủ 14 khối hợp lệ + danh sách rỗng (ẩn hết) đều hợp lệ.
	keys := make([]string, 0, len(define.CrmDashboardWidgetKeys))
	for k := range define.CrmDashboardWidgetKeys {
		keys = append(keys, k)
	}
	require.Len(t, keys, 14)
	code, _, body := call(t, ra, http.MethodPut, "", `{"config":`+cfg(keys...)+`}`)
	require.Equal(t, http.StatusOK, code, body)
	code, _, body = call(t, ra, http.MethodPut, "", `{"config":{"version":1,"items":[]}}`)
	require.Equal(t, http.StatusOK, code, body)
}

// Body không chỉ định được chủ bố cục: user_id/id trong body bị bỏ qua.
func TestLayout_BodyCannotTargetOthers(t *testing.T) {
	f := setup(t)
	body := fmt.Sprintf(`{"user_id":%d,"id":1,"config":%s}`, f.b, cfg("quick_stats"))
	code, _, resp := call(t, router(f.a), http.MethodPut, "", body)
	require.Equal(t, http.StatusOK, code, resp)
	require.EqualValues(t, 1, count(t, "user_id = ?", f.a))
	require.EqualValues(t, 0, count(t, "user_id = ? OR user_id IS NULL", f.b))
}

// Không có phiên (uid = 0) ⇒ 403 ở mọi method, không ghi.
func TestLayout_NoSession(t *testing.T) {
	setup(t)
	r := router(0)
	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		code, _, _ := call(t, r, m, "", `{"config":`+cfg("quick_stats")+`}`)
		require.Equal(t, http.StatusForbidden, code, m)
	}
	require.EqualValues(t, 0, count(t, "1=1"))
}

// Xoá user ⇒ FK ON DELETE CASCADE dọn bố cục của user đó; mặc định thương hiệu còn nguyên.
func TestLayout_DeleteUserCascades(t *testing.T) {
	f := setup(t)
	call(t, router(f.admin), http.MethodPut, "/brand-default", `{"config":`+cfg("quick_stats")+`}`)
	call(t, router(f.b), http.MethodPut, "", `{"config":`+cfg("quick_stats")+`}`)
	require.EqualValues(t, 1, count(t, "user_id = ?", f.b))

	// testhelpers.New tắt FK checks theo phiên — bật lại để chứng minh CHÍNH FK thật làm việc CASCADE.
	// Gỡ role của b trước (rbac_auth_assignment có FK RESTRICT tới user — không phải đối tượng test này).
	require.NoError(t, testDB.Exec("DELETE FROM rbac_auth_assignment WHERE user_id = ?", f.b).Error)
	require.NoError(t, testDB.Exec("SET FOREIGN_KEY_CHECKS = 1").Error)
	t.Cleanup(func() { testDB.Exec("SET FOREIGN_KEY_CHECKS = 0") })
	require.NoError(t, testDB.Exec("DELETE FROM `user` WHERE id = ?", f.b).Error)
	require.EqualValues(t, 0, count(t, "user_id = ?", f.b))
	require.EqualValues(t, 1, count(t, "user_id IS NULL"))

	var fk int64
	require.NoError(t, testDB.Raw(`SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS
		WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'dashboard_layout' AND REFERENCED_TABLE_NAME = 'user'
		AND DELETE_RULE = 'CASCADE'`).Scan(&fk).Error)
	require.EqualValues(t, 1, fk, "phải đúng 1 FK user_id → user ON DELETE CASCADE (không FK cho created_by/updated_by)")
}

// Migration idempotent: chạy lại file .up.sql trên DB đã có bảng không lỗi.
func TestLayout_MigrationRerun(t *testing.T) {
	setup(t)
	matches, err := filepath.Glob(filepath.Join(testhelpers.MigrationsDir(), "*_dashboard_layout.up.sql"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	sqlText, err := os.ReadFile(matches[0])
	require.NoError(t, err)
	for i := 0; i < 2; i++ {
		require.NoError(t, testDB.Exec(string(sqlText)).Error, "lần %d", i+1)
	}
}

// 2 lượt PUT đồng thời (cùng user / mặc định thương hiệu) không sinh dòng trùng, không 5xx.
func TestLayout_ConcurrentUpsert(t *testing.T) {
	f := setup(t)
	ra, radmin := router(f.a), router(f.admin)
	for round := 0; round < 3; round++ {
		testDB.Exec("DELETE FROM dashboard_layout")
		var wg sync.WaitGroup
		codes := make(chan int, 20)
		for i := 0; i < 10; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				c, _, _ := call(t, ra, http.MethodPut, "", `{"config":`+cfg("quick_stats")+`}`)
				codes <- c
			}()
			go func() {
				defer wg.Done()
				c, _, _ := call(t, radmin, http.MethodPut, "/brand-default", `{"config":`+cfg("additional_stats")+`}`)
				codes <- c
			}()
		}
		wg.Wait()
		close(codes)
		for c := range codes {
			require.Equal(t, http.StatusOK, c)
		}
		require.EqualValues(t, 1, count(t, "user_id = ?", f.a), "round %d", round)
		require.EqualValues(t, 1, count(t, "user_id IS NULL"), "round %d", round)
	}
}
