import { Routes } from '@angular/router';

export default [
  {
    path: '',
    loadComponent: () => import('./report-list/crm-report-list').then((c) => c.CrmReportList),
    data: { breadcrumb: null },
  },
  {
    path: 'service-used',
    loadComponent: () =>
      import('@/modules/crm/report/service-used/report-service-used/crm-report-service-used').then((c) => c.CrmReportServiceUsed),
    data: {
      breadcrumb: 'crm.report.treatment_services',
      title: 'crm.report.treatment_services',
    },
  },
  {
    path: 'service-type',
    loadComponent: () =>
      import('@/modules/crm/report/service-used/report-service-type/crm-report-service-type').then((c) => c.CrmReportServiceType),
    data: {
      breadcrumb: 'crm.report.service_type_stats.title',
      title: 'crm.report.service_type_stats.title',
    },
  },
  {
    path: '',
    loadChildren: () => import('./report-customer/report-customer.routes'),
  },
  {
    path: '',
    loadChildren: () => import('./report-department/report-department.routes'),
  },
  {
    path: 'appointment-statistics',
    loadComponent: () => import('@/modules/crm/report/appointment/statistics/app-statistics').then((c) => c.AppStatistics),
    data: {
      breadcrumb: 'crm.report.appointment_statistics',
      title: 'crm.report.appointment_statistics',
    },
  },
  {
    path: 'appointment-chart-scatter',
    loadComponent: () =>
      import('@/modules/crm/report/appointment/chart/chart-scatter/appointment-chart-scatter').then(
        (c) => c.AppointmentChartScatter,
      ),
    data: {
      breadcrumb: 'crm.report.appointment_chart.title',
      title: 'crm.report.appointment_chart.title',
    },
  },
  {
    path: 'note',
    loadComponent: () => import('@/modules/crm/report/note/note-list/note-list').then((c) => c.NoteList),
    data: { breadcrumb: 'common.label.note', title: 'common.label.note' },
  },
  {
    path: 'branch-appointment',
    loadComponent: () =>
      import('@/modules/crm/report/branch/branch-appointment/branch-appointment').then((c) => c.BranchAppointment),
    data: {
      breadcrumb: 'crm.report.branch.appointment',
      title: 'crm.report.branch.appointment',
    },
  },
  {
    path: 'appointment-consultation',
    loadComponent: () =>
      import('@/modules/crm/report/appointment/consultation/appointment-consultation').then(
        (c) => c.AppointmentConsultation,
      ),
    data: {
      breadcrumb: 'common.label.consultation_appointments',
      title: 'common.label.consultation_appointments',
    },
  },
  {
    path: 'revenue/by-date',
    loadComponent: () => import('@/modules/crm/report/revenue/by-date/revenue-by-date').then((c) => c.RevenueByDate),
    data: { breadcrumb: 'crm.report.revenue.by_date.title', title: 'crm.report.revenue.by_date.title' },
  },
  {
    path: 'revenue/by-method',
    loadComponent: () => import('@/modules/crm/report/revenue/by-method/revenue-by-method').then((c) => c.RevenueByMethod),
    data: { breadcrumb: 'crm.report.revenue.by_method.title', title: 'crm.report.revenue.by_method.title' },
  },
  {
    path: 'revenue/branch-receipts',
    loadComponent: () => import('@/modules/crm/report/revenue/branch-receipts/revenue-branch-receipts').then((c) => c.RevenueBranchReceipts),
    data: { breadcrumb: 'crm.report.revenue.branch_receipts.title', title: 'crm.report.revenue.branch_receipts.title' },
  },
  {
    path: 'revenue/refund-deposit-service',
    loadComponent: () => import('@/modules/crm/report/revenue/refund-deposit-service/revenue-refund-deposit-service').then((c) => c.RevenueRefundDepositService),
    data: { breadcrumb: 'crm.report.revenue.refund.title', title: 'crm.report.revenue.refund.title' },
  },
  {
    path: 'debt/customer',
    loadComponent: () => import('@/modules/crm/report/debt/customer-debts/customer-debts').then((c) => c.CustomerDebts),
    data: { breadcrumb: 'crm.report.debt.customer.title', title: 'crm.report.debt.customer.title' },
  },
  {
    path: 'debt/order-plan',
    loadComponent: () => import('@/modules/crm/report/debt/order-plan-debts/order-plan-debts').then((c) => c.OrderPlanDebts),
    data: { breadcrumb: 'crm.report.debt.order_plan.title', title: 'crm.report.debt.order_plan.title' },
  },
  {
    path: 'debt/service',
    loadComponent: () => import('@/modules/crm/report/debt/service-debts/service-debts').then((c) => c.ServiceDebts),
    data: { breadcrumb: 'crm.report.debt.service.title', title: 'crm.report.debt.service.title' },
  },
  {
    path: 'staff/revenue-telesale',
    loadComponent: () => import('@/modules/crm/report/staff/revenue-telesale/revenue-telesale').then((c) => c.CrmRevenueTelesale),
    data: { breadcrumb: 'crm.report.staff.revenue_telesale', title: 'crm.report.staff.revenue_telesale' },
  },
  {
    path: 'staff/revenue-telesale-summary',
    loadComponent: () => import('@/modules/crm/report/staff/revenue-telesale-summary/revenue-telesale-summary').then((c) => c.CrmRevenueTelesaleSummary),
    data: { breadcrumb: 'crm.report.staff.revenue_telesale_summary', title: 'crm.report.staff.revenue_telesale_summary' },
  },
  {
    path: 'staff/commission-telesale',
    loadComponent: () => import('@/modules/crm/report/staff/commission-telesale/commission-telesale').then((c) => c.CrmCommissionTelesale),
    data: { breadcrumb: 'crm.report.staff.commission_telesale', title: 'crm.report.staff.commission_telesale' },
  },
  {
    path: 'staff/commission-telesale-summary',
    loadComponent: () => import('@/modules/crm/report/staff/commission-telesale-summary/commission-telesale-summary').then((c) => c.CrmCommissionTelesaleSummary),
    data: { breadcrumb: 'crm.report.staff.commission_telesale_summary', title: 'crm.report.staff.commission_telesale_summary' },
  },
] as Routes;
