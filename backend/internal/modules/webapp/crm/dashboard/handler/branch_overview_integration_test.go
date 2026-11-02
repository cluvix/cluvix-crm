//go:build integration

package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

// Neo GET /dashboard/crm/payment-branch-overview:
//   - M-19 (user chốt 2026-10-18): cột "Đặt cọc" = tổng tiền cọc NHẬN trong kỳ (SUM amount), không
//     phải số dư deposit_after — cọc đã chuyển/hoàn hết (deposit_after = 0) vẫn phải hiện số đã nhận.
//   - M-15: `branch_id` từ query phải thuộc quyền chi nhánh của user, không thì 403.
//
// Chạy trên DB dùng-một-lần (TEST_MYSQL_DSN, tên *_test/*_tmp), KHÔNG chạm DB dev.

var testDB *gorm.DB

func TestMain(m *testing.M) {
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaAll)
	if err != nil {
		if errors.Is(err, testhelpers.ErrNoDSN) {
			log.Printf("testhelpers: %v — skipping integration tests", err)
			os.Exit(m.Run())
		}
		log.Fatalf("testhelpers: đã có TEST_MYSQL_DSN nhưng dựng schema hỏng: %v", err)
	}
	defer s.Close()
	testDB = s.DB
	os.Exit(m.Run())
}

const (
	bovB1 = 41
	bovB2 = 42
	bovT  = int64(1788195700)
)

func bovExec(t *testing.T, sql string, args ...interface{}) {
	t.Helper()
	if err := testDB.Exec(sql, args...).Error; err != nil {
		t.Fatalf("seed: %v\n%s", err, sql)
	}
}

// seedBranchOverview — CN1: cọc 200.000 đã dùng hết (deposit_after 0) + cọc 300.000 còn nguyên
// (deposit_after 300.000) + cọc ký gửi 400.000 (bị loại). CN2: cọc 999 (deposit_after 0).
func seedBranchOverview(t *testing.T) {
	t.Helper()
	for _, tbl := range []string{"payment", "customer", "branch"} {
		bovExec(t, "DELETE FROM `"+tbl+"`")
	}
	bovExec(t, `INSERT INTO branch (id, name) VALUES (?, 'CN1'), (?, 'CN2')`, bovB1, bovB2)
	bovExec(t, `INSERT INTO customer (id, code, branch_id, fullname, phone, stage_id) VALUES
		(951, 'KH951', ?, 'Khách Một', '0900000951', (SELECT id FROM customer_stage WHERE is_default = 1)), (952, 'KH952', ?, 'Khách Hai', '0900000952', (SELECT id FROM customer_stage WHERE is_default = 1))`, bovB1, bovB2)
	ins := func(id, customer, branch, depositType int, amount, depositAfter string) {
		bovExec(t, `INSERT INTO payment
			(id, code, customer_id, branch_id, method, amount, deposit_after, deposit_type, payment_for,
			 payment_time, payment_of_date, created_at, updated_at, note)
			VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, 'ghi chú')`,
			id, fmt.Sprintf("PT%d", id), customer, branch, amount, depositAfter, depositType,
			define.PaymentForDeposit, bovT, bovT, bovT, bovT)
	}
	ins(9501, 951, bovB1, 0, "200000", "0")
	ins(9502, 951, bovB1, 0, "300000", "300000")
	ins(9503, 951, bovB1, define.PaymentTypeDepositKG, "400000", "400000")
	ins(9504, 952, bovB2, 0, "999", "0")
}

func callOverview(t *testing.T, rawQuery string, branchIDs []uint) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x?"+rawQuery, nil)
	if len(branchIDs) > 0 {
		c.Set("branchIds", branchIDs)
	}
	c.Set("userID", uint(99))
	(&Handler{Deps: Deps{DB: testDB}}).CrmPaymentBranchOverview(c)
	return w
}

func overviewDeposits(t *testing.T, w *httptest.ResponseRecorder) map[int64]decimal.Decimal {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("phải 200, got %d %s", w.Code, w.Body.String())
	}
	var rows []struct {
		ID           int64           `json:"id"`
		TotalDeposit decimal.Decimal `json:"totalDeposit"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatalf("parse JSON: %v — %s", err, w.Body.String())
	}
	out := make(map[int64]decimal.Decimal, len(rows))
	for _, r := range rows {
		out[r.ID] = r.TotalDeposit
	}
	return out
}

func overviewQS() string {
	return fmt.Sprintf("from=%d&to=%d", bovT-3600, bovT+86400)
}

func TestPaymentBranchOverview_DatCocLaTienNhanTrongKy(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedBranchOverview(t)

	got := overviewDeposits(t, callOverview(t, overviewQS(), []uint{bovB1, bovB2}))
	// CN1: 200.000 (đã dùng hết) + 300.000 = 500.000 — số dư deposit_after chỉ là 300.000.
	if want := decimal.RequireFromString("500000"); !got[bovB1].Equal(want) {
		t.Errorf("CN1 totalDeposit = %s, muốn %s (tiền cọc nhận, không phải số dư)", got[bovB1], want)
	}
	// CN2: cọc 999 đã dùng hết ⇒ bản cũ (deposit_after) ra 0 và dòng CN2 biến mất.
	if want := decimal.RequireFromString("999"); !got[bovB2].Equal(want) {
		t.Errorf("CN2 totalDeposit = %s, muốn %s", got[bovB2], want)
	}
}

func TestPaymentBranchOverview_BranchIDNgoaiQuyen403(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedBranchOverview(t)

	// User chỉ có quyền CN1, cố xem CN2 ⇒ 403.
	if w := callOverview(t, overviewQS()+fmt.Sprintf("&branch_id=%d", bovB2), []uint{bovB1}); w.Code != http.StatusForbidden {
		t.Fatalf("branch_id ngoài quyền phải 403, got %d %s", w.Code, w.Body.String())
	}
	// Chi nhánh trong quyền ⇒ 200, chỉ số liệu CN1.
	got := overviewDeposits(t, callOverview(t, overviewQS()+fmt.Sprintf("&branch_id=%d", bovB1), []uint{bovB1}))
	if _, leak := got[bovB2]; leak {
		t.Errorf("chọn CN1 nhưng có dòng CN2: %+v", got)
	}
	if want := decimal.RequireFromString("500000"); !got[bovB1].Equal(want) {
		t.Errorf("CN1 totalDeposit = %s, muốn %s", got[bovB1], want)
	}
}

func TestAppointmentAcceptedStats_BranchIDNgoaiQuyen403(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/x?branch_id=%d", bovB2), nil)
	c.Set("branchIds", []uint{bovB1})
	(&Handler{Deps: Deps{DB: testDB}}).CrmAppointmentAcceptedStats(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("branch_id ngoài quyền phải 403, got %d %s", w.Code, w.Body.String())
	}
}
