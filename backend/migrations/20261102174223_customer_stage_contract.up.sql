-- Pipeline tiến trình khách hàng — PHA 2 (contract), epic customer-stage-pipeline story-12.
-- Pha 1 (`20261102095255_create_customer_stage`) đã thêm `customer.stage_id` (NULL) + lịch sử; code nay
-- KHÔNG còn đọc/ghi `customer.status`/`is_patient` ⇒ khoá lại:
--   1. Backfill `stage_id` NULL ⇒ bước mặc định (khách tạo trong lúc chuyển tiếp bởi đường chưa gán bước).
--   2. Cổng chặn: còn khách `stage_id` NULL (vd DB không có bước mặc định) hoặc `stage_id` trỏ bước không
--      tồn tại ⇒ DỪNG bằng bảng-không-tồn-tại (writing-mysql-migrations §Rẽ nhánh có điều kiện — SIGNAL
--      không chạy được trong PREPARE). Đặt TRƯỚC mọi thay đổi schema ⇒ abort không để lại schema dở dang.
--   3. Lịch sử khởi tạo cho khách chưa có dòng nào (source 4 = define.CustomerStageHistorySourceMigration).
--   4. `stage_id` NOT NULL (FK `fk-customer-stage_id` + chỉ mục `idx_customer_stage_branch` giữ nguyên).
--   5. DROP INDEX `idx_customer_scope_status`, `idx-customer-is_patient` (FK `stage_id` dựa vào
--      `idx_customer_stage_branch` của pha 1 ⇒ không vướng Error 1553).
--   6. DROP COLUMN `status`, `is_patient` — gom 1 ALTER (mỗi DROP COLUMN INSTANT tốn 1 row version).
--      Chỉ mục khác có chứa `status` (DB nào có) được MySQL tự rút cột theo.
--   7. Bump Dexie `customerStages` để máy đã đăng nhập trước deploy tải lại danh mục bước.
-- ⛔ KHÔNG khôi phục được tiến trình cũ sau khi chạy (cột `status` mất). KHÔNG deploy lên instance có dữ liệu
-- khách thật khi user chưa xác nhận (epics.md HARD STOP).
-- Idempotent: mọi ALTER qua information_schema + PREPARE/EXECUTE; backfill/lịch sử chỉ chạm dòng còn thiếu.

-- 1. Backfill bước mặc định.
UPDATE `customer`
SET `stage_id` = (SELECT `id` FROM `customer_stage` WHERE `is_default` = 1)
WHERE `stage_id` IS NULL;

-- 2. Cổng chặn — tên bảng là thông báo (≤64 ký tự).
SET @sql = IF(
  (SELECT COUNT(*) FROM `customer` WHERE `stage_id` IS NULL) > 0,
  'SELECT 1 FROM `MIGRATION_ABORT_customer_stage_id_null`',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- Cổng orphan: stage_id trỏ bước không tồn tại ⇒ DỪNG. Cần vì MySQL 9.7.1 KHÔNG kiểm
-- `fk-customer-stage_id` khi INSERT/UPDATE khách có branch_id NULL (chỉ mục đỡ FK là
-- `idx_customer_stage_branch (stage_id, branch_id)` — đo 02/11/2026), và bước 4 tắt kiểm FK.
SET @sql = IF(
  (SELECT COUNT(*) FROM `customer` c
    WHERE NOT EXISTS (SELECT 1 FROM `customer_stage` s WHERE s.`id` = c.`stage_id`)) > 0,
  'SELECT 1 FROM `MIGRATION_ABORT_customer_stage_id_orphan`',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 3. Lịch sử khởi tạo cho khách chưa có (cùng câu bước 4 của pha 1).
INSERT INTO `customer_stage_history` (`customer_id`, `from_stage_id`, `to_stage_id`, `source`, `changed_by`, `changed_at`)
SELECT c.`id`, NULL, c.`stage_id`, 4, NULL, UNIX_TIMESTAMP()
FROM `customer` c
WHERE c.`stage_id` IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM `customer_stage_history` h WHERE h.`customer_id` = c.`id`);

-- 4. stage_id NOT NULL. MySQL từ chối đổi NULL→NOT NULL trên cột có FK khi FOREIGN_KEY_CHECKS = 1
--    (ERROR 1832, đo trên 9.7.1) ⇒ tắt riêng cho lệnh MODIFY (khuôn bước FK của pha 1). Tắt kiểm FK thì
--    MySQL không soát dữ liệu cũ ⇒ đã có cổng orphan ở bước 2.
SET @old_fkc = @@SESSION.FOREIGN_KEY_CHECKS;
SET SESSION FOREIGN_KEY_CHECKS = 0;
SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer' AND COLUMN_NAME = 'stage_id' AND IS_NULLABLE = 'YES') > 0,
  'ALTER TABLE `customer` MODIFY COLUMN `stage_id` int NOT NULL COMMENT ''Bước tiến trình (customer_stage)''',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET SESSION FOREIGN_KEY_CHECKS = @old_fkc;

-- 5. Chỉ mục cũ (tên thật: migrations/20260928093229_create_customer.up.sql).
SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer' AND INDEX_NAME = 'idx_customer_scope_status') > 0,
  'ALTER TABLE `customer` DROP INDEX `idx_customer_scope_status`',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer' AND INDEX_NAME = 'idx-customer-is_patient') > 0,
  'ALTER TABLE `customer` DROP INDEX `idx-customer-is_patient`',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 6. Cột cũ — gom các cột còn tồn tại vào MỘT ALTER; không còn cột nào ⇒ @drops NULL ⇒ no-op.
SET @drops = NULL;
SELECT GROUP_CONCAT(CONCAT('DROP COLUMN `', COLUMN_NAME, '`') ORDER BY ORDINAL_POSITION SEPARATOR ', ') INTO @drops
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer'
    AND COLUMN_NAME IN ('status', 'is_patient');
SET @sql = IF(@drops IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `customer` ', @drops));
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 7. Bump Dexie `customerStages` (khuôn 20261102073750_bump_dexie_users; cùng cách ghi với
--    pkg/dexie.BumpDexieTable). Chạy lại chỉ bump lại timestamp ⇒ FE tải lại thêm 1 lần, vô hại.
UPDATE `config_systems` SET `content` = JSON_MERGE_PATCH(
	COALESCE(`content`, '{}'),
	JSON_OBJECT('dexieVersions', JSON_OBJECT('customerStages', UNIX_TIMESTAMP()))
);
