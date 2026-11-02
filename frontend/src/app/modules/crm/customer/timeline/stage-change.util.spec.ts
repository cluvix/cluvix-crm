import { describe, expect, it } from 'vitest';

import { TIMELINE_EVENT_STAGE_CHANGE, toStageChangeView } from './stage-change.util';

const base = {
  from_stage_id: 1,
  to_stage_id: 2,
  changed_by: 7,
  changed_by_name: 'Nguyễn Văn A',
  changed_at: 1760000000,
  source: 3,
};

describe('toStageChangeView (story-09 AC4)', () => {
  it('bước cũ → bước mới', () => {
    expect(toStageChangeView({ type: TIMELINE_EVENT_STAGE_CHANGE, stage_change: base })).toEqual({
      isInitial: false,
      fromId: 1,
      toId: 2,
      changedByName: 'Nguyễn Văn A',
    });
  });

  it('bước cũ null ⇒ "Khởi tạo"', () => {
    const v = toStageChangeView({
      type: TIMELINE_EVENT_STAGE_CHANGE,
      stage_change: { ...base, from_stage_id: null },
    });
    expect(v?.isInitial).toBe(true);
    expect(v?.fromId).toBeNull();
    expect(v?.toId).toBe(2);
  });

  it('không có người đổi ⇒ tên rỗng (template ẩn dòng "Người đổi")', () => {
    const v = toStageChangeView({
      type: TIMELINE_EVENT_STAGE_CHANGE,
      stage_change: { ...base, changed_by: null, changed_by_name: '' },
    });
    expect(v?.changedByName).toBe('');
  });

  it('sự kiện khác loại hoặc thiếu khối stage_change ⇒ null (rơi về title/content cũ)', () => {
    expect(toStageChangeView({ stage_change: base })).toBeNull();
    expect(toStageChangeView({ type: 'note', stage_change: base })).toBeNull();
    expect(toStageChangeView({ type: TIMELINE_EVENT_STAGE_CHANGE })).toBeNull();
    expect(
      toStageChangeView({ type: TIMELINE_EVENT_STAGE_CHANGE, stage_change: { ...base, to_stage_id: 0 } }),
    ).toBeNull();
  });
});
