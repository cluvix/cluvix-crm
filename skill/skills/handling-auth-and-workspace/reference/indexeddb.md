# IndexedDB Master Data Sync — Phương án B (per-table version) — phía BE

## Mục lục

- Goal
- Architecture
- BE API
  - Bump (gọi sau mỗi CRUD)
  - Storage
  - Sentinel `__all__`
- Contract — table names (khớp FE `table-registry.ts`)
- Race safety (BE)
- Performance
- Adding new master data table (BE steps)
- Status filter — quy tắc bất biến
- Files chính (BE)

---


Master data cấu hình (companies, branches, units, services, ...) được cache ở FE qua IndexedDB. BE thông báo thay đổi qua header → FE chỉ sync table cần thiết, **không full sync**.

> **Phía FE đã tách theo repo frontend (2026-07-16):** `frontend/skill/skills/working-on-fe-modules/reference/fe-indexeddb.md` — sync layers, table-registry, bump `INDEXEDDB_DB_VERSION`, cache service status filter, race safety FE, UserHelperService. Mobile (drift) mirror cơ chế này: `mobile/skill/skills/building-flutter-features/SKILL.md §Master-data sync engine`.

## Goal

- Cross-device real-time: admin sửa unit → mọi client làm việc, API call tiếp theo nhận biết → chỉ refresh `units` table
- Minimal bandwidth: 1 sync = 1 table, không tải lại 30 tables như cơ chế cũ
- Race-safe khi nhiều CRUD đồng thời

## Architecture

```
BE CRUD → BumpDexieTable(db, "tableName")
        → atomic JSON_MERGE_PATCH single UPDATE
        → invalidate Redis caches (config:dexie_versions, config:versions)

BE response middleware
        → getVersions (cached 5min Redis)
        → header X-Dexie-Versions: "companies=1745,branches=1746,units=1750,..."

FE/mobile interceptor → so version local → lệch → GET /api/crm/indexeddb/sync?tables=a,b
        (chi tiết phía client: doc FE/mobile ở trên)
```

## BE API

### Bump (gọi sau mỗi CRUD)

```go
import "erpCluvix/pkg/dexie"

// Single table
dexie.BumpDexieTable(h.DB, "companies")

// Multi-table (bulk operation)
dexie.BumpDexieTable(h.DB, "services", "service_combos")

// Generic handler không biết table cụ thể (toggle, settings, generic CRUD)
dexie.BumpDexieTable(h.DB, "__all__")
```

**Quan trọng:**
- `tableName` phải khớp **chính xác** FE table registry (xem section dưới)
- Sai tên → FE invalidate sai table → silent fail, dữ liệu stale
- Atomic qua `JSON_MERGE_PATCH` → race-safe khi 2 CRUD đồng thời

### Storage

- `config_systems.content.dexieVersions` JSON: `{tableName: unixTimestamp}`
- Cache Redis: `config:dexie_versions` (TTL 5min), invalidate khi bump
- Cache Redis: `config:versions` (middleware struct, TTL 5min)

### Sentinel `__all__`

Dùng cho 6 handler không có FE table 1-1:
- `show_hide.go` (toggle generic)
- `setting.go` (settings global)
- `bank_account.go` / `card_type.go` / `supplier-template.go` (no FE table)

FE sẽ full sync khi thấy mismatch sentinel này.

## Contract — table names (khớp FE `table-registry.ts`)

```
companies, branches, units, services, serviceTypes, serviceCombos, serviceComboTypes,
users, departments, ethnicity, job,
customerCategories, customerMedicalHistories, customerPathologies,
customerCareTypes, customerCareChannels, customerCareClassifications,
customerCareResults, customerCareStatus, customerCareComplaintsTypes,
appointmentTypes, appointmentReasons, appointmentNotBookedReasons, appointmentIsAccepteds,
campaigns, sources, groupSources, groupLocals, groupCountries,
locationCountries, locationProvinces,
paymentMethods, paymentTypes, couponCatagories,
brokerCatagories, brokerTypes, pageFacebooks,
sms_templates, smsTypes, noteTemplates, macros, categoryFaqs, listCallCenters,
warehouses, suppliers, supplierLaboStatus, stockUsageReasons, material, materialTypes,
warrantyCardTypes, statusWarrantyCards,
medManufacturer, medMedicines, medInstructions, medPrescriptions,
diagnostics, treatmentCategories, treatmentCourseTypes,
controlFieldInTables, userOptions, aiProvider, aiConfig
```

## Race safety (BE)

- `JSON_MERGE_PATCH` atomic — 2 CRUD đồng thời khác table không lost update
- Redis cache invalidate sau bump

## Performance

| Operation | Cost |
|-----------|------|
| BE bump 1 table | 1 atomic SQL UPDATE |
| BE getVersions | Redis hit ~0.1ms, miss → 1 SELECT |
| Header size | ~750 bytes (~30 tables) |
| Cross-device latency | Next API call (vài giây) |
| Single-table sync | 1 HTTP `/sync?tables=name` |
| Full sync | 1 HTTP `/sync` toàn bộ master data |

## Adding new master data table (BE steps)

1. Tạo handler CRUD (List, Get, Create, Update, Delete)
2. Sau mỗi mutation: `dexie.BumpDexieTable(h.DB, "tableName")`
3. Add table vào `configTables` map của `internal/modules/webapp/config/indexeddb/handler/sync.go` — endpoint `/api/crm/indexeddb/sync` query `?tables=...` chọn cụ thể
4. Nếu table có quy ước `branch_id NULL = áp dụng toàn company` (như `emr_department`) → set `NullableBranch: true` trong `tableConfig` để FE sync nhận đủ row

FE steps (table-registry + **bump `INDEXEDDB_DB_VERSION`** + cache service + form control): `frontend/skill/skills/working-on-fe-modules/reference/fe-indexeddb.md`.

## Status filter — quy tắc bất biến

| Endpoint | Mục đích | Status filter |
|---|---|---|
| **Selector/lookup API** (`load-*`, `get-all-*`, `*-by-keyword`) | FE form chọn dữ liệu khi tạo mới | **Bắt buộc `WHERE status = 1`** (hoặc `status = 10` cho user) |
| **IndexedDB sync** (`/sync?tables=...`) | Cache toàn bộ master data để hiển thị label | **KHÔNG filter status** — phải sync cả record đã disable |

**Lý do:** đơn hàng cũ chọn dịch vụ A (active), admin disable A → đơn vẫn phải hiển thị đúng tên A (FE tra IndexedDB có đủ mọi record). Sync filter `status = 1` → list hiển thị trắng/thiếu tên.

**Không được thêm `WHERE status = 1` vào `configTables` hay `customTables` trong `sync.go`.**

## Files chính (BE)

- `pkg/dexie/` — `BumpDexieTable`, `GetDexieVersions`, `SerializeDexieVersionsHeader`, `ConfigSystems{Get,Set}Option`. Cache set qua `dexie.SetCache(c)` ở bootstrap.
- `internal/middleware/response_headers.go` — setVersionHeaders, getVersions
- `internal/modules/webapp/config/indexeddb/handler/sync.go` — endpoint `/api/crm/indexeddb/sync` (mount qua `crm/router.go` để URL giữ prefix `/crm/...`)
- `internal/router/router.go` — CORS expose X-Dexie-Versions
