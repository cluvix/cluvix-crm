//go:build integration

// Integration test bộ lọc chung dashboard CRM (port ERP epic crm-dashboard-customizable story-01/02):
// kỳ [from,to), chi nhánh trong/ngoài quyền, so kỳ trước, ngày tham chiếu, cô lập khoá cache — chạy handler
// thật trên DB dùng-một-lần (TEST_MYSQL_DSN, tên *_test/*_tmp).
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/cache"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/tz"
)

type dashFixture struct {
	branchA, branchB, foreign uint
	customerID                uint
}

// seedDash: chi nhánh A (kèm 1 khách của FreshTenant), B; `foreign` = chi nhánh có thật nhưng KHÔNG được
// gán cho user (ngoài branchIds).
func seedDash(t *testing.T) dashFixture {
	t.Helper()
	tn := testhelpers.FreshTenant(t, testDB)
	// Phễu chỉ đếm khách có lịch sử bước; khách FreshTenant (branch_id NULL ⇒ luôn trong phạm vi) phải KHÔNG
	// bị đếm — xoá dòng lịch sử khởi tạo mà fixture đã ghi.
	if err := testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", tn.CustomerID).Error; err != nil {
		t.Fatalf("seedDash: %v", err)
	}
	return dashFixture{branchA: tn.BranchID, branchB: testhelpers.CreateBranch(t, testDB),
		foreign: testhelpers.CreateBranch(t, testDB), customerID: tn.CustomerID}
}

var paySeq int

// addPayment ghi 1 khoản thu "thực thu" (payment_for = Order) tại thời điểm at.
func addPayment(t *testing.T, fx dashFixture, branch uint, at int64, amount int64) {
	t.Helper()
	paySeq++
	err := testDB.Exec(`INSERT INTO payment (code, branch_id, customer_id, method, amount, deposit_after, deposit_type,
		payment_for, payment_time, payment_of_date, created_at, updated_at, note) VALUES (?,?,?,1,?,0,0,?,?,?,?,?,'dash test')`,
		fmt.Sprintf("TD%d_%d", time.Now().UnixNano()%1e12, paySeq), branch, fx.customerID, amount,
		define.PaymentForOrder, at, at, at, at).Error
	if err != nil {
		t.Fatalf("insert payment: %v", err)
	}
	t.Cleanup(func() { testDB.Exec("DELETE FROM payment WHERE customer_id = ?", fx.customerID) })
}

// addCustomer — khách thuộc chi nhánh branch, created_day = at, đứng ở bước mặc định "Liên hệ" (có 1 dòng
// lịch sử — phễu chỉ đếm khách có lịch sử, story-07 customer-stage-pipeline).
func addCustomer(t *testing.T, branch uint, at int64) {
	t.Helper()
	b := branch
	addStagedCustomer(t, &b, at, 0, "Liên hệ")
}

func callDashWith(t *testing.T, h *Handler, fn func(*Handler) gin.HandlerFunc, query string, branchIDs []uint) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x?"+query, nil)
	c.Set("userID", uint(1))
	c.Set("branchActiveId", uint(0))
	c.Set("branchIds", branchIDs)
	fn(h)(c)
	return w
}

func callDash(t *testing.T, fn func(*Handler) gin.HandlerFunc, query string, branchIDs []uint) *httptest.ResponseRecorder {
	t.Helper()
	return callDashWith(t, New(Deps{DB: testDB}), fn, query, branchIDs)
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("code %d body %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("parse JSON: %v — %s", err, w.Body.String())
	}
}

func thucThu(t *testing.T, w *httptest.ResponseRecorder) decimal.Decimal {
	t.Helper()
	var body struct {
		ThucThu struct {
			HienTai decimal.Decimal `json:"hienTai"`
		} `json:"thucThu"`
	}
	decodeJSON(t, w, &body)
	return body.ThucThu.HienTai
}

// customerTotal — tổng khách của phễu status-customer-overview = tầng ĐẦU (mọi khách có lịch sử đều từng
// qua bước mở đầu tiên); mảng rỗng = 0.
func customerTotal(t *testing.T, w *httptest.ResponseRecorder) int64 {
	t.Helper()
	rows := decodeFunnel(t, w)
	if len(rows) == 0 {
		return 0
	}
	if rows[0].Type == define.CustomerStageTypeLost {
		t.Fatalf("tầng đầu phải là bước mở/thành công: %+v", rows)
	}
	return rows[0].Value
}

var pim = func(h *Handler) gin.HandlerFunc { return h.CrmPaymentInMonth }

func TestDashboardFilter_PaymentInMonth(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	fx := seedDash(t)
	loc := tz.LocFromContext(nil, testDB) // cùng nguồn timezone với handler
	addPayment(t, fx, fx.branchA, tz.StartOfMonth(loc, 0)+3600, 100)
	addPayment(t, fx, fx.branchB, tz.StartOfMonth(loc, 0)+3600, 30)
	addPayment(t, fx, fx.branchA, tz.StartOfMonth(loc, -1)+3600, 7)
	both := []uint{fx.branchA, fx.branchB}

	// Không tham số ⇒ như cũ: tháng này, mọi chi nhánh trong quyền.
	if got := thucThu(t, callDash(t, pim, "", both)); !got.Equal(decimal.NewFromInt(130)) {
		t.Fatalf("mặc định: muốn 130, nhận %s", got)
	}
	// Mặc định == gửi tường minh đúng tháng này (cùng toán tử BETWEEN, mốc cuối bao gồm).
	qThis := fmt.Sprintf("from=%d&to=%d", tz.StartOfMonth(loc, 0), tz.StartOfMonth(loc, 1))
	if got := thucThu(t, callDash(t, pim, qThis, both)); !got.Equal(decimal.NewFromInt(130)) {
		t.Fatalf("tháng này tường minh: muốn 130, nhận %s", got)
	}
	if got := thucThu(t, callDash(t, pim, fmt.Sprintf("branch_id=%d", fx.branchA), both)); !got.Equal(decimal.NewFromInt(100)) {
		t.Fatalf("chi nhánh A: muốn 100, nhận %s", got)
	}
	// from/to khác ⇒ số khác.
	qLast := fmt.Sprintf("from=%d&to=%d", tz.StartOfMonth(loc, -1), tz.StartOfMonth(loc, 0))
	if got := thucThu(t, callDash(t, pim, qLast, both)); !got.Equal(decimal.NewFromInt(7)) {
		t.Fatalf("tháng trước: muốn 7, nhận %s", got)
	}
	// So kỳ trước của "tháng này" = tháng trước (7) ⇒ tăng.
	var body struct {
		ThucThu struct {
			TangGiam int `json:"tangGiam"`
		} `json:"thucThu"`
	}
	decodeJSON(t, callDash(t, pim, "", both), &body)
	if body.ThucThu.TangGiam != 2 {
		t.Fatalf("so kỳ trước: muốn tangGiam=2 (tăng), nhận %d", body.ThucThu.TangGiam)
	}
}

// Mọi endpoint data còn lại (13 — payment-today đã gỡ ở 7fd7403) đều: không tham số ⇒ 200; chi nhánh ngoài quyền ⇒ 403; kỳ > 366 ngày,
// thiếu 1 vế, branch_id không phải số ⇒ 422; chi nhánh hợp lệ ⇒ 200.
func TestDashboardFilter_AllEndpointsGuard(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	fx := seedDash(t)
	both := []uint{fx.branchA, fx.branchB}
	eps := map[string]func(*Handler) gin.HandlerFunc{
		"payment-in-month":               pim,
		"quick-starts":                   func(h *Handler) gin.HandlerFunc { return h.CrmQuickStarts },
		"additional-stats":               func(h *Handler) gin.HandlerFunc { return h.CrmAdditionalStats },
		"appointment-accepted-stats":     func(h *Handler) gin.HandlerFunc { return h.CrmAppointmentAcceptedStats },
		"overview-appointment":           func(h *Handler) gin.HandlerFunc { return h.CrmOverviewAppointment },
		"report-statistics":              func(h *Handler) gin.HandlerFunc { return h.CrmReportStatistics },
		"status-customer-overview":       func(h *Handler) gin.HandlerFunc { return h.CrmStatusCustomerOverview },
		"report-customer-by-region":      func(h *Handler) gin.HandlerFunc { return h.CrmReportCustomerByRegion },
		"payment-branch-overview":        func(h *Handler) gin.HandlerFunc { return h.CrmPaymentBranchOverview },
		"overview-service-type-accepted": func(h *Handler) gin.HandlerFunc { return h.CrmOverviewServiceTypeAccepted },
		"overview-chart-source":          func(h *Handler) gin.HandlerFunc { return h.CrmOverviewChartSource },
		"overview-service-type-interest": func(h *Handler) gin.HandlerFunc { return h.CrmOverviewServiceTypeInterest },
		"service-used-statistics":        func(h *Handler) gin.HandlerFunc { return h.CrmServiceUsedStatistics },
	}
	if len(eps) != 13 {
		t.Fatalf("phải đủ 13 endpoint, có %d", len(eps))
	}
	for name, fn := range eps {
		if w := callDash(t, fn, "", both); w.Code != http.StatusOK {
			t.Errorf("%s: không tham số muốn 200, nhận %d %s", name, w.Code, w.Body.String())
		}
		if w := callDash(t, fn, fmt.Sprintf("branch_id=%d", fx.foreign), both); w.Code != http.StatusForbidden {
			t.Errorf("%s: chi nhánh ngoài quyền muốn 403, nhận %d", name, w.Code)
		}
		for _, bad := range []string{"from=1700000000&to=1731708800", "from=1700000000", "from=10&to=5", "branch_id=5abc"} {
			if w := callDash(t, fn, bad, both); w.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s: %q muốn 422, nhận %d", name, bad, w.Code)
			}
		}
		if w := callDash(t, fn, fmt.Sprintf("from=1700000000&to=1731622400&branch_id=%d", fx.branchB), both); w.Code != http.StatusOK {
			t.Errorf("%s: kỳ đúng 366 ngày + chi nhánh hợp lệ muốn 200, nhận %d %s", name, w.Code, w.Body.String())
		}
	}
}

// Redis thật (miniredis): cùng user gọi lần lượt các kỳ/chi nhánh khác nhau — phạm vi sau KHÔNG được nhận
// kết quả đã cache của phạm vi trước (khoá gồm from/to/refDay/branch).
func TestDashboardFilter_CacheIsolation(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	fx := seedDash(t)
	loc := tz.LocFromContext(nil, testDB)
	som, lastM := tz.StartOfMonth(loc, 0), tz.StartOfMonth(loc, -1)
	sod := tz.StartOfDay(loc)
	addPayment(t, fx, fx.branchA, som+3600, 100)
	addPayment(t, fx, fx.branchB, som+3600, 30)
	addPayment(t, fx, fx.branchA, lastM+3600, 7)
	addPayment(t, fx, fx.branchA, sod+10, 4)
	addPayment(t, fx, fx.branchB, sod+20, 6)
	h := New(Deps{DB: testDB, Cache: cache.New(miniredis.RunT(t).Addr(), "", "t", 0)})
	both := []uint{fx.branchA, fx.branchB}
	qA, qB := fmt.Sprintf("branch_id=%d", fx.branchA), fmt.Sprintf("branch_id=%d", fx.branchB)
	qLast := fmt.Sprintf("from=%d&to=%d", lastM, som)

	thisM := decimal.NewFromInt(100 + 30 + 4 + 6)
	for _, step := range []struct {
		q    string
		want decimal.Decimal
	}{{"", thisM}, {qA, decimal.NewFromInt(104)}, {qB, decimal.NewFromInt(36)}, {qLast, decimal.NewFromInt(7)}, {"", thisM}} {
		if got := thucThu(t, callDashWith(t, h, pim, step.q, both)); !got.Equal(step.want) {
			t.Fatalf("payment-in-month q=%q: muốn %s, nhận %s (dính cache phạm vi khác?)", step.q, step.want, got)
		}
	}

	// Khối đếm khách theo kỳ: tháng trước chi nhánh A (2 khách) vs tháng trước chi nhánh B (1) vs tháng này A.
	addCustomer(t, fx.branchA, lastM+3600)
	addCustomer(t, fx.branchA, lastM+7200)
	addCustomer(t, fx.branchB, lastM+3600)
	addCustomer(t, fx.branchA, som+3600)
	sco := func(h *Handler) gin.HandlerFunc { return h.CrmStatusCustomerOverview }
	for _, step := range []struct {
		q    string
		want int64
	}{{qLast + "&" + qA, 2}, {qLast + "&" + qB, 1}, {qLast, 3}, {qLast + "&" + qA, 2}} {
		if got := customerTotal(t, callDashWith(t, h, sco, step.q, both)); got != step.want {
			t.Fatalf("status-customer-overview q=%q: muốn %d, nhận %d", step.q, step.want, got)
		}
	}
}
