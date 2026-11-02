# Order (CRM)

## Mục lục

- OrderTreatmentPlan (Đơn hàng)
  - Constants
  - Tạo mới
  - Cập nhật
  - Xóa
  - ⚠️ `order_treatment_plan.total` — ĐÃ BỊ DROP (migration 20260601221056)
- OrderServiceUsed (Dịch vụ sử dụng)
  - Constants
  - Tạo mới
  - Cập nhật
  - ⚠️ Invariant: `totalEnd >= payment` (CRITICAL)
  - Xóa
  - JSON array columns — listRevenues / listDoctors / listDirectSales
  - order_revenue_telesale / order_revenue_doctor_consultant / order_revenue_direct_sale — doanh số 1-nhiều (2026-07, redesign payment-based)
  - ⚠️ numTreatment >= quantity (CRITICAL — validate cả FE lẫn BE)
- OrderPlanForm — FE patterns (order-plan-form.ts)
  - Field naming trong row FormGroup (CRITICAL — tên gây nhầm)
  - onSelectServiceCombo — phải gọi handleService() (CRITICAL)
  - calcNumTreatment — edit mode vs new
  - serviceBaseNumTreatmentMap — dùng serviceInfo.numTreatment trực tiếp
  - Đổi dịch vụ (change-service) — re-init row sau dialog
- Trả góp (OrderInstallment)
  - BE — handleInstallments
  - FE — Lưu ý khi build payload
  - API
- handleCoupon — revert broker_id khi xóa coupon

---


## OrderTreatmentPlan (Đơn hàng)
Tạo khi khách đồng ý mua dịch vụ. Chứa danh sách OrderServiceUsed.

### Constants
- TYPE_NEW = 1 (Đơn hàng mới), TYPE_OLD = 2 (Đơn hàng cũ)

### Tạo mới
- Auto code generation (DH prefix)
- (đã bỏ: `version = 1` — D6 khoá lạc quan theo `updated_at`; `company_id = customer.company_id` — D15)
- afterSave:
  - Tạo OrderServiceUsed[] từ `serviceUsedArr`
  - Tạo Payment nếu có `paymentArr`
  - Tạo CouponUsedByOrderPlan nếu có coupon
  - Update customer: totalRevenue, totalDebt, status → STATUS_CUSTOMER
  - Tạo BrokerCommission nếu có broker
  - `recalcDebtsPlan(tx, order.ID)` — tính `debtsPlan = SUM(osu.totalEnd) - paymentPlan` sau khi services đã lưu

### Cập nhật
- **Optimistic locking**: version check → `version += 1`
- Update OrderServiceUsed[] (upsert + delete orphans)
- `recalcDebtsPlan(tx, order.ID)` — recalc sau handleServices
- Update customer financials

### Xóa
- Check: không cho xóa nếu đã có payment hoặc treatment
- Delete children: OrderServiceUsed, CouponUsedByOrderPlan
- Revert customer financials, delete BrokerCommission

### ⚠️ `order_treatment_plan.total` — ĐÃ BỊ DROP (migration 20260601221056)

**KHÔNG dùng `order_treatment_plan.total` trong bất kỳ query nào.** Column này đã bị xóa khỏi DB.

- **Doanh số đơn hàng** → `SUM(order_service_used.totalEnd)` (correlated subquery hoặc JOIN)
- **debtsPlan** → computed bởi `recalcDebtsPlan()` sau mỗi service CUD: `SUM(osu.totalEnd) - paymentPlan`
- **API response** → `find.go` trả `total` bằng correlated subquery, FE vẫn nhận được field `total`

```sql
-- ❌ NEVER
SELECT order_treatment_plan.total FROM order_treatment_plan

-- ✅ ALWAYS — dùng correlated subquery
SELECT COALESCE((SELECT SUM(osu.totalEnd) FROM order_service_used osu
    WHERE osu.treatment_plan_id = order_treatment_plan.id), 0) AS total
FROM order_treatment_plan
```

---

## OrderServiceUsed (Dịch vụ sử dụng)
Child của OrderTreatmentPlan. Mỗi dịch vụ khách mua = 1 record. Liên kết TreatmentCourse khi bắt đầu điều trị.

### Constants
```
isLock: NOT_LOCK=1, IS_LOCK=2  // KHÔNG phải 0/1
treatment_status: UNTREATED=1, BEING_TREATED=2, COMPLETE=3
payment_status: START=1, FREE=2, 1P=3, DONE=4, REFUND=5
type_discount: NORMAL=1 (VND), PERCENT=2 (%)
USED_SUCCESS = 100 (Hoàn thành 100%)
```

### Tạo mới
- Auto code generation (SV prefix)
- Calculate: totalEnd, service_discount, numTreatment
- Default: isLock=NOT_LOCK, treatment_status=UNTREATED, payment_status=START
- afterSave: Tạo TreatmentCourse nếu company type = dental

### Cập nhật
- Update TreatmentCourse nếu có thay đổi

### ⚠️ Invariant: `totalEnd >= payment` (CRITICAL)

`order_service_used.payment` = SUM thực tế từ bảng `payment` (tính bởi `calculatorPaymentServiceUsed`).
**Không bao giờ được giảm `totalEnd` xuống dưới `payment` đã ghi nhận.**

Guard được enforce ở `handleServices()` (service/order_treatment_plan.go):
- Chỉ kiểm tra khi FE **tường minh gửi `totalEnd`** trong request (tránh false-reject với legacy row có `totalEnd = NULL` khi FE chỉ đổi `note`/`numTreatment`).
- Dùng `svc.Payment` (DB-actual, preserve qua `mergeServiceData`) để so sánh.
- Trả `total_end_below_payment` (i18n key) với HTTP 422.

**Các write path khác vào `totalEnd` (cần lưu ý):**
- `calculatorPaymentServiceUsed` — chỉ ghi `payment`, `refundServiceUsed`, `payment_status`; không đụng `totalEnd`. An toàn.
- `LockServiceUsed` (treatment_service handler) — lock: set `totalEnd = payment` (safe); unlock: restore `totalEnd + discountServiceLock` bằng float64 arithmetic (B.2 violation, cần migrate sang decimal).

### Xóa
- Check: không cho xóa nếu đã có treatment hoặc payment

### JSON array columns — listRevenues / listDoctors / listDirectSales

Tồn tại trên cả `order_treatment_plan` và `order_service_used` (OSU là canonical source):

| Column | Ý nghĩa |
|---|---|
| `listRevenues` | Tư vấn viên phụ trách (1-nhiều) |
| `listDoctors` | Bác sĩ tư vấn phụ trách (1-nhiều) |
| `listDirectSales` | Direct sale phụ trách (1-nhiều) |

- Set khi tạo/cập nhật đơn — tồn tại **trước khi có payment**
- OTP propagate xuống OSU khi save services (`svc.ListRevenues = order.ListRevenues`)
- Commission tables (`order_commission_telesale` v.v.) là dữ liệu derived, sinh ra mỗi payment (FOR_ORDER hoặc FOR_MOVE_DEPOSIT) — không thay thế cho list* columns
  - Write pattern (payment side): `DELETE WHERE payment_id` + `INSERT per person` trong list → idempotent mỗi lần re-payment
  - (Lịch sử: guard `if companyID == nil { return }` vì commission tables từng có `company_id NOT NULL` — đã bỏ cùng cột, D15.)
- Filter: `JSON_CONTAINS(col, CAST(? AS JSON), '$')` — `?` là string số (`strconv.Itoa`)
- Aggregation: dùng `JSON_TABLE` (xem `skill/skills/debugging-backend-queries/reference/filter-mapping.md`)
- **KHÔNG xóa/thay thế 3 cột JSON này** — 18 file report/dashboard (`crm/report`, `workspace/revenue`, `marketing/dashboard`, `treatment_course_doctor`...) đang query trực tiếp qua `JSON_CONTAINS`/`JSON_TABLE`. Bảng `order_revenue_*` (bên dưới) là bổ sung song song (dual-write), không phải migrate thay thế.
- Export cần resolve các cột này ra tên → dùng preload map (`resolveUserNames` +
  `loadNameMap`/`loadUserNamesByGroupID`), KHÔNG JOIN trực tiếp (không JOIN được vào JSON array)
  — dễ bị bỏ sót cột khi build export mới, xem `skill/skills/working-on-ops/reference/export.md`.

### order_revenue_telesale / order_revenue_doctor_consultant / order_revenue_direct_sale — doanh số 1-nhiều (2026-07, redesign payment-based)

3 bảng ghi nhận **doanh số** (khác `order_commission_*` là **hoa hồng**) cho từng user trong `listRevenues`/`listDoctors`/`listDirectSales`.

**Ghi nhận mỗi lần thanh toán** (KHÔNG phải lúc lên đơn — thiết kế ban đầu ghi lúc lên đơn đã bị bỏ vì "ảo": chưa thanh toán mà đã tính doanh số):
- Write path: `handleRevenueTelesale/DoctorConsultant/DirectSale()` (`service/payment_commission.go`), gọi từ `afterSaveAll()` (`payment.go`) — **cùng chỗ, cùng điều kiện** (`PaymentFor IN (FOR_ORDER, FOR_MOVE_DEPOSIT)`) với `handleTelesaleCommission` v.v., nhưng **bỏ hoàn toàn điều kiện rate/serviceCategoryFee** — luôn ghi nhận bất kể có cấu hình hoa hồng hay không.
- Mỗi payment = 1 dòng riêng (key theo `payment_id`, xóa-tạo-lại theo `payment_id` khi payment sửa/xóa) — **không** key theo `order_service_used_id` (sẽ mất doanh số của các payment trước nếu dịch vụ có nhiều lần thanh toán/trả góp).
- `revenue = amount thanh toán / số member trong list đó` (chia đều, `.Round(0)`).
- `amount` = số tiền thanh toán gốc (trước khi chia), `confirm_time` = ngày nghiệp vụ của payment (dùng filter báo cáo, KHÔNG dùng `created_at`).

**Schema** (giống nhau, chỉ khác tên cột role):
```
customer_id, order_treatment_plan_id, order_service_used_id, payment_id,
<revenue_id|doctor_consultant_id|direct_sales_id>,
branch_id (nullable — theo NULL tenant scope rule), [company_id — đã bỏ, D15],
amount decimal(20,2), revenue decimal(20,2), confirm_time, created_at, updated_at
```

**Cleanup theo order/OSU** (`service/order_treatment_plan.go`, `Delete()` + orphan-service branch trong `handleServices()`): `DELETE FROM order_revenue_* WHERE order_service_used_id IN (...)` / `WHERE order_treatment_plan_id = ?` — vẫn cần dù ghi theo payment, vì rows vẫn tham chiếu `order_service_used_id`/`order_treatment_plan_id`.

**Cleanup theo payment** (`service/payment_crud.go` `deletePayment()`): `DELETE FROM order_revenue_* WHERE payment_id = ?` khi payment bị xóa.

**Resync khi sửa đơn** (KHÔNG chỉ lúc thanh toán): sửa `listRevenues`/`listDoctors`/`listDirectSales` sau khi OSU đã có payment → `resyncRevenueForOSU()` (`payment_commission.go`), gọi trong nhánh update của `handleServices()` sau `tx.Save(&svc)`. Tính lại **tất cả payment cũ** của OSU đó (`amount` giữ nguyên theo từng payment, chỉ chia lại theo list mới). Chỉ trigger khi so sánh `existingSvcMap[svc.ID]` với `svc` thấy 1 trong 3 cột JSON đổi — **không** trigger khi chỉ đổi `totalEnd` (`revenue` tính theo `payment.amount` thực nhận, không theo `totalEnd`).

**Gotcha đã gặp**: lấy `listRevenues`/`listDoctors`/`listDirectSales` từ `order_service_used` PHẢI dùng struct + `.Take()`, không `.Scan(&rawBytes []byte)` — xem `skill/skills/debugging-backend-queries/reference/patterns-gotchas-gorm.md`.


### ⚠️ numTreatment >= quantity (CRITICAL — validate cả FE lẫn BE)

BE validation trong `handleServices()` (sau quantity check, dùng biến `q` đã hoist ra ngoài if-block):
```go
if ntv, ok := svcData["numTreatment"]; ok {
    numTreatment := int(helpers.ToInt64(ntv))
    if numTreatment > 0 && numTreatment < q {  // skip khi numTreatment=0 (unlimited)
        return errs.ErrNumTreatmentMinQuantity
    }
}
```
i18n key: `num_treatment_min_quantity` (đã có 4 locale vi/en/jp/fil)

FE validation trong `onSubmit()` trước khi gọi API:
```typescript
const hasInvalidNumTreatment = (raw.services_used as any[]).some(
  (s: any) => s.numTreatment > 0 && s.quantity > 0 && s.numTreatment < s.quantity,
);
```

---

## OrderPlanForm — FE patterns (order-plan-form.ts)

### Field naming trong row FormGroup (CRITICAL — tên gây nhầm)

| Form control | Giá trị thực tế | Template col |
|---|---|---|
| `total` (row) | `qty × unit_price - service_discount - coupon` | Column (5) SUBTOTAL |
| `totalEnd` (row) | `total - discount_other` | Column (7) TOTAL_TO_PAY |
| `total` (rfModel) | `SUM(row.totalEnd)` — set bởi `handleServiceUsed()` | Bottom total |

`handleService()` tính `total` row = `totalTotalService(unitPrice, qty, service_discount, ...)` (đã trừ discount).
`handleServiceUsed()` (order-plan-form.ts:848-876) cộng dồn CẢ 3: `totalEnd`→`rfModel.total`, `discount`→`rfModel.discount`, `discount_other`→`rfModel.discount_other`. KHÔNG recalculate row values (giá/discount từng dòng), chỉ aggregate cấp đơn.

### onSelectServiceCombo — phải gọi handleService() (CRITICAL)

```typescript
// ✅ ĐÚNG
void this.handleService();  // recalc total (col 5) cho từng row, rồi tự gọi handleServiceUsed()

// ❌ SAI — chỉ sum totalEnd, KHÔNG recalc total (col 5) → hiển thị giá chưa trừ service_discount
this.handleServiceUsed();
```

**Lý do:** Khi combo được áp, `quantity` pre-set từ combo child (không thay đổi) → `quantity.valueChanges` KHÔNG fire → `handleService()` KHÔNG tự chạy → `total` form control giữ giá trị `child.total` = `qty × price` (chưa trừ discount).

Khi user chọn free service, `quantity` reset về 0, user nhập lại → `quantity.valueChanges` fire → `handleService()` chạy → OK.

### calcNumTreatment — edit mode vs new

```typescript
private calcNumTreatment(item): number | null {
  // Edit mode: item.id tồn tại → trust stored value (tránh overwrite khi service đổi định nghĩa)
  if (item?.id && item?.numTreatment != null) return item.numTreatment;
  // New/combo selection: recalculate
  const base = item?.serviceInfo?.numTreatment ?? null;
  const qty = item?.quantity || 0;
  if (base != null && qty > 0) return base * qty;
  return item?.numTreatment ?? null;
}
```

### serviceBaseNumTreatmentMap — dùng serviceInfo.numTreatment trực tiếp

```typescript
// ✅ ĐÚNG — base từ service definition
const baseNumTreatment = orderServiceItem?.serviceInfo?.numTreatment || 0;
if (serviceId && baseNumTreatment > 0) {
  this.serviceBaseNumTreatmentMap.set(serviceId, baseNumTreatment);
}

// ❌ SAI — derive từ numTreatment/quantity → float sai khi old data lưu thiếu
const baseNumTreatment = currentQuantity > 0
  ? currentNumTreatment / currentQuantity
  : currentNumTreatment;
```

### Đổi dịch vụ (change-service) — re-init row sau dialog

- onClose dialog: reuse `onSelectService(newServiceInfo, index)` (seed serviceBaseNumTreatmentMap + patch giá/unit + reset teeth), rồi override `patchValue({version: updated.version, numTreatment: null})` — onSelectService không biết version BE trả về và tính numTreatment theo quantity CŨ.
- CLEAR `status_tooth`/teeth UI là CHỦ ĐÍCH nghiệp vụ (quantity chọn theo vị trí răng, dịch vụ người lớn↔trẻ em vị trí khác hoàn toàn) — không "giữ tooth data" như ghi chú cũ story-07.
- Patch tay row (service_id/version/unitName...) mà bỏ qua onSelectService → tái phát bug: giá cũ + numTreatment không recalc khi đổi quantity.

---

## Trả góp (OrderInstallment)

### BE — handleInstallments
- Gọi khi `svcData["installment_config"] != nil` sau khi save OrderServiceUsed
- Skip nếu `period <= 0` hoặc `amount_per_period <= 0` → **phải đảm bảo FE gửi đúng giá trị**
- Kỳ 0: deposit (trả trước). Kỳ 1-N: trả góp, cách nhau `period_interval_days` ngày
- `order_installment` **không có `updated_at`** — dùng struct riêng, không embed `SimpleModel`/`BaseModel`

### FE — Lưu ý khi build payload
- `installmentServicesState` được init khi `totalEnd = 0` (lúc chọn service, chưa nhập quantity) → `depositAmount = 0`, `amountPerPeriod = 0`
- **Phải recalc khi submit** dựa trên `totalEnd` thực tế:
```typescript
const depositAmount = state.depositAmount > 0
  ? state.depositAmount
  : Math.ceil(totalEnd * minDepositPercent / 100);
const amountPerPeriod = Math.ceil((totalEnd - depositAmount) / period);
```
- Reset state khi chọn service mới (`stateMap.delete(index)`) để tránh dùng state cũ
- Áp dụng cho cả `order-plan-form-beauty` và `order-plan-form-dental`

### API
- `GET /treatment-services/installment-of-customer?customer_id=xxx` — danh sách kỳ chưa thu (status IN 1,3,4)

## handleCoupon — revert broker_id khi xóa coupon

- Chỉ revert `broker_id` về `customer.affiliate_id` khi: (1) coupon cũ có `broker_id` VÀ (2) `order.BrokerID == nil` sau `mergeOrder` (staff không chủ động set/clear broker).
- Nếu staff đã xóa broker thủ công (`BrokerID = nil` do FE gửi null) → không revert, tôn trọng staff intent.
- Nếu staff gán broker khác (`BrokerID != nil`) → cũng không revert, coupon broker cũ không override staff choice khi coupon bị xóa.

