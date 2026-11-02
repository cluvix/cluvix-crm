package service

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"erpCluvix/internal/modules/webapp/crm/customer_detail/dto"
	paymentdto "erpCluvix/internal/modules/webapp/crm/payment/dto"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/handlerutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/pagination"
)

// paymentTitleMap — map payment_for → tiêu đề hiển thị trên timeline.
var paymentTitleMap = map[int64]string{
	4: "Thanh toán",
	3: "Đặt cọc",
	2: `<b style="color:red">Hoàn cọc</b>`,
	5: "Chuyển cọc",
	1: `<b style="color:red">Hoàn dịch vụ</b>`,
}

// buildCustomerCreatedEvent — sự kiện "Khởi tạo khách hàng", dựng từ customer map đã enrich (paymentUserId, source).
func buildCustomerCreatedEvent(customer map[string]interface{}) dto.TimelineEvent {
	content := ""
	if pu, ok := customer["paymentUserId"].(map[string]interface{}); ok && pu["fullname"] != nil {
		content += "- Tư vấn: <b>" + fmt.Sprint(pu["fullname"]) + "</b><br />"
	}
	if s, ok := customer["source"].(map[string]interface{}); ok && s["name"] != nil {
		content += "- Nguồn: <b>" + fmt.Sprint(s["name"]) + "</b>"
	}
	day := customer["created_day"]
	if day == nil {
		day = customer["created_at"]
	}
	return dto.TimelineEvent{
		Title: "Khởi tạo khách hàng", Day: helpers.ToInt64(day), Content: content,
		IconDirection: "pi pi-user-plus", IconColor: "#3F51B5",
	}
}

// buildAppointmentEvents — appointment (scoped) + batch load appointment_type (shared, rawDB) + service (scoped riêng).
func buildAppointmentEvents(apptScopedDB, serviceScopedDB, rawDB *gorm.DB, customerID uint) ([]dto.TimelineEvent, error) {
	var appts []map[string]interface{}
	tx := apptScopedDB.Table("appointment").
		Select("appointment.appointment_type_id, appointment.type_service_id, appointment.created_at, appointment.appointment_date, appointment.content").
		Where("appointment.customer_id = ?", customerID).Order("appointment.created_at DESC").Limit(30).Find(&appts)
	if pagination.IsQueryError(tx.Error) {
		return nil, tx.Error
	}

	events := make([]dto.TimelineEvent, 0, len(appts))
	if len(appts) == 0 {
		return events, nil
	}

	atIDs := handlerutil.CollectIDs(appts, "appointment_type_id")
	tsIDs := handlerutil.CollectIDs(appts, "type_service_id")
	atMap := make(map[int64]string)
	tsMap := make(map[int64]string)
	if len(atIDs) > 0 {
		var rows []map[string]interface{}
		tx := rawDB.Table("appointment_type").Select("id, name").Where("id IN ?", atIDs).Find(&rows)
		if pagination.IsQueryError(tx.Error) {
			return nil, tx.Error
		}
		for _, r := range rows {
			atMap[helpers.ToInt64(r["id"])] = fmt.Sprint(r["name"])
		}
	}
	if len(tsIDs) > 0 {
		var rows []map[string]interface{}
		tx := serviceScopedDB.Table("service").Select("id, name").Where("id IN ?", tsIDs).Find(&rows)
		if pagination.IsQueryError(tx.Error) {
			return nil, tx.Error
		}
		for _, r := range rows {
			tsMap[helpers.ToInt64(r["id"])] = fmt.Sprint(r["name"])
		}
	}
	for _, v := range appts {
		ct := ""
		if v["appointment_date"] != nil {
			t := time.Unix(helpers.ToInt64(v["appointment_date"]), 0)
			ct += "- Ngày hẹn: <b>" + t.Format("02/01/2006") + "</b><br />"
		}
		if name := tsMap[helpers.ToInt64(v["type_service_id"])]; name != "" {
			ct += "- Quan tâm: <b>" + name + "</b><br />"
		}
		if v["content"] != nil {
			ct += "- Nội dung: <i>" + fmt.Sprint(v["content"]) + "</i>"
		}
		events = append(events, dto.TimelineEvent{
			Title: "Đặt hẹn: " + atMap[helpers.ToInt64(v["appointment_type_id"])],
			Day:   helpers.ToInt64(v["created_at"]), Content: ct,
			IconDirection: "pi pi-calendar", IconColor: "#7B3F9E",
		})
	}
	return events, nil
}

// buildTreatmentPlanEvents — order_plan LEFT JOIN order_service_used (scoped).
func buildTreatmentPlanEvents(scopedDB, rawDB *gorm.DB, customerID uint) ([]dto.TimelineEvent, error) {
	var plans []map[string]interface{}
	tx := scopedDB.Table("order_plan").
		Select("order_plan.created_time, COALESCE(SUM(osu.totalEnd), 0) AS total").
		Joins("LEFT JOIN order_service_used osu ON osu.order_plan_id = order_plan.id").
		Where("order_plan.customer_id = ?", customerID).
		Group("order_plan.id").
		Order("order_plan.created_time DESC").Limit(15).Find(&plans)
	if pagination.IsQueryError(tx.Error) {
		return nil, tx.Error
	}

	events := make([]dto.TimelineEvent, 0, len(plans))
	for _, v := range plans {
		events = append(events, dto.TimelineEvent{
			Title: "Lên đơn hàng", Day: helpers.ToInt64(v["created_time"]),
			Content:       fmt.Sprintf(`- Giá trị: <b style="color:red">%s</b>`, helpers.FormatAmount(rawDB, helpers.ToFloat64(v["total"]))),
			IconDirection: "pi pi-shopping-cart", IconColor: "#FF5722",
		})
	}
	return events, nil
}

// buildPaymentEvents — payment (scoped). FormatAmount(…, …): xem buildTreatmentPlanEvents.
func buildPaymentEvents(scopedDB, rawDB *gorm.DB, customerID uint) []dto.TimelineEvent {
	var payments []paymentdto.PaymentTimelineDTO
	scopedDB.Table("payment").
		Select("payment_for, amount, payment_time, payment_of_date, note, method").
		Where("customer_id = ?", customerID).Order("payment_time DESC").Limit(20).Find(&payments)

	events := make([]dto.TimelineEvent, 0, len(payments))
	for _, v := range payments {
		var pf int64
		if v.PaymentFor != nil {
			pf = int64(*v.PaymentFor)
		}
		title := paymentTitleMap[pf]
		if title == "" {
			title = "Thanh toán"
		}
		amt, _ := v.Amount.Float64()
		pc := fmt.Sprintf("- Số tiền: <b>%s</b><br />", helpers.FormatAmount(rawDB, amt))
		if v.Note != "" {
			pc += "<br />- Nội dung: " + v.Note
		}
		var d int64
		if v.PaymentTime != nil {
			d = *v.PaymentTime
		} else if v.PaymentOfDate != nil {
			d = *v.PaymentOfDate
		}
		events = append(events, dto.TimelineEvent{
			Title: title, Day: d, Content: pc,
			IconDirection: "pi pi-money-bill", IconColor: "#009688",
		})
	}
	return events
}

// batchLoadUserNames — batch-load user_profile.fullname theo user_id, tránh N+1.
// Dùng chung cho các builder cần tên user (bác sĩ check-up, agent tổng đài...).
func batchLoadUserNames(db *gorm.DB, userIDs []interface{}) (map[int64]string, error) {
	userMap := make(map[int64]string)
	if len(userIDs) == 0 {
		return userMap, nil
	}
	var profiles []map[string]interface{}
	tx := db.Table("user_profile").Select("user_id, fullname").Where("user_id IN ?", userIDs).Find(&profiles)
	if pagination.IsQueryError(tx.Error) {
		return nil, tx.Error
	}
	for _, p := range profiles {
		userMap[helpers.ToInt64(p["user_id"])] = fmt.Sprint(p["fullname"])
	}
	return userMap, nil
}

// buildReminderEvents — reminder (db do handler dựng — tenant.CrossBranchLookup như các builder khác).
func buildReminderEvents(scopedDB *gorm.DB, customerID uint) ([]dto.TimelineEvent, error) {
	var rows []map[string]interface{}
	tx := scopedDB.Table("reminder").
		Select("id, customer_id, type, start_time, note").
		Where("customer_id = ?", customerID).Order("start_time DESC").Limit(15).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		return nil, tx.Error
	}

	events := make([]dto.TimelineEvent, 0, len(rows))
	for _, v := range rows {
		if v["start_time"] == nil {
			continue
		}
		events = append(events, dto.TimelineEvent{
			Title: "Nhắc hẹn", Day: helpers.ToInt64(v["start_time"]), Content: helpers.ToString(v["note"]),
			IconDirection: "pi pi-bell", IconColor: "#FF6F00",
		})
	}
	return events, nil
}

// stageHistoryRow — 1 dòng customer_stage_history + tên người đổi (LEFT JOIN, có thể rỗng).
type stageHistoryRow struct {
	FromStageID   *int64 `gorm:"column:from_stage_id"`
	ToStageID     int64  `gorm:"column:to_stage_id"`
	Source        int    `gorm:"column:source"`
	ChangedBy     *int64 `gorm:"column:changed_by"`
	ChangedByName string `gorm:"column:changed_by_name"`
	ChangedAt     int64  `gorm:"column:changed_at"`
}

// buildStageChangeEvents — lịch sử chuyển bước của khách (story-04 customer-stage-pipeline, FR15).
// Khách cha đã qua scope ở handler TimelineCustomer; bảng không có branch_id. Tên người đổi lấy qua
// LEFT JOIN user_profile (changed_by NULL = hệ thống / user đã xoá ⇒ tên rỗng, dòng KHÔNG bị loại).
// Tên bước KHÔNG attach — FE tra Dexie (architecture D9). `id DESC` phá hoà khi cùng changed_at.
func buildStageChangeEvents(db *gorm.DB, customerID uint) ([]dto.TimelineEvent, error) {
	var rows []stageHistoryRow
	tx := db.Table("customer_stage_history AS h").
		Select("h.from_stage_id, h.to_stage_id, h.source, h.changed_by, h.changed_at, "+
			"COALESCE(up.fullname, '') AS changed_by_name").
		Joins("LEFT JOIN user_profile up ON up.user_id = h.changed_by").
		Where("h.customer_id = ?", customerID).
		Order("h.changed_at DESC, h.id DESC").Limit(define.CustomerTimelineStageChangeLimit).
		Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		return nil, tx.Error
	}

	events := make([]dto.TimelineEvent, 0, len(rows))
	// rows đang mới → cũ; duyệt ngược để sự kiện cùng giây giữ thứ tự ghi (SliceStable ở caller).
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		content := ""
		if r.ChangedByName != "" {
			content = "- Người đổi: <b>" + r.ChangedByName + "</b>"
		}
		events = append(events, dto.TimelineEvent{
			Title: "Chuyển tiến trình", Day: r.ChangedAt, Content: content,
			IconDirection: "pi pi-flag", IconColor: "#1E88E5",
			Type: define.CustomerTimelineEventStageChange,
			StageChange: &dto.TimelineStageChange{
				FromStageID: r.FromStageID, ToStageID: r.ToStageID,
				ChangedBy: r.ChangedBy, ChangedByName: r.ChangedByName,
				ChangedAt: r.ChangedAt, Source: r.Source,
			},
		})
	}
	return events, nil
}
