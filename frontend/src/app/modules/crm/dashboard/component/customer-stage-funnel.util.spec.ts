import { describe, expect, it } from 'vitest';

import { CUSTOMER_STAGE_COLORS } from '@/define';

import { buildCustomerStageFunnelView, CUSTOMER_STAGE_FUNNEL_HEX, CUSTOMER_STAGE_FUNNEL_MIN_SIZE } from './customer-stage-funnel.util';
import { isEmptyRows } from '../shell/widget-empty';

/** Response thật của `GET /crm/dashboard/status-customer-overview` (BE story-07). */
const REAL_RESPONSE = [
  { stage_id: 1, name: 'Liên hệ', type: 1, color: 'gray', value: 11343 },
  { stage_id: 2, name: 'Đang tư vấn', type: 1, color: 'blue', value: 7924 },
  { stage_id: 3, name: 'Đang đàm phán', type: 1, color: 'amber', value: 3611 },
  { stage_id: 4, name: 'Đã lên đơn', type: 1, color: 'purple', value: 885 },
  { stage_id: 5, name: 'Khách hàng đã thuê', type: 2, color: 'green', value: 136 },
  { stage_id: 6, name: 'Từ chối', type: 3, color: 'red', value: 1539 },
  { stage_id: 7, name: 'Dừng chăm sóc', type: 3, color: 'orange', value: 2191 },
  { stage_id: 8, name: 'Ngừng thuê', type: 3, color: 'teal', value: 2165 },
];

/** Tương phản WCAG 2.x giữa `#rrggbb` và trắng. */
function contrastWithWhite(hex: string): number {
  const lin = (v: number) => (v / 255 <= 0.03928 ? v / 255 / 12.92 : Math.pow((v / 255 + 0.055) / 1.055, 2.4));
  const [r, g, b] = [1, 3, 5].map((i) => lin(parseInt(hex.slice(i, i + 2), 16)));
  const lum = 0.2126 * r + 0.7152 * g + 0.0722 * b;
  return 1.05 / (lum + 0.05);
}

describe('buildCustomerStageFunnelView — phễu theo bước cấu hình', () => {
  it('tầng = bước đang mở rồi thành công theo thứ tự BE; thất bại tách riêng', () => {
    const v = buildCustomerStageFunnelView(REAL_RESPONSE);
    expect(v.tiers.map((t) => t.name)).toEqual(['Liên hệ', 'Đang tư vấn', 'Đang đàm phán', 'Đã lên đơn', 'Khách hàng đã thuê']);
    expect(v.tiers.map((t) => t.value)).toEqual([11343, 7924, 3611, 885, 136]);
    expect(v.lost.map((l) => l.name)).toEqual(['Từ chối', 'Dừng chăm sóc', 'Ngừng thuê']);
    expect(v.lost.map((l) => l.value)).toEqual([1539, 2191, 2165]);
  });

  it('màu tầng theo khoá màu bước (hex), màu thất bại theo class chấm màu', () => {
    const v = buildCustomerStageFunnelView(REAL_RESPONSE);
    expect(v.tiers[1].itemStyle.color).toBe(CUSTOMER_STAGE_FUNNEL_HEX.blue);
    expect(v.tiers[4].itemStyle.color).toBe(CUSTOMER_STAGE_FUNNEL_HEX.green);
    expect(v.lost[0].dotClass).toContain('bg-red-500');
    expect(v.lost[2].dotClass).toContain('bg-teal-500');
  });

  it('bước thành công đứng SAU bước đang mở dù BE trả lẫn thứ tự', () => {
    const v = buildCustomerStageFunnelView([
      { stage_id: 5, name: 'Thành công', type: 2, color: 'green', value: 1 },
      { stage_id: 1, name: 'Mở', type: 1, color: 'gray', value: 2 },
    ]);
    expect(v.tiers.map((t) => t.name)).toEqual(['Mở', 'Thành công']);
  });

  it('khoá màu lạ ⇒ xám, không throw; loại lạ coi như đang mở', () => {
    const v = buildCustomerStageFunnelView([{ stage_id: 9, name: 'X', type: 99, color: 'pink', value: 4 }]);
    expect(v.tiers).toHaveLength(1);
    expect(v.tiers[0].itemStyle.color).toBe(CUSTOMER_STAGE_FUNNEL_HEX.gray);
  });

  it('[] / null / không phải mảng ⇒ view rỗng; widget coi [] là trạng thái rỗng', () => {
    for (const bad of [[], null, undefined, {}, 'x']) {
      expect(buildCustomerStageFunnelView(bad)).toEqual({ tiers: [], lost: [] });
    }
    expect(isEmptyRows([])).toBe(true);
    expect(isEmptyRows(REAL_RESPONSE)).toBe(false);
  });

  it('mọi khoá màu hợp lệ của bước đều có mã hex và ngược lại (thêm/đổi khoá ở 1 bên quên bên kia ⇒ đỏ)', () => {
    for (const c of CUSTOMER_STAGE_COLORS) expect(CUSTOMER_STAGE_FUNNEL_HEX[c], c).toMatch(/^#[0-9a-f]{6}$/);
    expect(Object.keys(CUSTOMER_STAGE_FUNNEL_HEX).sort()).toEqual([...CUSTOMER_STAGE_COLORS].sort());
  });

  it('nhãn tầng là chữ trắng ⇒ mỗi hex đạt tương phản WCAG ≥ 4.5:1 với #fff', () => {
    for (const c of CUSTOMER_STAGE_COLORS) {
      expect(contrastWithWhite(CUSTOMER_STAGE_FUNNEL_HEX[c]), c).toBeGreaterThanOrEqual(4.5);
    }
    // đối chứng: hàm tính đúng — nền -500 cũ của amber trượt (≈2.15), đen/trắng đúng mốc
    expect(contrastWithWhite('#f59e0b')).toBeLessThan(4.5);
    expect(contrastWithWhite('#000000')).toBeCloseTo(21, 5);
    expect(contrastWithWhite('#ffffff')).toBeCloseTo(1, 5);
  });

  it('bảng hex được ghim đúng giá trị (đổi màu phải là quyết định có chủ đích, kèm đo lại tương phản)', () => {
    expect(CUSTOMER_STAGE_FUNNEL_HEX).toEqual({
      gray: '#4b5563',
      blue: '#1d4ed8',
      teal: '#0f766e',
      green: '#15803d',
      amber: '#b45309',
      orange: '#c2410c',
      red: '#b91c1c',
      purple: '#7e22ce',
    });
  });

  it('view mang đúng hex của bảng cho từng tầng (không truyền màu khác)', () => {
    const v = buildCustomerStageFunnelView(REAL_RESPONSE);
    expect(v.tiers.map((t) => t.itemStyle.color)).toEqual(
      ['gray', 'blue', 'amber', 'purple', 'green'].map((c) => CUSTOMER_STAGE_FUNNEL_HEX[c as keyof typeof CUSTOMER_STAGE_FUNNEL_HEX]),
    );
  });

  it('minSize phễu đủ rộng để nhãn nằm trong hình ở tầng cuối (smoke S2: nhãn tràn khi tầng thu về 0%)', () => {
    expect(CUSTOMER_STAGE_FUNNEL_MIN_SIZE).toMatch(/^\d+(\.\d+)?%$/);
    const minPct = parseFloat(CUSTOMER_STAGE_FUNNEL_MIN_SIZE);
    expect(minPct).toBeLessThan(100);
    // Mô phỏng thang ECharts funnel (directive chuẩn hoá value = N..1, min = 0): tầng cuối rộng
    // minSize + (100 − minSize)/N ở cạnh trên và minSize ở cạnh dưới; nhãn nằm giữa ⇒ bề rộng trung bình.
    // Nhãn "tên + số" cỡ 14px cần ≥ ~40% khung vẽ (khung ≥ 360px, ~20 ký tự) — với minSize 0% chỉ còn ~10%.
    const tiers = buildCustomerStageFunnelView(REAL_RESPONSE).tiers.length;
    const top = minPct + (100 - minPct) / tiers;
    const labelCenterWidth = (top + minPct) / 2;
    expect(labelCenterWidth).toBeGreaterThanOrEqual(40);
    // đối chứng công thức: minSize 0% (mặc định cũ) ⇒ trượt ngưỡng
    expect((100 / tiers + 0) / 2).toBeLessThan(40);
  });
});
