package testhelpers

import (
	"fmt"
	"testing"
	"time"

	"erpCluvix/pkg/define"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TestTenant holds the IDs of a self-consistent set of test records
// (branch, customer) for use across tests in a package.
type TestTenant struct {
	BranchID   uint
	CustomerID uint
}

// LastInsertID returns MySQL LAST_INSERT_ID() for the suite connection.
// GORM KHÔNG backfill primary key khi Create từ map (documented behavior) —
// mọi fixture tạo record bằng map phải lấy id qua hàm này ngay sau Create.
// An toàn vì Suite.New giới hạn SetMaxOpenConns(1): mọi query đi chung 1
// connection nên LAST_INSERT_ID() luôn thuộc đúng câu INSERT vừa chạy.
func LastInsertID(db *gorm.DB) uint {
	var id uint64
	db.Raw("SELECT LAST_INSERT_ID()").Scan(&id)
	return uint(id)
}

// CreateTestTenant inserts a minimal but FK-consistent set of records:
// branch → customer. Use the returned IDs when creating
// further test records (payments, appointments, etc.).
// FK checks must be disabled at the DB level (Suite.New does this automatically).
func CreateTestTenant(db *gorm.DB) (*TestTenant, error) {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now().Unix()

	branch := map[string]any{
		"name": "Branch " + suffix,
		// 10 chữ số CUỐI của nanotime (khuôn `CreateBranch` bên dưới đã sửa từ trước): 10 chữ số
		// ĐẦU chỉ đổi mỗi giây ⇒ hai tenant tạo trong CÙNG MỘT GIÂY sinh trùng mã. Vô hại khi
		// `branch.code` chưa có ràng buộc, nhưng migration 20260917160159 thêm `uq_branch_code`
		// ⇒ trùng mã thành `Error 1062` làm hỏng mọi test gọi CreateTestTenant 2 lần/giây.
		"code":       "BR" + suffix[len(suffix)-10:],
		"status":     1,
		"created_at": now,
		"updated_at": now,
	}
	if err := db.Table("branch").Create(&branch).Error; err != nil {
		return nil, fmt.Errorf("create branch: %w", err)
	}
	branchID := LastInsertID(db)

	stageID, err := DefaultCustomerStageID(db)
	if err != nil {
		return nil, err
	}
	customer := map[string]any{
		"fullname":    "Test Customer",
		"stage_id":    stageID,
		"created_day": now,
		"created_at":  now,
		"updated_at":  now,
	}
	if err := db.Table("customer").Create(&customer).Error; err != nil {
		return nil, fmt.Errorf("create customer: %w", err)
	}
	customerID := LastInsertID(db)
	if err := recordInitialStage(db, customerID, stageID, now); err != nil {
		return nil, err
	}

	return &TestTenant{
		BranchID:   branchID,
		CustomerID: customerID,
	}, nil
}

// DefaultCustomerStageID đọc id bước tiến trình mặc định (`customer_stage.is_default = 1`) từ DB test —
// KHÔNG hardcode id: seed của migration chia id theo AUTO_INCREMENT. Fixture mọi khách PHẢI gán
// `stage_id` (cột sẽ NOT NULL không DEFAULT ở pha contract, story-12).
//
// Cố ý KHÔNG gọi `customer_stage/service.DefaultStageID`: service đó kéo theo pkg/codegen, pkg/tenant,
// pkg/helpers… mà test white-box của chính các package ấy (vd `pkg/codegen`) lại import `testhelpers`
// ⇒ vòng phụ thuộc. Raw SQL ở đây là bản đọc tối thiểu cùng ngữ nghĩa.
func DefaultCustomerStageID(db *gorm.DB) (uint, error) {
	var ids []uint
	if err := db.Raw("SELECT id FROM customer_stage WHERE is_default = 1 LIMIT 1").Scan(&ids).Error; err != nil {
		return 0, fmt.Errorf("đọc bước mặc định customer_stage: %w", err)
	}
	if len(ids) == 0 || ids[0] == 0 {
		return 0, fmt.Errorf("customer_stage không có bước is_default = 1 (schema test thiếu seed migration?)")
	}
	return ids[0], nil
}

// MustDefaultCustomerStageID — bản `t.Fatalf` của DefaultCustomerStageID cho test tự dựng dòng khách.
func MustDefaultCustomerStageID(t testing.TB, db *gorm.DB) uint {
	t.Helper()
	id, err := DefaultCustomerStageID(db)
	if err != nil {
		t.Fatalf("MustDefaultCustomerStageID: %v", err)
	}
	return id
}

// recordInitialStage ghi 1 dòng `customer_stage_history` cho khách fixture (khởi tạo: from = NULL,
// source = tạo khách, changed_by = NULL) — khớp trạng thái khách do đường tạo thật sinh ra.
func recordInitialStage(db *gorm.DB, customerID, stageID uint, at int64) error {
	err := db.Exec(
		"INSERT INTO customer_stage_history (customer_id, from_stage_id, to_stage_id, source, changed_by, changed_at) VALUES (?, NULL, ?, ?, NULL, ?)",
		customerID, stageID, define.CustomerStageHistorySourceCreate, at).Error
	if err != nil {
		return fmt.Errorf("ghi lịch sử bước khách fixture: %w", err)
	}
	return nil
}

// FreshTenant creates an isolated tenant for a single test and registers
// t.Cleanup to delete the records when the test finishes. Use this in tests
// that mutate tenant state — it prevents cross-test pollution and enables
// safe t.Parallel().
//
// Example:
//
//	func TestSomething(t *testing.T) {
//	    tenant := testhelpers.FreshTenant(t, testDB)
//	    // tenant is unique to this test, auto-deleted afterwards.
//	}
func FreshTenant(t testing.TB, db *gorm.DB) *TestTenant {
	t.Helper()
	tenant, err := CreateTestTenant(db)
	if err != nil {
		t.Fatalf("FreshTenant: %v", err)
	}
	t.Cleanup(func() {
		// Best-effort cleanup — order matters for FKs even with checks disabled,
		// because cascading semantics are clearer when we delete leaves first.
		db.Exec("DELETE FROM payment WHERE customer_id = ?", tenant.CustomerID)
		// FK checks tắt ⇒ ON DELETE CASCADE của lịch sử bước KHÔNG chạy: xoá tay.
		db.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", tenant.CustomerID)
		db.Exec("DELETE FROM customer WHERE id = ?", tenant.CustomerID)
		db.Exec("DELETE FROM branch WHERE id = ?", tenant.BranchID)
	})
	return tenant
}

// WithTenantCols — nguồn DUY NHẤT gắn cột phạm vi (`branch_id`) vào 1 map fixture trước khi
// `db.Table(x).Create(map...)` (story-60; story-62 bỏ `company_id` theo schema mới).
func WithTenantCols(tenant *TestTenant, row map[string]any) map[string]any {
	if row == nil {
		row = map[string]any{}
	}
	row["branch_id"] = tenant.BranchID
	return row
}

// CreatePayment inserts a minimal payment record and returns its ID.
// BranchId (NOT NULL) is taken from tenant.BranchID.
func CreatePayment(db *gorm.DB, tenant *TestTenant, amount decimal.Decimal, paymentFor int) (uint, error) {
	now := time.Now().Unix()
	row := WithTenantCols(tenant, map[string]any{
		// `code` + `method` là NOT NULL không default ở schema SAU migration (SchemaAll). Baseline
		// còn lỏng nên fixture cũ chạy được; test nào dùng SchemaAll sẽ vỡ với
		// "Field 'code' doesn't have a default value" nếu thiếu 2 cột này.
		"code":         fmt.Sprintf("PT%d", time.Now().UnixNano()),
		"method":       1,
		"customer_id":  tenant.CustomerID,
		"amount":       amount,
		"payment_for":  paymentFor,
		"payment_time": now,
		"created_at":   now,
		"updated_at":   now,
	})
	if err := db.Table("payment").Create(&row).Error; err != nil {
		return 0, fmt.Errorf("create payment: %w", err)
	}
	return LastInsertID(db), nil
}

// SetCustomerStage ghi thẳng `customer.stage_id` trong DB, bỏ qua service (không kiểm bước chọn được,
// không ghi lịch sử). Dùng để dựng trạng thái "bước hiện tại" cho test — thay `SetCustomerStatus` cũ.
func SetCustomerStage(db *gorm.DB, customerID, stageID uint) error {
	return db.Table("customer").Where("id = ?", customerID).
		Updates(map[string]any{"stage_id": stageID}).Error
}

// ─────────────────────────────────────────────────────────────────────────────────────────────
// Fixture user — gộp từ 4 bản `mkUser` + 3 bản "user có phạm vi thật" (D5 + D76 của teamchat).
//
// Vì sao đưa lên đây: helper unexported KHÔNG dùng chung giữa package được, nên mỗi package test
// chép một bản. Hậu quả đã đo: hai bản dựng `status: 1` (= `define.UserStatusDeleted`) trong khi
// hai bản kia đã sửa sang `UserStatusActive` — cùng một khuôn, hai ngữ nghĩa trái ngược, và fan-out
// realtime lọc theo `user.status` nên bản sai làm test xanh/đỏ vì lý do sai.
// ─────────────────────────────────────────────────────────────────────────────────────────────

// CreateUser chèn một hàng `user` tối thiểu và đăng ký cleanup.
//
// `status` LUÔN là `define.UserStatusActive` (10) — KHÔNG bao giờ hằng 1 (`UserStatusDeleted`).
// `prefix` chỉ để username dễ đọc khi soi DB lúc debug; rỗng cũng chạy. `tenant` giữ trong chữ ký
// (caller hiện có) dù story-62 bỏ cột thương hiệu — user không còn gắn gì từ tenant.
func CreateUser(t testing.TB, db *gorm.DB, prefix string, tenant *TestTenant) uint {
	t.Helper()
	return createUserRow(t, db, prefix, 0, false)
}

// CreateScopedUser chèn `user` + `user_profile` sao cho `middleware.UserScopeFor` đọc ra ĐÚNG
// phạm vi mong muốn: branch active = `branchActive`.
//
// Dùng cho mọi test đi qua cửa quyền đọc phạm vi người dùng. CreateUser thuần KHÔNG đủ ở đó: thiếu `user_profile` thì phạm vi đọc ra
// bằng 0 và cửa quyền từ chối — test xanh vì lý do sai.
func CreateScopedUser(t testing.TB, db *gorm.DB, prefix string, tenant *TestTenant, branchActive uint) uint {
	t.Helper()
	return createUserRow(t, db, prefix, branchActive, true)
}

func createUserRow(t testing.TB, db *gorm.DB, prefix string, branchActive uint, scoped bool) uint {
	t.Helper()
	if prefix == "" {
		prefix = "tu"
	}
	now := time.Now().Unix()
	u := map[string]any{
		"username":      fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano()),
		"password_hash": "x",
		"status":        define.UserStatusActive,
		"created_at":    now,
		"updated_at":    now,
	}
	if branchActive != 0 {
		u["branchId"] = fmt.Sprintf("[%d]", branchActive)
	}
	if err := db.Table("user").Create(&u).Error; err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	id := LastInsertID(db)
	t.Cleanup(func() { db.Exec("DELETE FROM user WHERE id = ?", id) })

	if !scoped {
		return id
	}

	p := map[string]any{
		"user_id":         id,
		"fullname":        "NV " + fmt.Sprint(id),
		"branchActive_id": branchActive,
	}
	if err := db.Table("user_profile").Create(&p).Error; err != nil {
		t.Fatalf("CreateScopedUser (user_profile): %v", err)
	}
	t.Cleanup(func() { db.Exec("DELETE FROM user_profile WHERE user_id = ?", id) })
	return id
}

// CreateBranch chèn một hàng `branch` độc lập (không kèm customer như FreshTenant) và
// đăng ký cleanup. Dùng khi test cần chi nhánh THỨ HAI để dựng ca "khác chi nhánh".
func CreateBranch(t testing.TB, db *gorm.DB) uint {
	t.Helper()
	now := time.Now().Unix()
	// 10 chữ số CUỐI của nanotime: 10 chữ số ĐẦU chỉ đổi mỗi giây nên hai chi nhánh tạo trong
	// cùng một giây sẽ trùng mã (khuôn cũ của CreateTestTenant).
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	row := map[string]any{
		"name":       "Branch " + suffix,
		"code":       "BR" + suffix[len(suffix)-10:],
		"status":     1,
		"created_at": now,
		"updated_at": now,
	}
	if err := db.Table("branch").Create(&row).Error; err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	id := LastInsertID(db)
	t.Cleanup(func() { db.Exec("DELETE FROM branch WHERE id = ?", id) })
	return id
}

// CreateScopedCustomer chèn một `customer` THẬT (thuộc `branchID` nếu khác
// nil) rồi đăng ký cleanup. `branchID == nil` ⇒ cột `branch_id` để NULL — ca "khách chưa gắn chi
// nhánh", thứ cửa quyền phạm vi phải xử đúng chứ không được coi như thuộc mọi chi nhánh.
func CreateScopedCustomer(t testing.TB, db *gorm.DB, tenant *TestTenant, branchID *uint) uint64 {
	t.Helper()
	now := time.Now().Unix()
	stageID := MustDefaultCustomerStageID(t, db)
	row := map[string]any{
		"fullname":    fmt.Sprintf("KH %d", time.Now().UnixNano()),
		"stage_id":    stageID,
		"created_day": now,
		"created_at":  now,
		"updated_at":  now,
	}
	if branchID != nil {
		row["branch_id"] = *branchID
	}
	if err := db.Table("customer").Create(&row).Error; err != nil {
		t.Fatalf("CreateScopedCustomer: %v", err)
	}
	id := LastInsertID(db)
	if err := recordInitialStage(db, id, stageID, now); err != nil {
		t.Fatalf("CreateScopedCustomer: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", id)
		db.Exec("DELETE FROM customer WHERE id = ?", id)
	})
	return uint64(id)
}
