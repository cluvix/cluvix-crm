import {inject, Service} from '@angular/core';
import { HttpContext } from '@angular/common/http';
import { SKIP_GLOBAL_404 } from '@/core/interceptors/response-handler.interceptor';
import {Observable} from 'rxjs';
import {map} from 'rxjs/operators';

import {ApiResponseOneModel} from '@/core/interface/ApiResponseOneModel';
import {OrderPlanModel} from '@/core/models/order-plan.model';
import {ListApiResponse} from '@/share/models/list-api-response';
import {BaseApiService} from '@/share/services/base-api.service';
import {BranchHelperService} from '@/indexeddb/config/components/branch';
import {UserHelperService} from '@/indexeddb/config/components/user';
import {SourceHelperService} from '@/indexeddb/config/components/source';
import {ServicesHelperService} from '@/indexeddb/config/components/services';

@Service()
export class OrderPlanService extends BaseApiService<OrderPlanModel> {
  public override readonly unixTimesTampFields = ['created_at', 'created_time', 'updated_at'];
  protected override endpoint = 'order-plan';
  protected override dateFields = [];
  private servicesHelper = inject(ServicesHelperService);
  private sourceHelper: SourceHelperService = inject(SourceHelperService);
  private userHelper: UserHelperService = inject(UserHelperService);
  private branchHelper: BranchHelperService = inject(BranchHelperService);

  /**
   * Khởi tạo giá trị ban đầu.
   *
   * PHẢI trả ĐỦ KEY cho mọi field `OrderPlanForm` bind, giá trị rỗng dùng `null`/`[]`/`''` — Signal
   * Forms coi field `undefined` là "không tồn tại" và `rootField.x` trả `undefined` VĨNH VIỄN
   * (gotcha #1/#8 `skill/frontend/signal-forms.md`). `coupon` bind `<input pInputText>` nên seed `''`,
   * KHÔNG `null` (gotcha #9 — input text có giá trị `null` bị Signal Forms coi là field số).
   * Chỉ `OrderPlanForm` tiêu thụ hàm này (đã grep toàn app).
   */
  override getInitialObject(): OrderPlanModel {
    return {
      status: 1,
      created_time: this.timezoneService.now(),
      customer_id: null as any,
      appointment_id: null as any,
      branch_id: null as any,
      listRevenues: [],
      broker_id: null as any,
      source_id: null as any,
      discount_other: 0,
      total: 0,
      coupon: '',
      note: null as any,
      services_used: [],
    };
  }

  /**
   * API tạo mới đơn hàng / dịch vụ
   * @param createParams
   * @param options
   */
  createOrder(createParams: any, options: any = {}): Observable<OrderPlanModel> {
    const params = this.beforeSave(createParams);
    return this.apiService.post(`${this.path}/create-order`, params, options);
  }

  /**
   * API cập nhật đơn hàng / dịch vụ
   * @param id
   * @param updateParams
   * @param options
   */
  updateOrder(
    id: number,
    updateParams: any,
    options: any = {},
  ): Observable<ApiResponseOneModel<OrderPlanModel>> {
    const params = this.beforeSave(updateParams);
    return this.apiService.put<ApiResponseOneModel<OrderPlanModel>>(
      `${this.path}/${id}/update-order`,
      params,
      options,
    );
  }

  /**
   * Export Excel đơn hàng — `GET /crm/order-plan?_export=1`, cùng bộ lọc với danh sách (`list()` của
   * base). Port từ ERP `AccountantOrderPlanService.exportPlan` (ERP: `/accountant/order-plan`).
   */
  exportPlan(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}?_export=1`, params);
  }

  printOrderPlan(id: number): Observable<any> {
    return this.apiService.get(`print/order-plan/${id}`);
  }

  /**
   * API đổi dịch vụ (service_id) của 1 order_service_used đã có payment — order-service-change story-06.
   * Endpoint riêng biệt khỏi updateOrder (route đăng ký trong nhóm `/crm/order-plan/*`, đã gộp từ
   * `/order-treatment-plan` ở story-49 D19b).
   * @param orderServiceUsedId
   * @param payload
   */
  changeService(
    orderServiceUsedId: number,
    payload: { new_service_id: number; reason?: string; updated_at: number },
  ): Observable<ApiResponseOneModel<any>> {
    return this.apiService.put<ApiResponseOneModel<any>>(
      `crm/order-plan/service-used/${orderServiceUsedId}/change-service`,
      payload,
    );
  }

  addLabel(item: OrderPlanModel) {
    const itemWithDates = this.addFormattedTimestampFields(item, [
      'created_time',
      'created_at',
      'updated_at',
    ]);
    return {
      ...itemWithDates,
      customerDisplayName: `${item?.customer?.fullname || ''}${item?.customer?.nickname ? ' - ' + item?.customer?.nickname : ''}`,
      branchLabel: this.branchHelper.getBranchName(item?.branch_id),
      revenue: this.userHelper.getUserInfo(item.revenue_id),
      createdByInfo: this.userHelper.getUserInfo(item.created_by),
      updatedByInfo: this.userHelper.getUserInfo(item.updated_by),
      sourceLabel: this.sourceHelper.getSourceName(item.source_id),
      orderServiceUseds:
        item.orderServiceUseds?.map((service) => ({
          ...service,
          serviceInfo: this.servicesHelper.getServiceInfo(service.service_id!),
        })) || [],
    };
  }

  async addLabels(items: OrderPlanModel[]) {
    if (!Array.isArray(items)) return [];
    await this.ensureCacheLoaded();
    return items.map((item) => this.addLabel(item));
  }

  /**
   * Chi tiết đơn cho SlideOver — endpoint RIÊNG `/:id/detail`, không dùng lại `get(id)`.
   *
   * `get(id)` (tức `GET /crm/order-plan/:id`) đang trả `SELECT *` cả đơn lẫn dịch vụ con và có ≥6
   * màn khác tiêu thụ; endpoint riêng cho phép BE chỉ trả đúng cột màn này cần (và không kèm nhóm
   * hoa hồng CTV). Hình dạng response giữ nguyên tên khoá nên `addLabel` dùng lại được.
   */
  getDetail(id: number | string): Observable<OrderPlanModel> {
    return this.apiService
      .get<{ data: OrderPlanModel }>(`${this.path}/${id}/detail`, {
        // 404 ở đây = phiếu/đơn đã bị xoá — panel tự báo, KHÔNG đá cả trang sang /auth/notfound.
        context: new HttpContext().set(SKIP_GLOBAL_404, true),
      })
      .pipe(map((res: any) => (res?.data ?? res) as OrderPlanModel));
  }

  async ensureCacheLoaded() {
    await Promise.all([
      this.branchHelper.ensureBranchesLoaded(),
      this.userHelper.ensureUsersLoaded(),
      this.sourceHelper.ensureSourcesLoaded(),
      this.servicesHelper.ensureServicesLoaded(),
    ]);
  }
}
