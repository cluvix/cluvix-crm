//go:build integration

package handler

// DELETE /crm/customer/:id — đi qua crmServices.DeleteCustomer: khách đã có đơn hàng / phiếu thu-chi ⇒ 422,
// không xoá gì (user chốt 2026-11-01; bản cũ xoá kèm phiếu qua deletePayment). (crm-standalone story-09: bỏ guard kỳ kế toán đã khoá + bút toán đảo sổ kép
// cùng module kế toán ⇒ bỏ subtest "kỳ đã khoá ⇒ 422" và assert event đảo.)

import (
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"erpCluvix/db"
	customerService "erpCluvix/internal/modules/webapp/crm/customer/service"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/logger"
)

func callDeleteCustomer(t *testing.T, tn *testhelpers.TestTenant, userID uint) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	id := strconv.FormatUint(uint64(tn.CustomerID), 10)
	c.Request = httptest.NewRequest("DELETE", "/crm/customer/"+id, nil)
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Set("userID", userID)
	h := &Handler{DB: testDB, CustomerSvc: customerService.NewCustomerService(testDB)}
	h.DeleteCustomer(c)
	return w
}

func TestDeleteCustomer_WithPaymentsBlocked(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	// deletePayment đọc logger.L toàn cục (nil trong test ⇒ panic ở nhánh log) — trỏ tạm db.DB/logger.L.
	logger.Init(logger.Config{Env: "test", AppCode: "crm_customer_handler_test"})
	origDB := db.DB
	db.DB = testDB
	t.Cleanup(func() { db.DB = origDB })

	setup := func(t *testing.T) (*testhelpers.TestTenant, uint, uint) {
		tn := testhelpers.FreshTenant(t, testDB)
		userID := testhelpers.CreateUser(t, testDB, "delcus", tn)
		payID, err := testhelpers.CreatePayment(testDB, tn, decimal.NewFromInt(300000), define.PaymentForDeposit)
		require.NoError(t, err)
		now := time.Now()
		require.NoError(t, testDB.Exec("UPDATE payment SET payment_of_date = ? WHERE id = ?", now.Unix(), payID).Error)
		t.Cleanup(func() {
			for _, tbl := range []string{"log_customer", "log_payment"} {
				testDB.Exec("DELETE FROM "+tbl+" WHERE changed_by = ?", userID)
			}
		})
		require.NoError(t, testDB.Exec("SET SESSION foreign_key_checks = 1").Error)
		t.Cleanup(func() { testDB.Exec("SET SESSION foreign_key_checks = 0") })
		return tn, userID, payID
	}
	count := func(t *testing.T, sql string, args ...any) int64 {
		var n int64
		require.NoError(t, testDB.Raw(sql, args...).Scan(&n).Error)
		return n
	}

	t.Run("422, khách có phiếu cọc ⇒ khách + phiếu còn nguyên", func(t *testing.T) {
		tn, userID, payID := setup(t)
		w := callDeleteCustomer(t, tn, userID)
		require.Equal(t, 422, w.Code, w.Body.String())
		require.EqualValues(t, 1, count(t, "SELECT COUNT(*) FROM customer WHERE id = ?", tn.CustomerID))
		require.EqualValues(t, 1, count(t, "SELECT COUNT(*) FROM payment WHERE id = ?", payID))
	})
}

// Story-65 (D26a): DB mới thêm FK RESTRICT thật cho `attachment.customer_id` — xoá khách còn file
// đính kèm phải trả 4xx + i18n "đang được dùng", KHÔNG 500. Cột `customer_id` (NOT NULL, không
// default) đã có sẵn trên schema CŨ (migration squash 20260814091602) nhưng CHƯA có FK constraint
// — test tự thêm tạm đúng ràng buộc đó để chốt hành vi handler TRƯỚC khi story-25/26 gắn FK thật,
// và drop lại ở cuối test — không đụng schema thật của DB dev/prod.
func TestDeleteCustomer_WithAttachmentBlocks(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	logger.Init(logger.Config{Env: "test", AppCode: "crm_customer_handler_test"})
	origDB := db.DB
	db.DB = testDB
	t.Cleanup(func() { db.DB = origDB })

	tn := testhelpers.FreshTenant(t, testDB)
	userID := testhelpers.CreateUser(t, testDB, "delcusatt", tn)

	t.Cleanup(func() {
		testDB.Exec("ALTER TABLE attachment DROP FOREIGN KEY fk_s65_attachment_customer")
	})
	require.NoError(t, testDB.Exec(
		"ALTER TABLE attachment ADD CONSTRAINT fk_s65_attachment_customer FOREIGN KEY (customer_id) REFERENCES customer(id) ON DELETE RESTRICT").Error)

	attRow := map[string]any{
		"ref_type": "customer", "ref_id": tn.CustomerID, "file_name": "a.pdf", "file_url": "https://x/a.pdf",
		"customer_id": tn.CustomerID, "branch_id": tn.BranchID, "created_at": time.Now().Unix(),
	}
	require.NoError(t, testDB.Table("attachment").Create(&attRow).Error)
	attID := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() { testDB.Exec("DELETE FROM attachment WHERE id = ?", attID) })

	require.NoError(t, testDB.Exec("SET SESSION foreign_key_checks = 1").Error)
	t.Cleanup(func() { testDB.Exec("SET SESSION foreign_key_checks = 0") })

	w := callDeleteCustomer(t, tn, userID)
	require.Equal(t, 422, w.Code, "khách có đính kèm phải bị chặn xoá (4xx), KHÔNG 500 — body: %s", w.Body.String())
	require.Contains(t, w.Body.String(), "sử dụng", "message phải là i18n fk_constraint (\"đang được dùng\")")

	var n int64
	require.NoError(t, testDB.Raw("SELECT COUNT(*) FROM customer WHERE id = ?", tn.CustomerID).Scan(&n).Error)
	require.EqualValues(t, 1, n, "rollback: khách phải còn nguyên")
}
