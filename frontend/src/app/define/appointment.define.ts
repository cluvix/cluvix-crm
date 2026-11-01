export const CONFIRM_APPOINTMENT_SELECT = {
  CONFIRM_APPOINTMENT_NOT: 0,
  CONFIRM_APPOINTMENT_OK: 1,
  CONFIRM_APPOINTMENT_NOT_OK: 2,
} as const;

export const CONFIRM_APPOINTMENT_OPTION = [
  {
    value: CONFIRM_APPOINTMENT_SELECT.CONFIRM_APPOINTMENT_NOT,
    label: 'Chưa xác nhận',
    labelKey: 'define.appointment.confirm_status.not',
    severity: 'warning',
  },
  {
    value: CONFIRM_APPOINTMENT_SELECT.CONFIRM_APPOINTMENT_OK,
    label: 'Xác nhận đến',
    labelKey: 'define.appointment.confirm_status.ok',
    severity: 'success',
  },
  {
    value: CONFIRM_APPOINTMENT_SELECT.CONFIRM_APPOINTMENT_NOT_OK,
    label: 'Xác nhận không đến',
    labelKey: 'define.appointment.confirm_status.not_ok',
    severity: 'danger',
  },
];

export type confirmAppointmentInfo = { value: number; label: string; labelKey?: string; severity: string };

export const CONFIRM_APPOINTMENT_MAP = CONFIRM_APPOINTMENT_OPTION.reduce(
  (acc, item) => {
    acc[item.value] = item;
    return acc;
  },
  {} as Record<number, confirmAppointmentInfo>,
);

export const APPOINTMENT_TYPE_SELECT = {
  APPOINTMENT_TYPE_TU_VAN: 1,
  APPOINTMENT_TYPE_TREATMENT: 2,
} as const; // Lấy ra làm define, còn lại vẫn được quản lý trong Cấu hình

export const APPOINTMENT_IS_ACCEPTED_SELECT = {
  APPOINTMENT_IS_ACCEPTED: 1,
  APPOINTMENT_IS_NOT_ACCEPTED: 2,
} as const; // Lấy ra làm define, còn lại vẫn được quản lý trong Cấu hình

export type appointmentStatusInfo = {
  value: number;
  label: string;
  labelKey?: string;
  color: string;
  background: string;
};

export const APPOINTMENT_STATUS_SELECT = {
  APPOINTMENT_STATUS_NOT_CHECKIN: 1,
  APPOINTMENT_STATUS_CHECKED_IN: 2,
  APPOINTMENT_STATUS_CHECKED_OUT: 3,
  APPOINTMENT_STATUS_CANCEL: 4,
  APPOINTMENT_STATUS_ADVISE: 5,
  APPOINTMENT_STATUS_TREATMENT: 6,
} as const;

export const APPOINTMENT_STATUS_OPTIONS = [
  {
    label: 'Chưa đến',
    labelKey: 'define.appointment.status.not_checkin',
    value: APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_NOT_CHECKIN,
    color: '#15803d',
    background: '#dcfce7',
  },
  {
    label: 'Đã đến',
    labelKey: 'common.status.arrived',
    value: APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_CHECKED_IN,
    color: '#0369a1',
    background: '#e0f2fe',
  },
  {
    label: 'Ra về',
    labelKey: 'define.appointment.status.checked_out',
    value: APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_CHECKED_OUT,
    color: '#b91c1c',
    background: '#fef3c7',
  },
  {
    label: 'Hủy',
    labelKey: 'common.status.cancelled',
    value: APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_CANCEL,
    color: '#b91c1c',
    background: '#fee2e2',
  },
  {
    label: 'Đang tư vấn',
    labelKey: 'define.appointment.status.advise',
    value: APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_ADVISE,
    color: '#92400e',
    background: '#fcd34d',
  },
  {
    label: 'Đang sử dụng',
    labelKey: 'common.label.in_treatment',
    value: APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_TREATMENT,
    color: '#166534',
    background: '#86efac',
  },
];

export const APPOINTMENT_STATUS_MAP = APPOINTMENT_STATUS_OPTIONS.reduce(
  (acc, item) => {
    acc[item.value] = item;
    return acc;
  },
  {} as Record<number, appointmentStatusInfo>,
);

export const APPOINTMENT_BOOK_TYPE_SELECT = {
  BOOK_TYPE_DAT_TRUOC: 1,
  BOOK_TYPE_PHAT_SINH: 2,
  BOOK_TYPE_DU_KIEN: 3,
} as const;

export const APPOINTMENT_BOOK_TYPE_OPTIONS = [
  { label: 'Đặt trước', labelKey: 'define.appointment.book_type.advance', value: APPOINTMENT_BOOK_TYPE_SELECT.BOOK_TYPE_DAT_TRUOC, key: 'dTr' },
  { label: 'Phát sinh', labelKey: 'common.label.walk_in', value: APPOINTMENT_BOOK_TYPE_SELECT.BOOK_TYPE_PHAT_SINH, key: 'pSi' },
  { label: 'Dự kiến', labelKey: 'common.label.planned', value: APPOINTMENT_BOOK_TYPE_SELECT.BOOK_TYPE_DU_KIEN, key: 'dKi' },
];

export const APPOINTMENT_BOOK_TYPE_MAP = APPOINTMENT_BOOK_TYPE_OPTIONS.reduce(
  (acc, item) => {
    acc[item.value] = item;
    return acc;
  },
  {} as Record<number, { value: number; label: string; labelKey?: string; key?: string }>,
);
export const APPOINTMENT_COLOR_STATUS_SELECT = {
  1: { color: 'success', background: '#fff' },
  2: { color: 'info', background: '#f5f2e1' },
  3: { color: 'warn', background: '#e1ece1' },
  4: { color: 'danger', background: '#d78d8d40' },
};
