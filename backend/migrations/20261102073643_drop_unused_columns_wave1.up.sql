-- Đợt 1 nhóm A (db-column-sync): gỡ bảng/cột thừa — không code BE/FE nào đọc hoặc ghi
-- (bằng chứng: bmad/db-column-sync/evidence/).
--
--   * system_log                : DROP cả bảng (entity chỉ có dao, 0 importer).
--   * location_country          : CountryType, CountrySubType, Sovereignty, CountryCode3, CountryNumber,
--                                 InternetCountryCode, Flags, IsDeleted, slug, language
--   * location_province         : ZipCode, IsDeleted, slug, language
--   * location_ward             : nameWithType, pathWithType, slug
--   * unit                      : type
--   * control_field_in_table    : add_on
--   * appointment               : old_code, sale_start, sale_end   (KHÔNG đụng customer.old_code)
--   * user                      : logged_at
--   * service                   : installment_config               (KHÔNG đụng order_service_used.installment_config)
--
-- Đã kiểm FK / index / CHECK / generated column: chỉ có index 1 cột `index-slug`, `index-language`
-- trên các bảng location_* — MySQL tự bỏ theo cột. Không có FK, index nhiều cột, CHECK hay cột sinh
-- (generated) nào chứa các cột này. Không có FULLTEXT ở các bảng này.
-- KHÔNG gỡ trong đợt này: attachment.sort_order (entity nằm ở crm/customer/dao đang được phiên khác sửa).
--
-- Idempotent: mỗi bảng gom các cột còn tồn tại (INFORMATION_SCHEMA.COLUMNS) vào MỘT câu ALTER TABLE ... DROP COLUMN a, b, ...
-- (MySQL không có DROP COLUMN IF EXISTS). Gom thành một ALTER vì MySQL >= 8.0.29 mỗi DROP COLUMN INSTANT tốn 1 row
-- version (tối đa 64 mỗi bảng) — location_country từng 10 ALTER riêng sẽ ngốn 10 version thay vì 1.
-- Không có cột nào còn tồn tại => @drops NULL => chạy 'SELECT 1' (no-op).
-- Chỉ chạy trên DB dùng-một-lần để verify trước; KHÔNG chạy tay trên DB dev/production.

DROP TABLE IF EXISTS system_log;

-- location_country
SET @drops = NULL;
SELECT GROUP_CONCAT(CONCAT('DROP COLUMN `', COLUMN_NAME, '`') ORDER BY ORDINAL_POSITION SEPARATOR ', ') INTO @drops
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'location_country'
    AND COLUMN_NAME IN ('CountryType', 'CountrySubType', 'Sovereignty', 'CountryCode3', 'CountryNumber', 'InternetCountryCode', 'Flags', 'IsDeleted', 'slug', 'language');
SET @sql = IF(@drops IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `location_country` ', @drops));
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- location_province
SET @drops = NULL;
SELECT GROUP_CONCAT(CONCAT('DROP COLUMN `', COLUMN_NAME, '`') ORDER BY ORDINAL_POSITION SEPARATOR ', ') INTO @drops
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'location_province'
    AND COLUMN_NAME IN ('ZipCode', 'IsDeleted', 'slug', 'language');
SET @sql = IF(@drops IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `location_province` ', @drops));
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- location_ward
SET @drops = NULL;
SELECT GROUP_CONCAT(CONCAT('DROP COLUMN `', COLUMN_NAME, '`') ORDER BY ORDINAL_POSITION SEPARATOR ', ') INTO @drops
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'location_ward'
    AND COLUMN_NAME IN ('nameWithType', 'pathWithType', 'slug');
SET @sql = IF(@drops IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `location_ward` ', @drops));
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- unit
SET @drops = NULL;
SELECT GROUP_CONCAT(CONCAT('DROP COLUMN `', COLUMN_NAME, '`') ORDER BY ORDINAL_POSITION SEPARATOR ', ') INTO @drops
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'unit'
    AND COLUMN_NAME IN ('type');
SET @sql = IF(@drops IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `unit` ', @drops));
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- control_field_in_table
SET @drops = NULL;
SELECT GROUP_CONCAT(CONCAT('DROP COLUMN `', COLUMN_NAME, '`') ORDER BY ORDINAL_POSITION SEPARATOR ', ') INTO @drops
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'control_field_in_table'
    AND COLUMN_NAME IN ('add_on');
SET @sql = IF(@drops IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `control_field_in_table` ', @drops));
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- appointment
SET @drops = NULL;
SELECT GROUP_CONCAT(CONCAT('DROP COLUMN `', COLUMN_NAME, '`') ORDER BY ORDINAL_POSITION SEPARATOR ', ') INTO @drops
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'appointment'
    AND COLUMN_NAME IN ('old_code', 'sale_start', 'sale_end');
SET @sql = IF(@drops IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `appointment` ', @drops));
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- user
SET @drops = NULL;
SELECT GROUP_CONCAT(CONCAT('DROP COLUMN `', COLUMN_NAME, '`') ORDER BY ORDINAL_POSITION SEPARATOR ', ') INTO @drops
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user'
    AND COLUMN_NAME IN ('logged_at');
SET @sql = IF(@drops IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `user` ', @drops));
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- service
SET @drops = NULL;
SELECT GROUP_CONCAT(CONCAT('DROP COLUMN `', COLUMN_NAME, '`') ORDER BY ORDINAL_POSITION SEPARATOR ', ') INTO @drops
  FROM INFORMATION_SCHEMA.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'service'
    AND COLUMN_NAME IN ('installment_config');
SET @sql = IF(@drops IS NULL, 'SELECT 1', CONCAT('ALTER TABLE `service` ', @drops));
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
