//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Neo bản vá gộp của GET /crm/report/branch-statistics (2026-09-09):
// B/C/D (3 lượt quét `appointment` cùng dải) → 1, và R/S/T/U (4 lượt quét `payment` cùng dải)
// → 1. Cả hai đổi THỨ TỰ placeholder trong câu raw SQL — kiểu lỗi im lặng nhất của bản vá này.
// Test khoá CON SỐ của cả 7 chỉ số bị gộp.
//
// Chạy trên DB dùng-một-lần (TEST_MYSQL_DSN), KHÔNG chạm DB dev.

const (
	bsBranch = 44
	bsType   = 4410
	bsTypeNo = 4411
	bsFrom   = 1788195600
	bsTo     = 1788973200
)

// seedBranchStatistics dựng phân bố biết trước cho chi nhánh 44:
//
//	lịch hẹn (loại tính doanh thu, trong dải appointment_date):
//	  4431 status 2 accepted 1 · 4432 status 3 accepted 0 · 4433 status 1 (chưa đến)
//	  4434 loại KHÔNG tính doanh thu       -> loại khỏi mọi chỉ số
//	  4435 status 2 nhưng NGOÀI dải ngày   -> loại
//	⇒ totalAppointment = 3 · totalAppointmentDen = 2 · totalAppointmentDongY = 1
//
//	payment trong dải payment_time:
//	  4441 for=4 (order)      amount 100                -> totalPayment
//	  4442 for=3 (deposit)    amount 50, deposit_after 70 -> totalPayment + totalDeposit
//	  4443 for=3 deposit_type=3 (KG) amount 999          -> bị loại khỏi cả hai
//	  4444 for=2 (refund deposit) amount 20              -> totalRefundDeposit
//	  4445 for=1 (refund service) amount 30              -> totalRefundServiceUsed
//	  4446 for=4 amount 777 nhưng NGOÀI dải              -> loại
//	⇒ totalPayment = 150 · totalDeposit = 70 · totalRefundDeposit = 20 · totalRefundServiceUsed = 30
func seedBranchStatistics(t *testing.T) {
	t.Helper()
	testDB.Exec(`DELETE FROM appointment`)
	testDB.Exec(`DELETE FROM payment`)
	testDB.Exec(`DELETE FROM appointment_type WHERE id IN (?, ?)`, bsType, bsTypeNo)
	testDB.Exec(`DELETE FROM customer WHERE id = 4421`)
	testDB.Exec(`DELETE FROM branch WHERE id = ?`, bsBranch)

	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed %s: %v", what, err)
		}
	}
	must("branch", testDB.Exec(`INSERT INTO branch (id, name) VALUES (?, 'CN QA')`, bsBranch).Error)
	must("customer", testDB.Exec(`INSERT INTO customer (id, branch_id, fullname, created_day, stage_id)
		VALUES (4421, ?, 'KH', ?, (SELECT id FROM customer_stage WHERE is_default = 1))`, bsBranch, bsFrom+100).Error)
	for id, rev := range map[int]int{bsType: 1, bsTypeNo: 0} {
		must("appointment_type", testDB.Exec(`INSERT INTO appointment_type (id, name, isTypeRevenue) VALUES (?, 'LH', ?)`, id, rev).Error)
	}

	appts := []struct {
		id, typeID, status, accepted int
		apptDate                     int64
	}{
		{4431, bsType, 2, 1, bsFrom + 100},
		{4432, bsType, 3, 0, bsFrom + 200},
		{4433, bsType, 1, 0, bsFrom + 300},
		{4434, bsTypeNo, 2, 1, bsFrom + 400},
		{4435, bsType, 2, 1, bsTo + 86400*40},
	}
	for _, a := range appts {
		must("appointment", testDB.Exec(`INSERT INTO appointment
			(id, code, branch_id, customer_id, appointment_type_id, appointment_date,
			 created_at, status, is_accepted)
			VALUES (?, ?, ?, 4421, ?, ?, ?, ?, ?)`,
			a.id, fmt.Sprintf("AP%d", a.id), bsBranch, a.typeID, a.apptDate,
			a.apptDate, a.status, a.accepted).Error)
	}

	pays := []struct {
		id, paymentFor       int
		depositType          any
		amount, depositAfter int
		payTime              int64
	}{
		{4441, 4, nil, 100, 0, bsFrom + 100},
		{4442, 3, nil, 50, 70, bsFrom + 200},
		{4443, 3, 3, 999, 999, bsFrom + 300},
		{4444, 2, nil, 20, 0, bsFrom + 400},
		{4445, 1, nil, 30, 0, bsFrom + 500},
		{4446, 4, nil, 777, 0, bsTo + 86400*40},
	}
	for _, p := range pays {
		must("payment", testDB.Exec(`INSERT INTO payment
			(id, code, branch_id, customer_id, payment_for, deposit_type,
			 method, amount, deposit_after, payment_time, payment_of_date)
			VALUES (?, ?, ?, 4421, ?, ?, 4, ?, ?, ?, ?)`,
			p.id, fmt.Sprintf("PM%d", p.id), bsBranch, p.paymentFor, p.depositType,
			p.amount, p.depositAfter, p.payTime, p.payTime).Error)
	}
}

func TestBranchStatistics_BayChiSoSauKhiGop(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedBranchStatistics(t)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET",
		fmt.Sprintf("/?filter[created_at][gte]=%d&filter[created_at][lte]=%d", bsFrom, bsTo), nil)
	c.Set("branchIds", []uint{bsBranch})
	c.Set("userID", uint(99))

	h := &Handler{Deps: Deps{DB: testDB}}
	h.BranchStatistics(c)
	if w.Code >= 400 {
		t.Fatalf("phải 200, got %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse JSON: %v — %s", err, w.Body.String())
	}
	if len(body.Items) != 1 {
		t.Fatalf("chỉ chi nhánh %d có số liệu ⇒ 1 dòng, got %d: %+v", bsBranch, len(body.Items), body.Items)
	}
	row := body.Items[0]
	for _, tc := range []struct {
		field string
		want  string
		why   string
	}{
		{"totalAppointment", "3", "3 lịch loại tính doanh thu trong dải (loại-không-doanh-thu và ngoài-dải bị loại)"},
		{"totalAppointmentDen", "2", "trong 3 lịch đó chỉ 2 lịch status 2/3"},
		{"totalAppointmentDongY", "1", "chỉ lịch 4431 vừa status 2/3 vừa is_accepted = 1"},
		{"totalPayment", "150", "for=4 (100) + for=3 (50); bản KG deposit_type=3 bị loại"},
		{"totalDeposit", "70", "deposit_after của for=3 không phải KG"},
		{"totalRefundDeposit", "20", "for=2"},
		{"totalRefundServiceUsed", "30", "for=1"},
	} {
		if fmt.Sprint(row[tc.field]) != tc.want {
			t.Fatalf("%s = %v, muốn %s — %s", tc.field, row[tc.field], tc.want, tc.why)
		}
	}
}
