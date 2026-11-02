//go:build integration

// Integration test phễu tiến trình khách (epic customer-stage-pipeline story-07): handler thật trên DB
// dùng-một-lần (TEST_MYSQL_DSN, tên *_test/*_tmp) với 8 bước seed của migration create_customer_stage.
package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"

	stagesvc "erpCluvix/internal/modules/webapp/config/customer_stage/service"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/cache"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/tz"
)

// funnelItem — shape response mới (story-10 FE đọc đúng shape này).
type funnelItem struct {
	StageID int    `json:"stage_id"`
	Name    string `json:"name"`
	Type    int    `json:"type"`
	Color   string `json:"color"`
	Value   int64  `json:"value"`
}

var sfo = func(h *Handler) gin.HandlerFunc { return h.CrmStatusCustomerOverview }

// stageIDByName — id bước theo tên seed (khoá seed của migration là `name`, không phải id).
func stageIDByName(t *testing.T, name string) int {
	t.Helper()
	var ids []int
	if err := testDB.Raw("SELECT id FROM customer_stage WHERE name = ?", name).Scan(&ids).Error; err != nil || len(ids) != 1 {
		t.Fatalf("bước %q: ids=%v err=%v (seed migration create_customer_stage thiếu?)", name, ids, err)
	}
	return ids[0]
}

// addStagedCustomer — khách thuộc chi nhánh branch, created_day = at, đi lần lượt qua các bước `path`
// (ghi lịch sử bằng RecordStageChange — đúng đường ghi thật của story-03) và đứng ở bước cuối.
// Lịch sử tự xoá theo khách (FK customer_id ON DELETE CASCADE).
func addStagedCustomer(t *testing.T, branch *uint, at int64, provinceID int, path ...string) uint64 {
	t.Helper()
	id := testhelpers.CreateScopedCustomer(t, testDB, nil, branch)
	// Fixture đã ghi 1 dòng lịch sử khởi tạo; test tự dựng đường đi của khách nên xoá dòng đó trước.
	if err := testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	var from *int
	last := 0
	for i, name := range path {
		to := stageIDByName(t, name)
		src := define.CustomerStageHistorySourceManual
		if i == 0 {
			src = define.CustomerStageHistorySourceCreate
		}
		if err := stagesvc.RecordStageChange(testDB, int(id), from, to, src, 0, at+int64(i)); err != nil {
			t.Fatalf("RecordStageChange: %v", err)
		}
		f := to
		from, last = &f, to
	}
	// path rỗng ⇒ khách giữ stage_id mặc định của fixture nhưng KHÔNG có dòng lịch sử nào (không dùng NULL:
	// cột sẽ NOT NULL ở story-12).
	upd := map[string]any{"created_day": at}
	if last != 0 {
		upd["stage_id"] = last
	}
	if provinceID > 0 {
		upd["province_id"] = provinceID
	}
	if err := testDB.Table("customer").Where("id = ?", id).Updates(upd).Error; err != nil {
		t.Fatal(err)
	}
	return id
}

func decodeFunnel(t *testing.T, w *httptest.ResponseRecorder) []funnelItem {
	t.Helper()
	var rows []funnelItem
	decodeJSON(t, w, &rows)
	return rows
}

// funnelString — "tên=giá trị" theo đúng thứ tự response, để so sánh cả thứ tự lẫn số.
func funnelString(rows []funnelItem) string {
	parts := make([]string, 0, len(rows))
	for _, r := range rows {
		parts = append(parts, fmt.Sprintf("%s=%d", r.Name, r.Value))
	}
	return strings.Join(parts, ", ")
}

const prdFunnel = "Liên hệ=2, Đang tư vấn=2, Đang đàm phán=1, Đã lên đơn=1, Khách hàng đã thuê=1, Từ chối=1, Dừng chăm sóc=0, Ngừng thuê=0"

// AC1 + AC3: kịch bản PRD AC12 — A: Liên hệ→Đang tư vấn→Từ chối; B: Liên hệ→Đang đàm phán→Khách hàng
// đã thuê; cùng chi nhánh, ghi nhận trong kỳ. Khách chi nhánh khác / ngoài kỳ / không có lịch sử (khách
// của FreshTenant) không bị đếm.
func TestStageFunnel_PRDScenario(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	fx := seedDash(t)
	loc := tz.LocFromContext(nil, testDB)
	som, lastM := tz.StartOfMonth(loc, 0), tz.StartOfMonth(loc, -1)
	a, b := fx.branchA, fx.branchB
	addStagedCustomer(t, &a, som+3600, 0, "Liên hệ", "Đang tư vấn", "Từ chối")
	addStagedCustomer(t, &a, som+7200, 0, "Liên hệ", "Đang đàm phán", "Khách hàng đã thuê")
	addStagedCustomer(t, &b, som+3600, 0, "Liên hệ", "Đã lên đơn", "Dừng chăm sóc")        // chi nhánh khác
	addStagedCustomer(t, &a, lastM+3600, 0, "Liên hệ", "Khách hàng đã thuê", "Ngừng thuê") // ngoài kỳ
	both := []uint{fx.branchA, fx.branchB}

	rows := decodeFunnel(t, callDash(t, sfo, fmt.Sprintf("branch_id=%d", fx.branchA), both))
	if got := funnelString(rows); got != prdFunnel {
		t.Fatalf("phễu chi nhánh A:\n muốn %s\n nhận %s", prdFunnel, got)
	}
	// AC2 shape: đủ stage_id/type/color khớp danh mục; loại 1–2 trước, loại 3 sau.
	for i, r := range rows {
		if r.StageID != stageIDByName(t, r.Name) || r.Color == "" {
			t.Fatalf("dòng %d thiếu/sai stage_id hoặc color: %+v", i, r)
		}
		wantLost := i >= 5
		if (r.Type == define.CustomerStageTypeLost) != wantLost {
			t.Fatalf("dòng %d (%s) type=%d sai vị trí nhóm", i, r.Name, r.Type)
		}
	}

	// Không lọc chi nhánh (phạm vi phiên = A+B) ⇒ có thêm khách của B: Liên hệ..Đã lên đơn +1, Dừng chăm sóc 1.
	want := "Liên hệ=3, Đang tư vấn=3, Đang đàm phán=2, Đã lên đơn=2, Khách hàng đã thuê=1, Từ chối=1, Dừng chăm sóc=1, Ngừng thuê=0"
	if got := funnelString(decodeFunnel(t, callDash(t, sfo, "", both))); got != want {
		t.Fatalf("phễu A+B:\n muốn %s\n nhận %s", want, got)
	}
	// Phiên chỉ có quyền B ⇒ khách của A không lọt vào.
	want = "Liên hệ=1, Đang tư vấn=1, Đang đàm phán=1, Đã lên đơn=1, Khách hàng đã thuê=0, Từ chối=0, Dừng chăm sóc=1, Ngừng thuê=0"
	if got := funnelString(decodeFunnel(t, callDash(t, sfo, "", []uint{fx.branchB}))); got != want {
		t.Fatalf("phễu phiên chỉ B:\n muốn %s\n nhận %s", want, got)
	}
	// Kỳ tháng trước (from/to tường minh) chi nhánh A ⇒ chỉ khách ngoài kỳ ở trên.
	q := fmt.Sprintf("from=%d&to=%d&branch_id=%d", lastM, som, fx.branchA)
	want = "Liên hệ=1, Đang tư vấn=1, Đang đàm phán=1, Đã lên đơn=1, Khách hàng đã thuê=1, Từ chối=0, Dừng chăm sóc=0, Ngừng thuê=1"
	if got := funnelString(decodeFunnel(t, callDash(t, sfo, q, both))); got != want {
		t.Fatalf("phễu tháng trước A:\n muốn %s\n nhận %s", want, got)
	}
}

// AC2: bước ẩn chỉ hiện khi value > 0; không có khách trong kỳ ⇒ `[]` (không phải null).
func TestStageFunnel_HiddenStageAndEmpty(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	fx := seedDash(t)
	loc := tz.LocFromContext(nil, testDB)
	som := tz.StartOfMonth(loc, 0)
	a := fx.branchA
	addStagedCustomer(t, &a, som+3600, 0, "Liên hệ", "Đang tư vấn", "Từ chối")
	addStagedCustomer(t, &a, som+7200, 0, "Liên hệ", "Đang đàm phán", "Khách hàng đã thuê")
	for _, name := range []string{"Đang đàm phán", "Dừng chăm sóc"} {
		id := stageIDByName(t, name)
		if err := testDB.Exec("UPDATE customer_stage SET status = 0 WHERE id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { testDB.Exec("UPDATE customer_stage SET status = 1 WHERE id = ?", id) })
	}
	both := []uint{fx.branchA, fx.branchB}
	want := "Liên hệ=2, Đang tư vấn=2, Đang đàm phán=1, Đã lên đơn=1, Khách hàng đã thuê=1, Từ chối=1, Ngừng thuê=0"
	if got := funnelString(decodeFunnel(t, callDash(t, sfo, fmt.Sprintf("branch_id=%d", fx.branchA), both))); got != want {
		t.Fatalf("bước ẩn (Đang đàm phán có khách — giữ; Dừng chăm sóc = 0 — bỏ):\n muốn %s\n nhận %s", want, got)
	}

	// Chi nhánh B không có khách nào có lịch sử (chỉ khách không lịch sử nếu có) ⇒ [].
	w := callDash(t, sfo, fmt.Sprintf("branch_id=%d", fx.branchB), both)
	if body := strings.TrimSpace(w.Body.String()); body != "[]" {
		t.Fatalf("không có khách trong kỳ: muốn [], nhận %s", body)
	}
	// Khách KHÔNG có dòng lịch sử bước nào (chỉ có stage_id mặc định) ⇒ không được đếm.
	b := fx.branchB
	addStagedCustomer(t, &b, som+3600, 0)
	if body := strings.TrimSpace(callDash(t, sfo, fmt.Sprintf("branch_id=%d", fx.branchB), both).Body.String()); body != "[]" {
		t.Fatalf("khách không lịch sử: muốn [], nhận %s", body)
	}
}

// AC4: widget khu vực đếm khách đã TỪNG tới bước *thành công* (kể cả sau đó Ngừng thuê); khách chưa từng
// thành công không đếm; khách chi nhánh khác không đếm khi lọc chi nhánh.
func TestStageFunnel_RegionCountsEverWon(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	fx := seedDash(t)
	provName := fmt.Sprintf("Tỉnh test %d", time.Now().UnixNano())
	if err := testDB.Exec("INSERT INTO location_province (name, status) VALUES (?, 1)", provName).Error; err != nil {
		t.Fatal(err)
	}
	prov := int(testhelpers.LastInsertID(testDB))
	// Đăng ký TRƯỚC khi tạo khách ⇒ chạy SAU khi khách đã xoá (t.Cleanup LIFO) — tránh 1451.
	t.Cleanup(func() { testDB.Exec("DELETE FROM location_province WHERE id = ?", prov) })

	loc := tz.LocFromContext(nil, testDB)
	som := tz.StartOfMonth(loc, 0)
	a, b := fx.branchA, fx.branchB
	addStagedCustomer(t, &a, som+3600, prov, "Liên hệ", "Khách hàng đã thuê", "Ngừng thuê")  // từng thành công
	addStagedCustomer(t, &a, som+3600, prov, "Liên hệ", "Đang tư vấn", "Khách hàng đã thuê") // đang thành công
	addStagedCustomer(t, &a, som+3600, prov, "Liên hệ", "Đã lên đơn", "Từ chối")             // chưa từng
	addStagedCustomer(t, &a, som+3600, prov, "Liên hệ")                                      // chưa từng
	addStagedCustomer(t, &b, som+3600, prov, "Liên hệ", "Khách hàng đã thuê")                // chi nhánh khác

	region := func(h *Handler) gin.HandlerFunc { return h.CrmReportCustomerByRegion }
	both := []uint{fx.branchA, fx.branchB}
	value := func(q string) int64 {
		var rows []struct {
			Name  string `json:"name"`
			Value int64  `json:"value"`
		}
		decodeJSON(t, callDash(t, region, q, both), &rows)
		for _, r := range rows {
			if r.Name == provName {
				return r.Value
			}
		}
		return 0
	}
	if got := value(fmt.Sprintf("branch_id=%d", fx.branchA)); got != 2 {
		t.Fatalf("khu vực chi nhánh A: muốn 2 (từng/đang thành công), nhận %d", got)
	}
	if got := value(""); got != 3 {
		t.Fatalf("khu vực A+B: muốn 3, nhận %d", got)
	}
}

// AC6: cache 300s đi khoá MỚI `crm_stage_funnel` — payload shape cũ còn trong Redis dưới khoá cũ không bị
// đọc nhầm; lần gọi thứ 2 trả đúng payload đã cache (cùng shape).
func TestStageFunnel_CacheKeyRenamed(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	fx := seedDash(t)
	loc := tz.LocFromContext(nil, testDB)
	a := fx.branchA
	addStagedCustomer(t, &a, tz.StartOfMonth(loc, 0)+3600, 0, "Liên hệ", "Đang tư vấn", "Từ chối")
	mr := miniredis.RunT(t)
	h := New(Deps{DB: testDB, Cache: cache.New(mr.Addr(), "", "t", 0)})
	q := fmt.Sprintf("branch_id=%d", fx.branchA)
	both := []uint{fx.branchA, fx.branchB}

	first := callDashWith(t, h, sfo, q, both).Body.String()
	var hasNew bool
	for _, k := range mr.Keys() {
		if strings.Contains(k, "crm_status_customer_overview") {
			t.Fatalf("còn ghi khoá cũ: %s", k)
		}
		hasNew = hasNew || strings.Contains(k, "crm_stage_funnel_")
	}
	if !hasNew {
		t.Fatalf("không thấy khoá crm_stage_funnel_*: %v", mr.Keys())
	}
	// Thêm khách sau khi đã cache ⇒ lần 2 vẫn trả payload cache (đi nhánh HIT, decode đúng shape).
	addStagedCustomer(t, &a, tz.StartOfMonth(loc, 0)+7200, 0, "Liên hệ")
	second := callDashWith(t, h, sfo, q, both).Body.String()
	var x, y []funnelItem
	if json.Unmarshal([]byte(first), &x) != nil || json.Unmarshal([]byte(second), &y) != nil || funnelString(x) != funnelString(y) || len(y) == 0 || y[0].StageID == 0 {
		t.Fatalf("cache HIT phải trả nguyên payload lần 1:\n lần 1 %s\n lần 2 %s", first, second)
	}
}

// setStageSortOrder — đổi sort_order 1 bước trong phạm vi test, tự khôi phục (t.Cleanup).
func setStageSortOrder(t *testing.T, name string, order int) {
	t.Helper()
	id := stageIDByName(t, name)
	var old []int
	if err := testDB.Raw("SELECT sort_order FROM customer_stage WHERE id = ?", id).Scan(&old).Error; err != nil || len(old) != 1 {
		t.Fatalf("đọc sort_order %q: %v %v", name, old, err)
	}
	if err := testDB.Exec("UPDATE customer_stage SET sort_order = ? WHERE id = ?", order, id).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Exec("UPDATE customer_stage SET sort_order = ? WHERE id = ?", old[0], id) })
}

// D8: phễu theo thứ tự `sort_order` HIỆN TẠI, không theo id. Đảo "Đang tư vấn" ↔ "Đang đàm phán" (id seed vẫn
// tăng theo thứ tự cũ) ⇒ thứ tự tầng đổi theo, và A (từng tới Đang tư vấn — nay đứng SAU) được tính vào
// cả tầng Đang đàm phán.
func TestStageFunnel_FollowsSortOrderNotID(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	fx := seedDash(t)
	loc := tz.LocFromContext(nil, testDB)
	som := tz.StartOfMonth(loc, 0)
	a := fx.branchA
	addStagedCustomer(t, &a, som+3600, 0, "Liên hệ", "Đang tư vấn", "Từ chối")
	addStagedCustomer(t, &a, som+7200, 0, "Liên hệ", "Đang đàm phán", "Khách hàng đã thuê")
	if stageIDByName(t, "Đang tư vấn") >= stageIDByName(t, "Đang đàm phán") {
		t.Fatal("đối chứng: seed phải có id Đang tư vấn < Đang đàm phán (nếu không, ca này không phân biệt được id với sort_order)")
	}
	setStageSortOrder(t, "Đang tư vấn", 3)
	setStageSortOrder(t, "Đang đàm phán", 2)
	both := []uint{fx.branchA, fx.branchB}
	rows := decodeFunnel(t, callDash(t, sfo, fmt.Sprintf("branch_id=%d", fx.branchA), both))
	want := "Liên hệ=2, Đang đàm phán=2, Đang tư vấn=2, Đã lên đơn=1, Khách hàng đã thuê=1, Từ chối=1, Dừng chăm sóc=0, Ngừng thuê=0"
	if got := funnelString(rows); got != want {
		t.Fatalf("phễu sau khi đảo thứ tự:\n muốn %s\n nhận %s", want, got)
	}
	if rows[1].StageID != stageIDByName(t, "Đang đàm phán") || rows[2].StageID != stageIDByName(t, "Đang tư vấn") {
		t.Fatalf("stage_id không theo thứ tự mới: %+v", rows)
	}
}
