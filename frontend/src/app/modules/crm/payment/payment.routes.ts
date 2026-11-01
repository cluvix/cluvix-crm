import { Routes } from '@angular/router';

import { requirePerm } from '@/core/guards/has-perm.guard';
import { PERMISSION_SELECT } from '@/define';

/**
 * 6 màn danh sách phiếu — CÙNG một component `PaymentListAll`, khác nhau ở `data.kind`
 * (cấu hình: `list-all/payment-list-kinds.ts`). Route cha trong `crm.routes.ts`:
 * `{ path: 'payment', canActivate: [requirePerm(viewPayment)], loadChildren: () => import('./payment/payment.routes') }`.
 * Mỗi route con vẫn tự gắn `requirePerm` (như ERP gắn `permissionGuard` từng màn) để không phụ thuộc route cha.
 */
const loadPaymentListAll = () =>
  import('@/modules/crm/payment/list-all/payment-list-all').then((c) => c.PaymentListAll);

export default [
  {
    path: '',
    loadComponent: loadPaymentListAll,
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewPayment)],
    data: { kind: 'payment', breadcrumb: 'crm.payment_list.title.payment', title: 'crm.payment_list.title.payment' },
  },
  {
    path: 'receipts',
    loadComponent: loadPaymentListAll,
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewPayment)],
    data: { kind: 'receipts', breadcrumb: 'crm.payment_list.title.receipts', title: 'crm.payment_list.title.receipts' },
  },
  {
    path: 'deposit',
    loadComponent: loadPaymentListAll,
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewPayment)],
    data: { kind: 'deposit', breadcrumb: 'crm.payment_list.title.deposit', title: 'crm.payment_list.title.deposit' },
  },
  {
    path: 'refund-deposit',
    loadComponent: loadPaymentListAll,
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewPayment)],
    data: { kind: 'refund-deposit', breadcrumb: 'crm.payment_list.title.refund_deposit', title: 'crm.payment_list.title.refund_deposit' },
  },
  {
    path: 'refund-service',
    loadComponent: loadPaymentListAll,
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewPayment)],
    data: { kind: 'refund-service', breadcrumb: 'crm.payment_list.title.refund_service', title: 'crm.payment_list.title.refund_service' },
  },
] as Routes;
