/**
 * Mức VAT của đơn hàng — `order_plan.vat_rate`, khớp BE `pkg/define/vat.go`. Giá dịch vụ CHƯA gồm VAT; VAT tính theo
 * dòng = round(totalEnd × % / 100) (`order_service_used.vat_amount`), cộng vào số phải thu. `null` = đơn trước khi có
 * VAT ⇒ không VAT.
 */
export const VAT_RATE = {
  /** Không chịu thuế — khác 0% về khai thuế, cùng 0 đồng tiền thuế. */
  KCT: 'KCT',
  RATE_0: '0',
  RATE_5: '5',
  /** Mức giảm tạm thời — chỉ cho đơn có ngày tới hết `VAT_RATE_8_LAST_DAY`. */
  RATE_8: '8',
  RATE_10: '10',
} as const;

export type VatRate = (typeof VAT_RATE)[keyof typeof VAT_RATE];

/** % thuế của từng mức (KCT ⇒ 0). */
export const VAT_RATE_PERCENT: Record<VatRate, number> = {
  [VAT_RATE.KCT]: 0,
  [VAT_RATE.RATE_0]: 0,
  [VAT_RATE.RATE_5]: 5,
  [VAT_RATE.RATE_8]: 8,
  [VAT_RATE.RATE_10]: 10,
};

export const VAT_RATE_OPTION: { value: VatRate; label: string; labelKey?: string }[] = [
  { value: VAT_RATE.KCT, label: 'Không chịu thuế', labelKey: 'define.vat_rate.kct' },
  { value: VAT_RATE.RATE_0, label: '0%' },
  { value: VAT_RATE.RATE_5, label: '5%' },
  { value: VAT_RATE.RATE_8, label: '8%' },
  { value: VAT_RATE.RATE_10, label: '10%' },
];

/**
 * Ngày cuối (yyyy-mm-dd, giờ thương hiệu) mà ngày đơn còn được chọn 8% — giảm thuế tạm thời tới hết 31/12/2026.
 * Gia hạn ⇒ sửa cùng BE `define.VatRate8LastDay`.
 */
export const VAT_RATE_8_LAST_DAY = '2026-12-31';
