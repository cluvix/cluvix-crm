import { Injector, signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Subject, of, throwError } from 'rxjs';
import { describe, expect, it, vi } from 'vitest';

import { CrmDashboardDataCache } from '@/core/services/crm/crm-dashboard-data-cache.service';
import type { DashboardQuery } from '@/core/services/crm/crm-dashboard.service';

import { DashboardFilterStore } from './dashboard-filter.store';
import { createWidgetData, widgetCacheKey } from './widget-data';

/** Store giả: chỉ cần query + refDayLabel (createWidgetData đọc 2 thứ này). */
function fakeStore(q: DashboardQuery = {}) {
  const query = signal<DashboardQuery>(q);
  const refDayLabel = signal<{ kind: 'today' } | { kind: 'date'; date: string }>({ kind: 'today' });
  return { query, refDayLabel };
}

function setup(store: ReturnType<typeof fakeStore> | null) {
  TestBed.configureTestingModule({
    providers: store ? [{ provide: DashboardFilterStore, useValue: store }] : [],
  });
  return TestBed.inject(Injector);
}

const flush = () => TestBed.tick();

describe('createWidgetData', () => {
  it('không có store (dashboard cũ) ⇒ gọi API với query rỗng, loading → ready', () => {
    const inj = setup(null);
    const loader = vi.fn(() => of({ n: 1 }));
    const d = TestBed.runInInjectionContext(() => createWidgetData('additional_stats', loader));
    expect(d.status()).toBe('loading');
    flush();
    expect(loader).toHaveBeenCalledWith({});
    expect(d.status()).toBe('ready');
    expect(d.value()).toEqual({ n: 1 });
    void inj;
  });

  it('isEmpty ⇒ empty; lỗi ⇒ error; Thử lại ⇒ gọi lại', () => {
    setup(fakeStore());
    let fail = true;
    const loader = vi.fn(() => (fail ? throwError(() => new Error('500')) : of([] as number[])));
    const d = TestBed.runInInjectionContext(() => createWidgetData<number[]>('quick_stats', loader, (v) => !v.length));
    flush();
    expect(d.status()).toBe('error');
    fail = false;
    d.retry();
    flush();
    expect(loader).toHaveBeenCalledTimes(2);
    expect(d.status()).toBe('empty');
  });

  it('đổi bộ lọc ⇒ huỷ request cũ (switchMap): response cũ tới muộn KHÔNG ghi đè', () => {
    const st = fakeStore();
    setup(st);
    const first = new Subject<string>();
    const second = new Subject<string>();
    const loader = vi.fn((q: DashboardQuery) => (q.branch_id ? second : first));
    const d = TestBed.runInInjectionContext(() => createWidgetData<string>('quick_stats', loader));
    flush();
    st.query.set({ branch_id: 2 });
    flush();
    second.next('mới');
    first.next('cũ');
    expect(d.value()).toBe('mới');
    expect(loader).toHaveBeenLastCalledWith({ branch_id: 2 });
  });

  it('cache theo khoá: dựng lại khối cùng bộ lọc ⇒ có số ngay (stale) rồi làm mới', () => {
    setup(fakeStore({ from: 1, to: 2 }));
    const cache = TestBed.inject(CrmDashboardDataCache);
    cache.clear();
    let n = 0;
    const loader = () => of(++n);
    const a = TestBed.runInInjectionContext(() => createWidgetData<number>('source_overview', loader));
    flush();
    expect(a.value()).toBe(1);
    const fresh = new Subject<number>();
    const b = TestBed.runInInjectionContext(() => createWidgetData<number>('source_overview', () => fresh));
    flush();
    expect(b.value()).toBe(1);
    expect(b.status()).toBe('ready');
    fresh.next(5);
    expect(b.value()).toBe(5);
  });

  it('Làm mới ⇒ bỏ qua cache, gọi API mới', () => {
    setup(fakeStore());
    TestBed.inject(CrmDashboardDataCache).clear();
    const loader = vi.fn(() => of(7));
    const d = TestBed.runInInjectionContext(() => createWidgetData<number>('additional_stats', loader));
    flush();
    d.refresh();
    flush();
    expect(loader).toHaveBeenCalledTimes(2);
  });

  // CRM không còn khối `snapshot` (clv_overview của ERP đã bỏ) ⇒ chỉ kiểm day + range.
  it('nhãn kỳ theo timeMode: day ⇒ ngày tham chiếu; range ⇒ null', () => {
    const st = fakeStore();
    setup(st);
    const day = TestBed.runInInjectionContext(() => createWidgetData('additional_stats', () => of(1)));
    const range = TestBed.runInInjectionContext(() => createWidgetData('source_overview', () => of(1)));
    expect(day.period()).toEqual({ key: 'common.label.today' });
    st.refDayLabel.set({ kind: 'date', date: '30/09' });
    expect(day.period()).toEqual({ key: 'dashboard.crm.filter.ref_day', params: { date: '30/09' } });
    expect(range.period()).toBeNull();
  });

  it('đổi bộ lọc sang khoá CHƯA có cache ⇒ loading (không để số kỳ cũ dưới nhãn kỳ mới — QA P1)', () => {
    const st = fakeStore();
    setup(st);
    TestBed.inject(CrmDashboardDataCache).clear();
    const pending = new Subject<string>();
    const loader = vi.fn((q: DashboardQuery) => (q.branch_id ? pending : of('A-all')));
    const d = TestBed.runInInjectionContext(() => createWidgetData<string>('additional_stats', loader));
    flush();
    expect(d.status()).toBe('ready');
    st.query.set({ branch_id: 2 });
    flush();
    expect(d.status()).toBe('loading');
    pending.next('B');
    expect(d.status()).toBe('ready');
    expect(d.value()).toBe('B');
  });

  it('Làm mới khi ĐÃ có cache mà API lỗi ⇒ error (chứng minh Làm mới bỏ qua cache — QA P6/M5)', () => {
    setup(fakeStore());
    TestBed.inject(CrmDashboardDataCache).clear();
    let fail = false;
    const loader = vi.fn(() => (fail ? throwError(() => new Error('500')) : of(1)));
    const d = TestBed.runInInjectionContext(() => createWidgetData<number>('additional_stats', loader));
    flush();
    expect(d.status()).toBe('ready');
    fail = true;
    d.refresh();
    flush();
    expect(d.status()).toBe('error');
  });
});

describe('widgetCacheKey', () => {
  it('khác khối / chi nhánh / kỳ ⇒ khác khoá (CRM 1 thương hiệu/DB ⇒ không kèm công ty)', () => {
    expect(widgetCacheKey('w', {})).not.toBe(widgetCacheKey('v', {}));
    expect(widgetCacheKey('w', { branch_id: 3 })).not.toBe(widgetCacheKey('w', {}));
    expect(widgetCacheKey('w', { from: 1, to: 2 })).not.toBe(widgetCacheKey('w', { from: 1, to: 3 }));
  });
});
