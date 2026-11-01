---
name: working-on-crm
description: >-
  Quy tắc và pattern nghiệp vụ CRM Cluvix: khách hàng, lịch hẹn, liệu trình, đơn hàng,
  thanh toán. Dùng skill này KHI làm feature/sửa bug đụng module crm (customer,
  appointment, order, treatment, payment) — trước khi đọc code, để tránh phá vỡ business rule đã có (tái khai thác,
  customer status IN(...)).
---

# Working on CRM

> reference/customer.md: khách hàng · reference/order.md: đơn hàng · reference/treatment.md:
> liệu trình ·
> reference/check-up-psychiatry.md: khám sàng lọc tâm thần ·
> reference/appointment-list-performance.md: hợp đồng màn danh sách lịch hẹn (join + tìm kiếm).



Điều hướng file chuyên sâu: `INDEX.md` cùng folder.

## Note (Ghi chú)
- `togglePin`: đổi `is_pin` 0↔1
- Gắn với `customer_id`

## Reminder (Nhắc nhở)
- Auto code generation (RE prefix)
- `STATUS_DONE = 2`

## CustomerLead (Khách tiềm năng)
- Tạo từ website hoặc Zalo
- Rate limit, IP duplicate check 30s (từ website)

## Check-Up / Thăm khám ban đầu
> ⚠ Lịch sử: module check-up KHÔNG còn trong `crm/` của CRM standalone (đã xoá, D1/D17) — mục này chỉ để tra bản ERP cũ.
- Mỗi chuyên khoa có bảng riêng, KHÔNG trộn chung
- Tenant scope qua `customer.id` (bảng không có cột scope riêng)
- Pattern đầy đủ: `skills/working-on-crm/SKILL.md`
  - Tâm thần (`psych_assessment`): `skills/working-on-crm/reference/check-up-psychiatry.md`

## WarrantyCard (Thẻ bảo hành)
- Gắn với customer + service used
- `confirmCard`: xác nhận bảo hành

---

## Gotchas CRM-wide

- **Response write**: `DataWithMessage(c, result, "msg")` — reload từ DB vào `map[string]interface{}`, parse JSON fields, attach customer nếu FE cần
- **Preload**: chỉ customer (không có IndexedDB). Supplier/Unit/Medicine/Branch/User → FE resolve từ Dexie cache
- **isLock**: KHÔNG phải 0/1 — giá trị → Xem `skill/skills/working-on-crm/reference/order.md` (§ OrderServiceUsed Constants)
- **ShouldBind** thay `ShouldBindJSON` khi FE có thể gửi FormData (file upload)
- **JSON columns**: GORM scan thành string → gọi `ParseJSONFieldForTable()` trước khi trả response
- **Birthday**: `*time.Time` (DB date), KHÔNG `*int64`. Timestamp fields dùng `int64` (unix)
- **Codegen serial**: width tự nở `max(serial_length, len(serial))`, INSERT mới check `affected=1 → serial=1`

## Báo cáo CRM — quyền (epic manage-crm-report 2026-09-14)
- Toàn bộ 21 trang `/crm/report/*` (xem + export) gate bằng **1 quyền duy nhất `manageCrmReport`** — `viewCrmReport`/`viewReportAppointment` đã xoá khỏi catalog.
- Dashboard CRM (web `/crm` + mobile) **KHÔNG cần `manageCrmReport`** — gate bằng **quyền module CRM** (OR 7 quyền; cách lấy đúng chuỗi quyền: `frontend/skill/skills/handling-fe-auth-and-i18n/SKILL.md` §4):
  `viewAppointment, viewCustomer, viewCheckUp, viewPlanOrder, viewTreatmentCourse, viewLaboOrder, viewWarrantyCard`.
- Nguồn OR 7 quyền — **3 nơi, sửa 1 nơi phải sửa cả 3**: FE `frontend/src/app/define/permission.define.ts` hằng `CRM_MODULE_PERMISSIONS` (dùng bởi `system-module-menu.ts` ô `cxCrm` + `utilities/home/home.ts`); BE `route_perm_map.json` (21 entry `/api/crm/crm-dashboard/*` + 20 `/api/mobile/v1/crm/*`), giữ đồng bộ bởi test `backend/internal/modules/webapp/auth/rbac/seed/crm_module_perm_set_test.go`; mobile `packages/core_domain/lib/src/permissions.dart` hằng `kPermsCrmModule`.


## Tổng quan
Lịch hẹn — model trung tâm CRM, kết nối Customer, OrderPlan, Payment, TreatmentCourse.

### Constants
```
Status: NOT_CHECKIN=1, CHECKED_IN=2, CHECKED_OUT=3, CANCEL=4, ADVISE=5, TREATMENT=6
BookType: DAT_TRUOC=1, PHAT_SINH=2, DU_KIEN=3
IsAccepted: ACCEPTED=1, NOT_ACCEPTED=2
ConfirmAppointment: 0=Chưa xác nhận, 1=Xác nhận khách đến, 2=Xác nhận khách không đến
```

### Phân loại loại hẹn (appointment_type)
- Bảng `appointment_type` quản lý loại lịch hẹn (Tư vấn, Điều trị, Tái khám, Bảo hành…)
- **KHÔNG hardcode `appointment_type_id`** khi filter loại tư vấn — dùng cột `isTypeRevenue`:
  - `isTypeRevenue = 1` → loại hẹn tư vấn (revenue-generating consultation)
  - Pre-fetch IDs: `SELECT id FROM appointment_type WHERE isTypeRevenue = 1 AND status = 1`
  - Sau đó dùng `appointment_type_id IN (ids)` trong query chính
- Áp dụng ở mọi dashboard/report cần lọc riêng lịch hẹn tư vấn (ví dụ: `AppointmentAcceptedStats` trong marketing dashboard)

## Tạo mới
- `appointment_number` = `SELECT COALESCE(MAX(appointment_number),0) ... FOR UPDATE` + 1 cùng customer_id + appointment_type_id — KHÔNG dùng COUNT+1 (race 2 request đồng thời)
- `appointment_date_first = appointment_date`
- (đã bỏ: điền `company_id` từ khách — D15; auto BMI + `checkDoctorConflict` chặn trùng giờ bác sĩ — D17 bỏ vai trò y tế, entity `appointment` không còn `doctor_id`/`emr_*`)
- `addTimelineEvent('Tạo mới lịch hẹn', content)`
- afterSave: update `customer.last_contact_date`, customer status → STATUS_APPOINTMENT, gán `customer.branch_id` nếu null
- afterSave: khách chưa có mã mà (khách hoặc lịch hẹn) có chi nhánh → sinh mã khách (tác dụng phụ ngoài phạm vi lịch hẹn — Update cũng làm), quy tắc: `reference/customer.md` §Mã khách hàng
- `notifyAppointmentPush(..., "push_appointment_new")` — bắn cho `revenue_id`, trừ actor; fire-and-forget

## Cập nhật
- **Optimistic locking** theo `updated_at` (D6): FE gửi `updated_at` khác DB → `errs.ErrVersionConflict` (bảng không còn cột `version`)
- Track `appointmentListDate`: nếu appointment_date thay đổi → append `{"old": <unix>, "new": <unix>}` vào JSON array (object, KHÔNG phải chuỗi "d-m-Y -> d-m-Y" như comment entity ghi)
- Sync `customer.branch_id` + `customer.code` khi lịch hẹn có branch_id (`reference/customer.md` §Mã khách hàng)
- Đổi `appointment_date` → `notifyAppointmentPush(..., "push_appointment_time_changed")`
- afterSave:
  - **Log changes** → insert log_appointment
  - **Customer status mapping** — CANCEL → STATUS_APPOINTMENT_CANCEL · NOT_CHECKIN → STATUS_APPOINTMENT · CHECKED_IN/OUT → STATUS_VISITED (chi tiết + `last_visit_date`/`last_contact_date`: § updateCustomerStatus bên dưới)
  - sortAppointment nếu appointment_type_id thay đổi

## Check-in / Check-out (cùng endpoint `PUT /:id/customer-checkin`)
- Alias cho client cũ: `POST /crm/appointment/customer-checkin?id=` — route inject `id` từ query
- FE form `checkin.ts` chỉ gửi `checkin_date / checkout_date / doctor_id / is_accepted / noteAppointment / version` — **KHÔNG gửi `status`**
- BE auto-derive status nếu `appt.Status == nil`:
  - `merged.CheckoutDate != nil` → `STATUS_CHECKED_OUT` (3)
  - `merged.CheckinDate != nil`  → `STATUS_CHECKED_IN` (2)
  - cả 2 nil → `STATUS_NOT_CHECKIN` (1)
- **`explicitClears`**: handler đọc raw body, key CÓ MẶT với `null`/`""` mới được xoá (mergeAppointment bỏ qua nil/rỗng). Client gửi payload partial (mobile) không bị xoá oan. Tập cho phép xoá = `checkinClearableFields` (sinh hiệu + mô tả lâm sàng + ghi chú).
- **Cascade rollback khi xoá date** — xoá `checkin_date` → NULL luôn `doctor_id`, `direct_sales_id`, `checkout_date`, `is_accepted`, `type_service_accepted_id`; xoá `checkout_date` → NULL `is_accepted`, `type_service_accepted_id`
- Dual-write sinh hiệu sang `emr_vital_measurement` trong CÙNG tx (`prepareVitalDualWrite` TRƯỚC merge, `writeAppointmentVitals` sau Save) (module emr đã bỏ khỏi CRM standalone)
- Timeline label theo trạng thái cuối: "Khách check-in" hoặc "Khách check-out"
- Auto BMI + auto `emr_queue_number` khi chuyển sang CHECKED_IN
- Triggers beforeSave/afterSave giống Update

## Cancel
- Force `status = STATUS_CANCEL`
- `addTimelineEvent('Huỷ lịch hẹn', noteAppointment)`
- afterSave → customer status → STATUS_APPOINTMENT_CANCEL

## Confirm
- Bind confirmAppointment = 0/1/2
- `addTimelineEvent(mess, noteAppointment)` — mess theo giá trị confirm

## Xóa
- beforeDelete: `DELETE FROM log_appointment` + `DELETE FROM zalo_zns_log` theo appointment_id (xoá sạch lịch sử, KHÔNG insert bản ghi log action=DELETE)
- beforeDelete: unlink `order_service_used`, `order_treatment_plan`, `treatment_course` (SET appointment_id = NULL)
- afterDelete: `sortAppointment` cùng customer + type

## Ràng buộc lịch — trạng thái hiện tại

**`checkSlotCapacity` ĐANG TẮT** (`crm/appointment/handler/appointment.go`): thân hàm `return ""` ngay, toàn bộ logic `appointment_time_frame` bị comment (`// TODO: ... tạm thời tắt`). KHÔNG có chống overbooking theo công suất slot; đừng viết feature/report dựa trên giả định nó chạy. Endpoint Create/Update vẫn gọi hàm và vẫn trả 422 `slot_capacity_full` khi bật lại.

Hiện KHÔNG có ràng buộc trùng lịch nào chạy: `checkDoctorConflict` (chặn bác sĩ trùng khung giờ) đã bỏ cùng vai trò bác sĩ (D17) — lịch sử, đừng tìm hàm này trong code.

## AddNote (`POST /:id/add-note`)

Append vào `noteTimeline` atomic bằng `JSON_ARRAY_APPEND` trong 1 UPDATE — KHÔNG đọc-sửa-ghi (`First` → append → `UpdateColumn`) vì không có version guard: 2 request đồng thời (AddNote//AddNote hoặc AddNote//Update) làm mất event của nhau.

## updateCustomerStatus — 2 nhóm field độc lập

- `last_contact_date` / `last_visit_date`: cập nhật **không phụ thuộc** allow-list. CHECKED_IN → cả 2 = `checkin_date`; CHECKED_OUT → `last_visit_date` = `checkout_date`; NOT_CHECKIN/CANCEL → `last_contact_date` = now.
- `status` / `accepted`: chỉ đổi khi `define.AllowedAppointmentCustomerStatuses[customer.status]`.
- `is_accepted` override status: `IsNotAccepted` → LOST; id nằm trong `SELECT id FROM appointment_is_accepted WHERE isAccepted = 1 AND status = 1` → PRE_CUSTOMER + `accepted = ACCEPTED_OK`. KHÔNG hardcode id (cùng lớp với `isTypeRevenue`).

## Màn danh sách + tìm kiếm

Join customer có điều kiện (`customerFilterColumns`), hợp đồng `applyKeywordFast`, cột list vs detail → `skill/skills/working-on-crm/reference/appointment-list-performance.md`. Số đo + 4 biến thể tìm kiếm đã loại + quy trình bench → `skill/skills/debugging-backend-queries/reference/perf-list-endpoints.md`.

## Helpers
- **addTimelineEvent**: parse `noteTimeline` JSON → append `{action, note, user_id, timestamp}` → marshal back
- **sortAppointment**: reorder `appointment_number` cùng customer_id + appointment_type_id theo appointment_date ASC (ROW_NUMBER)

## Dependencies
Customer, OrderPlan, Payment, TreatmentCourse, LogAppointment, AppointmentReason


## Tổng quan
Xử lý tất cả loại thanh toán: đặt cọc, thanh toán đơn hàng, hoàn cọc, hoàn dịch vụ, thu/chi phòng khám.

## Loại thanh toán (payment_for)
```
1 = Hoàn dịch vụ (FOR_REFUND_SERVICE)
2 = Hoàn cọc (FOR_REFUND_DEPOSIT)
3 = Đặt cọc (FOR_DEPOSIT)
4 = Thanh toán đơn hàng (FOR_ORDER)
5 = Chuyển cọc (FOR_MOVE_DEPOSIT)
6 = Hoàn ký gửi (FOR_REFUND_DEPOSIT_KG)
7 = Thu tiền phòng khám (FOR_CLINIC_INCOME)
8 = Chi tiền phòng khám (FOR_CLINIC_EXPENSE)
```

## payment_method (Hình thức) — bảng global cố định, id có ý nghĩa

`payment_method` là bảng global (không scope chi nhánh), UNIQUE theo `name`, seed 1 lần (`migrations/00000000000001_seed_data.up.sql`), luôn đúng 4 dòng: `1=Chuyển khoản, 2=Máy POS, 3=Chuyển cọc, 4=Tiền mặt`. Vì bảng cố định → được phép hardcode id như enum (khác các bảng config khác thường phải lookup theo tên).

- **FOR_MOVE_DEPOSIT (chuyển cọc → thanh toán)**: `method` LUÔN set cứng server-side = `define.PaymentMethodTransferDeposit` (=3, "Chuyển cọc") trong `MoveDepositToPaymentFromPayment`/`MoveDepositToPayment` (`payment_crud.go`) — **FE không có field chọn method trong `FormMoveDeposit`** (cố ý, không phải thiếu sót), vì bản chất đây là bút toán nội bộ (không có dòng tiền mới phát sinh), không phải user tự chọn hình thức thanh toán.
- **Bug đã xảy ra 2026-07-27**: hằng số từng đặt tên `PaymentMethodTransfer = 1` (nhầm sang "Chuyển khoản" id=1) — trông giống đúng tên nhưng sai giá trị, khiến payment chuyển cọc hiện sai "Hình thức". Đã đổi tên `PaymentMethodTransferDeposit = 3` cho khớp nghĩa. Đừng đổi lại giá trị này nếu không verify lại `payment_method.name` trong seed.

## Phiếu DÙNG cọc — cọc phải của chính khách đó (2026-09-22)
- `validateDepositUse` (`crm/service/payment_crud.go`) chạy ở 4 luồng tạo: hoàn cọc, hoàn ký gửi, `MoveDepositToPayment*` (cả bản deprecated). Phiếu cọc phải `payment_for = FOR_DEPOSIT`, **cùng `customer_id`** (vế "cùng `company_id`" đã bỏ ở story-37, D15); chuyển cọc thì `serviceUsed_id` / `parent_id` từ body cũng phải của khách chủ cọc. Sai ⇒ `errs.ErrDepositCustomerMismatch` (422 `deposit_customer_mismatch`); không phải phiếu cọc ⇒ `ErrDepositNotFound`. Trước đây nhận `deposit_id` từ body không đối chiếu gì (IDOR body-id).
- Đường SỬA phiếu không đổi được `deposit_id` / `serviceUsed_id` / `customer_id` (`mergePaymentParams`) nên không cần guard ở đó — thêm field vào merge thì PHẢI gọi lại guard.
- Test: `crm/service/deposit_use_integration_test.go` (có ca cọc của khách khác — xem `../testing-backend/SKILL.md` §FreshTenant).
- `payment.deposit_id` có index `idx_payment_deposit_id` (migration `20260922154654`) — trước đó mọi `deletePayment` + tính số dư cọc quét toàn bảng payment.

## Xoá HÀNG LOẠT phiếu — gom phần tính lại (`deferredPaymentRecalc`)
`deletePaymentWith(p, userID, deferred)`: deferred != nil ⇒ phần lõi (guard kỳ khoá/hoá đơn, dọn hoa hồng/doanh thu/hoá đơn nháp, log, bút toán đảo, **hoa hồng CTV**) chạy ngay; phần tính lại gom và chạy 1 lần cuối (hàm tính lại đều state-based). Bảng giữ/gom/bỏ từng tác dụng: comment `deferredPaymentRecalc` (`crm/service/payment.go`). Chỉ dùng khi khách/đơn/dòng dịch vụ CHẮC CHẮN bị xoá cùng transaction — hiện chỉ `DeleteCustomerWithPayments`. Đo trên clone dev: 5 khách nhiều phiếu nhất 19.2s → 3.9s; so 377 bảng giữa cách cũ và cách gom: 0 khác biệt (bộ so đã đối chứng bằng 1 dòng sửa cố ý). Dev không có CTV nên nhánh hoa hồng CTV không được so — nhánh đó không đổi (vẫn chạy từng phiếu).

## Loại đặt cọc (deposit_type)
```
1 = Đặt cọc giữ chỗ (RESERVE)
2 = Đặt cọc tạm (TEMP)
3 = Ký gửi (KG) — loại trừ khỏi thực thu
```

## Routes (19 endpoints) — `internal/modules/webapp/crm/payment/route.go`

### Đặt cọc
- `POST create-deposit-customer` — Tạo đặt cọc → update customer.totalDeposit
- `PUT :id/update-deposit-customer` — Cập nhật
- `DELETE :id/delete-deposit-customer` — Xóa

### Thanh toán
- `POST create-payment-order` — Thanh toán đơn hàng
- `POST create-payment-customer` — Thanh toán khách hàng
- `POST ""` / `PUT :id` — Thu/chi phòng khám (CreatePaymentClinic / UpdatePaymentClinic)
- `PUT :id/update-payment-customer` — Cập nhật
- `DELETE :id/delete-payment-customer` — Xóa
- `GET :id` — Chi tiết payment
- `POST :id/move-deposit-to-payment` / `PUT :id/move-deposit-to-payment` — Chuyển cọc thành thanh toán

### Hoàn trả (reverse logic)
- `POST create-refund-deposit-customer` — Hoàn cọc
- `POST create-refund-kg-customer` — Hoàn ký gửi
- `POST create-refund-order-plan` — Hoàn dịch vụ
- `DELETE :id/delete-refund-customer` — Xóa hoàn cọc
- `DELETE :id/delete-refund-consignment` — Xóa hoàn ký gửi
- `DELETE :id/delete-refund-order-plan` — Xóa hoàn dịch vụ

`GET /crm/payment/get-payment-detail-for-customer` KHÔNG nằm trong module — handler ở `customer_detail.go` (package crm), wire riêng trong `router/crm.go`.

## Thanh toán 1 phần cho nhiều dịch vụ — waterfall theo created_day
- `create-payment-customer` (>1 serviceUsed_idArr): amount chỉ cần `<= tổng nợ` (không còn bắt khớp chính xác)
- Phân bổ waterfall: sort `created_day ASC` (nil = cũ nhất, id tie-break) → trả ĐỦ dịch vụ cũ trước, dư mới chảy qua dịch vụ sau — KHÔNG chia tỷ lệ
- Pure logic: `allocatePaymentWaterfall` (`payment_crud.go`) — tách riêng để unit-test không cần DB
- Đọc nợ (`order_service_used`) + tạo payment PHẢI cùng 1 transaction, khóa `clause.Locking{Strength:"UPDATE"}` — chặn double-submit tạo trùng/vượt payment. Cùng pattern `CreatePaymentOrderFromPayment`.
- Waterfall tạo **nhiều dòng `payment`** (1 dòng/dịch vụ), mỗi dòng mã riêng vì cột `code` UNIQUE — schema KHÔNG có bảng phân bổ riêng.
- Hiển thị/đối soát "phân bổ của một lần thu" → gom theo `parent_id` (cùng đơn hàng). KHÔNG gom theo `payment_time`/`created_at`: hai lần thu trong cùng một giây sẽ dính vào nhau.

## afterSave (rất phức tạp)
- Update customer financials: totalPayment, totalDebt, totalDeposit, totalBonus, refundDeposit, refundService, consignment, refundConsignment
- Update OrderTreatmentPlan: totalPayment, totalDebt
- Update OrderServiceUsed: payment_status
- Log changes (LogPayment)
- Tính commission cho telesale/directSale/doctor/technical/assistant
  - `listRevenues`, `listDoctors`, `listDirectSales`, `source_id` **KHÔNG còn trong payment entity** — đọc trực tiếp từ `order_service_used` (canonical source)
  - Handlers commission JOIN `order_service_used` để lấy các field này

**`payment_status = Done` KHÔNG bị hoàn tiền làm tụt (GOTCHA)** — `calculatorPaymentServiceUsed` tính `payment_status` từ `paymentTotal = SUM(payment_for IN Order/MoveDeposit)` so với `totalEnd`. Refund (`payment_for = RefundService`) ghi riêng vào `refundServiceUsed`, **KHÔNG trừ vào `paymentTotal`**.
- ⇒ hoàn tiền 100% vẫn để `payment_status = Done`. Side-effect nào cần nghĩa "còn hiệu lực" (kho, hoa hồng, loyalty, report) phải tự đối chiếu thêm `refundServiceUsed >= totalEnd`, KHÔNG được chỉ đọc `payment_status`.
- `totalEnd = 0` → luôn ra `Start (1)`, không bao giờ `Done` (điều kiện `totalEnd.IsPositive()`).

## list*/source của payment — đọc từ `order_service_used`
- Payment KHÔNG copy list*/source; export JOIN `order_service_used` trực tiếp khi cần.
- (Lịch sử: helper `setFromServiceUsed` / `setFromCustomer` / `setSourceFromCustomer` chỉ còn để điền `company_id` — đã xoá cùng cột, D15.)

## Commission 1-to-many — list* vs commission tables

→ Xem `skill/skills/working-on-crm/reference/order.md` § "JSON array columns — listRevenues / listDoctors / listDirectSales" (phân công vs derived, write pattern idempotent theo payment_id, JSON_CONTAINS/JSON_TABLE).

## Commission snapshot — rate lock tại thời điểm lên đơn

- `order_service_used` có 6 snapshot fields: `commission_telesale_snap`, `commission_telesale_type_snap`, `commission_direct_sale_snap`, `commission_direct_sale_type_snap`, `commission_doctor_consultant_snap`, `commission_doctor_consultant_type_snap`
- Set 1 lần khi CREATE `order_service_used` — hàm `snapshotCommissionRates` trong `crm/service/order_treatment_plan.go`
- `OrderPlanType=2` (old customer) → dùng `commission_*` (rate cũ); `=1` (new) → `commission_*_new`
- Payment handler đọc snap trước; nếu NULL (đơn cũ chưa có snapshot) → fallback JOIN service table
- KHÔNG re-snapshot khi UPDATE order_service_used — rate bị lock từ lúc lên đơn
- Commission type: `DiscountTypeNormal=1` = VND cố định, `DiscountTypePercent=2` = phần trăm (`pkg/define/order_service_used.go`)

**Rule chung (CRITICAL) — áp dụng mọi rate % tính tiền từ bảng master:** nếu commission/tiền được **recalculate nhiều lần** (mỗi payment, lock/unlock...), phải snapshot rate vào cột riêng NGAY LÚC gán FK — KHÔNG JOIN đọc live từ bảng master mỗi lần recalc, vì admin đổi rate sau đó sẽ làm số tiền đã chốt trước bị tính lại retroactive.

```go
// ❌ SAI — recalc sau này đọc rate MỚI thay vì rate lúc phát sinh
tx.Table("broker").Select("commissionRate").Where("id = ?", brokerID).Scan(&rate)

// ✅ ĐÚNG — snapshot vào cột riêng, recalc chỉ đọc lại cột đã snapshot
tx.Exec(`INSERT INTO broker_commission (..., commissionRateSnap) VALUES (..., ?)`, rateSnap)
```

Với upsert `INSERT ... ON DUPLICATE KEY UPDATE`, chỉ refresh snapshot khi FK thực sự đổi — dùng `IF()` so sánh ngay trong SQL:
```sql
commissionRateSnap = IF(broker_id = VALUES(broker_id), commissionRateSnap, VALUES(commissionRateSnap))
```

Precedent thứ 2: `broker_commission.commissionRateSnap` (loyalty module, affiliate path) — thiếu snapshot cho tới fix 2026-07-02, xem `loyalty/broker.md § broker_commission lifecycle`.

## Commission report endpoints — response format

- Handler telesale/direct-sale/doctor-consultant list + summary dùng flat `c.JSON(http.StatusOK, gin.H{...})` — **KHÔNG** `response.Data`
- FE nhận thẳng `{items, _meta, totalCommission}` — không có lớp `data` bọc ngoài
- Nếu handler đổi sang `response.Data(c, gin.H{...})`, FE service phải thêm `.pipe(map((r: any) => r?.data ?? r))` vào method tương ứng

## calculatorMoneyCustomer — logic theo payment_for
| payment_for | Fields cập nhật | Ghi chú |
|---|---|---|
| FOR_ORDER (4), FOR_MOVE_DEPOSIT (5) | totalPayment, totalDebt, customerType=OLD | totalDebt = totalRevenue - totalPayment |
| FOR_REFUND_SERVICE (1) | refundService, totalPayment | **KHÔNG** update totalDebt |
| FOR_REFUND_DEPOSIT (2) | refundDeposit, totalDeposit | |
| FOR_REFUND_DEPOSIT_KG (6) | consignment, refundConsignment | |
| FOR_DEPOSIT (3) | totalDeposit hoặc consignment | tuỳ deposit_type |
| FOR_CLINIC_INCOME (7), FOR_CLINIC_EXPENSE (8) | **Không gọi updateCustomer** | |

> ⚠️ `FOR_REFUND_SERVICE` chỉ update `refundService` + `totalPayment`, **không** recalc `totalDebt` — đúng theo Yii2.

## Business validation — beforeSave (CRITICAL)

| Loại | Trường check | Rule | Error key |
|---|---|---|---|
| FOR_ORDER | `payment_time` | `payment_time >= osu.created_day` | `payment_before_order_date` |
| FOR_MOVE_DEPOSIT | `payment_of_date` | `payment_of_date >= osu.created_day` | `payment_before_order_date` |

**Tại sao FOR_ORDER dùng `payment_time`, FOR_MOVE_DEPOSIT dùng `payment_of_date`:**
- FOR_ORDER: `payment_of_date` được sync từ `payment_time` trong `beforeSave` → dùng `payment_time` là canonical.
- FOR_MOVE_DEPOSIT: `payment_time` lấy từ cọc gốc (có thể trước ngày tạo đơn) → `payment_of_date` mới là ngày user chọn khi chuyển cọc.

**Bỏ qua validation nếu `osu.created_day IS NULL`** — đơn cũ không có trường này, không block.

> Lý do: `osu.created_day` là trường **có thể chỉnh sửa** ("ngày lên đơn nghiệp vụ"), khác với `created_at` (timestamp hệ thống). Nếu thanh toán được phép đặt ngày trước ngày lên đơn → `osu.payment` (running total) có thể vượt `totalSum` của báo cáo kế toán lọc theo `payment_of_date` cùng kỳ → số liệu phi logic.

## paymentListSelect — shared const yêu cầu JOIN

- `paymentListSelect` (payment.go) SELECT `order_service_used.listRevenues`, `.listDoctors` — bất kỳ handler nào dùng const này PHẢI tự JOIN `order_service_used`.
- `buildPaymentFilters` đã JOIN sẵn; các handler có filter riêng (`ListCustomerDepositMkt` v.v.) phải thêm JOIN độc lập:
  ```go
  Joins("LEFT JOIN order_service_used ON order_service_used.id = payment.serviceUsed_id")
  ```

## broker_commission — thêm/đổi CTV sau khi đã có payment (GOTCHA)

Sau upsert broker_commission trong update path PHẢI gọi `syncBrokerCommission(tx, int(svc.ID))` — nếu không, CTV mới có `amount=0` dù đã thanh toán đủ.

## Auto code generation
Prefix TT, sinh trong `BaseModelWithCode.BeforeCreate` (`internal/models/base.go`) → `codegen.GenerateErr(tx, table, branchID)` — chỉ còn tham số chi nhánh (vế company đã bỏ, D15).

## In lại phiếu thanh toán (print/payment) — date-scope theo payment_of_date (GOTCHA)

- `internal/modules/webapp/print/payment/handler/handler.go` `PrintPayment`: `historyPayment`/`totalPayment` PHẢI filter `payment.payment_of_date <= payment đang in` (tie-break bằng `id` khi trùng ngày), KHÔNG chỉ theo `parent_id`+`serviceUsed_id` — thiếu filter ngày sẽ gộp cả thanh toán phát sinh SAU thời điểm phiếu in, hiển thị sai "đã thanh toán hết".
- Áp dụng chung: mọi endpoint in lại chứng từ lịch sử (reprint) tính cumulative/running-total phải snapshot đến thời điểm bản ghi đang in, không query theo trạng thái hiện tại.
