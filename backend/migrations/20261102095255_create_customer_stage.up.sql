-- Pipeline tiến trình khách hàng — PHA 1 (expand), epic customer-stage-pipeline story-01.
-- Tạo danh mục bước `customer_stage` (+ seed 8 bước theo khoá seed `name`), bảng lịch sử
-- `customer_stage_history`, cột `customer.stage_id` (NULL ở pha này) + backfill + index + FK,
-- và 1 dòng lịch sử khởi tạo cho mỗi khách hiện có. KHÔNG xoá gì: `customer.status`/`is_patient`/
-- `idx_customer_scope_status` giữ nguyên tới story-12 (pha contract) — code hiện tại vẫn đọc/ghi `status`.
-- Idempotent: CREATE TABLE IF NOT EXISTS, seed WHERE NOT EXISTS, ALTER qua information_schema + PREPARE.

-- 1. Danh mục bước. `type`: 1 đang mở / 2 thành công / 3 thất bại (define.CustomerStageType*).
--    `color`: khoá màu cố định (define.CustomerStageColors), không nhận hex.
--    `default_key` VIRTUAL + UNIQUE ép "đúng 1 bước mặc định" (NULL khi không mặc định nên nhiều dòng 0 hợp lệ);
--    VIRTUAL không STORED theo writing-mysql-migrations §Cột SINH; cột DB-managed, không map vào struct Go.
--    `created_by`/`updated_by`: chỉ KEY, CẤM FK (ngoại lệ cột audit — CLAUDE.md).
CREATE TABLE IF NOT EXISTS `customer_stage` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL COMMENT 'Tên bước',
  `type` tinyint NOT NULL COMMENT 'Loại bước: 1 đang mở, 2 thành công, 3 thất bại',
  `color` varchar(20) NOT NULL COMMENT 'Khoá màu trong bảng màu cố định',
  `sort_order` int NOT NULL DEFAULT '0' COMMENT 'Thứ tự trong pipeline',
  `is_default` tinyint NOT NULL DEFAULT '0' COMMENT '1 = bước mặc định cho khách mới',
  `default_key` tinyint GENERATED ALWAYS AS (if((`is_default` = 1),1,NULL)) VIRTUAL COMMENT 'Ép đúng 1 bước mặc định (UNIQUE)',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '1 hiện, 0 ẩn',
  `created_at` bigint DEFAULT NULL COMMENT 'Thời gian tạo mới',
  `updated_at` bigint DEFAULT NULL COMMENT 'Thời gian cập nhật gần đây nhất',
  `created_by` int DEFAULT NULL COMMENT 'Được tạo bởi ai',
  `updated_by` int DEFAULT NULL COMMENT 'Người cập nhật gần đây nhất',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_customer_stage_name` (`name`),
  UNIQUE KEY `uq_customer_stage_default` (`default_key`),
  KEY `idx_customer_stage_created_by` (`created_by`),
  KEY `idx_customer_stage_updated_by` (`updated_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed 8 bước theo khoá seed `name` (không id cố định). type 1/2/3 = define.CustomerStageTypeOpen/Won/Lost.
-- "Liên hệ" là bước mặc định — chỉ seed khi DB chưa có bước mặc định nào (khách đã tự đổi mặc định
-- thì lần chạy lại không đụng UNIQUE uq_customer_stage_default).
INSERT INTO `customer_stage` (`name`, `type`, `color`, `sort_order`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'Liên hệ', 1, 'gray', 1, 1, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP() FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `customer_stage` WHERE `name` = 'Liên hệ')
  AND NOT EXISTS (SELECT 1 FROM `customer_stage` WHERE `is_default` = 1);

INSERT INTO `customer_stage` (`name`, `type`, `color`, `sort_order`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'Đang tư vấn', 1, 'blue', 2, 0, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP() FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `customer_stage` WHERE `name` = 'Đang tư vấn');

INSERT INTO `customer_stage` (`name`, `type`, `color`, `sort_order`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'Đang đàm phán', 1, 'amber', 3, 0, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP() FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `customer_stage` WHERE `name` = 'Đang đàm phán');

INSERT INTO `customer_stage` (`name`, `type`, `color`, `sort_order`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'Đã lên đơn', 1, 'purple', 4, 0, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP() FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `customer_stage` WHERE `name` = 'Đã lên đơn');

INSERT INTO `customer_stage` (`name`, `type`, `color`, `sort_order`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'Khách hàng đã thuê', 2, 'green', 5, 0, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP() FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `customer_stage` WHERE `name` = 'Khách hàng đã thuê');

INSERT INTO `customer_stage` (`name`, `type`, `color`, `sort_order`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'Từ chối', 3, 'red', 6, 0, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP() FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `customer_stage` WHERE `name` = 'Từ chối');

INSERT INTO `customer_stage` (`name`, `type`, `color`, `sort_order`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'Dừng chăm sóc', 3, 'orange', 7, 0, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP() FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `customer_stage` WHERE `name` = 'Dừng chăm sóc');

INSERT INTO `customer_stage` (`name`, `type`, `color`, `sort_order`, `is_default`, `status`, `created_at`, `updated_at`)
SELECT 'Ngừng thuê', 3, 'teal', 8, 0, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP() FROM DUAL
WHERE NOT EXISTS (SELECT 1 FROM `customer_stage` WHERE `name` = 'Ngừng thuê');

-- 2. Lịch sử đổi bước. Không có branch_id (scope qua khách cha).
--    customer_id CASCADE: lịch sử đi theo khách, không chặn xoá khách.
--    from/to_stage_id RESTRICT: bước từng dùng không xoá được, chỉ ẩn.
--    changed_by SET NULL: quan hệ thật tới user (không phải cột audit created_by/updated_by).
--    source: 1 tạo / 2 import / 3 đổi tay / 4 migration (define.CustomerStageHistorySource*).
CREATE TABLE IF NOT EXISTS `customer_stage_history` (
  `id` int NOT NULL AUTO_INCREMENT,
  `customer_id` int NOT NULL COMMENT 'Khách hàng',
  `from_stage_id` int DEFAULT NULL COMMENT 'Bước trước (NULL = khởi tạo)',
  `to_stage_id` int NOT NULL COMMENT 'Bước mới',
  `source` tinyint NOT NULL COMMENT 'Nguồn: 1 tạo, 2 import, 3 đổi tay, 4 migration',
  `changed_by` int DEFAULT NULL COMMENT 'Người đổi (NULL = hệ thống)',
  `changed_at` bigint NOT NULL COMMENT 'Thời điểm đổi (Unix)',
  PRIMARY KEY (`id`),
  KEY `idx_csh_customer_changed` (`customer_id`,`changed_at`),
  KEY `idx_csh_to_stage_customer` (`to_stage_id`,`customer_id`),
  KEY `idx_csh_from_stage_id` (`from_stage_id`),
  KEY `idx_csh_changed_by` (`changed_by`),
  CONSTRAINT `fk-customer_stage_history-customer_id` FOREIGN KEY (`customer_id`) REFERENCES `customer` (`id`) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT `fk-customer_stage_history-from_stage_id` FOREIGN KEY (`from_stage_id`) REFERENCES `customer_stage` (`id`) ON DELETE RESTRICT ON UPDATE CASCADE,
  CONSTRAINT `fk-customer_stage_history-to_stage_id` FOREIGN KEY (`to_stage_id`) REFERENCES `customer_stage` (`id`) ON DELETE RESTRICT ON UPDATE CASCADE,
  CONSTRAINT `fk-customer_stage_history-changed_by` FOREIGN KEY (`changed_by`) REFERENCES `user` (`id`) ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 3. customer.stage_id — pha 1 để NULL (NOT NULL ở story-12).
SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer' AND COLUMN_NAME = 'stage_id') = 0,
  'ALTER TABLE `customer` ADD COLUMN `stage_id` int NULL COMMENT ''Bước tiến trình (customer_stage)'' AFTER `status`',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- Backfill: mọi khách chưa có bước ⇒ bước mặc định.
UPDATE `customer`
SET `stage_id` = (SELECT `id` FROM `customer_stage` WHERE `is_default` = 1)
WHERE `stage_id` IS NULL;

SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer' AND INDEX_NAME = 'idx_customer_stage_branch') = 0,
  'ALTER TABLE `customer` ADD KEY `idx_customer_stage_branch` (`stage_id`, `branch_id`)',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- FK: cột vừa tạo + backfill bằng id có thật ⇒ dữ liệu sạch, tắt FOREIGN_KEY_CHECKS riêng cho lệnh ADD
-- để chạy INPLACE (không khoá ghi cả bảng) — writing-mysql-migrations §FK.
SET @old_fkc = @@SESSION.FOREIGN_KEY_CHECKS;
SET SESSION FOREIGN_KEY_CHECKS = 0;
SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer'
      AND CONSTRAINT_NAME = 'fk-customer-stage_id' AND CONSTRAINT_TYPE = 'FOREIGN KEY') = 0,
  'ALTER TABLE `customer` ADD CONSTRAINT `fk-customer-stage_id` FOREIGN KEY (`stage_id`) REFERENCES `customer_stage` (`id`) ON DELETE RESTRICT ON UPDATE CASCADE',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
SET SESSION FOREIGN_KEY_CHECKS = @old_fkc;

-- 4. Lịch sử khởi tạo cho khách hiện có (source 4 = define.CustomerStageHistorySourceMigration),
--    chỉ khách chưa có dòng lịch sử nào ⇒ chạy lại không nhân đôi.
INSERT INTO `customer_stage_history` (`customer_id`, `from_stage_id`, `to_stage_id`, `source`, `changed_by`, `changed_at`)
SELECT c.`id`, NULL, c.`stage_id`, 4, NULL, UNIX_TIMESTAMP()
FROM `customer` c
WHERE c.`stage_id` IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM `customer_stage_history` h WHERE h.`customer_id` = c.`id`);
