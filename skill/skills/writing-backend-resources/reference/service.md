# Service — Logic quan trọng

## Mục lục

- §1 Phân cấp service_type → service
- §2 Status filter — 2 loại endpoint (CRITICAL)
- §3 Selector endpoints
- §4 emr_department_id — bắt buộc (CRITICAL)
- §5 Create — 2 path
- §6 service_material — delete-all + re-insert
- §7 Update — optimistic lock double-check
- §8 Key fields service table
- §9 Toggle endpoints
- §9b servicePosition — define pattern
- §10 Checklist khi thêm selector service mới
- §11 Service Combo — logic & gotchas
  - Schema nhanh
  - numTreatment — quy tắc tính (CRITICAL)
  - FE — service_numTreatment (UI-only field)
  - FE — buildServiceInfo phải include numTreatment (CRITICAL)
  - FE — init numTreatment trong FormGroup (CRITICAL)
  - BE — validation & save pattern
  - Cache invalidation
- §12 Phí khám (`serviceCategory_id = 4`) — CRITICAL
  - Được phép
  - Bị loại trừ
  - `totalRevenue` / `totalDebt`
  - Pattern query chuẩn — exclude phí khám

---


Module: `backend/internal/modules/webapp/config/service/`  
Table chính: `service`, `service_type`, `service_material`

---

## §1 Phân cấp service_type → service

`service.service_type_id FK → service_type.id`

`service_type` và `service` đều KHÔNG có `branch_id` (danh mục chung toàn thương hiệu — 1 thương hiệu/DB; cột `company_id` cũ đã bỏ, D15).

**Cascade disable:** Disable `service_type.status = 0` sẽ ẩn toàn bộ dịch vụ trong nhóm đó khỏi mọi form picker. `service.status` của từng dịch vụ con giữ nguyên — enable lại nhóm là dịch vụ trở lại ngay.

Cascade được áp dụng ở **2 tầng**:
- **BE API selector** — JOIN `service_type st ON st.status = 1` (xem §3)
- **FE IndexedDB** — `ServicesCacheService` inject `ServiceTypeCacheService`, `getAll()` cross-check với `getAllServiceTypes()` (chỉ active type IDs). `getAllServicesUnfiltered()` vẫn trả tất cả để preserve selected disabled service khi update form.

---

## §2 Status filter — 2 loại endpoint (CRITICAL)

| Loại | Mục đích | Filter |
|---|---|---|
| **Management list** `GET /service` | Admin quản lý, xem cả inactive | Optional `?status=0/1` qua `ApplyFilter` |
| **Selector/lookup** (4 endpoints dưới) | FE form picker | `service.status = 1` **AND** `service_type.status = 1` |

**IndexedDB sync** (`/sync?tables=services`) — KHÔNG filter status. Sync toàn bộ kể cả disabled để FE vẫn hiển thị label đúng khi record cũ tham chiếu dịch vụ đã bị tắt sau đó.

---

## §3 Selector endpoints

Cả 4 đều JOIN `service_type` để respect cascade disable của nhóm:

```go
// Pattern bắt buộc cho mọi selector mới
q := tenant.ScopedDBNullBranch(c, h.DB, "service").Table("service").
    Joins("JOIN service_type st ON st.id = service.service_type_id AND st.status = 1").
    Where("service.status = 1")
```

| Endpoint | Handler | Dùng khi |
|---|---|---|
| `GET /service/load-service-by-type?listType[]=x` | `LoadServiceByType` | FE lọc IDs theo nhóm |
| `GET /service/load-by-keyword?keyword=x` | `LoadServiceByKeyword` | Autocomplete (tham số `?companyId=` cũ đã bỏ, D15) |
| `GET /service/load-list-service-by-keyword?keyword=x` | `LoadListServiceByKeyword` | Select dropdown (trả `{value, label}`) |
| `GET /service/load-service?keyword=x` | `LoadService` | Generic picker, limit 20 |

**Performance:** `service.service_type_id` đã có index. `service_type` là bảng nhỏ, JOIN qua PK. `service` không nằm trong `branchTables` ⇒ `ScopedDBNullBranch` trên `service` là no-op (trước đây sinh `service.company_id` qualified để khỏi ambiguous với `service_type.company_id` — đã bỏ, D15).

---

## §4 emr_department_id — bắt buộc (CRITICAL)

Mọi service phải gắn 1 khoa thực hiện (`emr_department_id`). BE tự sync `department_type` server-side — FE **không** gửi field này.

```go
// resolveDepartmentTypeForUpdates / resolveDepartmentTypeForStruct
// Lookup emr_department.department_type → ghi vào service.department_type
// Reject nếu emr_department_id = 0 hoặc department_type = 0
```

Lỗi: `errEmrDepartmentRequired` → `response.HandleError` → i18n key `emr_department_required`.

---

## §5 Create — 2 path

```
POST /config/service          → JSON path (ShouldBindJSON → Service struct)
POST /config/service          → multipart path (form-data + optional image upload)
```

**Sequence cả 2 path:**
1. Parse body / multipart
2. Auto-generate `code` nếu FE không gửi: `codegen.GenerateErr(tx, "service", bidPtr)` — chạy TRONG tx (rollback trả lại số thứ tự); tham số company đã bỏ, D15
3. (đã bỏ — `resolveDepartmentTypeForUpdates`/`emr_department_id` xoá ở story-12, D16)
4. `tx.Create(...)` trong transaction
5. `tx.Update("SortOrder", item.ID)` — SortOrder mặc định = id (auto-sort)
6. `saveServiceMaterials(tx, id, raw)` — delete old + insert new (xem §6)
7. Commit → `dexie.BumpDexieTable(h.DB, "services")`

---

## §6 service_material — delete-all + re-insert

```go
// saveServiceMaterials — áp dụng cho cả Create lẫn Update
tx.Where("service_id = ?", serviceID).Delete(&ServiceMaterial{})  // wipe old
for _, m := range arr { tx.Table("service_material").Create(...) } // insert new
```

`raw` có thể là `[]interface{}` (JSON path) hoặc `string` (multipart — FormData serialize array thành JSON string). Hàm xử lý cả 2 dạng.

---

## §7 Update — optimistic lock double-check

```go
// Version check ở app layer
if helpers.ToInt64(v) != int64(item.Version) { response.Conflict(...) }

// Double lock ở DB layer
tx.Model(&item).Where("id = ? AND version = ?", item.ID, dbVersion).Updates(updates)
// RowsAffected == 0 → conflict (race condition) → Rollback + 409
```

`version` FE gửi lên bắt buộc. `updates["version"] = dbVersion + 1` trước khi ghi.

JSON array fields phải `json.Marshal` trước khi GORM update (tránh serialize thành SQL tuple):
- `treatment_step_ids`

Cột `installment_config` đã bỏ khỏi bảng `service` (02/11/2026): key phẳng nằm trong `removedServicePayloadKeys`, key lồng `installment_config[xxx]` (PWA cũ) bị `normalizeFormPayload` gỡ — không còn gom thành JSON.

---

## §8 Key fields service table

| Field | Ý nghĩa |
|---|---|
| `serviceCategory_id` | 1 = Dịch vụ, 2 = Sản phẩm, 3 = Liệu trình, 4 = Phí khám — phân loại loại hình |
| `numTreatment` | Số buổi điều trị |
| `servicePosition` | JSON array `string[]` — vị trí cơ thể dịch vụ áp dụng, format `"<specialty>.<area>"` (vd `"dental.tooth"`, `"msk.knee"`) |
| `is_care` | Được chăm sóc sau điều trị (default 1) |
| `isUpdateMaterial` | Cho phép KTV cập nhật NVL khi thực hiện |
| `validity_days` | Số ngày hiệu lực của dịch vụ |
| `time_reminder` | Ngày nhắc lịch tái khám sau điều trị |
| `time_expired_at` | Thời gian bảo hành |
| `emr_department_id` | Khoa thực hiện (bắt buộc, xem §4) |
| `emr_room_id` | Phòng thực hiện mặc định |
| `emr_service_code` | Mã DVKT theo BYT |
| `commission_*` | Hoa hồng theo vai trò (telesale, directSale, technical, assistant, doctor) |
| `SortOrder` | Tự set = id sau insert, FE có thể đổi qua check-position |

---

## §9 Toggle endpoints

```
POST /service/handle-is-care      → toggle service.is_care
POST /service/update-material     → toggle service.isUpdateMaterial
```

Cả 2 dùng `toggleFieldWithBump(c, "service", field, "services")` — flip 0↔1 + `BumpDexieTable`.

`servicePosition` KHÔNG có toggle — chỉ edit qua form dialog (multiselect `p-multiselect`).

---

## §9b servicePosition — define pattern

`servicePosition` là `datatypes.JSON` (entity) / `string[]` (FE model). Format code: `"<specialty>.<area>"`.

**Define tập trung:**
- BE: `pkg/define/service_position.go` — constants `ServicePositionDentalTooth = "dental.tooth"`, v.v.
- FE: `src/app/define/service-position.define.ts` — `SERVICE_POSITION_OPTIONS_BY_DEPT: Record<number, ServicePositionOption[]>` + `getServicePositionOptions(departmentType)`

**FE ServiceForm — populate options:**
```ts
// init + reactive khi department_type thay đổi
this.servicePositionOptions.set(getServicePositionOptions(this.rfModel.get('department_type')?.value));
this.rfModel.controls['department_type'].valueChanges
  .pipe(takeUntilDestroyed(this.destroyRef))
  .subscribe((dtype) => {
    this.servicePositionOptions.set(getServicePositionOptions(dtype));
    this.rfModel.patchValue({ servicePosition: [] });
  });
```

**Khi thêm chuyên khoa mới:** thêm entries vào cả 2 file define + i18n key `define.service_position.<specialty>.<area>` trong 4 locale.

---

## §10 Checklist khi thêm selector service mới

- [ ] JOIN `service_type st ON st.id = service.service_type_id AND st.status = 1`
- [ ] `WHERE service.status = 1`
- [ ] Qualify tất cả column với `service.` (tránh ambiguous khi JOIN `service_type` — cả hai có `id`, `name`, `status`…; `company_id` đã bỏ, D15)
- [ ] IndexedDB sync endpoint KHÔNG filter status (xem §2)

---

## §11 Service Combo — logic & gotchas

> ⚠ Lịch sử: module `config/service_combo/` KHÔNG còn trong CRM standalone (không có trong `webapp/config/`, không có migration) — mục này chỉ để tra bản ERP.

Tables: `service_combo` (header) · `service_combo_child` (detail) · `service_combo_type` (global; lịch sử: no `company_id`, D15)

### Schema nhanh

| Field | Table | Ghi chú |
|---|---|---|
| `serviceCombo_id` | `order_treatment_plan` | FK nullable — link đơn hàng với combo |
| `numTreatment` | `service_combo_child` | **Computed**: `quantity × service.numTreatment` |
| `isUpdate` | `service_combo` | `0` = không cho sửa khi lên đơn, `1` = cho phép |
| `totalMoneyCombo` | `service_combo` | `SUM(child.totalEnd)` |

### numTreatment — quy tắc tính (CRITICAL)

```
service_combo_child.numTreatment = quantity × service.numTreatment
```

- `service.numTreatment = 0` → unlimited → bỏ qua validation
- `numTreatment >= quantity` bắt buộc (validate cả FE lẫn BE)
- **KHÔNG** dùng `totalEnd` để tính `moneyPerTreatment` khi check payment threshold — dùng `unit_price × quantity` (xem treatment.md §"Check thanh toán trước khi tạo điều trị")

### FE — service_numTreatment (UI-only field)

Form group có thêm control `service_numTreatment` để lưu base value từ service (KHÔNG gửi BE — không có trong allowlist `saveComboChildren`).

```typescript
// changeService() — khi chọn service mới
const qty = form.controls['quantity'].value || 1;
form.patchValue({
  service_numTreatment: service.numTreatment ?? null,
  numTreatment: service.numTreatment != null ? qty * service.numTreatment : null,
});

// handleServiceComboChild() — khi quantity thay đổi
const baseNum = form.controls['service_numTreatment'].value;
const qty = form.controls['quantity'].value || 0;
numTreatment = baseNum != null ? qty * baseNum : null;
```

### FE — buildServiceInfo phải include numTreatment (CRITICAL)

`ServicesHelperService.buildServiceInfo()` phải map `numTreatment`:
```typescript
numTreatment: service?.numTreatment ?? null,
```
Thiếu field này → edit mode load: `service.numTreatment = undefined` → `numTreatment` bị set `null`, xóa giá trị đã lưu.

### FE — init numTreatment trong FormGroup (CRITICAL)

```typescript
// ✅ ĐÚNG — nullish coalescing, giữ 0 (unlimited)
numTreatment: [serviceComboChild?.numTreatment ?? null],

// ❌ SAI — || 1 overwrite 0 → stored "unlimited" thành "1 buổi"
numTreatment: [serviceComboChild?.numTreatment || 1],
```

### BE — validation & save pattern

```go
// validateComboChildrenNumTreatment — GỌI TRƯỚC tx.Begin() trong CẢ Create VÀ Update
// (trong Update: gọi TRƯỚC tx.Model().Updates(), không phải sau)
// numTreatment = 0 (unlimited) bỏ qua
if numTreatment > 0 && numTreatment < q {
    return i18n.T(c, "num_treatment_min_quantity") // → response.Validate 422
}

// saveComboChildren — Master-Detail: delete all + re-insert
// allowedCols: service_combo_id, service_id, quantity, numTreatment,
//              unit_price, service_discount_type, total, service_discount, totalEnd
```

### Cache invalidation

```go
h.Cache.Delete("dropdown:service_combo:list")
h.Cache.InvalidateTag("service_combo:list")
dexie.BumpDexieTable(h.DB, "serviceCombos")
```

---

## §12 Phí khám (`serviceCategory_id = 4`) — CRITICAL

Constants: `pkg/define/order_service_used.go` → `define.ServiceCategoryFee = 4`

### Được phép
- Vào `order_treatment_plan` bình thường
- Điền commission fields (telesale, directSale, doctor) trên form — trường vẫn hiển thị

### Bị loại trừ
| Rule | File | Logic |
|---|---|---|
| Commission records | `payment_commission.go` | Guard `serviceCategory_id == 4` → `return` trước khi insert |
| Treatment list | `treatment_service.go` | Filter `serviceCategory_id NOT IN (2, 4)` |
| Customer care Bước 1 | `treatment_course.go` | Skip khi `serviceCategory_id IN (2, 4)` |
| Customer care Bước 2 | `treatment_course.go` | Skip khi `serviceCategory_id IN (2, 4)` |
| `customerType` → TYPE_OLD | `payment.go` | Không nâng nếu order chỉ có phí khám (LEFT JOIN check nonFeeCount > 0) |
| 3 report công nợ điều trị (accountant) | `treatment_debts`/`treatment_session_debts`/`treatment_session_period` `handler.go` | INNER JOIN `service` + `WHERE serviceCategory_id IN (1,3)` — loại 2 (sản phẩm), 4 (phí khám); vì tính theo buổi/hợp đồng điều trị nên chỉ áp dụng dịch vụ có buổi |

### `totalRevenue` / `totalDebt`
- **Include bình thường** — phí khám vẫn tính vào nợ và doanh thu tài chính (khác 3 report công nợ điều trị ở trên, vốn scope hẹp theo buổi điều trị)
- Chỉ `avg_order_value` mới exclude

### Pattern query chuẩn — exclude phí khám
```sql
-- avg_order_value: exclude phí khám khỏi cả revenue lẫn order_count
SELECT COALESCE(SUM(osu.totalEnd), 0) AS eff_revenue,
       COUNT(DISTINCT otp.id)          AS eff_order_count
FROM order_treatment_plan otp
LEFT JOIN order_service_used osu ON osu.treatment_plan_id = otp.id
LEFT JOIN service s ON s.id = osu.service_id
WHERE otp.customer_id = ?
AND (s.id IS NULL OR COALESCE(s.serviceCategory_id, 1) != 4)
-- LEFT JOIN để không drop OSU khi service bị hard-delete (NULL → treat as non-fee)
```
