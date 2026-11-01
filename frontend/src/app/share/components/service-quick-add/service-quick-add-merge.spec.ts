import { describe, expect, it } from 'vitest';

import { ServiceModel } from '@/core/models/config/clinic/service.model';
import { SERVICE_DISCOUNT_TYPE_SELECT } from '@/define';

import type { ServiceQuickAddEvent } from './service-quick-add';
import { findQuickAddMergeIndex, moveMergedRowToTop } from './service-quick-add-merge';

const PERCENT = SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT;
const CURRENCY = SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_CURRENCY;

const event = (patch: Partial<ServiceQuickAddEvent> = {}): ServiceQuickAddEvent => ({
  service: { id: 7, name: 'Gội đầu' } as ServiceModel,
  quantity: 2,
  unit_price: 100_000,
  service_discount: 0,
  service_discount_type: PERCENT,
  ...patch,
});

const row = (patch: Partial<Record<string, any>> = {}) => ({
  service_id: 7,
  unit_price: 100_000,
  service_discount: 0,
  service_discount_type: PERCENT,
  discount_other: null,
  quantity: 1,
  ...patch,
});

/** Luật cộng dồn của khung "Thêm dịch vụ" — gộp sai dòng là sai số lượng (và tiền) của đơn. */
describe('findQuickAddMergeIndex', () => {
  it('cùng dịch vụ, cùng giá, cùng chiết khấu, không Giảm khác ⇒ gộp vào dòng ĐẦU TIÊN khớp', () => {
    expect(findQuickAddMergeIndex([row({ service_id: 1 }), row(), row()], event())).toBe(1);
  });

  it('khác dịch vụ / khác đơn giá / dòng có Giảm khác ⇒ thêm dòng mới (-1)', () => {
    expect(findQuickAddMergeIndex([row({ service_id: 8 })], event())).toBe(-1);
    expect(findQuickAddMergeIndex([row({ unit_price: 120_000 })], event())).toBe(-1);
    expect(findQuickAddMergeIndex([row({ discount_other: 5_000 })], event())).toBe(-1);
  });

  it('chiết khấu: cùng % thì gộp; tiền (đ) khác 0 không gộp; cả hai = 0 thì gộp bất kể loại', () => {
    expect(findQuickAddMergeIndex([row({ service_discount: 10 })], event({ service_discount: 10 }))).toBe(0);
    expect(findQuickAddMergeIndex([row({ service_discount: 10 })], event({ service_discount: 5 }))).toBe(-1);
    expect(
      findQuickAddMergeIndex(
        [row({ service_discount: 10_000, service_discount_type: CURRENCY })],
        event({ service_discount: 10_000, service_discount_type: CURRENCY }),
      ),
    ).toBe(-1);
    expect(findQuickAddMergeIndex([row({ service_discount_type: CURRENCY })], event())).toBe(0);
  });

  it('canMerge của form từ chối (dòng khoá / đã thanh toán) ⇒ bỏ qua dòng đó', () => {
    expect(findQuickAddMergeIndex([row({ payment: 1 }), row()], event(), (r: any) => !r.payment)).toBe(1);
    expect(findQuickAddMergeIndex([row()], event(), () => false)).toBe(-1);
  });

  it('dịch vụ chưa có id ⇒ -1', () => {
    expect(findQuickAddMergeIndex([row()], event({ service: {} as ServiceModel }))).toBe(-1);
  });
});

describe('moveMergedRowToTop', () => {
  it('cộng SL vào dòng gộp, đưa lên đầu, giữ thứ tự dòng khác, không mutate mảng/dòng cũ', () => {
    const rows = [row({ service_id: 1 }), row({ quantity: 3 }), row({ service_id: 2 })];
    const out = moveMergedRowToTop(rows, 1, 2);
    expect(out.map((r) => [r.service_id, r.quantity])).toEqual([[7, 5], [1, 1], [2, 1]]);
    expect(rows[1].quantity).toBe(3);
  });
});
