-- Gỡ 3 công tắc `user_option` không còn dùng (seed ở 20260928094713_seed_lookup.up.sql):
--   21 "Tính số lần sử dụng dịch vụ theo tổng gói"        — không còn code đọc.
--   22 "Bỏ qua kiểm tra thanh toán khi sử dụng dịch vụ"   — không còn code đọc.
--   23 "Cho phép dùng dịch vụ đã mua ở chi nhánh khác"    — BE nay LUÔN cho dùng đơn hàng / dịch vụ
--      đã dùng / thanh toán của khách ở mọi chi nhánh (trước đây đọc qua tenant.IsCrossBranchServiceEnabled).
-- Không bảng nào có FK tới `user_option`. Idempotent: DELETE theo id, chạy lại không lỗi.
DELETE FROM `user_option` WHERE `id` IN (21, 22, 23);
