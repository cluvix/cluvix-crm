// Hằng số module Group Facebook — giá trị số trùng backend/pkg/define/fbgroup.go, KHÔNG đổi.

export const FB_GROUP_WORK_STATUS = {
  NEW: 0,
  TARGET: 1,
  WATCH: 2,
  EXCLUDED: 3,
} as const;

/** Tag trạng thái làm việc: nhãn i18n + severity của p-tag. */
export const FB_GROUP_WORK_STATUS_OPTIONS = [
  { value: FB_GROUP_WORK_STATUS.NEW, labelKey: 'fbgroup.work_status.new', severity: 'secondary' },
  { value: FB_GROUP_WORK_STATUS.TARGET, labelKey: 'fbgroup.work_status.target', severity: 'success' },
  { value: FB_GROUP_WORK_STATUS.WATCH, labelKey: 'fbgroup.work_status.watch', severity: 'info' },
  { value: FB_GROUP_WORK_STATUS.EXCLUDED, labelKey: 'fbgroup.work_status.excluded', severity: 'danger' },
] as const;

export const FB_GROUP_PRIVACY = { PUBLIC: 1, PRIVATE: 2 } as const;

export const FB_GROUP_PRIVACY_OPTIONS = [
  { value: FB_GROUP_PRIVACY.PUBLIC, labelKey: 'fbgroup.privacy.public' },
  { value: FB_GROUP_PRIVACY.PRIVATE, labelKey: 'fbgroup.privacy.private' },
] as const;

/** Nguồn gán chủ đề (fb_group_topic_map.source). */
export const FB_GROUP_TOPIC_SOURCE = { AUTO: 1, MANUAL_ADD: 2, MANUAL_REMOVED: 3 } as const;

/** Bảng màu cho chủ đề — đúng 5 màu chart của brand (ui.md §Chart Colors). */
export const FB_GROUP_TOPIC_COLORS = ['#7B3F9E', '#FF5722', '#10B981', '#3B82F6', '#F59E0B'] as const;

/** Ngưỡng hiển thị "nhóm lớn" — trùng define.FbGroupBigMembers ở BE. */
export const FB_GROUP_BIG_MEMBERS = 100000;
