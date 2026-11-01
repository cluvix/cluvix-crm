import { PaymentListSubPath } from '@/core/services/crm/payment.service';

/**
 * Cấu hình 6 màn danh sách phiếu (port từ ERP `accountant/payment/list-*`) — dùng chung 1 component
 * `PaymentListAll`, route truyền `data.kind`.
 *
 * `value` của cột = khoá `?columns=` của hàm export BE tương ứng (`backend/.../export/payment.go`),
 * KHÔNG đổi tuỳ tiện: lệch tên ⇒ cột đó biến mất khỏi file Excel mà không báo lỗi. Vì cùng một ý
 * nghĩa mà mỗi màn ERP đặt `value` khác nhau (`customer_id`/`fullname`, `branch_id`/`branch`…), cách
 * vẽ ô tách riêng ở `cell`.
 *
 * Khác ERP: bỏ cột Thương hiệu (1 thương hiệu/DB), Page facebook, Chiến dịch, Bác sĩ tư vấn, Direct
 * sale, % hoàn thành (schema CRM không có) và nút xuất hoá đơn.
 */
export type PaymentListKind = 'payment' | 'receipts' | 'deposit' | 'refund-deposit' | 'refund-service';

/** Cách vẽ 1 ô — template `payment-list-all.html` @switch theo giá trị này. */
export type PaymentCell =
  | 'id'
  | 'customerCode'
  | 'customerOldCode'
  | 'customer'
  | 'phone'
  | 'code'
  | 'orderCode'
  | 'oseCode'
  | 'serviceCode'
  | 'service'
  | 'quantity'
  | 'totalEnd'
  | 'vatAmount'
  | 'totalAfterVat'
  | 'serviceType'
  | 'statusPayment'
  | 'type'
  | 'method'
  | 'cardType'
  | 'typeDeposit'
  | 'amount'
  | 'amountRefund'
  | 'depositAfter'
  | 'depositCode'
  | 'depositNote'
  | 'contentOld'
  | 'note'
  | 'revenue'
  | 'sourceCustomer'
  | 'sourcePayment'
  | 'serviceInterested'
  | 'province'
  | 'country'
  | 'address'
  | 'createdOrder'
  | 'paymentOfDate'
  | 'paymentTime'
  | 'appointmentDate'
  | 'branch'
  | 'createdAt'
  | 'createdBy'
  | 'updatedBy'
  | 'updatedAt';

export interface PaymentListCol {
  /** Khoá cột (selectedColumns + `?columns=` export). */
  value: string;
  /** AppColumnToggle đọc `labelKey || label` qua pipe translate. */
  label: string;
  labelKey: string;
  cell: PaymentCell;
  /** Lớp min-width của `<th>`. */
  width: string;
  /** Ô dòng tổng: 'sum' = tổng số tiền, 'depositAfter' = tổng còn lại. */
  total?: 'sum' | 'depositAfter';
}

/** Bộ lọc khai báo được trong popover (tên = tên filter BE). */
export type PaymentFilterName =
  | 'branch_id'
  | 'service_type'
  | 'service_id'
  | 'source_id'
  | '_listRevenues'
  | 'method'
  | 'card_type_id'
  | 'payment_for'
  | 'deposit_type'
  | '_groupLocal'
  | '_groupCountry'
  | 'created_time';

export interface PaymentListKindConfig {
  subPath: PaymentListSubPath;
  urlPrefix: string;
  lsColumnsKey: string;
  /** Sort mặc định ('' = để base giữ `-id`). */
  defaultSort: string;
  /** Ô ngày chính trên thanh công cụ (mặc định "đầu tháng → hôm nay"). */
  dateField: 'payment_time' | 'payment_of_date';
  dateLabelKey: string;
  filters: PaymentFilterName[];
  /** Field tổng trong response BE. */
  sumField: string;
  depositAfterField?: string;
  /** Cột chứa lối vào "Chi tiết" (nên là cột BẬT mặc định). */
  detailCol: string;
  exportTooltipKey: string;
  cols: PaymentListCol[];
  selected: string[];
}

const col = (value: string, labelKey: string, cell: PaymentCell, width = 'min-w-[140px]', total?: PaymentListCol['total']): PaymentListCol => ({
  value,
  label: labelKey,
  labelKey,
  cell,
  width,
  total,
});

// Cột "thông tin hệ thống" cuối bảng — dùng chung mọi màn.
const AUDIT_COLS = (branchValue: string): PaymentListCol[] => [
  col(branchValue, 'common.label.branch', 'branch'),
  col('created_at', 'common.label.created_at', 'createdAt'),
  col('created_by', 'common.label.user_created', 'createdBy', 'min-w-[200px]'),
  col('updated_by', 'common.label.user_updated', 'updatedBy', 'min-w-[200px]'),
  col('updated_at', 'common.label.updated_at', 'updatedAt'),
];

export const PAYMENT_LIST_KINDS: Record<PaymentListKind, PaymentListKindConfig> = {
  payment: {
    subPath: '',
    urlPrefix: 'pm',
    lsColumnsKey: '/crm/payment/list-payment',
    defaultSort: '-payment_of_date',
    dateField: 'payment_of_date',
    dateLabelKey: 'common.label.payment_date',
    filters: ['branch_id', 'service_type', 'service_id', 'source_id', '_listRevenues', 'method', '_groupLocal', '_groupCountry', 'created_time'],
    sumField: 'totalSum',
    detailCol: 'service',
    exportTooltipKey: 'crm.payment_list.export_tooltip',
    cols: [
      col('id', 'ID', 'id', 'min-w-[60px]'),
      col('customer_code', 'common.label.customer_code', 'customerCode', 'min-w-[150px]'),
      col('customer_old_code', 'common.label.old_code', 'customerOldCode', 'min-w-[150px]'),
      col('customer_id', 'common.label.customer', 'customer', 'min-w-[180px]'),
      col('phone', 'common.label.phone', 'phone', 'min-w-[120px]'),
      col('payment_number', 'common.label.receipt_code', 'code', 'min-w-[120px]'),
      col('orderCode', 'common.label.order_code', 'orderCode', 'min-w-[120px]'),
      col('serviceUsedCode', 'common.label.treatment_service_code', 'oseCode', 'min-w-[150px]'),
      col('serviceCode', 'common.label.service_code', 'serviceCode', 'min-w-[100px]'),
      col('service', 'common.label.service', 'service', 'min-w-[250px]'),
      col('quantity', 'common.label.quantity', 'quantity', 'min-w-[60px]'),
      col('totalEnd', 'common.label.subtotal', 'totalEnd', 'min-w-[120px]'),
      col('vat_amount', 'common.label.vat_amount', 'vatAmount', 'min-w-[110px]'),
      col('total_after_vat', 'crm.order_plan.form.total_after_vat', 'totalAfterVat', 'min-w-[130px]'),
      col('serviceType', 'common.label.service_group', 'serviceType'),
      col('statusPayment', 'crm.payment_list.col.payment_status', 'statusPayment', 'min-w-[200px]'),
      col('type', 'crm.payment_list.col.transaction_type', 'type'),
      col('method', 'common.label.payment_method', 'method', 'min-w-[120px]'),
      col('cardType', 'crm.payment_list.col.payment_channel', 'cardType', 'min-w-[120px]'),
      col('amount', 'common.label.amount', 'amount', 'min-w-[120px]', 'sum'),
      col('code_old', 'crm.payment_list.col.original_receipt_code', 'depositCode', 'min-w-[120px]'),
      col('content_old', 'common.label.content', 'contentOld', 'min-w-[170px]'),
      col('tuVan', 'common.label.consultant', 'revenue', 'min-w-[200px]'),
      col('source', 'common.label.source', 'sourceCustomer', 'min-w-[180px]'),
      col('sourceApp', 'crm.payment_list.col.payment_source', 'sourcePayment', 'min-w-[180px]'),
      col('serviceInterested', 'common.label.service_group_of_interest', 'serviceInterested'),
      col('province', 'common.label.province', 'province'),
      col('country', 'common.label.country', 'country'),
      col('createdOrder', 'common.label.order_date', 'createdOrder'),
      col('payment_of_date', 'common.label.payment_date', 'paymentOfDate'),
      col('payment_time', 'common.label.collection_time', 'paymentTime'),
      col('address', 'crm.payment_list.col.customer_address', 'address', 'min-w-[180px]'),
      ...AUDIT_COLS('branch_id'),
    ],
    selected: ['customer_id', 'service', 'method', 'amount', 'branch_id', 'payment_of_date', 'payment_time', 'phone'],
  },
  receipts: {
    subPath: 'list-receipts',
    urlPrefix: 'rc',
    lsColumnsKey: '/crm/payment/list-receipts',
    defaultSort: '',
    dateField: 'payment_time',
    dateLabelKey: 'crm.payment_list.col.payment_time_recv',
    filters: ['payment_for', 'method', 'card_type_id', 'source_id', 'branch_id', '_listRevenues'],
    sumField: 'totalSum',
    detailCol: 'service',
    exportTooltipKey: 'crm.payment_list.export_tooltip',
    cols: [
      col('id', 'ID', 'id', 'min-w-[60px]'),
      col('customer_code', 'common.label.customer_code', 'customerCode', 'min-w-[150px]'),
      col('customer_old_code', 'common.label.old_code', 'customerOldCode', 'min-w-[150px]'),
      col('customer_id', 'common.label.customer', 'customer', 'min-w-[180px]'),
      col('phone', 'common.label.phone', 'phone', 'min-w-[120px]'),
      col('payment_number', 'crm.payment_list.col.code_recv', 'code', 'min-w-[120px]'),
      col('serviceUsedCode', 'common.label.treatment_service_code', 'oseCode', 'min-w-[150px]'),
      col('service', 'common.label.service', 'service', 'min-w-[250px]'),
      col('quantity', 'common.label.quantity', 'quantity', 'min-w-[60px]'),
      col('totalEnd', 'common.label.subtotal', 'totalEnd', 'min-w-[120px]'),
      col('vat_amount', 'common.label.vat_amount', 'vatAmount', 'min-w-[110px]'),
      col('total_after_vat', 'crm.order_plan.form.total_after_vat', 'totalAfterVat', 'min-w-[130px]'),
      col('serviceType', 'common.label.service_group', 'serviceType'),
      col('type', 'crm.payment_list.col.transaction_type', 'type'),
      col('method', 'common.label.payment_method', 'method', 'min-w-[120px]'),
      col('cardType', 'crm.payment_list.col.payment_channel', 'cardType', 'min-w-[120px]'),
      col('typeDepositLabel', 'common.label.deposit_type', 'typeDeposit'),
      col('amount', 'crm.report.revenue.amount_in_out', 'amount', 'min-w-[120px]', 'sum'),
      col('note', 'common.label.note', 'note', 'min-w-[170px]'),
      col('revenue', 'common.label.consultant', 'revenue', 'min-w-[200px]'),
      col('sourceApp', 'crm.payment_list.col.payment_source', 'sourcePayment', 'min-w-[180px]'),
      col('source', 'common.label.source', 'sourceCustomer', 'min-w-[180px]'),
      col('serviceInterested', 'common.label.service_group_of_interest', 'serviceInterested'),
      col('province', 'common.label.province', 'province'),
      col('country', 'common.label.country', 'country'),
      col('payment_time', 'crm.payment_list.col.payment_time_recv', 'paymentTime'),
      col('address', 'crm.payment_list.col.customer_address', 'address', 'min-w-[180px]'),
      ...AUDIT_COLS('branch_id'),
    ],
    selected: ['customer_id', 'payment_number', 'service', 'type', 'method', 'amount', 'branch_id', 'payment_time'],
  },
  deposit: {
    subPath: 'list-deposit',
    urlPrefix: 'dp',
    lsColumnsKey: '/crm/payment/list-deposit',
    defaultSort: '-payment_time',
    dateField: 'payment_time',
    dateLabelKey: 'crm.payment_list.col.deposit_date',
    filters: ['branch_id', 'source_id', 'deposit_type', '_groupCountry'],
    sumField: 'totalSumDeposit',
    depositAfterField: 'totalSumDepositAfter',
    detailCol: 'fullname',
    exportTooltipKey: 'crm.payment_list.export_tooltip',
    cols: [
      col('id', 'ID', 'id', 'min-w-[60px]'),
      col('customer_code', 'common.label.customer_code', 'customerCode', 'min-w-[150px]'),
      col('customer_old_code', 'common.label.old_code', 'customerOldCode', 'min-w-[150px]'),
      col('fullname', 'common.label.customer', 'customer', 'min-w-[180px]'),
      col('phone', 'common.label.phone', 'phone', 'min-w-[120px]'),
      col('payment_code', 'common.label.receipt_code', 'code', 'min-w-[120px]'),
      col('paymentMethod', 'common.label.payment_method', 'method', 'min-w-[120px]'),
      col('cardType', 'crm.payment_list.col.payment_channel', 'cardType', 'min-w-[120px]'),
      col('amount', 'common.label.deposit_amount', 'amount', 'min-w-[120px]', 'sum'),
      col('deposit_after', 'crm.payment_list.col.remaining_deposit', 'depositAfter', 'min-w-[120px]', 'depositAfter'),
      col('typeDepositLabel', 'common.label.deposit_type', 'typeDeposit'),
      col('source', 'common.label.source', 'sourceCustomer', 'min-w-[180px]'),
      col('appointment_date', 'crm.payment_list.col.appointment_date', 'appointmentDate'),
      col('province', 'common.label.province', 'province'),
      col('content', 'common.label.content', 'note', 'min-w-[170px]'),
      col('payment_time', 'crm.payment_list.col.deposit_date', 'paymentTime'),
      ...AUDIT_COLS('branch'),
    ],
    selected: ['fullname', 'paymentMethod', 'amount', 'branch', 'source', 'appointment_date', 'province', 'payment_time', 'phone'],
  },
  'refund-deposit': {
    subPath: 'list-refund-deposit',
    urlPrefix: 'rfd',
    lsColumnsKey: '/crm/payment/list-refund-deposit',
    defaultSort: '',
    dateField: 'payment_time',
    dateLabelKey: 'common.label.refund_date',
    filters: ['branch_id'],
    sumField: 'totalRefundDeposit',
    detailCol: 'customer_id',
    exportTooltipKey: 'crm.payment_list.export_tooltip',
    cols: [
      col('id', 'ID', 'id', 'min-w-[60px]'),
      col('customer_number', 'common.label.customer_code', 'customerCode', 'min-w-[150px]'),
      col('customer_old_code', 'common.label.old_code', 'customerOldCode', 'min-w-[150px]'),
      col('customer_id', 'common.label.customer', 'customer', 'min-w-[180px]'),
      col('phone', 'common.label.phone', 'phone', 'min-w-[120px]'),
      col('code', 'crm.payment_list.col.refund_deposit_code', 'code', 'min-w-[150px]'),
      col('deposit', 'common.label.refund_amount', 'amountRefund', 'min-w-[120px]', 'sum'),
      col('content', 'crm.payment_list.col.refund_deposit_content', 'note', 'min-w-[200px]'),
      col('codeDeposit', 'crm.payment_list.col.deposit_code', 'depositCode', 'min-w-[150px]'),
      col('contentDeposit', 'crm.payment_list.col.deposit_content', 'depositNote', 'min-w-[200px]'),
      col('payment_time', 'common.label.refund_date', 'paymentTime'),
      ...AUDIT_COLS('branch_id'),
    ],
    selected: ['customer_id', 'phone', 'code', 'deposit', 'branch_id'],
  },
  'refund-service': {
    subPath: 'list-refund-order-plan',
    urlPrefix: 'rfs',
    lsColumnsKey: '/crm/payment/list-refund-service',
    defaultSort: '',
    dateField: 'payment_time',
    dateLabelKey: 'common.label.refund_date',
    filters: ['branch_id'],
    sumField: 'totalRefund',
    detailCol: 'customer_id',
    exportTooltipKey: 'crm.payment_list.export_tooltip',
    cols: [
      col('id', 'ID', 'id', 'min-w-[60px]'),
      col('customer_number', 'common.label.customer_code', 'customerCode', 'min-w-[150px]'),
      col('customer_old_code', 'common.label.old_code', 'customerOldCode', 'min-w-[150px]'),
      col('customer_id', 'common.label.customer', 'customer', 'min-w-[180px]'),
      col('phone', 'common.label.phone', 'phone', 'min-w-[120px]'),
      col('serviceCode', 'common.label.treatment_service_code', 'oseCode', 'min-w-[150px]'),
      col('service', 'common.label.service', 'service', 'min-w-[250px]'),
      col('code', 'common.label.receipt_code', 'code', 'min-w-[120px]'),
      col('deposit', 'common.label.refund_amount', 'amountRefund', 'min-w-[120px]', 'sum'),
      col('note', 'common.label.content', 'note', 'min-w-[200px]'),
      col('payment_time', 'common.label.refund_date', 'paymentTime'),
      ...AUDIT_COLS('branch_id'),
    ],
    selected: ['customer_id', 'phone', 'code', 'deposit', 'branch_id'],
  },
};
