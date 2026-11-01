-- Bỏ hẳn 2 trường của đơn hàng (sản phẩm đã gỡ, production chưa có dữ liệu dùng):
--   1. Giảm giá dòng: order_service_used.discount (phần coupon chia xuống dòng) + tổng ở đơn order_plan.discount.
--      Công thức tiền dòng từ nay: total = đơn giá × số lượng − chiết khấu dịch vụ;
--      totalEnd = total − giảm khác (discount_other) − giảm trừ khoá dịch vụ (discountServiceLock).
--   2. Hiệu lực: order_service_used.expires_at.
-- GIỮ NGUYÊN: service_discount/service_discount_type, discount_other (dòng + đơn), discountServiceLock,
-- rank_discount_percent/rank_discount_amount.
--
-- Đã kiểm (information_schema, 2026-11-01): 3 cột KHÔNG có index, FK, CHECK, view, trigger, routine
-- hay generated column nào tham chiếu ⇒ DROP COLUMN thẳng, không phải gỡ ràng buộc trước.
-- Idempotent: mỗi lệnh chỉ chạy khi cột còn tồn tại (MySQL không có DROP COLUMN IF EXISTS).
--
-- ⚠ order_plan có FULLTEXT `ft_otp_note_ngram` ⇒ MySQL KHÔNG cho DROP COLUMN kiểu INSTANT/INPLACE
-- (ERROR 1846), chỉ còn COPY = dựng lại cả bảng (chặn ghi order_plan trong lúc chép):
--   - FOREIGN_KEY_CHECKS = 0 trong lúc chép: COPY với FK bật kiểm lại FK của MỌI dòng cũ ⇒ một dòng mồ côi
--     sẵn có (lỗi 1452) là migration dirty, chặn deploy. Bỏ một cột không đụng dữ liệu FK nên tắt kiểm an toàn.
--   - innodb_ft_enable_stopword = OFF (SESSION, cùng kết nối với ALTER): chỉ mục ngram được DỰNG LẠI theo
--     biến này; để ON thì danh sách stopword mặc định (có `a`, `i`) làm MATCH sót âm thầm ("giao", "khám"…)
--     — xem skill debugging-backend-queries §"Chỉ mục ngram dựng sai stopword".
-- Hai biến được trả lại giá trị cũ cuối file.

SET @old_fkc = @@SESSION.FOREIGN_KEY_CHECKS;
SET @old_ft_stopword = @@SESSION.innodb_ft_enable_stopword;
SET SESSION FOREIGN_KEY_CHECKS = 0;
SET SESSION innodb_ft_enable_stopword = OFF;

SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order_service_used' AND COLUMN_NAME = 'discount') > 0,
  'ALTER TABLE order_service_used DROP COLUMN discount',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order_service_used' AND COLUMN_NAME = 'expires_at') > 0,
  'ALTER TABLE order_service_used DROP COLUMN expires_at',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @sql = IF(
  (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'order_plan' AND COLUMN_NAME = 'discount') > 0,
  'ALTER TABLE order_plan DROP COLUMN discount',
  'SELECT 1');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET SESSION innodb_ft_enable_stopword = @old_ft_stopword;
SET SESSION FOREIGN_KEY_CHECKS = @old_fkc;
