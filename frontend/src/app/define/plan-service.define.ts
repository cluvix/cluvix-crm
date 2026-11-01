export const ORDER_PLAN_TYPE_SELECT = {
  ORDER_PLAN_TYPE_NEW: 1,
  ORDER_PLAN_TYPE_OLD: 2,
} as const;

export const ORDER_PLAN_TYPE_OPTION = [
  { value: ORDER_PLAN_TYPE_SELECT.ORDER_PLAN_TYPE_NEW, label: 'Đơn hàng mới', labelKey: 'define.order_plan_type.new' },
  { value: ORDER_PLAN_TYPE_SELECT.ORDER_PLAN_TYPE_OLD, label: 'Đơn hàng cũ', labelKey: 'define.order_plan_type.old' },
];

export const ORDER_PLAN_TYPE_MAP = ORDER_PLAN_TYPE_OPTION.reduce(
  (acc, item) => {
    acc[item.value] = item.label;
    return acc;
  },
  {} as Record<number, string>,
);
export const PAYMENT_STATUS_OPTION = [
  { value: 1, label: 'Chưa thanh toán', labelKey: 'common.status.unpaid' },
  { value: 2, label: 'Miễn phí', labelKey: 'common.label.free' },
  { value: 3, label: 'Thanh toán 1 phần', labelKey: 'common.label.partial_payment' },
  { value: 4, label: 'Hoàn thành thanh toán', labelKey: 'common.label.fully_paid' },
  { value: 5, label: 'Hoàn trả dịch vụ', labelKey: 'define.payment_status.refunded' },
];

/** `order_service_used.isLock` — khớp BE `define.ServiceUsedIsLock` (pkg/define/order_service_used.go). */
export const ORDER_SERVICE_USED_LOCK = {
  NOT_LOCK: 1,
  IS_LOCK: 2,
} as const;
