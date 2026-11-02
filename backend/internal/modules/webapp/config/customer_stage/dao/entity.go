package dao

import "erpCluvix/internal/models"

// CustomerStage — một bước trong pipeline tiến trình khách hàng (danh mục toàn DB, không có branch_id).
// Cột sinh `default_key` (ép đúng 1 bước mặc định qua UNIQUE) do DB quản lý — CỐ Ý không map vào struct,
// map vào thì GORM đưa cột đó vào INSERT ⇒ mọi Create lỗi.
type CustomerStage struct {
	models.SimpleModel
	Name      string `gorm:"column:name;size:100;not null;uniqueIndex:uq_customer_stage_name;comment:Tên bước" json:"name"`
	Type      int    `gorm:"column:type;not null;comment:Loại bước: 1 đang mở, 2 thành công, 3 thất bại (define.CustomerStageType*)" json:"type"`
	Color     string `gorm:"column:color;size:20;not null;comment:Khoá màu (define.CustomerStageColors)" json:"color"`
	SortOrder int    `gorm:"column:sort_order;not null;default:0;comment:Thứ tự trong pipeline" json:"sort_order"`
	IsDefault int    `gorm:"column:is_default;not null;default:0;comment:1 = bước mặc định cho khách mới" json:"is_default"`
	CreatedBy *int   `gorm:"column:created_by;index;comment:Được tạo bởi ai" json:"created_by"`
	UpdatedBy *int   `gorm:"column:updated_by;index;comment:Người cập nhật gần đây nhất" json:"updated_by"`
}

func (CustomerStage) TableName() string { return "customer_stage" }

// CustomerStageHistory — một lần khách đổi bước. Không có branch_id: chỉ đọc/ghi sau khi khách cha đã
// qua scope. Bảng không có updated_at/status ⇒ không embed BaseModel/SimpleModel.
type CustomerStageHistory struct {
	ID          uint  `gorm:"primaryKey" json:"id"`
	CustomerId  int   `gorm:"column:customer_id;not null;comment:FK → customer" json:"customer_id"`
	FromStageId *int  `gorm:"column:from_stage_id;comment:FK → customer_stage (NULL = khởi tạo)" json:"from_stage_id"`
	ToStageId   int   `gorm:"column:to_stage_id;not null;comment:FK → customer_stage" json:"to_stage_id"`
	Source      int   `gorm:"column:source;not null;comment:define.CustomerStageHistorySource*" json:"source"`
	ChangedBy   *int  `gorm:"column:changed_by;comment:FK → user (NULL = hệ thống)" json:"changed_by"`
	ChangedAt   int64 `gorm:"column:changed_at;not null;comment:Thời điểm đổi (Unix)" json:"changed_at"`
}

func (CustomerStageHistory) TableName() string { return "customer_stage_history" }
