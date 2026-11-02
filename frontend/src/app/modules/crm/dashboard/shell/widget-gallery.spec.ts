import { describe, expect, it } from 'vitest';

import { CRM_DASHBOARD_WIDGETS } from './dashboard-widgets';
import { filterWidgets, galleryGroups } from './widget-gallery';

const vi: Record<string, string> = {
  'dashboard.crm.widgets.appointment_status.title': 'Trạng thái lịch hẹn',
  'dashboard.crm.widgets.overview_appointment.title': 'Tổng quan Lịch hẹn',
  'dashboard.crm.widgets.payment_in_month.title': 'Doanh thu hôm nay',
  'dashboard.crm.widgets.payment_in_month.desc': 'Đơn hàng đã thanh toán trong ngày',
};
const label = (k: string) => vi[k] ?? '';

describe('filterWidgets (Kho khối, story-10 AC4)', () => {
  it('rỗng / toàn khoảng trắng ⇒ đủ 13 khối', () => {
    expect(filterWidgets(CRM_DASHBOARD_WIDGETS, '  ', label)).toHaveLength(CRM_DASHBOARD_WIDGETS.length);
  });
  it('không dấu + không phân biệt hoa thường: "lich" khớp "Lịch hẹn"', () => {
    expect(filterWidgets(CRM_DASHBOARD_WIDGETS, 'LICH', label).map((w) => w.key).sort()).toEqual(['appointment_status', 'overview_appointment']);
  });
  it('khớp cả mô tả; "đ" ⇒ "d"', () => {
    expect(filterWidgets(CRM_DASHBOARD_WIDGETS, 'don hang', label).map((w) => w.key)).toEqual(['payment_in_month']);
    expect(filterWidgets(CRM_DASHBOARD_WIDGETS, 'đơn', label).map((w) => w.key)).toEqual(['payment_in_month']);
  });
  it('story-06: đảo thứ tự từ + tách từ AND ⇒ "nay hom" khớp "hôm nay"; "dong" khớp "Đông" không bị lẫn', () => {
    expect(filterWidgets(CRM_DASHBOARD_WIDGETS, 'nay hom', label).map((w) => w.key)).toEqual(['payment_in_month']);
    expect(filterWidgets(CRM_DASHBOARD_WIDGETS, 'dong', (k) => (k.endsWith('payment_in_month.title') ? 'Đông y' : ''))
      .map((w) => w.key)).toEqual(['payment_in_month']);
  });
  it('story-06: từ thứ hai không có thì KHÔNG khớp (AND, không phải OR)', () => {
    expect(filterWidgets(CRM_DASHBOARD_WIDGETS, 'doanh xyz', label)).toEqual([]);
  });
  it('không khớp ⇒ rỗng', () => {
    expect(filterWidgets(CRM_DASHBOARD_WIDGETS, 'zzz', label)).toEqual([]);
  });
});

describe('galleryGroups — gác quyền khối (CRM)', () => {
  it('registry thiếu khối tiền ⇒ Kho khối không có khối tiền, nhóm "Doanh thu" biến mất', () => {
    const money = ['payment_in_month', 'payment_branch_overview', 'service_statistics'];
    const noMoney = CRM_DASHBOARD_WIDGETS.filter((w) => !money.includes(w.key));
    const groups = galleryGroups(noMoney, '', label);
    const keys = groups.flatMap((g) => g.items.map((w) => w.key));
    expect(keys.sort()).toEqual(noMoney.map((w) => w.key).sort());
    expect(groups.map((g) => g.category)).not.toContain('revenue');
    expect(galleryGroups(noMoney, 'doanh thu', label)).toEqual([]);
  });
  it('đủ quyền ⇒ đủ khối, nhóm theo thứ tự cố định', () => {
    const groups = galleryGroups(CRM_DASHBOARD_WIDGETS, '', label);
    expect(groups.flatMap((g) => g.items)).toHaveLength(CRM_DASHBOARD_WIDGETS.length);
    expect(groups[0].category).toBe('revenue');
  });
});
