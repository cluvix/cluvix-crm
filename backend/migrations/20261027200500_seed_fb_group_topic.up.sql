-- Seed 3 chủ đề group Facebook ban đầu (Dạ dày, Mẹ bầu, Vì con) + nhánh — bộ từ khoá rút từ đợt phân tích
-- 9.893 group GROUPCRAWL (2026-10-27). Từ khoá viết có dấu cho người đọc; bộ phân loại tự bỏ dấu cả hai phía.
-- Idempotent: INSERT IGNORE theo khoá seed UNIQUE thật (`fb_group_topic.name`, `(topic_id, name)` của nhánh).
-- Người dùng sửa từ khoá sau đó thì chạy lại migration KHÔNG ghi đè (IGNORE).
-- Tránh từ khoá 1 âm tiết dễ trùng (vd "bầu" = "bầu cử", "đẻ" = "để/dễ" sau khi bỏ dấu).

INSERT IGNORE INTO `fb_group_topic`
  (`name`, `color`, `include_keywords`, `exclude_keywords`, `min_members`, `min_posts_per_day`, `public_only`, `sort_order`, `status`, `created_at`, `updated_at`)
VALUES
  ('Mẹ bầu', '#7B3F9E',
   JSON_ARRAY('mẹ bầu', 'bà bầu', 'chị em bầu', 'đang bầu', 'bầu bí', 'hội bầu', 'xin vía bầu', 'mang thai', 'thai kỳ',
              'thai sản', 'sau sinh', 'ở cữ', 'team đẻ', 'sinh đôi', 'bỉm sữa', 'mẹ bỉm', 'làm mẹ', 'sinh con',
              'chuẩn bị mang thai', 'sữa bầu'),
   JSON_ARRAY(), 10000, 10, 1, 1, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
  ('Vì con', '#FF5722',
   JSON_ARRAY('vì con', 'vì tương lai con', 'nuôi con', 'mẹ và bé', 'mẹ & bé', 'mẹ vs bé', 'bé và mẹ', 'mẹ bé', 'dạy con',
              'mẹ nuôi', 'nghiện con', 'yêu con', 'chăm con', 'chăm sóc bé', 'con nhỏ', 'sơ sinh', 'ăn dặm', 'bé khỏe',
              'bé yêu', 'bé ngoan', 'trẻ em', 'trẻ nhỏ', 'cho bé', 'cho con', 'đồ bé', 'sữa mẹ', 'sữa non',
              'mẹ thông thái', 'bà mẹ', 'các mẹ', 'phụ huynh', 'mẹ con'),
   JSON_ARRAY(), 10000, 10, 1, 2, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
  ('Dạ dày', '#3B82F6',
   JSON_ARRAY('dạ dày', 'dạ giày', 'bao tử', 'tiêu hoá', 'tiêu hóa', 'trào ngược', 'vi khuẩn hp', 'hp dạ dày', 'đường ruột',
              'đại tràng', 'ruột kích thích', 'viêm loét', 'bình vị'),
   JSON_ARRAY('áo dạ', 'dạ hội', 'dạ tweed', 'dày sừng'), 10000, 10, 1, 3, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP());

INSERT IGNORE INTO `fb_group_topic_branch` (`topic_id`, `name`, `keywords`, `sort_order`, `created_at`, `updated_at`)
SELECT t.`id`, b.`name`, b.`keywords`, b.`sort_order`, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()
FROM `fb_group_topic` t
JOIN (
  SELECT 'Mẹ bầu' AS topic, 'sau sinh / ở cữ' AS name, JSON_ARRAY('sau sinh', 'ở cữ') AS keywords, 1 AS sort_order
  UNION ALL SELECT 'Mẹ bầu', 'mang thai', JSON_ARRAY('mang thai', 'mẹ bầu', 'bà bầu', 'thai kỳ', 'chuẩn bị mang thai', 'đang bầu', 'bầu bí', 'chị em bầu'), 2
  UNION ALL SELECT 'Mẹ bầu', 'mẹ bỉm sữa', JSON_ARRAY('bỉm sữa', 'mẹ bỉm'), 3
  UNION ALL SELECT 'Vì con', 'ăn dặm / dinh dưỡng', JSON_ARRAY('ăn dặm', 'dinh dưỡng', 'sữa công thức', 'ăn ngon'), 1
  UNION ALL SELECT 'Vì con', 'dạy con', JSON_ARRAY('dạy con', 'giáo dục', 'kỹ năng'), 2
  UNION ALL SELECT 'Vì con', 'trẻ sơ sinh', JSON_ARRAY('sơ sinh', 'trẻ nhỏ', 'em bé'), 3
  UNION ALL SELECT 'Dạ dày', 'trào ngược', JSON_ARRAY('trào ngược'), 1
  UNION ALL SELECT 'Dạ dày', 'vi khuẩn HP', JSON_ARRAY('vi khuẩn hp', 'hp dạ dày', 'vi khuẩn'), 2
  UNION ALL SELECT 'Dạ dày', 'nội soi', JSON_ARRAY('nội soi'), 3
  UNION ALL SELECT 'Dạ dày', 'ăn uống', JSON_ARRAY('thực đơn', 'ăn uống', 'dinh dưỡng', 'món ăn'), 4
  UNION ALL SELECT 'Dạ dày', 'tiêu hoá / đại tràng', JSON_ARRAY('đại tràng', 'tiêu hoá', 'tiêu hóa', 'ruột'), 5
) b ON b.`topic` COLLATE utf8mb4_unicode_ci = t.`name`;
