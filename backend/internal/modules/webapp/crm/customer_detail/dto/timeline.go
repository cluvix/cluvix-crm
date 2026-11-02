package dto

// TimelineEvent — 1 sự kiện hiển thị trong customer timeline.
type TimelineEvent struct {
	Title         string `json:"title"`
	Day           int64  `json:"day"`
	Content       string `json:"content"`
	IconDirection string `json:"iconDirection"`
	IconColor     string `json:"iconColor"`

	// Type — loại sự kiện có dữ liệu cấu trúc cho FE (vd define.CustomerTimelineEventStageChange);
	// rỗng (bỏ khỏi JSON) = sự kiện hiển thị thuần title/content như trước.
	Type string `json:"type,omitempty"`
	// StageChange — chỉ có ở sự kiện chuyển bước (story-04 customer-stage-pipeline).
	StageChange *TimelineStageChange `json:"stage_change,omitempty"`
}

// TimelineStageChange — 1 dòng `customer_stage_history`. BE chỉ trả id bước; tên/màu bước FE tra
// Dexie `customerStages` (architecture D9 — KHÔNG attach từ BE).
type TimelineStageChange struct {
	FromStageID   *int64 `json:"from_stage_id"`   // null = lần gán bước đầu tiên (tạo/import/migration)
	ToStageID     int64  `json:"to_stage_id"`     //
	ChangedBy     *int64 `json:"changed_by"`      // null = hệ thống / người đổi đã bị xoá (FK SET NULL)
	ChangedByName string `json:"changed_by_name"` // user_profile.fullname; "" khi changed_by null/không có hồ sơ
	ChangedAt     int64  `json:"changed_at"`      // Unix giây — trùng `day` của sự kiện
	Source        int    `json:"source"`          // define.CustomerStageHistorySource*
}
