export const CUSTOMER_STATUS = {
  STOP_CARE: 1,
  TICKET: 2,
  OLD_SYSTEM: 3,
  LEAD: 4,
  DEPOSIT: 5,
  APPOINTMENT: 6,
  CANCEL_APPOINTMENT: 7,
  ARRIVED: 8,
  DISAGREE: 9,
  AGREED: 10,
  IN_TREATMENT: 11,
  COMPLETED: 12,
  COMPLAINT: 13,
  COMPLAINT_RESOLVED: 14,
  COLLABORATOR: 15,
  CONTACT: 16,
  ZALO: 17,
} as const;

export const CUSTOMER_STATUS_OPTIONS = [
  { value: CUSTOMER_STATUS.STOP_CARE, label: 'Dừng chăm sóc', labelKey: 'define.customer_status.stop_care' },
  { value: CUSTOMER_STATUS.TICKET, label: 'Ticket', labelKey: 'common.label.ticket' },
  { value: CUSTOMER_STATUS.OLD_SYSTEM, label: 'Phần mềm cũ', labelKey: 'define.customer_status.old_system' },
  { value: CUSTOMER_STATUS.LEAD, label: 'Lead', labelKey: 'common.label.lead' },
  { value: CUSTOMER_STATUS.DEPOSIT, label: 'Đặt cọc', labelKey: 'common.label.deposit' },
  { value: CUSTOMER_STATUS.APPOINTMENT, label: 'Đặt hẹn', labelKey: 'define.customer_status.appointment' },
  { value: CUSTOMER_STATUS.CANCEL_APPOINTMENT, label: 'Huỷ hẹn', labelKey: 'define.customer_status.cancel_appointment' },
  { value: CUSTOMER_STATUS.ARRIVED, label: 'Đã đến', labelKey: 'common.status.arrived' },
  { value: CUSTOMER_STATUS.DISAGREE, label: 'Không đồng ý', labelKey: 'common.label.disagreed' },
  { value: CUSTOMER_STATUS.AGREED, label: 'Khách đồng ý', labelKey: 'common.label.agreed' },
  { value: CUSTOMER_STATUS.IN_TREATMENT, label: 'Đang sử dụng', labelKey: 'common.label.in_treatment' },
  { value: CUSTOMER_STATUS.COMPLETED, label: 'Hoàn thành', labelKey: 'common.status.completed' },
  { value: CUSTOMER_STATUS.COMPLAINT, label: 'Khiếu nại', labelKey: 'common.label.complaint' },
  { value: CUSTOMER_STATUS.COMPLAINT_RESOLVED, label: 'Hoàn thành khiếu nại', labelKey: 'define.customer_status.complaint_resolved' },
  { value: CUSTOMER_STATUS.COLLABORATOR, label: 'Cộng tác viên', labelKey: 'define.customer_status.collaborator' },
  { value: CUSTOMER_STATUS.CONTACT, label: 'Liên hệ', labelKey: 'common.label.contact' },
  { value: CUSTOMER_STATUS.ZALO, label: 'Khách Zalo', labelKey: 'define.customer_status.zalo' },
];

export const getCustomerStatusLabel = (value: number): string => {
  const option = CUSTOMER_STATUS_OPTIONS.find((opt) => opt.value === value);
  return option?.label || 'Không xác định';
};

// i18n key resolver — returns translation key for runtime language switching.
// Templates: {{ item.statusKey ? (item.statusKey | translate) : item.statusLabel }}
export const getCustomerStatusKey = (value: number): string => {
  return CUSTOMER_STATUS_OPTIONS.find((o) => o.value === value)?.labelKey ?? '';
};

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
