export const CUSTOMER_TYPE_SELECT = {
  CUSTOMER_TYPE_NEW: 1,
  CUSTOMER_TYPE_OLD: 2,
} as const;

export const CUSTOMER_TYPE_OPTION = [
  { value: CUSTOMER_TYPE_SELECT.CUSTOMER_TYPE_NEW, label: 'Khách hàng mới', labelKey: 'common.label.new_customer' },
  { value: CUSTOMER_TYPE_SELECT.CUSTOMER_TYPE_OLD, label: 'Khách hàng cũ', labelKey: 'define.customer_type.old' },
];

export const CUSTOMER_TYPE_LABELS = {
  [CUSTOMER_TYPE_SELECT.CUSTOMER_TYPE_NEW]: 'Khách hàng mới',
  [CUSTOMER_TYPE_SELECT.CUSTOMER_TYPE_OLD]: 'Khách hàng cũ',
} as const;

export const getCustomerTypeLabel = (value: number): string => {
  return CUSTOMER_TYPE_LABELS[value as keyof typeof CUSTOMER_TYPE_LABELS] || 'Không xác định';
};

// i18n key resolver — returns translation key for runtime language switching.
// Templates: {{ item.customerTypeKey ? (item.customerTypeKey | translate) : item.customerTypeLabel }}
export const getCustomerTypeKey = (value: number): string => {
  return CUSTOMER_TYPE_OPTION.find((o) => o.value === value)?.labelKey ?? '';
};

export const CUSTOMER_CONNECT_SELECT = {
  CONNECTING: 1,
  DISCONECT: 2,
} as const;

export const CUSTOMER_CONNECT_OPTION = [
  { value: CUSTOMER_CONNECT_SELECT.CONNECTING, label: 'Đã kết nối', labelKey: 'common.label.connected' },
  { value: CUSTOMER_CONNECT_SELECT.DISCONECT, label: 'Chưa kết nối', labelKey: 'define.customer_connect.disconnected' },
];
export const CUSTOMER_CONNECTED_LABELS = {
  [CUSTOMER_CONNECT_SELECT.CONNECTING]: 'Đã kết nối',
  [CUSTOMER_CONNECT_SELECT.DISCONECT]: 'Chưa kết nối',
} as const;

export const getCustomerConnectLabel = (value: number): string => {
  return (
    CUSTOMER_CONNECTED_LABELS[value as keyof typeof CUSTOMER_CONNECTED_LABELS] || 'Không xác định'
  );
};

// i18n key resolver — returns translation key for runtime language switching.
// Templates: {{ item.customerConnectKey ? (item.customerConnectKey | translate) : item.customerConnectLabel }}
export const getCustomerConnectKey = (value: number): string => {
  return CUSTOMER_CONNECT_OPTION.find((o) => o.value === value)?.labelKey ?? '';
};

export const CUSTOMER_DATA = {
  NEW: 1,
  OLD: 2,
} as const;

export const CUSTOMER_DATA_LABELS = {
  [CUSTOMER_DATA.NEW]: 'Dữ liệu mới',
  [CUSTOMER_DATA.OLD]: 'Dữ liệu cũ',
} as const;

export const getCustomerDataLabel = (value: number): string => {
  return CUSTOMER_DATA_LABELS[value as keyof typeof CUSTOMER_DATA_LABELS] || 'Không xác định';
};

export const CUSTOMER_LEAD_TYPY_SELECT = {
  ZMA: 1,
  WEBSITE: 2,
} as const;
