import { SERVICE_DISCOUNT_TYPE_SELECT } from '@/define';

import type { ServiceQuickAddEvent } from './service-quick-add';

/** Các field của 1 dòng bảng (Đơn hàng) mà luật cộng dồn cần đọc. */
export interface QuickAddMergeableRow {
  service_id: number | null;
  unit_price: number | null;
  service_discount: number | null;
  service_discount_type: number | null;
  discount_other: number | null;
}

const num = (v: number | null | undefined) => Number(v ?? 0) || 0;

/**
 * Chiết khấu DV của khung và dòng có gộp được không: cả hai = 0 (bất kể loại), hoặc cùng loại % và cùng %. Loại đ (tiền)
 * khác 0 KHÔNG gộp — số tiền tuyệt đối theo dòng, cộng SL vào sẽ sai tiền.
 */
function sameServiceDiscount(row: QuickAddMergeableRow, e: ServiceQuickAddEvent): boolean {
  const rowDiscount = num(row.service_discount);
  const entryDiscount = num(e.service_discount);
  if (rowDiscount === 0 && entryDiscount === 0) return true;
  const percent = SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT;
  return row.service_discount_type === percent && e.service_discount_type === percent && rowDiscount === entryDiscount;
}

/**
 * Luật CỘNG DỒN của khung "Thêm dịch vụ" (port từ ERP, bỏ nhánh dịch vụ răng — CRM không có sơ đồ răng). Trả index dòng
 * ĐẦU TIÊN (từ trên xuống) thoả MỌI điều kiện, `-1` ⇒ thêm dòng mới:
 * 1. cùng `service_id`; 2. cùng đơn giá (so số, null ≡ 0); 3. chiết khấu DV khớp (`sameServiceDiscount`);
 * 4. dòng đích không có Giảm khác (rỗng/0 — số tuyệt đối theo dòng); 5. `canMerge(row)` của form (Đơn hàng: dòng chưa
 *    khoá, chưa thanh toán).
 */
export function findQuickAddMergeIndex<R extends QuickAddMergeableRow>(
  rows: readonly R[],
  e: ServiceQuickAddEvent,
  canMerge: (row: R) => boolean = () => true,
): number {
  if (e.service.id == null) return -1;
  return rows.findIndex(
    (row) =>
      row.service_id === e.service.id &&
      num(row.unit_price) === num(e.unit_price) &&
      sameServiceDiscount(row, e) &&
      num(row.discount_other) === 0 &&
      canMerge(row),
  );
}

/** Mảng mới: dòng `index` (đã cộng SL) lên ĐẦU, các dòng khác giữ thứ tự — KHÔNG mutate mảng/dòng cũ. */
export function moveMergedRowToTop<R extends { quantity: number | null }>(rows: readonly R[], index: number, addQuantity: number): R[] {
  const target = rows[index];
  const merged = { ...target, quantity: num(target.quantity) + addQuantity };
  return [merged, ...rows.slice(0, index), ...rows.slice(index + 1)];
}
