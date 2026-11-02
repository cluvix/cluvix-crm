//go:build integration

// story-32 AC3-AC5 — GetCustomer phải áp scope telesale (crm/customer/scope.Apply) giống ListCustomer: telesale (phòng ban
// Telesale, OPTION_VIEW_LEAD_OF_TELE OFF) chỉ đọc được chi tiết khách mình phụ trách
// (payment_user_id), không phải mọi khách trong DB chỉ vì biết/đoán được id.
package contracttest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	customerHandler "erpCluvix/internal/modules/webapp/crm/customer/handler"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

// seedTelesaleUser tạo 1 user + user_profile với departmentList chứa HrDepartmentTelesale.
func seedTelesaleUser(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	now := time.Now().Unix()

	user := map[string]any{
		"username":      "s32tele" + suffix,
		"password_hash": "x",
		"status":        10,
		"created_at":    now,
		"updated_at":    now,
	}
	if err := db.Table("user").Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	userID := testhelpers.LastInsertID(db)
	t.Cleanup(func() { db.Exec("DELETE FROM user WHERE id = ?", userID) })

	profile := map[string]any{
		"user_id":        userID,
		"fullname":       "Telesale " + suffix,
		"departmentList": fmt.Sprintf("[%d]", define.HrDepartmentTelesale),
	}
	if err := db.Table("user_profile").Create(&profile).Error; err != nil {
		t.Fatalf("seed user_profile: %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM user_profile WHERE user_id = ?", userID) })

	return userID
}

// seedCustomerOwnedBy tạo 1 customer của tenant fixture, gán payment_user_id.
func seedCustomerOwnedBy(t *testing.T, db *gorm.DB, tenant *testhelpers.TestTenant, paymentUserID uint) uint {
	t.Helper()
	now := time.Now().Unix()
	row := testhelpers.WithTenantCols(tenant, map[string]any{
		"fullname":        "Owned Customer " + strconv.FormatInt(time.Now().UnixNano(), 10),
		"stage_id":        testhelpers.MustDefaultCustomerStageID(t, db),
		"payment_user_id": paymentUserID,
		"created_day":     now,
		"created_at":      now,
		"updated_at":      now,
	})
	// Giữ branch_id NULL như bản gốc (test khoá scope theo payment_user_id, KHÔNG theo branch;
	// context gọi GetCustomer không set branchActiveId nên record branch_id=NULL mới match).
	delete(row, "branch_id")
	if err := db.Table("customer").Create(&row).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	id := testhelpers.LastInsertID(db)
	t.Cleanup(func() { db.Exec("DELETE FROM customer WHERE id = ?", id) })
	return id
}

func callGetCustomerAs(db *gorm.DB, userID uint, customerID uint) *httptest.ResponseRecorder {
	h := customerHandler.New(customerHandler.Deps{DB: db})
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	id := strconv.FormatUint(uint64(customerID), 10)
	c.Request = httptest.NewRequest(http.MethodGet, "/crm/customer/"+id, nil)
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Set("userID", userID)
	h.GetCustomer(c)
	return w
}

// AC3 — telesale (option OFF) đọc khách KHÔNG phải của mình → 404; đọc khách CỦA MÌNH → 200.
func TestS32GetCustomer_TelesaleScopedToOwnCustomer(t *testing.T) {
	if testDB == nil {
		t.Skip("testDB nil — TEST_MYSQL_DSN chưa set")
	}

	// Đảm bảo OPTION_VIEW_LEAD_OF_TELE (id=5) đang OFF trong suite này (không seed dữ liệu —
	// nếu 1 test khác trong cùng process từng bật ON và không dọn, guard rõ ràng ở đây).
	testDB.Exec("DELETE FROM user_option WHERE id = 5")

	tenant := testhelpers.FreshTenant(t, testDB)
	telesaleID := seedTelesaleUser(t, testDB)
	notMineID := tenant.CustomerID // seed mặc định, payment_user_id NULL — không phải của telesale
	mineID := seedCustomerOwnedBy(t, testDB, tenant, telesaleID)

	wNotMine := callGetCustomerAs(testDB, telesaleID, notMineID)
	if wNotMine.Code != http.StatusNotFound {
		t.Fatalf("telesale đọc khách KHÔNG phải của mình phải 404, nhận HTTP %d\nbody: %s",
			wNotMine.Code, wNotMine.Body.String())
	}

	wMine := callGetCustomerAs(testDB, telesaleID, mineID)
	if wMine.Code != http.StatusOK {
		t.Fatalf("telesale đọc khách CỦA MÌNH phải 200, nhận HTTP %d\nbody: %s",
			wMine.Code, wMine.Body.String())
	}
}

// AC4 — user KHÔNG thuộc phòng ban Telesale/Convert không bị scope telesale lọc.
func TestS32GetCustomer_NonTelesaleNotFiltered(t *testing.T) {
	if testDB == nil {
		t.Skip("testDB nil — TEST_MYSQL_DSN chưa set")
	}
	testDB.Exec("DELETE FROM user_option WHERE id = 5")

	tenant := testhelpers.FreshTenant(t, testDB)
	// userID=1 giả định không có user_profile.departmentList khớp Telesale/Convert trong DB test
	// trống — scope.Apply đọc user_profile theo user_id, không tìm thấy row ⇒ IsTelesale=0.
	w := callGetCustomerAs(testDB, 999999, tenant.CustomerID)
	if w.Code != http.StatusOK {
		t.Fatalf("user không thuộc Telesale/Convert phải xem được khách bình thường (200), nhận HTTP %d\nbody: %s",
			w.Code, w.Body.String())
	}
}

// AC5 — option ON (user_option id=5, options=1) → không filter, mọi user xem được mọi khách.
func TestS32GetCustomer_OptionOn_NoFilter(t *testing.T) {
	if testDB == nil {
		t.Skip("testDB nil — TEST_MYSQL_DSN chưa set")
	}

	testDB.Exec("DELETE FROM user_option WHERE id = 5")
	opt := map[string]any{
		"id":         5,
		"name":       "OPTION_VIEW_LEAD_OF_TELE_S32_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		"options":    1,
		"status":     1,
		"created_at": time.Now().Unix(),
		"updated_at": time.Now().Unix(),
	}
	if err := testDB.Table("user_option").Create(&opt).Error; err != nil {
		t.Fatalf("seed user_option: %v", err)
	}
	t.Cleanup(func() { testDB.Exec("DELETE FROM user_option WHERE id = 5") })

	tenant := testhelpers.FreshTenant(t, testDB)
	telesaleID := seedTelesaleUser(t, testDB)
	notMineID := tenant.CustomerID // payment_user_id NULL, không phải của telesale — nhưng option ON

	w := callGetCustomerAs(testDB, telesaleID, notMineID)
	if w.Code != http.StatusOK {
		t.Fatalf("option ON phải KHÔNG filter (200) kể cả với telesale, nhận HTTP %d\nbody: %s",
			w.Code, w.Body.String())
	}
}
