import {inject, Service} from '@angular/core';
import {HttpParams} from '@angular/common/http';
import { HttpContext } from '@angular/common/http';
import { SKIP_GLOBAL_404 } from '@/core/interceptors/response-handler.interceptor';
import {Observable} from 'rxjs';
import {map} from 'rxjs/operators';

import {ApiResponseOneModel} from '@/core/interface/ApiResponseOneModel';
import {PaymentModel} from '@/core/models/crm/payment.model';
import {ListApiResponse} from '@/share/models/list-api-response';
import {BaseApiService} from '@/share/services/base-api.service';
import {BranchHelperService} from '@/indexeddb/config/components/branch';
import {UserHelperService} from '@/indexeddb/config/components/user';
import {
  LocationCountryHelperService
} from '@/indexeddb/config/components/location-country/location-country-helper.service';
import {
  LocationProvinceHelperService
} from '@/indexeddb/config/components/location-province/location-province-helper.service';
import {PaymentMethodHelperService} from '@/indexeddb/config/components/payment-method';
import {PaymentTypeHelperService} from '@/indexeddb/config/components/payment-type';
import {SourceHelperService} from '@/indexeddb/config/components/source';
import {getPaymentForKey, PAYMENT_FOR_MAP, SERVICE_CATEGORY_MAP, TYPE_DEPOSIT_MAP} from '@/define';
import {ServicesHelperService} from '@/indexeddb/config/components/services';
import {ServiceTypeHelperService} from '@/indexeddb/config/components/service-type/service-type-helper.service';
import {CurrencyFormatPipe} from '@/share/pipes/currency-format/currency-format.pipe';

/** Đường con của các list phiếu dưới `crm/payment` ('' = phiếu thanh toán). */
export type PaymentListSubPath =
  | ''
  | 'list-receipts'
  | 'list-deposit'
  | 'list-refund-deposit'
  | 'list-refund-order-plan';

@Service()
export class PaymentService extends BaseApiService<PaymentModel> {
  protected override endpoint = 'payment';
  public readonly TYPE_DEPOSIT_RESERVE: number = 1;
  public readonly TYPE_DEPOSIT_TEMP: number = 2;
  public override unixTimesTampFields = [
    'created_at',
    'payment_time',
    'updated_at',
    'payment_of_date',
  ];
  protected override autoCompleteFields = [];
  private locationCountryHelper = inject(LocationCountryHelperService);
  private locationProvinceHelper = inject(LocationProvinceHelperService);
  private sourceHelper = inject(SourceHelperService);
  private paymentMethodHelper = inject(PaymentMethodHelperService);
  private paymentTypeHelper = inject(PaymentTypeHelperService);
  private userHelper = inject(UserHelperService);
  private branchHelper = inject(BranchHelperService);
  private servicesHelper = inject(ServicesHelperService);
  private serviceTypeHelper = inject(ServiceTypeHelperService);
  private currencyPipe = new CurrencyFormatPipe();

  /**
   * Khởi tạo giá trị ban đầu
   */
  override getInitialObject(): PaymentModel {
    return {
      amount: 0,
    };
  }

  getDepositOfCustomer(params: any = {}): Observable<ListApiResponse<PaymentModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/get-deposit-of-customer-for-payment-user`, params)
      .pipe<ListApiResponse<PaymentModel>>(
        map((listApiResponse: ListApiResponse<PaymentModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned.items.map((model: PaymentModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  /**
   * Danh sách phiếu (6 màn `modules/crm/payment/list-all`) — BE `crm/payment/handler/list.go`.
   * Response: `{object: {items, _meta}, <field tổng>}`; field tổng tuỳ list (totalSum, totalSumDeposit,
   * totalSumDepositAfter, totalRefundDeposit, totalRefund). Cùng endpoint + `_export=1` ⇒ URL file Excel.
   * `subPath` rỗng = phiếu thanh toán (`GET crm/payment`).
   */
  listPaymentKind(subPath: PaymentListSubPath, params: any = {}): Observable<any> {
    return this.apiService.get<any>(this.listPath(subPath), params).pipe(
      map((res: any) => {
        const cloned = JSON.parse(JSON.stringify(res));
        cloned['object'].items = (cloned['object']?.items ?? []).map((model: PaymentModel) => this.afterLoad(model));
        return cloned;
      }),
    );
  }

  exportPaymentKind(subPath: PaymentListSubPath, params: any = {}): Observable<string> {
    return this.apiService.get(`${this.listPath(subPath)}?_export=1`, params);
  }

  private listPath(subPath: PaymentListSubPath): string {
    return subPath ? `${this.path}/${subPath}` : this.path;
  }

  /** Phiếu thanh toán (GET crm/payment). */
  listPayments(params: any = {}): Observable<any> {
    return this.listPaymentKind('', params);
  }

  /**
   * API export thanh toán
   * @param params
   */
  exportPayment(params: any = {}): Observable<string> {
    return this.exportPaymentKind('', params);
  }

  /**
   * API export csv hoàn dịch vụ
   * @param params
   */
  exportRefundOrderPlan(params: any = {}): Observable<string> {
    return this.exportPaymentKind('list-refund-order-plan', params);
  }

  /**
   * API trả về danh sách thanh toán theo khách hàng
   * @param params
   */
  getListPaymentDetailCustomer(params: any = {}) {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/get-payment-detail-for-customer`, params)
      .pipe<ListApiResponse<PaymentModel>>(
        map((listApiPayment: ListApiResponse<PaymentModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiPayment));
          cloned.items.map((item: any) => {
            this.afterLoad(item);
          });
          return cloned;
        }),
      );
  }

  /**
   * API tạo đặt cọc
   * @param createParams
   * @param options
   */
  createDepositCustomer(createParams: any, options: any = {}): Observable<PaymentModel> {
    const params = this.beforeSave(createParams);
    return this.apiService.post<any>(`${this.path}/create-deposit-customer`, params, options).pipe(map((res) => res?.data ?? res));
  }

  /**
   * API tạo mới hoàn cọc
   * @param createParams
   * @param options
   */
  createRefundDeposit(createParams: any, options: any = {}): Observable<PaymentModel> {
    const params = this.beforeSave(createParams);
    return this.apiService.post<any>(`${this.path}/create-refund-deposit-customer`, params, options).pipe(map((res) => res?.data ?? res));
  }

  /**
   * API cập nhật đặt cọc
   * @param id
   * @param updateParams
   * @param options
   */
  updateDepositCustomer(
    id: number,
    updateParams: any,
    options: any = {},
  ): Observable<ApiResponseOneModel<PaymentModel>> {
    const params = this.beforeSave(updateParams);
    return this.apiService
      .put<
        ApiResponseOneModel<PaymentModel>
      >(`${this.path}/${id}/update-deposit-customer`, params, options)
      .pipe(
        map((res: any) => {
          const cloned = JSON.parse(JSON.stringify(res));
          cloned.data = this.afterLoad(cloned.data);
          return cloned;
        }),
      );
  }

  /**
   * API tạo mới hoàn dịch vụ
   * @param createParams
   * @param options
   */
  createRefundOrderPlan(createParams: any, options: any = {}): Observable<PaymentModel> {
    const params = this.beforeSave(createParams);
    return this.apiService.post<any>(
      `${this.path}/create-refund-order-plan`,
      params,
      options,
    ).pipe(map((res) => res?.data ?? res));
  }

  /**
   * API xóa đặt cọc
   * @param id
   * @param options
   */
  deleteDepositCustomer(id: number, options: any = {}): Observable<string> {
    return this.apiService.delete(`${this.path}/${id}/delete-deposit-customer`, options);
  }

  /**
   * API xóa hoàn cọc
   * @param id
   * @param options
   */
  deleteRefundCustomer(id: number, options: any = {}): Observable<string> {
    return this.apiService.delete(`${this.path}/${id}/delete-refund-customer`, options);
  }

  /**
   * API xóa hoàn dịch vụ
   * @param id
   * @param options
   */
  deleteRefundOrderPlan(id: number, options: any = {}): Observable<string> {
    return this.apiService.delete(`${this.path}/${id}/delete-refund-order-plan`, options);
  }

  /**
   * API chuyển cọc
   * @param id
   * @param updateParams
   * @param options
   */
  moveDepositToPayment(id: number, updateParams: any, options: any = {}): Observable<PaymentModel> {
    const params = this.beforeSave(updateParams);
    return this.apiService.put(
      `${this.path}/${id}/move-deposit-to-payment`,
      params,
      options,
    );
  }

  /**
   * API tạo thanh toán cho đơn hàng
   * @param createParams
   * @param options
   */
  createPaymentOrder(createParams: any, options: any = {}): Observable<string> {
    const params = this.beforeSave(createParams);
    return this.apiService.post<any>(`${this.path}/create-payment-order`, params, options).pipe(map((res) => res?.data ?? res));
  }

  /**
   * API tạo thanh toán khách hàng
   * @param createParams
   * @param options
   */
  createPaymentCustomer(createParams: any, options: any = {}): Observable<PaymentModel> {
    const params = this.beforeSave(createParams);
    return this.apiService.post<any>(`${this.path}/create-payment-customer`, params, options).pipe(map((res) => res?.data ?? res));
  }

  /**
   * API cập nhật thanh toán khách hàng
   * @param id
   * @param updateParams
   * @param options
   */
  updatePaymentCustomer(
    id: number,
    updateParams: any,
    options: any = {},
  ): Observable<ApiResponseOneModel<PaymentModel>> {
    const params = this.beforeSave(updateParams);
    return this.apiService
      .put<
        ApiResponseOneModel<PaymentModel>
      >(`${this.path}/${id}/update-payment-customer`, params, options)
      .pipe(
        map((res: any) => {
          const cloned = JSON.parse(JSON.stringify(res));
          cloned.data = this.afterLoad(cloned.data);
          return cloned;
        }),
      );
  }

  /**
   * API xóa thanh toán khách hàng
   * @param id
   * @param options
   */
  deletePaymentCustomer(id: number, options: any = {}): Observable<string> {
    return this.apiService.delete(`${this.path}/${id}/delete-payment-customer`, options);
  }

  /**
   * API export csv đặt cọc
   * @param params
   */
  exportCustomerDeposit(params: any = {}): Observable<string> {
    return this.exportPaymentKind('list-deposit', params);
  }

  /**
   * API in tuỳ chỉnh thanh toán
   * @param params
   */
  // In phieu thanh toan tuy chon - goi API print module rieng
  printMultiPayment(params: any = {}): Observable<any> {
    if (params.ids) {
      const idsString = Array.isArray(params.ids) ? params.ids.join(',') : params.ids;
      let httpParams = new HttpParams().set('ids', idsString);
      if (params.customerId) {
        httpParams = httpParams.set('customerId', params.customerId);
      }
      return this.apiService.get(`print/multi-payment`, {
        params: httpParams,
      });
    }
    return this.apiService.get(`print/multi-payment`);
  }

  // In phieu thu thanh toan - goi API print module rieng
  printPayment(id: number): Observable<any> {
    return this.apiService.get(`print/payment/${id}`);
  }

  printDeposit(id: number): Observable<any> {
    return this.apiService.get(`print/deposit/${id}`);
  }

  // In phieu hoan dat coc - goi API print module rieng
  printRefundDeposit(id: number): Observable<any> {
    return this.apiService.get(`print/refund-deposit/${id}`);
  }
  // In phieu hoan dich vu - goi API print module rieng
  printRefundService(id: number): Observable<any> {
    return this.apiService.get(`print/refund-service/${id}`);
  }

  /**
   * API in phiếu thu lễ tân
   * @param params query filters
   */
  listPaymentToday(params: any = {}): Observable<ListApiResponse<PaymentModel>> {
    const query = {...(params?.params ?? params)};
    query.page = query.page ? (query.page < 1 ? 1 : query.page) : 1;
    return this.apiService
      .get(`${this.path}/list-payment-for-receptionist`, {params: query})
      .pipe<ListApiResponse<PaymentModel>>(
        map((res: ListApiResponse<PaymentModel>) => {
          const cloned = JSON.parse(JSON.stringify(res));
          cloned['object'].items.map((model: PaymentModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  /**
   * API print phiếu thu lễ tân (danh sách theo filter, không phân trang)
   * Đã chuyển sang module print: GET /print/reception-payment
   * @param params query filters (filter[payment_time][gte], filter[branch_id], ...)
   */
  printReceptionPayment(params: any = {}): Observable<any> {
    return this.apiService.get('print/reception-payment', { params });
  }

  addLabel(item: PaymentModel) {
    const itemWithDates = this.addFormattedTimestampFields(item, [
      'created_day',
      'payment_of_date',
      'created_at',
      'updated_at',
      'payment_time',
    ]);
    const serviceInfo = this.servicesHelper.getServiceInfo(item.orderServiceUsed?.service_id!);
    return {
      ...itemWithDates,
      amountFormatted: this.currencyPipe.transform(item.amount),
      deposit_afterFormatted: this.currencyPipe.transform(item.deposit_after),
      customerDisplayName: `${item?.customer?.fullname || ''}${item?.customer?.nickname ? ' - ' + item?.customer?.nickname : ''}`,
      typeDepositInfo: TYPE_DEPOSIT_MAP[item.deposit_type!]!,
      branchLabel: this.branchHelper.getBranchName(item?.branch_id),
      revenue: this.userHelper.getUserInfo(item.orderServiceUsed?.listRevenues?.[0]),
      doctorRevenue: this.userHelper.getUserInfo(item.orderServiceUsed?.listDoctors?.[0]),
      createdByInfo: this.userHelper.getUserInfo(item.created_by),
      updatedByInfo: this.userHelper.getUserInfo(item.updated_by),
      countryInfo: this.locationCountryHelper.getCountryInfo(item.customer?.country_id),
      provinceLabel: this.locationProvinceHelper.getLabel(item?.customer?.province_id),
      paymentMethodLabel: this.paymentMethodHelper.getPaymentMethodName(item.method),
      // Kênh thanh toán = card_type (trước đây tra nhầm theo `method` ⇒ cột hiện sai/trống).
      paymentTypeLabel: this.paymentTypeHelper.getPaymentTypeName(item.card_type_id),
      sourceLabel: this.sourceHelper.getSourceName(item.orderServiceUsed?.source_id),
      sourceCustomerLabel: this.sourceHelper.getSourceName(item.customer?.source_id),
      serviceInterestedLabel: this.serviceTypeHelper.getLabel(item.customer?.type_service_id),
      branchAppointmentLabel: this.branchHelper.getBranchName((item.customer as any)?.appointment?.branch_id),
      paymentForLabel: PAYMENT_FOR_MAP[item.payment_for!]! || 'Không xác định',
      paymentForKey: getPaymentForKey(item.payment_for!),
      paymentForClass: `paymentFor-${item.payment_for}`,
      serviceInfo: serviceInfo,
      serviceTypeLabel: this.serviceTypeHelper.getLabel(serviceInfo?.service_type_id),
      serviceCategoryInfo: SERVICE_CATEGORY_MAP[serviceInfo?.serviceCategory_id],
    };
  }

  async addLabels(items: PaymentModel[]) {
    if (!Array.isArray(items)) return [];
    await this.ensureCacheLoaded();
    return items.map((item) => this.addLabel(item));
  }

  /**
   * Chi tiết phiếu thu cho SlideOver — endpoint RIÊNG `/:id/detail`, không dùng `get(id)`.
   * `get(id)` trả nguyên entity và đang phục vụ các form sửa phiếu; endpoint riêng cho phép BE trả
   * đúng cột màn này cần + kèm `allocations` (các phiếu cùng đơn hàng).
   */
  getDetail(id: number | string): Observable<PaymentModel> {
    return this.apiService
      .get<{ data: PaymentModel }>(`${this.path}/${id}/detail`, {
        // 404 ở đây = phiếu/đơn đã bị xoá — panel tự báo, KHÔNG đá cả trang sang /auth/notfound.
        context: new HttpContext().set(SKIP_GLOBAL_404, true),
      })
      .pipe(map((res: any) => (res?.data ?? res) as PaymentModel));
  }

  async ensureCacheLoaded() {
    await Promise.all([
      this.branchHelper.ensureBranchesLoaded(),
      this.userHelper.ensureUsersLoaded(),
      this.locationCountryHelper.ensureLocationCountriesLoaded(),
      this.locationProvinceHelper.ensureLocationProvincesLoaded(),
      this.paymentMethodHelper.ensurePaymentMethodsLoaded(),
      this.paymentTypeHelper.ensurePaymentTypesLoaded(),
      this.sourceHelper.ensureSourcesLoaded(),
      this.servicesHelper.ensureServicesLoaded(),
      this.serviceTypeHelper.ensureServiceTypesLoaded(),
    ]);
  }
}
