package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"erpCluvix/internal/middleware"
	history "erpCluvix/internal/modules/webapp/history/log/service"
	hrUserDao "erpCluvix/internal/modules/webapp/hr/user/dao"
	"erpCluvix/pkg/cache"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/dexie"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/optlock"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/storage"
	"erpCluvix/pkg/token"
)

type UserService struct {
	DB       *gorm.DB
	Cache    *cache.RedisCache
	TokenSvc *token.Service
}

// ── Create User ──

type CreateUserInput struct {
	Username    string          `json:"username" binding:"required,min=5,max=20,alphanumunicode"`
	Password    string          `json:"password" binding:"required,min=6,max=64"`
	Status      int             `json:"status"   form:"status" binding:"oneof=1 9 10"`
	BranchId    json.RawMessage `json:"branchId"`
	Role        string          `json:"role"`
	UserProfile json.RawMessage `json:"user_profile"`
	AllowedIps  json.RawMessage `json:"allowed_ips"`
}

func (s *UserService) CreateUser(input CreateUserInput, operatorID uint) (*hrUserDao.User, error) {
	if input.Status == 0 {
		input.Status = define.UserStatusInactive
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), 12)
	if err != nil {
		return nil, errors.New("cannot_hash_password")
	}

	now := time.Now().Unix()
	opID := int(operatorID)
	user := hrUserDao.User{
		Username:     input.Username,
		PasswordHash: string(hash),
		// R-7: admin đặt mật khẩu hộ ⇒ chủ tài khoản phải đổi ở lần đăng nhập đầu.
		MustChangePassword: define.UserMustChangePasswordYes,
		Status:             input.Status,
		BranchId:           datatypes.JSON(input.BranchId),
		AllowedIps:         datatypes.JSON(input.AllowedIps),
		CreatedAt:          now,
		UpdatedAt:          now,
		CreatedBy:          &opID,
		UpdatedBy:          &opID,
	}

	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := tx.Create(&user).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Create profile
	//
	// LUÔN tạo dòng `user_profile`, kể cả khi client không gửi `user_profile` (locale, chi nhánh
	// đang xem… đọc qua LEFT JOIN user_profile). `fullname` NOT NULL không default nên phải có giá
	// trị: lấy tạm username. Story-58: không còn ghi `companyActive_id` (session không còn company).
	var profile hrUserDao.UserProfile
	if input.UserProfile != nil {
		if err := json.Unmarshal(input.UserProfile, &profile); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("unmarshal user profile: %w", err)
		}
	}
	profile.UserId = int(user.ID)
	if profile.Fullname == "" {
		profile.Fullname = input.Username
	}
	if err := tx.Create(&profile).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Assign role
	if input.Role != "" {
		if err := tx.Exec("INSERT INTO rbac_auth_assignment (item_name, user_id, created_at) VALUES (?, ?, ?)",
			input.Role, user.ID, now).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	s.invalidateCache(user.ID)
	return &user, nil
}

// ── Update User ──

type UpdateUserInput struct {
	UpdatedAt   int64           `json:"updated_at"` // khoá lạc quan (D6)
	Username    *string         `json:"username"`
	Status      *int            `json:"status" form:"status"`
	BranchId    json.RawMessage `json:"branchId"`
	Role        *string         `json:"role"`
	UserProfile json.RawMessage `json:"user_profile"`
	AllowedIps  json.RawMessage `json:"allowed_ips"`
}

// hrProfileEditableFields — whitelist cột `user_profile` mà màn quản trị nhân sự (PUT /hr/user/:id)
// được ghi: các cột self-service (`config/user_profile` userProfileEditableFields) + `code`,
// `departmentList`, `cover`. CỐ Ý KHÔNG có `user_id` (PK — dời hồ sơ sang user khác),
// `branchActive_id` (đổi qua /switch-profile có kiểm quyền chi nhánh; ghi thẳng ở đây ⇒ tự nhảy
// sang chi nhánh ngoài phạm vi), `updated_at` (callback optlock tự bump).
var hrProfileEditableFields = map[string]bool{
	"fullname":        true,
	"nickname":        true,
	"code":            true,
	"birthday":        true,
	"about":           true,
	"address":         true,
	"phone":           true,
	"email":           true,
	"id_pancake":      true,
	"facebook":        true,
	"telegram":        true,
	"avatar":          true,
	"cover":           true,
	"locale":          true,
	"gender":          true,
	"departmentList":  true,
	"branchOffice_id": true,
}

func (s *UserService) UpdateUser(userID uint, input UpdateUserInput, operatorID uint) (*hrUserDao.User, error) {
	var user hrUserDao.User
	if err := s.DB.First(&user, userID).Error; err != nil {
		return nil, errors.New("user_not_found")
	}

	// Optimistic locking — so updated_at client đã đọc với DB (D6, pkg/optlock)
	if input.UpdatedAt != user.UpdatedAt {
		return nil, errs.ErrVersionConflict
	}

	oldSnap := history.SnapshotRow(s.DB, "user", userID, history.UserIncludeFields)

	updates := map[string]interface{}{
		"updated_by": operatorID, // updated_at: callback optlock bump GREATEST(now, updated_at+1)
	}

	statusRevoke := false // chỉ status thay đổi mới cần forced logout

	if input.Username != nil {
		updates["username"] = *input.Username
	}
	if input.Status != nil {
		if *input.Status != user.Status {
			statusRevoke = true
		}
		updates["status"] = *input.Status
	}
	if input.BranchId != nil {
		updates["branchId"] = string(input.BranchId)
	}
	if input.AllowedIps != nil {
		updates["allowed_ips"] = string(input.AllowedIps)
	}

	tx := s.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Ghi có khoá: WHERE updated_at = ? + bump; 0 dòng ⇒ ErrVersionConflict
	if _, err := optlock.Updates(tx.Model(&hrUserDao.User{}).Where("id = ?", userID), input.UpdatedAt, updates); err != nil {
		tx.Rollback()
		return nil, err
	}

	// Update profile
	if input.UserProfile != nil {
		var profileUpdates map[string]interface{}
		if err := json.Unmarshal(input.UserProfile, &profileUpdates); err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("unmarshal user profile: %w", err)
		}
		// Updates(map) ghi thẳng key thành cột ⇒ CHỈ giữ cột trong whitelist. Key lạ bỏ im lặng
		// (FE cũ còn gửi `companyActive_id` — bảng không còn cột, để lại là lỗi 1054).
		for k := range profileUpdates {
			if !hrProfileEditableFields[k] {
				delete(profileUpdates, k)
			}
		}
		if len(profileUpdates) > 0 {
			// Fullname → title case. golang.org/x/text/cases thay strings.Title (Go 1.18+
			// deprecated cho Unicode handling).
			if fn, ok := profileUpdates["fullname"].(string); ok {
				profileUpdates["fullname"] = cases.Title(language.Vietnamese).String(strings.ToLower(strings.TrimSpace(fn)))
			}
			// birthday: map update KHÔNG chạy qua GORM BeforeSave hook (hook chỉ tác động
			// struct truyền cho .Model(), không phải map .Updates()) — normalize thủ công ở
			// đây để tránh MySQL Error 1292 khi FE gửi chuỗi ngày sai dạng (đồng bug với
			// crm/customer đã fix, incident 2026-08-19). Chuỗi rỗng/không hợp lệ → NULL.
			if bd, ok := profileUpdates["birthday"].(string); ok {
				if normalized := helpers.NormalizeDateOnly(bd); normalized == "" {
					profileUpdates["birthday"] = nil
				} else {
					profileUpdates["birthday"] = normalized
				}
			}
			// JSON fields: re-marshal slices/maps to JSON string for GORM
			for k, v := range profileUpdates {
				switch v.(type) {
				case []interface{}, map[string]interface{}:
					b, _ := json.Marshal(v)
					profileUpdates[k] = string(b)
				}
			}
			tx.Model(&hrUserDao.UserProfile{}).Where("user_id = ?", userID).Updates(profileUpdates)
		}
	}

	// Update role
	if input.Role != nil && *input.Role != "" {
		var currentRole string
		tx.Table("rbac_auth_assignment").Select("item_name").Where("user_id = ?", userID).Scan(&currentRole)
		if currentRole != *input.Role {
			tx.Exec("DELETE FROM rbac_auth_assignment WHERE user_id = ?", userID)
			tx.Exec("INSERT INTO rbac_auth_assignment (item_name, user_id, created_at) VALUES (?, ?, ?)",
				*input.Role, userID, time.Now().Unix())
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	// Chỉ forced logout khi status thay đổi — user bị vô hiệu hóa không được tiếp tục dùng app.
	// branch/role/department: BE enforce per-request (UserScope + RBAC cache cleared),
	// FE sync qua BumpUserAuthVersion bên dưới — không cần logout.
	if statusRevoke && s.TokenSvc != nil {
		s.TokenSvc.RevokeAllSessions(userID)
		s.TokenSvc.RevokeAllRefreshTokens(userID)
	}

	// Bump auth version sau mọi update — FE detect qua X-User-Auth-Version → re-fetch /me.
	// Kể cả needRevoke=true: nếu JTI revoke fail, header vẫn là safety net.
	middleware.BumpUserAuthVersion(s.Cache, userID)

	s.invalidateCache(userID)

	newSnap := history.SnapshotRow(s.DB, "user", userID, history.UserIncludeFields)
	_ = history.LogChanges(s.DB, "log_user", "user_id", int(userID), oldSnap, newSnap, history.UserIncludeFields, history.UserSkipNullFields, int(operatorID))

	s.DB.First(&user, userID)
	return &user, nil
}

// ── Change Password ──

// mustChange — giá trị mới của cờ user.must_change_password (S-4/D-1): người dùng TỰ đổi ⇒ false
// (gỡ cờ); người khác (admin) đặt lại/đổi hộ ⇒ true (người đặt biết mật khẩu ⇒ chủ tài khoản phải
// đổi lại ở lần đăng nhập kế). Cache user_scope bị xoá trong invalidateCache ⇒ middleware thấy cờ mới
// ngay request sau.
func (s *UserService) ChangePassword(userID uint, newPassword string, mustChange bool) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), 12)
	if err != nil {
		return err
	}
	mustChangeVal := define.UserMustChangePasswordNo
	if mustChange {
		mustChangeVal = define.UserMustChangePasswordYes
	}
	if err := s.DB.Model(&hrUserDao.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{
			"password_hash":        string(hash),
			"must_change_password": mustChangeVal,
			"updated_at":           time.Now().Unix(),
		}).Error; err != nil {
		return err
	}
	if s.TokenSvc != nil {
		s.TokenSvc.RevokeAllSessions(userID)      // revoke access tokens (JTI blacklist)
		s.TokenSvc.RevokeAllRefreshTokens(userID) // revoke refresh tokens (prevent silent re-auth bypass)
	}
	s.invalidateCache(userID)
	return nil
}

// ── Get User By ID ──

func (s *UserService) GetUserByID(id uint) (*hrUserDao.User, *hrUserDao.UserProfile, error) {
	var user hrUserDao.User
	if err := s.DB.First(&user, id).Error; err != nil {
		return nil, nil, err
	}
	var profile hrUserDao.UserProfile
	s.DB.Where("user_id = ?", id).First(&profile)
	return &user, &profile, nil
}

// ── Get All Users (for dropdown/sync) ──

// GetAllUsers — story-58: bỏ lọc JSON `u.companyId` (1 thương hiệu/DB) ⇒ luôn 1 khoá cache chung
// (trước đây chỉ cache ở nhánh không lọc company), `invalidateCache` xoá đúng khoá này.
func (s *UserService) GetAllUsers() ([]map[string]interface{}, error) {
	const cacheKey = "getAllUsers"

	if s.Cache != nil {
		var cached []map[string]interface{}
		if err := s.Cache.Get(cacheKey, &cached); err == nil {
			storage.ResolveRows(cached, "avatar")
			return cached, nil
		}
	}

	q := s.DB.Table("user_profile p").
		Select("p.user_id, p.fullname, p.nickname, p.avatar, p.departmentList, u.status, u.branchId").
		Joins("JOIN user u ON u.id = p.user_id").
		Joins("JOIN rbac_auth_assignment a ON a.user_id = u.id").
		// Gồm cả admin (id 1) — xem ghi chú "users" ở config/indexeddb/handler/sync.go.
		Where("a.item_name <> 'partner' AND u.status = ?", define.UserStatusActive)

	var rows []map[string]interface{}
	tx := q.Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		return nil, tx.Error
	}

	if s.Cache != nil {
		s.Cache.Set(cacheKey, rows, 30*time.Minute) // cache giữ KEY; URL ghép lúc trả (đổi CDN không phải xoá cache)
	}
	storage.ResolveRows(rows, "avatar")
	return rows, nil
}

// ── Get Roles Dropdown ──

func (s *UserService) GetRolesDropdown() ([]map[string]interface{}, error) {
	var roles []map[string]interface{}
	tx := s.DB.Table("rbac_auth_item").
		Select("name as value, description as label").
		Where("type = 1 AND name NOT IN ('admin', 'develop', 'partner')").
		Find(&roles)
	if pagination.IsQueryError(tx.Error) {
		return nil, tx.Error
	}
	return roles, nil
}

// ── Get User List (active + inactive) ──

func (s *UserService) GetUserList() ([]map[string]interface{}, error) {
	var rows []map[string]interface{}
	tx := s.DB.Table("user").Where("status IN (?, ?)", define.UserStatusActive, define.UserStatusInactive).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		return nil, tx.Error
	}
	return rows, nil
}

// ── Delete User ──

func (s *UserService) DeleteUser(userID uint, operatorID uint) error {
	oldSnap := history.SnapshotRow(s.DB, "user", userID, history.UserIncludeFields)

	// Soft delete: set status = 1 (STATUS_DELETED), match Yii2
	result := s.DB.Model(&hrUserDao.User{}).Where("id = ?", userID).
		Updates(map[string]interface{}{"status": define.UserStatusDeleted, "updated_at": time.Now().Unix()})
	if result.RowsAffected == 0 {
		return errors.New("employee_not_found")
	}
	if s.TokenSvc != nil {
		s.TokenSvc.RevokeAllSessions(userID)
		s.TokenSvc.RevokeAllRefreshTokens(userID)
	}
	s.invalidateCache(userID)

	// Soft-delete chỉ đổi field "status" — dùng LogChangesWithAction (diff thật, chỉ field
	// thay đổi mới xuất hiện) thay vì LogDelete (snapshot toàn bộ field về nil), tránh audit
	// trail hiển thị sai username/branchId như bị xóa dù chúng không hề đổi.
	newSnap := history.SnapshotRow(s.DB, "user", userID, history.UserIncludeFields)
	_ = history.LogChangesWithAction(s.DB, "log_user", "user_id", int(userID), oldSnap, newSnap, history.UserIncludeFields, history.UserSkipNullFields, int(operatorID), history.ActionDelete)
	return nil
}

// ── Cache Invalidation ──

func (s *UserService) invalidateCache(userID uint) {
	if s.Cache == nil {
		return
	}
	id := fmt.Sprintf("%d", userID)
	keys := []string{
		"getAllUsers",
		"getListUserActive",
		"get-list-user-pause",
		"get-list-user-filter",
		"list-user-for-dropdown",
		"getUserProfileById-" + id,
		"rbac-" + id,
		"user_scope:" + id,
	}
	for _, k := range keys {
		s.Cache.Delete(k)
	}
	s.Cache.InvalidateTag("user_profile")
	s.Cache.InvalidateTag("user")
	s.Cache.InvalidateTag("user:" + id)
	s.Cache.InvalidateTag("rbac:user:" + id)
	s.Cache.InvalidateTag("getListUser")
	s.Cache.InvalidateTag("getUserByRole")
	s.Cache.InvalidateTag("getListUserByRole")

	dexie.BumpDexieTable(s.DB, "users")
}

// InvalidateAllUserCache clears all user-related caches
func (s *UserService) InvalidateAllUserCache() {
	if s.Cache == nil {
		return
	}
	keys := []string{
		"getAllUsers",
		"getListUserActive",
		"get-list-user-pause",
		"get-list-user-filter",
		"list-user-for-dropdown",
	}
	for _, k := range keys {
		s.Cache.Delete(k)
	}
	s.Cache.InvalidateTag("user_profile")
	s.Cache.InvalidateTag("user")
	s.Cache.InvalidateTag("getListUser")
	s.Cache.InvalidateTag("getUserByRole")
	dexie.BumpDexieTable(s.DB, "users")
}
