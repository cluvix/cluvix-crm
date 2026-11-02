import type { Type } from '@angular/core';

import { CRM_MODULE_PERMISSIONS, PERMISSION_SELECT } from '@/define';

/**
 * Registry 13 khối Dashboard CRM (port từ ERP epic crm-dashboard-customizable story-05).
 * Hợp đồng chung với BE: `key` khớp allowlist `define.CrmDashboardWidgetKeys` (BE KHÔNG giữ vùng/thứ tự/cỡ).
 * So với ERP (20 khối) KHÔNG có: clv_overview, marketing_view, estimate_price_marketing, event_overview (CRM standalone
 * không có nguồn dữ liệu), customer_status_marketing, convert_online (user chốt 2026-10-19 bỏ 2 khối marketing).
 * Vùng/thứ tự/cỡ mặc định = BỐ CỤC CHUẨN khi bật đủ khối: nhóm doanh thu → lịch hẹn → dịch vụ; khối biểu đồ ½ đi CẶP
 * cùng chiều cao để cột chính và cột phụ cao xấp xỉ nhau. 5 khối bật mặc định đứng đầu vùng của mình.
 *
 * `perms` (CRM, ERP không có): khối chỉ dành cho user có ÍT NHẤT 1 quyền trong danh sách — khớp `route_perm_map.json`
 * BE của endpoint khối gọi. Thiếu quyền ⇒ khối bị lọc khỏi registry của trang (`allowedWidgets`) ⇒ không vào bố cục,
 * Kho khối, mẫu bố cục; không tải chunk, không gọi API (tránh 403 + toast).
 *
 * `loadComponent` là dynamic import ⇒ mỗi khối 1 chunk riêng: khối không bật thì không tải JS.
 * KHÔNG import tĩnh component ở file này (phá code-split).
 */
export type DashboardZone = 'top' | 'main' | 'side';
export type DashboardSize = 'half' | 'full';
/** range = theo kỳ · day = ngày tham chiếu · mixed = kỳ + ngày tham chiếu · snapshot = không áp kỳ */
export type WidgetTimeMode = 'range' | 'day' | 'mixed' | 'snapshot';
export type WidgetCategory = 'revenue' | 'appointment' | 'customer' | 'marketing' | 'service' | 'care';
/**
 * Màu nhấn theo nhóm khối — CHỈ lấy từ palette `ui.md` (brand/semantic/chart series), dùng qua `tint()` cho nền/viền ô
 * KPI (chữ vẫn theo token ⇒ không hụt contrast).
 */
export const WIDGET_CATEGORY_ACCENT: Record<WidgetCategory, string> = {
  revenue: '#7B3F9E', // Purple (S1)
  appointment: '#3B82F6', // Blue (S4)
  customer: '#FF5722', // Orange (S2)
  marketing: '#8B1538', // Burgundy
  service: '#F59E0B', // Amber (S5) — chỉ làm nền/viền alpha, đúng quy định ui.md
  care: '#10B981', // Green (S3)
};
/** Nhãn nhóm (tiêu đề khối + Kho khối) — dùng lại `common.label.*`. */
export const WIDGET_CATEGORY_LABEL: Record<WidgetCategory, string> = {
  revenue: 'common.label.revenue',
  appointment: 'common.label.appointment',
  customer: 'common.label.customer',
  marketing: 'common.label.marketing',
  service: 'common.label.service',
  care: 'common.label.care',
};
/** Icon nhóm cạnh tiêu đề khối (PrimeIcons). */
export const WIDGET_CATEGORY_ICON: Record<WidgetCategory, string> = {
  revenue: 'pi pi-wallet',
  appointment: 'pi pi-calendar',
  customer: 'pi pi-users',
  marketing: 'pi pi-megaphone',
  service: 'pi pi-star',
  care: 'pi pi-heart',
};
export type WidgetThumb = 'kpi' | 'bar' | 'donut' | 'funnel' | 'list' | 'map';
export type WidgetSkeleton = 'kpi' | 'chart' | 'list';

export interface DashboardWidgetDef {
  key: string;
  titleKey: string;
  descKey: string;
  category: WidgetCategory;
  thumb: WidgetThumb;
  loadComponent: () => Promise<Type<unknown>>;
  defaultZone: DashboardZone;
  defaultSize: DashboardSize;
  defaultVisible: boolean;
  /** Thứ tự trong vùng mặc định (bắt đầu từ 1). */
  defaultOrder: number;
  skeleton: WidgetSkeleton;
  timeMode: WidgetTimeMode;
  /**
   * Chiều cao vùng biểu đồ (px) — NGUỒN DUY NHẤT cho cả vùng vẽ trong khối lẫn khung xương lúc tải (host + frame),
   * để khung không nhảy giữa các pha tải. Chỉ khối `skeleton: 'chart'`.
   */
  chartHeight?: number;
  /** Route báo cáo chi tiết (menu "…"). Chỉ gắn khi đã chốt ánh xạ khối ↔ báo cáo — KHÔNG đoán. */
  reportRoute?: string;
  /** Cần ÍT NHẤT 1 quyền trong danh sách (OR, khớp BE route_perm_map). Không khai ⇒ ai vào được trang cũng thấy. */
  perms?: readonly string[];
}

/** 3 khối tiền: BE `/payment-in-month|payment-branch-overview|service-used-statistics` = viewPayment OR manageCrmReport. */
export const DASHBOARD_MONEY_PERMS: readonly string[] = [
  PERMISSION_SELECT.permissionViewPayment,
  PERMISSION_SELECT.permissionManageCrmReport,
];
/** Khối còn lại: BE `/api/crm/crm-dashboard/*` = viewAppointment OR viewCustomer OR viewPlanOrder (CRM_MODULE_PERMISSIONS). */
const MODULE_PERMS: readonly string[] = CRM_MODULE_PERMISSIONS;

const t = (key: string) => `dashboard.crm.widgets.${key}.title`;
const d = (key: string) => `dashboard.crm.widgets.${key}.desc`;

// Mảng nguồn khai kiểu TƯỜNG MINH: thiếu kiểu thì literal union (`category`, `defaultZone`…) bị nới thành `string`
// ⇒ TS2322 ở chỗ gán.
const RAW: readonly Omit<DashboardWidgetDef, 'titleKey' | 'descKey'>[] = [
  { key: 'payment_in_month', category: 'revenue', thumb: 'kpi', defaultZone: 'top', defaultOrder: 1, defaultSize: 'full', defaultVisible: true, skeleton: 'kpi', timeMode: 'range',
    perms: DASHBOARD_MONEY_PERMS, loadComponent: () => import('../component/payment-in-month').then((m) => m.PaymentInMonth) },
  { key: 'quick_stats', category: 'customer', thumb: 'kpi', defaultZone: 'main', defaultOrder: 1, defaultSize: 'full', defaultVisible: true, skeleton: 'kpi', timeMode: 'mixed',
    perms: MODULE_PERMS, loadComponent: () => import('../component/quick-stats').then((m) => m.QuickStats) },
  { key: 'appointment_accepted_stats', category: 'appointment', thumb: 'list', defaultZone: 'main', defaultOrder: 2, defaultSize: 'full', defaultVisible: true, skeleton: 'list', timeMode: 'day',
    perms: MODULE_PERMS, loadComponent: () => import('../component/appointment-accepted-stats').then((m) => m.AppointmentAcceptedStats) },
  { key: 'payment_branch_overview', category: 'revenue', thumb: 'list', defaultZone: 'main', defaultOrder: 3, defaultSize: 'full', defaultVisible: false, skeleton: 'list', timeMode: 'range',
    perms: DASHBOARD_MONEY_PERMS, loadComponent: () => import('../component/payment-branch-overview').then((m) => m.PaymentBranchOverview) },
  { key: 'service_statistics', chartHeight: 500, category: 'service', thumb: 'bar', defaultZone: 'main', defaultOrder: 4, defaultSize: 'full', defaultVisible: false, skeleton: 'chart', timeMode: 'range',
    perms: DASHBOARD_MONEY_PERMS, loadComponent: () => import('../component/chart-service-statistics').then((m) => m.ChartServiceStatistics) },
  // Cỡ đầy đủ (ERP: ½ đi cặp với event_overview — CRM không có khối đó): biểu đồ tròn lồng + chú thích cần bề ngang.
  { key: 'source_overview', chartHeight: 320, category: 'marketing', thumb: 'donut', defaultZone: 'main', defaultOrder: 5, defaultSize: 'full', defaultVisible: false, skeleton: 'chart', timeMode: 'range',
    perms: MODULE_PERMS, loadComponent: () => import('../component/chart-source-overview').then((m) => m.ChartSourceOverview) },
  { key: 'service_type_accepted', chartHeight: 320, category: 'service', thumb: 'donut', defaultZone: 'main', defaultOrder: 6, defaultSize: 'half', defaultVisible: false, skeleton: 'chart', timeMode: 'range',
    perms: MODULE_PERMS, loadComponent: () => import('../component/chart-service-type-overview').then((m) => m.ChartServiceTypeOverview) },
  { key: 'service_type_interest', chartHeight: 320, category: 'service', thumb: 'bar', defaultZone: 'main', defaultOrder: 7, defaultSize: 'half', defaultVisible: false, skeleton: 'chart', timeMode: 'range',
    perms: MODULE_PERMS, loadComponent: () => import('../component/chart-service-type-interest').then((m) => m.ChartServiceTypeInterest) },
  { key: 'overview_appointment', category: 'appointment', thumb: 'kpi', defaultZone: 'side', defaultOrder: 2, defaultSize: 'full', defaultVisible: true, skeleton: 'kpi', timeMode: 'mixed',
    perms: MODULE_PERMS, loadComponent: () => import('../component/overview-appointment').then((m) => m.OverviewAppointment) },
  // CRM: BE chỉ còn nhắc việc trong NGÀY tham chiếu (CSKH/CTV/coupon/liên hệ đã bỏ) ⇒ `day`, không phải `mixed` như ERP.
  { key: 'additional_stats', category: 'care', thumb: 'kpi', defaultZone: 'side', defaultOrder: 3, defaultSize: 'full', defaultVisible: false, skeleton: 'kpi', timeMode: 'day',
    perms: MODULE_PERMS, loadComponent: () => import('../component/additional-stats').then((m) => m.AdditionalStats) },
  { key: 'appointment_status', chartHeight: 400, category: 'appointment', thumb: 'donut', defaultZone: 'side', defaultOrder: 4, defaultSize: 'full', defaultVisible: false, skeleton: 'chart', timeMode: 'range',
    perms: MODULE_PERMS, loadComponent: () => import('../component/chart-appointment-status').then((m) => m.ChartAppointmentStatus) },
  { key: 'customer_status_overview', chartHeight: 400, category: 'customer', thumb: 'funnel', defaultZone: 'side', defaultOrder: 5, defaultSize: 'full', defaultVisible: false, skeleton: 'chart', timeMode: 'range',
    perms: MODULE_PERMS, loadComponent: () => import('../component/customer-status-chart-overview').then((m) => m.CustomerStatusChartOverview) },
  { key: 'customer_region', chartHeight: 400, category: 'customer', thumb: 'map', defaultZone: 'side', defaultOrder: 6, defaultSize: 'full', defaultVisible: false, skeleton: 'chart', timeMode: 'range',
    perms: MODULE_PERMS, loadComponent: () => import('../component/chart-customer-region').then((m) => m.ChartCustomerRegion) },
];

export const CRM_DASHBOARD_WIDGETS: readonly DashboardWidgetDef[] = RAW.map((w) => ({ ...w, titleKey: t(w.key), descKey: d(w.key) }));

export const CRM_DASHBOARD_WIDGET_MAP: ReadonlyMap<string, DashboardWidgetDef> = new Map(
  CRM_DASHBOARD_WIDGETS.map((w) => [w.key, w]),
);

/** Chiều cao vùng biểu đồ của khối (px) — xem `DashboardWidgetDef.chartHeight`. */
export function widgetChartHeight(key: string): number | null {
  return CRM_DASHBOARD_WIDGET_MAP.get(key)?.chartHeight ?? null;
}

/**
 * Các khối user được thấy (thuần — test được): khối không khai `perms` luôn qua; khai thì cần `canAny(perms)`.
 * Giữ nguyên thứ tự registry.
 */
export function allowedWidgets(
  registry: readonly DashboardWidgetDef[],
  canAny: (perms: readonly string[]) => boolean,
): readonly DashboardWidgetDef[] {
  return registry.filter((w) => !w.perms?.length || canAny(w.perms));
}
