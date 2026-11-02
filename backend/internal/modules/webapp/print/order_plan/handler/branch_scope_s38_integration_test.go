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
	s38BranchPlan  = uint(1)
	s38BranchOther = uint(2)
)

func newPrintCtx(w *httptest.ResponseRecorder, branchActive uint) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(w)
	c.Set("branchActiveId", branchActive)
	c.Set("branchIds", []uint{branchActive})
	c.Set("userID", uint(99))
	return c
}

// Neo hành vi story-38: PrintOrderPlan attach customer/orderServiceUseds/historyPayment vẫn PHẢI
// hoạt động khi các bản ghi con thuộc CHI NHÁNH KHÁC chi nhánh của order_plan/session — trước
// story-38 đi qua scope chỉ lọc thương hiệu; sau story-38 phải là
// `tenant.CrossBranchLookup` (identity) để giữ NGUYÊN hành vi không lọc branch khi attach.
func TestPrintOrderPlan_AttachChiNhanhKhac(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	testDB.Exec(`DELETE FROM payment`)
	testDB.Exec(`DELETE FROM order_service_used`)
	testDB.Exec(`DELETE FROM order_plan`)
	testDB.Exec(`DELETE FROM service`)
	testDB.Exec(`DELETE FROM customer`)

	if err := testDB.Exec(`INSERT INTO customer (id, branch_id, fullname, phone, address, stage_id)
		VALUES (801, ?, 'Khach Chi Nhanh Khac', '0900000088', 'Dia chi test', (SELECT id FROM customer_stage WHERE is_default = 1))`, s38BranchOther).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := testDB.Exec(`INSERT INTO order_plan (id, branch_id, customer_id, code)
		VALUES (801, ?, 801, 'OTP801')`, s38BranchPlan).Error; err != nil {
		t.Fatalf("seed order_plan: %v", err)
	}
	if err := testDB.Exec(`INSERT INTO service (id, name, code, status)
		VALUES (801, 'Dich vu test', 'SVC801', 1)`).Error; err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := testDB.Exec(`INSERT INTO order_service_used
		(id, branch_id, customer_id, service_id, order_plan_id, code, quantity, unit_price, totalEnd, payment)
		VALUES (801, ?, 801, 801, 801, 'OSU801', 1, 100000, 100000, 0)`, s38BranchOther).Error; err != nil {
		t.Fatalf("seed order_service_used: %v", err)
	}
	if err := testDB.Exec(`INSERT INTO payment
		(id, branch_id, customer_id, parent_id, code, amount, payment_time, method, payment_for)
		VALUES (801, ?, 801, 801, 'PM801', 50000, 1788195700, 1, 4)`, s38BranchOther).Error; err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	w := httptest.NewRecorder()
	c := newPrintCtx(w, s38BranchPlan)
	c.Params = gin.Params{{Key: "id", Value: "801"}}
	c.Request = httptest.NewRequest("GET", "/api/print/order-plan/801", nil)

	h := &Handler{Deps: Deps{DB: testDB}}
	h.PrintOrderPlan(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body struct {
		OrderPlan      map[string]interface{}   `json:"orderPlan"`
		HistoryPayment []map[string]interface{} `json:"historyPayment"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	customer, ok := body.OrderPlan["customer"].(map[string]interface{})
	if !ok || customer == nil {
		t.Fatalf("customer chi nhanh khac KHONG duoc attach: orderPlan=%+v", body.OrderPlan)
	}
	services, ok := body.OrderPlan["orderServiceUseds"].([]interface{})
	if !ok || len(services) != 1 {
		t.Fatalf("orderServiceUseds chi nhanh khac KHONG duoc attach: orderPlan=%+v", body.OrderPlan)
	}
	if len(body.HistoryPayment) != 1 {
		t.Fatalf("payment chi nhanh khac KHONG duoc attach: historyPayment=%+v", body.HistoryPayment)
	}
}
