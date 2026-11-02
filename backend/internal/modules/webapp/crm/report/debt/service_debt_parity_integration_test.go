//go:build integration

package debt

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

// Đối chiếu old == new == tính tay cho bản vá hiệu năng 2026-10-19 của GET /crm/report/debt/service
// (STRAIGHT_JOIN service + FORCE INDEX idx_osu_created_day_branch có cổng). Bản cũ:
// service_debt_legacy_integration_test.go (chép nguyên văn HEAD).

const (
	sdFrom   = int64(1788195600)
	sdTo     = sdFrom + 20*86400
	sdDayIn  = sdFrom + 100
	sdDayOut = sdFrom - 40*86400
	sdTele   = uint(97701) // user phòng Telesale
)

type sdFix struct{ b1, b2, b3 uint }

// seedServiceDebt — phân bố cố ý: dòng còn nợ / đã trả đủ / payment NULL, isLock 1·2·3, danh mục
// dịch vụ 1·3 (tính) và 2 (sản phẩm — loại), chi nhánh trong/ngoài phạm vi + NULL, ngày trong/ngoài dải,
// 2 khách thuộc telesale sdTele.
func seedServiceDebt(t *testing.T) sdFix {
	t.Helper()
	exec := func(q string, args ...any) {
		t.Helper()
		require.NoError(t, testDB.Exec(q, args...).Error, q)
	}
	exec(`DELETE FROM order_service_used`)
	exec(`DELETE FROM service WHERE id BETWEEN 9501 AND 9599`)
	exec(`DELETE FROM customer WHERE id BETWEEN 9601 AND 9699`)
	f := sdFix{testhelpers.CreateBranch(t, testDB), testhelpers.CreateBranch(t, testDB), testhelpers.CreateBranch(t, testDB)}

	exec(`INSERT INTO service (id, name, code, serviceCategory_id) VALUES
		(9501, 'DV nợ 1', 'SD1', ?), (9502, 'Liệu trình nợ', 'SD2', ?), (9503, 'Sản phẩm', 'SD3', 2)`,
		define.ServiceCategoryService, define.ServiceCategoryTreatment)
	exec(`INSERT INTO customer (id, branch_id, fullname, code, phone, totalDeposit, payment_user_id, stage_id) VALUES
		(9601, ?, 'Nguyễn Văn Nợ', 'KN9601', '0911000001', 100, ?, (SELECT id FROM customer_stage WHERE is_default = 1)),
		(9602, ?, 'Trần Thị Nợ',   'KN9602', '0911000002', 20, ?, (SELECT id FROM customer_stage WHERE is_default = 1)),
		(9603, ?, 'Lê Văn Khác',   'KN9603', '0911000003', 3, NULL, (SELECT id FROM customer_stage WHERE is_default = 1))`, f.b1, sdTele, f.b2, sdTele, f.b1)

	var b1, b2, b3 any = f.b1, f.b2, f.b3
	rows := []struct {
		svc, cust int
		branch    any
		isLock    int
		day       int64
		end       int
		pay       any
	}{
		{9501, 9601, b1, 1, sdDayIn, 1000, 400},       // nợ 600
		{9502, 9602, b2, 1, sdDayIn + 60, 500, 0},     // nợ 500
		{9501, 9603, nil, 3, sdDayIn + 120, 300, 100}, // isLock 3 vẫn tính, NULL branch — nợ 200
		{9501, 9601, b1, 1, sdDayIn + 180, 700, 700},  // đã trả đủ — loại
		{9501, 9602, b1, 2, sdDayIn + 240, 900, 0},    // isLock 2 — loại
		{9503, 9601, b1, 1, sdDayIn + 300, 800, 0},    // sản phẩm — loại
		{9502, 9603, b3, 1, sdDayIn + 360, 650, 0},    // chi nhánh ngoài phạm vi — loại
		{9502, 9601, b1, 1, sdDayOut, 250, 50},        // ngoài dải tháng — nợ 200
		{9501, 9602, b2, 1, sdDayIn + 420, 120, nil},  // payment NULL ⇒ predicate NULL ⇒ loại (như ERP)
		{9502, 9603, nil, 1, sdDayOut + 600, 90, 10},  // NULL branch ngoài dải — nợ 80
	}
	for i, r := range rows {
		exec(`INSERT INTO order_service_used (code, customer_id, order_plan_id, service_id, branch_id, isLock,
			created_day, quantity, totalEnd, payment) VALUES (?, ?, 0, ?, ?, ?, ?, 1, ?, ?)`,
			fmt.Sprintf("SDV%d", 9700+i), r.cust, r.svc, r.branch, r.isLock, r.day, r.end, r.pay)
	}
	return f
}

// setTelesale bật/tắt phạm vi telesale cho sdTele (option 5 tắt + user_profile thuộc phòng Telesale).
func setTelesale(t *testing.T) {
	t.Helper()
	require.NoError(t, testDB.Exec(`INSERT INTO user_option (id, name, options) VALUES (?, 'view lead tele', 0)
		ON DUPLICATE KEY UPDATE options = 0`, define.UserOptionViewLeadOfTele).Error)
	require.NoError(t, testDB.Exec(`INSERT INTO user_profile (user_id, fullname, departmentList) VALUES (?, 'Tele', ?)
		ON DUPLICATE KEY UPDATE departmentList = VALUES(departmentList)`, sdTele, fmt.Sprintf("[%d]", define.HrDepartmentTelesale)).Error)
	t.Cleanup(func() { testDB.Exec(`DELETE FROM user_profile WHERE user_id = ?`, sdTele) })
}

func callDebt(t *testing.T, fn func(*gin.Context), rawQuery string, active, user uint, ids []uint) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x?"+rawQuery, nil)
	c.Set("branchActiveId", active)
	c.Set("branchIds", ids)
	c.Set("userID", user)
	fn(c)
	require.Equal(t, 200, w.Code, "%s: %s", rawQuery, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

// stripVatKeys — bản cũ có trước VAT (bmad/order-vat) nên không có `vat_amount` từng dòng / `vatAmount` ở tổng.
// Seed ở đây đều vat_amount = 0 ⇒ mọi con số khác phải khớp tuyệt đối; chỉ bỏ 2 khoá mới trước khi so.
func stripVatKeys(body map[string]any) map[string]any {
	if tot, ok := body["total"].(map[string]any); ok {
		delete(tot, "vatAmount")
	}
	if items, ok := body["items"].([]any); ok {
		for _, it := range items {
			if row, ok := it.(map[string]any); ok {
				delete(row, "vat_amount")
			}
		}
	}
	return body
}

func sdRange(from, to int64) string {
	return fmt.Sprintf("_orderCreatedTime[gte]=%d&_orderCreatedTime[lte]=%d", from, to)
}

func TestServiceDebts_ParityVaTinhTay(t *testing.T) {
	if testDB == nil {
		t.Skip("no TEST_MYSQL_DSN")
	}
	f := seedServiceDebt(t)
	h := &Handler{DB: testDB}
	all := []uint{f.b1, f.b2}

	month, year := sdRange(sdFrom, sdTo), sdRange(sdFrom-300*86400, sdTo)
	queries := []string{
		month, year, "",
		month + "&_keyword=Nguyễn", "_keyword=Nợ", "_keyword=khongkhop",
		month + fmt.Sprintf("&filter[branch_id][0]=%d", f.b2),
		month + "&filter[service_id][0]=9501", "filter[service_id][0]=9502",
		month + "&sort=-created_day", "sort=id&page=2&per-page=1", "page=50",
		sdRange(sdFrom, 0), // chỉ 1 cận ⇒ không lọc ngày (from/to phải cùng > 0)
	}
	type sess struct {
		name         string
		active, user uint
	}
	for _, s := range []sess{{"toàn CN", 0, 99}, {"1 CN", f.b1, 99}, {"telesale", 0, sdTele}} {
		if s.name == "telesale" {
			setTelesale(t)
		}
		for _, q := range queries {
			oldB := callDebt(t, h.legacyServiceDebts, q, s.active, s.user, all)
			newB := stripVatKeys(callDebt(t, h.ServiceDebts, q, s.active, s.user, all))
			if !reflect.DeepEqual(oldB, newB) {
				t.Fatalf("[%s] %q: old != new\nold: %v\nnew: %v", s.name, q, oldB, newB)
			}
		}
	}

	// Tính tay (đang ở phiên telesale sau vòng trên ⇒ bật lại phiên thường bằng user khác).
	// Tháng, toàn CN (b1, b2, NULL): dòng 1 (600) + 2 (500) + 3 (200) ⇒ nợ 1.300, phát sinh 1.800, đã thu 500;
	// cọc = khách 9601 + 9602 + 9603 = 123 (mỗi khách 1 lần).
	b := callDebt(t, h.ServiceDebts, month, 0, 99, all)
	tot := b["total"].(map[string]any)
	requireMoney(t, 1300, tot["totalDebt"], "totalDebt")
	requireMoney(t, 1800, tot["totalEnd"], "totalEnd")
	requireMoney(t, 500, tot["totalPayment"], "totalPayment")
	requireMoney(t, 123, tot["totalDeposit"], "totalDeposit")
	require.Len(t, b["items"].([]any), 3)
	// Không lọc ngày: + dòng 8 (200) + dòng 10 (80).
	requireMoney(t, 1580, callDebt(t, h.ServiceDebts, "", 0, 99, all)["total"].(map[string]any)["totalDebt"], "totalDebt không lọc ngày")
	// Telesale sdTele chỉ thấy khách 9601, 9602 (payment_user_id): tháng ⇒ 600 + 500.
	b = callDebt(t, h.ServiceDebts, month, 0, sdTele, all)
	requireMoney(t, 1100, b["total"].(map[string]any)["totalDebt"], "totalDebt telesale")
	requireMoney(t, 120, b["total"].(map[string]any)["totalDeposit"], "totalDeposit telesale")
}

// Export dùng CHÍNH serviceDebtQuery (kèm JOIN customer) ⇒ so tập dòng của câu export cũ vs mới
// (cùng Select + JOIN branch như export/report_debt.go ExportServiceDebts).
func TestServiceDebts_ExportQueryParity(t *testing.T) {
	if testDB == nil {
		t.Skip("no TEST_MYSQL_DSN")
	}
	f := seedServiceDebt(t)
	h := &Handler{DB: testDB}
	rowsOf := func(q *gorm.DB) []map[string]any {
		var rows []map[string]any
		require.NoError(t, q.Session(&gorm.Session{}).
			Select(`order_service_used.id, order_service_used.code,
				order_service_used.totalEnd, order_service_used.payment, order_service_used.created_day,
				customer.code AS customer_code, customer.fullname AS customer_fullname, customer.totalDeposit,
				service.name AS service_name, br.name AS branch_name`).
			Joins("LEFT JOIN branch br ON br.id = order_service_used.branch_id").Find(&rows).Error)
		sort.Slice(rows, func(i, j int) bool { return fmt.Sprint(rows[i]["id"]) < fmt.Sprint(rows[j]["id"]) })
		return rows
	}
	for _, r := range []struct {
		q        string
		from, to int64
	}{{"", sdFrom, sdTo}, {"", 0, 0}, {"_keyword=Nợ", 0, 0}, {"filter[service_id][0]=9501", sdFrom, sdTo}} {
		c := testhelpers.TenantCtx(0, 99)
		c.Set("branchIds", []uint{f.b1, f.b2})
		c.Request = httptest.NewRequest("GET", "/x?"+r.q, nil)
		o := rowsOf(serviceDebtSort(c, h.legacyServiceDebtQuery(c, true, r.from, r.to)))
		n := rowsOf(serviceDebtSort(c, h.serviceDebtQuery(c, true, r.from, r.to)))
		require.NotEmpty(t, o, "%+v: tập rỗng = so sánh xanh giả", r)
		require.Equal(t, o, n, "%+v: export old != new", r)
	}
}

// Khuôn câu thật: osu PHẢI dẫn trước service (STRAIGHT_JOIN) + hint đúng cổng — revert vẫn ra cùng số
// nên test so số ở trên không bắt được. DB thật chỉ để serviceDebtQuery đọc phạm vi telesale; câu chính
// dựng bằng DryRun.
func TestServiceDebtQuery_OsuLuonDanTruocService(t *testing.T) {
	if testDB == nil {
		t.Skip("no TEST_MYSQL_DSN")
	}
	h := &Handler{DB: testDB}
	sqlOf := func(raw string, from, to int64) string {
		c := testhelpers.TenantCtx(0, 99)
		c.Request = httptest.NewRequest("GET", "/x?"+raw, nil)
		var rows []map[string]any
		return h.serviceDebtQuery(c, true, from, to).Select("order_service_used.id").
			Session(&gorm.Session{DryRun: true}).Find(&rows).Statement.SQL.String()
	}
	s := sqlOf("", sdFrom, sdTo)
	for _, want := range []string{
		"STRAIGHT_JOIN service ON service.id = order_service_used.service_id",
		"FORCE INDEX (" + serviceDebtDateIndex + ")",
		"((order_service_used.totalEnd + order_service_used.vat_amount) <> order_service_used.payment) = 1",
		"order_service_used.branch_id",
	} {
		require.Contains(t, s, want)
	}
	s = sqlOf("", 0, 0)
	require.Contains(t, s, "STRAIGHT_JOIN service")
	// Câu GOM dải rộng: hash join, service làm bảng build, osu mất chỉ mục service_id.
	cAgg := testhelpers.TenantCtx(0, 99)
	cAgg.Request = httptest.NewRequest("GET", "/x", nil)
	var aggRows []map[string]any
	s = h.serviceDebtQueryPlan(cAgg, false, 0, 0, serviceDebtPlanAgg).Select("order_service_used.id").
		Session(&gorm.Session{DryRun: true}).Find(&aggRows).Statement.SQL.String()
	require.Contains(t, s, "FROM "+serviceDebtHashFrom)
	cAgg = testhelpers.TenantCtx(0, 99) // ctx mới: gin cache query string theo ctx
	cAgg.Request = httptest.NewRequest("GET", "/x?filter[service_id][0]=9501", nil)
	s = h.serviceDebtQueryPlan(cAgg, false, 0, 0, serviceDebtPlanAgg).Select("order_service_used.id").
		Session(&gorm.Session{DryRun: true}).Find(&aggRows).Statement.SQL.String()
	require.NotContains(t, s, "IGNORE INDEX", "lọc service_id thì giữ ref fk service_id: %s", s)
	require.False(t, strings.Contains(s, "FORCE INDEX"), "không lọc ngày thì KHÔNG được ép chỉ mục ngày: %s", s)
	// Câu trang: KHÔNG ép (đọc ngược PRIMARY dừng sớm 0,4ms vs ép 231ms) nhưng vẫn STRAIGHT_JOIN.
	c := testhelpers.TenantCtx(0, 99)
	var rows []map[string]any
	s = h.serviceDebtQueryPlan(c, false, sdFrom, sdTo, serviceDebtPlanPage).Select("order_service_used.id").
		Session(&gorm.Session{DryRun: true}).Find(&rows).Statement.SQL.String()
	require.Contains(t, s, "STRAIGHT_JOIN service")
	require.False(t, strings.Contains(s, "FORCE INDEX"), "câu trang không được ép chỉ mục ngày: %s", s)
}
