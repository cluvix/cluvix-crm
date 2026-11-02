---
name: managing-crm-options
description: >-
  Bảng `user_option` (global CRM system options), component `OptionCrm` và
  `UserOptionHelperService` — checklist thêm option mới, mask số điện thoại theo option
  14, cross-branch treatment option 23, option 21 tính tiền theo tổng gói. Dùng skill
  này KHI thêm/sửa 1 option CRM mới, KHI cần check `user_option` trong handler/component,
  hoặc trước khi đụng logic ẩn số điện thoại/cross-branch/thanh toán theo gói — tránh phá
  vỡ hành vi option đã có.
---

# Managing CRM Options

> reference/option-crm-options.md: chi tiết option 1–13 · reference/option-crm-options-p2.md:
> chi tiết option 15–20. SKILL.md này gồm tổng quan `user_option` (option-crm.md) + option 14
> mask phone (p4) + ghi chú option 21–23 đã gỡ.



Prompt cho mọi task liên quan đến `user_option` table, `OptionCrm` component, và `UserOptionHelperService`.

---

## 1. Tổng quan

`user_option` là bảng **global** — lưu các cài đặt hành vi toàn hệ thống của một cơ sở (không có `company_id`/`branch_id`). Mỗi row là 1 setting toggle với giá trị `options: int` (thường `0` = tắt, `1` = bật).

**KHÔNG dùng `tenant.ScopedDBNullBranch`** cho bảng này — bảng đã được whitelist trong scope.go, raw `h.DB` hoặc `db.Table("user_option")` là đúng.

---

## 2. Danh sách options hiện tại

| ID | Constant (FE) | Ý nghĩa | `options=0` | `options=1` |
|----|---|---|---|---|
| 1 | `OPTION_UPDATE_PAYMENT` | Cập nhật thanh toán | Không cho phép | Cho phép |
| 2 | `OPTION_UPDATE_TREATMENT` | Cập nhật điều trị | Không cho phép | Cho phép |
| 3 | `OPTION_UPDATE_ORDER_PLAN` | Cập nhật đơn hàng | Không cho phép | Cho phép |
| 5 | `OPTION_VIEW_LEAD_OF_TELE` | Lead của telesale | Chỉ thấy khách của mình | Thấy hết danh sách |
| 6 | `OPTION_DELETE_NOTE` | Xóa ghi chú | Không cho xóa | Cho phép xóa |
| 12 | `OPTION_UPDATE_CUSTOMER_FOR_TELESALE` | Telesale cập nhật khách | Không cho phép | Cho phép |
| 14 | `OPTION_HIDE_NUMBER_PHONE` | Hiển thị số điện thoại | Hiện toàn bộ | Chỉ 4 số cuối |
| 15 | `OPTION_USE_SOCKET_IO` | Socket.IO | (không hiển thị trong UI option-crm) | |
| 16 | `OPTION_TIME_BEFORE` | Chọn ngày trước đó | Không cho phép | Cho phép (dev only) |
| 17 | `OPTION_ALLOW_PREVIOUS_DAY` | Chọn ngày hôm qua | Không cho phép | Cho phép chọn T-1 |
| 18 | `OPTION_CUSTOMER_CROSS_BRANCH` | Hiển thị dữ liệu toàn cơ sở | Chỉ thấy cơ sở của mình | Tất cả cơ sở thấy nhau |
| 19 | `OPTION_AUTO_CARE_NEXT_DAY` | Tự động tạo chăm sóc ngày hôm sau | Không tạo (mặc định) | Tạo care T+1 sau mỗi điều trị |
| 20 | `OPTION_EXPORT_PHONE` | Hiển thị SĐT khi export | Chỉ BGĐ (dept=9) thấy SĐT | Tất cả user thấy SĐT |
| 21–23 | — | ĐÃ GỠ 2026-11-01 — xem §Option 21–23 cuối file | | |

**File constants:** `frontend/src/app/define/config-general.define.ts` → `CONFIG_OPTION_CRM_SELECT`

---

## 3. Backend

### API endpoints
```
GET  /config/options/get-options         → tất cả options (không cần tenant scope)
PUT  /config/options/update-option/:id   → update options value, tự BumpDexieTable("userOptions")
```

Handler: `internal/modules/webapp/config/options/handler/handler.go`  
Entity: `internal/modules/webapp/config/options/dao/entity.go` — `UserOption{ID, Name, Desc, Options int, SortOrder}`

### BE consumption — đọc option trong handler khác
Dùng raw DB query trực tiếp (không qua service):

```go
var optionVal int
db.Table("user_option").Where("id = ?", 14).Limit(1).Pluck("options", &optionVal)
// optionVal == 0 → tính năng tắt, optionVal == 1 → bật
```

Ví dụ thực tế: `internal/phoneutil/phone.go` — `CanSeePhone()` đọc id=14 để quyết định mask phone.

**Option 18 (OPTION_CUSTOMER_CROSS_BRANCH):** `IsCrossBranchEnabled` tồn tại trong `scope.go` nhưng **KHÔNG được gọi trong `ScopedDBNullBranch`**. Branch scope luôn áp dụng cho mọi endpoint. Ngoại lệ **duy nhất** (tự build scope thủ công + check option 18): `SearchGlobal` (`/crm/customer/search-global`) — đúng phạm vi tên gọi FE "Tìm kiếm & xem chéo dữ liệu giữa các cơ sở". `visit_history/handler.go` và `crm/report/handler/crm.go` đã bỏ check này 2026-07-28 (dùng sai phạm vi trước đó); `workspace/dashboard/handler.go` chưa từng dùng. Xem `option-crm-options-p2.md` §Option 18.

### IndexedDB sync
`BumpDexieTable(h.DB, "userOptions")` đã được gọi tự động sau mỗi `UpdateOption` — **không cần gọi thêm** khi sửa handler update.

---

## 4. Frontend

### FE consumption pattern — cách đọc option trong component

```typescript
import { UserOptionHelperService } from '@/indexeddb/config/components/user-option/user-option-helper.service';
import { CONFIG_OPTION_CRM_SELECT } from '@/define';

private userOptionHelper = inject(UserOptionHelperService);

// Trong ngOnInit hoặc async block — đảm bảo cache đã load
await this.userOptionHelper.ensureSettingLoaded();

// Đọc option
const info = this.userOptionHelper.getUserOptionInfo(CONFIG_OPTION_CRM_SELECT.OPTION_UPDATE_PAYMENT);
this.optionsUpdatePayment = !!info && info.options === 1;
```

`getUserOptionInfo(id)` trả `{ id, name, options } | null` từ IndexedDB cache.

### Nơi đang dùng (consumer list)

| File | Option ID | Biến |
|---|---|---|
| `emr/examination/tabs/order/emr-order-list.ts` | 3 (ORDER_PLAN) | `optionsUpdateOrderPlan` |
| `emr/examination/tabs/payment/emr-payment-list.ts` | 1 (PAYMENT) | `optionsUpdatePayment` |
| `emr/examination/tabs/treatment/emr-treatment-list.ts` | 2 (TREATMENT) | `optionsUpdateTreatment` |
| `crm/note/app-note/app-note.ts` | 6 (DELETE_NOTE) | `optionsDeleteNote` |
| `crm/order-plan/list/order-plan-in-customer.ts` | 3 (ORDER_PLAN) | `optionsUpdateOrderPlan` |
| `crm/order-plan/form/order-plan-form.ts` | 16 (TIME_BEFORE) | `optionsTimeBefore` |
| `crm/payment/form/form-payment/form-payment.ts` | 16 (TIME_BEFORE) | `optionsTimeBefore` |
| `crm/payment/form/form-deposit/form-deposit.ts` | 16 (TIME_BEFORE) | `optionsTimeBefore` |
| `crm/payment/form/form-refund-deposit/...` | 16 (TIME_BEFORE) | `optionsTimeBefore` |
| `crm/payment/form/form-refund-order-plan/...` | 16 (TIME_BEFORE) | `optionsTimeBefore` |
| `crm/payment/form/form-move-deposit/...` | 16 (TIME_BEFORE) | `optionsTimeBefore` |
| `crm/payment/form/form-installment/...` | 16 (TIME_BEFORE) | `optionsTimeBefore` |
| `core/services/crm/customer.service.ts` | (ensureSettingLoaded) | phone mask check |

**Option 14 (phone):** FE không check trực tiếp — BE đã mask tại server qua `phoneutil.MaskPhoneInItems`. FE nhận số đã mask sẵn.

### OptionsService (UI component riêng)
`option-crm` component dùng `OptionsService.getOptions()` trực tiếp (HTTP API), **không qua IndexedDB** — đây là config page, load 1 lần khi mở. Consumer khác (emr, crm forms) mới dùng IndexedDB cache.

---

## 4b. Chọn store cho cờ/config — FE đọc được gì

- FE chỉ sync được `user_option` (config/options) và bảng `setting` — KHÔNG có kênh đọc `config_systems`.
- Cờ/behavior mà FE cần biết → dùng `user_option`/`setting` ngay từ đầu; `config_systems` chỉ cho cờ thuần BE (guard server-side). Cờ đặt sai chỗ → phải chế cờ FE song song, 2 nguồn phải bật cùng lúc.

## 5. Thêm option mới — checklist

### BE
- [ ] Migration: `INSERT INTO user_option (name, desc, options, SortOrder) VALUES (...)`
- [ ] Dùng raw `db.Table("user_option")` — KHÔNG `tenant.ScopedDBNullBranch`

### FE define
- [ ] Thêm constant vào `CONFIG_OPTION_CRM_SELECT` trong `config-general.define.ts`

### FE option-crm component
- [ ] Thêm `selectedXxx: number` field trong `option-crm.ts`
- [ ] Thêm `categoryXxx` array với `{ name, key }` items
- [ ] Thêm `case CONFIG_OPTION_CRM_SELECT.OPTION_XXX:` trong `switch` của `ngOnInit`
- [ ] Thêm card HTML trong `option-crm.html` (copy pattern card hiện có)
- [ ] `@for` dùng `track category.key` (không phải `category.value` — items có field `key`, không có `value`)

### FE consumer (nơi cần check option)
- [ ] Inject `UserOptionHelperService`
- [ ] Gọi `await this.userOptionHelper.ensureSettingLoaded()` trước khi đọc
- [ ] `const info = this.userOptionHelper.getUserOptionInfo(CONFIG_OPTION_CRM_SELECT.OPTION_XXX)`
- [ ] `this.optionsXxx = !!info && info.options === 1`

---


## Option 14 — OPTION_HIDE_NUMBER_PHONE — Ẩn số điện thoại

### Logic (ngược với các option khác)

- `options=0` (OFF) = **hiện** toàn bộ số điện thoại → `CanSeePhone = true`
- `options=1` (ON) = **ẩn**, chỉ hiển thị 4 số cuối → `CanSeePhone = false` (trừ BOD)

**Ngoại lệ BOD**: `HrDepartmentBoardOfDirectors = 9` luôn thấy số thật dù option ON.

**⚠️ Đây là option DUY NHẤT có logic ngược** — `options=1` = restrict (ẩn), không phải enable.

### Implement

**Package**: `internal/phoneutil/phone.go`

```go
// Query order: option trước → BOD check chỉ khi option ON
func CanSeePhone(c *gin.Context, db *gorm.DB) bool {
    var optionVal int
    db.Table("user_option").Where("id = 14").Limit(1).Pluck("options", &optionVal)
    if optionVal == 0 { return true } // OFF → mọi người thấy số thật

    // option ON: BOD vẫn thấy số thật
    userID := middleware.GetUserID(c)
    var count int64
    db.Table("user_profile").
        Where("user_id = ? AND JSON_CONTAINS(departmentList, ?)", userID, "9").Count(&count)
    return count > 0
}
```

`MaskPhone(phone)` — `'x'` lặp + 4 số cuối: `"0901234567"` → `"xxxxxx4567"`.  
`MaskPhoneInItems(show, items)` — mask `phone` + `other_phone` trong `[]map[string]interface{}`.

### Coverage — list API callsites dùng `CanSeePhone`

**Export files dùng `CanSeePhoneExport` (option 20), không phải `CanSeePhone`.**

**⚠️ Bảng dưới KHÔNG đầy đủ, đừng dùng để xác nhận "đã cover" — audit 2026-07-15 tìm thêm ~20 handler thiếu mask (treatment_course, note, labo_order, customer_warranty_card, order_treatment_plan, check_up, check_up_general, psych_quiz, workspace/technical, config/call_center_log, sale/opportunity, nhiều report accountant, EMR medical_record/reception, med config/customer_prescription, loyalty/broker) ngoài danh sách đã biết bên dưới — tất cả đã fix. Muốn biết coverage thật → grep `AttachRelationCompanyOnly` / `Preload.*Customer\|Patient` trong `internal/modules/` rồi đối chiếu từng file có gọi `phoneutil` hay chưa, đừng tin bảng này.**

| Module | Files |
|---|---|
| CRM | `customer/handler/customer.go` (6 nơi list + GetCustomer detail + GetCustomerByCode), `appointment/handler/appointment.go` (list + detail + attachCustomer), `reminder/handler/find.go` |
| Sale | `lead/handler/handler.go` (2 nơi) |
| Workspace | `appointment`, `customer`, `reminder`, `revenue`, `treatment_service` |
| Accountant | `customer_debts`, `commission_direct_sale`, `treatment_debts`, `treatment_course_assistant`, `payment` |
| Zalo | `zalo_user/handler/handler.go` (3 nơi) |

### ⚠️ CRITICAL — Mask phone ở MỌI endpoint trả customer.phone, không chỉ list

**Bug đã fix (2026-06-30):** `appointment/handler/appointment.go` — `GetAppointment` (GET /:id) và `attachCustomerToAppointment` (dùng bởi Update/Checkin/Cancel/Confirm) bị thiếu mask. Phone lộ nguyên qua GET detail và tất cả PUT endpoints dù option ON.

**Rule bắt buộc:** Handler nào fetch customer và trả về `phone`/`other_phone` — dù là list, detail, hay response sau CUD — **đều phải** gọi mask.

```go
// List API (items là []map[string]interface{}):
phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), items)

// Detail / single customer (wrap slice để dùng MaskPhoneInItems):
phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), []map[string]interface{}{cust})

// Export Excel — dùng CanSeePhoneExport, KHÔNG dùng CanSeePhone:
showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
```

**Pattern `attachCustomer` helper** — nếu handler có helper riêng để attach customer vào response (như `attachCustomerToAppointment`), mask phải nằm **bên trong helper**, trước khi assign vào item:

```go
func (h *Handler) attachCustomerToXxx(c *gin.Context, item map[string]interface{}) {
    // ... fetch cust ...
    if cust != nil {
        phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), []map[string]interface{}{cust})
        item["customer"] = cust
    }
}
```

**Checklist khi thêm endpoint mới trả phone:**
- [ ] List endpoint → `MaskPhoneInItems` sau khi fetch
- [ ] Detail endpoint (GET /:id) → `MaskPhoneInItems` sau khi fetch customer
- [ ] CUD endpoint (trả lại data sau save) → mask trong `attachCustomer` helper hoặc inline

### FE

FE không mask — hoàn toàn rely vào BE. `option-crm` chỉ là config UI.


## Option 21–23 — ĐÃ GỠ (2026-11-01)

3 công tắc này đã xoá khỏi BE + FE + DB (migration `*_drop_user_option_21_22_23.up.sql`, `DELETE … WHERE id IN (21,22,23)`). KHÔNG tái dùng id 21–23 cho option mới.

| id | Tên cũ | Hành vi cố định sau khi gỡ |
|---|---|---|
| 21 | `OPTION_PAYMENT_BY_TOTAL` — tính số lần dùng theo tổng gói | Không còn code đọc (flow treatment_course của ERP cũ không có trong CRM) |
| 22 | `OPTION_SKIP_PAYMENT_CHECK` — bỏ qua kiểm tra thanh toán | Không còn code đọc (như trên) |
| 23 | `OPTION_CROSS_BRANCH_TREATMENT` — dùng dịch vụ đã mua ở chi nhánh khác | **LUÔN cho phép** (user chốt): mọi chỗ từng gọi `tenant.IsCrossBranchServiceEnabled` (đã xoá) nay truyền `skipBranch=true` vào `tenant.ScopedDBNullBranchSkipBranch` — `order_plan`, `order_service_used`, `payment`, `customer_detail`, `print/order_plan`, `print/payment`. Nhánh creator+same-day của `verifyCanEditOrderPlan`/`verifyCanEditPaymentCustomer` KHÔNG còn check chi nhánh; `UpdatePaymentCustomer`/`UpdateDepositCustomer` chỉ `branchAllowed` khi `branch_id` ĐỔI. **NGOẠI LỆ — list tổng `ListOrderPlan` (+ export) GIỮ `ScopedDBNullBranch`**: mở list toàn hệ thống = lộ doanh thu/khách chi nhánh khác (review 2026-11-02). |

Cần bật/tắt lại theo khách ⇒ thêm option MỚI (id mới) theo checklist §5, không khôi phục id cũ.
