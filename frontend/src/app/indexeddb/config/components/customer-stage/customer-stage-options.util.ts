import { CustomerStageModel } from '@/core/models/config/customer-stage.model';

/** Một dòng trong dropdown/bộ lọc chọn bước. `hidden` = bước đã bị ẩn (status = 0). */
export interface CustomerStageOption {
  id: number;
  name: string;
  color: string;
  type: number | null;
  hidden: boolean;
}

/** Trạng thái "đang hiện" của bước (BE `status` 1 = hiện, 0 = ẩn). */
export const CUSTOMER_STAGE_STATUS_VISIBLE = 1;

/** Sắp theo `sort_order` tăng dần, cùng thứ tự thì theo `id` — trả mảng MỚI, không đổi mảng nguồn. */
export function sortCustomerStages(items: readonly CustomerStageModel[]): CustomerStageModel[] {
  return [...items].sort(
    (a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0) || (a.id ?? 0) - (b.id ?? 0),
  );
}

export interface BuildStageOptionsParams {
  /** Id đang chọn của bản ghi — bước ẩn nằm trong đây vẫn được liệt kê (kèm cờ `hidden`). */
  selectedIds?: readonly number[] | null;
  /** true = liệt kê cả bước ẩn (bộ lọc). false = chỉ bước hiện + bước ẩn đang được chọn (form). */
  includeHidden?: boolean;
}

/**
 * Dựng danh sách lựa chọn bước từ toàn bộ bước trong cache (gồm cả ẩn):
 * - form chọn bước: bước hiện theo `sort_order` + bước hiện tại của bản ghi nếu nó đang ẩn;
 * - bộ lọc (`includeHidden`): mọi bước, bước ẩn có `hidden = true` để UI đánh dấu "đã ẩn".
 * Hàm thuần — không đụng cache/DOM nên test được độc lập.
 */
export function buildCustomerStageOptions(
  all: readonly CustomerStageModel[],
  params: BuildStageOptionsParams = {},
): CustomerStageOption[] {
  const selected = new Set(params.selectedIds ?? []);
  return sortCustomerStages(all)
    .filter(
      (s) =>
        s.id !== undefined &&
        (params.includeHidden ||
          s.status === CUSTOMER_STAGE_STATUS_VISIBLE ||
          selected.has(s.id)),
    )
    .map((s) => ({
      id: s.id as number,
      name: s.name ?? '',
      color: s.color ?? '',
      type: s.type ?? null,
      hidden: s.status !== CUSTOMER_STAGE_STATUS_VISIBLE,
    }));
}
