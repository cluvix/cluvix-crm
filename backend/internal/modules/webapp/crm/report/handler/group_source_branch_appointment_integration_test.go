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
)

// Neo hai bản vá 2026-09-09:
//
//   - GroupSourceStatistics: gộp B/C/D (3 lượt quét `appointment` cùng dải) thành 1, và bỏ lớp
//     bọc `FROM customer LEFT JOIN (...)` vô dụng ở E/B/C/D. Cụm P/Q/R/S/T/U CỐ Ý giữ nguyên
//     (gộp đo được CHẬM 2,4× — xem comment trong crm.go).
//   - BranchAppointment: gộp A/B/C/D thành 1 lượt quét.
//
// Cả hai đổi hình dạng câu SQL nhưng PHẢI giữ nguyên từng con số. Test seed phân bố biết trước
// rồi khoá đúng các chỉ số bị gộp — mutation vào điều kiện status/is_accepted phải làm test đỏ.
//
// Chạy trên DB dùng-một-lần (TEST_MYSQL_DSN), KHÔNG chạm DB dev.

const (
	gsBranchA  = 71
	gsBranchB  = 72
	gsSource   = 7310
	gsGroup    = 7320
	gsType     = 7330
	gsTypeNo   = 7331
	gsCustomer = 7340
	gsFrom     = 1788195600
	gsTo       = 1788973200
)

// seedGroupSourceBranch dựng phân bố biết trước dùng cho CẢ HAI endpoint.
//
// Lịch hẹn (loại 7330 = tính doanh thu, nguồn 7310, khách 7340), trong dải appointment_date:
//
//	7351 CN A status 2 accepted 1 · 7352 CN A status 3 accepted 0 · 7353 CN A status 1
//	7354 CN A status 4 (huỷ)      · 7355 CN B status 2 accepted 1
//	7356 CN A loại KHÔNG tính doanh thu · 7357 CN A ngoài dải
//	7358 CN A status 1 (dòng status-1 thứ hai) · 7359 CN A status 2 accepted 1, customer_id NULL
//
// ⇒ group-source (lọc isTypeRevenue + customer_id NOT NULL, KHÔNG lọc chi nhánh):
//
//	totalAppointment = 6 · Den = 3 · DongY = 2   (bỏ 7356 khác loại, 7357 ngoài dải, 7359 thiếu khách)
//
// ⇒ branch-appointment (KHÔNG lọc isTypeRevenue, KHÔNG lọc khách, tách theo chi nhánh):
//
//	CN A: total 7 · arrived 4 · cancelled 1 · agreed 3
//	CN B: total 1 · arrived 1 · cancelled 0 · agreed 1
func seedGroupSourceBranch(t *testing.T) {
	t.Helper()
	testDB.Exec(`DELETE FROM appointment`)
	testDB.Exec(`DELETE FROM appointment_type WHERE id IN (?, ?)`, gsType, gsTypeNo)
	testDB.Exec(`DELETE FROM customer WHERE id = ?`, gsCustomer)
	testDB.Exec(`DELETE FROM group_source WHERE id = ?`, gsGroup)
	testDB.Exec(`DELETE FROM source WHERE id = ?`, gsSource)
	testDB.Exec(`DELETE FROM branch WHERE id IN (?, ?)`, gsBranchA, gsBranchB)

	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed %s: %v", what, err)
		}
	}
	for _, b := range []struct {
		id   int
		name string
	}{{gsBranchA, "CN A"}, {gsBranchB, "CN B"}} {
		must("branch", testDB.Exec(`INSERT INTO branch (id, name) VALUES (?, ?)`, b.id, b.name).Error)
	}
	must("source", testDB.Exec(`INSERT INTO source (id, name) VALUES (?, 'Nguon QA')`, gsSource).Error)
	must("group_source", testDB.Exec(`INSERT INTO group_source (id, name, sourceId)
		VALUES (?, 'Nhom QA', ?)`, gsGroup,
		fmt.Sprintf("[%d]", gsSource)).Error)
	must("customer", testDB.Exec(`INSERT INTO customer (id, branch_id, fullname, created_day, source_id, stage_id)
		VALUES (?, ?, 'KH', ?, ?, (SELECT id FROM customer_stage WHERE is_default = 1))`, gsCustomer, gsBranchA, gsFrom+100, gsSource).Error)
	for id, rev := range map[int]int{gsType: 1, gsTypeNo: 0} {
		must("appointment_type", testDB.Exec(`INSERT INTO appointment_type (id, name, isTypeRevenue) VALUES (?, 'LH', ?)`,
			id, rev).Error)
	}
	appts := []struct {
		id, branch, typeID, status, accepted int
		apptDate                             int64
	}{
		{7351, gsBranchA, gsType, 2, 1, gsFrom + 100},
		{7352, gsBranchA, gsType, 3, 0, gsFrom + 200},
		{7353, gsBranchA, gsType, 1, 0, gsFrom + 300},
		{7354, gsBranchA, gsType, 4, 0, gsFrom + 400},
		{7355, gsBranchB, gsType, 2, 1, gsFrom + 500},
		{7356, gsBranchA, gsTypeNo, 2, 1, gsFrom + 600},
		{7357, gsBranchA, gsType, 2, 1, gsTo + 86400*40},
		// 7358: lịch status 1 THỨ HAI. Có nó thì số lịch status 1 (2) khác số lịch status 4 (1),
		// nên mutation đổi `status = 4` thành `status = 1` ở cột `cancelled` mới đỏ được.
		// Trước khi thêm, mỗi loại đúng 1 dòng ⇒ đổi hằng vẫn ra cùng con số, test xanh giả.
		{7358, gsBranchA, gsType, 1, 0, gsFrom + 700},
	}
	for _, a := range appts {
		must("appointment", testDB.Exec(`INSERT INTO appointment
			(id, code, branch_id, customer_id, appointment_type_id, source_id,
			 appointment_date, created_at, status, is_accepted)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.id, fmt.Sprintf("AG%d", a.id), a.branch, gsCustomer, a.typeID, gsSource,
			a.apptDate, a.apptDate, a.status, a.accepted).Error)
	}
	// 7359: lịch KHÔNG gắn khách (`customer_id` nullable). Nó tồn tại để khoá vế
	// `appointment.customer_id IS NOT NULL` — vế thay cho lớp bọc `LEFT JOIN customer` đã bỏ.
	// Thiếu dòng này thì xoá vế đó đi test vẫn xanh, và bản vá sẽ âm thầm đếm dư.
	// group-source PHẢI bỏ qua nó; branch-appointment (không lọc khách) PHẢI đếm nó.
	must("appointment 7359", testDB.Exec(`INSERT INTO appointment
		(id, code, branch_id, customer_id, appointment_type_id, source_id,
		 appointment_date, created_at, status, is_accepted)
		VALUES (7359, 'AG7359', ?, NULL, ?, ?, ?, ?, 2, 1)`, gsBranchA, gsType, gsSource, gsFrom+800, gsFrom+800).Error)
}

func callGsb(t *testing.T, fn func(*gin.Context), rawQuery string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?"+rawQuery, nil)
	c.Set("userID", uint(99))

	fn(c)
	if w.Code >= 400 {
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
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse JSON: %v — %s", err, body)
	}
	return out
}

func num(t *testing.T, v any) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(v)), 64)
	if err != nil {
		t.Fatalf("không parse được số từ %v: %v", v, err)
	}
	return f
}

func TestGroupSourceStatistics_GopBCD(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedGroupSourceBranch(t)
	h := &Handler{Deps: Deps{DB: testDB}}
	out := callGsb(t, h.GroupSourceStatistics,
		fmt.Sprintf("filter[created_at][gte]=%d&filter[created_at][lte]=%d", gsFrom, gsTo))
	items, _ := out["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("đúng 1 nhóm nguồn có số liệu, got %d: %+v", len(items), items)
	}
	row := items[0].(map[string]any)
	for _, tc := range []struct {
		field string
		want  float64
		why   string
	}{
		{"totalLead", 1, "1 khách tạo trong dải"},
		{"totalAppointment", 6, "6 lịch tính doanh thu, CÓ khách, trong dải — 7359 bị loại vì customer_id NULL"},
		{"totalAppointmentDen", 3, "chỉ 7351/7352/7355 có status 2 hoặc 3"},
		{"totalAppointmentDongY", 2, "chỉ 7351/7355 vừa status 2/3 vừa is_accepted = 1"},
		{"totalAppointmentNew", 6, "cùng 6 lịch đó, tính theo created_at"},
	} {
		if got := num(t, row[tc.field]); got != tc.want {
			t.Fatalf("%s = %v, muốn %v — %s", tc.field, got, tc.want, tc.why)
		}
	}
}

func TestBranchAppointment_GopABCD(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedGroupSourceBranch(t)
	h := &Handler{Deps: Deps{DB: testDB}}
	out := callGsb(t, h.BranchAppointment,
		fmt.Sprintf("_created_at[gte]=%d&_created_at[lte]=%d", gsFrom, gsTo))
	items, _ := out["items"].([]any)
	got := map[float64][4]float64{}
	for _, it := range items {
		r := it.(map[string]any)
		got[num(t, r["branchId"])] = [4]float64{
			num(t, r["totalAppointment"]), num(t, r["arrived"]),
			num(t, r["cancelled"]), num(t, r["agreed"]),
		}
	}
	// CN A: 7 lịch trong dải (7351..7354, 7356, 7358, 7359) — branch-appointment KHÔNG lọc
	// isTypeRevenue và cũng KHÔNG lọc khách, nên đếm cả 7356 lẫn 7359.
	for _, tc := range []struct {
		branch float64
		want   [4]float64
		why    string
	}{
		{gsBranchA, [4]float64{7, 4, 1, 3}, "CN A: 7 lịch · đến 7351/7352/7356/7359 · huỷ 7354 · đồng ý 7351/7356/7359"},
		{gsBranchB, [4]float64{1, 1, 0, 1}, "CN B: chỉ 7355"},
	} {
		if got[tc.branch] != tc.want {
			t.Fatalf("chi nhánh %v = %v, muốn %v — %s", tc.branch, got[tc.branch], tc.want, tc.why)
		}
	}
	totals, _ := out["totals"].(map[string]any)
	if v := num(t, totals["totalAppointment"]); v != 8 {
		t.Fatalf("totals.totalAppointment = %v, muốn 8 (7 CN A + 1 CN B)", v)
	}
}

func TestGroupSourceStatistics_GuardDaiNgay(t *testing.T) {
	h := &Handler{Deps: Deps{DB: nil}} // DB nil: guard phải chặn TRƯỚC khi chạm DB
	for _, tc := range []struct {
		name, q string
	}{
		{"thiếu cả hai đầu", ""},
		{"thiếu lte", "filter[created_at][gte]=1788195600"},
		{"thiếu gte", "filter[created_at][lte]=1788973200"},
		{"gte không phải số", "filter[created_at][gte]=abc&filter[created_at][lte]=1788973200"},
		{"lte < gte", "filter[created_at][gte]=1788973200&filter[created_at][lte]=1788195600"},
		{"vượt trần 1 năm", "filter[created_at][gte]=1700000000&filter[created_at][lte]=1788973200"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/?"+tc.q, nil)
			h.GroupSourceStatistics(c)
			if w.Code != 422 {
				t.Fatalf("phải 422, got %d — %s", w.Code, w.Body.String())
			}
		})
	}
}
