//go:build integration

package handler

import (
	"errors"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/pagination"
)

// Neo bản vá hiệu năng của GET /crm/report/appointment-consultation (2026-09-09):
//   1. lọc loại hẹn bằng IN (<hằng số>) thay cho IN (SELECT ...) — MySQL biến subquery
//      thành semi-join lấy appointment_type làm bảng dẫn, giết bộ lọc ngày;
//   2. LEFT JOIN customer CHỈ gắn khi _export=1 — export.ExportAppointment select
//      customer.* nên bỏ join là chết "Unknown column"; đường list không đọc cột nào
//      của customer nên join chỉ tốn tra PK cho từng dòng, kể cả trong COUNT derived.
//
// Chạy trên DB dùng-một-lần (TEST_MYSQL_DSN), KHÔNG chạm DB dev.

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
	acBranch = uint(1)
)

// seedConsultation dựng 2 loại hẹn tính doanh thu + 1 loại KHÔNG tính, mỗi loại 1 lịch hẹn.
func seedConsultation(t *testing.T, revenueTypes []int) {
	t.Helper()
	testDB.Exec(`DELETE FROM appointment`)
	testDB.Exec(`DELETE FROM appointment_type`)
	testDB.Exec(`DELETE FROM customer`)

	if err := testDB.Exec(`INSERT INTO customer (id, branch_id, fullname, phone, stage_id)
		VALUES (501, ?, 'Khách QA', '0900000001', (SELECT id FROM customer_stage WHERE is_default = 1))`, acBranch).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	for _, id := range []int{1, 2, 3} {
		rev := 0
		for _, r := range revenueTypes {
			if r == id {
				rev = 1
			}
		}
		if err := testDB.Exec(`INSERT INTO appointment_type (id, name, isTypeRevenue) VALUES (?, ?, ?)`,
			id, "loai-"+string(rune('A'+id-1)), rev).Error; err != nil {
			t.Fatalf("seed appointment_type %d: %v", id, err)
		}
		if err := testDB.Exec(`INSERT INTO appointment
			(id, code, branch_id, customer_id, appointment_type_id, appointment_date, status)
			VALUES (?, ?, ?, 501, ?, 1788195700, 2)`,
			100+id, "AP-QA-"+string(rune('A'+id-1)), acBranch, id).Error; err != nil {
			t.Fatalf("seed appointment %d: %v", id, err)
		}
	}
}

func buildSQL(t *testing.T, rawQuery string) (string, []map[string]interface{}) {
	t.Helper()
	c := testhelpers.TenantCtx(acBranch, 99)
	c.Request = httptest.NewRequest("GET", "/?"+rawQuery, nil)

	q, err := buildAppointmentConsultationQuery(c, testDB)
	if err != nil {
		t.Fatalf("build query: %v", err)
	}

	var rows []map[string]interface{}
	dry := q.Session(&gorm.Session{DryRun: true}).Find(&rows)
	sql := dry.Statement.SQL.String()
	if strings.TrimSpace(sql) == "" {
		// Không có SQL nghĩa là phép kiểm dưới đây LUÔN xanh — đối chứng bắt buộc.
		t.Fatalf("DryRun không sinh ra SQL — phép kiểm join/subquery sẽ xanh giả")
	}

	items := make([]map[string]interface{}, 0)
	if tx := q.Session(&gorm.Session{}).Find(&items); pagination.IsQueryError(tx.Error) {
		t.Fatalf("chạy query thật: %v", tx.Error)
	}
	return sql, items
}

func TestAppointmentConsultation_LocDungLoaiTinhDoanhThu(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedConsultation(t, []int{1, 3})

	sql, items := buildSQL(t, "")
	if len(items) != 2 {
		t.Fatalf("phải trả đúng 2 lịch hẹn của loại isTypeRevenue=1, got %d (sql=%s)", len(items), sql)
	}
	for _, it := range items {
		id := it["appointment_type_id"]
		if id == nil {
			t.Fatalf("thiếu appointment_type_id trong kết quả")
		}
	}
	if strings.Contains(sql, "SELECT id FROM appointment_type") {
		t.Fatalf("vẫn còn subquery loại hẹn — bản vá bị revert: %s", sql)
	}
}

// Không có loại hẹn nào tính doanh thu ⇒ PHẢI trả 0 dòng.
// Nếu ai đó "tối ưu" bằng cách bỏ luôn điều kiện khi danh sách rỗng thì report sẽ
// trả TOÀN BỘ lịch hẹn — sai nghiệp vụ mà nhìn vẫn như đang chạy.
func TestAppointmentConsultation_KhongCoLoaiDoanhThu_TraRong(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedConsultation(t, nil)

	sql, items := buildSQL(t, "")
	if len(items) != 0 {
		t.Fatalf("không có loại hẹn isTypeRevenue=1 ⇒ phải rỗng, got %d (sql=%s)", len(items), sql)
	}
}

func TestAppointmentConsultation_JoinCustomer_ChiKhiExport(t *testing.T) {
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	seedConsultation(t, []int{1, 3})

	listSQL, _ := buildSQL(t, "")
	if strings.Contains(listSQL, "JOIN customer") {
		t.Fatalf("đường list KHÔNG được join customer (không đọc cột nào của nó): %s", listSQL)
	}

	exportSQL, _ := buildSQL(t, "_export=1")
	if !strings.Contains(exportSQL, "LEFT JOIN customer") {
		t.Fatalf("đường _export=1 BẮT BUỘC giữ join customer (ExportAppointment select customer.*): %s", exportSQL)
	}
}
