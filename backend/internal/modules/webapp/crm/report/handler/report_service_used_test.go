//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/internal/testhelpers"
)

// Neo bản vá hiệu năng GET /crm/order-service-used/report-service-used (2026-09-09):
//  1. GROUP BY chỉ `service_id` (bỏ 2 cột phụ thuộc hàm của bảng `service` khỏi khoá sort);
//  2. `customerCount` gộp chung câu totals thay vì chạy câu thứ 2;
//  3. tổng số dòng của phân trang = `COUNT(DISTINCT service_id)` thay cho `COUNT(*)` bọc
//     nguyên bảng dẫn xuất GROUP BY.
//
// Test khoá CẢ con số (2 và 3 sai là ra số khác) LẪN khuôn câu (1 trả đúng cùng số nên
// một test chỉ so số sẽ không bao giờ bắt được lúc ai đó revert).

const (
	rsuBranch   = 55
	rsuService1 = 8801
	rsuService2 = 8802
	rsuCust1    = 8811
	rsuCust2    = 8812
)

// seedReportServiceUsed dựng phân bố biết trước:
//
//	dịch vụ 8801: 3 dòng — khách 8811 (2 dòng) + khách 8812 (1 dòng)
//	dịch vụ 8802: 1 dòng — khách 8812
//	+ 1 dòng isLock = 0 (phải bị loại) và 1 dòng của CHI NHÁNH khác (phải bị loại)
//
// Story-35 (D15 — 1 thương hiệu/DB): dòng nhiễu là "chi nhánh khác" (vế scope duy nhất còn lại);
// con số kỳ vọng giữ nguyên.
//
// ⇒ 2 nhóm · 4 dòng tính vào tổng · 2 khách phân biệt (KHÔNG phải 3 = tổng khách của
// từng nhóm cộng lại — đó chính là cái bẫy khi gộp customerCount vào câu totals).
func seedReportServiceUsed(t *testing.T) {
	t.Helper()
	testDB.Exec(`DELETE FROM order_service_used`)
	testDB.Exec(`DELETE FROM service WHERE id IN (?, ?)`, rsuService1, rsuService2)
	testDB.Exec(`DELETE FROM customer WHERE id IN (?, ?)`, rsuCust1, rsuCust2)

	for _, sid := range []int{rsuService1, rsuService2} {
		// `service.code` là UNIQUE — mã phải khác nhau giữa 2 dịch vụ.
		if err := testDB.Exec(`INSERT INTO service (id, name, code)
			VALUES (?, ?, ?)`, sid, fmt.Sprintf("DV %d", sid), fmt.Sprintf("SV%d", sid)).Error; err != nil {
			t.Fatalf("seed service %d: %v", sid, err)
		}
	}
	for _, cid := range []int{rsuCust1, rsuCust2} {
		if err := testDB.Exec(`INSERT INTO customer (id, branch_id, fullname, stage_id)
			VALUES (?, ?, 'KH', (SELECT id FROM customer_stage WHERE is_default = 1))`, cid, rsuBranch).Error; err != nil {
			t.Fatalf("seed customer %d: %v", cid, err)
		}
	}
	rows := []struct {
		id, service, customer, branch, isLock int
	}{
		{9001, rsuService1, rsuCust1, rsuBranch, 1},
		{9002, rsuService1, rsuCust1, rsuBranch, 1},
		{9003, rsuService1, rsuCust2, rsuBranch, 1},
		{9004, rsuService2, rsuCust2, rsuBranch, 1},
		{9005, rsuService2, rsuCust1, rsuBranch, 0},     // isLock=0 ⇒ loại
		{9006, rsuService1, rsuCust1, rsuBranch + 1, 1}, // chi nhánh khác ⇒ loại
	}
	for _, r := range rows {
		if err := testDB.Exec(`INSERT INTO order_service_used
			(id, code, customer_id, order_plan_id, service_id, branch_id, isLock,
			 quantity, totalEnd, payment, service_discount_type, service_discount, unit_price,
			 discount_other, refundServiceUsed, created_day)
			VALUES (?, ?, ?, 0, ?, ?, ?, 1, 100, 50, 1, 10, 100, 0, 0, 1788195700)`,
			r.id, fmt.Sprintf("OSU%d", r.id), r.customer, r.service, r.branch, r.isLock).Error; err != nil {
			t.Fatalf("seed osu %d: %v", r.id, err)
		}
	}
}

func callReportServiceUsed(t *testing.T) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// Guard dải ngày (2026-09-09) bắt buộc có `created_day` — dữ liệu seed nằm ở 1788195700.
	c.Request = httptest.NewRequest("GET",
		"/?page=1&per-page=20&filter[created_day][gte]=1788195600&filter[created_day][lte]=1788282000", nil)
	c.Set("branchActiveId", uint(rsuBranch))
	c.Set("branchIds", []uint{rsuBranch})
	c.Set("userID", uint(99))

	h := &Handler{Deps: Deps{DB: testDB}}
	h.ReportServiceUsed(c)
	if w.Code >= 400 {
		t.Fatalf("phải 200, got %d %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse JSON: %v — %s", err, w.Body.String())
	}
	return body
}

func TestReportServiceUsed_TongVaSoNhom(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedReportServiceUsed(t)
	body := callReportServiceUsed(t)

	obj, _ := body["object"].(map[string]any)
	meta, _ := obj["_meta"].(map[string]any)
	if got := meta["totalCount"]; got != float64(2) {
		t.Fatalf("totalCount phải là SỐ NHÓM (2 dịch vụ), got %v", got)
	}
	items, _ := obj["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("phải trả 2 nhóm, got %d", len(items))
	}

	totals, _ := body["totals"].(map[string]any)
	if got := totals["customerCount"]; got != float64(2) {
		t.Fatalf("customerCount phải đếm khách PHÂN BIỆT trên toàn bộ bộ lọc (2), không cộng dồn theo nhóm (3), got %v", got)
	}
	if got := totals["numService"]; got != float64(4) {
		t.Fatalf("numService phải là 4 dòng isLock=1 của đúng chi nhánh, got %v", got)
	}
}

// Khoá KHUÔN câu: bản GROUP BY 3 cột cũ trả ĐÚNG cùng kết quả nên test so số ở trên
// sẽ xanh kể cả khi bản vá bị revert.
func TestReportServiceUsed_GroupByChiMotCot(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedReportServiceUsed(t)

	c := testhelpers.TenantCtx(rsuBranch, 99)
	c.Request = httptest.NewRequest("GET",
		"/?filter[created_day][gte]=1788195600&filter[created_day][lte]=1788282000", nil)
	h := &Handler{Deps: Deps{DB: testDB}}

	var rows []map[string]any
	tx := h.buildReportServiceUsedQuery(c).Session(&gorm.Session{DryRun: true}).Find(&rows)
	sql := tx.Statement.SQL.String()
	if strings.TrimSpace(sql) == "" {
		t.Fatal("DryRun không sinh SQL — phép kiểm dưới đây sẽ xanh giả")
	}
	// GORM bọc backtick quanh định danh ⇒ so sánh trên bản đã bỏ backtick.
	bare := strings.ReplaceAll(sql, "`", "")
	i := strings.Index(bare, "GROUP BY ")
	if i < 0 {
		t.Fatalf("không thấy GROUP BY: %s", sql)
	}
	groupBy := bare[i+len("GROUP BY "):]
	if j := strings.Index(groupBy, " ORDER BY"); j >= 0 {
		groupBy = groupBy[:j]
	}
	if strings.TrimSpace(groupBy) != "order_service_used.service_id" {
		t.Fatalf("GROUP BY phải CHỈ gồm order_service_used.service_id — thêm cột của bảng join làm khoá sort phình lên. Đang là: %q", groupBy)
	}
}
