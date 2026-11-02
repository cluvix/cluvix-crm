package dao

import (
	"erpCluvix/internal/models"

	"gorm.io/datatypes"
)

type Appointment struct {
	models.BaseModelWithCode
	CustomerId           *int   `gorm:"column:customer_id;index" json:"customer_id" form:"customer_id"`                            // FK → customer
	AppointmentTypeId    *int   `gorm:"column:appointment_type_id;index" json:"appointment_type_id" form:"appointment_type_id"`    // FK → appointment_type (loại lịch hẹn)
	BranchId             *int   `gorm:"column:branch_id;index" json:"branch_id" form:"branch_id"`                                  // FK → branch (chi nhánh)
	AppointmentDateFirst *int64 `gorm:"column:appointment_date_first" json:"appointment_date_first" form:"appointment_date_first"` // Ngày ghi nhận hẹn đầu tiên (timestamp)
	AppointmentDate      *int64 `gorm:"column:appointment_date;index" json:"appointment_date" form:"appointment_date"`             // Thời gian hẹn hiện tại (timestamp)
	// Lịch sử thay đổi ngày hẹn — JSON array object [{"old": <unix>, "new": <unix>}]
	// (service.Update ghi; KHÔNG phải array chuỗi "04-10-2025 -> 05-01-2026" như data cũ từ PHP).
	// Mỗi lần đổi appointment_date thêm 1 entry. Nhiều entry = khách không ổn định, có thể hủy.
	AppointmentListDate datatypes.JSON `gorm:"column:appointmentListDate;type:json" json:"appointmentListDate"`
	ScheduleTime        *int           `gorm:"column:schedule_time" json:"schedule_time" form:"schedule_time"`                // Thời gian dự kiến của lịch hẹn (phút)
	AppointmentNumber   *int           `gorm:"column:appointment_number" json:"appointment_number" form:"appointment_number"` // Tổng số lần hẹn (cộng dồn sau mỗi lần đặt)
	CheckinDate         *int64         `gorm:"column:checkin_date" json:"checkin_date" form:"checkin_date"`                   // Thời gian khách đến (timestamp)
	CheckoutDate        *int64         `gorm:"column:checkout_date" json:"checkout_date" form:"checkout_date"`                // Thời gian khách về (timestamp)
	// --- Nhân sự phụ trách ---
	RevenueId *int `gorm:"column:revenue_id;index" json:"revenue_id" form:"revenue_id"` // FK → user (người tư vấn doanh thu)
	ConvertId *int `gorm:"column:convert_id;index" json:"convert_id" form:"convert_id"` // FK → user (người chuyển đổi)
	// --- Dịch vụ & nguồn ---
	ServiceId             *int `gorm:"column:service_id;index" json:"service_id" form:"service_id"`                                           // FK → service (dịch vụ hẹn)
	TypeServiceId         *int `gorm:"column:type_service_id;index" json:"type_service_id" form:"type_service_id"`                            // FK → service_type (nhóm dịch vụ quan tâm)
	TypeServiceAcceptedId *int `gorm:"column:type_service_accepted_id;index" json:"type_service_accepted_id" form:"type_service_accepted_id"` // FK → service_type (nhóm dịch vụ đồng ý)
	OrderServiceUsedId    *int `gorm:"column:order_service_used_id;index" json:"order_service_used_id" form:"order_service_used_id"`          // FK → order_service (dịch vụ đang điều trị)
	SourceId              *int `gorm:"column:source_id;index" json:"source_id" form:"source_id"`                                              // FK → source (nguồn khách theo lịch hẹn)
	AppointmentReasonId   *int `gorm:"column:appointment_reason_id;index" json:"appointment_reason_id" form:"appointment_reason_id"`          // FK → appointment_reason (lý do hẹn)
	// Khách đồng ý làm dịch vụ sau tư vấn: nil=chưa tư vấn, 1=đồng ý, 2=không đồng ý.
	// is_accepted=2 cần win-back campaign.
	IsAccepted         *int `gorm:"column:is_accepted;index" json:"is_accepted" form:"is_accepted"`
	ConfirmAppointment *int `gorm:"column:confirmAppointment;default:0" json:"confirmAppointment" form:"confirmAppointment"` // Xác nhận lịch hẹn (0=chưa xác nhận)
	// Phân loại lịch hẹn: 1=Đặt trước, 2=Phát sinh, 3=Dự kiến
	BookType        *int   `gorm:"column:bookType" json:"bookType" form:"bookType"`
	Content         string `gorm:"column:content" json:"content" form:"content"`                         // Nội dung lịch hẹn (HTML: THÔNG TIN, TÌNH TRẠNG, MONG MUỐN, ĐIỂM ĐAU, CHI PHÍ, ORDER BÁC SỸ, LƯU Ý)
	ContentPrivate  string `gorm:"column:contentPrivate" json:"contentPrivate" form:"contentPrivate"`    // Nội dung nội bộ (không hiển thị cho khách)
	NoteAppointment string `gorm:"column:noteAppointment" json:"noteAppointment" form:"noteAppointment"` // Ghi chú lịch hẹn
	// Timeline tương tác — JSON array [{action, note, user_id, timestamp}].
	// Ghi mọi sự kiện: created, updated, cancelled, checked_in, checked_out.
	NoteTimeline datatypes.JSON `gorm:"column:noteTimeline;type:json" json:"noteTimeline"`
	// Tình trạng: 1=Chưa đến, 2=Đã đến, 3=Hoàn thành, 4=Hủy, 5=Đang tư vấn, 6=Đang phục vụ.
	// Chỉ tính status 2,3 cho engagement. Status 4 = dấu hiệu không hài lòng.
	Status *int `gorm:"default:1" json:"status" form:"status"`
}

func (Appointment) TableName() string { return "appointment" }
