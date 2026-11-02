package service

import (
	"errors"

	"gorm.io/gorm"

	"erpCluvix/internal/modules/webapp/config/customer_stage/dao"
	"erpCluvix/pkg/errs"
)

// Khoá i18n của đường GHI bước khách (tạo khách, import, đổi bước) — story-03.
const (
	KeyDefaultMissing = "customer_stage_default_missing"
	KeyNotSelectable  = "customer_stage_not_selectable"
)

// ErrDefaultStageMissing — DB không có bước nào is_default = 1. Là *errs.LocalizedError nên
// response.HandleError trả 422 + message i18n; caller so bằng errors.Is (cùng con trỏ).
var ErrDefaultStageMissing = errs.NewLocalizedError(KeyDefaultMissing)

// DefaultStageID — id bước đang is_default = 1, đọc trên `tx` của caller (cùng transaction với INSERT
// khách). Đọc MỖI LẦN gọi, không cache: admin đổi mặc định thì khách tạo sau đó nhận bước mới.
// Không có ⇒ ErrDefaultStageMissing; lỗi SQL trả nguyên văn.
func DefaultStageID(tx *gorm.DB) (int, error) {
	var ids []int
	if err := tx.Model(&dao.CustomerStage{}).Where("is_default = 1").Limit(1).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 || ids[0] <= 0 {
		return 0, ErrDefaultStageMissing
	}
	return ids[0], nil
}

// RecordStageChange — INSERT 1 dòng customer_stage_history trên `tx` của caller (cùng transaction với
// thay đổi customer.stage_id: lỗi ở đây ⇒ caller rollback, khách không đổi bước). from = nil khi khách
// nhận bước lần đầu (tạo/import). userID = 0 ⇒ changed_by NULL (hệ thống). at = Unix giây do caller lấy.
func RecordStageChange(tx *gorm.DB, customerID int, from *int, to int, source int, userID int, at int64) error {
	if customerID <= 0 || to <= 0 {
		return errors.New("customer_stage_history: customer_id/to_stage_id không hợp lệ")
	}
	row := dao.CustomerStageHistory{
		CustomerId:  customerID,
		FromStageId: from,
		ToStageId:   to,
		Source:      source,
		ChangedAt:   at,
	}
	if userID != 0 {
		row.ChangedBy = &userID
	}
	return tx.Create(&row).Error
}
