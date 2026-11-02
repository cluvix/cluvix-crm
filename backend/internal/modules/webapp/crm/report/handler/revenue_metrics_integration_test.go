//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Neo bản vá gộp 3 chỉ số lịch hẹn của GET /crm/report/revenue (2026-09-09):
// F/G/H (3 bảng dẫn xuất đọc CÙNG dải ngày, khác nhau mỗi điều kiện status/is_accepted)
// gộp thành MỘT lượt quét bằng `MAX(<điều kiện>)` + `COUNT(CASE WHEN …)`.
// Bản cũ trả ĐÚNG cùng con số nên test này khoá CON SỐ — sai ngữ nghĩa `MAX()` là lệch ngay.
//
// Chạy trên DB dùng-một-lần (TEST_MYSQL_DSN), KHÔNG chạm DB dev.

const (
	revBranch  = 66
	revUserA   = 7701 // nhân viên tư vấn A
	revUserB   = 7702
	revTypeRev = 7710 // loại hẹn isTypeRevenue = 1
	revTypeNo  = 7711 // loại hẹn KHÔNG tính doanh thu
)

// seedRevenue dựng phân bố biết trước cho nhân viên A (revenue_id = 7701):
//
//	khách 7721: 2 lịch — 1 lịch status 1 (chưa đến), 1 lịch status 2 + is_accepted 1
//	khách 7722: 1 lịch — status 3, is_accepted 0
//	khách 7723: 1 lịch — status 1 (chưa đến)
//	khách 7724: 1 lịch — status 2, nhưng loại hẹn KHÔNG tính doanh thu ⇒ bị loại
//
// ⇒ A: totalAppointment = 3 khách · totalAppointmentDen = 2 (7721, 7722) ·
// totalAppointmentDongY = 1 (chỉ 7721).
// Khách 7721 là ca then chốt: một khách có NHIỀU lịch, chỉ MỘT lịch thoả điều kiện —
// `MAX()` phải cho khách đó vào cả 3 chỉ số đúng một lần.
func seedRevenue(t *testing.T) {
	t.Helper()
	testDB.Exec(`DELETE FROM appointment`)
	testDB.Exec(`DELETE FROM appointment_type WHERE id IN (?, ?)`, revTypeRev, revTypeNo)
	testDB.Exec(`DELETE FROM customer WHERE id BETWEEN 7721 AND 7724`)
	testDB.Exec(`DELETE FROM user_profile WHERE user_id IN (?, ?)`, revUserA, revUserB)
	testDB.Exec(`DELETE FROM user WHERE id IN (?, ?)`, revUserA, revUserB)

	for _, u := range []int{revUserA, revUserB} {
		if err := testDB.Exec(`INSERT INTO user (id, username, password_hash) VALUES (?, ?, 'x')`,
			u, fmt.Sprintf("rev_%d", u)).Error; err != nil {
			t.Fatalf("seed user %d: %v", u, err)
		}
		if err := testDB.Exec(`INSERT INTO user_profile (user_id, fullname) VALUES (?, ?)`,
			u, fmt.Sprintf("NV %d", u)).Error; err != nil {
			t.Fatalf("seed user_profile %d: %v", u, err)
		}
	}
	for id, rev := range map[int]int{revTypeRev: 1, revTypeNo: 0} {
		if err := testDB.Exec(`INSERT INTO appointment_type (id, name, isTypeRevenue) VALUES (?, ?, ?)`,
			id, fmt.Sprintf("LH %d", id), rev).Error; err != nil {
			t.Fatalf("seed appointment_type %d: %v", id, err)
		}
	}
	for cid := 7721; cid <= 7724; cid++ {
		if err := testDB.Exec(`INSERT INTO customer (id, branch_id, fullname, stage_id) VALUES (?, ?, 'KH', (SELECT id FROM customer_stage WHERE is_default = 1))`,
			cid, revBranch).Error; err != nil {
			t.Fatalf("seed customer %d: %v", cid, err)
		}
	}
	appts := []struct {
		id, customer, typeID, status, accepted int
	}{
		{7731, 7721, revTypeRev, 1, 0},
		{7732, 7721, revTypeRev, 2, 1},
		{7733, 7722, revTypeRev, 3, 0},
		{7734, 7723, revTypeRev, 1, 0},
		{7735, 7724, revTypeNo, 2, 1},
	}
	for _, a := range appts {
		if err := testDB.Exec(`INSERT INTO appointment
			(id, code, branch_id, customer_id, appointment_type_id, appointment_date,
			 status, is_accepted, revenue_id)
			VALUES (?, ?, ?, ?, ?, 1788195700, ?, ?, ?)`,
			a.id, fmt.Sprintf("AP%d", a.id), revBranch, a.customer, a.typeID,
			a.status, a.accepted, revUserA).Error; err != nil {
			t.Fatalf("seed appointment %d: %v", a.id, err)
		}
	}
}

func callRevenue(t *testing.T) []map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET",
		"/?filter[created_at][gte]=1788195600&filter[created_at][lte]=1788282000", nil)
	c.Set("branchActiveId", uint(revBranch))
	c.Set("branchIds", []uint{revBranch})
	c.Set("userID", uint(99))

	h := &Handler{Deps: Deps{DB: testDB}}
	h.Revenue(c)
	if w.Code >= 400 {
		t.Fatalf("phải 200, got %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse JSON: %v — %s", err, w.Body.String())
	}
	return body.Items
}

func TestRevenue_BaChiSoLichHen(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedRevenue(t)

	items := callRevenue(t)
	if len(items) != 1 {
		t.Fatalf("chỉ nhân viên A có số liệu ⇒ đúng 1 dòng, got %d: %+v", len(items), items)
	}
	row := items[0]
	if fmt.Sprint(row["id"]) != fmt.Sprint(float64(revUserA)) {
		t.Fatalf("dòng phải của nhân viên %d, got %v", revUserA, row["id"])
	}
	for _, tc := range []struct {
		field string
		want  float64
		why   string
	}{
		{"totalAppointment", 3, "3 khách có lịch loại tính doanh thu (loại KHÔNG tính doanh thu bị loại)"},
		{"totalAppointmentDen", 2, "khách 7721 (có 1 lịch status 2) và 7722 (status 3)"},
		{"totalAppointmentDongY", 1, "chỉ khách 7721 có lịch vừa status 2/3 vừa is_accepted = 1"},
	} {
		if row[tc.field] != tc.want {
			t.Fatalf("%s = %v, muốn %v — %s", tc.field, row[tc.field], tc.want, tc.why)
		}
	}
}

// Khách có NHIỀU lịch chỉ được đếm MỘT lần cho mỗi chỉ số. Đây là chỗ dễ vỡ nhất khi gộp:
// bỏ tầng `GROUP BY revenue_id, customer_id` bên trong là 7721 bị đếm 2 lần ở totalAppointment.
func TestRevenue_KhachNhieuLichChiDemMotLan(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedRevenue(t)
	// Thêm 3 lịch nữa cho chính khách 7721, đều thoả cả 3 điều kiện.
	for i, id := range []int{7741, 7742, 7743} {
		if err := testDB.Exec(`INSERT INTO appointment
			(id, code, branch_id, customer_id, appointment_type_id, appointment_date,
			 status, is_accepted, revenue_id)
			VALUES (?, ?, ?, 7721, ?, 1788195700, 2, 1, ?)`,
			id, fmt.Sprintf("AP%d", id), revBranch, revTypeRev, revUserA).Error; err != nil {
			t.Fatalf("seed lịch thứ %d: %v", i, err)
		}
	}

	row := callRevenue(t)[0]
	if row["totalAppointment"] != float64(3) {
		t.Fatalf("totalAppointment vẫn phải là 3 KHÁCH (không phải số lịch), got %v", row["totalAppointment"])
	}
	if row["totalAppointmentDongY"] != float64(1) {
		t.Fatalf("totalAppointmentDongY vẫn phải là 1 khách, got %v", row["totalAppointmentDongY"])
	}
}
