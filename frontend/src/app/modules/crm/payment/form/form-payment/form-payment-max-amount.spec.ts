import { describe, expect, it } from 'vitest';

import { FormPayment } from './form-payment';

/**
 * Neo R-2 (lens-money): trần số tiền thanh toán = totalEnd − payment (+ số đang lưu khi sửa).
 * `payment` là tổng thu GỘP (thanh toán + chuyển cọc), KHÔNG trừ hoàn dịch vụ — hoàn = huỷ bớt dịch vụ,
 * công nợ không đổi. Kỳ vọng viết bằng SỐ CỤ THỂ; gọi hàm sản phẩm với `this` giả (không dựng component).
 */
function maxAmount(serviceUsed: unknown, model: { id?: number; amount?: number }): number {
  const fake = { model: () => model };
  return (FormPayment.prototype.getMaxAmount as (s: unknown) => number).call(fake, serviceUsed);
}

describe('FormPayment.getMaxAmount (R-2)', () => {
  it('tạo mới: nợ = totalEnd − payment', () => {
    expect(maxAmount({ totalEnd: 10_000_000, payment: 4_000_000 }, { id: 0, amount: 0 })).toBe(6_000_000);
  });

  it('KHÔNG trừ tiền hoàn: refundServiceUsed không ảnh hưởng trần', () => {
    const su = { totalEnd: 10_000_000, payment: 4_000_000, refundServiceUsed: 3_000_000 };
    expect(maxAmount(su, { id: 0, amount: 0 })).toBe(6_000_000);
  });

  it('có VAT: trần = (totalEnd + vat_amount) − payment', () => {
    expect(maxAmount({ totalEnd: 10_000_000, vat_amount: 1_000_000, payment: 4_000_000 }, { id: 0, amount: 0 })).toBe(
      7_000_000,
    );
  });

  it('sửa phiếu: cộng lại số tiền đang lưu của chính phiếu', () => {
    expect(maxAmount({ totalEnd: 10_000_000, payment: 4_000_000 }, { id: 7, amount: 1_500_000 })).toBe(7_500_000);
  });

  it('không có dịch vụ / thiếu số liệu ⇒ 0', () => {
    expect(maxAmount(null, { id: 0, amount: 0 })).toBe(0);
    expect(maxAmount({}, { id: 0, amount: 0 })).toBe(0);
  });

  it('đã thu đủ ⇒ trần 0', () => {
    expect(maxAmount({ totalEnd: 5_000_000, payment: 5_000_000 }, { id: 0, amount: 0 })).toBe(0);
  });
});
