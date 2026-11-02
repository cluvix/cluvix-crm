-- Bump phiên bản IndexedDB của bảng `users` (2026-11-02).
--
-- Vì sao: đồng bộ "users" (`config/indexeddb/handler/sync.go`) và `GetAllUsers` thôi loại `u.id = 1`
-- (tài khoản `admin` do seed tạo) để chọn được admin làm người liên quan / tư vấn viên… Đổi câu query
-- KHÔNG đổi `dexieVersions.users` ⇒ trình duyệt đã đồng bộ trước đó giữ danh sách cũ (thiếu admin) tới
-- khi có người lưu một nhân viên. Bump tại đây để mọi máy tải lại bảng `users` sau deploy.
-- Cùng cách ghi với `pkg/dexie.BumpDexieTable`; cache Redis `config:dexie_versions` hết hạn sau 5 phút.
--
-- Idempotent: chạy lại chỉ bump lại timestamp ⇒ FE tải lại `users` thêm 1 lần, vô hại.
UPDATE `config_systems` SET `content` = JSON_MERGE_PATCH(
	COALESCE(`content`, '{}'),
	JSON_OBJECT('dexieVersions', JSON_OBJECT('users', UNIX_TIMESTAMP()))
);
