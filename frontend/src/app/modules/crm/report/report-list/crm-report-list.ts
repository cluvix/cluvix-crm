import {ChangeDetectionStrategy, Component, signal} from '@angular/core';

import {BaseApp} from '@/share/base/BaseApp';
import {PERMISSION_SELECT} from '@/define';
import {ListShareImports} from '@/share/import/list-share-imports';
import {CrmReportPanel} from '@/modules/crm/report/report-list/crm-report-panel';

export function getTableConfig(): number {
  const width = window.innerWidth;

  if (width >= 2560) return 130;
  if (width >= 1920) return 120;
  if (width >= 1512) return 110;
  if (width >= 1440) return 95;
  if (width >= 1024) return 110;
  if (width >= 768) return 95;
  if (width >= 640) return 95;

  return 95;
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-report-list',
  templateUrl: './crm-report-list.html',
  imports: [...ListShareImports, CrmReportPanel],
  standalone: true,
})
export class CrmReportList extends BaseApp {
  public permission = PERMISSION_SELECT;
  tableConfig = signal(getTableConfig());

  // Doanh thu
  revenueItems = [
    {
      label: 'Doanh thu theo ngày thu tiền',
      labelKey: 'crm.report.revenue.by_date.title',
      link: '/report/revenue/by-date',
      icon: 'pi pi-calendar',
      iconClass: 'text-blue-600',
      descriptionKey: 'crm.report.revenue.desc.by_date',
    },
    {
      label: 'Hình thức thanh toán',
      labelKey: 'crm.report.revenue.by_method.title',
      link: '/report/revenue/by-method',
      icon: 'pi pi-credit-card',
      iconClass: 'text-green-600',
      descriptionKey: 'crm.report.revenue.desc.by_method',
    },
    {
      label: 'Phiếu thu theo chi nhánh',
      labelKey: 'crm.report.revenue.branch_receipts.title',
      link: '/report/revenue/branch-receipts',
      icon: 'pi pi-sitemap',
      iconClass: 'text-orange-600',
      descriptionKey: 'crm.report.revenue.desc.branch_receipts',
    },
    {
      label: 'Giảm trừ doanh số',
      labelKey: 'crm.report.revenue.refund.title',
      link: '/report/revenue/refund-deposit-service',
      icon: 'pi pi-replay',
      iconClass: 'text-red-600',
      descriptionKey: 'crm.report.revenue.desc.refund',
    },
  ];

  // Công nợ
  debtItems = [
    {
      label: 'Công nợ theo khách hàng',
      labelKey: 'crm.report.debt.customer.title',
      link: '/report/debt/customer',
      icon: 'pi pi-user',
      iconClass: 'text-red-600',
      descriptionKey: 'crm.report.debt.desc.customer',
    },
    {
      label: 'Công nợ theo đơn hàng',
      labelKey: 'crm.report.debt.order_plan.title',
      link: '/report/debt/order-plan',
      icon: 'pi pi-shopping-cart',
      iconClass: 'text-orange-600',
      descriptionKey: 'crm.report.debt.desc.order_plan',
    },
    {
      label: 'Công nợ theo dịch vụ',
      labelKey: 'crm.report.debt.service.title',
      link: '/report/debt/service',
      icon: 'pi pi-briefcase',
      iconClass: 'text-teal-600',
      descriptionKey: 'crm.report.debt.desc.service',
    },
  ];

  // Nhân viên
  staffItems = [
    {
      label: 'Doanh số tư vấn viên',
      labelKey: 'crm.report.staff.revenue_telesale',
      link: '/report/staff/revenue-telesale',
      icon: 'pi pi-dollar',
      iconClass: 'text-blue-600',
      descriptionKey: 'crm.report.staff.desc.revenue_telesale',
    },
    {
      label: 'Tổng hợp doanh số tư vấn viên',
      labelKey: 'crm.report.staff.revenue_telesale_summary',
      link: '/report/staff/revenue-telesale-summary',
      icon: 'pi pi-chart-pie',
      iconClass: 'text-indigo-600',
      descriptionKey: 'crm.report.staff.desc.revenue_telesale_summary',
    },
    {
      label: 'Hoa hồng tư vấn viên',
      labelKey: 'crm.report.staff.commission_telesale',
      link: '/report/staff/commission-telesale',
      icon: 'pi pi-gift',
      iconClass: 'text-green-600',
      descriptionKey: 'crm.report.staff.desc.commission_telesale',
    },
    {
      label: 'Tổng hợp hoa hồng tư vấn viên',
      labelKey: 'crm.report.staff.commission_telesale_summary',
      link: '/report/staff/commission-telesale-summary',
      icon: 'pi pi-chart-bar',
      iconClass: 'text-teal-600',
      descriptionKey: 'crm.report.staff.desc.commission_telesale_summary',
    },
  ];

  // Khách hàng
  customerPortraitItems = [
    {
      label: 'Chi nhánh',
      labelKey: 'common.label.branch',
      link: '/report/branch-statistics',
      icon: 'pi pi-sitemap',
      iconClass: 'text-blue-600',
      descriptionKey: 'crm.report.desc.branch_statistics',
    },
    {
      label: 'Nguồn khách',
      labelKey: 'crm.report.customer_portrait.source',
      link: '/report/customer-source',
      icon: 'pi pi-database',
      iconClass: 'text-green-600',
      descriptionKey: 'crm.report.desc.customer_source',
    },
    {
      label: 'Tỉnh/Thành phố',
      labelKey: 'crm.report.customer_portrait.province',
      link: '/report/province-statistics',
      icon: 'pi pi-map-marker',
      iconClass: 'text-orange-600',
      descriptionKey: 'crm.report.desc.province_statistics',
    },
    {
      label: 'Quốc gia',
      labelKey: 'common.label.country',
      link: '/report/country-statistics',
      icon: 'pi pi-globe',
      iconClass: 'text-red-600',
      descriptionKey: 'crm.report.desc.country_statistics',
    },
    {
      label: 'Nhóm dịch vụ quan tâm',
      labelKey: 'crm.report.customer_portrait.interested',
      link: '/report/customer-interested',
      icon: 'pi pi-star',
      iconClass: 'text-teal-600',
      descriptionKey: 'crm.report.desc.customer_interested',
    },
    {
      label: 'Nhóm khách hàng',
      labelKey: 'crm.report.customer_portrait.category',
      link: '/report/category-statistics',
      icon: 'pi pi-users',
      iconClass: 'text-cyan-600',
      descriptionKey: 'crm.report.desc.category_statistics',
    },
    {
      label: 'Nhóm nguồn khách hàng',
      labelKey: 'common.label.customer_source_group',
      link: '/report/group-source',
      icon: 'pi pi-database',
      iconClass: 'text-cyan-600',
      descriptionKey: 'crm.report.desc.group_source',
    },
  ];

  serviceClinicItems = [
    {
      label: 'Nhóm dịch vụ đồng ý',
      labelKey: 'crm.report.service_type_stats.title',
      link: '/report/service-type',
      icon: 'pi pi-objects-column',
      iconClass: 'text-violet-600',
      descriptionKey: 'crm.report.desc.service_type',
    },
    {
      label: 'Thống kê dịch vụ',
      labelKey: 'common.label.service_statistics',
      link: '/report/service-used',
      icon: 'pi pi-chart-line',
      iconClass: 'text-teal-600',
      descriptionKey: 'crm.report.desc.treatment_services',
    },
    {
      label: 'Danh sách ghi chú',
      labelKey: 'crm.report.service.note_list',
      link: '/report/note',
      icon: 'pi pi-book',
      iconClass: 'text-cyan-600',
      descriptionKey: 'crm.report.desc.note',
    },
  ];

  // Lịch hẹn
  appointmentItems = [
    {
      label: 'Thống kê theo ngày',
      labelKey: 'crm.report.appointment.daily_statistics',
      link: '/report/appointment-statistics',
      icon: 'pi pi-chart-bar',
      iconClass: 'text-cyan-600',
      descriptionKey: 'crm.report.desc.appointment_statistics',
    },
    {
      label: 'Biểu đồ phân bổ',
      labelKey: 'crm.report.appointment.distribution_chart',
      link: '/report/appointment-chart-scatter',
      icon: 'pi pi-chart-line',
      iconClass: 'text-cyan-600',
      descriptionKey: 'crm.report.desc.appointment_chart_scatter',
    },
    {
      label: 'Lịch hẹn tư vấn',
      labelKey: 'common.label.consultation_appointments',
      link: '/report/appointment-consultation',
      icon: 'pi pi-list',
      iconClass: 'text-cyan-600',
      descriptionKey: 'crm.report.desc.appointment_consultation',
    },
  ];

  // Hiệu quả
  departmentEffectiveItems = [
    {
      label: 'Phòng tư vấn',
      labelKey: 'crm.report.department.consulting',
      link: '/report/revenue',
      icon: 'pi pi-users',
      iconClass: 'text-blue-600',
      descriptionKey: 'crm.report.desc.department_revenue',
    },
    {
      label: 'Phòng telesales online',
      labelKey: 'crm.report.department.telesales_online',
      link: '/report/telesales-online',
      icon: 'pi pi-phone',
      iconClass: 'text-green-600',
      descriptionKey: 'crm.report.desc.telesales_online',
    },
    {
      label: 'Hiệu quả lịch hẹn',
      labelKey: 'crm.report.department.appointment_effective',
      link: '/report/effective-advise',
      icon: 'pi pi-calendar',
      iconClass: 'text-cyan-600',
      descriptionKey: 'crm.report.desc.effective_advise',
    },
  ];

  // Chi nhánh
  branchItems = [
    {
      label: 'Chi nhánh x Lịch hẹn',
      labelKey: 'crm.report.branch.appointment',
      link: '/report/branch-appointment',
      icon: 'pi pi-calendar',
      iconClass: 'text-blue-600',
      descriptionKey: 'crm.report.desc.branch_appointment',
    },
  ];
}
