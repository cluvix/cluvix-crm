//go:build integration

// Story-41 (E6/D15) — phạm vi chi nhánh của GetCustomer + UpdateCustomer với khách ở chi nhánh KHÁC
// chi nhánh của người gọi. Trước đây cả hai đi `ScopedDBNullBranchSkipBranch(..., true)` (neo bằng
// test chờ quyết) ⇒ user chi nhánh A đọc/sửa được khách chi nhánh B theo id. Đã quyết chặn (fix IDOR
// xuyên chi nhánh, 2026-10-20): lọc chi nhánh như list, chỉ bỏ lọc khi user_option cross-branch
// (id=18) bật — giống SearchGlobal. Scope telesale (scope.Apply) của GetCustomer vẫn áp như cũ.
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	customerService "erpCluvix/internal/modules/webapp/crm/customer/service"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

// s41SeedCustomerInBranch chèn khách ở chi nhánh branchID.
func s41SeedCustomerInBranch(t *testing.T, tenant *testhelpers.TestTenant, branchID uint, paymentUserID *uint) uint {
	t.Helper()
	now := time.Now().Unix()
	row := map[string]any{
		"branch_id":       branchID,
		"fullname":        "S41 Cross " + strconv.FormatInt(time.Now().UnixNano(), 10),
		"stage_id":        testhelpers.MustDefaultCustomerStageID(t, testDB),
		"payment_user_id": paymentUserID,
		"created_day":     now,
		"created_at":      now,
		"updated_at":      now,
	}
	require.NoError(t, testDB.Table("customer").Create(&row).Error)
	id := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer WHERE id = ?", id) })
	return id
}

// s41SeedTelesaleUser — user + user_profile thuộc phòng ban Telesale (scope.Apply lọc theo payment_user_id).
func s41SeedTelesaleUser(t *testing.T) uint {
	t.Helper()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	now := time.Now().Unix()
	user := map[string]any{
		"username": "s41tele" + suffix, "password_hash": "x", "status": 10,
		"created_at": now, "updated_at": now,
	}
	require.NoError(t, testDB.Table("user").Create(&user).Error)
	uid := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() { testDB.Exec("DELETE FROM user WHERE id = ?", uid) })
	profile := map[string]any{
		"user_id": uid, "fullname": "S41 Telesale " + suffix,
		"departmentList": fmt.Sprintf("[%d]", define.HrDepartmentTelesale),
	}
	require.NoError(t, testDB.Table("user_profile").Create(&profile).Error)
	t.Cleanup(func() { testDB.Exec("DELETE FROM user_profile WHERE user_id = ?", uid) })
	return uid
}

// s41Ctx — session chỉ có chi nhánh (story-58: session không còn company).
func s41Ctx(w *httptest.ResponseRecorder, branchID, userID uint) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(w)
	c.Set("branchActiveId", branchID)
	c.Set("branchIds", []uint{branchID})
	c.Set("userID", userID)
	return c
}

func s41Get(t *testing.T, branchID, userID, customerID uint) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c := s41Ctx(w, branchID, userID)
	id := strconv.FormatUint(uint64(customerID), 10)
	c.Request = httptest.NewRequest(http.MethodGet, "/crm/customer/"+id, nil)
	c.Params = gin.Params{{Key: "id", Value: id}}
	h := &Handler{DB: testDB, CustomerSvc: customerService.NewCustomerService(testDB)}
	h.GetCustomer(c)
	return w
}

func s41Update(t *testing.T, branchID, userID, customerID uint, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c := s41Ctx(w, branchID, userID)
	id := strconv.FormatUint(uint64(customerID), 10)
	c.Request = httptest.NewRequest(http.MethodPut, "/crm/customer/"+id, strings.NewReader(string(raw)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: id}}
	h := &Handler{DB: testDB, CustomerSvc: customerService.NewCustomerService(testDB)}
	h.UpdateCustomer(c)
	return w
}

func TestS41GetCustomer_CrossBranchByID_ChanTheoChiNhanh(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	testDB.Exec("DELETE FROM user_option WHERE id = 5") // OPTION_VIEW_LEAD_OF_TELE OFF
	setCrossBranchOption(t, 0)
	tn := testhelpers.FreshTenant(t, testDB)
	branchB := testhelpers.CreateBranch(t, testDB)
	custB := s41SeedCustomerInBranch(t, tn, branchB, nil)

	// Đổi hành vi (fix IDOR xuyên chi nhánh): user chi nhánh A đọc khách chi nhánh B theo id ⇒ 404.
	w := s41Get(t, tn.BranchID, 999999, custB)
	require.Equal(t, http.StatusNotFound, w.Code,
		"GetCustomer phải lọc chi nhánh như list. body=%s", w.Body.String())

	// Scope telesale vẫn áp trong chi nhánh của mình: khách không phải của mình ⇒ 404, của mình ⇒ 200.
	tele := s41SeedTelesaleUser(t)
	custA := s41SeedCustomerInBranch(t, tn, tn.BranchID, nil)
	require.Equal(t, http.StatusNotFound, s41Get(t, tn.BranchID, tele, custA).Code,
		"telesale scope phải chặn khách không phải của mình")
	mine := s41SeedCustomerInBranch(t, tn, tn.BranchID, &tele)
	require.Equal(t, http.StatusOK, s41Get(t, tn.BranchID, tele, mine).Code,
		"telesale đọc khách của mình ở chi nhánh mình ⇒ 200")

	// user_option cross-branch (id=18) bật ⇒ mở được khách chi nhánh khác (giống SearchGlobal).
	setCrossBranchOption(t, 1)
	require.Equal(t, http.StatusOK, s41Get(t, tn.BranchID, 999999, custB).Code,
		"cross-branch bật ⇒ đọc được khách chi nhánh khác")
}

func TestS41UpdateCustomer_CrossBranchByID_ChanTheoChiNhanh(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	setCrossBranchOption(t, 0)
	tn := testhelpers.FreshTenant(t, testDB)
	branchB := testhelpers.CreateBranch(t, testDB)
	custB := s41SeedCustomerInBranch(t, tn, branchB, nil)

	var before int64
	require.NoError(t, testDB.Table("customer").Where("id = ?", custB).Pluck("updated_at", &before).Error)

	// User chi nhánh A sửa khách chi nhánh B theo id từ URL ⇒ 404, DB không đổi.
	w := s41Update(t, tn.BranchID, 999999, custB, map[string]any{
		"updated_at": before,
		"nickname":   "s41-cross-branch",
	})
	require.Equal(t, http.StatusNotFound, w.Code,
		"UpdateCustomer phải lọc chi nhánh như GetCustomer. body=%s", w.Body.String())
	var nick string
	require.NoError(t, testDB.Table("customer").Where("id = ?", custB).Pluck("COALESCE(nickname, '')", &nick).Error)
	require.NotEqual(t, "s41-cross-branch", nick, "khách chi nhánh khác không được bị sửa")

	// Khách chi nhánh A: sửa được, nhưng KHÔNG được kéo sang chi nhánh ngoài phạm vi (B) ⇒ 422.
	custA := s41SeedCustomerInBranch(t, tn, tn.BranchID, nil)
	var beforeA int64
	require.NoError(t, testDB.Table("customer").Where("id = ?", custA).Pluck("updated_at", &beforeA).Error)
	w = s41Update(t, tn.BranchID, 999999, custA, map[string]any{"updated_at": beforeA, "branch_id": branchB})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, "đổi branch_id ngoài phạm vi phải bị chặn. body=%s", w.Body.String())
	var branchAfter uint
	require.NoError(t, testDB.Table("customer").Where("id = ?", custA).Pluck("branch_id", &branchAfter).Error)
	require.Equal(t, tn.BranchID, branchAfter)

	w = s41Update(t, tn.BranchID, 999999, custA, map[string]any{"updated_at": beforeA, "nickname": "s41-own-branch"})
	require.Equal(t, http.StatusOK, w.Code, "sửa khách chi nhánh mình ⇒ 200. body=%s", w.Body.String())

	// Id không tồn tại vẫn 404.
	require.Equal(t, http.StatusNotFound, s41Update(t, tn.BranchID, 999999, 987654321,
		map[string]any{"updated_at": 1, "nickname": "x"}).Code)
}
