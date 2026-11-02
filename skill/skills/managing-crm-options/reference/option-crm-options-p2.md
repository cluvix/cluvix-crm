# Option CRM — Chi tiết option 17–20

## Mục lục

- Option 17 — OPTION_ALLOW_PREVIOUS_DAY — Cho phép chọn ngày hôm qua
  - Bối cảnh
  - Logic
  - Scope — FE-only
  - 12 FE form components đã apply (2026-06-03)
  - Pattern trong mỗi form
  - Không conflict với options 1/2/3
  - Migration
- Option 18 — OPTION_CUSTOMER_CROSS_BRANCH — Tìm kiếm & xem chéo dữ liệu giữa các cơ sở
  - Logic
  - Implement — `pkg/tenant/scope.go`
  - Endpoints tôn trọng option 18 (READ path)
  - Write-path security
  - Migration
  - FE
  - Changelog
  - Logic
  - Scope — BE-only
  - Implement
  - Không ảnh hưởng Bước 2
  - Migration
  - FE
- Option 20 — OPTION_EXPORT_PHONE — Hiển thị SĐT khi export
  - Logic
  - Implement
  - FE
  - Migration
  - Khi thêm export file mới có SĐT khách hàng
- Gotcha / pitfalls

---


Options id 17–20 + gotchas. Tiếp theo từ `option-crm-options.md`. Options id 21–23 → `skills/managing-crm-options/SKILL.md`.

---

## Option 17 — OPTION_ALLOW_PREVIOUS_DAY — Cho phép chọn ngày hôm qua

### Bối cảnh

Staff nhập đơn trễ cuối ngày (chưa kịp tạo trước 00:00) cần chọn ngày hôm qua cho `payment_time`. Option này cho phép điều đó mà không cần bật OPTION_TIME_BEFORE (dev-only).

### Logic

- `options=0` (OFF, default): chỉ chọn được hôm nay (`todayStart`)
- `options=1` (ON): chọn được hôm qua + hôm nay (`yesterdayStart`)

**Priority chain đầy đủ (FE datepicker `minDate`):**
```
có permissionUpdate           → minDate = undefined (bất kỳ ngày)
OPTION_TIME_BEFORE ON (dev)   → minDate = undefined
OPTION_ALLOW_PREVIOUS_DAY ON  → minDate = yesterdayStart()  ← mới
else                          → minDate = todayStart()
```

### Scope — FE-only

Không cần BE enforcement — đây là UX constraint cho datepicker, không ảnh hưởng authorization. Consistent với OPTION_TIME_BEFORE.

### 12 FE form components đã apply (2026-06-03)

| Nhóm | Files |
|---|---|
| Payment | `form-payment`, `form-deposit`, `form-installment`, `form-move-deposit`, `form-payment-order`, `form-refund-deposit`, `form-refund-kg`, `form-refund-order-plan` |
| Treatment | `form-beauty`, `form-dental`, `form-rehab` |
| Order plan | `order-plan-form` |

### Pattern trong mỗi form

```typescript
// Field
optionsAllowPreviousDay: boolean = false;

// Constructor (sau OPTION_TIME_BEFORE)
this.optionsAllowPreviousDay = !!this.userOptionHelper.getUserOptionInfo(
    CONFIG_OPTION_CRM_SELECT.OPTION_ALLOW_PREVIOUS_DAY,
)?.options;

// ngOnInit — trong block if (!optionsTimeBefore):
this.minPaymentTime = this.optionsAllowPreviousDay
    ? this.timezoneService.yesterdayStart()
    : this.timezoneService.todayStart();
```

**Lưu ý**: Class property initialization (`minPaymentTime: Date = todayStart()`) ở đầu class KHÔNG được thay — đó là giá trị khởi tạo trước ngOnInit.

### Không conflict với options 1/2/3

Options 1/2/3 và option 17 kiểm soát **2 operation tách biệt**:

| | Options 1/2/3 | Option 17 |
|---|---|---|
| Scope | **EDIT mode** — `if (this.model())` branch | **CREATE mode** — `else` branch |
| Kiểm tra | `created_at` (ngày DB record được tạo) | `minDate` của datepicker khi tạo mới |
| Khi bật | Staff được sửa record trong ngày `created_at` | Staff được chọn `payment_time = hôm qua` |

Kịch bản: staff tạo payment lúc 1am ngày T với `payment_time = T-1` (nhờ option 17) → `created_at = T`. Khi sửa trong ngày T: options 1/2/3 check `created_at = T == today` → **được sửa**. Form edit lock minDate/maxDate theo `payment_time = T-1` → chỉ chỉnh giờ trong ngày T-1. Logic đúng.

### Migration

`20260603085200_add_user_option_allow_previous_day` — INSERT id=17, `options=0` default, `ON DUPLICATE KEY UPDATE` idempotent.

---

## Option 18 — OPTION_CUSTOMER_CROSS_BRANCH — Tìm kiếm & xem chéo dữ liệu giữa các cơ sở

**Phạm vi: CHỈ global search (`SearchGlobal`), không áp dụng cho report/list/EMR/dashboard** — đúng theo tên hiển thị FE. Trước 2026-07-28 option này từng bị dùng lan sang `visit_history` và `crm/report/handler/crm.go`, đã refactor bỏ vì sai phạm vi (xem Changelog cuối mục).

### Logic

- `options=0` (OFF, default): search chỉ thấy khách hàng chi nhánh mình (behavior cũ)
- `options=1` (ON): search thấy khách hàng mọi chi nhánh trong cùng company

Company scope (`company_id`) giữ nguyên — chỉ bỏ branch filter, và chỉ trong `SearchGlobal`.

### Implement — `pkg/tenant/scope.go`

`IsCrossBranchEnabled` còn trong `scope.go` nhưng **KHÔNG còn được gọi bởi `ScopedDBNullBranch`** (đã bỏ 2026-06-30):

```go
const optCrossBranchID = 18
const ctxKeyCrossBranch = "_opt_cross_branch"

// IsCrossBranchEnabled — per-request gin.Context cache.
func IsCrossBranchEnabled(c *gin.Context, db *gorm.DB) bool {
    if v, ok := c.Get(ctxKeyCrossBranch); ok { return v.(bool) }
    var val int
    db.Table("user_option").Where("id = ?", optCrossBranchID).Limit(1).Pluck("options", &val)
    enabled := val == 1
    c.Set(ctxKeyCrossBranch, enabled)
    return enabled
}
```

`ScopedDBNullBranch` **luôn** áp dụng branch scope — option 18 không override được nữa.

### Endpoints tôn trọng option 18 (READ path)

**Chỉ 1 nơi** gọi `tenant.IsCrossBranchEnabled` — tự build scope tường minh, bỏ branch filter khi option 18 ON:

| Nơi | File |
|---|---|
| `GET /crm/customer/search-global` → `SearchGlobal` | `crm/customer/handler/customer.go` |

EMR visit history (`emr/visit_history/handler/handler.go`) và CRM report (`crm/report/handler/crm.go`, qua `buildScopeFilter`) **KHÔNG còn gọi** `IsCrossBranchEnabled` (bỏ 2026-07-28) — luôn branch-scoped bất kể option 18. Workspace dashboard chưa từng dùng.

Pattern (ví dụ `SearchGlobal`, hiện tại — D15/story-36 đã bỏ vế company, chỉ còn branch):

```go
// Branch scope: bỏ qua khi user_option id=18 (cross-branch) bật.
// Ngoặc tường minh: GORM chỉ tự bọc vế OR khi WHERE có ≥2 vế — không dựa vào đó.
if !tenant.IsCrossBranchEnabled(c, h.DB) {
    if branchActiveID := middleware.GetBranchActiveID(c); branchActiveID > 0 {
        q = q.Where("(customer.branch_id = ? OR customer.branch_id IS NULL)", branchActiveID)
    } else if branchIDs := middleware.GetBranchIDs(c); len(branchIDs) > 0 {
        q = q.Where("customer.branch_id IN ? OR customer.branch_id IS NULL", branchIDs)
    }
}
```

**KHÔNG mở rộng option 18 sang endpoint mới ngoài search** (đúng bug đã fix 2026-07-28). Cần cross-branch cho report/list/EMR → phải là 1 `user_option` riêng (như option 23 cũ — đã gỡ 2026-11-01), không tái sử dụng id=18.

### Write-path security

WRITE path (creator+same-day) vẫn enforce `branchAllowed` — xem `option-crm-options.md` §BE enforcement, phần "branchAllowed".

### Migration

`20260605160000_add_user_option_cross_branch.up.sql` — INSERT id=18, `options=0` default, `ON DUPLICATE KEY UPDATE`.

### FE

- Constant: `CONFIG_OPTION_CRM_SELECT.OPTION_CUSTOMER_CROSS_BRANCH = 18` trong `config-general.define.ts`
- UI: `option-crm.ts` + `option-crm.html` — card i18n key `config.option_crm.cross_search_title` = "Tìm kiếm & xem chéo dữ liệu giữa các cơ sở" (`categoryCrossBranch` radio group)
- FE không cần enforce — chỉ ảnh hưởng `SearchGlobal`

### Changelog

- 2026-07-28: bỏ `IsCrossBranchEnabled` khỏi `visit_history/handler.go` (2 chỗ) và `crm/report/handler/crm.go` (`buildScopeFilter`, dùng bởi 6 report handler) — 2 nơi này trước đó lỡ dùng option 18 dù không phải search, gây report/EMR bị lệch scope so với tên/ý nghĩa thật của option. Không thêm option thay thế (chấp nhận bỏ khả năng cross-branch report/EMR, không giữ đường lùi).

---

---

## Option 20 — OPTION_EXPORT_PHONE — Hiển thị SĐT khi export

### Logic

- `options=0` (OFF, mặc định): chỉ **BGĐ** (dept=9) thấy SĐT đầy đủ trong file export
- `options=1` (ON): tất cả user thấy SĐT đầy đủ trong file export

**Hoàn toàn độc lập với option 14** — option 14 kiểm soát list API (mask 4 số cuối); option 20 kiểm soát export Excel.

### Implement

**Hàm mới:** `internal/phoneutil/phone.go` → `CanSeePhoneExport(c, db)`

```go
func CanSeePhoneExport(c *gin.Context, db *gorm.DB) bool {
    var exportOpt int
    db.Table("user_option").Where("id = ?", 20).Limit(1).Pluck("options", &exportOpt)
    if exportOpt == 1 { return true }
    // OFF (default): chỉ BGĐ
    userID := middleware.GetUserID(c)
    var count int64
    db.Table("user_profile").
        Where("user_id = ? AND JSON_CONTAINS(departmentList, ?)", userID, fmt.Sprintf("%d", define.HrDepartmentBoardOfDirectors)).
        Count(&count)
    return count > 0
}
```

**Callsite đã thay:** `CanSeePhone` → `CanSeePhoneExport` trong:
- `internal/modules/webapp/export/customer.go` (3 chỗ)

**Export files KHÔNG áp dụng** (broker/coupon phone, không phải customer phone): `broker.go`, `coupon.go`, `appointment.go`, `order.go`

### FE

- Constant: `CONFIG_OPTION_CRM_SELECT.OPTION_EXPORT_PHONE = 20`
- UI: card "Hiển thị số điện thoại khi export", `categoryExportPhone` radio group
- FE không cần consume — logic hoàn toàn ở BE export

### Migration

`20260610120940_add_user_option_export_phone.up.sql` — INSERT id=20, `options=0` default, `ON DUPLICATE KEY UPDATE`.

### Khi thêm export file mới có SĐT khách hàng

Dùng `phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))` thay vì `CanSeePhone`.

---

## Gotcha / pitfalls

**`@for track category.value` là sai** — `category` objects có `{ name, key }`, không có `value`. Dùng `track category.key`. (Bug hiện tại trong template option-crm.html — chưa gây lỗi runtime vì Angular fallback undefined thành index, nhưng defeats mục đích track.)

**`selectedXxx` khai báo `!: number` nhưng gán object** — TypeScript không bắt vì `!` assertion. Thực tế giá trị là `{ name, key }` object, không phải `number`. Type đúng là `{ name: string; key: number } | undefined`.

**Không cần cache invalidate thủ công** — `BumpDexieTable("userOptions")` trong `UpdateOption` handler tự trigger sync.

**option-crm chỉ visible với `permissionDev`** cho card OPTION_TIME_BEFORE — dùng `*hasPerm="permissionDev"` directive.
