//go:build integration

package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	appdb "erpCluvix/db"
	"erpCluvix/internal/testhelpers"
)

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
	appdb.DB = s.DB
	os.Exit(m.Run())
}

const (
	s38BranchPayment  = uint(1)
	s38BranchCustomer = uint(2)
)

func newPrintCtx(w *httptest.ResponseRecorder, branchActive uint) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(w)
	c.Set("branchActiveId", branchActive)
	c.Set("branchIds", []uint{branchActive})
	c.Set("userID", uint(99))
	return c
}

// Neo hành vi story-38: PrintPayment attach customer/orderServiceUsed vẫn PHẢI hoạt động khi
// customer/order_service_used thuộc CHI NHÁNH KHÁC chi nhánh của payment/session — trước
// story-38 việc này đi qua scope chỉ lọc thương hiệu (KHÔNG lọc branch); sau
// story-38 phải đổi sang `tenant.CrossBranchLookup` (identity) để giữ NGUYÊN hành vi
// đó. Đổi nhầm sang `ScopedDBNullBranch` sẽ làm mất khách/dịch vụ chi nhánh khác.
func TestPrintPayment_AttachChiNhanhKhac(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	testDB.Exec(`DELETE FROM payment`)
	testDB.Exec(`DELETE FROM order_service_used`)
	testDB.Exec(`DELETE FROM order_plan`)
	testDB.Exec(`DELETE FROM service`)
	testDB.Exec(`DELETE FROM customer`)

	if err := testDB.Exec(`INSERT INTO customer (id, branch_id, fullname, phone, address, stage_id)
		VALUES (701, ?, 'Khach Chi Nhanh Khac', '0900000077', 'Dia chi test', (SELECT id FROM customer_stage WHERE is_default = 1))`, s38BranchCustomer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := testDB.Exec(`INSERT INTO service (id, name, code, status)
		VALUES (701, 'Dich vu test', 'SVC701', 1)`).Error; err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := testDB.Exec(`INSERT INTO order_plan (id, branch_id, customer_id, code)
		VALUES (701, ?, 701, 'OTP701')`, s38BranchCustomer).Error; err != nil {
		t.Fatalf("seed order_plan: %v", err)
	}
	if err := testDB.Exec(`INSERT INTO order_service_used
		(id, branch_id, customer_id, service_id, order_plan_id, code, quantity, unit_price, totalEnd, payment)
		VALUES (701, ?, 701, 701, 701, 'OSU701', 1, 100000, 100000, 0)`, s38BranchCustomer).Error; err != nil {
		t.Fatalf("seed order_service_used: %v", err)
	}
	if err := testDB.Exec(`INSERT INTO payment
		(id, branch_id, customer_id, serviceUsed_id, code, amount, payment_time, method, payment_for)
		VALUES (701, ?, 701, 701, 'PM701', 50000, 1788195700, 1, 4)`, s38BranchPayment).Error; err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	w := httptest.NewRecorder()
	// Session active branch = chi nhánh CỦA PAYMENT (khác chi nhánh khách/dịch vụ).
	c := newPrintCtx(w, s38BranchPayment)
	c.Params = gin.Params{{Key: "id", Value: "701"}}
	c.Request = httptest.NewRequest("GET", "/api/print/payment/701", nil)

	h := &Handler{Deps: Deps{DB: testDB}}
	h.PrintPayment(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Payment map[string]interface{} `json:"payment"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	customer, ok := body.Payment["customer"].(map[string]interface{})
	if !ok || customer == nil {
		t.Fatalf("customer chi nhanh khac KHONG duoc attach: payment=%+v", body.Payment)
	}
	if customer["fullname"] != "Khach Chi Nhanh Khac" {
		t.Fatalf("attach sai khach: %+v", customer)
	}
	osu, ok := body.Payment["orderServiceUsed"].(map[string]interface{})
	if !ok || osu == nil {
		t.Fatalf("orderServiceUsed chi nhanh khac KHONG duoc attach: payment=%+v", body.Payment)
	}
	if osu["code"] != "OSU701" {
		t.Fatalf("attach sai orderServiceUsed: %+v", osu)
	}
}
