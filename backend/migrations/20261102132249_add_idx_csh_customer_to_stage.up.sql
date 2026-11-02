-- Epic customer-stage-pipeline story-07 — chỉ mục phục vụ phễu dashboard
-- (`GET /api/crm/crm-dashboard/status-customer-overview`, handler CrmStatusCustomerOverview).
-- Phễu chạy 1 subquery tương quan theo khách: MAX(FIELD(h.to_stage_id, …)) WHERE h.customer_id = c.id.
-- (customer_id, to_stage_id) cho câu đó đọc thuần chỉ mục (covering); 2 chỉ mục của story-01 không phủ:
--   (customer_id, changed_at) phải tra dòng lấy to_stage_id; (to_stage_id, customer_id) buộc dò 1 lần/bước.
-- /check-scale 2026-11-02 (DB *_tmp từ migration, 600.000 khách / 5 chi nhánh, 1.575.019 dòng lịch sử,
-- created_day rải 12 tháng, buffer pool 128MB, EXPLAIN ANALYZE ấm):
--   kỳ 12 tháng toàn chi nhánh: không chỉ mục này 8.498ms → có 1.520ms (FORCE INDEX 1.521ms = optimizer tự chọn);
--   kỳ 1 tháng toàn chi nhánh 700 → 127ms; 1 chi nhánh 12 tháng 842 → 513ms.
-- Chỉ THÊM (không xoá/đổi chỉ mục nào). Idempotent: kiểm theo tên qua information_schema + PREPARE.
SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer_stage_history'
      AND INDEX_NAME = 'idx_csh_customer_to_stage') = 0,
  'ALTER TABLE `customer_stage_history` ADD KEY `idx_csh_customer_to_stage` (`customer_id`, `to_stage_id`)',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
