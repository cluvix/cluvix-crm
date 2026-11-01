import { VAT_RATE } from '@/define/order-vat.define';

/**
 * Nhãn mức VAT của đơn để hiển thị / in: KCT → "Không chịu thuế (KCT)" (qua i18n), còn lại `${rate}%` (kể cả 0%
 * — khác KCT về khai thuế). `null`/rỗng (đơn trước khi có VAT) → '' để nơi gọi tự ẩn dòng VAT.
 *
 * Nhận hàm dịch thay vì inject trực tiếp để dùng được cả trong component lẫn `computed`/method thường.
 */
export function vatRateLabel(rate: string | null | undefined, instant: (key: string) => string): string {
  if (rate == null || rate === '') return '';
  return rate === VAT_RATE.KCT ? instant('define.vat_rate.kct') : `${rate}%`;
}
