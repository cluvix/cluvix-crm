//go:build integration

// Epic swallowed-sql-errors-sweep, story-04b (cụm `crm/customer_detail`) — lưới FR5.
//
// Ca đặc thù của cụm này (khác mọi cụm trước): `BuildCustomerTimeline`
// (`service/timeline.go`) gộp nhiều event-builder khác nhau (lịch hẹn, đơn hàng, điều trị, nhắc
// hẹn, cuộc gọi...) thành MỘT timeline (story-11/13 crm-standalone bỏ builder chăm sóc/khiếu
// nại/labo/bảo hành/khám nha khoa/khám tổng quát). Trước story-04b mỗi builder tự nuốt lỗi SQL của
// mình — 1 nhóm lỗi chỉ làm nhóm đó rỗng
// trên UI, các nhóm khác vẫn hiển thị, không ai biết có lỗi. story-04b đổi TẤT CẢ 12 builder có
// query thủ công sang trả `(events, error)`; `BuildCustomerTimeline` dừng ngay khi builder ĐẦU TIÊN
// lỗi và trả lỗi lên handler — quyết định có chủ đích: 1 nhóm lỗi ⇒ CẢ timeline 500, không trả
// timeline thiếu 1 phần coi như bình thường (đúng tinh thần epic, xem comment tại
// `BuildCustomerTimeline`). Lưới này khẳng định đúng hành vi đó bằng cách gọi thẳng
// `BuildCustomerTimeline` với 1 DB cố tình lỗi ở builder giữa chừng (buildAppointmentEvents).
package contracttest

import (
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"erpCluvix/internal/modules/webapp/crm/customer_detail/dto"
	customerDetailSvc "erpCluvix/internal/modules/webapp/crm/customer_detail/service"
	"erpCluvix/pkg/pagination"
)

// brokenDB — *gorm.DB trỏ tới cổng TCP không ai lắng nghe (connection refused gần như ngay lập
// tức, không cần chờ timeout DNS). Bất kỳ query nào qua nó đều lỗi thật — dùng để ép 1 builder cụ
// thể lỗi mà KHÔNG cần bảng/cột giả (builder tự gọi `.Table(...)`/`.Select(...)` đè lên bất kỳ giá
// trị nào set sẵn trên *gorm.DB truyền vào, nên không thể ép lỗi bằng cách pre-config Table/Select
// từ bên ngoài — phải ép ở tầng kết nối).
func brokenDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "root:x@tcp(127.0.0.1:1)/x?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true, // bỏ probe `SELECT VERSION()` lúc Open — tránh connect sớm
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open (không nên lỗi ở bước mở — kết nối thật chỉ xảy ra lúc query): %v", err)
	}
	return db
}

func TestCrmCustomerDetail_TimelineOneBuilderErrorFailsWholeTimeline(t *testing.T) {
	if testDB == nil {
		t.Skip("testDB nil — TEST_MYSQL_DSN chưa set")
	}

	// appointment dùng DB kết nối hỏng (connection refused) — builder truy vấn ĐẦU TIÊN lỗi (story-11
	// crm-standalone bỏ 2 builder chăm sóc/khiếu nại đứng trước); các builder sau không được chạy tiếp.
	deps := customerDetailSvc.TimelineDeps{
		DB:              testDB,
		AppointmentDB:   brokenDB(t),
		ServiceDB:       testDB,
		TreatmentPlanDB: testDB,
		PaymentDB:       testDB,
		ReminderDB:      testDB,
		StageHistoryDB:  testDB, // story-04 customer-stage-pipeline: builder chuyển bước
		CustomerID:      1,
		Customer:        map[string]interface{}{},
	}

	events, err := customerDetailSvc.BuildCustomerTimeline(deps)

	if !pagination.IsQueryError(err) {
		t.Fatalf("builder appointment lỗi kết nối nhưng BuildCustomerTimeline không trả error — lỗi bị nuốt, err=%v", err)
	}
	if events != nil {
		t.Fatalf("khi có lỗi, events phải nil (KHÔNG trả timeline THIẾU 1 phần coi như bình thường), nhận %d events", len(events))
	}
}

// TestCrmCustomerDetail_TimelineValidStaysOK — canh regression: mọi builder chạy đúng (bảng tồn
// tại, không có dữ liệu) vẫn phải trả timeline OK (rỗng), không lỗi.
func TestCrmCustomerDetail_TimelineValidStaysOK(t *testing.T) {
	if testDB == nil {
		t.Skip("testDB nil — TEST_MYSQL_DSN chưa set")
	}

	deps := customerDetailSvc.TimelineDeps{
		DB:              testDB,
		AppointmentDB:   testDB,
		ServiceDB:       testDB,
		TreatmentPlanDB: testDB,
		PaymentDB:       testDB,
		ReminderDB:      testDB,
		StageHistoryDB:  testDB, // story-04 customer-stage-pipeline: builder chuyển bước
		CustomerID:      999999,
		Customer:        map[string]interface{}{},
	}

	events, err := customerDetailSvc.BuildCustomerTimeline(deps)
	if err != nil {
		t.Fatalf("mọi bảng hợp lệ, customer không có dữ liệu — phải KHÔNG lỗi, nhận err=%v", err)
	}
	if events == nil {
		t.Fatalf("events phải là slice rỗng (không nil) khi build thành công, dù 0 sự kiện")
	}
	_ = dto.TimelineEvent{}
}

// Story-04 customer-stage-pipeline (QA pass 1, P2): builder chuyển bước (`customer_stage_history`, chạy
// CUỐI) lỗi SQL ⇒ cả timeline phải trả lỗi — không được nuốt rồi trả timeline thiếu nhóm "chuyển bước".
func TestCrmCustomerDetail_TimelineStageHistoryErrorFailsWholeTimeline(t *testing.T) {
	if testDB == nil {
		t.Skip("testDB nil — TEST_MYSQL_DSN chưa set")
	}

	deps := customerDetailSvc.TimelineDeps{
		DB:              testDB,
		AppointmentDB:   testDB,
		ServiceDB:       testDB,
		TreatmentPlanDB: testDB,
		PaymentDB:       testDB,
		ReminderDB:      testDB,
		StageHistoryDB:  brokenDB(t),
		CustomerID:      999999,
		Customer:        map[string]interface{}{},
	}

	events, err := customerDetailSvc.BuildCustomerTimeline(deps)

	if !pagination.IsQueryError(err) {
		t.Fatalf("builder chuyển bước lỗi kết nối nhưng BuildCustomerTimeline không trả error — lỗi bị nuốt, err=%v", err)
	}
	if events != nil {
		t.Fatalf("khi có lỗi, events phải nil, nhận %d events", len(events))
	}
}
