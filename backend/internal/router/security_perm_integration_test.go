//go:build integration

// Integration test — S-1/S-7/S-9/S-11 (lens bảo mật crm-prod-readiness 2026-10-18).
//
// Dựng engine giống nhóm `authed` của Setup (PermissionEnforcer + route thật của các module) nhưng
// thay JWTAuth/AuthResponseHeaders bằng middleware test đọc uid từ header — RbacService là bản THẬT
// trên DB dùng-một-lần (quyền seed qua rbac_auth_item/_child/_assignment). Mỗi ca chặn đều kèm đối
// chứng chiều cho qua để không xanh giả khi mọi thứ đều 403.
package router

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/db"
	"erpCluvix/internal/deps"
	"erpCluvix/internal/middleware"
	"erpCluvix/internal/modules/webapp/auth"
	"erpCluvix/internal/modules/webapp/auth/rbac/seed"
	authSvc "erpCluvix/internal/modules/webapp/auth/rbac/service"
	"erpCluvix/internal/modules/webapp/crm"
	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/internal/modules/webapp/history"
	"erpCluvix/internal/modules/webapp/hr"
	printModule "erpCluvix/internal/modules/webapp/print"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/logger"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	logger.Init(logger.Config{Env: "test", AppCode: "router_sec_test"})
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaAll)
	if err != nil {
		log.Printf("testhelpers: %v — skipping integration tests", err)
		os.Exit(m.Run())
	}
	testDB = s.DB
	db.DB = s.DB // pagination.Paginate + vài handler đọc global db.DB
	code := m.Run()
	s.Close()
	os.Exit(code)
}

const hdrUID = "X-Test-Uid"

var secSeq atomic.Int64

// secEngine — nhóm authed như Setup: auth test (uid từ header) → PermissionEnforcer(map embed) → module.
func secEngine(t *testing.T, branchID uint) *gin.Engine {
	t.Helper()
	entries, err := seed.LoadRoutePermMap()
	if err != nil {
		t.Fatalf("LoadRoutePermMap: %v", err)
	}
	rbac := &authSvc.RbacService{DB: testDB}
	r := testhelpers.NewGin()
	api := r.Group("/api")
	authed := api.Group("", func(c *gin.Context) {
		uid, _ := strconv.ParseUint(c.GetHeader(hdrUID), 10, 64)
		c.Set("branchActiveId", branchID)
		c.Set("branchIds", []uint{branchID})
		c.Set("userID", uint(uid))
		c.Next()
	}, middleware.PermissionEnforcer(rbac, entries))
	d := deps.Deps{DB: testDB, RbacSvc: rbac}
	auth.Register(api, authed, d)
	crm.Register(authed, d)
	hr.Register(authed, d)
	history.Register(authed, d)
	printModule.Register(authed, d)
	return r
}

func withEnforce(t *testing.T, on bool) {
	t.Helper()
	prev := middleware.PermEnforcing()
	middleware.SetPermEnforce(on)
	t.Cleanup(func() { middleware.SetPermEnforce(prev) })
}

// grant tạo 1 role riêng cho user, gắn các perm (perm item INSERT IGNORE — catalog dùng chung).
func grant(t *testing.T, uid uint, perms ...string) {
	t.Helper()
	now := time.Now().Unix()
	role := fmt.Sprintf("sec_role_%d_%d", uid, secSeq.Add(1))
	mustExec(t, "INSERT INTO rbac_auth_item (name, type, created_at, updated_at) VALUES (?, 1, ?, ?)", role, now, now)
	for _, p := range perms {
		mustExec(t, "INSERT IGNORE INTO rbac_auth_item (name, type, created_at, updated_at) VALUES (?, 2, ?, ?)", p, now, now)
		mustExec(t, "INSERT INTO rbac_auth_item_child (parent, child) VALUES (?, ?)", role, p)
	}
	mustExec(t, "INSERT INTO rbac_auth_assignment (item_name, user_id, created_at) VALUES (?, ?, ?)", role, uid, now)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM rbac_auth_assignment WHERE item_name = ?", role)
		testDB.Exec("DELETE FROM rbac_auth_item_child WHERE parent = ?", role)
		testDB.Exec("DELETE FROM rbac_auth_item WHERE name = ?", role)
	})
}

func makeSuper(t *testing.T, uid uint) {
	t.Helper()
	now := time.Now().Unix()
	mustExec(t, "INSERT IGNORE INTO rbac_auth_item (name, type, created_at, updated_at) VALUES (?, 1, ?, ?)", define.RoleAdmin, now, now)
	mustExec(t, "INSERT INTO rbac_auth_assignment (item_name, user_id, created_at) VALUES (?, ?, ?)", define.RoleAdmin, uid, now)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM rbac_auth_assignment WHERE item_name = ? AND user_id = ?", define.RoleAdmin, uid)
	})
}

func mustExec(t *testing.T, q string, args ...any) {
	t.Helper()
	if err := testDB.Exec(q, args...).Error; err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func call(r *gin.Engine, method, url string, uid uint) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, nil)
	req.Header.Set(hdrUID, strconv.FormatUint(uint64(uid), 10))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func expect(t *testing.T, w *httptest.ResponseRecorder, want int, what string) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("%s: HTTP %d, mong đợi %d — body=%.300s", what, w.Code, want, w.Body.String())
	}
}

func skipNoDB(t *testing.T) {
	t.Helper()
	if testDB == nil {
		t.Skip("TEST_MYSQL_DSN chưa set")
	}
}

// (a) S-1/S-9: route GET dữ liệu nghiệp vụ ⇒ 403 cho user thiếu quyền khi enforce bật; đối chứng:
// cùng request ở audit-mode KHÔNG bị chặn, user có đúng quyền view* và super đều qua.
func TestSecurity_GetDataRoutes_EnforcedWhenOn(t *testing.T) {
	skipNoDB(t)
	tn := testhelpers.FreshTenant(t, testDB)
	r := secEngine(t, tn.BranchID)
	nobody := testhelpers.CreateScopedUser(t, testDB, "sec_nobody", tn, tn.BranchID)
	viewer := testhelpers.CreateScopedUser(t, testDB, "sec_viewer", tn, tn.BranchID)
	grant(t, viewer, "viewCustomer")
	super := testhelpers.CreateScopedUser(t, testDB, "sec_super", tn, tn.BranchID)
	makeSuper(t, super)

	withEnforce(t, true)
	for _, url := range []string{
		"/api/crm/customer",
		fmt.Sprintf("/api/crm/customer/%d", tn.CustomerID),
		"/api/crm/appointment",
		"/api/crm/payment/1",
		"/api/hr/user",
		"/api/history/log",
		"/api/print/payment-in-day",
		"/api/crm/report/revenue",
		"/api/crm/appointment/appointment-schedule-byhour-grouped",
	} {
		expect(t, call(r, http.MethodGet, url, nobody), http.StatusForbidden, "enforce, thiếu quyền "+url)
	}
	expect(t, call(r, http.MethodGet, "/api/crm/customer", viewer), http.StatusOK, "enforce, có viewCustomer")
	expect(t, call(r, http.MethodGet, "/api/crm/appointment", viewer), http.StatusForbidden, "enforce, viewCustomer KHÔNG mở lịch hẹn")
	expect(t, call(r, http.MethodGet, "/api/crm/customer", super), http.StatusOK, "enforce, super")

	middleware.SetPermEnforce(false)
	expect(t, call(r, http.MethodGet, "/api/crm/customer", nobody), http.StatusOK, "audit-mode không chặn (đối chứng)")
}

// (b) S-7: export cần ext* (customer, appointment) / manageCrmReport (báo cáo) — chặn THẬT kể cả
// audit-mode; có quyền ⇒ 200; list thường (không export) không bị guard export ảnh hưởng.
func TestSecurity_ExportRequiresExtPerm(t *testing.T) {
	skipNoDB(t)
	t.Chdir(t.TempDir()) // ExportDir "./storage/export/" — không ghi file vào cây source
	exportHandlers.SetDB(testDB)
	t.Cleanup(func() { exportHandlers.SetDB(nil) })

	tn := testhelpers.FreshTenant(t, testDB)
	// Export rỗng trả 422 "không có dữ liệu" ⇒ seed 1 khách (đang nợ) + 1 lịch hẹn để ca "có quyền" ra 200.
	mustExec(t, "UPDATE customer SET branch_id = ?, totalRevenue = 100, totalDebt = 100 WHERE id = ?",
		tn.BranchID, tn.CustomerID)
	now := time.Now().Unix()
	appt := map[string]any{
		"code": fmt.Sprintf("LH%d", time.Now().UnixNano()), "customer_id": tn.CustomerID, "branch_id": tn.BranchID,
		"appointment_date": now, "status": 1, "content": "sec", "created_at": now, "updated_at": now,
	}
	if err := testDB.Table("appointment").Create(&appt).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}
	apptID := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() { testDB.Exec("DELETE FROM appointment WHERE id = ?", apptID) })

	r := secEngine(t, tn.BranchID)
	viewOnly := testhelpers.CreateScopedUser(t, testDB, "sec_view", tn, tn.BranchID)
	grant(t, viewOnly, "viewCustomer", "viewAppointment")
	exporter := testhelpers.CreateScopedUser(t, testDB, "sec_ext", tn, tn.BranchID)
	grant(t, exporter, "viewCustomer", "viewAppointment", define.PermExtCustomer, define.PermExtAppointment,
		define.PermManageCrmReport)

	cases := []struct{ name, list, export string }{
		{"customer", "/api/crm/customer", "/api/crm/customer?_export=1"},
		{"appointment", "/api/crm/appointment", "/api/crm/appointment?_export=1"},
	}
	for _, enforce := range []bool{true, false} {
		withEnforce(t, enforce)
		for _, tc := range cases {
			tag := fmt.Sprintf("[enforce=%v] %s", enforce, tc.name)
			expect(t, call(r, http.MethodGet, tc.list, viewOnly), http.StatusOK, tag+" list với view*")
			expect(t, call(r, http.MethodGet, tc.export, viewOnly), http.StatusForbidden, tag+" export thiếu ext*")
			expect(t, call(r, http.MethodGet, tc.export, exporter), http.StatusOK, tag+" export có ext*")
		}
	}

	// Báo cáo: audit-mode (route không chặn) để chứng minh guard export tự chặn độc lập PERM_ENFORCE.
	withEnforce(t, false)
	const rep = "/api/crm/report/debt/customer"
	expect(t, call(r, http.MethodGet, rep, viewOnly), http.StatusOK, "report list audit-mode")
	expect(t, call(r, http.MethodGet, rep+"?_export=1", viewOnly), http.StatusForbidden, "report export thiếu manageCrmReport")
	expect(t, call(r, http.MethodGet, rep+"?_export=1", exporter), http.StatusOK, "report export có manageCrmReport")
}

// (c) S-11: clear-cache chỉ super — user thường 403 kể cả audit-mode (guard trong handler); đối
// chứng super KHÔNG bị 403 (Cache nil trong test ⇒ handler trả 400 cache_not_configured).
func TestSecurity_ClearCacheSuperOnly(t *testing.T) {
	skipNoDB(t)
	tn := testhelpers.FreshTenant(t, testDB)
	r := secEngine(t, tn.BranchID)
	normal := testhelpers.CreateScopedUser(t, testDB, "sec_cc", tn, tn.BranchID)
	grant(t, normal, "viewCustomer")
	super := testhelpers.CreateScopedUser(t, testDB, "sec_cc_super", tn, tn.BranchID)
	makeSuper(t, super)

	for _, enforce := range []bool{true, false} {
		withEnforce(t, enforce)
		expect(t, call(r, http.MethodPost, "/api/auth/clear-cache", normal), http.StatusForbidden,
			fmt.Sprintf("[enforce=%v] clear-cache user thường", enforce))
		if w := call(r, http.MethodPost, "/api/auth/clear-cache", super); w.Code == http.StatusForbidden {
			t.Fatalf("[enforce=%v] super không được 403: %s", enforce, w.Body.String())
		}
	}
}
