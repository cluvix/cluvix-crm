import { CUSTOMER_STAGE_TYPE, customerStageColorClass, type CustomerStageColor } from '@/define';

/** 1 dòng response `GET /crm/dashboard/status-customer-overview` (BE story-07): mỗi bước đang hiện + số khách. */
export interface CustomerStageFunnelRow {
  stage_id: number;
  name: string;
  /** `CUSTOMER_STAGE_TYPE`: 1 đang mở · 2 thành công · 3 thất bại. */
  type: number;
  /** Khoá màu bước (`gray`…`purple`). */
  color: string;
  value: number;
}

/** Tầng phễu: `name`/`value` cho `FunnelChartDirective`; `itemStyle.color` ghi đè màu theo index của directive. */
export interface CustomerStageFunnelTier {
  name: string;
  value: number;
  itemStyle: { color: string };
}

/** Bước thất bại — tách khỏi phễu, vẽ thành danh sách riêng. */
export interface CustomerStageLostRow {
  stageId: number;
  name: string;
  value: number;
  /** Class Tailwind chấm màu (cùng bảng màu bước ở bộ lọc/badge). */
  dotClass: string;
}

export interface CustomerStageFunnelView {
  tiers: CustomerStageFunnelTier[];
  lost: CustomerStageLostRow[];
}

/**
 * Khoá màu → mã hex cho ECharts (canvas không dùng được class Tailwind). Mức ĐẬM (-700; gray -600) của palette
 * mặc định Tailwind — cùng họ màu với `CUSTOMER_STAGE_COLOR_CLASS`, KHÔNG màu brand mới — vì nhãn tầng phễu là
 * chữ trắng (`funnelChart.directive.ts`) và phải đạt tương phản ≥ 4.5:1 (ui.md §chart); mức -500 trượt ở 7/8 màu.
 * `Record` ⇒ thêm khoá màu mà quên khai ở đây là lỗi biên dịch.
 */
export const CUSTOMER_STAGE_FUNNEL_HEX: Record<CustomerStageColor, string> = {
  gray: '#4b5563',
  blue: '#1d4ed8',
  teal: '#0f766e',
  green: '#15803d',
  amber: '#b45309',
  orange: '#c2410c',
  red: '#b91c1c',
  purple: '#7e22ce',
};

/**
 * Bề rộng tối thiểu của tầng phễu (% khung vẽ). Nhãn tầng nằm TRONG hình (tên + số, chữ trắng) nên không được để
 * tầng cuối thu về 0% như mặc định ECharts (smoke S2: "Khách hàng đã thuê 0" tràn ra ngoài, chữ trắng trên nền sáng).
 * Với 40%: tầng cuối rộng ~45% khung vẽ (cạnh trên ~50%, cạnh dưới 40%) ⇒ đủ chứa tên bước ~20 ký tự ở cỡ 14px.
 * Thứ tự/giá trị tầng KHÔNG đổi — chỉ đổi thang bề rộng.
 */
export const CUSTOMER_STAGE_FUNNEL_MIN_SIZE = '40%';

const FUNNEL_HEX_FALLBACK = CUSTOMER_STAGE_FUNNEL_HEX.gray;

function funnelHex(color: string | null | undefined): string {
  return CUSTOMER_STAGE_FUNNEL_HEX[(color ?? '') as CustomerStageColor] ?? FUNNEL_HEX_FALLBACK;
}

/**
 * Tách response phễu thành: các TẦNG phễu (bước loại đang mở rồi thành công, giữ thứ tự BE trong từng loại) và
 * KHỐI THẤT BẠI (bước loại thất bại, giữ thứ tự BE). Không phải mảng / dòng hỏng ⇒ bỏ qua, không throw.
 */
export function buildCustomerStageFunnelView(rows: unknown): CustomerStageFunnelView {
  if (!Array.isArray(rows)) return { tiers: [], lost: [] };
  const valid = (rows as Partial<CustomerStageFunnelRow>[]).filter(
    (r): r is CustomerStageFunnelRow => !!r && typeof r === 'object' && r.stage_id != null,
  );
  const toTier = (r: CustomerStageFunnelRow): CustomerStageFunnelTier => ({
    name: r.name,
    value: Number(r.value) || 0,
    itemStyle: { color: funnelHex(r.color) },
  });
  // Loại lạ (ngoài 1/2/3) coi như đang mở ⇒ vẫn hiện trong phễu thay vì mất im lặng.
  const open = valid.filter((r) => r.type !== CUSTOMER_STAGE_TYPE.WON && r.type !== CUSTOMER_STAGE_TYPE.LOST).map(toTier);
  const won = valid.filter((r) => r.type === CUSTOMER_STAGE_TYPE.WON).map(toTier);
  const lost = valid
    .filter((r) => r.type === CUSTOMER_STAGE_TYPE.LOST)
    .map<CustomerStageLostRow>((r) => ({
      stageId: r.stage_id,
      name: r.name,
      value: Number(r.value) || 0,
      dotClass: customerStageColorClass(r.color).dot,
    }));
  return { tiers: [...open, ...won], lost };
}
