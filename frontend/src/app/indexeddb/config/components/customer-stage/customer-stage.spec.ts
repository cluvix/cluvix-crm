import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { CustomerStageSelectFormControl } from './customer-stage-select-form-control';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { CustomerStageModel } from '@/core/models/config/customer-stage.model';
import {
  CUSTOMER_STAGE_COLOR_CLASS,
  CUSTOMER_STAGE_COLOR_OPTIONS,
  CUSTOMER_STAGE_COLORS,
  customerStageColorClass,
} from '@/define';
import { CacheInvalidationRegistry } from '@/indexeddb/config/service/cache-invalidation-registry.service';
import { MasterDataRegistryService } from '@/indexeddb/config/service/master-data-registry.service';
import { CustomerStageCacheService } from './customer-stage-cache.service';
import { CustomerStageHelperService } from './customer-stage-helper.service';
import { buildCustomerStageOptions, sortCustomerStages } from './customer-stage-options.util';

// Dữ liệu giả: cố ý để lệch thứ tự + có bước ẩn (id 3) + hai bước trùng sort_order (id 5, 6).
const STAGES: CustomerStageModel[] = [
  { id: 2, name: 'Tư vấn', type: 1, color: 'blue', sort_order: 2, is_default: 0, status: 1 },
  { id: 1, name: 'Mới', type: 1, color: 'gray', sort_order: 1, is_default: 1, status: 1 },
  { id: 3, name: 'Tạm dừng', type: 1, color: 'amber', sort_order: 3, is_default: 0, status: 0 },
  { id: 4, name: 'Chốt', type: 2, color: 'green', sort_order: 4, is_default: 0, status: 1 },
  { id: 6, name: 'Rớt B', type: 3, color: 'red', sort_order: 5, is_default: 0, status: 1 },
  { id: 5, name: 'Rớt A', type: 3, color: 'purple', sort_order: 5, is_default: 0, status: 0 },
];

describe('buildCustomerStageOptions (AC3)', () => {
  it('form: chỉ liệt kê bước hiện theo sort_order (bước ẩn không lọt vào)', () => {
    const ids = buildCustomerStageOptions(STAGES).map((o) => o.id);
    expect(ids).toEqual([1, 2, 4, 6]);
  });

  it('form: cộng bước hiện tại của bản ghi nếu đang ẩn — đúng vị trí sort_order, hidden=true', () => {
    const opts = buildCustomerStageOptions(STAGES, { selectedIds: [3] });
    expect(opts.map((o) => o.id)).toEqual([1, 2, 3, 4, 6]);
    expect(opts.find((o) => o.id === 3)?.hidden).toBe(true);
    expect(opts.find((o) => o.id === 2)?.hidden).toBe(false);
  });

  it('form: id đang chọn không còn trong cache thì bỏ qua, không throw', () => {
    expect(buildCustomerStageOptions(STAGES, { selectedIds: [999] }).map((o) => o.id)).toEqual([
      1, 2, 4, 6,
    ]);
  });

  it('bộ lọc (includeHidden): liệt kê MỌI bước, bước ẩn đánh dấu hidden', () => {
    const opts = buildCustomerStageOptions(STAGES, { includeHidden: true });
    expect(opts.map((o) => o.id)).toEqual([1, 2, 3, 4, 5, 6]);
    expect(opts.filter((o) => o.hidden).map((o) => o.id)).toEqual([3, 5]);
  });

  it('sort_order trùng nhau thì theo id; không đổi mảng nguồn', () => {
    const before = STAGES.map((s) => s.id);
    const sorted = sortCustomerStages(STAGES).map((s) => s.id);
    expect(sorted).toEqual([1, 2, 3, 4, 5, 6]);
    expect(STAGES.map((s) => s.id)).toEqual(before);
  });
});

describe('customerStageColorClass (map khoá màu → class)', () => {
  it('đủ 8 khoá màu, mỗi khoá có badge + dot', () => {
    expect(CUSTOMER_STAGE_COLORS.length).toBe(8);
    for (const c of CUSTOMER_STAGE_COLORS) {
      expect(CUSTOMER_STAGE_COLOR_CLASS[c].badge).toContain(c);
      expect(CUSTOMER_STAGE_COLOR_CLASS[c].dot).toContain(c);
    }
  });

  it('CUSTOMER_STAGE_COLOR_OPTIONS = đúng tập khoá CUSTOMER_STAGE_COLORS, cùng thứ tự', () => {
    expect(CUSTOMER_STAGE_COLOR_OPTIONS.map((o) => o.value)).toEqual([...CUSTOMER_STAGE_COLORS]);
  });

  it('khoá lạ/rỗng/null ⇒ xám, không throw', () => {
    const gray = CUSTOMER_STAGE_COLOR_CLASS.gray;
    expect(customerStageColorClass('hotpink')).toEqual(gray);
    expect(customerStageColorClass('')).toEqual(gray);
    expect(customerStageColorClass(null)).toEqual(gray);
    expect(customerStageColorClass(undefined)).toEqual(gray);
  });
});

describe('CustomerStageHelperService / CacheService với registry giả (AC2, AC3)', () => {
  const data = signal<CustomerStageModel[]>(STAGES);
  let helper: CustomerStageHelperService;
  let cache: CustomerStageCacheService;

  beforeEach(() => {
    data.set(STAGES);
    const fakeTable = { getAll: () => data.asReadonly(), isLoaded: () => true };
    TestBed.configureTestingModule({
      providers: [
        { provide: MasterDataRegistryService, useValue: { getTableService: () => fakeTable } },
        { provide: CacheInvalidationRegistry, useValue: { register: () => {}, invalidate: () => {} } },
      ],
    });
    helper = TestBed.inject(CustomerStageHelperService);
    cache = TestBed.inject(CustomerStageCacheService);
  });

  it('getName/getColor/getType đúng theo stage_id — kể cả bước đã ẩn', () => {
    expect(helper.getName(2)).toBe('Tư vấn');
    expect(helper.getColor(2)).toBe('blue');
    expect(helper.getType(4)).toBe(2);
    expect(helper.getName(3)).toBe('Tạm dừng');
    expect(helper.isHidden(3)).toBe(true);
    expect(helper.isHidden(2)).toBe(false);
  });

  it('id không có trong cache ⇒ rỗng/"-"/null, không throw', () => {
    expect(helper.getName(null)).toBe('');
    expect(helper.getName(undefined)).toBe('');
    expect(helper.getName(0)).toBe('');
    expect(helper.getName(999)).toBe('-');
    expect(helper.getColor(999)).toBe('');
    expect(helper.getType(999)).toBeNull();
    expect(helper.getBadgeClass(999)).toBe(CUSTOMER_STAGE_COLOR_CLASS.gray.badge);
  });

  it('getName đổi theo khi dữ liệu registry đổi (sync xong thấy tên mới, không đóng băng)', () => {
    expect(helper.getName(2)).toBe('Tư vấn');
    data.set(STAGES.map((s) => (s.id === 2 ? { ...s, name: 'Tư vấn mới' } : s)));
    expect(helper.getName(2)).toBe('Tư vấn mới');
  });

  it('cache.getAll chỉ bước hiện; getAllUnfiltered đủ cả ẩn; cả hai theo sort_order', () => {
    expect(cache.getAll().map((s) => s.id)).toEqual([1, 2, 4, 6]);
    expect(cache.getAllUnfiltered().map((s) => s.id)).toEqual([1, 2, 3, 4, 5, 6]);
  });
});

describe('CustomerStageCacheService.ensureCustomerStagesLoaded — store mới tạo còn rỗng (AC1)', () => {
  function setup(initial: CustomerStageModel[]) {
    const data = signal<CustomerStageModel[]>(initial);
    const invalidateAndSync = vi.fn(async () => {
      data.set(STAGES);
    });
    const fakeTable = { getAll: () => data.asReadonly(), isLoaded: () => true };
    TestBed.configureTestingModule({
      providers: [
        {
          provide: MasterDataRegistryService,
          useValue: {
            getTableService: () => fakeTable,
            loadTableOnDemand: async () => {},
            invalidateAndSync,
          },
        },
        { provide: CacheInvalidationRegistry, useValue: { register: () => {}, invalidate: () => {} } },
      ],
    });
    return { cache: TestBed.inject(CustomerStageCacheService), invalidateAndSync };
  }

  it('store rỗng ⇒ tự đồng bộ đúng 1 lần dù gọi đồng thời nhiều nơi', async () => {
    const { cache, invalidateAndSync } = setup([]);
    await Promise.all([cache.ensureCustomerStagesLoaded(), cache.ensureCustomerStagesLoaded()]);
    expect(invalidateAndSync).toHaveBeenCalledTimes(1);
    expect(invalidateAndSync).toHaveBeenCalledWith('customerStages');
    expect(cache.allUnfiltered().length).toBe(STAGES.length);
  });

  it('store đã có dữ liệu ⇒ KHÔNG đồng bộ thêm', async () => {
    const { cache, invalidateAndSync } = setup(STAGES);
    await cache.ensureCustomerStagesLoaded();
    expect(invalidateAndSync).not.toHaveBeenCalled();
  });
});

describe('CustomerStageSelectFormControl — giữ bước ẩn đang chọn (AC3, nối writeValue → options)', () => {
  function build() {
    const fakeTable = { getAll: () => signal(STAGES).asReadonly(), isLoaded: () => true };
    TestBed.configureTestingModule({
      providers: [
        { provide: MasterDataRegistryService, useValue: { getTableService: () => fakeTable } },
        { provide: CacheInvalidationRegistry, useValue: { register: () => {}, invalidate: () => {} } },
      ],
    });
    // `options` là protected — đọc qua ép kiểu, test chỉ quan tâm giá trị.
    return TestBed.runInInjectionContext(() => new CustomerStageSelectFormControl()) as unknown as {
      writeValue(v: number | null): void;
      options(): { id: number; hidden: boolean }[];
    };
  }

  it('writeValue(id bước ẩn) ⇒ options có bước đó, đánh dấu hidden', () => {
    const ctrl = build();
    ctrl.writeValue(3);
    const hiddenOpt = ctrl.options().find((o) => o.id === 3);
    expect(hiddenOpt).toBeDefined();
    expect(hiddenOpt?.hidden).toBe(true);
  });

  it('chưa chọn bước ẩn ⇒ options KHÔNG có bước ẩn', () => {
    const ctrl = build();
    ctrl.writeValue(2);
    expect(ctrl.options().some((o) => o.id === 3)).toBe(false);
    ctrl.writeValue(null);
    expect(ctrl.options().map((o) => o.id)).toEqual([1, 2, 4, 6]);
  });
});
