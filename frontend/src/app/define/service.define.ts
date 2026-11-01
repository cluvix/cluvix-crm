export const COMMISSION_TYPE_OPTIONS = [
  { value: 1, label: 'Số tiền', labelKey: 'define.commission_type.amount' },
  { value: 2, label: '%', labelKey: 'common.label.item' },
];

export type SERVICE_CATEGORY_INFO = {
  value: number;
  label: string;
  labelKey?: string;
};
export const SERVICE_CATEGORY_SELECT = {
  SERVICE_CATEGORY_SERVICE: 1,
  SERVICE_CATEGORY_PRODUCT: 2,
  SERVICE_CATEGORY_TREATMENT: 3,
  SERVICE_CATEGORY_FEE: 4,
} as const;

export const SERVICE_CATEGORY_OPTIONS = [
  { value: SERVICE_CATEGORY_SELECT.SERVICE_CATEGORY_SERVICE, label: 'Dịch vụ', labelKey: 'common.label.service' },
  { value: SERVICE_CATEGORY_SELECT.SERVICE_CATEGORY_PRODUCT, label: 'Sản phẩm', labelKey: 'define.service.category.product' },
  { value: SERVICE_CATEGORY_SELECT.SERVICE_CATEGORY_TREATMENT, label: 'Gói dịch vụ', labelKey: 'define.service.category.treatment' },
  { value: SERVICE_CATEGORY_SELECT.SERVICE_CATEGORY_FEE, label: 'Phí tư vấn', labelKey: 'define.service.category.fee' },
];

export const SERVICE_CATEGORY_MAP = SERVICE_CATEGORY_OPTIONS.reduce(
  (acc, item) => {
    acc[item.value] = item;
    return acc;
  },
  {} as Record<number, SERVICE_CATEGORY_INFO>,
);

export const SERVICE_FOR_SELECT = {
  SERVICE_FOR_ADULT: 1,
  SERVICE_FOR_CHILD: 2,
} as const;

export const SERVICE_FOR_OPTIONS = [
  { value: SERVICE_FOR_SELECT.SERVICE_FOR_ADULT, label: 'Người lớn', labelKey: 'define.service.for.adult' },
  { value: SERVICE_FOR_SELECT.SERVICE_FOR_CHILD, label: 'Trẻ em', labelKey: 'define.service.for.child' },
];

export const EXPIRED_TYPE_SELECT = {
  EXPIRED_TYPE_DAY: 1,
  EXPIRED_TYPE_YEAR: 2,
} as const;

export const SERVICE_DISCOUNT_TYPE_SELECT = {
  SERVICE_DISCOUNT_TYPE_CURRENCY: 1, // Theo số tiền
  SERVICE_DISCOUNT_TYPE_PERCENT: 2, // Theo %
};
