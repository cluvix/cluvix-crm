//go:build integration

package handler

// Story-03 (customer-stage-pipeline) — đường GHI bước khách ở tầng handler:
//   - Tạo khách: nhận bước ĐANG mặc định (đọc lại mỗi lần) + 1 dòng lịch sử khởi tạo; không có mặc
//     định ⇒ 422, không tạo khách (AC1-AC3; AC1 nằm trong TestCreateCustomer_NewCustomerAlwaysContact).
//   - PUT /crm/customer/:id/stage qua router thật: shape response, 404/422 không ghi gì (AC5-AC7).
//   - PUT /crm/customer/:id kèm stage_id ⇒ bị bỏ qua (AC10).
// Logic giao dịch (rollback, đồng thời) khoá ở crm/customer/service (change_stage_integration_test.go).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	stagesvc "erpCluvix/internal/modules/webapp/config/customer_stage/service"
	customerService "erpCluvix/internal/modules/webapp/crm/customer/service"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/i18n"
)

// stageIDByName — id bước seed của migration 20261102095255 (khoá seed `name`).
func stageIDByName(t *testing.T, name string) int {
	t.Helper()
	var id int
	require.NoError(t, testDB.Raw("SELECT id FROM customer_stage WHERE name = ?", name).Scan(&id).Error)
	require.NotZero(t, id, "thiếu bước seed %q", name)
	return id
}

func defaultStageID(t *testing.T) int {
	t.Helper()
	var id int
	require.NoError(t, testDB.Raw("SELECT id FROM customer_stage WHERE is_default = 1").Scan(&id).Error)
	require.NotZero(t, id, "DB test phải có bước mặc định (seed)")
	return id
}

// swapDefaultStage đặt bước `to` làm mặc định (to = 0 ⇒ không còn bước mặc định) và khôi phục bước
// mặc định cũ khi test xong. 2 lệnh riêng vì UNIQUE default_key kiểm từng dòng.
func swapDefaultStage(t *testing.T, to int) {
	t.Helper()
	old := defaultStageID(t)
	require.NoError(t, testDB.Exec("UPDATE customer_stage SET is_default = 0 WHERE is_default = 1").Error)
	if to != 0 {
		require.NoError(t, testDB.Exec("UPDATE customer_stage SET is_default = 1 WHERE id = ?", to).Error)
	}
	t.Cleanup(func() {
		testDB.Exec("UPDATE customer_stage SET is_default = 0 WHERE is_default = 1")
		testDB.Exec("UPDATE customer_stage SET is_default = 1 WHERE id = ?", old)
	})
}

type stageHistoryRow struct {
	ID          int
	FromStageID *int `gorm:"column:from_stage_id"`
	ToStageID   int  `gorm:"column:to_stage_id"`
	Source      int
	ChangedBy   *int  `gorm:"column:changed_by"`
	ChangedAt   int64 `gorm:"column:changed_at"`
}

func stageHistory(t *testing.T, customerID uint) []stageHistoryRow {
	t.Helper()
	rows := make([]stageHistoryRow, 0)
	require.NoError(t, testDB.Table("customer_stage_history").
		Select("id, from_stage_id, to_stage_id, source, changed_by, changed_at").
		Where("customer_id = ?", customerID).Order("id ASC").Find(&rows).Error)
	return rows
}

func customerStageID(t *testing.T, customerID uint) *int {
	t.Helper()
	var row struct {
		StageID *int `gorm:"column:stage_id"`
	}
	require.NoError(t, testDB.Table("customer").Select("stage_id").Where("id = ?", customerID).Take(&row).Error)
	return row.StageID
}

// assertInitialStage — khách vừa tạo/import: stage_id = wantStage + ĐÚNG 1 dòng lịch sử
// (from NULL, to = wantStage, source, changed_by = uid, changed_at ≈ now).
func assertInitialStage(t *testing.T, customerID uint, wantStage int, uid uint, source int) {
	t.Helper()
	got := customerStageID(t, customerID)
	require.NotNil(t, got, "khách %d chưa có stage_id", customerID)
	require.Equal(t, wantStage, *got, "khách %d phải nhận bước mặc định", customerID)
	hist := stageHistory(t, customerID)
	require.Len(t, hist, 1, "khách %d phải có đúng 1 dòng lịch sử khởi tạo", customerID)
	h := hist[0]
	require.Nil(t, h.FromStageID, "lịch sử khởi tạo: from_stage_id NULL")
	require.Equal(t, wantStage, h.ToStageID)
	require.Equal(t, source, h.Source)
	require.NotNil(t, h.ChangedBy)
	require.EqualValues(t, uid, *h.ChangedBy)
	require.InDelta(t, time.Now().Unix(), h.ChangedAt, 10, "changed_at ≈ now")
}

// AC2 — đổi bước mặc định rồi tạo khách ⇒ khách nhận bước mặc định MỚI (không cache cứng).
func TestCreateCustomer_UsesCurrentDefaultStage(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	t.Cleanup(func() { cleanupS33Customers(tn) })
	uid := testhelpers.CreateScopedUser(t, testDB, "s03", tn, tn.BranchID)
	consulting := stageIDByName(t, "Đang tư vấn")
	require.NotEqual(t, consulting, defaultStageID(t), "tiền đề: 'Đang tư vấn' chưa là mặc định")

	// Tạo 1 khách TRƯỚC khi đổi ⇒ nạp bước mặc định cũ (nếu có cache thì lần sau sẽ trả bước cũ).
	first := fmt.Sprintf("S33 S03 truoc %d", time.Now().UnixNano())
	require.Equal(t, 201, callCreateCustomer(t, uid, map[string]any{"fullname": first, "branch_id": tn.BranchID}).Code)
	oldDefault := defaultStageID(t)

	swapDefaultStage(t, consulting)
	name := fmt.Sprintf("S33 S03 sau %d", time.Now().UnixNano())
	w := callCreateCustomer(t, uid, map[string]any{"fullname": name, "branch_id": tn.BranchID})
	require.Equal(t, 201, w.Code, "body: %s", w.Body.String())

	var ids []uint
	require.NoError(t, testDB.Table("customer").Where("fullname IN ?", []string{first, name}).
		Order("id ASC").Pluck("id", &ids).Error)
	require.Len(t, ids, 2)
	assertInitialStage(t, ids[0], oldDefault, uid, define.CustomerStageHistorySourceCreate)
	assertInitialStage(t, ids[1], consulting, uid, define.CustomerStageHistorySourceCreate)
}

// AC3 — không có bước mặc định ⇒ 422 customer_stage_default_missing, KHÔNG có dòng customer nào.
func TestCreateCustomer_NoDefaultStage_422(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	t.Cleanup(func() { cleanupS33Customers(tn) })
	uid := testhelpers.CreateScopedUser(t, testDB, "s03", tn, tn.BranchID)
	swapDefaultStage(t, 0)

	name := fmt.Sprintf("S33 S03 nodefault %d", time.Now().UnixNano())
	w := callCreateCustomer(t, uid, map[string]any{"fullname": name, "branch_id": tn.BranchID, "note": "ghi chu"})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, "body: %s", w.Body.String())
	require.Contains(t, w.Body.String(), i18n.TL(i18n.Vi, stagesvc.KeyDefaultMissing))

	var n int64
	require.NoError(t, testDB.Table("customer").Where("fullname = ?", name).Count(&n).Error)
	require.Zero(t, n, "không được tạo dòng customer khi thiếu bước mặc định")
}

// --- PUT /crm/customer/:id/stage ------------------------------------------------------------------

func stageRouter(branchID, userID uint) *gin.Engine {
	r := testhelpers.NewGin()
	r.Use(testhelpers.AuthMiddleware(branchID, userID))
	h := &Handler{DB: testDB, CustomerSvc: customerService.NewCustomerService(testDB)}
	r.PUT("/crm/customer/:id/stage", h.ChangeStage)
	r.PUT("/crm/customer/:id", h.UpdateCustomer)
	return r
}

func putJSON(r *gin.Engine, path string, rawBody string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(rawBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

// seedStageCustomer — khách ở chi nhánh branchID (nil = chưa gán), đang ở bước stageID, updated_at cũ
// (để thấy rõ bump), payment_user_id tuỳ chọn.
func seedStageCustomer(t *testing.T, branchID *uint, stageID int, paymentUserID *uint) uint {
	t.Helper()
	row := map[string]any{
		"fullname":        fmt.Sprintf("S03 Stage %d", time.Now().UnixNano()),
		"stage_id":        stageID,
		"payment_user_id": paymentUserID,
		"created_at":      1000,
		"updated_at":      1000,
	}
	if branchID != nil {
		row["branch_id"] = *branchID
	}
	require.NoError(t, testDB.Table("customer").Create(&row).Error)
	id := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", id)
		testDB.Exec("DELETE FROM customer WHERE id = ?", id)
	})
	return id
}

func customerUpdatedAt(t *testing.T, id uint) int64 {
	t.Helper()
	var v int64
	require.NoError(t, testDB.Raw("SELECT updated_at FROM customer WHERE id = ?", id).Scan(&v).Error)
	return v
}

func TestChangeStage_HTTP_SuccessShapeAndNoop(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := testhelpers.CreateScopedUser(t, testDB, "s03", tn, tn.BranchID)
	contact := stageIDByName(t, "Liên hệ")
	negotiating := stageIDByName(t, "Đang đàm phán")
	id := seedStageCustomer(t, &tn.BranchID, contact, nil)
	r := stageRouter(tn.BranchID, uid)

	w := putJSON(r, fmt.Sprintf("/crm/customer/%d/stage", id), fmt.Sprintf(`{"stage_id": %d}`, negotiating))
	require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
	var resp struct {
		Data    map[string]any `json:"data"`
		Message string         `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, i18n.TL(i18n.Vi, "customer_stage_changed"), resp.Message)
	require.Len(t, resp.Data, 3, "shape {id, stage_id, updated_at}: %v", resp.Data)
	require.EqualValues(t, id, resp.Data["id"])
	require.EqualValues(t, negotiating, resp.Data["stage_id"])
	ua := customerUpdatedAt(t, id)
	require.EqualValues(t, ua, resp.Data["updated_at"], "updated_at trả về = giá trị trong DB")
	require.Greater(t, ua, int64(1000), "updated_at phải tăng")
	var updatedBy *uint
	require.NoError(t, testDB.Raw("SELECT updated_by FROM customer WHERE id = ?", id).Scan(&updatedBy).Error)
	require.NotNil(t, updatedBy)
	require.Equal(t, uid, *updatedBy, "updated_by = user đổi bước")

	hist := stageHistory(t, id)
	require.Len(t, hist, 1)
	require.NotNil(t, hist[0].FromStageID)
	require.Equal(t, contact, *hist[0].FromStageID)
	require.Equal(t, negotiating, hist[0].ToStageID)
	require.Equal(t, define.CustomerStageHistorySourceManual, hist[0].Source)
	require.NotNil(t, hist[0].ChangedBy)
	require.EqualValues(t, uid, *hist[0].ChangedBy)

	// AC6 — gửi lại đúng bước hiện tại ⇒ 200, không lịch sử mới, updated_at giữ nguyên.
	w = putJSON(r, fmt.Sprintf("/crm/customer/%d/stage", id), fmt.Sprintf(`{"stage_id": %d}`, negotiating))
	require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
	require.Len(t, stageHistory(t, id), 1, "cùng bước ⇒ không ghi lịch sử")
	require.Equal(t, ua, customerUpdatedAt(t, id), "cùng bước ⇒ updated_at không đổi")
}

// AC7 — mọi ca từ chối: KHÔNG ghi gì (stage_id, updated_at, lịch sử giữ nguyên).
func TestChangeStage_HTTP_RejectsWithoutWriting(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := testhelpers.CreateScopedUser(t, testDB, "s03", tn, tn.BranchID)
	contact := stageIDByName(t, "Liên hệ")
	consulting := stageIDByName(t, "Đang tư vấn")

	now := time.Now().Unix()
	require.NoError(t, testDB.Exec(
		"INSERT INTO customer_stage (name, type, color, sort_order, is_default, status, created_at, updated_at) VALUES (?, 1, 'gray', 99, 0, 0, ?, ?)",
		fmt.Sprintf("S03 ẩn %d", time.Now().UnixNano()), now, now).Error)
	hidden := int(testhelpers.LastInsertID(testDB))
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer_stage WHERE id = ?", hidden) })

	other := testhelpers.CreateBranch(t, testDB)
	mine := seedStageCustomer(t, &tn.BranchID, contact, nil)
	foreign := seedStageCustomer(t, &other, contact, nil)

	// Telesale: chỉ thấy khách mình phụ trách (payment_user_id = mình).
	tele := s41SeedTelesaleUser(t)
	teleOwn := seedStageCustomer(t, &tn.BranchID, contact, &tele)
	teleOther := seedStageCustomer(t, &tn.BranchID, contact, &uid)

	r := stageRouter(tn.BranchID, uid)
	rTele := stageRouter(tn.BranchID, tele)
	okBody := fmt.Sprintf(`{"stage_id": %d}`, consulting)

	cases := []struct {
		name   string
		r      *gin.Engine
		path   string
		body   string
		want   int
		target uint
	}{
		{"stage_id bước ẩn", r, fmt.Sprintf("/crm/customer/%d/stage", mine), fmt.Sprintf(`{"stage_id": %d}`, hidden), 422, mine},
		{"stage_id không tồn tại", r, fmt.Sprintf("/crm/customer/%d/stage", mine), `{"stage_id": 99999999}`, 422, mine},
		{"stage_id chuỗi", r, fmt.Sprintf("/crm/customer/%d/stage", mine), `{"stage_id": "abc"}`, 422, mine},
		{"stage_id chuỗi số", r, fmt.Sprintf("/crm/customer/%d/stage", mine), fmt.Sprintf(`{"stage_id": "%d"}`, consulting), 422, mine},
		{"stage_id số thực", r, fmt.Sprintf("/crm/customer/%d/stage", mine), `{"stage_id": 2.5}`, 422, mine},
		{"thiếu stage_id", r, fmt.Sprintf("/crm/customer/%d/stage", mine), `{}`, 422, mine},
		{"stage_id = 0", r, fmt.Sprintf("/crm/customer/%d/stage", mine), `{"stage_id": 0}`, 422, mine},
		{"khách chi nhánh khác", r, fmt.Sprintf("/crm/customer/%d/stage", foreign), okBody, 404, foreign},
		{":id = <id>abc", r, fmt.Sprintf("/crm/customer/%dabc/stage", mine), okBody, 404, mine},
		{":id không tồn tại", r, "/crm/customer/99999999/stage", okBody, 404, mine},
		{"telesale — khách người khác phụ trách", rTele, fmt.Sprintf("/crm/customer/%d/stage", teleOther), okBody, 404, teleOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := customerUpdatedAt(t, tc.target)
			w := putJSON(tc.r, tc.path, tc.body)
			require.Equal(t, tc.want, w.Code, "body: %s", w.Body.String())
			got := customerStageID(t, tc.target)
			require.NotNil(t, got)
			require.Equal(t, contact, *got, "bị từ chối ⇒ stage_id không đổi")
			require.Equal(t, before, customerUpdatedAt(t, tc.target), "bị từ chối ⇒ updated_at không đổi")
			require.Empty(t, stageHistory(t, tc.target), "bị từ chối ⇒ không có lịch sử")
		})
	}

	// Đối chứng: cùng router/telesale, khách TRONG phạm vi thì đổi được (404 phía trên là do phạm vi).
	require.Equal(t, 200, putJSON(rTele, fmt.Sprintf("/crm/customer/%d/stage", teleOwn), okBody).Code)
	require.Equal(t, 200, putJSON(r, fmt.Sprintf("/crm/customer/%d/stage", mine), okBody).Code)
	require.Equal(t, consulting, *customerStageID(t, mine))
}

// AC10 — form sửa khách gửi kèm stage_id ⇒ bị bỏ qua (không có trong whitelist), bước không đổi,
// không lịch sử.
func TestUpdateCustomer_IgnoresStageID(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := testhelpers.CreateScopedUser(t, testDB, "s03", tn, tn.BranchID)
	contact := stageIDByName(t, "Liên hệ")
	consulting := stageIDByName(t, "Đang tư vấn")
	id := seedStageCustomer(t, &tn.BranchID, contact, nil)
	r := stageRouter(tn.BranchID, uid)

	body := fmt.Sprintf(`{"fullname": "S03 Sua Ten %d", "stage_id": %d, "updated_at": %d}`,
		time.Now().UnixNano(), consulting, customerUpdatedAt(t, id))
	w := putJSON(r, fmt.Sprintf("/crm/customer/%d", id), body)
	require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
	require.Equal(t, contact, *customerStageID(t, id), "PUT form sửa khách KHÔNG được đổi bước")
	require.Empty(t, stageHistory(t, id))
	var fullname string
	require.NoError(t, testDB.Raw("SELECT fullname FROM customer WHERE id = ?", id).Scan(&fullname).Error)
	require.Contains(t, fullname, "Sua Ten", "đối chứng: field hợp lệ trong cùng request vẫn được ghi")
}

// Story Change Log 02/11/2026 — đổi bước dùng CHUNG phạm vi với GetCustomer/UpdateCustomer
// (customerByIDScope): option cross-branch (user_option id=18) BẬT ⇒ đổi được bước khách chi nhánh
// khác (200, có lịch sử); TẮT ⇒ 404, không ghi gì.
func TestChangeStage_HTTP_FollowsCrossBranchOption(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := testhelpers.CreateScopedUser(t, testDB, "s03", tn, tn.BranchID)
	contact := stageIDByName(t, "Liên hệ")
	consulting := stageIDByName(t, "Đang tư vấn")
	other := testhelpers.CreateBranch(t, testDB)
	foreign := seedStageCustomer(t, &other, contact, nil)
	r := stageRouter(tn.BranchID, uid)
	path := fmt.Sprintf("/crm/customer/%d/stage", foreign)
	body := fmt.Sprintf(`{"stage_id": %d}`, consulting)

	t.Run("option TẮT ⇒ 404", func(t *testing.T) {
		setCrossBranchOption(t, 0)
		w := putJSON(r, path, body)
		require.Equal(t, 404, w.Code, "body: %s", w.Body.String())
		require.Equal(t, contact, *customerStageID(t, foreign))
		require.Empty(t, stageHistory(t, foreign))
	})
	t.Run("option BẬT ⇒ 200", func(t *testing.T) {
		setCrossBranchOption(t, 1)
		w := putJSON(r, path, body)
		require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
		require.Equal(t, consulting, *customerStageID(t, foreign))
		hist := stageHistory(t, foreign)
		require.Len(t, hist, 1)
		require.Equal(t, consulting, hist[0].ToStageID)
	})
}
