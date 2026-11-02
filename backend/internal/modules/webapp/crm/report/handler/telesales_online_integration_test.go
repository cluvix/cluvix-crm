//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Neo bản vá GET /crm/report/telesales-online (2026-09-09):
//  1. nhánh C "chăm lại" đẩy dải ngày VÀO trong bảng dẫn xuất trên `note`;
//  2. D+E gộp một lượt quét, F+G+H gộp một lượt quét.
//
// Cả hai đổi THỨ TỰ placeholder trong câu raw SQL — đây là kiểu lỗi im lặng nhất của bản vá này
// (đúng số lượng `?`, sai thứ tự ⇒ vẫn chạy, ra số sai). Test khoá CON SỐ của cả 6 chỉ số.
//
// Chạy trên DB dùng-một-lần (TEST_MYSQL_DSN), KHÔNG chạm DB dev.

const (
	tsoBranch   = 88
	tsoUser     = 8801 // nhân viên telesales (departmentList chứa 3)
	tsoSrcOn    = 8810 // source thuộc group_source.online = 1
	tsoSrcOff   = 8811 // source KHÔNG online ⇒ phải bị loại
	tsoTypeRev  = 8820
	tsoGroupSrc = 8830

	tsoFrom = 1788195600 // 2026-09-01
	tsoTo   = 1788973200 // 2026-09-09
)

// seedTelesales dựng phân bố biết trước cho nhân viên 8801:
//
//	khách 8841 (tạo TRƯỚC dải ⇒ "khách cũ") · khách 8842 (tạo TRONG dải) · khách 8843 (tạo trong dải)
//	lịch hẹn (source online, loại tính doanh thu, revenue_id = 8801):
//	  8851 khách 8841 — created_at trong dải · appointment_date trong dải · status 2 · accepted 1
//	  8852 khách 8842 — created_at trong dải · appointment_date trong dải · status 3 · accepted 0
//	  8853 khách 8843 — created_at trong dải · appointment_date NGOÀI dải · status 1
//	  8854 khách 8841 — source KHÔNG online ⇒ bị loại khỏi mọi chỉ số
//	  8855 khách 8841 — lịch THỨ HAI trong dải, status 1 (để MIN != MAX)
//	ghi chú (note) của nhân viên 8801: 1 ghi chú trong dải cho khách 8841 (ngày khác created_day)
//
// ⇒ totalAppointmentNew = 3 (đếm theo created_at) · totalAppointmentNewPlus = 1 (chỉ khách cũ 8841)
// ⇒ totalAppointment = 2 (đếm theo appointment_date) · totalAppointmentDen = 2 · totalAppointmentDongY = 1
// ⇒ totalChamLai = 1
func seedTelesales(t *testing.T) {
	t.Helper()
	for _, q := range []string{
		`DELETE FROM appointment`, `DELETE FROM note`,
		`DELETE FROM appointment_type WHERE id = ?`, `DELETE FROM customer WHERE id BETWEEN 8841 AND 8843`,
		`DELETE FROM user_profile WHERE user_id = ?`, `DELETE FROM user WHERE id = ?`,
		`DELETE FROM group_source WHERE id = ?`,
	} {
		switch q {
		case `DELETE FROM appointment_type WHERE id = ?`:
			testDB.Exec(q, tsoTypeRev)
		case `DELETE FROM user_profile WHERE user_id = ?`, `DELETE FROM user WHERE id = ?`:
			testDB.Exec(q, tsoUser)
		case `DELETE FROM group_source WHERE id = ?`:
			testDB.Exec(q, tsoGroupSrc)
		default:
			testDB.Exec(q)
		}
	}

	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("seed %s: %v", what, err)
		}
	}
	must("user", testDB.Exec(`INSERT INTO user (id, username, password_hash) VALUES (?, 'tso', 'x')`, tsoUser).Error)
	must("user_profile", testDB.Exec(`INSERT INTO user_profile (user_id, fullname, departmentList) VALUES (?, 'NV TSO', JSON_ARRAY(3))`, tsoUser).Error)
	must("group_source", testDB.Exec(`INSERT INTO group_source (id, name, online, sourceId) VALUES (?, 'online', 1, JSON_ARRAY(?))`, tsoGroupSrc, tsoSrcOn).Error)
	must("appointment_type", testDB.Exec(`INSERT INTO appointment_type (id, name, isTypeRevenue) VALUES (?, 'LH', 1)`, tsoTypeRev).Error)

	cust := []struct {
		id        int
		createdAt int64
	}{
		{8841, tsoFrom - 86400*30}, // khách CŨ
		{8842, tsoFrom + 3600},
		{8843, tsoFrom + 7200},
	}
	for _, cu := range cust {
		must("customer", testDB.Exec(`INSERT INTO customer (id, branch_id, fullname, created_day, payment_user_id, source_id, stage_id)
			VALUES (?, ?, 'KH', ?, ?, ?, (SELECT id FROM customer_stage WHERE is_default = 1))`, cu.id, tsoBranch, cu.createdAt, tsoUser, tsoSrcOn).Error)
	}

	appts := []struct {
		id, customer, source, status, accepted int
		createdAt, apptDate                    int64
	}{
		{8851, 8841, tsoSrcOn, 2, 1, tsoFrom + 100, tsoFrom + 200},
		{8852, 8842, tsoSrcOn, 3, 0, tsoFrom + 100, tsoFrom + 200},
		{8853, 8843, tsoSrcOn, 1, 0, tsoFrom + 100, tsoTo + 86400*40}, // ngoài dải appointment_date
		{8854, 8841, tsoSrcOff, 2, 1, tsoFrom + 100, tsoFrom + 200},   // source không online
		// ⚠ Lịch THỨ HAI của khách 8841 trong cùng dải, status 1 (chưa đến). Không có dòng này
		// thì mỗi khách chỉ có 1 lịch ⇒ MIN(<điều kiện>) và MAX(<điều kiện>) cho cùng kết quả,
		// và bản gộp F+G+H có thể bị đổi MAX→MIN mà test vẫn xanh (đã kiểm bằng mutation).
		{8855, 8841, tsoSrcOn, 1, 0, tsoFrom + 100, tsoFrom + 300},
	}
	for _, a := range appts {
		must("appointment", testDB.Exec(`INSERT INTO appointment
			(id, code, branch_id, customer_id, appointment_type_id, source_id,
			 created_at, appointment_date, status, is_accepted, revenue_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.id, fmt.Sprintf("AP%d", a.id), tsoBranch, a.customer, tsoTypeRev, a.source,
			a.createdAt, a.apptDate, a.status, a.accepted, tsoUser).Error)
	}

	// Ghi chú: ngày tạo ghi chú KHÁC customer.created_day ⇒ tính là "chăm lại".
	must("note", testDB.Exec(`INSERT INTO note (id, customer_id, created_at, created_by)
		VALUES (8861, 8841, ?, ?)`, tsoFrom+3*86400, tsoUser).Error)
}

func callTelesales(t *testing.T) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET",
		fmt.Sprintf("/?filter[created_at][gte]=%d&filter[created_at][lte]=%d", tsoFrom, tsoTo), nil)
	c.Set("branchActiveId", uint(tsoBranch))
	c.Set("branchIds", []uint{tsoBranch})
	c.Set("userID", uint(99))

	h := &Handler{Deps: Deps{DB: testDB}}
	h.TelesalesOnline(c)
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
		t.Fatalf("chỉ 1 nhân viên có số liệu ⇒ 1 dòng, got %d: %+v", len(body.Items), body.Items)
	}
	return body.Items[0]
}

func TestTelesalesOnline_SauKhiGopNhanh(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedTelesales(t)
	row := callTelesales(t)

	for _, tc := range []struct {
		field string
		want  float64
		why   string
	}{
		{"totalAppointmentNew", 3, "3 khách có lịch tạo trong dải, nguồn online (lịch nguồn không-online bị loại)"},
		{"totalAppointmentNewPlus", 1, "chỉ khách 8841 được tạo TRƯỚC dải ngày"},
		{"totalAppointment", 2, "chỉ 2 lịch có appointment_date trong dải"},
		{"totalAppointmentDen", 2, "cả 2 lịch đó đều status 2/3"},
		{"totalAppointmentDongY", 1, "chỉ lịch 8851 có is_accepted = 1"},
		{"totalChamLai", 1, "1 ghi chú trong dải, ngày khác created_day của khách"},
	} {
		if row[tc.field] != tc.want {
			t.Fatalf("%s = %v, muốn %v — %s", tc.field, row[tc.field], tc.want, tc.why)
		}
	}
}

// Dải ngày lệch đi phải làm các chỉ số đổi theo — bắt trường hợp placeholder bị hoán vị khiến
// tham số ngày rơi nhầm chỗ mà bộ dữ liệu ở test trên vẫn tình cờ đúng.
func TestTelesalesOnline_DoiDaiNgayThiSoDoiTheo(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedTelesales(t)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// Dải chỉ gồm NGÀY ĐẦU (from .. hết ngày): lịch hẹn vẫn nằm trong, nhưng ghi chú
	// (from + 3 ngày) rơi ra ngoài ⇒ totalChamLai phải về 0.
	c.Request = httptest.NewRequest("GET",
		fmt.Sprintf("/?filter[created_at][gte]=%d&filter[created_at][lte]=%d", tsoFrom, tsoFrom+86399), nil)
	c.Set("branchIds", []uint{tsoBranch})
	c.Set("userID", uint(99))

	h := &Handler{Deps: Deps{DB: testDB}}
	h.TelesalesOnline(c)
	if w.Code >= 400 {
		t.Fatalf("phải 200, got %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	json.Unmarshal(w.Body.Bytes(), &body) //nolint:errcheck
	if len(body.Items) == 0 {
		t.Fatalf("vẫn phải có dòng (lịch hẹn còn trong dải), got rỗng")
	}
	if got := body.Items[0]["totalChamLai"]; got != float64(0) {
		t.Fatalf("ghi chú nằm ngoài dải 1 ngày ⇒ totalChamLai phải là 0, got %v", got)
	}
}
