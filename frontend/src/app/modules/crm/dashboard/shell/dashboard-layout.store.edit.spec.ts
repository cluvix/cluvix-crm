import { TestBed } from '@angular/core/testing';
import { Subject } from 'rxjs';
import { describe, expect, it } from 'vitest';

// ⚠ Đứng TRƯỚC base-api (vòng `auth.service` ⇄ `base-api.service` có sẵn).
import { AuthService } from '@/core/services/auth/auth.service';
import { CrmDashboardLayoutService, type DashboardLayoutView } from '@/core/services/crm/crm-dashboard-layout.service';
import { StorageService } from '@/share/services/storage.service';

import { DASHBOARD_LAYOUT_OWNER, DASHBOARD_LAYOUT_PERSIST, DASHBOARD_WIDGET_REGISTRY, DashboardLayoutStore } from './dashboard-layout.store';
import { layoutSignature, moveBy, systemDefaultLayout } from './layout.util';
import { CRM_DASHBOARD_WIDGETS } from './dashboard-widgets';

void AuthService;


type Call = { m: string; body?: unknown; res: Subject<DashboardLayoutView> };

function setup(stored: unknown = null) {
  const get = new Subject<DashboardLayoutView>();
  const calls: Call[] = [];
  const mk = (m: string) => (body?: unknown) => {
    const res = new Subject<DashboardLayoutView>();
    calls.push({ m, body, res });
    return res;
  };
  let saved: unknown = stored;
  TestBed.configureTestingModule({
    providers: [
      DashboardLayoutStore,
      { provide: DASHBOARD_LAYOUT_OWNER, useValue: '7' }, // user 7
      { provide: DASHBOARD_WIDGET_REGISTRY, useValue: CRM_DASHBOARD_WIDGETS },
      {
        provide: CrmDashboardLayoutService,
        useValue: {
          getLayout: () => get, saveLayout: mk('save'), resetLayout: mk('reset'), saveBrandDefault: mk('brand'),
          deleteBrandDefault: mk('delbrand'),
        },
      },
      { provide: StorageService, useValue: { loadStorage: () => saved, saveStorage: (_k: string, v: unknown) => (saved = v) } },
    ],
  });
  return { store: TestBed.inject(DashboardLayoutStore), get, calls, saved: () => saved as Record<string, { source: string; config: unknown }> };
}

const sys: DashboardLayoutView = { config: null, source: 'system', hasBrandDefault: false, updatedAt: 0 };
const view = (config: unknown, source: DashboardLayoutView['source']): DashboardLayoutView => ({ config, source, hasBrandDefault: false, updatedAt: 9 });
const keys = (s: DashboardLayoutStore) => s.main().map((x) => x.key);
const cached = { '7': { config: systemDefaultLayout(CRM_DASHBOARD_WIDGETS), source: 'system', updatedAt: 1 } };

describe('DashboardLayoutStore — chế độ sửa (story-10)', () => {
  it('sửa ⇒ vẽ bản nháp + dirty; Huỷ ⇒ về đúng bản trước khi sửa, hết dirty (AC5)', () => {
    const { store, get } = setup();
    get.next(sys);
    const before = layoutSignature(store.layout()!);
    store.startEdit();
    expect(store.dirty()).toBe(false);
    store.edit((c) => moveBy(c, 'quick_stats', 1));
    expect(keys(store)).toEqual(['appointment_accepted_stats', 'quick_stats']);
    expect(store.dirty()).toBe(true);
    expect(layoutSignature(store.layout()!)).toBe(before); // bản đã lưu không đổi
    store.cancelEdit();
    expect(store.dirty()).toBe(false);
    expect(keys(store)).toEqual(['quick_stats', 'appointment_accepted_stats']);
  });

  it('sửa rồi sửa ngược về như cũ ⇒ KHÔNG dirty (AC7 không hỏi oan)', () => {
    const { store, get } = setup();
    get.next(sys);
    store.startEdit();
    store.edit((c) => moveBy(c, 'quick_stats', 1));
    store.edit((c) => moveBy(c, 'quick_stats', -1));
    expect(store.dirty()).toBe(false);
  });

  it('Lưu ⇒ PUT đúng bản nháp, thoát sửa, bản tạm = bản server (AC5)', () => {
    const { store, get, calls, saved } = setup();
    get.next(sys);
    store.startEdit();
    store.edit((c) => moveBy(c, 'quick_stats', 1));
    const draft = store.view();
    store.save().subscribe();
    expect(calls.map((c) => c.m)).toEqual(['save']);
    expect(calls[0].body).toBe(draft);
    expect(store.saving()).toBe(true);
    calls[0].res.next(view(draft, 'user'));
    calls[0].res.complete();
    expect(store.editing()).toBe(false);
    expect(store.saving()).toBe(false);
    expect(keys(store)).toEqual(['appointment_accepted_stats', 'quick_stats']);
    expect(saved()['7'].source).toBe('user');
  });

  it('Lưu TRƯỚC khi GET đầu về ⇒ GET cũ về sau KHÔNG ghi đè (QA story-09)', () => {
    const { store, get, calls } = setup(cached);
    store.startEdit();
    store.edit((c) => moveBy(c, 'quick_stats', 1));
    store.save().subscribe();
    calls[0].res.next(view(store.view() ?? calls[0].body, 'user'));
    get.next(sys); // GET ban đầu về muộn
    expect(keys(store)).toEqual(['appointment_accepted_stats', 'quick_stats']);
  });

  it('GET đầu về khi đang sửa mà CHƯA đổi gì ⇒ nháp theo server, không dirty oan', () => {
    const { store, get } = setup(cached);
    store.startEdit();
    const srv = moveBy(systemDefaultLayout(CRM_DASHBOARD_WIDGETS), 'quick_stats', 1);
    get.next(view(srv, 'brand'));
    expect(store.dirty()).toBe(false);
    expect(keys(store)).toEqual(['appointment_accepted_stats', 'quick_stats']);
  });

  it('GET đầu về khi đang sửa và ĐÃ đổi ⇒ giữ nháp của user', () => {
    const { store, get } = setup(cached);
    store.startEdit();
    store.edit((c) => moveBy(c, 'quick_stats', 1));
    const d = store.view();
    get.next(view(systemDefaultLayout(CRM_DASHBOARD_WIDGETS), 'brand'));
    expect(store.view()).toBe(d);
  });

  it('Thêm khối khi XEM ⇒ hiện ngay + PUT; lỗi ⇒ trả lại như cũ (AC4)', () => {
    const { store, get, calls } = setup();
    get.next(sys);
    store.addWidget('source_overview').subscribe({ error: () => undefined });
    expect(keys(store)).toContain('source_overview');
    expect(calls.map((c) => c.m)).toEqual(['save']);
    calls[0].res.error(new Error('500'));
    expect(keys(store)).not.toContain('source_overview');
    expect(store.saving()).toBe(false);
  });

  it('Thêm khối khi SỬA ⇒ chỉ vào nháp, không gọi API', () => {
    const { store, get, calls } = setup();
    get.next(sys);
    store.startEdit();
    store.addWidget('source_overview').subscribe();
    store.hideWidget('quick_stats').subscribe();
    expect(calls).toEqual([]);
    expect(keys(store)).toEqual(['appointment_accepted_stats', 'source_overview']);
    expect(store.dirty()).toBe(true);
  });

  it('Mẫu bố cục khi XEM ⇒ đủ 13 khối + PUT; lỗi ⇒ trả lại bố cục cũ', () => {
    const { store, get, calls } = setup();
    get.next(sys);
    expect(store.serverLoaded()).toBe(true);
    store.applyPreset('marketing').subscribe({ error: () => undefined });
    expect(store.visibleCount()).toBe(13);
    expect(store.activePreset()).toBe('marketing');
    expect(calls.map((c) => c.m)).toEqual(['save']);
    calls[0].res.error(new Error('500'));
    expect(store.visibleCount()).toBe(4);
    expect(store.activePreset()).toBeNull();
  });

  it('Mẫu bố cục khi SỬA ⇒ chỉ vào nháp, không gọi API; Huỷ ⇒ về như cũ', () => {
    const { store, get, calls } = setup();
    get.next(sys);
    store.startEdit();
    store.applyPreset('front_desk').subscribe();
    expect(calls).toEqual([]);
    expect(store.activePreset()).toBe('front_desk');
    expect(store.dirty()).toBe(true);
    store.cancelEdit();
    expect(store.visibleCount()).toBe(4);
  });

  it('đang có 1 lệnh ghi ⇒ lệnh thứ 2 bị bỏ (không gọi API, không đổi state)', () => {
    const { store, get, calls } = setup();
    get.next(sys);
    store.addWidget('source_overview').subscribe();
    store.hideWidget('quick_stats').subscribe();
    store.reset().subscribe();
    expect(calls.map((c) => c.m)).toEqual(['save']);
    expect(keys(store)).toContain('quick_stats');
  });

  it('gọi mà KHÔNG subscribe ⇒ không khoá trang (defer)', () => {
    const { store, get } = setup();
    get.next(sys);
    void store.addWidget('source_overview');
    void store.reset();
    expect(store.saving()).toBe(false);
    expect(keys(store)).not.toContain('source_overview');
  });

  it('Khôi phục mặc định ⇒ DELETE, thoát sửa, hiện bố cục BE trả (AC5)', () => {
    const { store, get, calls } = setup();
    get.next(view(moveBy(systemDefaultLayout(CRM_DASHBOARD_WIDGETS), 'quick_stats', 1), 'user'));
    store.startEdit();
    store.reset().subscribe();
    expect(calls.map((c) => c.m)).toEqual(['reset']);
    calls[0].res.next(sys);
    expect(store.editing()).toBe(false);
    expect(keys(store)).toEqual(['quick_stats', 'appointment_accepted_stats']);
    expect(store.source()).toBe('system');
  });

  it('Lưu mặc định thương hiệu: admin CÓ bản riêng ⇒ ở lại chế độ sửa; KHÔNG có ⇒ thoát (AC6)', () => {
    const a = setup();
    a.get.next(sys);
    a.store.startEdit();
    a.store.edit((c) => moveBy(c, 'quick_stats', 1));
    a.store.saveBrandDefault().subscribe();
    expect(a.calls[0].m).toBe('brand');
    expect(a.calls[0].body).toBe(a.store.view());
    a.calls[0].res.next(view(a.calls[0].body, 'brand'));
    expect(a.store.editing()).toBe(false);
    expect(keys(a.store)).toEqual(['appointment_accepted_stats', 'quick_stats']);

    TestBed.resetTestingModule();
    const b = setup();
    b.get.next(view(systemDefaultLayout(CRM_DASHBOARD_WIDGETS), 'user'));
    b.store.startEdit();
    b.store.edit((c) => moveBy(c, 'quick_stats', 1));
    b.store.saveBrandDefault().subscribe();
    b.calls[0].res.next(view(systemDefaultLayout(CRM_DASHBOARD_WIDGETS), 'user'));
    expect(b.store.editing()).toBe(true);
    expect(b.store.dirty()).toBe(true);
  });

  it('Xoá mặc định thương hiệu (FR7): không có thay đổi chưa lưu ⇒ DELETE, thoát sửa, hiện bố cục BE trả', () => {
    const { store, get, calls } = setup();
    get.next({ ...view(moveBy(systemDefaultLayout(CRM_DASHBOARD_WIDGETS), 'quick_stats', 1), 'brand'), hasBrandDefault: true });
    expect(store.hasBrandDefault()).toBe(true);
    store.startEdit();
    store.deleteBrandDefault().subscribe();
    expect(calls.map((c) => c.m)).toEqual(['delbrand']);
    calls[0].res.next(sys);
    expect(store.editing()).toBe(false);
    expect(store.hasBrandDefault()).toBe(false);
    expect(store.source()).toBe('system');
    expect(keys(store)).toEqual(['quick_stats', 'appointment_accepted_stats']);
  });

  it('Xoá mặc định thương hiệu khi ĐANG có thay đổi chưa lưu ⇒ giữ nháp, ở lại chế độ sửa', () => {
    const { store, get, calls } = setup();
    get.next({ ...view(systemDefaultLayout(CRM_DASHBOARD_WIDGETS), 'brand'), hasBrandDefault: true });
    store.startEdit();
    store.edit((c) => moveBy(c, 'quick_stats', 1));
    const d = store.view();
    store.deleteBrandDefault().subscribe();
    calls[0].res.next(sys);
    expect(store.editing()).toBe(true);
    expect(store.view()).toBe(d);
    expect(store.dirty()).toBe(true);
  });

  it('Gác quyền khối (CRM): registry thiếu khối tiền ⇒ không vào bố cục/mẫu, không thêm được', () => {
    const money = ['payment_in_month', 'payment_branch_overview', 'service_statistics'];
    const noMoney = CRM_DASHBOARD_WIDGETS.filter((w) => !money.includes(w.key));
    const get = new Subject<DashboardLayoutView>();
    const calls: string[] = [];
    TestBed.configureTestingModule({
      providers: [
        DashboardLayoutStore,
        { provide: DASHBOARD_LAYOUT_OWNER, useValue: '7' },
        { provide: DASHBOARD_WIDGET_REGISTRY, useValue: noMoney },
        {
          provide: CrmDashboardLayoutService,
          useValue: { getLayout: () => get, saveLayout: () => (calls.push('save'), new Subject<DashboardLayoutView>()) },
        },
        { provide: StorageService, useValue: { loadStorage: () => null, saveStorage: () => undefined } },
      ],
    });
    const store = TestBed.inject(DashboardLayoutStore);
    // Bố cục server (vd mặc định thương hiệu do admin lưu) có khối tiền đang hiện ⇒ bị gạt khi gộp.
    get.next(view(allVisible(), 'brand'));
    const shown = () => [...store.top(), ...store.main(), ...store.side()].map((x) => x.key);
    expect(shown().some((k) => money.includes(k))).toBe(false);
    expect(store.layout()!.items.some((x) => money.includes(x.key))).toBe(false);
    expect(store.visibleCount()).toBe(noMoney.length);
    // Thêm khối tiền (vd lệnh cũ) ⇒ không đổi gì, không gọi API.
    store.addWidget('payment_branch_overview').subscribe();
    expect(calls).toEqual([]);
    // Mẫu bố cục cũng chỉ gồm khối được thấy.
    store.startEdit();
    store.applyPreset('revenue').subscribe();
    expect(store.view()!.items.map((x) => x.key).sort()).toEqual(noMoney.map((w) => w.key).sort());
    expect(store.activePreset()).toBe('revenue');
    expect(store.top()).toEqual([]);
  });

  it('Không có quyền lưu bố cục (thiếu quyền module CRM) ⇒ không gọi API, mặc định hệ thống chỉ đọc', () => {
    const calls: string[] = [];
    const rec = (m: string) => () => (calls.push(m), new Subject<DashboardLayoutView>());
    TestBed.configureTestingModule({
      providers: [
        DashboardLayoutStore,
        { provide: DASHBOARD_LAYOUT_OWNER, useValue: '7' },
        { provide: DASHBOARD_WIDGET_REGISTRY, useValue: CRM_DASHBOARD_WIDGETS },
        { provide: DASHBOARD_LAYOUT_PERSIST, useValue: false },
        { provide: CrmDashboardLayoutService, useValue: { getLayout: rec('get'), saveLayout: rec('save'), resetLayout: rec('reset') } },
        // Bản tạm (lúc còn quyền) KHÔNG được dùng.
        { provide: StorageService, useValue: { loadStorage: () => ({ '7': { config: allVisible(), source: 'user', updatedAt: 1 } }), saveStorage: () => undefined } },
      ],
    });
    const store = TestBed.inject(DashboardLayoutStore);
    expect(store.visibleCount()).toBe(4);
    store.addWidget('source_overview').subscribe();
    store.hideWidget('quick_stats').subscribe();
    store.applyPreset('revenue').subscribe();
    store.reset().subscribe();
    store.startEdit();
    expect(store.editing()).toBe(false);
    expect(calls).toEqual([]);
    expect(store.visibleCount()).toBe(4);
  });
});

function allVisible() {
  const cfg = systemDefaultLayout(CRM_DASHBOARD_WIDGETS);
  return { ...cfg, items: cfg.items.map((x) => ({ ...x, visible: true })) };
}
