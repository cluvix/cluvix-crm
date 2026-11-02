import { describe, expect, it } from 'vitest';

import { applyStageChange, shouldChangeStage } from './customer-stage-change.util';

describe('shouldChangeStage (story-09 AC3)', () => {
  it('chọn bước khác bước hiện tại ⇒ gọi API', () => {
    expect(shouldChangeStage(1, 2)).toBe(true);
  });

  it('chọn lại đúng bước hiện tại ⇒ KHÔNG gọi API', () => {
    expect(shouldChangeStage(3, 3)).toBe(false);
  });

  it('id lệch kiểu (chuỗi/số từ JSON) vẫn so đúng', () => {
    expect(shouldChangeStage('3' as unknown as number, 3)).toBe(false);
  });

  it('bước đích rỗng/không hợp lệ ⇒ không gọi', () => {
    expect(shouldChangeStage(1, null)).toBe(false);
    expect(shouldChangeStage(1, undefined)).toBe(false);
    expect(shouldChangeStage(1, 0)).toBe(false);
    expect(shouldChangeStage(1, -4)).toBe(false);
  });

  it('khách chưa có bước hiện tại ⇒ chọn bước hợp lệ là đổi', () => {
    expect(shouldChangeStage(null, 5)).toBe(true);
    expect(shouldChangeStage(undefined, 5)).toBe(true);
  });
});

describe('applyStageChange (story-09 AC2 — badge cập nhật không cần F5)', () => {
  const current = { id: 10, stage_id: 1, updated_at: 1000, fullname: 'Khách A' };

  it('đúng khách ⇒ model MỚI với stage_id + updated_at từ response, field khác giữ nguyên', () => {
    const next = applyStageChange(current, { id: 10, stage_id: 4, updated_at: 2000 });
    expect(next).not.toBe(current);
    expect(next).toEqual({ id: 10, stage_id: 4, updated_at: 2000, fullname: 'Khách A' });
    expect(current.stage_id).toBe(1); // không mutate model cũ
  });

  it('khách khác (id lệch) ⇒ trả NGUYÊN reference, không đổi gì', () => {
    const next = applyStageChange(current, { id: 99, stage_id: 4, updated_at: 2000 });
    expect(next).toBe(current);
  });

  it('model rỗng ⇒ trả nguyên (không throw)', () => {
    expect(applyStageChange(null, { id: 10, stage_id: 4 })).toBeNull();
    expect(applyStageChange(undefined, { id: 10, stage_id: 4 })).toBeUndefined();
  });

  it('response thiếu updated_at ⇒ giữ updated_at cũ', () => {
    const next = applyStageChange(current, { id: 10, stage_id: 4 });
    expect(next?.updated_at).toBe(1000);
    expect(next?.stage_id).toBe(4);
  });
});
