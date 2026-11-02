-- Gỡ hẳn tính năng HẠNG THÀNH VIÊN (giảm giá theo hạng) — sản phẩm đã bỏ, code BE/FE gỡ cùng đợt:
--   1. Bảng customer_rank_services (giảm % theo hạng × dịch vụ) và customer_ranks (định nghĩa hạng, kèm
--      min_points/max_points — dấu vết cuối cùng của điểm tích luỹ trên DB).
--   2. Cột customer.customer_rank_id (+ FK `fk-customer-customer_rank_id` → customer_ranks).
--   3. Cột order_service_used.rank_discount_percent / rank_discount_amount. Số tiền giảm theo hạng CHỈ được
--      lưu, KHÔNG trừ vào totalEnd/phải thu ⇒ bỏ cột không đổi công nợ hay tiền của đơn cũ.
--
-- Đã kiểm (information_schema, 2026-11-01): ngoài 2 FK nêu trên, không view, trigger, routine, CHECK hay
-- generated column nào tham chiếu các cột/bảng này. Index `fk-customer-customer_rank_id` chỉ có 1 cột ⇒
-- MySQL tự bỏ khi DROP COLUMN; FK thì PHẢI gỡ trước (lỗi 1828).
-- Idempotent: tên FK tra động qua information_schema, mỗi DROP COLUMN chỉ chạy khi cột còn tồn tại,
-- DROP TABLE IF EXISTS.
--
-- ⚠ customer có FULLTEXT `ft_customer_ngram` ⇒ DROP COLUMN chỉ còn thuật toán COPY = dựng lại cả bảng
-- (chặn ghi customer trong lúc chép):
--   - FOREIGN_KEY_CHECKS = 0 trong lúc chép: COPY với FK bật kiểm lại FK của MỌI dòng cũ ⇒ một dòng mồ côi
--     sẵn có (lỗi 1452) là migration dirty, chặn deploy. Bỏ một cột không đụng dữ liệu FK nên tắt kiểm an toàn.
--   - innodb_ft_enable_stopword = OFF (SESSION, cùng kết nối với ALTER): chỉ mục ngram được DỰNG LẠI theo
--     biến này; để ON thì danh sách stopword mặc định làm MATCH sót âm thầm — xem skill
--     debugging-backend-queries §"Chỉ mục ngram dựng sai stopword".
-- Hai biến được trả lại giá trị cũ cuối file.

SET @old_fkc = @@SESSION.FOREIGN_KEY_CHECKS;
SET @old_ft_stopword = @@SESSION.innodb_ft_enable_stopword;
SET SESSION FOREIGN_KEY_CHECKS = 0;
SET SESSION innodb_ft_enable_stopword = OFF;

-- 1. customer.customer_rank_id: gỡ FK (tên tra động) rồi bỏ cột.
SET @fk = (SELECT CONSTRAINT_NAME FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer' AND COLUMN_NAME = 'customer_rank_id'
    AND REFERENCED_TABLE_NAME IS NOT NULL
  LIMIT 1);
SET @sql = IF(@fk IS NOT NULL, CONCAT('ALTER TABLE customer DROP FOREIGN KEY `', @fk, '`'), 'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer' AND COLUMN_NAME = 'customer_rank_id') > 0,
  'ALTER TABLE customer DROP COLUMN customer_rank_id',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 2. order_service_used: 2 cột giảm theo hạng (không index/FK).
SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order_service_used' AND COLUMN_NAME = 'rank_discount_percent') > 0,
  'ALTER TABLE order_service_used DROP COLUMN rank_discount_percent',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order_service_used' AND COLUMN_NAME = 'rank_discount_amount') > 0,
  'ALTER TABLE order_service_used DROP COLUMN rank_discount_amount',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 3. Hai bảng hạng: bảng con (FK rank_id → customer_ranks) trước.
DROP TABLE IF EXISTS customer_rank_services;
DROP TABLE IF EXISTS customer_ranks;

SET SESSION innodb_ft_enable_stopword = @old_ft_stopword;
SET SESSION FOREIGN_KEY_CHECKS = @old_fkc;
