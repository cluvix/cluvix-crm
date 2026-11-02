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

// Neo các báo cáo DOANH THU của CRM (/crm/report/revenue/*): con số theo ngày gộp đúng 3 nhánh
// UNION (lịch hẹn đồng ý / doanh số đơn / phiếu thu), phạm vi chi nhánh của phiên được tôn trọng
// (branch là trục scope duy nhất sau D15), guard dải ngày trả 422, và FORCE INDEX trỏ đúng chỉ mục
// đang có trong schema (nhánh toàn chi nhánh — tên chỉ mục sai là lỗi SQL ngay).
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
	revB1   = 31
	revB2   = 32
	revT    = int64(1788195700)
	revFrom = revT - 3600
	revTo   = revT + 86400
)

func mustExec(t *testing.T, sql string, args ...interface{}) {
	t.Helper()
	if err := testDB.Exec(sql, args...).Error; err != nil {
		t.Fatalf("seed: %v\n%s", err, sql)
	}
}

func insertPayment(t *testing.T, id, customer, branch, paymentFor, depositType int, amount, depositAfter string, at int64, depositID interface{}) {
	t.Helper()
	mustExec(t, `INSERT INTO payment
		(id, code, customer_id, branch_id, method, amount, deposit_after, deposit_type, payment_for,
		 payment_time, created_at, updated_at, deposit_id, note)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, 'ghi chú')`,
		id, fmt.Sprintf("PT%d", id), customer, branch, amount, depositAfter, depositType, paymentFor,
		at, at, at, depositID)
}

// seedRevenue — 2 chi nhánh, mọi sự kiện trong dải [revFrom, revTo] trừ 1 phiếu ngoài dải.
//
//	B1: khách 901 — 1 lịch đồng ý; đơn 801 (osu 1.000.000 + 500.000);
//	    phiếu: thanh toán 300.000 · cọc 200.000 (deposit_after 200.000) · hoàn cọc 50.000 (từ cọc 9003)
//	    · hoàn dịch vụ 20.000 · cọc ký gửi KG 400.000 (bị loại khỏi thực thu/đặt cọc)
//	    · thanh toán 777.000 NGOÀI dải ngày
//	B2: khách 902 — 1 lịch đồng ý; đơn 802 (osu 700.000); phiếu thanh toán 999 + hoàn cọc 1.000
func seedRevenue(t *testing.T) {
	t.Helper()
	for _, tbl := range []string{"payment", "order_service_used", "order_plan", "appointment", "appointment_type", "customer", "branch"} {
		mustExec(t, "DELETE FROM `"+tbl+"`")
	}
	mustExec(t, `INSERT INTO branch (id, name) VALUES (?, 'CN1'), (?, 'CN2')`, revB1, revB2)
	mustExec(t, `INSERT INTO appointment_type (id, name, isTypeRevenue) VALUES (1, 'Doanh thu', 1)`)
	mustExec(t, `INSERT INTO customer (id, code, branch_id, fullname, phone, stage_id) VALUES
		(901, 'KH901', ?, 'Khách Một', '0900000901', (SELECT id FROM customer_stage WHERE is_default = 1)), (902, 'KH902', ?, 'Khách Hai', '0900000902', (SELECT id FROM customer_stage WHERE is_default = 1))`, revB1, revB2)
	mustExec(t, `INSERT INTO appointment (id, code, branch_id, customer_id, appointment_type_id, appointment_date, status, is_accepted)
		VALUES (701, 'AP701', ?, 901, 1, ?, 2, 1), (702, 'AP702', ?, 902, 1, ?, 3, 1)`, revB1, revT, revB2, revT)
	mustExec(t, `INSERT INTO order_plan (id, code, branch_id, customer_id, created_time) VALUES
		(801, 'DH801', ?, 901, ?), (802, 'DH802', ?, 902, ?)`, revB1, revT, revB2, revT)
	mustExec(t, `INSERT INTO order_service_used (id, code, branch_id, customer_id, order_plan_id, service_id, totalEnd) VALUES
		(8011, 'DV8011', ?, 901, 801, 1, 1000000), (8012, 'DV8012', ?, 901, 801, 1, 500000),
		(8021, 'DV8021', ?, 902, 802, 1, 700000)`, revB1, revB1, revB2)

	insertPayment(t, 9001, 901, revB1, define.PaymentForOrder, 0, "300000", "0", revT, nil)
	insertPayment(t, 9003, 901, revB1, define.PaymentForDeposit, 0, "200000", "200000", revT, nil)
	insertPayment(t, 9004, 901, revB1, define.PaymentForRefundDeposit, 0, "50000", "0", revT, 9003)
	insertPayment(t, 9005, 901, revB1, define.PaymentForRefundService, 0, "20000", "0", revT, nil)
	insertPayment(t, 9006, 901, revB1, define.PaymentForDeposit, define.PaymentTypeDepositKG, "400000", "400000", revT, nil)
	insertPayment(t, 9007, 901, revB1, define.PaymentForOrder, 0, "777000", "0", revT+10*86400, nil)
	insertPayment(t, 9101, 902, revB2, define.PaymentForOrder, 0, "999", "0", revT, nil)
	insertPayment(t, 9102, 902, revB2, define.PaymentForRefundDeposit, 0, "1000", "0", revT, nil)
}

// call chạy handler với branchActive (0 = phiên toàn chi nhánh, scope theo branchIds).
func call(t *testing.T, fn func(*Handler, *gin.Context), rawQuery string, branchActive uint) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x?"+rawQuery, nil)
	if branchActive > 0 {
		c.Set("branchActiveId", branchActive)
	}
	c.Set("branchIds", []uint{revB1, revB2})
	c.Set("userID", uint(99))
	fn(&Handler{Deps: Deps{DB: testDB}}, c)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, dst interface{}) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("phải 200, got %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), dst); err != nil {
		t.Fatalf("parse JSON: %v — %s", err, w.Body.String())
	}
}

func eqDec(t *testing.T, name string, got json.Number, want string) {
	t.Helper()
	g, err := decimal.NewFromString(got.String())
	if err != nil {
		t.Fatalf("%s: không phải số: %q", name, got)
	}
	if !g.Equal(decimal.RequireFromString(want)) {
		t.Errorf("%s = %s, muốn %s", name, g, want)
	}
}

func byDateQS() string {
	return fmt.Sprintf("_ngay_thang[gte]=%d&_ngay_thang[lte]=%d", revFrom, revTo)
}

func TestPaymentByDate_SoLieuVaPhamViChiNhanh(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedRevenue(t)

	type summary struct {
		NumAppOk           json.Number `json:"numAppOk"`
		TotalDoanhSo       json.Number `json:"totalDoanhSo"`
		TotalThucThu       json.Number `json:"totalThucThu"`
		TotalDeposit       json.Number `json:"totalDeposit"`
		TotalRefundDepoit  json.Number `json:"totalRefundDepoit"`
		TotalRefundService json.Number `json:"totalRefundService"`
	}
	type body struct {
		Items   []map[string]json.Number `json:"items"`
		Summary summary                  `json:"summary"`
	}
	cases := []struct {
		name                                             string
		branchActive                                     uint
		numApp, doanhSo, thucThu, deposit, rfDep, rfServ string
	}{
		// Toàn chi nhánh ⇒ nhánh FORCE INDEX (tên chỉ mục phải tồn tại trong schema).
		{"toàn chi nhánh", 0, "2", "2200000", "500999", "200000", "51000", "20000"},
		// Phiên ở CN1 ⇒ mọi nhánh UNION chỉ lấy CN1 (+ NULL).
		{"phiên ở CN1", revB1, "1", "1500000", "500000", "200000", "50000", "20000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b body
			decodeJSON(t, call(t, (*Handler).ReportPaymentByDate, byDateQS(), tc.branchActive), &b)
			if len(b.Items) != 1 {
				t.Fatalf("mọi sự kiện cùng 1 ngày ⇒ phải đúng 1 dòng, got %d: %+v", len(b.Items), b.Items)
			}
			s := b.Summary
			eqDec(t, "numAppOk", s.NumAppOk, tc.numApp)
			eqDec(t, "totalDoanhSo", s.TotalDoanhSo, tc.doanhSo)
			eqDec(t, "totalThucThu", s.TotalThucThu, tc.thucThu)
			eqDec(t, "totalDeposit", s.TotalDeposit, tc.deposit)
			eqDec(t, "totalRefundDepoit", s.TotalRefundDepoit, tc.rfDep)
			eqDec(t, "totalRefundService", s.TotalRefundService, tc.rfServ)
			// Dòng ngày phải khớp dòng tổng (1 ngày duy nhất) và ra JSON number (đã parse decimal).
			eqDec(t, "items[0].totalThucThu", b.Items[0]["totalThucThu"], tc.thucThu)
		})
	}

	t.Run("lọc branch_id=CN2 trong phiên toàn chi nhánh", func(t *testing.T) {
		var b body
		decodeJSON(t, call(t, (*Handler).ReportPaymentByDate, byDateQS()+fmt.Sprintf("&filter[branch_id][0]=%d", revB2), 0), &b)
		eqDec(t, "totalThucThu", b.Summary.TotalThucThu, "999")
		eqDec(t, "totalDoanhSo", b.Summary.TotalDoanhSo, "700000")
	})
}

// M-19 (user chốt 2026-10-18): cột "Đặt cọc" = tiền cọc NHẬN trong kỳ (amount), không phải số dư
// deposit_after. Cọc 9003 (200.000) bị dùng hết (deposit_after về 0) ⇒ ngày đã qua KHÔNG được đổi số.
func TestPaymentByDate_DatCocLaTienNhanKhongPhaiSoDu(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedRevenue(t)
	mustExec(t, `UPDATE payment SET deposit_after = 0 WHERE id = 9003`)

	var b struct {
		Items   []map[string]json.Number `json:"items"`
		Summary struct {
			TotalDeposit json.Number `json:"totalDeposit"`
		} `json:"summary"`
	}
	decodeJSON(t, call(t, (*Handler).ReportPaymentByDate, byDateQS(), 0), &b)
	eqDec(t, "summary.totalDeposit", b.Summary.TotalDeposit, "200000")
	if len(b.Items) != 1 {
		t.Fatalf("phải đúng 1 dòng ngày, got %d", len(b.Items))
	}
	eqDec(t, "items[0].totalDeposit", b.Items[0]["totalDeposit"], "200000")
}

func TestRevenueReports_GuardDaiNgay(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	over := fmt.Sprintf("%d", revFrom+define.ReportMaxRangeSec+1)
	cases := []struct {
		name string
		fn   func(*Handler, *gin.Context)
		qs   string
	}{
		{"by-date thiếu ngày", (*Handler).ReportPaymentByDate, ""},
		{"by-date vượt trần", (*Handler).ReportPaymentByDate, fmt.Sprintf("_ngay_thang[gte]=%d&_ngay_thang[lte]=%s", revFrom, over)},
		{"by-date export thiếu ngày", (*Handler).ReportPaymentByDateExport, ""},
		{"by-method thiếu ngày", (*Handler).ReportPaymentByMethod, ""},
		{"branch-receipts vượt trần", (*Handler).ListBranchReceipts, fmt.Sprintf("filter[payment_time][gte]=%d&filter[payment_time][lte]=%s", revFrom, over)},
		{"branch-receipts export thiếu ngày", (*Handler).ListBranchReceipts, "_export=1"},
		{"refund thiếu ngày", (*Handler).ListRefundDepositService, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if w := call(t, tc.fn, tc.qs, 0); w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("phải 422, got %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func payTimeQS() string {
	return fmt.Sprintf("filter[payment_time][gte]=%d&filter[payment_time][lte]=%d", revFrom, revTo)
}

func TestBranchReceipts_TongVaTrang(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedRevenue(t)
	type body struct {
		Object struct {
			Items []map[string]interface{} `json:"items"`
			Meta  struct {
				TotalCount int64 `json:"totalCount"`
			} `json:"_meta"`
		} `json:"object"`
		TotalSum json.Number `json:"totalSum"`
	}

	// Toàn chi nhánh ⇒ câu tổng có FORCE INDEX. Trong dải: CN1 5 phiếu (trừ KG) + CN2 2 phiếu.
	var all body
	decodeJSON(t, call(t, (*Handler).ListBranchReceipts, payTimeQS(), 0), &all)
	if all.Object.Meta.TotalCount != 6 {
		t.Fatalf("totalCount = %d, muốn 6 (KG + ngoài dải bị loại)", all.Object.Meta.TotalCount)
	}
	eqDec(t, "totalSum toàn CN", all.TotalSum, "571999") // 300000+200000+50000+20000+999+1000

	var b1 body
	decodeJSON(t, call(t, (*Handler).ListBranchReceipts, payTimeQS()+"&per-page=2", revB1), &b1)
	if b1.Object.Meta.TotalCount != 4 || len(b1.Object.Items) != 2 {
		t.Fatalf("CN1: total=%d items=%d, muốn 4/2", b1.Object.Meta.TotalCount, len(b1.Object.Items))
	}
	eqDec(t, "totalSum CN1", b1.TotalSum, "570000")
	cust, _ := b1.Object.Items[0]["customer"].(map[string]interface{})
	if cust["code"] != "KH901" {
		t.Fatalf("customer lồng phải có code KH901, got %+v", b1.Object.Items[0])
	}

	// Từ khoá theo mã khách ⇒ JOIN customer chỉ ở đường có từ khoá.
	var kw body
	decodeJSON(t, call(t, (*Handler).ListBranchReceipts, payTimeQS()+"&_keyword=KH902", 0), &kw)
	if kw.Object.Meta.TotalCount != 2 {
		t.Fatalf("từ khoá KH902: total=%d, muốn 2", kw.Object.Meta.TotalCount)
	}
}

func TestRefundDepositService_ChiHoanTien(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedRevenue(t)
	type body struct {
		Object struct {
			Items []map[string]interface{} `json:"items"`
		} `json:"object"`
		Total json.Number `json:"totalRefundDeposit"`
	}
	var b body
	decodeJSON(t, call(t, (*Handler).ListRefundDepositService, payTimeQS(), 0), &b)
	if len(b.Object.Items) != 3 {
		t.Fatalf("phải 3 phiếu hoàn (2 CN1 + 1 CN2), got %d", len(b.Object.Items))
	}
	eqDec(t, "totalRefundDeposit", b.Total, "71000")
	for _, it := range b.Object.Items {
		if fmt.Sprint(it["id"]) == "9004" {
			dep, _ := it["deposit"].(map[string]interface{})
			if dep["code"] != "PT9003" {
				t.Fatalf("phiếu hoàn cọc phải gắn phiếu cọc gốc PT9003, got %+v", it["deposit"])
			}
		}
	}

	var b1 body
	decodeJSON(t, call(t, (*Handler).ListRefundDepositService, payTimeQS(), revB1), &b1)
	eqDec(t, "totalRefundDeposit CN1", b1.Total, "70000")
}

func TestPaymentByMethod_PhamVi(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedRevenue(t)
	mustExec(t, `DELETE FROM payment_method WHERE id = 1`)
	mustExec(t, `INSERT INTO payment_method (id, name) VALUES (1, 'Tiền mặt')`)
	qs := fmt.Sprintf("from=%d&to=%d", revFrom, revTo)

	var rows []struct {
		MethodName string      `json:"method_name"`
		PaymentFor int         `json:"payment_for"`
		Amount     json.Number `json:"amount"`
	}
	decodeJSON(t, call(t, (*Handler).ReportPaymentByMethod, qs, revB1), &rows)
	got := map[int]string{}
	for _, r := range rows {
		if r.MethodName != "Tiền mặt" {
			t.Fatalf("method_name = %q", r.MethodName)
		}
		got[r.PaymentFor] = r.Amount.String()
	}
	// CN1 trong dải: thanh toán 300.000 · cọc 200.000 + KG 400.000 · hoàn cọc 50.000 · hoàn DV 20.000.
	want := map[int]string{
		define.PaymentForOrder: "300000", define.PaymentForDeposit: "600000",
		define.PaymentForRefundDeposit: "50000", define.PaymentForRefundService: "20000",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, muốn %v", got, want)
	}
	for k, v := range want {
		if !decimal.RequireFromString(got[k]).Equal(decimal.RequireFromString(v)) {
			t.Errorf("payment_for %d = %s, muốn %s", k, got[k], v)
		}
	}
}
