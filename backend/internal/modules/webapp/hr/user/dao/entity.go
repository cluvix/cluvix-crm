package dao

import (
	"erpCluvix/pkg/helpers"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type User struct {
	ID           uint   `gorm:"primaryKey" json:"id" form:"-"`
	IdOld        *int   `gorm:"column:id_old;uniqueIndex" json:"id_old"`
	Username     string `gorm:"uniqueIndex;size:255;not null;comment:Tên đăng nhập" json:"username"`
	PasswordHash string `gorm:"column:password_hash;size:255;not null" json:"-"`
	// MustChangePassword — user do admin tạo/import (người tạo biết mật khẩu) phải đổi ở lần đăng nhập
	// đầu (R-7, S-4/D-1); ghi lúc Create. Đổi cờ sau đó: UserService.ChangePassword (map-Updates).
	// KHÔNG dùng quyền `<-:create` — GORM chặn luôn map-Updates cột này qua Model(&User{}) (đo 2026-10-18:
	// tự đổi mật khẩu không gỡ được cờ). Không có chỗ nào Save() struct User nạp thiếu cột.
	MustChangePassword int            `gorm:"column:must_change_password" json:"-"`
	BranchId           datatypes.JSON `gorm:"column:branchId;type:json;comment:JSON array ID chi nhánh được phân quyền" json:"branchId"`
	Status             int            `gorm:"not null;default:10;comment:10=Active, 0=Inactive, 9=Deleted" json:"status"`
	AllowedIps         datatypes.JSON `gorm:"column:allowed_ips;type:json;comment:Override IP allowlist riêng user" json:"allowed_ips"`
	CreatedAt          int64          `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt          int64          `gorm:"column:updated_at;not null" json:"updated_at"`
	CreatedBy          *int           `gorm:"column:created_by" json:"created_by"`
	UpdatedBy          *int           `gorm:"column:updated_by" json:"updated_by"`
}

func (User) TableName() string { return "user" }

type UserProfile struct {
	UserId         int            `gorm:"primaryKey;column:user_id" json:"user_id"`
	Fullname       string         `gorm:"size:255;not null;comment:Họ tên" json:"fullname"`
	Nickname       string         `gorm:"size:50;comment:Tên gọi khác" json:"nickname"`
	Code           string         `gorm:"size:25;comment:Mã nhân viên" json:"code"`
	Birthday       *string        `gorm:"type:date;comment:Ngày sinh" json:"birthday"`
	About          string         `gorm:"comment:Giới thiệu bản thân" json:"about"`
	Address        string         `gorm:"size:255;comment:Địa chỉ" json:"address"`
	Phone          *string        `gorm:"uniqueIndex;size:25;comment:Số điện thoại" json:"phone"`
	Email          *string        `gorm:"size:100;comment:Email" json:"email"`
	IdPancake      string         `gorm:"column:id_pancake;size:50;comment:ID Pancake" json:"id_pancake"`
	Facebook       string         `gorm:"size:255;comment:Facebook" json:"facebook"`
	Telegram       string         `gorm:"size:255;comment:Telegram" json:"telegram"`
	Avatar         string         `gorm:"size:255;comment:Avatar" json:"avatar"`
	Cover          string         `gorm:"size:255;comment:Ảnh bìa" json:"cover"`
	Locale         string         `gorm:"size:32;default:vi-VN;comment:Ngôn ngữ hiển thị" json:"locale"`
	Gender         *int           `gorm:"comment:Giới tính: 1=Nam, 2=Nữ" json:"gender"`
	DepartmentList datatypes.JSON `gorm:"column:departmentList;type:json;comment:JSON array ID phòng ban" json:"departmentList"`
	BranchOfficeId *int           `gorm:"column:branchOffice_id;comment:Chi nhánh nhân viên làm việc" json:"branchOffice_id"`
	BranchActiveId *int           `gorm:"column:branchActive_id;index;comment:Chi nhánh nhân viên muốn thấy khách hàng" json:"branchActive_id"`
	UpdatedAt      int64          `gorm:"column:updated_at;not null;default:0" json:"updated_at"` // khoá lạc quan (D6, pkg/optlock)
}

func (UserProfile) TableName() string { return "user_profile" }

func (p *UserProfile) BeforeSave(tx *gorm.DB) error {
	if p.Phone != nil && *p.Phone == "" {
		p.Phone = nil
	}
	if p.Email != nil && *p.Email == "" {
		p.Email = nil
	}
	helpers.NormalizeDateOnlyPtr(&p.Birthday)
	return nil
}
