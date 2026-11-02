package tenant

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Story-41 (E6/D15 — 1 thương hiệu/DB): registry bảng có company_id + mọi API scope theo company
// (bản deprecated, bản company-only cho FK lookup, bản raw SQL, bản cột JSON) ĐÃ XOÁ — phạm vi dữ
// liệu chỉ còn chi nhánh (story-62: schema mới bỏ hẳn cột company_id). `branchTables` tỉa luôn
// 24 bảng của module đã xoá (quỹ, hoá đơn, bút toán, HR, phê duyệt, omnichat…) — 0 điểm gọi.

// Tables có branch_id (verified against DB schema)
//
// ⚠ ĐÃ THỬ VÀ GỠ: `warehouse` (2026-09-09). Trên DB dev nó có `branch_id` nên trông như thiếu
// scope, nhưng DB dựng từ MIGRATION chỉ có `branchId` (camelCase) — `branch_id` là cột THỪA của
// riêng dev. Đăng ký vào đây ⇒ helper sinh `warehouse.branch_id = ?` ⇒ **lỗi 1054 Unknown column
// trên mọi DB khách**, chết cả `ListWarehouse` lẫn đồng bộ master-data xuống IndexedDB. Muốn scope
// bảng này phải cho helper biết TÊN CỘT khác, không phải thêm tên bảng vào danh sách.
// Bài học chung: kiểm cột scope trên DB dựng-từ-migration, KHÔNG kiểm trên dev.
//
// ⚠ CÓ cột `branch_id` KHÔNG tự động nghĩa là phải nằm ở đây. Chỉ đăng ký khi `branch_id` là
// QUYỀN SỞ HỮU của dòng dữ liệu. `gcal_subscription` là phản ví dụ đã cân nhắc (2026-09-09):
// `branch_id` ở đó là THAM SỐ của đăng ký ("đồng bộ lịch hẹn của chi nhánh nào"), không phải chi
// nhánh sở hữu — đăng ký vào đây sẽ làm chính chủ mất đăng ký của mình khỏi danh sách ngay khi họ
// đổi chi nhánh đang hoạt động. (Module gcal đã xoá — ghi chú giữ làm bài học.)
var branchTables = map[string]bool{
	"appointment":               true,
	"config_code":               true,
	"customer":                  true,
	"order_commission_telesale": true,
	"order_revenue_telesale":    true,
	"order_service_used":        true,
	"order_plan":                true,
	"payment":                   true,
}

// ScopedDBNullBranch — list scope theo phân quyền chi nhánh của user:
//
//   - branchActive != null  → branch_id = activeId OR IS NULL
//   - branchActive == null  → branch_id IN user.branchIds OR IS NULL
//
// "+ NULL" = record chưa gán branch (phải hiện để user gán hoặc record áp dụng mọi
// branch). NULL LUÔN được lấy, kể cả khi đang active.
// Story-35 (D15 — 1 thương hiệu/DB): KHÔNG còn vế company; tên hàm giữ nguyên để 162
// caller không phải sửa. Thân hàm: applyNullBranchScope (scope_explicit.go).
// optCrossBranchID là user_option id=18 ("Tìm kiếm & xem chéo dữ liệu giữa các cơ sở"):
// khi ON, bỏ branch filter cho global search. KHÔNG áp dụng cho report/list/EMR —
// những endpoint đó luôn branch-scoped bất kể option này.
const optCrossBranchID = 18

// ctxKeyCrossBranch là key lưu cache per-request trong gin.Context.
const ctxKeyCrossBranch = "_opt_cross_branch"

// IsCrossBranchEnabled trả true khi user_option id=18 = 1 (ON).
// Cache trong gin.Context để chỉ query DB 1 lần/request.
// Chỉ dùng cho global search (SearchGlobal) — KHÔNG dùng cho report/list/dashboard/EMR.
func IsCrossBranchEnabled(c *gin.Context, db *gorm.DB) bool {
	if v, ok := c.Get(ctxKeyCrossBranch); ok {
		return v.(bool)
	}
	var val int
	db.Table("user_option").Where("id = ?", optCrossBranchID).Limit(1).Pluck("options", &val)
	enabled := val == 1
	c.Set(ctxKeyCrossBranch, enabled)
	return enabled
}

// ScopeFunc khớp chữ ký ScopedDBNullBranch/CrossBranchLookupScope — dùng khi caller cần NHẬN scope
// function làm tham số (story-33: gộp các bản fork attachRelation* khác nhau ở khoản chọn
// ScopedDB* nào, xem pkg/handlerutil).
type ScopeFunc func(c *gin.Context, db *gorm.DB, table string, alias ...string) *gorm.DB

func ScopedDBNullBranch(c *gin.Context, db *gorm.DB, table string, alias ...string) *gorm.DB {
	return scopedDBNullBranch(c, db, table, false, alias...)
}

// ScopedDBNullBranchSkipBranch là ScopedDBNullBranch nhưng bỏ qua branch filter khi skipBranch=true.
// Dùng cho các flow cố ý bypass branch scope (khác id=18 cross-branch chung) — ví dụ đơn hàng /
// dịch vụ đã dùng / thanh toán của khách luôn dùng được ở mọi chi nhánh (`true`; option 23 cũ đã gỡ).
// KHÔNG dùng tùy tiện: chỉ bypass đúng phạm vi flow cần, không thay ScopedDBNullBranch ở nơi khác.
func ScopedDBNullBranchSkipBranch(c *gin.Context, db *gorm.DB, table string, skipBranch bool, alias ...string) *gorm.DB {
	return scopedDBNullBranch(c, db, table, skipBranch, alias...)
}

func scopedDBNullBranch(c *gin.Context, db *gorm.DB, table string, skipBranch bool, alias ...string) *gorm.DB {
	// Thân hàm nằm ở scope_explicit.go — dùng chung với đường không-context (cron/worker).
	// Ở đây chỉ làm đúng một việc: đọc phạm vi ra khỏi session.
	return applyNullBranchScope(db, BoundsFromContext(c), table, skipBranch, alias...)
}

// CrossBranchLookup — FK lookup CỐ Ý KHÔNG lọc branch (cross-branch relation loading): vd
// customer.branch_id = chi nhánh đăng ký, khác chi nhánh của lịch hẹn/dịch vụ/thanh toán đang xem;
// lọc NullBranch ở đây sẽ làm mất customer cross-branch.
//
// Là hàm identity (trả nguyên db) — tồn tại để GIỮ DẤU Ý ĐỒ: reviewer/grep phân biệt được
// "cố ý không lọc branch" với "quên scope". Thay helper FK-lookup company-only cũ (story-36..38 đổi
// caller, story-41 xoá tên cũ) — sau D15 (1 thương hiệu/DB) vế company của hàm cũ không còn ý nghĩa.
//
// `table`/`alias` không dùng tới nhưng giữ để call site đọc như mọi scope khác.
//
// KHÔNG dùng cho list query tự do (search/paginate) hay GET/UPDATE/DELETE theo id → ScopedDBNullBranch.
func CrossBranchLookup(db *gorm.DB, table string, alias ...string) *gorm.DB {
	return db
}

// CrossBranchLookupScope — CrossBranchLookup dưới dạng ScopeFunc, cho caller NHẬN scope function
// (handlerutil.AttachRelation*) — story-36 thêm, story-41 thành mặc định của handlerutil.
func CrossBranchLookupScope(_ *gin.Context, db *gorm.DB, table string, alias ...string) *gorm.DB {
	return CrossBranchLookup(db, table, alias...)
}

func getUintFromCtx(c *gin.Context, key string) uint {
	v, _ := c.Get(key)
	if id, ok := v.(uint); ok {
		return id
	}
	return 0
}

func getUintSliceFromCtx(c *gin.Context, key string) []uint {
	v, _ := c.Get(key)
	if ids, ok := v.([]uint); ok {
		return ids
	}
	return nil
}
