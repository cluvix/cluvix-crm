//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/helpers"
)

// Đối chiếu old == new == tính tay cho bản vá hiệu năng 2026-10-19 của
//   - GET /crm/report/service-type-statistics (gom theo service_type_id trong bảng dẫn xuất + ROLLUP)
//   - GET /crm/order-service-used/report-service-used (index hint theo dải ngày, JOIN customer có điều kiện,
//     gộp COUNT(DISTINCT service_id) vào câu totals) + đường export của nó.
// Bản cũ: osu_reports_legacy_integration_test.go (chép nguyên văn HEAD).

type osuParityFix struct {
	b1, b2, b3 uint
}

// seedOsuParity dựng phân bố cố ý (tiền mỗi nhóm khác nhau ⇒ thứ tự ORDER BY tổng tiền xác định):
//
//	loại A = {s1, s2}, loại B = {s3}, loại C = {s5}, s4 KHÔNG có loại
//	khách k1,k2 nguồn 7 · k3 nguồn 8 · k4 không nguồn
//	dòng nhiễu: ngoài dải ngày, isLock=2, isLock=3, chi nhánh ngoài phạm vi, dịch vụ không loại
//	dòng branch_id NULL (luôn thấy được), service_discount_type=2 (CASE tính %)
const (
	opDayIn  = int64(1788195700)
	opFrom   = int64(1788195600)
	opTo     = opFrom + 20*86400
	opDayOut = opDayIn - 40*86400
)

func seedOsuParity(t *testing.T) osuParityFix {
	t.Helper()
	for _, q := range []string{
		`DELETE FROM order_service_used`, `DELETE FROM service`, `DELETE FROM service_type`, `DELETE FROM customer`,
	} {
		if err := testDB.Exec(q).Error; err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	f := osuParityFix{
		b1: testhelpers.CreateBranch(t, testDB),
		b2: testhelpers.CreateBranch(t, testDB),
		b3: testhelpers.CreateBranch(t, testDB),
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if err := testDB.Exec(q, args...).Error; err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO service_type (id, name) VALUES (9101, 'Loại A'), (9102, 'Loại B'), (9103, 'Loại C')`)
	exec(`INSERT INTO service (id, name, code, service_type_id, serviceCategory_id) VALUES
		(9201, 'Dịch vụ 1', 'PS1', 9101, 1), (9202, 'Liệu trình 2', 'PS2', 9101, 3),
		(9203, 'Dịch vụ 3', 'PS3', 9102, 1), (9204, 'Không loại', 'PS4', NULL, 1),
		(9205, 'Sản phẩm 5', 'PS5', 9103, 2)`)
	exec(`INSERT INTO customer (id, branch_id, fullname, code, phone, source_id, totalDeposit) VALUES
		(9301, ?, 'Nguyễn Văn A', 'KH9301', '0900000001', 7, 100),
		(9302, ?, 'Trần Thị B',  'KH9302', '0900000002', 7, 20),
		(9303, ?, 'Lê Văn C',    'KH9303', '0900000003', 8, 3),
		(9304, ?, 'Phạm D',      'KH9304', '0900000004', NULL, 0)`, f.b1, f.b2, f.b1, f.b1)

	var b1, b2, b3 any = f.b1, f.b2, f.b3
	rows := []struct {
		svc, cust int
		branch    any
		isLock    int
		day       int64
		qty       int
		unit      int
		sdt, sd   int
		oth       int
		rank, ref int
		end, pay  int
	}{
		{9201, 9301, b1, 1, opDayIn, 1, 1000, 1, 100, 10, 5, 0, 835, 835},
		{9201, 9302, b1, 1, opDayIn + 3600, 2, 500, 2, 10, 0, 0, 0, 900, 400},
		{9202, 9301, b2, 1, opDayIn + 7200, 1, 300, 1, 0, 0, 0, 100, 200, 0},
		{9203, 9303, nil, 1, opDayIn + 86400, 3, 100, 2, 5, 0, 0, 0, 285, 285},
		{9203, 9301, b1, 1, opDayOut, 1, 700, 1, 0, 0, 0, 0, 700, 100}, // ngoài dải tháng
		{9204, 9302, b1, 1, opDayIn, 1, 50, 1, 0, 0, 0, 0, 50, 0},      // dịch vụ không loại
		{9201, 9303, b1, 2, opDayIn, 1, 999, 1, 0, 0, 0, 0, 999, 999},  // isLock=2
		{9202, 9304, b1, 3, opDayIn, 1, 400, 1, 0, 0, 0, 0, 400, 100},  // isLock=3
		{9201, 9304, b3, 1, opDayIn, 1, 111, 1, 0, 0, 0, 0, 111, 0},    // chi nhánh ngoài phạm vi
		{9205, 9302, b2, 1, opDayIn, 1, 80, 1, 0, 0, 0, 0, 80, 80},
		{9202, 9302, b2, 1, opDayIn + 60, 4, 250, 2, 20, 0, 0, 0, 800, 300},
		{9203, 9302, nil, 1, opDayOut + 60, 2, 10, 1, 0, 0, 0, 0, 20, 5}, // NULL branch ngoài dải tháng
	}
	for i, r := range rows {
		exec(`INSERT INTO order_service_used
			(code, customer_id, order_plan_id, service_id, branch_id, isLock, created_day,
			 quantity, unit_price, service_discount_type, service_discount, discount_other,
			 rank_discount_amount, refundServiceUsed, totalEnd, payment)
			VALUES (?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("OP%d", 9400+i), r.cust, r.svc, r.branch, r.isLock, r.day,
			r.qty, r.unit, r.sdt, r.sd, r.oth, r.rank, r.ref, r.end, r.pay)
	}
	return f
}

// callReport gọi 1 handler với query + phiên (branchActive, branchIds), trả body đã decode.
func callReport(t *testing.T, fn func(*gin.Context), rawQuery string, active uint, ids []uint) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x?"+rawQuery, nil)
	c.Set("branchActiveId", active)
	c.Set("branchIds", ids)
	c.Set("userID", uint(99))
	fn(c)
	if w.Code != 200 {
		t.Fatalf("%s: phải 200, got %d %s", rawQuery, w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse JSON: %v — %s", err, w.Body.String())
	}
	return body
}

func month() string {
	return fmt.Sprintf("filter[created_day][gte]=%d&filter[created_day][lte]=%d", opFrom, opTo)
}
func year() string {
	return fmt.Sprintf("filter[created_day][gte]=%d&filter[created_day][lte]=%d", opFrom-300*86400, opTo)
}

type scopeCase struct {
	name   string
	active uint
	ids    []uint
}

func parityScopes(f osuParityFix) []scopeCase {
	return []scopeCase{
		{"toàn chi nhánh được cấp", 0, []uint{f.b1, f.b2}},
		{"1 chi nhánh", f.b1, []uint{f.b1, f.b2}},
	}
}

func requireDec(t *testing.T, what string, got any, want string) {
	t.Helper()
	g, err := decimal.NewFromString(fmt.Sprint(got))
	if err != nil || !g.Equal(decimal.RequireFromString(want)) {
		t.Fatalf("%s: want %s, got %v", what, want, got)
	}
}

func TestServiceTypeStats_ParityVaTinhTay(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	f := seedOsuParity(t)
	h := &Handler{Deps: Deps{DB: testDB}}

	queries := []string{month(), year(), "", "created_day[gte]=" + fmt.Sprint(opFrom) + "&created_day[lte]=" + fmt.Sprint(opTo),
		"filter[created_day][gte]=" + fmt.Sprint(opFrom), month() + "&page=2&per-page=1", "filter[created_day][gte]=1&filter[created_day][lte]=2"}
	for _, sc := range parityScopes(f) {
		for _, q := range queries {
			oldB := callReport(t, h.legacyServiceTypeStatistics, q, sc.active, sc.ids)
			newB := callReport(t, h.ServiceTypeStatistics, q, sc.active, sc.ids)
			if !reflect.DeepEqual(oldB, newB) {
				t.Fatalf("[%s] %q: old != new\nold: %v\nnew: %v", sc.name, q, oldB, newB)
			}
		}
	}

	// Tính tay — tháng, toàn chi nhánh được cấp (b1, b2 + NULL):
	// A = dòng 1,2,3,11 · B = dòng 4 (NULL branch) · C = dòng 10.
	b := callReport(t, h.ServiceTypeStatistics, month(), 0, []uint{f.b1, f.b2})
	items := b["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("phải 3 loại, got %d: %v", len(items), items)
	}
	want := []struct {
		id, cust, num, qty  int
		end, pay, sDiscount string
	}{
		// sdt=2: 10% × 500 × 2 = 100 · 20% × 250 × 4 = 200 · sdt=1: 100 + 0 ⇒ A = 400
		{9101, 2, 4, 8, "2735", "1535", "400"},
		{9102, 1, 1, 3, "285", "285", "15"}, // 5% × 100 × 3
		{9103, 1, 1, 1, "80", "80", "0"},
	}
	for i, w := range want {
		it := items[i].(map[string]any)
		// decimal ra JSON là chuỗi trong test (MarshalJSONWithoutQuotes chỉ bật ở main.go) ⇒ so qua decimal.
		requireDec(t, "id", it["id"], fmt.Sprint(w.id))
		requireDec(t, "customerCount", it["customerCount"], fmt.Sprint(w.cust))
		requireDec(t, "numService", it["numService"], fmt.Sprint(w.num))
		requireDec(t, "quantity", it["quantity"], fmt.Sprint(w.qty))
		requireDec(t, "totalEnd", it["totalEnd"], w.end)
		requireDec(t, "payment", it["payment"], w.pay)
		requireDec(t, "service_discount_amount", it["service_discount_amount"], w.sDiscount)
	}
	totals := b["totals"].(map[string]any)
	// k1, k2, k3 — KHÔNG phải 2+1+1 = 4 (khách k2 mua cả loại A lẫn C).
	if got := totals["customerCount"]; got != float64(3) {
		t.Fatalf("totals.customerCount phải là khách PHÂN BIỆT (3), got %v", got)
	}
	requireDec(t, "totals.totalEnd", totals["totalEnd"], "3100")
	if meta := b["_meta"].(map[string]any); meta["totalCount"] != float64(3) {
		t.Fatalf("_meta.totalCount phải 3, got %v", meta)
	}

	// Không có dòng khớp ⇒ ROLLUP không sinh dòng nào ⇒ tổng khách 0, items rỗng (cũ cũng vậy).
	b = callReport(t, h.ServiceTypeStatistics, "filter[created_day][gte]=1&filter[created_day][lte]=2", 0, []uint{f.b1})
	if len(b["items"].([]any)) != 0 || b["totals"].(map[string]any)["customerCount"] != float64(0) {
		t.Fatalf("dải rỗng: want items=[] customerCount=0, got %v", b)
	}
}

// sortItemsByID — report-service-used xếp theo COUNT(id) DESC: nhóm bằng số dòng thì thứ tự không xác
// định ở CẢ hai bản ⇒ so tập nhóm (sort theo id) + kiểm riêng thứ tự numService không tăng.
func sortItemsByID(t *testing.T, b map[string]any) []any {
	t.Helper()
	obj := b["object"].(map[string]any)
	items := append([]any(nil), obj["items"].([]any)...)
	for i := 1; i < len(items); i++ {
		if items[i].(map[string]any)["numService"].(float64) > items[i-1].(map[string]any)["numService"].(float64) {
			t.Fatalf("items phải xếp numService giảm dần: %v", items)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].(map[string]any)["id"].(float64) < items[j].(map[string]any)["id"].(float64)
	})
	return items
}

func TestReportServiceUsed_ParityVaTinhTay(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	// Như production (bootstrap/app.go): bản đồ cột decimal phải nạp thì Paginate (đường cũ) mới parse tiền —
	// thiếu nó bản cũ ra "50.00" còn bản mới (parse tường minh) ra 50, so sánh lệch giả.
	helpers.InitDecimalColumnMap(testDB)
	f := seedOsuParity(t)
	h := &Handler{Deps: Deps{DB: testDB}}

	queries := []string{
		month(), year(),
		"created_day[gte]=" + fmt.Sprint(opFrom) + "&created_day[lte]=" + fmt.Sprint(opTo),
		month() + "&_keyword=Dịch",
		month() + "&_keyword=khongkhop",
		month() + "&filter[source_id][0]=7",
		year() + "&filter[source_id]=8",
		month() + "&filter[source_id][is]=1",
		month() + fmt.Sprintf("&filter[branch_id][0]=%d", f.b2),
		month() + "&filter[service_type_id][0]=9101",
		year() + "&filter[sourceService_id][0]=1",
		month() + "&page=2&per-page=2",
		month() + "&page=9",
		"filter[created_day][gte]=1&filter[created_day][lte]=2", // dải rỗng: ROLLUP không sinh dòng tổng
		month() + "&_groupLocal[0]=1",                           // đường 2 câu cũ (totals không áp _group*)
		year() + "&_groupCountry[0]=1&_keyword=Dịch",
	}
	for _, sc := range parityScopes(f) {
		for _, q := range queries {
			oldB := callReport(t, h.legacyReportServiceUsed, q, sc.active, sc.ids)
			newB := callReport(t, h.ReportServiceUsed, q, sc.active, sc.ids)
			if !reflect.DeepEqual(oldB["totals"], newB["totals"]) {
				t.Fatalf("[%s] %q: totals old != new\nold: %v\nnew: %v", sc.name, q, oldB["totals"], newB["totals"])
			}
			oldObj, newObj := oldB["object"].(map[string]any), newB["object"].(map[string]any)
			if !reflect.DeepEqual(oldObj["_meta"], newObj["_meta"]) {
				t.Fatalf("[%s] %q: _meta old != new\nold: %v\nnew: %v", sc.name, q, oldObj["_meta"], newObj["_meta"])
			}
			// Trang 2 trở đi phụ thuộc thứ tự nhóm hoà — chỉ so tập khi lấy trọn trong 1 trang.
			if oldObj["_meta"].(map[string]any)["pageCount"].(float64) <= 1 {
				if o, n := sortItemsByID(t, oldB), sortItemsByID(t, newB); !reflect.DeepEqual(o, n) {
					t.Fatalf("[%s] %q: items old != new\nold: %v\nnew: %v", sc.name, q, o, n)
				}
			}
		}
	}

	// Tính tay — tháng, toàn chi nhánh được cấp: dòng 1,2,3,4,6,10,11 ⇒ 5 dịch vụ, 7 dòng, khách k1,k2,k3.
	b := callReport(t, h.ReportServiceUsed, month(), 0, []uint{f.b1, f.b2})
	tot := b["totals"].(map[string]any)
	requireDec(t, "numService", tot["numService"], "7")
	requireDec(t, "customerCount", tot["customerCount"], "3")
	requireDec(t, "numQuantity", tot["numQuantity"], "13")
	requireDec(t, "totalTotalEnd", tot["totalTotalEnd"], "3150")
	requireDec(t, "totalDiscountService", tot["totalDiscountService"], "415")
	if meta := b["object"].(map[string]any)["_meta"].(map[string]any); meta["totalCount"] != float64(5) {
		t.Fatalf("_meta.totalCount phải = 5 dịch vụ, got %v", meta)
	}
	// Lọc nguồn 7 (k1, k2) qua JOIN customer: dòng 1,2,3,6,10,11 ⇒ 6 dòng.
	b = callReport(t, h.ReportServiceUsed, month()+"&filter[source_id][0]=7", 0, []uint{f.b1, f.b2})
	if got := b["totals"].(map[string]any)["numService"]; got != float64(6) {
		t.Fatalf("lọc source_id=7: numService want 6, got %v", got)
	}
}

// Export (`?_export=1`) chạy `ExportTreatmentServices(c, q)` trên CHÍNH builder danh sách ⇒ so tập dòng
// của câu export dựng trên builder cũ vs mới (cùng Select/Group như export/service_used.go).
func TestReportServiceUsed_ExportQueryParity(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	f := seedOsuParity(t)
	h := &Handler{Deps: Deps{DB: testDB}}
	exportRows := func(q *gorm.DB) []map[string]any {
		var rows []map[string]any
		tx := q.Session(&gorm.Session{}).
			Select(`order_service_used.service_id as id,
				service.name as service_name, service.code as service_code,
				service_type.name as service_type_name,
				COUNT(DISTINCT order_service_used.customer_id) AS customerCount,
				COUNT(order_service_used.id) as numService,
				SUM(quantity) as quantity, SUM(totalEnd) as totalEnd, SUM(payment) as payment,
				SUM(CASE WHEN service_discount_type = 2 THEN (service_discount * unit_price * quantity / 100) ELSE service_discount END) as service_discount_amount,
				SUM(discount_other) as discount_other,
				SUM(rank_discount_amount) as rank_discount_amount, SUM(refundServiceUsed) as refundServiceUsed`).
			Group("order_service_used.service_id").Find(&rows)
		if tx.Error != nil {
			t.Fatalf("export query: %v", tx.Error)
		}
		sort.Slice(rows, func(i, j int) bool { return fmt.Sprint(rows[i]["id"]) < fmt.Sprint(rows[j]["id"]) })
		return rows
	}
	for _, q := range []string{month(), year(), month() + "&filter[source_id][0]=7", month() + "&_groupLocal[0]=1"} {
		c := testhelpers.TenantCtx(0, 99)
		c.Set("branchIds", []uint{f.b1, f.b2})
		c.Request = httptest.NewRequest("GET", "/x?"+q, nil)
		o, n := exportRows(h.legacyBuildReportServiceUsedQuery(c)), exportRows(h.buildReportServiceUsedQuery(c))
		if len(o) == 0 && q == month() {
			t.Fatalf("%q: export phải có dòng — phép so rỗng = xanh giả", q)
		}
		if !reflect.DeepEqual(o, n) {
			t.Fatalf("%q: export old != new\nold: %v\nnew: %v", q, o, n)
		}
	}
}
