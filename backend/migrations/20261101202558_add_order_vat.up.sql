-- VAT cho đơn hàng (bmad/order-vat — phương án B, story S1):
--   1. order_plan.vat_rate: mức VAT chọn cho CẢ đơn — 'KCT' (không chịu thuế), '0', '5', '8', '10' (%).
--      NULL = đơn trước khi có VAT / chưa chọn ⇒ coi như không VAT.
--   2. order_service_used.vat_amount: tiền VAT của dòng = round(totalEnd × mức / 100), server tính.
--      totalEnd GIỮ là tiền TRƯỚC thuế; số phải thu của dòng = totalEnd + vat_amount.
-- Dòng/đơn cũ: vat_amount = 0, vat_rate = NULL ⇒ mọi số liệu tiền giữ nguyên.
--
-- Idempotent: mỗi ADD COLUMN chỉ chạy khi cột chưa có (MySQL 8 không có ADD COLUMN IF NOT EXISTS).
--
-- ⚠ order_plan có FULLTEXT `ft_otp_note_ngram` ⇒ ADD COLUMN không chạy INSTANT, có thể phải dựng lại bảng
-- (xem migration 20261101180841): tắt FOREIGN_KEY_CHECKS trong lúc chép (thêm cột không đụng dữ liệu FK)
-- và innodb_ft_enable_stopword = OFF để chỉ mục ngram dựng lại không nuốt stopword. Trả giá trị cũ cuối file.

SET @old_fkc = @@SESSION.FOREIGN_KEY_CHECKS;
SET @old_ft_stopword = @@SESSION.innodb_ft_enable_stopword;
SET SESSION FOREIGN_KEY_CHECKS = 0;
SET SESSION innodb_ft_enable_stopword = OFF;

SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order_plan' AND COLUMN_NAME = 'vat_rate') = 0,
  'ALTER TABLE order_plan ADD COLUMN vat_rate VARCHAR(4) NULL DEFAULT NULL COMMENT ''Mức VAT của đơn: KCT, 0, 5, 8, 10 (%). NULL = không VAT''',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order_service_used' AND COLUMN_NAME = 'vat_amount') = 0,
  'ALTER TABLE order_service_used ADD COLUMN vat_amount DECIMAL(20,2) NOT NULL DEFAULT 0.00 COMMENT ''Tiền VAT của dòng = round(totalEnd × vat_rate của đơn / 100); phải thu = totalEnd + vat_amount''',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET SESSION innodb_ft_enable_stopword = @old_ft_stopword;
SET SESSION FOREIGN_KEY_CHECKS = @old_fkc;
