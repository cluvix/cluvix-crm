package service

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	appointmentDao "erpCluvix/internal/modules/webapp/crm/appointment/dao"
	apptRepo "erpCluvix/internal/modules/webapp/crm/appointment/repo"
	history "erpCluvix/internal/modules/webapp/history/log/service"
	"erpCluvix/pkg/codegen"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/optlock"
)

type AppointmentService struct {
	DB   *gorm.DB
	repo apptRepo.Reader
}

func NewAppointmentService(db *gorm.DB) *AppointmentService {
	return &AppointmentService{DB: db, repo: apptRepo.NewGormReader()}
}

// NewAppointmentServiceWithReader cho phép test inject fake Reader (no DB).
func NewAppointmentServiceWithReader(db *gorm.DB, r apptRepo.Reader) *AppointmentService {
	return &AppointmentService{DB: db, repo: r}
}

// --- Timeline helper ---
// addTimelineEvent: append 1 event vào noteTimeline JSON array.
// PHP equivalent: Appointment::addTimelineEvent(action, note, userId, time)
// noteTimeline lưu lịch sử hành động trên appointment (tạo, checkin, huỷ, xác nhận...)

type TimelineEvent struct {
	Action    string `json:"action"`
	Note      string `json:"note"`
	UserID    int    `json:"user_id"`
	Timestamp int64  `json:"timestamp"`
}

func addTimelineEvent(appt *appointmentDao.Appointment, action, note string, userID int) {
	var timeline []TimelineEvent
	if appt.NoteTimeline != nil {
		_ = json.Unmarshal(appt.NoteTimeline, &timeline)
	}
	timeline = append(timeline, TimelineEvent{
		Action:    action,
		Note:      note,
		UserID:    userID,
		Timestamp: time.Now().Unix(),
	})
	data, _ := json.Marshal(timeline)
	appt.NoteTimeline = data
}

// --- Create ---
// Flow: addTimeline → transaction { beforeSave → Create → afterSave(update customer) }
//
// beforeSave (insert):
//   - appointment_number = COUNT(cùng customer_id + appointment_type_id) + 1
//   - appointment_date_first = appointment_date (lưu ngày hẹn gốc, không đổi khi dời lịch)
//
// afterSave (insert):
//   - customer.last_contact_date = appointment.created_at
//   - customer.branch_id = appointment.branch_id (nếu customer chưa có branch)
//   - KHÔNG đổi tiến trình khách (status/stage_id) — chỉ đổi tay (customer-stage-pipeline story-06)

func (s *AppointmentService) Create(appt *appointmentDao.Appointment, userID int) error {
	if err := validateAppointmentCreate(appt); err != nil {
		return err
	}
	addTimelineEvent(appt, "Tạo mới lịch hẹn", appt.Content, userID)

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		appt.AppointmentDateFirst = appt.AppointmentDate

		// MAX+1 FOR UPDATE thay COUNT+1 để tránh race condition khi 2 request đồng thời
		var maxNum int
		tx.Raw(`SELECT COALESCE(MAX(appointment_number), 0) FROM appointment WHERE customer_id = ? AND appointment_type_id = ? FOR UPDATE`,
			appt.CustomerId, appt.AppointmentTypeId).Scan(&maxNum)
		appt.AppointmentNumber = new(maxNum + 1)

		// Lấy customer info 1 lần duy nhất — bao gồm code để quyết định gen không cần query thêm
		if appt.CustomerId != nil {
			var cust struct {
				BranchId *int   `gorm:"column:branch_id"`
				Code     string `gorm:"column:code"`
			}
			tx.Table("customer").Select("branch_id, COALESCE(code,'') as code").Where("id = ?", *appt.CustomerId).Take(&cust)

			uid := uint(userID)
			if err := tx.Set("userID", uid).Create(appt).Error; err != nil {
				return err
			}

			// afterSave: update customer — dùng created_at từ GORM đã set
			custUpdates := map[string]interface{}{
				"last_contact_date": appt.CreatedAt,
			}
			if cust.BranchId == nil && appt.BranchId != nil {
				custUpdates["branch_id"] = *appt.BranchId
			}
			// Gen mã nếu khách chưa có mã — theo chi nhánh CỦA KHÁCH (chi nhánh lịch hẹn chỉ dùng khi
			// khách chưa có, tức vừa được gán ở trên). Không chi nhánh nào ⇒ chưa có mã.
			if cust.Code == "" {
				custBranchID := cust.BranchId
				if custBranchID == nil {
					custBranchID = appt.BranchId
				}
				if custBranchID != nil {
					if code := codegen.NewCustomerCode(tx, int64(*custBranchID)); code != "" {
						custUpdates["code"] = code
					}
				}
			}
			tx.Table("customer").Where("id = ?", *appt.CustomerId).Updates(custUpdates)
		} else {
			uid := uint(userID)
			if err := tx.Set("userID", uid).Create(appt).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// --- Update ---
// Flow: optimistic lock check → transaction { snapshot → merge → beforeSave → Save → log → afterSave }
//
// beforeSave (update):
//   - Optimistic locking: so sánh version FE gửi vs DB, reject nếu khác
//   - version += 1
//   - Nếu appointment_date thay đổi → append {old, new} vào appointmentListDate JSON array
//
// afterSave (update):
//   - Log changes: so sánh oldSnap vs newSnap cho AppointmentIncludeFields → insert log_appointment
//   - Cập nhật ngày liên hệ/ghé thăm của khách theo appointment status (xem updateCustomerContactDates)
//   - Nếu appointment_type_id thay đổi → sort lại appointment_number cho cả type cũ và type mới
//
// Merge pattern: FE gửi partial JSON → giữ current DB làm base, chỉ ghi đè fields non-zero từ FE
// Tránh GORM Save() xóa sạch fields mà FE không gửi

func (s *AppointmentService) Update(appt *appointmentDao.Appointment, userID int, scopedDB *gorm.DB) error {
	if err := validateAppointmentUpdate(appt); err != nil {
		return err
	}
	// scopedDB là chained instance — First trực tiếp sẽ mutate statement (id + ORDER + LIMIT),
	// và Transaction từ scopedDB leak tenant scope (appointment.company_id) vào MỌI query trong tx,
	// kể cả bảng khác (customer, config_code) → Error 1054. Vì vậy: First qua Session clone,
	// Transaction từ s.DB sạch — ownership đã verify bằng First scoped + verifyAppointmentOwnership ở handler.
	var current appointmentDao.Appointment
	if err := scopedDB.Session(&gorm.Session{}).First(&current, appt.ID).Error; err != nil {
		return errs.ErrNotFound
	}

	if appt.UpdatedAt != current.UpdatedAt { // khoá lạc quan theo updated_at (D6)
		return errs.ErrVersionConflict
	}

	oldAppointmentTypeId := current.AppointmentTypeId

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// Merge: giữ current làm base, ghi đè bằng fields FE gửi (pure, không đụng DB)
		merged := current
		mergeAppointment(&merged, appt)

		// Khoá nhóm (customer_id, appointment_type_id) TRƯỚC khi khoá row đơn lẻ (Save bên dưới) —
		// thứ tự cha(nhóm)→con(row) nhất quán, tránh deadlock 1213 với sortAppointment khi 2 request
		// sửa/xoá đồng thời appointment cùng customer (PUT /update-appointment, 2026-09-19).
		lockAppointmentGroups(tx, merged.CustomerId, oldAppointmentTypeId, merged.AppointmentTypeId)

		// Snapshot bên trong transaction để tránh stale read
		oldSnap := history.SnapshotRow(tx, "appointment", appt.ID, history.AppointmentIncludeFields)

		// Track appointmentListDate
		if merged.AppointmentDate != nil && current.AppointmentDate != nil && *merged.AppointmentDate != *current.AppointmentDate {
			var listDate []map[string]interface{}
			if current.AppointmentListDate != nil {
				_ = json.Unmarshal(current.AppointmentListDate, &listDate)
			}
			listDate = append(listDate, map[string]interface{}{
				"old": *current.AppointmentDate,
				"new": *merged.AppointmentDate,
			})
			data, _ := json.Marshal(listDate)
			merged.AppointmentListDate = data
		}

		uid := uint(userID)
		if err := optlock.Save(tx.Session(&gorm.Session{FullSaveAssociations: false}).Set("userID", uid), &merged, current.UpdatedAt); err != nil {
			return err
		}

		// Log changes
		newSnap := history.SnapshotRow(tx, "appointment", merged.ID, history.AppointmentIncludeFields)
		if err := history.LogChanges(tx, "log_appointment", "appointment_id", int(merged.ID), oldSnap, newSnap, history.AppointmentIncludeFields, history.AppointmentSkipNullFields, userID); err != nil {
			return err
		}

		// Load customer một lần — dùng cho branch/code sync.
		var custInfo struct {
			BranchId *int   `gorm:"column:branch_id"`
			Code     string `gorm:"column:code"`
		}
		if merged.CustomerId != nil {
			tx.Table("customer").
				Select("branch_id, COALESCE(code,'') as code").
				Where("id = ?", *merged.CustomerId).Scan(&custInfo)
		}

		s.updateCustomerContactDates(tx, &merged)

		// Sync branch + code: chỉ thực hiện khi lịch hẹn có chi nhánh.
		// custInfo đã load sẵn → 0 extra SELECT dù customer đã có code hay chưa.
		if merged.BranchId != nil && merged.CustomerId != nil {
			custUpdates := map[string]interface{}{}
			if custInfo.BranchId == nil {
				custUpdates["branch_id"] = *merged.BranchId
			}
			if custInfo.Code == "" {
				branchID := *merged.BranchId
				if custInfo.BranchId != nil {
					branchID = *custInfo.BranchId
				}
				if code := codegen.NewCustomerCode(tx, int64(branchID)); code != "" {
					custUpdates["code"] = code
				}
			}
			if len(custUpdates) > 0 {
				tx.Table("customer").Where("id = ?", *merged.CustomerId).Updates(custUpdates)
			}
		}

		// Sort nếu appointment_type_id thay đổi
		if oldAppointmentTypeId != nil && merged.AppointmentTypeId != nil && *oldAppointmentTypeId != *merged.AppointmentTypeId {
			s.sortAppointment(tx, merged.CustomerId, oldAppointmentTypeId)
			s.sortAppointment(tx, merged.CustomerId, merged.AppointmentTypeId)
		}

		// Copy merged back cho handler trả response
		*appt = merged
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// --- Checkin ---
// Flow giống Update, thêm:
//   - addTimeline "Khách check-in" với noteAppointment
//   - FE gửi status=2 (CHECKED_IN), checkin_date
//   - Merge current trước addTimeline để giữ noteTimeline cũ (append, không ghi đè)

// checkinClearableFields — field GIÁ TRỊ của form tiếp nhận được phép xoá về rỗng khi client gửi
// key kèm null/"". Chỉ gồm ghi chú; KHÔNG gồm `checkin_date`/
// `checkout_date` (xử lý riêng bên dưới vì có cascade) và KHÔNG gồm `is_accepted`
// (chỉ bị xoá theo cascade của checkin/checkout, xoá độc lập là câu hỏi nghiệp vụ khác).
var checkinClearableFields = map[string]func(*appointmentDao.Appointment){
	"noteAppointment": func(a *appointmentDao.Appointment) { a.NoteAppointment = "" },
}

// Checkin — `explicitClears` = tập key CÓ MẶT trong body với giá trị null/"" (xem CustomerCheckin).
// Client gửi map rỗng (mobile) ⇒ giữ nguyên hành vi merge-bỏ-qua-rỗng như cũ.
func (s *AppointmentService) Checkin(appt *appointmentDao.Appointment, explicitClears map[string]bool, userID int, scopedDB *gorm.DB) error {
	var current appointmentDao.Appointment
	if err := scopedDB.Session(&gorm.Session{}).First(&current, appt.ID).Error; err != nil {
		return errs.ErrNotFound
	}

	if appt.UpdatedAt != current.UpdatedAt { // khoá lạc quan theo updated_at (D6)
		return errs.ErrVersionConflict
	}

	// Merge trước khi addTimeline để giữ noteTimeline cũ
	merged := current
	mergeAppointment(&merged, appt)

	// Apply explicit clears — FE gửi null/"" có chủ đích (vd: xoá checkin_date, xoá ô huyết áp).
	// mergeAppointment bỏ qua field nil/"", nên phải apply riêng sau merge.
	for key, clear := range checkinClearableFields {
		if explicitClears[key] {
			clear(&merged)
		}
	}
	if explicitClears["checkin_date"] {
		merged.CheckinDate = nil
		// Rollback về chưa checkin → xóa cascade toàn bộ dữ liệu phát sinh sau checkin:
		// kết quả tư vấn và checkout.
		merged.CheckoutDate = nil
		merged.IsAccepted = nil
		merged.TypeServiceAcceptedId = nil
	}
	if explicitClears["checkout_date"] {
		merged.CheckoutDate = nil
		// Rollback về chưa checkout → xóa kết quả tư vấn của visit đó.
		merged.IsAccepted = nil
		merged.TypeServiceAcceptedId = nil
	}

	// FE không gửi status — derive từ checkin/checkout date.
	// checkout_date có → CheckedOut; chỉ có checkin_date → CheckedIn; cả 2 nil → NotCheckin.
	if appt.Status == nil {
		if merged.CheckoutDate != nil {
			merged.Status = new(define.AppointmentStatusCheckedOut)
		} else if merged.CheckinDate != nil {
			merged.Status = new(define.AppointmentStatusCheckedIn)
		} else {
			merged.Status = new(define.AppointmentStatusNotCheckin)
		}
	}

	timelineLabel := "Khách check-in"
	if merged.Status != nil && *merged.Status == define.AppointmentStatusCheckedOut {
		timelineLabel = "Khách check-out"
	}
	addTimelineEvent(&merged, timelineLabel, merged.NoteAppointment, userID)

	// Transaction từ DB sạch — scoped tx leak scope sang bảng khác gây 1054 (xem comment ở Update)
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		oldSnap := history.SnapshotRow(tx, "appointment", merged.ID, history.AppointmentIncludeFields)

		uid := uint(userID)
		if err := optlock.Save(tx.Session(&gorm.Session{FullSaveAssociations: false}).Set("userID", uid), &merged, current.UpdatedAt); err != nil {
			return err
		}

		newSnap := history.SnapshotRow(tx, "appointment", merged.ID, history.AppointmentIncludeFields)
		if err := history.LogChanges(tx, "log_appointment", "appointment_id", int(merged.ID), oldSnap, newSnap, history.AppointmentIncludeFields, history.AppointmentSkipNullFields, userID); err != nil {
			return err
		}

		s.updateCustomerContactDates(tx, &merged)

		*appt = merged
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// --- Cancel ---
// Flow: merge → force status=CANCEL → addTimeline → transaction { Save → log }
// Luôn force status = STATUS_CANCEL bất kể FE gửi gì
// KHÔNG đụng khách: không đổi tiến trình (customer-stage-pipeline story-06), và (như trước story-06)
// không cập nhật last_contact_date — đường huỷ chưa từng gọi updateCustomerContactDates.

func (s *AppointmentService) Cancel(appt *appointmentDao.Appointment, userID int, scopedDB *gorm.DB) error {
	var current appointmentDao.Appointment
	if err := scopedDB.Session(&gorm.Session{}).First(&current, appt.ID).Error; err != nil {
		return errs.ErrNotFound
	}

	if appt.UpdatedAt != current.UpdatedAt { // khoá lạc quan theo updated_at (D6)
		return errs.ErrVersionConflict
	}

	merged := current
	mergeAppointment(&merged, appt)
	merged.Status = new(define.AppointmentStatusCancel)
	addTimelineEvent(&merged, "Huỷ lịch hẹn", merged.NoteAppointment, userID)

	// Transaction từ DB sạch — scoped tx leak scope sang bảng khác gây 1054 (xem comment ở Update)
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		oldSnap := history.SnapshotRow(tx, "appointment", merged.ID, history.AppointmentIncludeFields)

		uid := uint(userID)
		if err := optlock.Save(tx.Session(&gorm.Session{FullSaveAssociations: false}).Set("userID", uid), &merged, current.UpdatedAt); err != nil {
			return err
		}

		newSnap := history.SnapshotRow(tx, "appointment", merged.ID, history.AppointmentIncludeFields)
		if err := history.LogChanges(tx, "log_appointment", "appointment_id", int(merged.ID), oldSnap, newSnap, history.AppointmentIncludeFields, history.AppointmentSkipNullFields, userID); err != nil {
			return err
		}

		*appt = merged
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// --- UpdateConfirm ---
// Endpoint nhẹ: chỉ update confirmAppointment (0/1/2) + append timeline
// confirmAppointment: 0 = Chưa xác nhận, 1 = Xác nhận khách đến, 2 = Xác nhận khách không đến
// Không trigger beforeSave/afterSave phức tạp, không log changes, không update customer status

func (s *AppointmentService) UpdateConfirm(appt *appointmentDao.Appointment, userID int, scopedDB *gorm.DB) error {
	var current appointmentDao.Appointment
	if err := scopedDB.Session(&gorm.Session{}).First(&current, appt.ID).Error; err != nil {
		return errs.ErrNotFound
	}

	if appt.UpdatedAt != current.UpdatedAt { // khoá lạc quan theo updated_at (D6)
		return errs.ErrVersionConflict
	}

	// Chỉ update confirmAppointment + noteTimeline, giữ nguyên record cũ
	merged := current
	merged.ConfirmAppointment = appt.ConfirmAppointment

	var mess string
	if merged.ConfirmAppointment != nil {
		switch *merged.ConfirmAppointment {
		case 1:
			mess = "Xác nhận khách đến"
		case 2:
			mess = "Xác nhận khách không đến"
		default:
			mess = "Chưa xác nhận"
		}
	}
	addTimelineEvent(&merged, mess, appt.NoteAppointment, userID)

	uid := uint(userID)
	if err := optlock.Save(scopedDB.Session(&gorm.Session{FullSaveAssociations: false}).Set("userID", uid), &merged, current.UpdatedAt); err != nil {
		return err
	}

	*appt = merged
	return nil
}

// AddNote appends a staff note to noteTimeline.
// Append atomic bằng JSON_ARRAY_APPEND ngay trong 1 UPDATE — pattern cũ (First → append in-app →
// UpdateColumn) là read-modify-write không có version guard: 2 request đồng thời (AddNote//AddNote
// hoặc AddNote//Update) làm mất event của nhau.
func (s *AppointmentService) AddNote(id uint, note string, userID int) error {
	event, err := json.Marshal(TimelineEvent{
		Action:    "Ghi chú",
		Note:      note,
		UserID:    userID,
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		return err
	}
	res := s.DB.Exec(
		"UPDATE appointment SET noteTimeline = JSON_ARRAY_APPEND(COALESCE(noteTimeline, JSON_ARRAY()), '$', CAST(? AS JSON)), "+optlock.RawSet+" WHERE id = ?",
		string(event), id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errs.ErrNotFound
	}
	return nil
}

// --- Delete ---
// Flow: find → transaction { beforeDelete → Delete → afterDelete }
//
// beforeDelete:
//   - Xóa log_appointment cũ → insert 1 record log với action_type = DELETE (snapshot id/code/
//     customer_id + company_id; FK ON DELETE SET NULL giữ dòng này lại sau khi xoá lịch hẹn)
//   - Xóa appointment_reason liên quan
//
// afterDelete:
//   - Sort lại appointment_number cho các appointment còn lại cùng customer + type

func (s *AppointmentService) Delete(id uint, userID int, scopedDB *gorm.DB) error {
	var appt appointmentDao.Appointment
	if err := scopedDB.Session(&gorm.Session{}).First(&appt, id).Error; err != nil {
		return errs.ErrNotFound
	}

	// Transaction từ DB sạch — scoped tx leak scope sang bảng khác gây 1054 (xem comment ở Update)
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// Khoá nhóm (customer_id, appointment_type_id) TRƯỚC khi xoá row đơn lẻ — cùng lý do
		// lock-order với Update() (xem comment ở đó).
		lockAppointmentGroups(tx, appt.CustomerId, appt.AppointmentTypeId)

		// Log cũ bị dọn (sau khi xoá, appointment_id NULL ⇒ không còn biết của lịch hẹn nào); dòng
		// log XOÁ ghi SAU lệnh dọn nên sống sót.
		if err := tx.Exec("DELETE FROM log_appointment WHERE appointment_id = ?", id).Error; err != nil {
			return fmt.Errorf("delete log_appointment: %w", err)
		}
		if err := history.LogDeleteSnapshot(tx, "log_appointment", int(id), history.AppointmentDeleteFields, userID); err != nil {
			return err
		}
		// Unlink trước khi xóa để tránh FK constraint
		if err := tx.Exec("UPDATE order_service_used SET appointment_id = NULL, "+optlock.RawSet+" WHERE appointment_id = ?", id).Error; err != nil {
			return fmt.Errorf("unlink order_service_used: %w", err)
		}
		if err := tx.Exec("UPDATE order_plan SET appointment_id = NULL, "+optlock.RawSet+" WHERE appointment_id = ?", id).Error; err != nil {
			return fmt.Errorf("unlink order_plan: %w", err)
		}

		if err := tx.Delete(&appt).Error; err != nil {
			return err
		}

		s.sortAppointment(tx, appt.CustomerId, appt.AppointmentTypeId)

		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// --- updateCustomerContactDates ---
// Ghi ngày liên hệ/ghé thăm của khách theo trạng thái lịch hẹn (Update/Checkin gọi):
//   - CHECKED_IN(2)  → last_visit_date = last_contact_date = checkin_date (thiếu ngày ⇒ last_contact_date = now)
//   - CHECKED_OUT(3) → last_visit_date = last_contact_date = checkout_date (thiếu ngày ⇒ last_contact_date = now)
//   - NOT_CHECKIN(1)/CANCEL(4) → last_contact_date = now
//
// KHÔNG đổi tiến trình khách (status/stage_id/accepted) và KHÔNG ghi customer_stage_history —
// tiến trình chỉ đổi tay (customer-stage-pipeline story-06 gỡ phần map trạng thái lịch hẹn +
// is_accepted sang tiến trình cũ/accepted của khách).
func (s *AppointmentService) updateCustomerContactDates(tx *gorm.DB, appt *appointmentDao.Appointment) {
	if appt.CustomerId == nil || appt.Status == nil {
		return
	}

	updates := map[string]interface{}{}

	// last_contact_date: mọi tương tác với lịch hẹn (update/checkin/checkout)
	// đều là một lần liên hệ với khách → luôn ghi nhận. Dùng now() cho operation,
	// dùng checkin_date/checkout_date cho event thật khi khách đến.
	// last_visit_date: chỉ update khi khách thực sự ghé (checkin/checkout).
	now := time.Now().Unix()
	switch *appt.Status {
	case define.AppointmentStatusCheckedIn:
		if appt.CheckinDate != nil {
			updates["last_visit_date"] = *appt.CheckinDate
			updates["last_contact_date"] = *appt.CheckinDate
		} else {
			updates["last_contact_date"] = now
		}
	case define.AppointmentStatusCheckedOut:
		if appt.CheckoutDate != nil {
			updates["last_visit_date"] = *appt.CheckoutDate
			updates["last_contact_date"] = *appt.CheckoutDate
		} else {
			updates["last_contact_date"] = now
		}
	case define.AppointmentStatusNotCheckin, define.AppointmentStatusCancel:
		// Update lịch (dời/sửa) = nhân sự liên hệ khách.
		updates["last_contact_date"] = now
	}

	if len(updates) > 0 {
		tx.Table("customer").Where("id = ?", *appt.CustomerId).Updates(updates)
	}
}

// --- sortAppointment ---
// Đánh lại appointment_number cho tất cả appointments cùng customer_id + appointment_type_id
// Sắp xếp theo appointment_date ASC, dùng ROW_NUMBER()
// Dùng subquery UPDATE thay vì temp table để tránh collision khi gọi 2 lần liên tiếp
// (VD: khi appointment_type_id thay đổi → sort cả type cũ và type mới)

// lockAppointmentGroups khoá TRƯỚC (SELECT ... FOR UPDATE, thứ tự id tăng dần) toàn bộ row
// appointment thuộc các nhóm (customer_id, appointment_type_id) mà sortAppointment sẽ renumber
// trong CÙNG transaction. Phải gọi TRƯỚC khi khoá/update row đơn lẻ (tx.Save/tx.Delete) để giữ
// thứ tự khoá nhất quán cha(nhóm)→con(row) — khoá con trước rồi mới nhóm là nguyên nhân deadlock
// MySQL 1213 khi 2 request sửa/xoá đồng thời 2 appointment cùng customer (ROW_NUMBER() OVER
// UPDATE...JOIN của sortAppointment không tự khoá nhóm trước khi chạy).
func lockAppointmentGroups(tx *gorm.DB, customerID *int, appointmentTypeIDs ...*int) {
	if customerID == nil {
		return
	}
	seen := map[int]bool{}
	types := make([]int, 0, len(appointmentTypeIDs))
	for _, t := range appointmentTypeIDs {
		if t == nil || seen[*t] {
			continue
		}
		seen[*t] = true
		types = append(types, *t)
	}
	if len(types) == 0 {
		return
	}
	var ids []int
	tx.Raw(`SELECT id FROM appointment WHERE customer_id = ? AND appointment_type_id IN (?) ORDER BY id FOR UPDATE`,
		*customerID, types).Scan(&ids)
}

func (s *AppointmentService) sortAppointment(tx *gorm.DB, customerID, appointmentTypeID *int) {
	if customerID == nil || appointmentTypeID == nil {
		return
	}
	// Dùng subquery UPDATE trực tiếp, tránh temp table collision khi gọi 2 lần liên tiếp
	tx.Exec(`
		UPDATE appointment a
		INNER JOIN (
			SELECT id, ROW_NUMBER() OVER (ORDER BY appointment_date ASC) AS new_number
			FROM appointment
			WHERE customer_id = ? AND appointment_type_id = ?
		) t ON a.id = t.id
		SET a.appointment_number = t.new_number, `+optlock.RawSetFor("a")+`
		WHERE NOT (a.appointment_number <=> t.new_number)
	`, *customerID, *appointmentTypeID)
}

// --- mergeAppointment ---
// Giải quyết vấn đề: FE gửi partial JSON → ShouldBindJSON vào struct → fields không gửi = zero-value
// Nếu Save() trực tiếp → zero-value sẽ xóa sạch data cũ trong DB
// Solution: load current từ DB làm base (dst), chỉ ghi đè fields mà FE thực sự gửi (non-zero từ src)
// Pointer fields: nil = FE không gửi → giữ nguyên; non-nil = FE gửi → ghi đè
// String fields: "" = FE không gửi → giữ nguyên; non-empty = FE gửi → ghi đè

func mergeAppointment(dst, src *appointmentDao.Appointment) {
	if src.CustomerId != nil {
		dst.CustomerId = src.CustomerId
	}
	if src.AppointmentTypeId != nil {
		dst.AppointmentTypeId = src.AppointmentTypeId
	}
	if src.BranchId != nil {
		dst.BranchId = src.BranchId
	}
	if src.AppointmentDate != nil {
		dst.AppointmentDate = src.AppointmentDate
	}
	if src.ScheduleTime != nil {
		dst.ScheduleTime = src.ScheduleTime
	}
	if src.CheckinDate != nil {
		dst.CheckinDate = src.CheckinDate
	}
	if src.CheckoutDate != nil {
		dst.CheckoutDate = src.CheckoutDate
	}
	if src.IsAccepted != nil {
		dst.IsAccepted = src.IsAccepted
	}
	if src.ConfirmAppointment != nil {
		dst.ConfirmAppointment = src.ConfirmAppointment
	}
	if src.RevenueId != nil {
		dst.RevenueId = src.RevenueId
	}
	if src.ConvertId != nil {
		dst.ConvertId = src.ConvertId
	}
	if src.TypeServiceId != nil {
		dst.TypeServiceId = src.TypeServiceId
	}
	if src.OrderServiceUsedId != nil {
		dst.OrderServiceUsedId = src.OrderServiceUsedId
	}
	if src.ServiceId != nil {
		dst.ServiceId = src.ServiceId
	}
	if src.SourceId != nil {
		dst.SourceId = src.SourceId
	}
	if src.TypeServiceAcceptedId != nil {
		dst.TypeServiceAcceptedId = src.TypeServiceAcceptedId
	}
	if src.AppointmentReasonId != nil {
		dst.AppointmentReasonId = src.AppointmentReasonId
	}
	if src.NoteAppointment != "" {
		dst.NoteAppointment = src.NoteAppointment
	}
	if src.BookType != nil {
		dst.BookType = src.BookType
	}
	if src.Content != "" {
		dst.Content = src.Content
	}
	if src.ContentPrivate != "" {
		dst.ContentPrivate = src.ContentPrivate
	}
	if src.Status != nil {
		dst.Status = src.Status
	}
}
