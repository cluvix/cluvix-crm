/** Loại sự kiện timeline do BE trả cho lần đổi/gán bước (story-04). Khớp `define.CustomerTimelineEventStageChange`. */
export const TIMELINE_EVENT_STAGE_CHANGE = 'stage_change';

/** Khối `stage_change` của sự kiện timeline — BE chỉ trả id bước, tên/màu tra Dexie (architecture D9). */
export interface TimelineStageChange {
  from_stage_id: number | null;
  to_stage_id: number;
  changed_by: number | null;
  changed_by_name: string;
  changed_at: number;
  source: number;
}

/** Dữ liệu đã chuẩn hoá để template render: "Khởi tạo: <bước>" hoặc "<bước cũ> → <bước mới>". */
export interface StageChangeView {
  /** true = lần gán bước đầu tiên (không có bước cũ) ⇒ hiển thị "Khởi tạo: <bước>". */
  isInitial: boolean;
  fromId: number | null;
  toId: number;
  changedByName: string;
}

/**
 * Sự kiện không phải `stage_change` (hoặc thiếu khối `stage_change`) ⇒ null để template rơi về nhánh
 * title/content cũ. `from_stage_id` null/0/thiếu ⇒ khởi tạo.
 */
export function toStageChangeView(event: {
  type?: string | null;
  stage_change?: Partial<TimelineStageChange> | null;
}): StageChangeView | null {
  if (event?.type !== TIMELINE_EVENT_STAGE_CHANGE || !event.stage_change) return null;
  const sc = event.stage_change;
  if (!sc.to_stage_id) return null;
  const fromId = sc.from_stage_id ? Number(sc.from_stage_id) : null;
  return {
    isInitial: fromId === null,
    fromId,
    toId: Number(sc.to_stage_id),
    changedByName: (sc.changed_by_name ?? '').trim(),
  };
}
