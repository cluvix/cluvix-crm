import { describe, expect, it } from 'vitest';

import { allowedWidgets, CRM_DASHBOARD_WIDGETS } from './dashboard-widgets';
import { fullLayout, layoutSignature, mergeLayout, systemDefaultLayout, zoneItems } from './layout.util';

/** Bảng khối chuẩn = bố cục khi bật đủ 13 khối CRM ("Hiện tất cả khối") — lệch là ĐỎ. Cột cuối: khối tiền (gác quyền). */
const EXPECTED: [key: string, zone: string, order: number, visible: boolean, timeMode: string, category: string, size: string][] = [
  ['payment_in_month', 'top', 1, true, 'range', 'revenue', 'full'],
  ['quick_stats', 'main', 1, true, 'mixed', 'customer', 'full'],
  ['appointment_accepted_stats', 'main', 2, true, 'day', 'appointment', 'full'],
  ['payment_branch_overview', 'main', 3, false, 'range', 'revenue', 'full'],
  ['service_statistics', 'main', 4, false, 'range', 'service', 'full'],
  ['source_overview', 'main', 5, false, 'range', 'marketing', 'full'],
  ['service_type_accepted', 'main', 6, false, 'range', 'service', 'half'],
  ['service_type_interest', 'main', 7, false, 'range', 'service', 'half'],
  ['overview_appointment', 'side', 2, true, 'mixed', 'appointment', 'full'],
  ['additional_stats', 'side', 3, false, 'day', 'care', 'full'],
  ['appointment_status', 'side', 4, false, 'range', 'appointment', 'full'],
  ['customer_status_overview', 'side', 5, false, 'range', 'customer', 'full'],
  ['customer_region', 'side', 6, false, 'range', 'customer', 'full'],
];
const MONEY_KEYS = ['payment_in_month', 'payment_branch_overview', 'service_statistics'];

describe('CRM_DASHBOARD_WIDGETS registry', () => {
  it('đúng 13 khối, khớp từng dòng bảng khối chuẩn', () => {
    expect(CRM_DASHBOARD_WIDGETS).toHaveLength(13);
    const got = CRM_DASHBOARD_WIDGETS.map((w) => [w.key, w.defaultZone, w.defaultOrder, w.defaultVisible, w.timeMode, w.category, w.defaultSize]);
    expect(got).toEqual(EXPECTED);
  });
  it('key duy nhất, mỗi khối có loadComponent + i18n key riêng', () => {
    expect(new Set(CRM_DASHBOARD_WIDGETS.map((w) => w.key)).size).toBe(13);
    for (const w of CRM_DASHBOARD_WIDGETS) {
      expect(typeof w.loadComponent).toBe('function');
      expect(w.titleKey).toBe(`dashboard.crm.widgets.${w.key}.title`);
      expect(w.descKey).toBe(`dashboard.crm.widgets.${w.key}.desc`);
    }
  });
  it('mọi khối có perms khớp BE route_perm_map: 4 khối tiền = viewPayment|manageCrmReport, còn lại = quyền module CRM', () => {
    for (const w of CRM_DASHBOARD_WIDGETS) {
      expect(w.perms?.length, w.key).toBeGreaterThan(0);
      expect([...w.perms!].sort(), w.key).toEqual(
        MONEY_KEYS.includes(w.key) ? ['manageCrmReport', 'viewPayment'] : ['viewAppointment', 'viewCustomer', 'viewPlanOrder'],
      );
    }
  });
  it('allowedWidgets: lọc theo OR quyền, giữ thứ tự registry; khối không khai perms luôn qua', () => {
    const only = (granted: string[]) => (perms: readonly string[]) => perms.some((p) => granted.includes(p));
    expect(allowedWidgets(CRM_DASHBOARD_WIDGETS, only(['viewCustomer'])).map((w) => w.key)).toEqual(
      CRM_DASHBOARD_WIDGETS.map((w) => w.key).filter((k) => !MONEY_KEYS.includes(k)),
    );
    expect(allowedWidgets(CRM_DASHBOARD_WIDGETS, only(['manageCrmReport'])).map((w) => w.key)).toEqual(MONEY_KEYS);
    expect(allowedWidgets(CRM_DASHBOARD_WIDGETS, only([]))).toEqual([]);
    const open = { ...CRM_DASHBOARD_WIDGETS[0], key: 'open_widget', perms: undefined };
    expect(allowedWidgets([open], only([])).map((w) => w.key)).toEqual(['open_widget']);
  });
});

describe('systemDefaultLayout', () => {
  it('đủ 13 khối, đúng 4 khối bật mặc định (vừa 1 màn hình)', () => {
    const cfg = systemDefaultLayout(CRM_DASHBOARD_WIDGETS);
    expect(cfg.items).toHaveLength(13);
    expect(cfg.items.filter((x) => x.visible).map((x) => x.key)).toEqual([
      'payment_in_month', 'quick_stats', 'appointment_accepted_stats', 'overview_appointment',
    ]);
    expect(zoneItems(cfg, 'top').map((x) => x.key)).toEqual(['payment_in_month']);
  });
});

describe('fullLayout — "Hiện tất cả khối"', () => {
  it('bật đủ 13 khối, đúng vị trí/cỡ của bảng khối chuẩn', () => {
    const cfg = fullLayout(CRM_DASHBOARD_WIDGETS);
    expect(cfg.items.every((x) => x.visible)).toBe(true);
    expect(cfg.items.map((x) => [x.key, x.zone, x.size])).toEqual(EXPECTED.map(([k, z, , , , , sz]) => [k, z, sz]));
  });
  it('cặp ½ trong cột chính luôn đi đôi (không bỏ trống nửa hàng)', () => {
    const main = zoneItems(fullLayout(CRM_DASHBOARD_WIDGETS), 'main');
    let run = 0;
    for (const x of main) {
      if (x.size === 'half') run++;
      else {
        expect(run % 2).toBe(0);
        run = 0;
      }
    }
    expect(run % 2).toBe(0);
  });
  it('5 khối mặc định giữ nguyên thứ tự tương đối như bố cục mặc định', () => {
    const vis = systemDefaultLayout(CRM_DASHBOARD_WIDGETS).items.filter((x) => x.visible).map((x) => x.key);
    const full = fullLayout(CRM_DASHBOARD_WIDGETS).items.map((x) => x.key).filter((k) => vis.includes(k));
    expect(full).toEqual(vis);
  });
});

describe('mergeLayout', () => {
  const sys = systemDefaultLayout(CRM_DASHBOARD_WIDGETS);
  it('null / sai hình dạng / khác phiên bản ⇒ mặc định hệ thống', () => {
    for (const bad of [null, undefined, 'x', 42, {}, { version: 2, items: [] }, { version: 1, items: 'x' }]) {
      expect(layoutSignature(mergeLayout(bad, CRM_DASHBOARD_WIDGETS))).toBe(layoutSignature(sys));
    }
  });
  it('bỏ key lạ + key trùng (giữ lần đầu), giữ thứ tự đã lưu', () => {
    const out = mergeLayout(
      { version: 1, items: [
        { key: 'additional_stats', zone: 'main', visible: true, size: 'half' },
        { key: 'hacked', zone: 'main', visible: true, size: 'full' },
        { key: 'additional_stats', zone: 'side', visible: false, size: 'full' },
        { key: 'quick_stats', zone: 'main', visible: false, size: 'full' },
      ] },
      CRM_DASHBOARD_WIDGETS,
    );
    expect(out.items.find((x) => x.key === 'hacked')).toBeUndefined();
    expect(out.items.filter((x) => x.key === 'additional_stats')).toEqual([{ key: 'additional_stats', zone: 'main', visible: true, size: 'half' }]);
    const main = zoneItems(out, 'main').map((x) => x.key);
    expect(main.slice(0, 2)).toEqual(['additional_stats', 'quick_stats']);
    expect(out.items).toHaveLength(13);
  });
  it('vùng/cỡ/visible sai kiểu ⇒ lấy mặc định của khối', () => {
    const out = mergeLayout({ version: 1, items: [{ key: 'customer_region', zone: 'left', visible: 'yes', size: 'third' }] }, CRM_DASHBOARD_WIDGETS);
    expect(out.items.find((x) => x.key === 'customer_region')).toEqual({ key: 'customer_region', zone: 'side', visible: false, size: 'full' });
  });
  it('khối mới chưa có trong bản lưu ⇒ chèn cuối vùng mặc định, theo defaultVisible', () => {
    const out = mergeLayout(
      { version: 1, items: [
        { key: 'quick_stats', zone: 'main', visible: true, size: 'full' },
        { key: 'overview_appointment', zone: 'side', visible: true, size: 'full' },
      ] },
      CRM_DASHBOARD_WIDGETS,
    );
    expect(out.items).toHaveLength(13);
    const side = zoneItems(out, 'side').map((x) => x.key);
    expect(side[0]).toBe('overview_appointment');
    expect(side).toContain('customer_region');
    expect(out.items.find((x) => x.key === 'payment_in_month')).toEqual({ key: 'payment_in_month', zone: 'top', visible: true, size: 'full' });
    expect(out.items.find((x) => x.key === 'source_overview')!.visible).toBe(false);
  });
  it('bản lưu rỗng (ẩn hết rồi lưu) vẫn bổ sung đủ khối theo mặc định', () => {
    const out = mergeLayout({ version: 1, items: [] }, CRM_DASHBOARD_WIDGETS);
    expect(layoutSignature(out)).toBe(layoutSignature(sys));
  });
});

describe('mergeLayout — gác quyền khối (CRM)', () => {
  it('registry đã lọc quyền ⇒ khối tiền trong bản lưu bị gạt, kể cả đang hiện', () => {
    const noMoney = CRM_DASHBOARD_WIDGETS.filter((w) => !MONEY_KEYS.includes(w.key));
    const out = mergeLayout(fullLayout(CRM_DASHBOARD_WIDGETS), noMoney);
    expect(out.items.map((x) => x.key)).toEqual(noMoney.map((w) => w.key));
    expect(zoneItems(out, 'top')).toEqual([]);
  });
  it('khối bị gỡ khỏi CRM (vd clv_overview, convert_online của ERP) trong bản lưu ⇒ bỏ', () => {
    const out = mergeLayout({ version: 1, items: [
      { key: 'clv_overview', zone: 'side', visible: true, size: 'full' },
      { key: 'convert_online', zone: 'side', visible: true, size: 'full' },
    ] }, CRM_DASHBOARD_WIDGETS);
    expect(out.items.map((x) => x.key)).not.toContain('clv_overview');
    expect(out.items.map((x) => x.key)).not.toContain('convert_online');
    expect(out.items).toHaveLength(13);
  });
});
