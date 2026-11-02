package define

// UserOption IDs (bảng user_option)
const (
	UserOptionUpdateOrderPlan = 3 // Cho phép user sửa đơn hàng trong ngày (không có quyền updatePlanOrder)
	UserOptionViewLeadOfTele  = 5 // OPTION_VIEW_LEAD_OF_TELE — bật (=1): nhân viên phòng Telesale thấy mọi khách; tắt: chỉ khách mình phụ trách (customer.payment_user_id)
	UserOptionDeleteNote      = 6 // Cho phép user xóa note do chính mình tạo trong ngày
)

// User statuses
const (
	UserStatusActive   = 10
	UserStatusInactive = 9
	UserStatusDeleted  = 1
)

// UserDevice statuses (bảng user_device — device binding)
const (
	UserDeviceTrusted = 1
	UserDevicePending = 0
	UserDeviceRevoked = -1
)

// UserDeviceStaleDays — device không hoạt động quá ngưỡng này bị cron dọn (chống phình bảng).
const UserDeviceStaleDays = 90

// AuditRetentionDays — nhật ký bảo mật giữ tối đa ngưỡng này rồi bị cron dọn.
const AuditRetentionDays = 180

// UserProfileLocales — giá trị hợp lệ cho user_profile.locale (FE ngx-translate BCP47-ish,
// khác pkg/i18n.{Vi,En} là mã ngắn dùng riêng cho message BE theo Accept-Language).
// Đồng bộ với CONFIG_LOCALE_LANGUAGE_OPTIONS (frontend/src/app/define/config-general.define.ts).
// D5 (story-22): chỉ còn vi-VN + en-US.
var UserProfileLocales = map[string]bool{
	"vi-VN": true,
	"en-US": true,
}

// OrderServiceUsed lock status
const (
	OrderServiceUsedNotLock = 1
	OrderServiceUsedIsLock  = 2
	OrderServiceUsedPause   = 3
)

// UserMustChangePassword — giá trị cột user.must_change_password (S-4/D-1): 1 = bắt buộc đổi
// mật khẩu trước khi dùng hệ thống (admin seed mật khẩu mặc định, mật khẩu do người khác đặt lại).
const (
	UserMustChangePasswordNo  = 0
	UserMustChangePasswordYes = 1
)

// ErrCodePasswordChangeRequired — `code` trong body 403 khi user còn cờ must_change_password;
// FE interceptor bắt code này để điều hướng sang màn đổi mật khẩu bắt buộc.
const ErrCodePasswordChangeRequired = "password_change_required"
