import { describe, expect, it } from 'vitest';

import { CRM_DASHBOARD_WIDGETS, CRM_DASHBOARD_WIDGET_MAP } from './dashboard-widgets';
import { DASHBOARD_LAYOUT_PRESETS, presetLayout, presetSlots } from './layout-presets';
import { fullLayout, layoutSignature, zoneItems } from './layout.util';
import { presetCards } from './preset-chooser';

const ALL_KEYS = CRM_DASHBOARD_WIDGETS.map((w) => w.key).sort();

describe('Mẫu bố cục đủ khối', () => {
  it.each(DASHBOARD_LAYOUT_PRESETS.map((p) => p.id))('%s: đủ 13 khối, mỗi khối 1 lần, đều hiện', (id) => {
    const cfg = presetLayout(id, CRM_DASHBOARD_WIDGETS);
    expect(cfg.items.map((x) => x.key).sort()).toEqual(ALL_KEYS);
    expect(cfg.items.every((x) => x.visible)).toBe(true);
  });

  it.each(DASHBOARD_LAYOUT_PRESETS.map((p) => p.id))('%s: vùng trên 1 khối, cột phụ chỉ cỡ đầy đủ', (id) => {
    const cfg = presetLayout(id, CRM_DASHBOARD_WIDGETS);
    expect(zoneItems(cfg, 'top')).toHaveLength(1);
    expect(zoneItems(cfg, 'side').every((x) => x.size === 'full')).toBe(true);
  });

  it.each(DASHBOARD_LAYOUT_PRESETS.map((p) => p.id))('%s: khối ½ trong cột chính đi CẶP, cùng chiều cao biểu đồ', (id) => {
    const main = zoneItems(presetLayout(id, CRM_DASHBOARD_WIDGETS), 'main');
    const halves: string[] = [];
    for (const x of main) {
      if (x.size === 'half') {
        halves.push(x.key);
        continue;
      }
      expect(halves.length % 2).toBe(0);
    }
    expect(halves.length % 2).toBe(0);
    for (let i = 0; i < halves.length; i += 2) {
      const h = (k: string) => CRM_DASHBOARD_WIDGET_MAP.get(k)?.chartHeight ?? null;
      expect(h(halves[i]), `${halves[i]} ↔ ${halves[i + 1]}`).toBe(h(halves[i + 1]));
    }
  });

  it('slot của 3 mẫu vai trò liệt kê ĐỦ 13 khối (không dựa vào phần bù tự chèn)', () => {
    for (const id of ['revenue', 'front_desk', 'marketing'] as const) {
      const s = presetSlots(id);
      expect([...s.top, ...s.main, ...s.side].map(([k]) => k).sort(), id).toEqual(ALL_KEYS);
    }
  });

  it('overview = bố cục chuẩn của registry; 4 mẫu khác nhau từng đôi', () => {
    expect(presetLayout('overview', CRM_DASHBOARD_WIDGETS)).toEqual(fullLayout(CRM_DASHBOARD_WIDGETS));
    const sigs = DASHBOARD_LAYOUT_PRESETS.map((p) => layoutSignature(presetLayout(p.id, CRM_DASHBOARD_WIDGETS)));
    expect(new Set(sigs).size).toBe(sigs.length);
  });

  it('khối mới thêm vào registry mà mẫu chưa liệt kê ⇒ vẫn có mặt, ở cuối vùng mặc định', () => {
    const extra = { ...CRM_DASHBOARD_WIDGETS[0], key: 'future_widget', defaultZone: 'side' as const };
    const cfg = presetLayout('revenue', [...CRM_DASHBOARD_WIDGETS, extra]);
    const side = zoneItems(cfg, 'side');
    expect(side[side.length - 1]).toEqual({ key: 'future_widget', zone: 'side', visible: true, size: 'full' });
  });

  it.each(DASHBOARD_LAYOUT_PRESETS.map((p) => p.id))('%s: registry thiếu khối tiền (gác quyền CRM) ⇒ mẫu không có khối tiền, ½ vẫn đi cặp', (id) => {
    const money = ['payment_in_month', 'payment_branch_overview', 'service_statistics'];
    const noMoney = CRM_DASHBOARD_WIDGETS.filter((w) => !money.includes(w.key));
    const cfg = presetLayout(id, noMoney);
    expect(cfg.items.map((x) => x.key).sort()).toEqual(noMoney.map((w) => w.key).sort());
    const halves = zoneItems(cfg, 'main').filter((x) => x.size === 'half');
    expect(halves.length % 2).toBe(0);
  });

  it('thẻ "Chọn bố cục khởi đầu" dựng trên registry đã lọc ⇒ hình thu nhỏ không có khối thiếu quyền', () => {
    const money = ['payment_in_month', 'payment_branch_overview', 'service_statistics'];
    const noMoney = CRM_DASHBOARD_WIDGETS.filter((w) => !money.includes(w.key));
    for (const c of presetCards(noMoney)) {
      const keys = [...c.top, ...c.main, ...c.side].map((b) => b.key);
      expect(keys.some((k) => money.includes(k)), c.id).toBe(false);
      expect(keys).toHaveLength(noMoney.length);
    }
  });
});
