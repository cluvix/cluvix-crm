import {Routes} from '@angular/router';

export default [
  {
    // In đơn hàng
    path: 'print-order/:id',
    loadComponent: () => import('@/prints/print-order/print-order').then((c) => c.PrintOrder),
  },
  {
    // In đặt cọc
    path: 'print-deposit/:id',
    loadComponent: () => import('@/prints/print-deposit/print-deposit').then((c) => c.PrintDeposit),
  },
  {
    // In thanh toán
    path: 'print-payment/:id',
    loadComponent: () => import('@/prints/print-payment/print-payment').then((c) => c.PrintPayment),
  },
  {
    // In thanh toán tuỳ chọn (query params: ?ids=1,2,3&customerId=X)
    path: 'print-multi-payment',
    loadComponent: () =>
      import('@/prints/print-multi-payment/print-multi-payment').then((c) => c.PrintMultiPayment),
  },
  {
    // In hoàn đặt cọc
    path: 'print-refund-deposit/:id',
    loadComponent: () => import('./refund-deposit/refund-deposit').then((c) => c.RefundDeposit),
  },
  {
    // In hoàn dịch vụ
    path: 'print-refund-service/:id',
    loadComponent: () =>
      import('./refund-service/refund-service').then(
        (c) => c.RefundService,
      ),
  },
  {
    path: 'print-reception-payment',
    loadComponent: () =>
      import('./print-reception-payment/print-reception-payment').then(
        (c) => c.PrintReceptionPayment,
      ),
  },
  {
    // In danh sách lịch hẹn theo chi nhánh theo giờ
    path: 'print-appointment-byhour-list',
    loadComponent: () =>
      import('./print-appointment-byhour-list/print-appointment-byhour-list').then(
        (c) => c.PrintAppointmentByhourList,
      ),
  },
  {
    // In thanh toán theo ngày
    path: 'print-payment-in-day/:datePrint',
    loadComponent: () =>
      import('./print-payment-in-day/print-payment-in-day').then((c) => c.PrintPaymentInDay),
  },
] as Routes;
