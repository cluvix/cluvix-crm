---
name: working-on-ops
description: >-
  Hướng dẫn nghiệp vụ dashboard CRM, export,
  và import dữ liệu trong module ops của backend Cluvix. Dùng skill này KHI
  đụng report/dashboard CRM, thêm cột export
  (`?columns=`), hoặc import khách hàng (migration
  onboard SAAS client) — đọc trước khi đổi
  bất kỳ hành vi nào trong các luồng này.
---

# Ops — Dashboard, Export, Import

> `reference/export.md` = cơ chế export list (`?_export=1`, colDef core/optional); (cron/scheduler đã bỏ 2026-10-17); `reference/import.md` = import migration onboard SAAS client một lần.



## Accountant — đã bỏ

CRM standalone đã xoá module `webapp/accountant/` (công nợ đơn hàng/điều trị/khách hàng/buổi điều trị, sổ cái, khoá kỳ — D2, xem `bmad/crm-standalone/architecture.md` §scope) và company (D15). Không còn báo cáo công nợ nào để sửa.

## CRM Dashboard (20 API)
Route prefix: `/dashboard/crm/`

### Tài chính
- `payment-in-month` — doanhSo + doanhThu + thucThu + hoanDV + hoanCoc + so sánh tháng trước
- `payment-branch-overview` — 7 metrics per branch (1 query, 7 LEFT JOIN subqueries)
- `marketing-view` — budget + thucThu online + doanhSo online + ROI

### Thống kê nhanh
- `quick-starts` — customer (month/today) + appointment (today/coming) + orderNew + serviceNew
- `additional-stats` — reminder + customerLead + customerCare + broker + coupon

### Lịch hẹn
- `compare-appointment` — Lịch hẹn theo ngày tháng này vs tháng trước
- `appointment-accepted-stats` — Trạng thái đồng ý hôm nay
- `overview-appointment` — lichHen/daDen/dongY/huy/conLai (unique customer, cached)
- `report-statistics` — Pie chart is_accepted tháng, top 5 + Khác

### Marketing & Funnel
- `overview-online` — Tỷ lệ chuyển đổi online (lead→lịch→đến→đồng ý)
- `estimate-price-marketing` — Chi phí per-action marketing (budget / count)
- `customer-status-marketing` — Funnel marketing online
- `status-customer-overview` — Funnel customer status (ticket→lead→hẹn→đến→đồng ý→hoàn thành)

### Charts
- `report-customer-by-region` — Top 8 tỉnh + Khu vực khác (cached 300s)
- `overview-service-type-accepted` — Top 3 nhóm DV đồng ý + Khác
- `overview-chart-source` — Top 5 nguồn KH + Nguồn khác
- `overview-chart-event` — Top 10 sự kiện + Sự kiện khác
- `overview-service-type-interest` — Top 10 nhóm DV quan tâm + Khác
- `service-used-statistics` — Top 10 DV (quantity + revenue) + Dịch vụ khác

## Lưu ý FE
- Tất cả route GET, response trả raw numbers → FE tự format VND
- ROI trả float (85.5) → FE thêm `%`
- `calcTrend`: 0=không đổi, 1=giảm, 2=tăng
- Chart data: `[{name, value}]` hoặc `[{name, value, itemStyle}]`
