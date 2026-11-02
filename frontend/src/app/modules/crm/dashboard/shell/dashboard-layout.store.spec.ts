import { Injector } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Subject } from 'rxjs';
import { describe, expect, it } from 'vitest';

// ⚠ Đứng TRƯỚC base-api (vòng `auth.service` ⇄ `base-api.service` có sẵn) — khuôn asset-disposal.service.spec.ts.
import { AuthService } from '@/core/services/auth/auth.service';
import { CrmDashboardLayoutService, type DashboardLayoutView } from '@/core/services/crm/crm-dashboard-layout.service';
import { StorageService } from '@/share/services/storage.service';

import { authUser, userPermissions } from '@/core/services/auth/auth.state';
import type { UserModel } from '@/core/models/hr/user.model';

import {
  DASHBOARD_LAYOUT_OWNER, DASHBOARD_WIDGET_REGISTRY, DashboardLayoutStore, provideDashboardLayoutOwner, provideDashboardWidgetRegistry,
} from './dashboard-layout.store';
import { CRM_DASHBOARD_WIDGETS } from './dashboard-widgets';

void AuthService;


function setup(stored: unknown) {
  const res = new Subject<DashboardLayoutView>();
  let saved: unknown = stored;
  TestBed.configureTestingModule({
    providers: [
      DashboardLayoutStore,
      { provide: DASHBOARD_LAYOUT_OWNER, useValue: '7' }, // user 7
      { provide: DASHBOARD_WIDGET_REGISTRY, useValue: CRM_DASHBOARD_WIDGETS },
      { provide: CrmDashboardLayoutService, useValue: { getLayout: () => res } },
      { provide: StorageService, useValue: { loadStorage: () => saved, saveStorage: (_k: string, v: unknown) => (saved = v) } },
    ],
  });
  const store = TestBed.inject(DashboardLayoutStore);
  return { store, res, saved: () => saved };
}

const userCfg = { version: 1, items: [{ key: 'customer_region', zone: 'top', visible: true, size: 'full' }] };

describe('DashboardLayoutStore', () => {
  it('không có bản tạm ⇒ layout null (skeleton) tới khi API về; API system ⇒ 4 khối mặc định (AC1, AC3)', () => {
    const { store, res } = setup(null);
    expect(store.layout()).toBeNull();
    res.next({ config: null, source: 'system', hasBrandDefault: false, updatedAt: 0 });
    expect([...store.top(), ...store.main(), ...store.side()].map((x) => x.key)).toEqual([
      'payment_in_month', 'quick_stats', 'appointment_accepted_stats', 'overview_appointment',
    ]);
  });

  it('có bản tạm ⇒ dựng NGAY trước khi API về; API khác ⇒ thay + ghi đè bản tạm (AC2)', () => {
    const { store, res, saved } = setup({ '7': { config: userCfg, source: 'user', updatedAt: 1 } });
    expect(store.top().map((x) => x.key)).toContain('customer_region');
    res.next({ config: null, source: 'system', hasBrandDefault: false, updatedAt: 0 });
    expect(store.top().map((x) => x.key)).toEqual(['payment_in_month']);
    expect((saved() as Record<string, { source: string }>)['7'].source).toBe('system');
  });

  it('bản tạm của USER KHÁC không được dùng (AC5)', () => {
    const { store } = setup({ '8': { config: userCfg, source: 'user', updatedAt: 1 } });
    expect(store.layout()).toBeNull();
  });

  it('API lỗi: có bản tạm ⇒ giữ; không có ⇒ mặc định hệ thống', () => {
    const a = setup({ '7': { config: userCfg, source: 'user', updatedAt: 1 } });
    a.res.error(new Error('500'));
    expect(a.store.top().map((x) => x.key)).toContain('customer_region');
    TestBed.resetTestingModule();
    const b = setup(null);
    b.res.error(new Error('500'));
    expect(b.store.top().map((x) => x.key)).toEqual(['payment_in_month']);
  });

  it('bản tạm rác ⇒ như không có (AC4)', () => {
    const { store } = setup('rác');
    expect(store.layout()).toBeNull();
  });
});

describe('provideDashboardLayoutOwner (QA story-10 P2)', () => {
  it('tính lại owner MỖI injector — đăng nhập user khác trong cùng tab không dùng khoá cũ', () => {
    const prev = authUser();
    try {
      authUser.set({ id: 7, userProfile: {} } as unknown as UserModel);
      const a = Injector.create({ providers: [provideDashboardLayoutOwner()] }).get(DASHBOARD_LAYOUT_OWNER);
      authUser.set({ id: 8, userProfile: {} } as unknown as UserModel);
      const b = Injector.create({ providers: [provideDashboardLayoutOwner()] }).get(DASHBOARD_LAYOUT_OWNER);
      authUser.set(null);
      const c = Injector.create({ providers: [provideDashboardLayoutOwner()] }).get(DASHBOARD_LAYOUT_OWNER);
      expect([a, b, c]).toEqual(['7', '8', null]);
    } finally {
      authUser.set(prev);
    }
  });
});

describe('provideDashboardWidgetRegistry (gác quyền khối CRM)', () => {
  const MONEY = ['payment_in_month', 'payment_branch_overview', 'service_statistics'];
  const registryFor = (perms: string[], role = 'staff') => {
    const prevUser = authUser();
    const prevPerms = userPermissions();
    try {
      authUser.set({ id: 7, userRole: { name: role }, userProfile: {} } as unknown as UserModel);
      userPermissions.set(new Set(perms));
      return Injector.create({ providers: [provideDashboardWidgetRegistry()] }).get(DASHBOARD_WIDGET_REGISTRY).map((w) => w.key);
    } finally {
      authUser.set(prevUser);
      userPermissions.set(prevPerms);
    }
  };

  it('chỉ quyền module CRM (không viewPayment/manageCrmReport) ⇒ KHÔNG có 4 khối tiền', () => {
    const keys = registryFor(['viewCustomer']);
    expect(keys).toHaveLength(CRM_DASHBOARD_WIDGETS.length - MONEY.length);
    for (const k of MONEY) expect(keys).not.toContain(k);
  });

  it('có 1 trong 2 quyền tiền (OR) ⇒ thấy khối tiền', () => {
    expect(registryFor(['viewAppointment', 'viewPayment'])).toHaveLength(CRM_DASHBOARD_WIDGETS.length);
    expect(registryFor(['viewPlanOrder', 'manageCrmReport'])).toHaveLength(CRM_DASHBOARD_WIDGETS.length);
  });

  it('chỉ quyền tiền, không quyền module ⇒ chỉ 4 khối tiền', () => {
    expect(registryFor(['viewPayment']).sort()).toEqual([...MONEY].sort());
  });

  it('super role ⇒ đủ khối', () => {
    expect(registryFor([], 'admin')).toHaveLength(CRM_DASHBOARD_WIDGETS.length);
  });
});
