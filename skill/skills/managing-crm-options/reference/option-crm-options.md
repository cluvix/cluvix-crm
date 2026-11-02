# Option CRM — Chi tiết option 1, 2, 3, 5, 6, 12, 16

## Mục lục

- BE enforcement — pattern "creator + same-day" (options id 1, 2, 3)
  - Công thức chung
  - 3 module đã enforce (2026-06-01)
  - Khi thêm module mới cùng pattern
  - branchAllowed — write-path security khi option 18 bật
  - Timestamp gotcha — đã fix toàn bộ 3 module
  - created_at — field dùng để check same-day (tất cả 3 module)
  - Logic
  - Shared helper (2026-06-01)
  - Endpoints đã apply
  - ⚠ List và Detail PHẢI lọc song song — lệch nhau là ra 404 câm
  - Khi thêm endpoint mới trả về lead/customer
  - FE
- Option 12 — OPTION_UPDATE_CUSTOMER_FOR_TELESALE — Cập nhật chéo khách hàng
  - Logic
  - Implement (2026-06-01)
  - Query order — tối ưu theo tần suất
  - Dùng phòng ban thay role
- Option 6 — OPTION_DELETE_NOTE — Xóa ghi chú trong ngày
  - Logic
  - Implement (2026-06-01)
  - Query order tối ưu
  - FE fix — timestamp bug
- Option 16 — OPTION_TIME_BEFORE — Chọn ngày trước đó
  - Logic
  - Scope
  - 12 FE form components dùng option này
  - Pattern trong mỗi form

---


BE enforcement pattern + chi tiết option id 1, 2, 3, 5, 6, 12, 16.

---

## BE enforcement — pattern "creator + same-day" (options id 1, 2, 3)

Options id=1/2/3 không chỉ ẩn button ở FE — BE **bắt buộc** enforce cùng logic để chặn API call trực tiếp.

### Công thức chung

```
Được phép nếu:
  có quyền "<update>" (e.g. updatePayment)
  HOẶC (có quyền "<create>" + option_id = 1 + created_by = userID + cùng ngày tạo theo TZ company)
Còn lại → 403 Forbidden
```

Permission check dùng `RbacSvc.GetUserPermissions(userID)` (Redis-cached, TTL 1h). `RbacSvc` inject qua `handler.Deps.RbacSvc` → wired từ `d.RbacSvc` trong `crm/router.go`.

### 3 module đã enforce (2026-06-01)

| Module | Option ID | FE check | BE verify fn | Update endpoints |
|---|---|---|---|---|
| Payment | 1 | `payment-of-customer.ts` → `checkPaymentUpdate()` | `payment/handler/helpers.go` → `verifyCanEditPaymentCustomer()` | `update-payment-customer`, `update-deposit-customer` |
| Order Plan | 3 | `order-plan-in-customer.ts` → `checkOrderPlanUpdate()` | `order_treatment_plan/handler/helpers.go` → `verifyCanEditOrderPlan()` | `PUT /:id` |
| Treatment | 2 | `treatment-course-list-in-customer.ts` → `checkTreatmentUpdate()` | `treatment_course/handler/helpers.go` → `verifyCanEditTreatmentCourse()` | `update-treatment-dental`, `update-treatment-beauty` |

Permission strings (FE define → BE rbac):

| Module | Perm update | Perm create |
|---|---|---|
| Payment | `updatePayment` | `createPayment` |
| Order Plan | `updatePlanOrder` | `createPlanOrder` |
| Treatment | `updateTreatmentCourse` | `createTreatmentCourse` |

### Khi thêm module mới cùng pattern

1. Thêm option ID vào bảng §2, thêm `verifyCanEdit<Module>` vào `helpers.go` (copy pattern từ payment)
2. Inject `RbacSvc` vào handler `Deps` + `Handler` struct
3. Wire `RbacSvc: d.RbacSvc` trong `crm/router.go`
4. Replace `verifyOwnership` bằng `verifyCanEdit<Module>` ở UPDATE handlers (KHÔNG thay ở DELETE)

### branchAllowed — write-path security khi option 18 bật

Khi `OPTION_CUSTOMER_CROSS_BRANCH` (id=18) ON, `ScopedDBNullBranch` bỏ branch filter → record cross-branch load được. Trong `verifyCanEdit*`, path creator+same-day không được phép ghi cross-branch. Mỗi `helpers.go` có `branchAllowed` helper:

```go
func branchAllowed(c *gin.Context, branchID int) bool {
    if branchID == 0 { return true } // unassigned
    if active := middleware.GetBranchActiveID(c); active > 0 {
        return int(active) == branchID
    }
    for _, bid := range middleware.GetBranchIDs(c) {
        if int(bid) == branchID { return true }
    }
    return false
}
```

Gọi trong `verifyCanEdit*` **sau** `hasPermission("update*")` check, **trước** `hasPermission("create*")`:

```go
if h.hasPermission(userID, "updatePayment") {
    return true // update perm → cross-branch write OK
}
// Creator+same-day path: branch check bắt buộc dù option 18 ON
if !branchAllowed(c, p.BranchId) { response.Forbidden(c); return false }
if !h.hasPermission(userID, "createPayment") { ... }
```

`Payment.BranchId` là `int`; `OrderTreatmentPlan.BranchId` và `TreatmentCourse.BranchId` là `*int` (deref cần nil check).

**CRM hiện tại (2026-11-01, gỡ option 23):** `verifyCanEditOrderPlan`/`verifyCanEditPaymentCustomer` KHÔNG còn check `branchAllowed` — đơn hàng/thanh toán của khách luôn sửa được ở mọi chi nhánh (hành vi option 23 ON, user chốt). `branchAllowed` chỉ còn ở `payment/handler/upsert.go` (chặn TẠO thanh toán vào chi nhánh ngoài phạm vi). Xem `skills/managing-crm-options/SKILL.md` §Option 21–23.

### Timestamp gotcha — đã fix toàn bộ 3 module

FE: `new Date(createdAt)` → **`new Date(createdAt * 1000)`** vì `created_at` từ BE là unix seconds. Bug này khiến same-day check **luôn fail** (kết quả là button edit không bao giờ hiện với staff dù option ON).

FE thêm: `treatment-course-list-in-customer.ts` dùng `this.toDay` (stale từ lúc init component) → đã fix thành `this.timezoneService.now()`.

### created_at — field dùng để check same-day (tất cả 3 module)

**Tất cả 3 module đều dùng `created_at`** (DB auto timestamp — khi record thực sự được insert).

`created_time` là ngày business user nhập (có thể backdate) → **không dùng** để check "tạo hôm nay". Yii2 dùng `created_time` cho Treatment/OrderPlan là bug — intent đúng phải là "record được tạo hôm nay" chứ không phải "ngày user nhập là hôm nay".

`created_by` check là **chủ ý** — chỉ người tạo ra record mới được sửa.

---

---

## Option 12 — OPTION_UPDATE_CUSTOMER_FOR_TELESALE — Cập nhật chéo khách hàng

### Logic

Kiểm soát việc telesale có được phép đổi `payment_user_id` (tư vấn viên) của customer sang người khác không.

- `options=0` (OFF, mặc định): telesale **không được** đổi `payment_user_id` sang người khác (trừ trường hợp ngoại lệ)
- `options=1` (ON): telesale **được phép** cập nhật chéo tự do

**Flow đầy đủ** (chạy khi `UpdateCustomer` có `payment_user_id` trong body):

```
payment_user_id nil/0 → skip
Đổi thành chính mình → OK
option id=12 = 1 (ON) → OK  ← check trước để tránh dept/target queries
Không thuộc phòng Telesale (id=3) → OK (admin/manager không bị chặn)
Thuộc phòng Telesale + option OFF:
  target user status = inactive/deleted → OK (claim lead từ người nghỉ việc)
  còn lại → 422 "customer_cross_update_not_allowed"
```

**Ngoại lệ "paused user"**: telesale vẫn được claim lead từ nhân sự đã nghỉ (`UserStatusInactive=9`, `UserStatusDeleted=1`), tránh lead bị "mồ côi".

### Implement (2026-06-01)

- **Helper**: `crm/customer/handler/helpers.go` → `validatePaymentUserIDChange(c, db, body)`
- **Gọi**: `crm/customer/handler/customer.go` → `UpdateCustomer()` trước khi delete body fields
- **i18n key**: `customer_cross_update_not_allowed` — đã thêm cả 4 locale
- **Error pattern**: `errors.New("key")` + `response.HandleError` (auto-translate, đúng chuẩn)

### Query order — tối ưu theo tần suất

| Thứ tự | Query | Lý do |
|---|---|---|
| 1 | `user_option` id=12 | Nếu ON → exit ngay, bỏ 2 query sau |
| 2 | `user_profile` dept check | Chỉ chạy nếu option OFF; non-telesale exit sớm |
| 3 | `user` status check | Chỉ chạy nếu là telesale + option OFF |

### Dùng phòng ban thay role

`HrDepartmentTelesale = 3` (`pkg/define/hr.go`) — role có thể bị xóa/đổi tên. Pattern `JSON_CONTAINS(departmentList, '3')` giống `phoneutil.CanSeePhone` và `leadscope`.

---

## Option 6 — OPTION_DELETE_NOTE — Xóa ghi chú trong ngày

### Logic

- `options=0` (OFF, mặc định): staff **không được** xóa note (trừ có `deleteNote` perm)
- `options=1` (ON): staff có thể xóa note mình tạo trong ngày hôm nay

**Flow đầy đủ:**
```
Có quyền deleteNote → OK
Không có quyền createNote → BLOCK
Không phải creator (created_by ≠ userID) → BLOCK  ← check trước option để tránh DB
option id=6 ≠ 1 → BLOCK
note.CreatedAt nil → BLOCK
Không cùng ngày (timezone company) → BLOCK
→ OK
```

### Implement (2026-06-01)

- **Helper**: `crm/note/handler/helpers.go` → `canDeleteNote(userID, note)`
- **Gọi**: `crm/note/handler/delete.go` → `DeleteNote()` → 403 nếu không đủ điều kiện
- **Wire**: `crm/router.go` → `noteHandler.Deps{DB: db, RbacSvc: d.RbacSvc}`

### Query order tối ưu

1. `getPermissions(userID)` — **1 Redis call**, check `deleteNote` + `createNote` cùng lúc
2. Creator check — **free** (pointer compare), non-creator exit trước khi chạm DB
3. `user_option` id=6 — **1 DB query** (chỉ khi là creator)
4. `tz.Now` — **in-memory cache** sau lần đầu, không tốn DB

### FE fix — timestamp bug

`app-note.ts` → `checkDeleteNote`: `new Date(createdAt)` → `new Date(Number(createdAt) * 1000)` vì `createdAt` có type `string | number | Date`, dùng `Number()` để convert an toàn trước khi `* 1000`.

---

## Option 16 — OPTION_TIME_BEFORE — Chọn ngày trước đó

### Logic

- `options=0` (OFF, default): user không có `permissionUpdate` bị **lock to today** trên datepicker `payment_time`
- `options=1` (ON): `minDate = undefined` → user có thể chọn bất kỳ ngày nào trong quá khứ

### Scope

**FE-only** — UX constraint trên datepicker, không phải security constraint. BE không enforce và không cần enforce. Authorization (ai được update) đã được xử lý bởi options 1/2/3.

**Developer only** trong UI: card hiển thị với `*hasPerm="permissionDev"`.

### 12 FE form components dùng option này

| Nhóm | Files |
|---|---|
| Payment | `form-payment`, `form-deposit`, `form-refund-deposit`, `form-refund-order-plan`, `form-refund-kg`, `form-move-deposit`, `form-installment`, `form-payment-order` |
| Treatment | `form-beauty`, `form-dental`, `form-rehab` |
| Order plan | `order-plan-form` |

### Pattern trong mỗi form

```typescript
this.optionsTimeBefore = !!this.userOptionHelper.getUserOptionInfo(
    CONFIG_OPTION_CRM_SELECT.OPTION_TIME_BEFORE,
)?.options;

// ngOnInit:
if (!permissionUpdate && !this.optionsTimeBefore) {
    this.minPaymentTime = this.timezoneService.todayStart(); // locked to today
}
// option ON → minPaymentTime = undefined → past dates available
```

---

> Options id 17–20 + gotchas → [`option-crm-options-p2.md`](option-crm-options-p2.md) · Options id 21–23 → [`skills/managing-crm-options/SKILL.md`](skills/managing-crm-options/SKILL.md) · Option 14 → [`skills/managing-crm-options/SKILL.md`](skills/managing-crm-options/SKILL.md)
