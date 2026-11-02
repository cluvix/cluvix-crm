package service

import (
	"sort"

	"gorm.io/gorm"

	"erpCluvix/internal/modules/webapp/crm/customer_detail/dto"
)

// TimelineDeps — các *gorm.DB đã được handler dựng sẵn theo từng bảng (Hard Rules: service không
// có *gin.Context, scope phải chọn ở handler — hiện là tenant.CrossBranchLookup),
// truyền vào BuildCustomerTimeline để build từng event builder.
type TimelineDeps struct {
	// DB — raw *gorm.DB, dùng cho bảng shared không tenant (appointment_type, supplier)
	// và helpers.FormatAmount (đọc tiền tệ thương hiệu qua pkg/brand).
	DB *gorm.DB

	AppointmentDB   *gorm.DB
	ServiceDB       *gorm.DB // "service" table — dùng cho Appointment builder (type_service_id lookup)
	TreatmentPlanDB *gorm.DB
	PaymentDB       *gorm.DB
	ReminderDB      *gorm.DB
	// StageHistoryDB — customer_stage_history: không có branch_id, đọc SAU khi khách cha đã qua
	// scope ở handler (architecture §Tenant scope).
	StageHistoryDB *gorm.DB

	CustomerID uint
	Customer   map[string]interface{}
}

// BuildCustomerTimeline gộp toàn bộ event builder thành 1 timeline, sort theo Day ASC.
//
// swallowed-sql-errors-sweep (story-04b): mỗi builder con nay trả kèm error (đọc `.Error` sau
// Find/Scan/Raw thủ công — trước đây bị nuốt, timeline hiển thị THIẾU nhóm sự kiện mà không ai biết
// là do lỗi SQL hay khách hàng thật sự không có dữ liệu nhóm đó). Gặp lỗi ở BẤT KỲ builder nào ⇒
// dừng ngay, trả lỗi cho handler xử lý qua `pagination.FailQuery` — không trả timeline THIẾU 1 phần
// coi như bình thường (đúng tinh thần epic: "hỏng" và "không có dữ liệu" phải phân biệt được).
func BuildCustomerTimeline(deps TimelineDeps) ([]dto.TimelineEvent, error) {
	var events []dto.TimelineEvent

	events = append(events, buildCustomerCreatedEvent(deps.Customer))

	apptEvents, err := buildAppointmentEvents(deps.AppointmentDB, deps.ServiceDB, deps.DB, deps.CustomerID)
	if err != nil {
		return nil, err
	}
	events = append(events, apptEvents...)

	planEvents, err := buildTreatmentPlanEvents(deps.TreatmentPlanDB, deps.DB, deps.CustomerID)
	if err != nil {
		return nil, err
	}
	events = append(events, planEvents...)

	events = append(events, buildPaymentEvents(deps.PaymentDB, deps.DB, deps.CustomerID)...)

	reminderEvents, err := buildReminderEvents(deps.ReminderDB, deps.CustomerID)
	if err != nil {
		return nil, err
	}
	events = append(events, reminderEvents...)

	stageEvents, err := buildStageChangeEvents(deps.StageHistoryDB, deps.CustomerID)
	if err != nil {
		return nil, err
	}
	events = append(events, stageEvents...)

	// SliceStable: sự kiện cùng giây giữ thứ tự builder (vd "Khởi tạo khách hàng" trước bước khởi tạo).
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Day < events[j].Day
	})
	return events, nil
}
