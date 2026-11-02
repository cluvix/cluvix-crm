import {Service} from '@angular/core';
import {HttpContext, HttpParams} from '@angular/common/http';
import {Observable} from "rxjs";
import {BaseApiService} from "@/share/services/base-api.service";
import {SKIP_GLOBAL_403, SKIP_GLOBAL_404} from '@/core/interceptors/response-handler.interceptor';

/** Tham số bộ lọc gửi BE (`crm/dashboard/handler/filter.go`). Rỗng ⇒ kỳ mặc định. */
export interface DashboardQuery {
  from?: number;
  to?: number;
  branch_id?: number;
}

/**
 * API Dashboard CRM. Mọi endpoint nhận bộ lọc chung (dashboard tuỳ biến): `from`/`to` Unix giây nửa mở [from, to),
 * `branch_id`. Query rỗng ⇒ không gửi tham số ⇒ BE dùng kỳ mặc định (tháng/ngày hiện tại).
 *
 * ⚠ Tham số PHẢI qua `{params: HttpParams}` — bản cũ truyền object thô vào vị trí `options` nên `from/to/branch_id`
 * KHÔNG BAO GIỜ được gửi (ApiService chỉ đọc `options.params`).
 */
@Service()
export class CrmDashboardService extends BaseApiService<any> {
  override apiModule = 'crm';
  override endpoint = 'crm-dashboard';

  /**
   * 403 (vd `branch_id` ngoài phạm vi — BE kiểm cho cả 14 endpoint) là lỗi CỦA KHỐI: khối tự hiện trạng thái lỗi + Thử lại
   * (`createWidgetData`), KHÔNG toast toàn cục — 14 khối cùng 403 sẽ thành 14 toast.
   */
  private opts(q?: DashboardQuery): {params?: HttpParams; context: HttpContext} {
    // 404 cũng xử lý tại khối (BE cũ chưa có endpoint ⇒ khối báo lỗi, KHÔNG đá cả trang sang notfound).
    const context = new HttpContext().set(SKIP_GLOBAL_403, true).set(SKIP_GLOBAL_404, true);
    if (!q) return {context};
    let params = new HttpParams();
    if (q.from) params = params.set('from', q.from);
    if (q.to) params = params.set('to', q.to);
    if (q.branch_id) params = params.set('branch_id', q.branch_id);
    return params.keys().length ? {params, context} : {context};
  }

  private fetch(path: string, q?: DashboardQuery): Observable<any> {
    return this.apiService.get(`${this.path}/${path}`, this.opts(q));
  }

  /** Nhắc việc trong ngày tham chiếu (CRM standalone: chỉ còn reminderTotal/reminderDone). */
  additionalStats(q?: DashboardQuery): Observable<any> {
    return this.fetch('additional-stats', q);
  }

  /** Trạng thái tư vấn của lịch hẹn trong ngày tham chiếu. */
  appointmentAcceptedStats(q?: DashboardQuery): Observable<any> {
    return this.fetch('appointment-accepted-stats', q);
  }

  /** Trạng thái lịch hẹn (khách đến). */
  reportStatistics(q?: DashboardQuery): Observable<any> {
    return this.fetch('report-statistics', q);
  }

  /** Khách đồng ý làm dịch vụ theo khu vực. */
  reportCustomerByRegion(q?: DashboardQuery): Observable<any> {
    return this.fetch('report-customer-by-region', q);
  }

  /** Dịch vụ sử dụng: số lượng + thực thu. */
  serviceUsedStatistics(q?: DashboardQuery): Observable<any> {
    return this.fetch('service-used-statistics', q);
  }

  /** Nhóm dịch vụ quan tâm. */
  overviewServiceTypeInterest(q?: DashboardQuery): Observable<any> {
    return this.fetch('overview-service-type-interest', q);
  }

  /** Nhóm dịch vụ đồng ý. */
  overviewServiceTypeAccepted(q?: DashboardQuery): Observable<any> {
    return this.fetch('overview-service-type-accepted', q);
  }

  /** Nguồn khách hàng. */
  overviewChartSource(q?: DashboardQuery): Observable<any> {
    return this.fetch('overview-chart-source', q);
  }

  /** Tiến trình khách hàng. */
  statusCustomerOverview(q?: DashboardQuery): Observable<any> {
    return this.fetch('status-customer-overview', q);
  }

  /** Lịch hẹn tư vấn. */
  overviewAppointment(q?: DashboardQuery): Observable<any> {
    return this.fetch('overview-appointment', q);
  }

  /** Tổng quan chi nhánh (tiền). */
  paymentBranchOverview(q?: DashboardQuery): Observable<any> {
    return this.fetch('payment-branch-overview', q);
  }

  /** Khách hàng mới, đơn hàng mới, lịch hẹn mới. */
  quickStats(q?: DashboardQuery): Observable<any> {
    return this.fetch('quick-starts', q);
  }

  /** Doanh thu theo kỳ. */
  paymentInMonth(q?: DashboardQuery): Observable<any> {
    return this.fetch('payment-in-month', q);
  }
}
