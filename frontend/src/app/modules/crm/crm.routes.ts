import {Routes} from '@angular/router';

import {PERMISSION_SELECT} from '@/define';
import {requirePerm} from '@/core/guards/has-perm.guard';

export default [
  {
    path: '',
    loadComponent: () =>
      import('@/modules/crm/dashboard/dashboard').then((c) => c.Dashboard),
    // Bố cục đang sửa chưa lưu ⇒ hỏi trước khi rời (dashboard CRM tuỳ biến).
    canDeactivate: [(c: {confirmDiscard: () => Promise<boolean>}) => c.confirmDiscard()],
    data: {breadcrumb: null},
  },
  {
    path: 'report',
    canActivate: [requirePerm(PERMISSION_SELECT.permissionManageCrmReport)],
    loadChildren: () => import('./report/report-crm.routes'),
    data: {breadcrumb: 'common.label.report'},
  },
  {
    path: 'appointment',
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewAppointment)],
    loadChildren: () => import('./appointment/appointment.routes'),
    data: {breadcrumb: 'common.label.appointment'},
  },
  {
    path: 'customer',
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewCustomer)],
    loadChildren: () => import('./customer/customer.routes'),
    data: {preload: true, breadcrumb: 'common.label.customer_management'},
  },
  {
    path: 'order-plan',
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewPlanOrder)],
    data: {
      breadcrumb: 'layout.app_menu.order_management',
      title: 'layout.app_menu.order_management',
    },
    loadComponent: () =>
      import('@/modules/crm/order-plan/list-all/order-plan-list-all').then(
        (c) => c.OrderPlanListAll,
      ),
  },
  {
    path: 'payment',
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewPayment)],
    loadChildren: () => import('./payment/payment.routes'),
    data: {breadcrumb: 'common.label.payment'},
  },
  {
    path: 'reminder',
    canActivate: [requirePerm(PERMISSION_SELECT.permissionViewReminder)],
    data: {breadcrumb: 'common.label.reminders', title: 'common.label.reminders'},
    loadComponent: () =>
      import('@/modules/crm/reminder/list/reminder-list').then((c) => c.ReminderList),
  },
  {
    // Group Facebook (bmad/fb-group/thiet-ke.md) — quyền gác từng màn trong fbgroup.routes.
    path: 'fb-group',
    loadChildren: () => import('@/modules/fbgroup/fbgroup.routes'),
    data: {breadcrumb: 'fbgroup.menu.root'},
  },
] as Routes;
