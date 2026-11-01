export const TYPE_DEPOSIT_SELECT = {
  TYPE_DEPOSIT_PROMOTION: 1,
  TYPE_DEPOSIT_TEMP: 2,
};

export const TYPE_DEPOSIT_OPTION = [
  { value: TYPE_DEPOSIT_SELECT.TYPE_DEPOSIT_PROMOTION, label: 'Cọc khuyến mại', labelKey: 'define.payment.type_deposit.promotion' },
  { value: TYPE_DEPOSIT_SELECT.TYPE_DEPOSIT_TEMP, label: 'Cọc tạm', labelKey: 'common.label.temporary_deposit' },
];

export const TYPE_DEPOSIT_MAP = TYPE_DEPOSIT_OPTION.reduce(
  (acc, item) => {
    acc[item.value] = item;
    return acc;
  },
  {} as Record<number, { value: number; label: string }>,
);

export const PAYMENT_FOR = {
  SERVICE_REFUND: 1,
  DEPOSIT_REFUND: 2,
  DEPOSIT: 3,
  ORDER: 4,
  TRANSFER_DEPOSIT: 5,
  FOR_CLINIC_INCOME: 7,
  FOR_CLINIC_EXPENSE: 8,
} as const;

export const PAYMENT_FOR_OPTION = [
  { value: PAYMENT_FOR.SERVICE_REFUND, label: 'Hoàn dịch vụ', labelKey: 'common.label.service_refund' },
  { value: PAYMENT_FOR.DEPOSIT_REFUND, label: 'Hoàn cọc', labelKey: 'common.label.deposit_refund' },
  { value: PAYMENT_FOR.DEPOSIT, label: 'Đặt cọc', labelKey: 'common.label.deposit' },
  { value: PAYMENT_FOR.ORDER, label: 'Thanh toán dịch vụ', labelKey: 'define.payment_for.order' },
  { value: PAYMENT_FOR.TRANSFER_DEPOSIT, label: 'Chuyển cọc', labelKey: 'common.label.transfer_deposit' },
  { value: PAYMENT_FOR.FOR_CLINIC_INCOME, label: 'Thu tiền chi nhánh', labelKey: 'common.label.clinic_income' },
  { value: PAYMENT_FOR.FOR_CLINIC_EXPENSE, label: 'Chi tiền chi nhánh', labelKey: 'common.label.clinic_expense' },
];

export const PAYMENT_FOR_MAP = PAYMENT_FOR_OPTION.reduce(
  (acc, item) => {
    acc[item.value] = item.label;
    return acc;
  },
  {} as Record<number, string>,
);

// i18n key resolver — returns translation key for runtime language switching.
// Templates: {{ item.paymentForKey ? (item.paymentForKey | translate) : item.paymentForLabel }}
export const getPaymentForKey = (value: number): string => {
  return PAYMENT_FOR_OPTION.find((o) => o.value === value)?.labelKey ?? '';
};

export const PAYMENT_FOR_CLINIC_OPTIONS = [
  { value: PAYMENT_FOR.FOR_CLINIC_INCOME, label: 'Thu tiền chi nhánh', labelKey: 'common.label.clinic_income' },
  { value: PAYMENT_FOR.FOR_CLINIC_EXPENSE, label: 'Chi tiền chi nhánh', labelKey: 'common.label.clinic_expense' },
];

export const CASH_FLOW_CATEGORY_TYPE = {
  INCOME: 1,
  EXPENSE: 2,
} as const;

export const CASH_FLOW_CATEGORY_TYPE_OPTION: { value: number; label: string; labelKey: string; severity: 'success' | 'danger' }[] = [
  { value: CASH_FLOW_CATEGORY_TYPE.INCOME, label: 'Thu', labelKey: 'accountant.cash_flow_category.income', severity: 'success' },
  { value: CASH_FLOW_CATEGORY_TYPE.EXPENSE, label: 'Chi', labelKey: 'accountant.cash_flow_category.expense', severity: 'danger' },
];
