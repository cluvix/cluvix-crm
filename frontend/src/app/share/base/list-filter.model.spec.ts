import { describe, expect, it } from 'vitest';

import { FilterFieldDef, clearedValueOf, defaultValueOf } from './list-filter.model';

/**
 * Khoá ngữ nghĩa "Xoá bộ lọc" của `BaseListV2.clearFilter()` (user chốt 2026-11-01):
 * `default` chỉ áp lúc mở màn / bấm lại menu; xoá bộ lọc về RỖNG, trừ field `required`
 * (BE trả 422 khi thiếu) thì về `default`.
 */
describe('list-filter.model — giá trị sau khi xoá bộ lọc', () => {
  const range = () => [new Date(2026, 10, 1), new Date(2026, 10, 2)];

  it('field có default nhưng không required → xoá về rỗng (null)', () => {
    const def: FilterFieldDef = { name: 'created_at', kind: 'dateRange', default: range };
    expect(defaultValueOf(def)).toEqual(range());
    expect(clearedValueOf(def)).toBeNull();
  });

  it('field kind "in" có default → xoá về mảng rỗng', () => {
    const def: FilterFieldDef = { name: 'status', kind: 'in', default: () => [1, 2] };
    expect(clearedValueOf(def)).toEqual([]);
  });

  it('field required → xoá về default (factory được gọi lại)', () => {
    let calls = 0;
    const def: FilterFieldDef = {
      name: 'created_at',
      kind: 'dateRange',
      required: true,
      default: () => {
        calls++;
        return range();
      },
    };
    expect(clearedValueOf(def)).toEqual(range());
    expect(clearedValueOf(def)).toEqual(range());
    expect(calls).toBe(2);
  });

  it('field không có default → mặc định và giá trị xoá đều rỗng', () => {
    const def: FilterFieldDef = { name: '_keyword', kind: 'text' };
    expect(defaultValueOf(def)).toBeNull();
    expect(clearedValueOf(def)).toBeNull();
  });
});
