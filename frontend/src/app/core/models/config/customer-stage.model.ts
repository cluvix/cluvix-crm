import { Identifiable } from '@/core/interface/identifiable';

/**
 * Một bước trong pipeline tiến trình khách hàng (BE `customer_stage`, Dexie `customerStages`).
 * `type`/`color` khớp hằng số ở `@/define` (`CUSTOMER_STAGE_TYPE`, `CUSTOMER_STAGE_COLORS`).
 * `status`: 1 = đang hiện, 0 = đã ẩn (bước ẩn vẫn nằm trong cache để tra tên cho bản ghi cũ).
 */
export interface CustomerStageModel extends Identifiable {
  id?: number;
  name?: string;
  type?: number | null;
  color?: string | null;
  sort_order?: number;
  is_default?: number;
  status?: number;
  created_at?: number | null;
  updated_at?: number | null;
  created_by?: number | null;
  updated_by?: number | null;
}
