import type { CustomerStageChangeResult } from '@/core/models/crm/customer.model';

/**
 * Hàm thuần cho ô đổi bước ở chi tiết khách — tách khỏi component (PrimeNG) để test bằng vitest-base.
 *
 * Chọn lại đúng bước hiện tại ⇒ KHÔNG gọi API (AC3). `next` rỗng/không hợp lệ cũng không gọi.
 * Trả `true` khi cần gọi `PUT /crm/customer/:id/stage`.
 */
export function shouldChangeStage(
  current: number | null | undefined,
  next: number | null | undefined,
): boolean {
  if (!next || next <= 0) return false;
  return Number(current ?? 0) !== Number(next);
}

/**
 * Gộp kết quả `PUT /crm/customer/:id/stage` vào model đang hiển thị (hàm thuần — component chỉ gọi).
 * - model rỗng hoặc kết quả của KHÁCH KHÁC (id lệch — vd đã chuyển trang trong lúc chờ API) ⇒ trả NGUYÊN
 *   `current` (cùng reference) để caller biết không có gì đổi;
 * - đúng khách ⇒ model MỚI (OnPush cần reference mới) với `stage_id` + `updated_at` lấy từ response.
 *   `updated_at` PHẢI nhận về, không thì lần sửa form kế tiếp bị 409 (optlock); response thiếu thì giữ cũ.
 */
export function applyStageChange<
  T extends { id?: number; stage_id?: number; updated_at?: number | Date },
>(current: T | null | undefined, result: CustomerStageChangeResult): T | null | undefined {
  if (!current || current.id !== result.id) return current;
  return {
    ...current,
    stage_id: result.stage_id,
    updated_at: result.updated_at ?? current.updated_at,
  };
}
