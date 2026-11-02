import { describe, expect, it } from 'vitest';

import { CRM_DASHBOARD_WIDGETS } from './dashboard-widgets';
import {
  type DashboardLayoutConfig, hideItem, layoutSignature, moveBy, placeItem, setItemSize, showItem, systemDefaultLayout, zoneItems,
} from './layout.util';

const base = () => systemDefaultLayout(CRM_DASHBOARD_WIDGETS);
const shown = (c: DashboardLayoutConfig, z: 'top' | 'main' | 'side') => zoneItems(c, z).filter((x) => x.visible).map((x) => x.key);
const frozen = (c: DashboardLayoutConfig) => JSON.stringify(c);

describe('layout reducers (story-10)', () => {
  it('mặc định: 4 khối hiện đúng vùng (mốc cho các ca dưới)', () => {
    const c = base();
    expect(shown(c, 'top')).toEqual(['payment_in_month']);
    expect(shown(c, 'main')).toEqual(['quick_stats', 'appointment_accepted_stats']);
    expect(shown(c, 'side')).toEqual(['overview_appointment']);
  });

  it('placeItem cùng vùng: index = currentIndex của CDK (lên đầu / xuống cuối)', () => {
    const c = base();
    expect(shown(placeItem(c, 'appointment_accepted_stats', 'main', 0), 'main')).toEqual(['appointment_accepted_stats', 'quick_stats']);
    expect(shown(placeItem(c, 'quick_stats', 'main', 1), 'main')).toEqual(['appointment_accepted_stats', 'quick_stats']);
    expect(shown(placeItem(c, 'quick_stats', 'main', 0), 'main')).toEqual(['quick_stats', 'appointment_accepted_stats']);
  });

  it('placeItem sang vùng khác: chèn đúng vị trí, rời vùng cũ, số khối không đổi', () => {
    const c = placeItem(base(), 'overview_appointment', 'main', 1);
    expect(shown(c, 'main')).toEqual(['quick_stats', 'overview_appointment', 'appointment_accepted_stats']);
    expect(shown(c, 'side')).toEqual([]);
    expect(c.items.length).toBe(base().items.length);
    expect(shown(placeItem(base(), 'overview_appointment', 'top', 99), 'top')).toEqual(['payment_in_month', 'overview_appointment']);
    // vùng đích rỗng
    const emptyTop = hideItem(base(), 'payment_in_month');
    expect(shown(placeItem(emptyTop, 'quick_stats', 'top', 0), 'top')).toEqual(['quick_stats']);
  });

  it('moveBy: Lên/Xuống 1 bậc; ở biên ⇒ trả ĐÚNG object cũ', () => {
    const c = base();
    expect(shown(moveBy(c, 'quick_stats', 1), 'main')).toEqual(['appointment_accepted_stats', 'quick_stats']);
    expect(shown(moveBy(c, 'appointment_accepted_stats', -1), 'main')).toEqual(['appointment_accepted_stats', 'quick_stats']);
    expect(moveBy(c, 'quick_stats', -1)).toBe(c);
    expect(moveBy(c, 'appointment_accepted_stats', 1)).toBe(c);
  });

  it('hide ⇒ mất khỏi vùng; show ⇒ về CUỐI vùng mặc định (dù trước đó ở vùng khác)', () => {
    let c = hideItem(base(), 'quick_stats');
    expect(shown(c, 'main')).toEqual(['appointment_accepted_stats']);
    c = showItem(c, 'quick_stats', CRM_DASHBOARD_WIDGETS);
    expect(shown(c, 'main')).toEqual(['appointment_accepted_stats', 'quick_stats']);
    // khối mặc định ở side, đang nằm ở main rồi bị ẩn ⇒ bật lại về side
    let d = hideItem(placeItem(base(), 'overview_appointment', 'main', 0), 'overview_appointment');
    d = showItem(d, 'overview_appointment', CRM_DASHBOARD_WIDGETS);
    expect(shown(d, 'side')).toEqual(['overview_appointment']);
    expect(shown(d, 'main')).not.toContain('overview_appointment');
    // khối mặc định ẩn (Kho khối "+")
    expect(shown(showItem(base(), 'additional_stats', CRM_DASHBOARD_WIDGETS), 'side')).toEqual(['overview_appointment', 'additional_stats']);
  });

  it('setItemSize đổi cỡ; giá trị như cũ / key lạ ⇒ trả ĐÚNG object cũ (dirty không bật oan)', () => {
    const c = base();
    expect(setItemSize(c, 'quick_stats', 'half').items.find((x) => x.key === 'quick_stats')?.size).toBe('half');
    expect(setItemSize(c, 'quick_stats', 'full')).toBe(c);
    expect(hideItem(c, 'additional_stats')).toBe(c);
    expect(showItem(c, 'quick_stats', CRM_DASHBOARD_WIDGETS)).toBe(c);
    expect(placeItem(c, 'nope', 'main', 0)).toBe(c);
  });

  it('khối ẩn nằm XEN GIỮA vùng: index chỉ tính khối hiện (QA story-10 P3)', () => {
    // Dựng tay vùng main = [quick_stats, payment_branch_overview(ẩn), appointment_accepted_stats, …] (mặc định khối ẩn chỉ ở cuối vùng).
    const mid = base();
    const qi = mid.items.findIndex((x) => x.key === 'appointment_accepted_stats');
    const [aas] = mid.items.splice(qi, 1);
    mid.items.splice(mid.items.findIndex((x) => x.key === 'payment_branch_overview') + 1, 0, aas);
    expect(zoneItems(mid, 'main').slice(0, 3).map((x) => x.key)).toEqual(['quick_stats', 'payment_branch_overview', 'appointment_accepted_stats']);
    expect(shown(mid, 'main')).toEqual(['quick_stats', 'appointment_accepted_stats']);
    // thả overview_appointment vào index 1 ⇒ ngay TRƯỚC appointment_accepted_stats (không phải trước khối ẩn rồi lệch)
    expect(shown(placeItem(mid, 'overview_appointment', 'main', 1), 'main')).toEqual(['quick_stats', 'overview_appointment', 'appointment_accepted_stats']);
    // thả vào index 2 (= số khối hiện) ⇒ CUỐI, sau appointment_accepted_stats
    expect(shown(placeItem(mid, 'overview_appointment', 'main', 2), 'main')).toEqual(['quick_stats', 'appointment_accepted_stats', 'overview_appointment']);
    // Xuống 1 bậc vượt qua khối ẩn
    expect(shown(moveBy(mid, 'quick_stats', 1), 'main')).toEqual(['appointment_accepted_stats', 'quick_stats']);
  });

  it('KHÔNG mutate đầu vào (OnPush: replace reference)', () => {
    const c = base();
    const snap = frozen(c);
    placeItem(c, 'overview_appointment', 'main', 0);
    moveBy(c, 'quick_stats', 1);
    hideItem(c, 'quick_stats');
    showItem(c, 'additional_stats', CRM_DASHBOARD_WIDGETS);
    setItemSize(c, 'quick_stats', 'half');
    expect(frozen(c)).toBe(snap);
  });

  it('chữ ký (dirty): đổi rồi đổi lại ⇒ cùng chữ ký', () => {
    const c = base();
    const back = moveBy(moveBy(c, 'quick_stats', 1), 'quick_stats', -1);
    expect(layoutSignature(back)).toBe(layoutSignature(c));
    expect(layoutSignature(moveBy(c, 'quick_stats', 1))).not.toBe(layoutSignature(c));
  });
});
