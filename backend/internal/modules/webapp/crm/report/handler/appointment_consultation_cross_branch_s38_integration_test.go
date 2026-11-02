//go:build integration

package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	appdb "erpCluvix/db"

	"github.com/gin-gonic/gin"
)

// Neo hành vi story-38: attach thông tin khách (`item["customer"]`) trong report tư vấn
// vẫn PHẢI hoạt động khi khách thuộc CHI NHÁNH KHÁC chi nhánh của lịch hẹn/session — trước
// story-38 việc này đi qua scope chỉ lọc thương hiệu (KHÔNG lọc branch);
// sau story-38 phải đổi sang `tenant.CrossBranchLookup` (identity, không filter
// gì) để giữ NGUYÊN hành vi "không lọc branch khi tra khách theo customer_id". Đổi nhầm sang
// `ScopedDBNullBranch` sẽ làm mất khách chi nhánh khác ⇒ item["customer"] = nil.
func TestAppointmentConsultation_AttachKhachChiNhanhKhac(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	appdb.DB = testDB // pagination.Paginate đọc db.DB global
	testDB.Exec(`DELETE FROM appointment`)
	testDB.Exec(`DELETE FROM appointment_type`)
	testDB.Exec(`DELETE FROM customer`)

	const (
		branchOfAppointment = uint(1)
		branchOfCustomer    = uint(2)
	)
	if err := testDB.Exec(`INSERT INTO appointment_type (id, name, isTypeRevenue) VALUES (1, 'tu-van', 1)`).Error; err != nil {
		t.Fatalf("seed appointment_type: %v", err)
	}
	// Khách thuộc chi nhánh 2 — KHÁC chi nhánh của lịch hẹn (chi nhánh 1).
	if err := testDB.Exec(`INSERT INTO customer (id, branch_id, fullname, phone, stage_id)
		VALUES (601, ?, 'Khach Chi Nhanh Khac', '0900000099', (SELECT id FROM customer_stage WHERE is_default = 1))`, branchOfCustomer).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	if err := testDB.Exec(`INSERT INTO appointment
		(id, code, branch_id, customer_id, appointment_type_id, appointment_date, status)
		VALUES (601, 'AP-QA-CROSS', ?, 601, 1, 1788195700, 2)`, branchOfAppointment).Error; err != nil {
		t.Fatalf("seed appointment: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("branchActiveId", branchOfAppointment)
	c.Set("branchIds", []uint{branchOfAppointment})
	c.Set("userID", uint(99))
	c.Request = httptest.NewRequest("GET", "/?per-page=50", nil)

	h := &Handler{Deps: Deps{DB: testDB}}
	h.AppointmentConsultationReport(c)

	if w.Code != 200 {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, w.Body.String())
	}
	var found map[string]interface{}
	for _, it := range body.Items {
		if int64(it["id"].(float64)) == 601 {
			found = it
		}
	}
	if found == nil {
		t.Fatalf("khong thay appointment 601 trong ket qua: %+v", body.Items)
	}
	customer, ok := found["customer"].(map[string]interface{})
	if !ok || customer == nil {
		t.Fatalf("customer chi nhanh khac KHONG duoc attach (item[\"customer\"] = %v) — CrossBranchLookup bi thay bang scope co loc branch", found["customer"])
	}
	if customer["fullname"] != "Khach Chi Nhanh Khac" {
		t.Fatalf("attach sai khach: %+v", customer)
	}
}
