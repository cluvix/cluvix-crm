# Customer (CRM)

## Mục lục

- Status flow
- Constants (`pkg/define/customer.go`)
- Thêm customer status mới — checklist điểm chạm
- branch_id — NULL semantics
- List / Get
- Create — Duplicate check (THIẾT KẾ CỐ Ý)
- Update
- Delete
- CLV (đã bỏ 2026-10-17)
- Financial fields (KHÔNG update trực tiếp từ handler)
- Business actions
- Auto-update từ các module khác
- accepted
- customerType và orderPlanType
- Mã khách hàng (code)
- JSON columns (cần ParseJSONFieldForTable)
- Phone visibility — CRITICAL business rule
  - Logic (đồng bộ với Yii2 `Customer::fields()`)
  - Cài đặt
  - Áp dụng
  - Nguyên tắc mở rộng
- Lưu ý
- Field lock theo phòng ban — control_field_in_table

---


## Status flow
```
1(Trash) → 2(Ticket) → 4(Lead) → 6(Hẹn) → 8(Đã đến) → 10(Đồng ý) → 11(Điều trị) → 12(KH)
                                                                                       → 13(Khiếu nại) → 14(Xong KN)
                                                                                       → 15(CTV)
         → 3(OldSystem) → 5(Deposit) → 7(Hủy hẹn) → 9(Lost)
         → 16(Contact)
```

## Constants (`pkg/define/customer.go`)
```
Trash=1, Ticket=2, OldSystem=3, Lead=4, Deposit=5
Appointment=6, AppointmentCancel=7, Visited=8, Lost=9
PreCustomer=10, Treatment=11, Customer=12, Complaint=13, DoneComplaint=14, Broker=15
Contact=16

CustomerStatuses: list dùng cho filter (bỏ Trash, Ticket, Lead)
AllowedAppointmentCustomerStatuses: statuses được phép auto-update từ Appointment (1-9)
CustomerAcceptedOk = 2  // ACCEPTED_DEFAULT=1 (khởi tạo), ACCEPTED_OK=2 (đã đồng ý)
CustomerTypeNew=1, CustomerTypeOld=2
```

`CustomerStatusContact=16` — **(D14, crm-standalone)** khách mới LUÔN status Contact(16), MỌI phòng ban — để hiện ngay trong danh sách khách CRM (`status IN CustomerStatuses`). Trước đây (ERP gốc) 5 dept saleStaff/telesale/convert/customerCare/marketing tạo ra status Lead(4), chỉ hiện ở module Sale — module Sale đã xoá trong CRM standalone nên Lead(4) sẽ mất khỏi mọi danh sách nếu còn dùng; `appointment_date` vẫn override như cũ (Visited/Appointment). Xem `internal/modules/webapp/crm/customer/handler/customer.go` (`CreateCustomer`/tương đương).

**`customer.status` KHÔNG so sánh `>=`/`<=`** (SQL lẫn TS) — giá trị số không đảm bảo giữ thứ tự nghiệp vụ (Contact=16 là giai đoạn sớm nhưng số lớn nhất). Luôn `status IN (...)` tường minh.

**Funnel / metric "đã đồng ý/chốt/thành công"** — dùng const `CustomerFunnel*Up` (`pkg/define/customer.go`), KHÔNG magic number:
- Converted/chốt = `status IN CustomerFunnelPreCustomerUp` (={10,11,12,13,14,15}).
- Bậc funnel lồng nhau: base = `CustomerFunnelLeadUp` — KHÔNG dùng `LeadStatuses` làm base (nó loại 10-15 → khách đã chốt rớt khỏi base, vỡ funnel). Thứ tự bao nhau: `LeadUp ⊃ AppointmentUp ⊃ VisitedUp ⊃ PreCustomerUp`.

## Thêm customer status mới — checklist điểm chạm
- [ ] `CustomerStatuses`/`LeadStatuses`/`AllowedAppointmentCustomerStatuses` (`pkg/define/customer.go`) — list nào cần thấy status mới
- [ ] `CustomerStatusTransitions[N]` — target hợp lệ tiếp theo
- [ ] `orderAllowedCustomerStatuses` (`crm/service/order_treatment_plan.go`) — gate auto-update Visited khi lên đơn
- [ ] `customerStatusExportLabel` — label export
- [ ] Dashboard funnel buckets `CustomerFunnel*Up` (nếu status nằm ngoài chuỗi tuần tự 1-15)
- [ ] `ForAffiliate` WHERE status IN (...) (`crm/customer/handler/customer.go`) — nếu cần xuất hiện autocomplete affiliate
- [ ] FE `CUSTOMER_STATUS` + `CUSTOMER_STATUS_OPTIONS` (`customer.define.ts`)
- [ ] FE `.statusCustomer-N` CSS (`frontend/src/assets/overrides/custom.scss`) — thiếu = badge không màu
- [ ] i18n `define.customer_status.*` — sửa **CẢ 2 nơi × 4 locale**: `frontend/src/assets/i18n/*.json` (nguồn) VÀ `public/assets/i18n/*.json` ở repo GỐC (artifact đang serve, git-tracked). Chỉ sửa `src` → runtime thiếu key tới lần build kế; chỉ sửa `public` → key mất khi `ng build` regenerate. Nguồn sự thật: `frontend/skill/skills/handling-fe-auth-and-i18n/SKILL.md` §i18n key mới (sửa 2026-08-31 — dòng này trước đây ghi "KHÔNG sửa `public/`", mâu thuẫn với rule đó)

## branch_id — NULL semantics

Customer **được phép có `branch_id = NULL`** (chưa gán chi nhánh). Đây là trường hợp hợp lệ.

**List query — MỌI nơi filter customer đều dùng `tenant.ScopedDBNullBranch`** (KHÔNG phải `ScopedDB`):
- `branchActiveId > 0` → `WHERE branch_id = ? OR branch_id IS NULL`
- `branchIds` → `WHERE branch_id IN ? OR branch_id IS NULL`

Áp dụng cho: `BuildCustomerQuery`, `SearchGlobal`, `LoadCustomerByKeyword`, `ForAffiliate`, `BuildLeadQuery` (sale), `ListPatient` (emr), `ListConvert` (marketing), `GetCustomerDebts` (accountant), `ListCustomerBirthday` (customer-care).

**Read-back sau CREATE:** đọc lại bản vừa tạo bằng `ScopedDBNullBranchSkipBranch(…, true)` (`crm/customer/handler/customer.go`, sau `LAST_INSERT_ID()`; `branch_id` đã qua `customerBranchWriteAllowed` trước khi ghi):
```go
tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "customer", true).Table("customer").
    Where("id = ?", customerID).Take(&item)
```
Lý do: scope chi nhánh thường làm khách vừa tạo cho chi nhánh KHÁC chi nhánh đang active trả nil ⇒ 500/`data: null` sau khi đã commit. (Bản cũ lọc `id + company_id` — đã bỏ, D15.)

## List / Get
- List: `ScopedDBNullBranch` + filter `status IN CustomerStatuses`, keyword search (phone/code/fullname/nickname/old_code/other_phone/link_message/loai_message)
- Get/Update theo id: phạm vi `customerByIDScope` — lọc chi nhánh như list (NULL luôn thấy), chỉ bỏ lọc khi user_option cross-branch (id=18) bật, giống SearchGlobal. Trước 2026-10-20 cả hai luôn `SkipBranch(…, true)` ⇒ IDOR xuyên chi nhánh; KHÔNG quay lại. Create/Update/CreateSwitch gửi `branch_id` ⇒ bắt buộc `customerBranchWriteAllowed` (`middleware.IsBranchAllowed`), sai ⇒ 422 `branch_id_not_allowed`.
- Get: trả đầy đủ fields + `helpers.ParseJSONFieldForTable(item, "customer")` cho JSON columns (tags, relatives, medicalHistory...)
- SearchGlobal: phone suffix matching (9 digits) khi keyword 10-11 ký tự số
- LoadByKeyword: limit 10, label = `fullname - nickname`
- ForAffiliate: filter status IN (3,5,6,7,8,9,10), label = `code | fullname | phone | branch`

## Create — Duplicate check (THIẾT KẾ CỐ Ý)

Check trùng `phone + fullname` **trong TOÀN DB, không phân biệt branch** (D15/story-36 — 1 thương
hiệu/DB nên phạm vi đúng là toàn bảng; trước đây kiểm trong phạm vi `company_id` của body — đã bỏ, D15).

```go
phoneCond, phoneArgs := helpers.PhoneMatchCond("phone", phone, define.TelephoneCodeVN)
h.DB.Table("customer").
    Where(phoneCond, phoneArgs...).
    Where("fullname = ?", fullname).Count(&count)
```

**KHÔNG dùng `ScopedDBNullBranch`** — lý do:
- `ScopedDBNullBranch` thêm `branch_id = X OR IS NULL` → bỏ sót duplicate ở branch khác; khách trùng ở bất kỳ branch nào đều phải bị chặn.
- `phoneCond`/`PhoneMatchCond` so khớp mọi định dạng đã lưu (`0906904884`/`84906904884`/`+84906904884`/`906904884`) — ZMA lưu dạng Zalo trả về, web lưu dạng người dùng gõ; `phone = ?` thuần sẽ trượt và cho tạo trùng.

**`CreateCustomerSwitch`:** cùng logic query, không cần validate lại phạm vi (đã scope-checked ở luồng gốc).

## Update
- **Optimistic locking**: check version khớp DB → `version += 1`
- Whitelist fields qua `customerAllowedFields` map (service layer)
- Snapshot old → update → snapshot new → `history.LogChanges(db, "log_customer", "customer_id", ...)`
- Log include fields: `fullname, nickname, phone, category, branch_id, source_id, marketing_id, payment_user_id, type_service_id, created_day`
- Log skip null: `branch_id, fullname, nickname, phone, type_service_id`
- Response: reload từ DB → `ParseJSONFieldForTable` → trả về

## Delete
- KHÔNG `DB.Delete` trực tiếp — FK 1451 (nhiều bảng con tham chiếu `customer_id`)
- Xoá qua `DeleteCustomerWithPayments` (`crm/service/customer_delete.go`); guard đếm bảng con theo FK
  - **KHÔNG dùng `ScopedDBNullBranch` cho guard count** — FK không phân biệt branch; scope branch bỏ sót record cross-branch → guard pass sai (xem patterns-gotchas.md § Delete guard)
  - BAĐT đếm cả soft-deleted vì FK `emr_medical_record.patient_id` vẫn giữ row
- Pass guard → `crmService.DeleteCustomerWithPayments(db, id, userID)` (`crm/service/customer_delete.go`) — 1 transaction:
  1. Xoá MỌI phiếu thu/chi của khách (gắn thẳng `customer_id` — đặt cọc/hoàn cọc; theo `serviceUsed_id`; theo `parent_id`) qua `deletePaymentWith(…, deferred)` từng phiếu, mới → cũ; phần tính lại gom chạy 1 lần cuối (xem `../SKILL.md` §Xoá HÀNG LOẠT phiếu) — phiếu mang `customer_id` của khách KHÁC vẫn xoá kiểu thường ⇒ bút toán đảo sổ kép (`ledger_event` `ledger_reversal`), dọn hoa hồng/doanh thu/hoá đơn nháp. Phiếu thuộc kỳ đã khoá sổ / có hoá đơn đã phát hành ⇒ `LocalizedError` `customer_delete_payment_period_locked` / `…_invoice_issued` kèm MÃ PHIẾU ⇒ handler trả 422 và huỷ cả lần xoá.
  2. `CustomerSvc.DeleteWithCleanupTx(tx, …)`: dọn bảng con + xoá customer; ghi 1 dòng `log_customer` action_type=3 (snapshot `history.CustomerDeleteFields` = `code` + field theo dõi; không còn `company_id`, D15) — xem `../../debugging-backend-queries/reference/log-history.md`.
  - ⛔ **KHÔNG xoá thẳng payment bằng SQL** (`DELETE FROM payment …`) — bỏ qua bút toán đảo ⇒ journal treo cho chứng từ không còn (`ledger_event` đang LIVE). Vì vậy `DeleteWithCleanup*` không đụng payment; FK `payment.customer_id → customer` RESTRICT (migration `20260922145614`) chặn nếu còn sót phiếu ⇒ lỗi 1451, không âm thầm.
  - Vì sao hàm điều phối nằm ở `crm/service`: `crm/service` đã import `customer/service` ⇒ `CustomerService` không gọi được `deletePayment` (vòng import).
  - Handler chỉ đưa `LocalizedError` qua `response.HandleError`; lỗi khác vẫn `response.ServerError` (HandleError với lỗi hạ tầng = 422 lộ nguyên chuỗi SQL).
- Lỗi FK 1451 → 422 `fk_constraint`; infra error → 500 — dùng `response.ServerError(c, err)` (tự phân biệt)
- Dùng chung cho `DELETE /crm/customer/:id` và `DELETE /emr/patient/:id` (BE-037/038, 2026-06-11)

## CLV (đã bỏ 2026-10-17)
Các cột denormalized CLV và hook recompute đã bị xoá (migration `20261017123004`), không áp dụng.

## Financial fields (KHÔNG update trực tiếp từ handler)

Entity-side fields + công thức:

| Field | Compute |
|---|---|
| `totalRevenue` | `SUM(osu.totalEnd + refundServiceUsed)` — trigger từ OrderServiceUsed CUD |
| `totalDebt` | `totalRevenue - totalPayment` |
| `totalPayment` | `SUM(amount WHERE payment_for IN 1,4,5)` |
| `totalDeposit` | `SUM(deposit_after)` (reserve/temp, không KG) |
| `refundDeposit` / `refundService` | `SUM(amount)` theo loại refund |
| `consignment` | `SUM(deposit_after WHERE KG)` |
| `refundConsignment` | `SUM(amount)` |
| `totalBonus` | Không có logic update — chỉ set = 0 khi tạo mới |

**Go implementation:**
- `totalRevenue` + `totalDebt`: `OrderTreatmentPlanService.updateCustomerRevenue()` — 1 query JOIN, gọi sau mỗi Create/Update/Delete order
- Các fields còn lại: `PaymentService.updateCustomer()` — gọi trong `afterSaveAll` + `afterDeleteAll`
- Mapping `payment_for` → fields cập nhật: → Xem `skill/skills/working-on-crm/SKILL.md` § calculatorMoneyCustomer
- Enum `payment_for` + `deposit_type`: → Xem `skill/skills/working-on-crm/SKILL.md`

## Business actions
- `POST /:id/add-tag` — append tag vào JSON array `tags`, skip nếu đã tồn tại
- `POST /:id/remove-tag` — xóa tag khỏi JSON array
- `POST /:id/update-macro?macro=Y` — update `macro_id`
- `POST /:id/update-appointment-not-booked-reasons?appointmentNotBookedReasons=Y`
- `POST /:id/create-avatar` — upload ảnh S3 `customer/{code}/avatar/` dùng Options riêng (Mode Fill 500×500, TargetKB 100, SkipBelowKB -1) — KHÔNG dùng `OptFill500()` (preset đó dùng chung coupon, SkipBelowKB mặc định 200 không đảm bảo crop đồng nhất)

## Auto-update từ các module khác

**Quy tắc tổng:**
- `last_contact_date` = mọi tương tác giữa nhân sự ↔ khách (Note, Reminder, CustomerCare, Appointment Create/Update/Cancel/Checkin/Checkout, Customer Update, Payment, Order, Treatment). Mặc định = now hoặc timestamp của event tương ứng.
- `last_visit_date` = chỉ khi khách thực sự ghé (checkin/checkout, payment FOR_ORDER, order create, treatment). KHÔNG update khi reschedule/cancel/note.
- `status` transition trên Customer **chỉ áp dụng** khi `customer.status ∈ AllowedAppointmentCustomerStatuses` (Trash/Ticket/OldSystem/Lead/Deposit/Appointment/AppointmentCancel/Visited/Lost/Contact/Zalo). Khách đã thành Customer/Treatment/PreCustomer/Complaint/Broker → status giữ nguyên (chỉ tracking date update). Zalo(17) = khách tự đăng ký qua Zalo Mini App (`findOrCreateCustomer`).

| Module | Trigger | Field update |
|--------|---------|-------------|
| Appointment Create | afterSave | status → Appointment (gated), last_contact_date = appt.created_at, branch_id (nếu null) |
| Appointment Update/NotCheckin | afterSave | last_contact_date = now |
| Appointment Checkin | afterSave | status → Visited (gated), last_visit_date + last_contact_date = checkin_date |
| Appointment Checkout | afterSave | status → Visited (gated), last_visit_date + last_contact_date = checkout_date |
| Appointment Cancel | afterSave | status → AppointmentCancel (gated), last_contact_date = now |
| Customer Update endpoint | handler | last_contact_date = now (FE có thể override bằng cách gửi explicit) |
| Payment FOR_ORDER insert | afterSave | last_visit_date, last_contact_date = payment_time |
| OrderTreatmentPlan insert | afterSave | last_visit_date, last_contact_date = created_time |
| TreatmentCourse insert | afterSave | status → Treatment/Customer, last_visit_date, last_contact_date |
| OrderServiceUsed CUD | handleServices | totalRevenue, totalDebt (updateCustomerRevenue) |
| Payment/Order | afterSaveAll/afterDeleteAll | totalPayment, totalDebt, totalDeposit, refundDeposit, refundService, consignment, refundConsignment |
| Broker | afterSave/afterDelete | status → Broker/Customer |
| CustomerCare | afterSave | last_contact_date = time_care |
| Note | afterCreate | last_contact_date = note.created_at |
| Reminder | afterCreate | last_contact_date = now |
| CustomerCareComplaints | afterSave | status → Complaint |
| CustomerReview | afterSave | customer_review (rating) |

## accepted
- `1 = ACCEPTED_DEFAULT` — khởi tạo, chưa có quyết định
- `2 = ACCEPTED_OK` — khách đã đồng ý làm dịch vụ sau tư vấn
- **Trigger duy nhất**: `Appointment.afterSave` khi `is_accepted = IS_ACCEPTED (1)` → set `customer.accepted = 2`
- **Tác dụng**: điều kiện trong `Payment.updateCustomer` + `TreatmentCourse.updateCustomer` để quyết định nâng status lên `STATUS_CUSTOMER (12)` — chỉ upgrade khi `accepted = 2` VÀ đã thanh toán đủ/hoàn thành điều trị

## customerType và orderPlanType
- `customerType`: `1=TYPE_NEW` (mặc định), `2=TYPE_OLD`
- **Đổi sang TYPE_OLD**: `Payment.updateCustomer` khi `payment_for IN (FOR_ORDER=4, FOR_MOVE_DEPOSIT=5)` — tức là khi khách **phát sinh thanh toán đơn hàng thực sự**
- **NGOẠI LỆ Phí khám**: nếu `order_treatment_plan` chỉ chứa `serviceCategory_id=4` (phí khám), KHÔNG nâng `customerType` lên TYPE_OLD — kiểm tra `COUNT(*) FROM order_service_used LEFT JOIN service WHERE COALESCE(serviceCategory_id,1) != 4 > 0` trước khi set
- **Tác dụng**: dùng trong TreatmentCourse để tính hoa hồng — `TYPE_OLD` dùng rate `commission_directSale`, `TYPE_NEW` dùng rate `commission_directSale_new`
- `orderPlanType`: **snapshot `customerType` tại thời điểm tạo đơn** (`beforeSave insert` → `order.orderPlanType = customer.customerType`)
  - `1` = đơn hàng mới (khách chưa từng thanh toán)
  - `2` = đơn hàng cũ (khách đã từng thanh toán trước đó)
  - Dùng để tính hoa hồng commission cho direct sale trong Payment

## Mã khách hàng (code)
- Khách **chưa có chi nhánh ⇒ chưa có mã** (`code` NULL — khách lưu từ lúc còn Lead); có chi nhánh ⇒ sinh ngay. **`branch_id` là FK nullable — giá trị đúng là `NULL`, không phải `0`**
- Mọi đường tạo khách / gán chi nhánh cho khách gọi `codegen.NewCustomerCode` (khách sắp tạo) hoặc `codegen.EnsureCustomerCode` (khách đã có, tự bỏ qua nếu đã có mã) — `pkg/codegen/customer_code.go`. **CẤM** `codegen.Generate(tx, "customer", …)`: nó không biết quy tắc chi nhánh lẫn kiểu mã đang chọn.
- Mã theo chi nhánh **CỦA KHÁCH** — chi nhánh của lịch hẹn/đơn hàng chỉ dùng khi khách chưa có chi nhánh (tức vừa được gán theo lịch hẹn/đơn).
- Kiểu mã chọn bằng env `CUSTOMER_CODE_OPTION` 1–6 (`.env` riêng từng khách SaaS, bảng ví dụ ở `pkg/define/customer.go`; bỏ trống/giá trị lạ = 1 = theo `config_code`). Bộ đếm dùng lại `config_code_monthly`: option đếm theo chi nhánh dùng row của chính chi nhánh; option 5/6 đếm chung toàn DB (1 thương hiệu/DB) dùng row chi nhánh id nhỏ nhất. Đổi option giữa chừng ⇒ số thứ tự đếm TIẾP, không về 1.
- `UNIQUE (code)` toàn DB (D23 — schema mới `migrations/20260928093229_create_customer.up.sql`, tên index giữ lịch sử `uniq-customer-code-company`; bản cũ `(code, company_id)` đã bỏ, D15); mã sinh ra đã có ⇒ helper tự nhảy số (tối đa 50 lần, `customerCodeMaxAttempts`).
- Sinh nhiều mã trong 1 request (import) ⇒ `codegen.NewCustomerCoder(tx)` MỘT lần rồi `.New`/`.Ensure` từng dòng (cache config_code + mã chi nhánh). Bộ đếm mã khách tăng NGOÀI tx nghiệp vụ (rollback vẫn tiêu số) ⇒ row `config_code` "customer" phải ĐÃ COMMIT trước khi sinh mã.
- id khách lấy từ payload/file ⇒ kiểm scope TRƯỚC khi gọi `EnsureCustomerCode` — nó đọc khách không qua tenant scope và mỗi lần gọi đốt 1 số thứ tự (khuôn: `pkg/customerimport` nhánh UPDATE theo id).

## JSON columns (cần ParseJSONFieldForTable)
`tags, relatives, medicalHistory, emr_allergy, emr_family_history`

## Phone visibility — CRITICAL business rule

Số điện thoại (`phone`, `other_phone`) là dữ liệu nhạy cảm. **Phải mask ở BE trước khi trả response** (không để FE tự quyết).

### Logic (đồng bộ với Yii2 `Customer::fields()`)

```
canSeePhone = isBOD(user) || !hidePhoneOption
```

- **isBOD**: `user_profile.departmentList` chứa `HrDepartment.BOARD_OF_DIRECTORS (id=9)` — một user có thể nhiều phòng ban
- **hidePhoneOption**: `user_option` id=14, `options=1` = ẩn, `options=0` = hiển thị toàn bộ
- BOD luôn thấy số thật dù `user_option` được bật

### Cài đặt

```go
// Shared helper: internal/phoneutil/phone.go
phoneutil.CanSeePhone(c, db)                   // → bool
phoneutil.MaskPhone(phone)                      // "0901234567" → "xxxxxx4567"
phoneutil.MaskPhoneInItems(show, items)         // mask phone + other_phone trong []map
```

### Áp dụng

- **List handler** (leads, customers, zalo-lead, ...): gọi `MaskPhoneInItems` sau `pagination.Paginate`
- **Export** (`ExportCustomer`, `ExportCustomerBirthday`, `ExportCustomerConvert`, `ExportCustomerDebts` — `ExportCustomerLead` đã bỏ cùng Lead/Sale module, D14): check `CanSeePhone` ở đầu hàm, dùng trong row mapper
- **FE KHÔNG tự mask** — BE đã lo, FE render thẳng giá trị trả về
- Format mask: `'x'` lặp lại cho toàn bộ phần đầu, giữ 4 số cuối (vd `"0901234567"` → `"xxxxxx4567"`)

### Nguyên tắc mở rộng

Bất kỳ endpoint mới nào trả `phone`/`other_phone` của customer → **bắt buộc** gọi `phoneutil.CanSeePhone` + `MaskPhoneInItems` (hoặc `MaskPhone` per-field). Không exception.

**Struct + `Preload` (EMR/Med, khác `[]map`)**: `MaskPhoneInItems` chỉ nhận `[]map[string]interface{}`, không áp được cho struct (`*PatientRef`/`*CustomerRef` field `Phone string`). Mask trực tiếp field sau khi query:
```go
if !phoneutil.CanSeePhone(c, h.DB) {
    for i := range items {
        if items[i].Customer != nil {
            items[i].Customer.Phone = phoneutil.MaskPhone(items[i].Customer.Phone)
        }
    }
}
```

**2026-07-15 audit:** phát hiện ~20 handler CRM/workspace/sale/accountant/EMR/med/loyalty thiếu mask dù rule này đã có từ trước — do dev thêm handler ở module khác không đọc `customer.md`. Đã fix hết + thêm pointer vào `skills/backend-hard-rules/SKILL.md` §Nested data (đọc chung mọi module) để không lọt lại. Audit KHÔNG exhaustive — 2026-07-27 phát hiện thêm `customer-care/{care,complaints,review}` cũng thiếu (3 module cùng share 1 helper `attachCustomer` y hệt), đã fix. Bất kỳ module mới `attachRelation` customer kèm field `phone` → tự grep `phoneutil` ngay, đừng tin audit cũ là đã quét hết.

## Lưu ý
- `birthday`: `*time.Time` (DB date), KHÔNG `*int64`
- `created_day`: int64 (unix timestamp ngày tạo — khác `created_at`)
- Phone search: suffix 9 digits để match cả `0xxx` và `84xxx`
- `CustomerStatuses` filter bỏ Trash(1), Ticket(2), Lead(4) — FE dùng `_status` để filter thêm

## Field lock theo phòng ban — control_field_in_table

Form nào áp dụng chọn theo `user_profile.departmentList` (JSON_CONTAINS), KHÔNG theo role:
- Thuộc 1 trong {saleStaff=12, convert=11, customerCare=6, marketing=4, telesale=3} → form `id=2` (lead)
- Khác → form `id=1` (contact/customer)
- BOD (`id=9`) → bypass toàn bộ field lock

BE enforce lại field `edit=true` ("Không được cập nhật") bằng cách `delete()` field đó khỏi payload trước update — KHÔNG chỉ dựa FE `.disable()` (client gọi thẳng API sẽ bypass được FE). Xem `CustomerService.applyFieldLocks()` (`internal/modules/webapp/crm/customer/service/service.go`), mirror FE `customer-form.ts:handleEditMode()`.

Xem thêm: `skills/working-on-config/SKILL.md` (schema, JSON format gotcha), `skill/skills/debugging-backend-queries/reference/patterns-gotchas-hr-dept.md` (department constant nào đúng/sai khi viết dept check mới).
