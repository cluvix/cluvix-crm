//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"erpCluvix/pkg/define"
	"erpCluvix/pkg/logger"
)

// `pagination.FailQuery` ghi log qua `logger.L`; trong test binary nó nil ⇒ panic che mất
// thông điệp lỗi SQL thật. Gắn observer để lỗi SQL hiện nguyên văn khi test đỏ — nếu không,
// mọi hỏng hóc của câu raw SQL đều hiện ra như một dòng "500 Đã có lỗi xảy ra" vô dụng.
var dimLogs *observer.ObservedLogs

func init() {
	var core zapcore.Core
	core, dimLogs = observer.New(zapcore.ErrorLevel)
	logger.L = zap.New(core)
}

// Neo bản vá gộp của `dimensionStatistics` (2026-09-09) — hàm dùng chung cho 8 endpoint
// `*-statistics`. Gộp B/C/D (3 lượt quét `appointment` cùng dải) → 1, và nhánh tài chính
// R/S/T/U → 1 (useOsuFinancial=false) hoặc R+U / S+T (=true).
//
// ⚠ Kiểu lỗi nguy hiểm nhất ở đây là **lệch số tham số `fmt.Sprintf`**: `financialSQLArgs` là
// slice động nên `go vet` KHÔNG kiểm được. Lệch thì câu SQL mang `%!d(MISSING)` và MySQL báo
// lỗi — test này chạy CẢ HAI biến thể nên bắt được ngay.
//
// Chạy trên DB dùng-một-lần (TEST_MYSQL_DSN), KHÔNG chạm DB dev.

const (
	dimBranch = 33
	dimSource = 3310
	dimProv   = 3320
	dimType   = 3330
	dimTypeNo = 3331
	dimFrom   = 1788195600
	dimTo     = 1788973200

	dimSvc  = 3361
	dimPlan = 3362
	dimOsu  = 3363
	dimPayM = 3364
)

// seedDimension dựng phân bố biết trước, dùng chung cho cả hai biến thể:
//
//	khách 3341 (source 3310, province 3320) — tạo trong dải
//	lịch hẹn loại tính doanh thu, trong dải appointment_date:
//	  3351 status 2 accepted 1 · 3352 status 3 accepted 0 · 3353 status 1
//	  3354 loại KHÔNG tính doanh thu -> loại · 3355 ngoài dải -> loại
//	⇒ totalAppointment = 3 · totalAppointmentDen = 2 · totalAppointmentDongY = 1
//
// Khối tài chính (osu 3363 totalEnd = 1234, mọi payment trong dải):
//
//	A for=Order       amount 1000            → totalRevenue 1000 · totalPayment +1000
//	B for=Deposit     amount 200 sau 500     → totalPayment +200 · totalDeposit 500
//	C for=RefundDep   amount  70             → totalRefundDeposit 70
//	D for=RefundSvc   amount   9             → totalRefundServiceUsed 9
//	E for=Deposit deposit_type=KG amount 5000 → BỊ LOẠI khỏi cả totalPayment lẫn totalDeposit
//
// Bốn số tiền cố ý ĐÔI MỘT KHÁC NHAU: hoán vị bất kỳ hằng `payment_for` nào trong
// `financialSQLArgs` sẽ đổi kết quả ⇒ test đỏ. Trước 2026-09-09 test không seed `payment`
// nên cả 4 cột tài chính luôn = 0 và mọi hằng số đó vô hình (mutation hoán vị
// RefundDeposit↔RefundService vẫn xanh) — đó là lý do khối này tồn tại.
func seedDimension(t *testing.T) {
	t.Helper()
	testDB.Exec(`DELETE FROM appointment`)
	testDB.Exec(`DELETE FROM payment`)
	testDB.Exec(`DELETE FROM order_service_used`)
	testDB.Exec(`DELETE FROM appointment_type WHERE id IN (?, ?)`, dimType, dimTypeNo)
	testDB.Exec(`DELETE FROM customer WHERE id = 3341`)
	testDB.Exec(`DELETE FROM order_plan WHERE id = ?`, dimPlan)
	testDB.Exec(`DELETE FROM service WHERE id = ?`, dimSvc)
	testDB.Exec(`DELETE FROM payment_method WHERE id = ?`, dimPayM)
	testDB.Exec(`DELETE FROM source WHERE id = ?`, dimSource)
	testDB.Exec(`DELETE FROM location_province WHERE id = ?`, dimProv)

	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed %s: %v", what, err)
		}
	}
	must("source", testDB.Exec(`INSERT INTO source (id, name) VALUES (?, 'Nguon QA')`, dimSource).Error)
	must("location_province", testDB.Exec(`INSERT INTO location_province (id, name) VALUES (?, 'Tinh QA')`, dimProv).Error)
	must("customer", testDB.Exec(`INSERT INTO customer (id, branch_id, fullname, created_day, source_id, province_id, stage_id)
		VALUES (3341, ?, 'KH', ?, ?, ?, (SELECT id FROM customer_stage WHERE is_default = 1))`, dimBranch, dimFrom+100, dimSource, dimProv).Error)
	for id, rev := range map[int]int{dimType: 1, dimTypeNo: 0} {
		must("appointment_type", testDB.Exec(`INSERT INTO appointment_type (id, name, isTypeRevenue) VALUES (?, 'LH', ?)`, id, rev).Error)
	}
	appts := []struct {
		id, typeID, status, accepted int
		apptDate                     int64
	}{
		{3351, dimType, 2, 1, dimFrom + 100},
		{3352, dimType, 3, 0, dimFrom + 200},
		{3353, dimType, 1, 0, dimFrom + 300},
		{3354, dimTypeNo, 2, 1, dimFrom + 400},
		{3355, dimType, 2, 1, dimTo + 86400*40},
	}
	for _, a := range appts {
		must("appointment", testDB.Exec(`INSERT INTO appointment
			(id, code, branch_id, customer_id, appointment_type_id, source_id,
			 appointment_date, created_at, status, is_accepted)
			VALUES (?, ?, ?, 3341, ?, ?, ?, ?, ?, ?)`,
			a.id, fmt.Sprintf("AP%d", a.id), dimBranch, a.typeID, dimSource,
			a.apptDate, a.apptDate, a.status, a.accepted).Error)
	}

	must("service", testDB.Exec(`INSERT INTO service (id, name, code)
		VALUES (?, 'DV QA', 'SVQA1')`, dimSvc).Error)
	must("order_plan", testDB.Exec(`INSERT INTO order_plan (id, code, customer_id)
		VALUES (?, 'PLQA1', 3341)`, dimPlan).Error)
	must("payment_method", testDB.Exec(`INSERT INTO payment_method (id, name) VALUES (?, 'TM QA')`, dimPayM).Error)
	must("order_service_used", testDB.Exec(`INSERT INTO order_service_used
		(id, code, customer_id, order_plan_id, service_id, branch_id, source_id, created_day, totalEnd)
		VALUES (?, 'OSUQA1', 3341, ?, ?, ?, ?, ?, 1234)`,
		dimOsu, dimPlan, dimSvc, dimBranch, dimSource, dimFrom+100).Error)

	pays := []struct {
		id, paymentFor int
		depositType    interface{}
		amount, after  string
	}{
		{3371, define.PaymentForOrder, nil, "1000.00", "0.00"},
		{3372, define.PaymentForDeposit, nil, "200.00", "500.00"},
		{3373, define.PaymentForRefundDeposit, nil, "70.00", "0.00"},
		{3374, define.PaymentForRefundService, nil, "9.00", "0.00"},
		{3375, define.PaymentForDeposit, define.PaymentTypeDepositKG, "5000.00", "5000.00"},
	}
	for _, p := range pays {
		must("payment", testDB.Exec(`INSERT INTO payment
			(id, code, branch_id, method, customer_id, serviceUsed_id,
			 payment_for, deposit_type, amount, deposit_after, payment_time, payment_of_date)
			VALUES (?, ?, ?, ?, 3341, ?, ?, ?, ?, ?, ?, ?)`,
			p.id, fmt.Sprintf("PM%d", p.id), dimBranch, dimPayM, dimOsu,
			p.paymentFor, p.depositType, p.amount, p.after, dimFrom+100, dimFrom+100).Error)
	}
}

func callDim(t *testing.T, fn func(*gin.Context)) []map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET",
		fmt.Sprintf("/?filter[created_at][gte]=%d&filter[created_at][lte]=%d", dimFrom, dimTo), nil)
	c.Set("branchIds", []uint{dimBranch})
	c.Set("userID", uint(99))

	fn(c)
	if w.Code >= 400 {
		// Lệch tham số Sprintf lộ ra ở ĐÂY: MySQL từ chối câu chứa %!d(MISSING).
		var detail string
		for _, e := range dimLogs.All() {
			detail += e.Message + " | " + fmt.Sprint(e.ContextMap()) + "\n"
		}
		t.Fatalf("phải 200, got %d %s\nlog: %s", w.Code, w.Body.String(), detail)
	}
	body := w.Body.String()
	if strings.Contains(body, "%!") {
		t.Fatalf("câu SQL bị lệch tham số fmt.Sprintf (còn %%! trong response): %s", body)
	}
	var out struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse JSON: %v — %s", err, body)
	}
	return out.Items
}

// Biến thể useOsuFinancial = FALSE (7/8 endpoint dùng đường này).
func TestDimensionStatistics_BienTheCustomerDim(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedDimension(t)
	h := &Handler{Deps: Deps{DB: testDB}}
	items := callDim(t, h.ProvinceStatistics)
	if len(items) != 1 {
		t.Fatalf("đúng 1 tỉnh có số liệu, got %d: %+v", len(items), items)
	}
	assertDimCounts(t, items[0])
}

// Biến thể useOsuFinancial = TRUE (chỉ `source-statistics`).
func TestDimensionStatistics_BienTheOsuFinancial(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedDimension(t)
	h := &Handler{Deps: Deps{DB: testDB}}
	items := callDim(t, h.SourceStatistics)
	if len(items) != 1 {
		t.Fatalf("đúng 1 nguồn có số liệu, got %d: %+v", len(items), items)
	}
	assertDimCounts(t, items[0])
}

func assertDimCounts(t *testing.T, row map[string]any) {
	t.Helper()
	for _, tc := range []struct {
		field string
		want  string
		why   string
	}{
		{"totalAppointment", "3", "3 lịch loại tính doanh thu trong dải"},
		{"totalAppointmentDen", "2", "chỉ 2 lịch status 2/3"},
		{"totalAppointmentDongY", "1", "chỉ lịch 3351 vừa status 2/3 vừa is_accepted = 1"},
		{"totalLead", "1", "1 khách tạo trong dải"},
	} {
		if fmt.Sprint(row[tc.field]) != tc.want {
			t.Fatalf("%s = %v, muốn %s — %s", tc.field, row[tc.field], tc.want, tc.why)
		}
	}
	// Cột tiền: so bằng số, không so chuỗi — decimal(20,2) có thể về JSON dưới dạng
	// "1000.00", 1000 hay 1000.0 tuỳ driver/serializer.
	for _, tc := range []struct {
		field string
		want  float64
		why   string
	}{
		{"totalOrderPlan", 1234, "osu 3363 totalEnd"},
		{"totalRevenue", 1000, "chỉ payment A (payment_for = Order)"},
		{"totalPayment", 1200, "A 1000 + B 200; E bị loại vì deposit_type = KG"},
		{"totalDeposit", 500, "deposit_after của B; E bị loại vì deposit_type = KG"},
		{"totalRefundDeposit", 70, "payment C — đổi chỗ hằng RefundDeposit/RefundService là lệch ngay"},
		{"totalRefundServiceUsed", 9, "payment D"},
	} {
		got, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(row[tc.field])), 64)
		if err != nil {
			t.Fatalf("%s = %v, không parse được số: %v", tc.field, row[tc.field], err)
		}
		if got != tc.want {
			t.Fatalf("%s = %v, muốn %v — %s", tc.field, got, tc.want, tc.why)
		}
	}
}
