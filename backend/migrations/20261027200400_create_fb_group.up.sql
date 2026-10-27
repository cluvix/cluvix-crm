-- Module Group Facebook (bmad/fb-group/thiet-ke.md): group do NextCore GROUPCRAWL crawl về + chủ đề phân loại.
-- 1 DB = 1 thương hiệu (D15) ⇒ không có company_id. Group là dữ liệu thị trường dùng chung toàn thương hiệu
-- ⇒ KHÔNG có branch_id (anh Lộc duyệt 2026-10-27).
-- created_by/updated_by: cột audit ⇒ KHÔNG FK (Hard Rule ngoại lệ), chỉ KEY.
-- Idempotent: CREATE TABLE IF NOT EXISTS (chạy lại khi dirty state không lỗi).

CREATE TABLE IF NOT EXISTS `fb_group` (
  `id` int NOT NULL AUTO_INCREMENT,
  `facebook_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Mã group Facebook (số, hoặc tên rút gọn khi chưa đọc được mã số) — khoá ghi đè khi nhận lại',
  `name` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Tên group',
  `name_normalized` varchar(255) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'Tên chữ thường, bỏ dấu — dùng phân loại và tìm không dấu',
  `url` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Link group',
  `slug` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Tên rút gọn trong link',
  `member_count` bigint DEFAULT NULL COMMENT 'Số thành viên',
  `member_rounded` tinyint(1) NOT NULL DEFAULT '0' COMMENT '1 = số Facebook làm tròn (27K)',
  `posts_per_day` int DEFAULT NULL COMMENT 'Số bài mỗi ngày Facebook hiện ở trang tìm',
  `privacy` tinyint DEFAULT NULL COMMENT '1 công khai, 2 riêng tư',
  `ncb_region` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Khu vực NextCore gửi kèm',
  `ncb_category` varchar(160) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Danh mục ngành NextCore gửi kèm',
  `province` varchar(64) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Tỉnh CRM tự nhận từ tên group',
  `found_keyword` varchar(255) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Từ khoá GROUPCRAWL tìm ra (từ tệp CSV)',
  `found_by_profile` varchar(128) COLLATE utf8mb4_unicode_ci DEFAULT NULL COMMENT 'Profile NextCore tìm ra (từ tệp CSV)',
  `ncb_quality` tinyint DEFAULT NULL COMMENT '1 = NextCore Phân tích lại giữ (có trong tệp CSV)',
  `work_status` tinyint NOT NULL DEFAULT '0' COMMENT '0 Mới, 1 Mục tiêu, 2 Theo dõi, 3 Loại',
  `note` text COLLATE utf8mb4_unicode_ci COMMENT 'Ghi chú nội bộ',
  `first_seen_at` bigint DEFAULT NULL COMMENT 'Lần đầu NextCore thấy group',
  `last_seen_at` bigint DEFAULT NULL COMMENT 'Lần gần nhất NextCore thấy group',
  `synced_at` bigint DEFAULT NULL COMMENT 'Lần gần nhất CRM nhận số liệu group',
  `status` tinyint DEFAULT '1',
  `created_at` bigint DEFAULT NULL,
  `updated_at` bigint NOT NULL DEFAULT '0',
  `created_by` int DEFAULT NULL,
  `updated_by` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fb_group_facebook_id` (`facebook_id`),
  KEY `idx_fb_group_member_count` (`member_count`),
  KEY `idx_fb_group_work_status` (`work_status`),
  KEY `idx_fb_group_province` (`province`),
  KEY `idx_fb_group_created_at` (`created_at`),
  KEY `idx_fb_group_created_by` (`created_by`),
  KEY `idx_fb_group_updated_by` (`updated_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Group Facebook crawl từ NextCore GROUPCRAWL';

CREATE TABLE IF NOT EXISTS `fb_group_topic` (
  `id` int NOT NULL AUTO_INCREMENT,
  `name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Tên chủ đề',
  `color` varchar(16) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '#7B3F9E' COMMENT 'Màu hiển thị (bảng màu chart)',
  `include_keywords` json NOT NULL COMMENT 'Từ khoá gồm — tên group khớp ≥1 từ',
  `exclude_keywords` json NOT NULL COMMENT 'Từ khoá loại trừ — khớp là bỏ',
  `min_members` int NOT NULL DEFAULT '10000' COMMENT 'Ngưỡng mục tiêu: thành viên tối thiểu',
  `min_posts_per_day` int NOT NULL DEFAULT '10' COMMENT 'Ngưỡng mục tiêu: bài/ngày tối thiểu',
  `public_only` tinyint(1) NOT NULL DEFAULT '1' COMMENT 'Ngưỡng mục tiêu: chỉ group công khai',
  `sort_order` int NOT NULL DEFAULT '0',
  `status` tinyint DEFAULT '1',
  `created_at` bigint DEFAULT NULL,
  `updated_at` bigint NOT NULL DEFAULT '0',
  `created_by` int DEFAULT NULL,
  `updated_by` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fb_group_topic_name` (`name`),
  KEY `idx_fb_group_topic_created_by` (`created_by`),
  KEY `idx_fb_group_topic_updated_by` (`updated_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Chủ đề phân loại group Facebook';

CREATE TABLE IF NOT EXISTS `fb_group_topic_branch` (
  `id` int NOT NULL AUTO_INCREMENT,
  `topic_id` int NOT NULL COMMENT 'Chủ đề cha',
  `name` varchar(128) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Tên nhánh',
  `keywords` json NOT NULL COMMENT 'Từ khoá của nhánh',
  `sort_order` int NOT NULL DEFAULT '0' COMMENT 'Thứ tự xét — nhánh đầu tiên khớp được gắn',
  `created_at` bigint DEFAULT NULL,
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fb_group_topic_branch_name` (`topic_id`, `name`),
  CONSTRAINT `fk_fb_group_topic_branch_topic_id` FOREIGN KEY (`topic_id`) REFERENCES `fb_group_topic` (`id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Nhánh trong chủ đề group Facebook';

-- Group ↔ chủ đề (nhiều-nhiều). source: 1 tự động, 2 gán tay, 3 bỏ tay. Dòng tay (2,3) phân loại tự động
-- KHÔNG ghi đè — nhờ PK (group_id, topic_id) + INSERT IGNORE.
CREATE TABLE IF NOT EXISTS `fb_group_topic_map` (
  `group_id` int NOT NULL,
  `topic_id` int NOT NULL,
  `branch_id` int DEFAULT NULL COMMENT 'Nhánh khớp; NULL = chưa thuộc nhánh nào',
  `source` tinyint NOT NULL DEFAULT '1' COMMENT '1 tự động, 2 gán tay, 3 bỏ tay',
  `created_at` bigint DEFAULT NULL,
  PRIMARY KEY (`group_id`, `topic_id`),
  KEY `idx_fb_group_topic_map_topic` (`topic_id`, `source`, `branch_id`),
  KEY `idx_fb_group_topic_map_branch` (`branch_id`),
  CONSTRAINT `fk_fb_group_topic_map_group_id` FOREIGN KEY (`group_id`) REFERENCES `fb_group` (`id`) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT `fk_fb_group_topic_map_topic_id` FOREIGN KEY (`topic_id`) REFERENCES `fb_group_topic` (`id`) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT `fk_fb_group_topic_map_branch_id` FOREIGN KEY (`branch_id`) REFERENCES `fb_group_topic_branch` (`id`) ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Group Facebook thuộc chủ đề nào';

-- Khoá API NextCore dùng để đẩy group vào POST /api/fbgroup/ingest. Chỉ lưu SHA-256 của khoá (khoá chỉ
-- hiện 1 lần lúc tạo) — không cần giải mã, nên KHÔNG dùng pkg/crypto (đó là cho secret phải đọc lại được).
CREATE TABLE IF NOT EXISTS `fb_group_ingest_key` (
  `id` int NOT NULL AUTO_INCREMENT,
  `key_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'SHA-256 hex của khoá',
  `key_hint` varchar(8) COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '4 ký tự cuối để nhận ra khoá',
  `last_used_at` bigint DEFAULT NULL COMMENT 'Lần NextCore gọi gần nhất',
  `last_batch_count` int DEFAULT NULL COMMENT 'Số group của lô gần nhất',
  `total_received` bigint NOT NULL DEFAULT '0' COMMENT 'Tổng số group đã nhận qua khoá này',
  `status` tinyint DEFAULT '1' COMMENT '1 đang dùng, 0 đã thu hồi',
  `created_at` bigint DEFAULT NULL,
  `updated_at` bigint NOT NULL DEFAULT '0',
  `created_by` int DEFAULT NULL,
  `updated_by` int DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_fb_group_ingest_key_hash` (`key_hash`),
  KEY `idx_fb_group_ingest_key_created_by` (`created_by`),
  KEY `idx_fb_group_ingest_key_updated_by` (`updated_by`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Khoá API NextCore đẩy group Facebook';
