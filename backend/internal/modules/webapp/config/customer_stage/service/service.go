// Package service — logic có giao dịch của danh mục bước tiến trình khách hàng
// (reorder, set-default, xoá, kiểm tra đầu vào). Handler chỉ bind + scope + trả response.
package service

import (
	"errors"
	"slices"
	"strings"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"erpCluvix/internal/models"
	"erpCluvix/internal/modules/webapp/config/customer_stage/dao"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/helpers"
)

// Khoá i18n (vi.go/en.go) — gom một chỗ để handler và test dùng chung, không rải chuỗi.
const (
	KeyNameRequired       = "customer_stage_name_required"
	KeyNameDuplicate      = "customer_stage_name_duplicate"
	KeyTypeInvalid        = "customer_stage_type_invalid"
	KeyColorInvalid       = "customer_stage_color_invalid"
	KeyStatusInvalid      = "invalid_status"
	KeyDefaultCannotHide  = "customer_stage_default_cannot_hide"
	KeyDefaultCannotDel   = "customer_stage_default_cannot_delete"
	KeyDefaultMustBeOpen  = "customer_stage_default_must_be_open"
	KeyDefaultMustBeShown = "customer_stage_default_must_be_visible"
	KeyInUse              = "customer_stage_in_use"
	KeyReorderInvalid     = "customer_stage_reorder_invalid"
)

// uniqueNameIndex — tên UNIQUE index của customer_stage.name (migration 20261102095255).
const uniqueNameIndex = "uq_customer_stage_name"

type Service struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Service { return &Service{db: db} }

// NormalizeName — bỏ khoảng trắng hai đầu. Tên LƯU và tên đem so trùng đều là bản đã trim.
func NormalizeName(name string) string { return strings.TrimSpace(name) }

// ValidateFields kiểm name/type/color/status của bước (đã NormalizeName). Trả lỗi i18n (LocalizedError)
// để handler đẩy qua response.HandleError. status=nil nghĩa là "để mặc định" (create).
func ValidateFields(name string, typ int, color string, status *int) error {
	if name == "" {
		return errs.NewLocalizedError(KeyNameRequired)
	}
	if typ != define.CustomerStageTypeOpen && typ != define.CustomerStageTypeWon && typ != define.CustomerStageTypeLost {
		return errs.NewLocalizedError(KeyTypeInvalid)
	}
	if !slices.Contains(define.CustomerStageColors, color) {
		return errs.NewLocalizedError(KeyColorInvalid)
	}
	if status != nil && *status != define.StatusHidden && *status != define.StatusActive {
		return errs.NewLocalizedError(KeyStatusInvalid)
	}
	return nil
}

// ValidateDefaultInvariant — bước đang là mặc định không được ẩn và phải còn là loại "đang mở".
func ValidateDefaultInvariant(isDefault int, typ int, status *int) error {
	if isDefault != 1 {
		return nil
	}
	if status != nil && *status == define.StatusHidden {
		return errs.NewLocalizedError(KeyDefaultCannotHide)
	}
	if typ != define.CustomerStageTypeOpen {
		return errs.NewLocalizedError(KeyDefaultMustBeOpen)
	}
	return nil
}

// NameTaken — đã có bước KHÁC dùng tên này chưa. So bằng chính collation của cột (utf8mb4_unicode_ci:
// không phân biệt hoa/thường VÀ dấu) để app và UNIQUE DB cùng một ngữ nghĩa — KHÔNG LOWER()/TRIM() tự chế.
// excludeID=0 khi tạo mới.
func NameTaken(db *gorm.DB, name string, excludeID uint) (bool, error) {
	var n int64
	err := db.Model(&dao.CustomerStage{}).Where("name = ? AND id <> ?", name, excludeID).Count(&n).Error
	return n > 0, err
}

// IsDuplicateNameErr — lỗi 1062 do UNIQUE name (lưới cuối khi 2 request cùng tên chạy đồng thời).
func IsDuplicateNameErr(err error) bool {
	var me *mysqldriver.MySQLError
	return helpers.IsDuplicateKeyError(err) && errors.As(err, &me) && strings.Contains(me.Message, uniqueNameIndex)
}

// NextSortOrder — max(sort_order)+1 (bước đầu tiên = 1).
func NextSortOrder(db *gorm.DB) (int, error) {
	var maxOrder int
	err := db.Model(&dao.CustomerStage{}).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder).Error
	return maxOrder + 1, err
}

// Reorder đặt sort_order = 1..n theo thứ tự ids. ids PHẢI đúng bằng tập bước hiện có (không thiếu,
// không thừa, không trùng) — sai ⇒ LocalizedError(customer_stage_reorder_invalid), không ghi gì.
// Một giao dịch; khoá toàn bộ dòng danh mục để tập bước không đổi giữa lúc kiểm và lúc ghi.
func (s *Service) Reorder(ids []int, userID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var current []struct {
			ID        uint
			SortOrder int
		}
		if err := tx.Model(&dao.CustomerStage{}).Select("id, sort_order").Order("id ASC").
			Clauses(clause.Locking{Strength: "UPDATE"}).Find(&current).Error; err != nil {
			return err
		}
		if len(ids) != len(current) {
			return errs.NewLocalizedError(KeyReorderInvalid)
		}
		have := make(map[uint]int, len(current))
		for _, r := range current {
			have[r.ID] = r.SortOrder
		}
		seen := make(map[int]struct{}, len(ids))
		for _, id := range ids {
			if _, dup := seen[id]; dup {
				return errs.NewLocalizedError(KeyReorderInvalid)
			}
			seen[id] = struct{}{}
			if id <= 0 {
				return errs.NewLocalizedError(KeyReorderInvalid)
			}
			if _, ok := have[uint(id)]; !ok {
				return errs.NewLocalizedError(KeyReorderInvalid)
			}
		}
		now := time.Now().Unix()
		for i, id := range ids {
			pos := i + 1
			if have[uint(id)] == pos {
				continue
			}
			upd := map[string]any{"sort_order": pos, "updated_at": now}
			if userID != 0 {
				upd["updated_by"] = userID
			}
			if err := tx.Model(&dao.CustomerStage{}).Where("id = ?", id).UpdateColumns(upd).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SetDefault đặt bước `id` làm mặc định: bước phải đang hiện và loại "đang mở". Trong 1 giao dịch,
// 2 lệnh riêng (bỏ cờ cũ rồi đặt cờ mới) vì UNIQUE default_key kiểm từng dòng — gộp 1 lệnh có thể
// vỡ tạm thời. Đã là mặc định ⇒ không làm gì (idempotent).
func (s *Service) SetDefault(id uint, userID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Khoá TOÀN BỘ danh mục theo thứ tự id (như Reorder) TRƯỚC mọi thứ khác — KHÔNG "tối ưu" bỏ đi, khoá này làm 2 việc:
		// (a) chống deadlock: khoá dòng đích trước rồi `UPDATE … WHERE is_default = 1` (quét cả bảng, không có index)
		//     làm 2 request đặt mặc định cho 2 bước khác nhau giữ khoá ngược thứ tự nhau ⇒ deadlock 1213 ⇒ 500.
		// (b) giữ bất biến "bước mặc định phải đang hiện + loại mở" khi chạy song song với ẩn/xoá: bước đích chỉ được
		//     ĐỌC sau khi mọi giao dịch ẩn/xoá đang giữ khoá đã commit ⇒ không thể đặt mặc định cho bước vừa bị ẩn
		//     hoặc vừa bị xoá (Update/Delete đều khoá dòng FOR UPDATE rồi mới kiểm). Test canh cả hai:
		//     TestSetDefault_Concurrent* trong handler/customer_stage_integration_test.go.
		var lockedIDs []uint
		if err := tx.Model(&dao.CustomerStage{}).Order("id ASC").
			Clauses(clause.Locking{Strength: "UPDATE"}).Pluck("id", &lockedIDs).Error; err != nil {
			return err
		}
		var stage dao.CustomerStage
		if err := tx.Select("id, type, status, is_default").First(&stage, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.ErrNotFound
			}
			return err
		}
		if stage.Status == nil || *stage.Status != define.StatusActive {
			return errs.NewLocalizedError(KeyDefaultMustBeShown)
		}
		if stage.Type != define.CustomerStageTypeOpen {
			return errs.NewLocalizedError(KeyDefaultMustBeOpen)
		}
		if stage.IsDefault == 1 {
			return nil
		}
		now := time.Now().Unix()
		unset := map[string]any{"is_default": 0, "updated_at": now}
		set := map[string]any{"is_default": 1, "updated_at": now}
		if userID != 0 {
			unset["updated_by"] = userID
			set["updated_by"] = userID
		}
		if err := tx.Model(&dao.CustomerStage{}).Where("is_default = 1").UpdateColumns(unset).Error; err != nil {
			return err
		}
		return tx.Model(&dao.CustomerStage{}).Where("id = ?", id).UpdateColumns(set).Error
	})
}

// Delete xoá bước chưa dùng. Bước mặc định ⇒ LocalizedError(customer_stage_default_cannot_delete);
// bước đang có khách / có trong lịch sử ⇒ FK RESTRICT (1451) do models.DeleteWithCheck bắt, đổi sang
// LocalizedError(customer_stage_in_use) để message gợi ý ẩn thay vì xoá.
func (s *Service) Delete(id uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var stage dao.CustomerStage
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id, is_default").First(&stage, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errs.ErrNotFound
			}
			return err
		}
		if stage.IsDefault == 1 {
			return errs.NewLocalizedError(KeyDefaultCannotDel)
		}
		// KIỂM TRƯỚC ở app, KHÔNG dựa vào 1451: mysqld 9.7.1 SIGSEGV khi một DELETE cha bị 1451 bởi dòng `customer`
		// từ lần thứ hai (cột sinh VIRTUAL regexp `customer.phone_unique_key` làm hỏng table cache). Bước đang dùng
		// ⇒ trả lỗi mà KHÔNG gửi DELETE. DeleteWithCheck + map 1451 phía dưới chỉ còn là lưới cuối (race).
		inUse, err := stageInUse(tx, id)
		if err != nil {
			return err
		}
		if inUse {
			return errs.NewLocalizedError(KeyInUse)
		}
		if err := models.DeleteWithCheck(tx, &dao.CustomerStage{}, id); err != nil {
			if err.Error() == "fk_constraint" {
				return errs.NewLocalizedError(KeyInUse)
			}
			return err
		}
		return nil
	})
}

// stageInUse — có khách đang ở bước này, hoặc bước này nằm trong lịch sử (from/to) của khách nào đó.
func stageInUse(db *gorm.DB, id uint) (bool, error) {
	var n int
	err := db.Raw(`SELECT
		EXISTS(SELECT 1 FROM customer WHERE stage_id = ?)
		OR EXISTS(SELECT 1 FROM customer_stage_history WHERE from_stage_id = ?)
		OR EXISTS(SELECT 1 FROM customer_stage_history WHERE to_stage_id = ?)`, id, id, id).Scan(&n).Error
	return n == 1, err
}
