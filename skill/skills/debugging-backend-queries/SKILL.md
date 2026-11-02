---
name: debugging-backend-queries
description: >-
  Data patterns (Master-Detail Save, Tenant scope, Raw SQL, JSON, Sort, Codegen) và tập gotchas
  CRITICAL đã gây bug thật (branch filter/IsCrossBranchEnabled, FK lookup CrossBranchLookup,
  GetBranchActiveID, HandleError vs ServerError, delete guard, GORM read/write pitfalls,
  datetime/timezone, data-integrity, routing/HTTP, bulk import Excel, audit log, filter mapping,
  runtime diagnostics). Dùng skill này KHI đang SỬA hoặc DEBUG một query/handler backend Go đang
  sai kết quả, trước khi đụng GORM Transaction/Preload/Subquery, trước khi filter theo
  branch, hoặc khi cần tra runtime diagnostics/log-history. KHÔNG dùng cho việc tạo mới
  resource — xem writing-backend-resources cho việc đó.
---

# Debug backend queries & gotchas (Go)

> Gotchas theo chủ đề (11 file, xem bảng dưới): `reference/`
> Runtime diagnostics + audit log + filter mapping + bulk import: cũng trong `reference/`

| File | Khi nào đọc |
|------|-------------|
| `reference/patterns-gotchas.md` | Gotchas CRITICAL — hợp đồng handler: branch filter/IsCrossBranchEnabled, FK lookup CrossBranchLookup, GetBranchActiveID, HandleError vs ServerError, delete guard, i18n không nhúng err.Error(), sync-list DELETE-before-early-return, bind body optional |
| `reference/patterns-gotchas-datetime.md` | Timezone day-boundary, cột SQL DATE/DATETIME format ở BE (không đẩy FE parse), đổi format string ngày phải trace consumer FE thật, DATE + parseTime → RFC3339 |
| `reference/patterns-gotchas-data-integrity.md` | Transaction 1 connection, lock order cha→con, FK thiếu constraint cleanup thủ công, ToDecimal từ map, operator phủ định trên cột nullable, cắt chuỗi theo biên rune, masked credentials, codegen/seed theo branch, source_id financial metrics |
| `reference/patterns-gotchas-routing.md` | Routing/HTTP layer gotchas: Query param type mismatch, Pagination c.JSON vs response.Data, Gin wildcard param reuse |
| `reference/patterns-gotchas-http.md` | Outbound HTTP gotchas: force tcp4 khi IPv6 fail, secret trong URL rò vào log |
| `reference/patterns-gotchas-gorm.md` | GORM v2 gotchas — read/query path: Scan interface{}, new(expr), Transaction, Preload ORDER BY, Subquery scope, only_full_group_by |
| `reference/patterns-gotchas-gorm-write.md` | GORM v2 gotchas — write path: Create tag `default:` nuốt mọi zero-value, decimal→map, OnConflict typed model, Reload upsert ORDER BY, Select(cols) explicit, reuse Service(tx) SAVEPOINT, created_by FK, rename field → Omit()/whitelist string, Create(&struct) có relation mang PK cướp child record khác, FK RESTRICT phải dọn/chặn ở beforeDelete |
| `reference/patterns-gotchas-gorm-subquery.md` | GORM v2 subquery/Session reuse: Paginate mutate q in-place (.Session() sau không hồi tố), Order("") không xoá ORDER BY, Session({}) giữ nguyên Group/Having |
| `reference/patterns-gotchas-hr-dept.md` | HR department constants: `HrDepartmentConvert`/`HrDepartmentMarketing` sai giá trị thật |
| `reference/patterns-bulk-import.md` | Bulk import Excel: FK-by-name batch resolve, numeric-FK existence check, accumulate-all-errors → 1 dialog, SanitizeErrorMessage, đọc ngày/serial (RawCellValue + parse khoan dung) |
| `reference/log-history.md` | Audit trail: LogChanges API, 5 bảng log_, delete pattern |
| `reference/filter-mapping.md` | Filter query mapping |
| `reference/runtime-diagnostics.md` | Ghi log chẩn đoán khi hệ thống đang chạy (`app_event_log` + `pkg/applog`) + workflow kiểm chứng logic bằng SQL thay vì grep file log |
| `reference/perf-list-endpoints.md` | "Màn này chậm khi nhiều dữ liệu": MySQL không loại-bỏ-left-join, `COUNT(*)` của `Paginate` trên 298 endpoint, `LIKE '%kw%'` xuyên join, cách sửa tìm kiếm đã đo + 4 biến thể đã loại, quy trình đo trên DB clone 500k |


## Master-Detail Save

```
1. Extract children TRƯỚC khi delete khỏi updates
2. Wrap trong transaction
3. Save parent
4. afterSave: delete old children → create new (chỉ khi FE gửi field)
5. Commit hoặc rollback
```

Đã implement: config/service.go, service-combo.go, Med, OrderPlan, Payment, Loyalty.

## Optimistic Locking
Danh sách entity + flow version → Xem `skill/skills/backend-hard-rules/SKILL.md` §Optimistic Locking.

## Auto Code Generation (codegen)
Danh sách 13 tables + cách gọi → Xem `skill/skills/backend-hard-rules/SKILL.md` §Codegen.

## JSON Fields
MySQL JSON columns scan thành string trong `map[string]interface{}`.
Fix: `helpers.ParseJSONFields()` — schema-aware từ INFORMATION_SCHEMA.

### Single-record GET — bắt buộc gọi ParseJSONFieldsForTables (CRITICAL)

`pagination.Paginate` tự gọi `ParseJSONFieldsForTables` + `ParseDecimalFieldsForTables`.
Single-record handler KHÔNG đi qua Paginate → phải gọi thủ công.

```go
// ❌ NEVER — JSON columns về []uint8, FE hỏng
var items []map[string]interface{}
q.Limit(1).Find(&items)
item := items[0]

// ✅ ALWAYS
var items []map[string]interface{}
q.Limit(1).Find(&items)
helpers.ParseJSONFieldsForTables(items, "table_name")  // "appointment", "customer", ...
item := items[0]
```

**Checklist GET /:id handler:**
- [ ] `Limit(1).Find(&items)` — không `First` với map (GORM thêm ORDER BY sai)
- [ ] `helpers.ParseJSONFieldsForTables(items, "table_name")` trước `items[0]`
- [ ] `helpers.ParseDecimalFieldsForTables(items, "table_name")` nếu table có decimal

### Raw-map handler tự build SELECT — JSON string/int slice không qua ParseJSONFieldsForTables

`ParseJSONFieldsForTables` chỉ cover schema-registered tables. Handler dùng raw `map[string]any` + custom SELECT → JSON column về `[]uint8`; Gin marshal `[]byte` thành **base64**, không phải JSON array.

```go
// ❌ r["servicePosition"] → base64 string ở FE
"servicePosition": r["servicePosition"],

// ✅ dùng helper riêng
func parseJSONStringSlice(raw any) []string {
    var b []byte
    switch v := raw.(type) {
    case []uint8: b = v
    case string:  b = []byte(v)
    default:      return nil
    }
    var result []string
    json.Unmarshal(b, &result)
    return result
}
// tương tự parseJSONIntSlice() đã có trong treatment_service handler
"servicePosition": parseJSONStringSlice(r["servicePosition"]),
```

## MySQL Driver Go — Type Mapping

| MySQL type | Go type |
|---|---|
| tinyint(1) | `int8` |
| int | `int32` |
| bigint | `int64` |
| decimal, float | `string` ("10000.00") |
| varchar, text | `string` |
| json | `[]uint8` |

`toInt`/`toFloat(v any)` phải xử lý đủ: `int8`, `int16`, `int32`, `int64`, `float32`, `float64`, `string`, `[]uint8` — thiếu case nào sẽ silent-return 0 thay vì lỗi rõ ràng.

**Hệ quả cho response JSON (bug MOB-010 2026-07-28)**: handler scan raw SQL vào `[]map[string]any` rồi `c.JSON(rows)` → cột/SUM DECIMAL thành Go `string` trong map → client nhận `"1054000000.00"` (string có quote) thay vì number, lệch swagger `number` → generated client mobile crash type-cast. Repo đã set `decimal.MarshalJSONWithoutQuotes = true` global (`main.go`) nên field `decimal.Decimal` luôn ra number đúng — vậy response có field tiền BẮT BUỘC scan vào struct với `decimal.Decimal`, KHÔNG map-scan generic. Đã dính: `CrmPaymentBranchOverview` (đã fix scan struct, các endpoint tiền khác actor crm + marketing đều đúng sẵn).

## Read-back sau CREATE — KHÔNG dùng ScopedDB (CRITICAL)

```go
// ❌ NEVER — branch scope fail nếu branch_id = NULL
tenant.ScopedDBNullBranch(c, h.DB, "table").Table("table").Where("id = ?", newID).Take(&item)

// ✅ ALWAYS — exact ID + branch scope
h.DB.Table("table").Where("id = ? AND branch_id = ?", newID, branchID).Take(&item)
```

## Tenant scope — `ScopedDBNullBranch` là default duy nhất (CRITICAL)

Rule đầy đủ + example ❌/✅ → Xem `skill/skills/backend-hard-rules/SKILL.md` §Tenant scope. `tenant.CrossBranchLookup` chỉ dùng cho FK lookup / cross-branch relation (xem bên dưới).

`branch_id IS NULL` cover 2 ngữ nghĩa:

| Table | NULL nghĩa là gì |
|---|---|
| `customer` | Chưa gán chi nhánh |
| `emr_department` | Áp dụng toàn thương hiệu (mọi chi nhánh) |

IndexedDB sync: set `NullableBranch: true` trong `configTables` map (`internal/modules/webapp/config/indexeddb/handler/sync.go`).

### Bypass branch scope theo option riêng (không phải cross-branch global id=18)

`tenant.ScopedDBNullBranchSkipBranch(c, db, table, skipBranch bool, alias...)` — sibling function của `ScopedDBNullBranch`. Branch filter bị bỏ qua khi `skipBranch=true`. Dùng khi 1 flow cụ thể cần bypass branch scope theo 1 `user_option` riêng, không đụng option 18 chung (`IsCrossBranchEnabled`) hay ảnh hưởng các endpoint khác dùng chung table.

```go
q := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_service_used", someOptionEnabled(h.DB), "osu")
```

Precedent: `OPTION_CROSS_BRANCH_TREATMENT` (id=23) — đã gỡ 2026-11-01, các flow đó nay truyền hằng `true` (xem `skills/managing-crm-options/SKILL.md` §Option 21–23).

## Global Config Table — không có branch_id

- Query thẳng `h.DB` — KHÔNG qua `ScopedDBNullBranch`
- Không cần cột `branch_id`/`status` kiểu tenant-owned — bảng cấu hình global dùng struct riêng, không nhất thiết embed `BaseModel`/`SimpleModel`

```go
type ServiceTreatmentStep struct {
    ID        uint   `gorm:"primaryKey" json:"id"`
    Name      string `gorm:"size:255;not null" json:"name"`
    Desc      string `gorm:"type:text" json:"desc"`
    Status    *int   `gorm:"default:1" json:"status"`
    SortOrder *int   `gorm:"column:sort_order" json:"sort_order"`
    CreatedAt int64  `gorm:"column:created_at" json:"created_at"`
    CreatedBy *uint  `gorm:"column:created_by" json:"created_by"`
}
```

Handler set timestamp thủ công:
```go
uid := middleware.GetUserID(c)
item.CreatedAt = time.Now().Unix()
item.CreatedBy = &uid
h.DB.Create(&item)
```

## JSON ID Reference

```go
// service.treatment_step_ids = [1, 2, 3]
TreatmentStepIds datatypes.JSON `gorm:"column:treatment_step_ids" json:"treatment_step_ids"`
```

- Dùng khi chỉ cần list ID đơn giản, không cần sort_order hay metadata per-relation
- Master-detail dùng bảng junction khi cần sort_order, quantity, metadata

## Default Sort Order Convention

```go
// ✅ Table có sort_order
db.Table("service_type").Order("sort_order DESC")

// ✅ Fallback
db.Table("payment_method").Order("id DESC")

// ❌ NEVER — thứ tự không xác định
db.Table("payment_method").Find(&items)
```

IndexedDB sync fetcher dùng cùng convention.
Exception: `services` sort bằng `SortOrder DESC` (PascalCase legacy).

`ApplySort` TỰ nối `<bảng gốc>.id DESC` ở cả 3 nhánh (có `sort` param · không có · `sort` bị reject) — KHÔNG chain tie-breaker tay nữa.

```go
q = query.ApplySort(c, q, "payment.payment_of_date DESC", ...) // → ORDER BY payment.payment_of_date DESC, payment.id DESC
```

- `defaultOrder` không có tiền tố bảng (`"records DESC"`, `"count DESC"`) → không suy được pk → không thêm gì. Đây cũng là lưới chặn lỗi 1055 cho query `GROUP BY` không theo pk.
- Query KHÔNG qua `ApplySort` mà có `LIMIT/OFFSET` → tự thêm tie-breaker; thiếu → row lặp ở trang này và MẤT khỏi trang kia, không lỗi nào được ném ra.
- Query có `GROUP BY` → tie-breaker là **khoá nhóm**, KHÔNG phải `id` (thêm `id` → lỗi 1055).
- Bảng có JOIN → qualify `<bảng>.id`; `id` trần chỉ dùng được khi câu lệnh đơn bảng.
- Cần tie-breaker theo **chiều khác** `id DESC` (vd `id ASC` để giữ thứ tự tạo trong cùng đơn) → đặt NGAY TRONG `defaultOrder`: `ApplySort(c, q, "osu.created_day DESC, osu.id ASC", …)`. `withTiebreaker` thấy pk đã có thì không nối thêm. Chain `.Order("osu.id ASC")` rời phía sau là **dead code câm lặng**: ORDER BY thành `created_day DESC, id DESC, id ASC`, MySQL đã xác định thứ tự ở `id DESC` nên vế cuối vô nghĩa — comment nói một đằng, SQL chạy một nẻo.

## `tenant.CrossBranchLookup` — cross-branch intentional query

Dùng khi table nằm trong `branchTables` nhưng cần lấy data cross-branch (cùng thương hiệu/DB), với điều kiện query phải có FK filter chặt (không phải free search):

```go
// ✅ Đúng: broker_id = ? giới hạn chặt, không leak data ngoài ý muốn
q := tenant.CrossBranchLookup(h.DB, "order_service_used").
    Table("order_service_used").
    Where("order_service_used.broker_id = ?", id)

// ❌ Sai: free paginate không có FK filter
q := tenant.CrossBranchLookup(h.DB, "order_service_used").
    Table("order_service_used").Find(&items)
```

- IDOR guard bắt buộc: verify FK target (vd broker) thuộc tenant trước khi query
- `CrossBranchLookup` là hàm identity (không tự lọc gì) — chỉ dùng khi handler ĐÃ tự filter chặt bằng FK cụ thể như ví dụ trên

## Khoá "chỉ MỘT lượt chạy" — 3 điều kiện, thiếu cái nào cũng hỏng

Bảng trạng thái kiểu `*_run`/`*_job` với một row `status = 'running'` làm khoá (đồng bộ, backfill,
import). Cả 3 điều dưới đây đều cần; mỗi cái từng thiếu một lần và hỏng theo một kiểu khác nhau:

1. **Khoá phải ở DB, không ở Go.** `Count(status='running')` rồi `Create` là TOCTOU — hai lời gọi
   đồng thời (người bấm 2 lần, hoặc lượt tự động trùng lúc bấm tay) đều đếm được 0 rồi cùng insert.
   Dùng UNIQUE có điều kiện (functional index — `skills/writing-mysql-migrations/reference/mysql97.md`
   §3) và map lỗi **1062** về sentinel "đang chạy"; giữ `Count` chỉ để có thông điệp đẹp ở ca thường.
2. **Khoá phải TỰ MỞ được, và ngưỡng chết đo `updated_at` chứ KHÔNG `started_at`.** Tiến trình chết
   giữa chừng để lại row `running` vĩnh viễn. Nhưng đo từ lúc BẮT ĐẦU thì một lượt chạy dài vẫn bị
   dọn nhầm ⇒ khoá tự mở giữa chừng ⇒ hai tiến trình chạy song song — đúng thứ khoá sinh ra để chặn.
   Đo từ lần có tiến độ gần nhất, và engine phải có **nhịp tim** (chạm `updated_at` cả khi bộ đếm
   không đổi, vd pha phân trang dài không phát sinh bản ghi nào).
3. **Dọn phải chạy ở CẢ đường đọc.** Chỉ dọn trong hàm mở lượt thì FE đọc trạng thái vẫn thấy `running`
   ⇒ nút bị vô hiệu ⇒ không ai gọi được hàm mở lượt ⇒ không bao giờ dọn. Khoá tự nhốt mình.

## IDOR guard — bắt buộc trên CẢ Create/Update/Delete, không chỉ List/Get

Bảng con chỉ scope qua FK cha (không có `company_id`/`branch_id` riêng, vd `broker_commission` scope qua `broker_id`) dễ bị quên guard tenant ở Create/Update/Delete — thường chỉ List/Get có `ScopedDBNullBranch`/subquery verify, còn CUD gọi thẳng service.

```go
// ❌ Sai — Update/Delete không verify id thuộc tenant, chỉ check ở List/Get
func (h *Handler) UpdateX(c *gin.Context) {
    id := helpers.GetIDParam(c)
    // bind input, gọi service luôn — record cross-tenant vẫn update được
}

// ✅ Đúng — verify record hiện tại + FK mới (nếu request cho phép đổi FK) TRƯỚC khi gọi service
if !h.xInScope(c, id) {
    response.NotFound(c)
    return
}
if !h.fkInScope(c, input.FkID) {
    response.NotFound(c)
    return
}
```

Precedent: `loyalty/broker_commission` Create/Update/Delete hoàn toàn thiếu tenant verify — fix 2026-07-02 (`brokerInScope`/`brokerCommissionInScope` trong `handler.go`).

## (đã bỏ, D15) `company_id` scope — mô hình multi-company cũ

Trước D15 (1 thương hiệu/DB, story-62), mục này mô tả pattern "table có `company_id` nhưng không có
`branch_id`" (raw `WHERE table.company_id = ?` / fallback `IN companyIDs`) và pattern raw SQL company
scope (`companyScope(alias)` helper viết tay, context key `companyActiveId`/`companyIds`). Cột
`company_id` đã xoá hoàn toàn khỏi schema, `GetCompanyActiveID`/`GetCompanyIDs` đã xoá khỏi middleware
(story-41) — KHÔNG còn pattern raw-company-scope nào trong code. Bảng có `branch_id` → dùng
`ScopedDBNullBranch` (§Tenant scope ở trên); bảng KHÔNG có `branch_id` (`log_*`, global config...) →
query thẳng `h.DB`, không cần raw scope thủ công.

---

## Timezone-safe MySQL date grouping

`FROM_UNIXTIME()` dùng MySQL server TZ (thường UTC) → month/day label sai khi company TZ khác UTC.

```go
// ❌ sai timezone khi MySQL server = UTC, company = +07:00
DATE_FORMAT(FROM_UNIXTIME(se.created_at), '%Y-%m')

// ✅ dùng tz.SqlFromUnix() + tz.Offset()
loc := tz.LocFromContext(c, h.DB)
offset := tz.Offset(loc)            // → "+07:00"
expr := tz.SqlFromUnix("se.created_at", offset)
// expr = CONVERT_TZ(FROM_UNIXTIME(se.created_at), @@session.time_zone, '+07:00')
// dùng trong query:
h.DB.Raw(`SELECT DATE_FORMAT(`+expr+`, '%%Y-%%m') AS month ...`)
```

Áp dụng: mọi raw SQL GROUP BY month/day/year từ unix timestamp.

---

## Pattern — TOCTOU fix cho state-machine endpoint

Endpoint trigger trạng thái (StartSend, Approve, Publish…): **KHÔNG** Read→Check→goroutine (2 request đồng thời đều pass check). Dùng **optimistic lock UPDATE + RowsAffected**.

```go
// ❌ SAI — TOCTOU
if item.Status == StatusSending { response.Validate(...); return }
go svc.Execute(...)

// ✅ ĐÚNG — optimistic lock
res := h.DB.Model(&T{}).
    Where("id = ? AND status IN ?", id, []int{StatusDraft, StatusPaused}).
    Updates(map[string]any{"status": StatusSending, "updated_at": time.Now().Unix()})
if res.RowsAffected == 0 {
    response.Validate(c, i18n.T(c, "already_sending"))
    return
}
go svc.Execute(companyID, id)
```

- Chỉ 1 request thắng khi `RowsAffected == 1`; request thứ 2 gặp `RowsAffected == 0` → trả lỗi
- Áp dụng mọi nơi: approve order, start campaign, publish template, lock record…

---

## Đồng bộ preference chọn trước-auth ngay trong request login (public endpoint)

User chọn preference (vd ngôn ngữ) TRƯỚC khi có token → gửi kèm field đó luôn trong body request login, BE tự update NGAY sau khi xác thực xong (trước khi build response) — response trả về đã phản ánh giá trị mới, FE không cần tự gọi thêm API PATCH sau khi có token (tránh race với điều hướng/redirect ngay sau login). Ref: `internal/modules/webapp/auth/core/handler/handler.go` (`Login` → `syncUserLocale` chạy trước `issueLoginSuccess`/`loadUserData`).
- Không cần mutex hay Redis lock cho trường hợp này vì MySQL UPDATE là atomic

## Engine generic dùng bởi N module — registry init() + tránh import cycle

Engine generic được nhiều module nghiệp vụ dùng chung (approval workflow, omnichat channel): module nghiệp vụ **`Register()` qua `init()`**, chiều import 1 chiều **module→engine** — engine KHÔNG import module, KHÔNG `switch doc_type`/`switch channel` trong engine. Const/type mà CẢ engine lẫn handler cùng cần (vd `doc_type` string) → đặt ở package **`dao/` leaf** (KHÔNG package root — root import handler nên đặt const ở root gây import cycle). Registry fail-fast: `panic` khi Register trùng key hoặc lookup miss key lạ. Precedent: `approval/subject` (Subject registry), `omnichat/channel` (adapter registry).

## Cảnh báo bắn trên đường LÀNH tệ hơn không có cảnh báo

Chỉ số/log chẩn đoán gây hiểu nhầm dạy người đọc bỏ qua nó — mất luôn giá trị lúc sự cố thật.

- Trước khi thêm 1 log chẩn đoán: liệt kê những đường **lành** cũng chạm điều kiện đó và loại trừ
  trước (vd `empty_message` bắn mỗi tin sticker, vì URL sticker được resolve ở tầng sau).
- Lỗi đã che phía user thì **bắt buộc** có log phía server — che cả hai đầu là biến sự cố thành vô hình.
- `zap.Error(err)` với error type không implement `MarshalLogObject` in ra `{"error": {}}`: log có
  dòng, không có nội dung. Kiểm bằng cách đọc log thật một lần, đừng tin là đã log.

## `RowsAffected == 0` KHÔNG có nghĩa "không tìm thấy row"

Rule gốc + ngoại lệ optimistic lock: `skills/backend-hard-rules/SKILL.md` §Data patterns. Triệu chứng khi debug: UPDATE chạy đúng, dữ liệu vào DB, nhưng handler vẫn trả lỗi "không tìm thấy" — kiểm chỗ suy ra kết luận từ `RowsAffected` trước khi nghi query sai.
