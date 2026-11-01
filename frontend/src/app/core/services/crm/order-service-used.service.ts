import { HttpParams } from '@angular/common/http';
import { inject, Service} from '@angular/core';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

import { OrderServiceUsedModel } from '../../models/crm/order-service-used.model';

import { ListApiResponse } from '@/share/models/list-api-response';
import { BaseApiService } from '@/share/services/base-api.service';
import { BranchHelperService } from '@/indexeddb/config/components/branch';
import { UserHelperService } from '@/indexeddb/config/components/user';
import { LocationCountryHelperService } from '@/indexeddb/config/components/location-country/location-country-helper.service';
import { LocationProvinceHelperService } from '@/indexeddb/config/components/location-province/location-province-helper.service';
import { SourceHelperService } from '@/indexeddb/config/components/source';
import { ServicesHelperService } from '@/indexeddb/config/components/services';
import { ServiceModel } from '@/core/models/config/clinic/service.model';
import { UnitHelperService } from '@/indexeddb/config/components/unit';
import { ORDER_PLAN_TYPE_MAP } from '@/define';
import { ServiceTypeHelperService } from '@/indexeddb/config/components/service-type/service-type-helper.service';
import { AppointmentTypeHelperService } from '@/indexeddb/config/components/appointment-type';

@Service()
export class OrderServiceUsedService extends BaseApiService<OrderServiceUsedModel> {
  public readonly NOT_LOCK: number = 1;
  public readonly IS_LOCK: number = 2;
  public readonly STATUS_COMMISSION_ACTIVE: number = 1;
  public readonly STATUS_COMMISSION_NOTACTIVE: number = 0;
  public readonly SERVICE_DISCOUNT_TYPE_VND: number = 1; // Loại chiết khấu dịch vụ theo tiền
  protected override unixTimesTampFields = [
    'created_day',
    'dayComplete',
    'dayReExploitation',
    'activated_at',
    'expires_at',
    'created_at',
    'updated_at',
  ];
  protected override endpoint = 'order-service-used';
  protected override dateFields = [];
  private appointmentTypeHelper = inject(AppointmentTypeHelperService);
  private serviceTypeHelper = inject(ServiceTypeHelperService);
  private unitHelper = inject(UnitHelperService);
  private locationCountryHelper = inject(LocationCountryHelperService);
  private locationProvinceHelper = inject(LocationProvinceHelperService);
  private sourceHelper = inject(SourceHelperService);
  private userHelper = inject(UserHelperService);
  private branchHelper = inject(BranchHelperService);
  private servicesHelper = inject(ServicesHelperService);

  /**
   * API trả về dịch dụ khách hàng đã order để đặt hẹn
   * @param customerId
   */
  serviceUseds(customerId: number): Observable<any> {
    return this.apiService.get(
      `${this.path}/service-useds?customerId=${customerId}`,
    );
  }


  reportServiceUsed(params: any = {}): Observable<ListApiResponse<ServiceModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/report-service-used`, params)
      .pipe<ListApiResponse<ServiceModel>>(
        map((listApiResponse: ListApiResponse<ServiceModel>) => {
          // BE có lỗi hiếm gặp trả 200 kèm body rỗng → response parse ra null, tránh vỡ cả tab.
          if (!listApiResponse) return { items: [] } as ListApiResponse<ServiceModel>;
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          (cloned['object']?.items ?? []).map((model: ServiceModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  exportReportServiceUsed(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/report-service-used?_export=1`, params);
  }

  listDoctorRevenue(params: any = {}): Observable<ListApiResponse<OrderServiceUsedModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/list-doctor-revenue`, params)
      .pipe<ListApiResponse<OrderServiceUsedModel>>(
        map((listApiResponse: ListApiResponse<OrderServiceUsedModel>) => {
          // BE có lỗi hiếm gặp trả 200 kèm body rỗng → response parse ra null, tránh vỡ cả tab.
          if (!listApiResponse) return { items: [] } as ListApiResponse<OrderServiceUsedModel>;
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          (cloned['object']?.items ?? []).map((model: OrderServiceUsedModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  lockServiceUsed(id: any, params: any = {}): Observable<string> {
    // `updated_at` (khoá optimistic lock, story-23) tới từ model đã qua `afterLoad()` nên có thể là
    // `Date` (field nằm trong `unixTimesTampFields` ở trên) — PHẢI `beforeSave()` để quy đổi lại unix
    // trước khi POST, nếu không JSON.stringify sẽ gửi ISO string thay vì số giây BE cần.
    return this.apiService.post(`${this.path}/${id}/lock-service-used`, this.beforeSave(params));
  }

  override list(params: any = {}): Observable<ListApiResponse<OrderServiceUsedModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}`, params)
      .pipe<ListApiResponse<OrderServiceUsedModel>>(
        map((listApiResponse: ListApiResponse<OrderServiceUsedModel>) => {
          // BE có lỗi hiếm gặp trả 200 kèm body rỗng → response parse ra null, tránh vỡ cả tab.
          if (!listApiResponse) return { items: [] } as ListApiResponse<OrderServiceUsedModel>;
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          (cloned['object']?.items ?? []).map((model: OrderServiceUsedModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  listWithAppointment(params: any = {}): Observable<ListApiResponse<OrderServiceUsedModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/service-used-with-appointment`, params)
      .pipe<ListApiResponse<OrderServiceUsedModel>>(
        map((listApiResponse: ListApiResponse<OrderServiceUsedModel>) => {
          // BE có lỗi hiếm gặp trả 200 kèm body rỗng → response parse ra null, tránh vỡ cả tab.
          if (!listApiResponse) return { items: [] } as ListApiResponse<OrderServiceUsedModel>;
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          (cloned['object']?.items ?? []).map((model: OrderServiceUsedModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  /**
   * API trả về dịch dụ khách hàng sử dụng
   * @param $serviceUsedId
   */
  loadGetServiceUsedOfCustomer($serviceUsedId: any): Observable<OrderServiceUsedModel> {
    return this.apiService
      .get(`${this.path}/${$serviceUsedId}/get-service-used-of-customer`)
      .pipe<OrderServiceUsedModel>(
        map((model: any) => {
          const cloned = JSON.parse(JSON.stringify(model));
          return this.afterLoad(cloned);
        }),
      );
  }

  /**
   * API export csv dịch vụ điều trị của khách hàng
   * @param params
   */
  exportServiceUsed(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}?_export=1`, params);
  }

  /**
   * API export csv dịch vụ điều trị của khách hàng
   * @param params
   */
  exportServiceUsedWithAppointmentExcel(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/service-used-with-appointment?_export=1`, params);
  }

  /**
   * API trả về dịch vụ khách hàng sử dụng
   * @param idCustomer
   */
  serviceUsedOfCustomer(idCustomer: number) {
    return this.apiService.get(
      `${this.path}/service-used-of-customer?idCustomer=${idCustomer}`,
    );
  }

  /**
   * API trả về danh sách đơn hàng or dịch vụ theo khách hàng
   * @param params
   */
  listServiceUsedCustomer(params: any = {}): Observable<any> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/list-service-used-customer`, params)
      .pipe<ListApiResponse<OrderServiceUsedModel>>(
        map((listApiResponse: ListApiResponse<OrderServiceUsedModel>) => {
          // BE có lỗi hiếm gặp trả 200 kèm body rỗng → response parse ra null, tránh vỡ cả tab.
          if (!listApiResponse) return { items: [], _meta: undefined } as ListApiResponse<OrderServiceUsedModel>;
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          (cloned.items ?? []).map((model: OrderServiceUsedModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  /**
   * API in phiếu tổng hợp dịch vụ điều trị theo khách hàng
   * Đã chuyển sang module print: GET /print/treatment/:customerId
   * @param params { idCustomer, from?, to? }
   */
  printTreatment(params: any = {}): Observable<any> {
    const { idCustomer, ...rest } = params;
    // Phải build HttpParams — apiService.get nhận ApiOptions.params: HttpParams, không nhận plain object
    let httpParams = new HttpParams();
    Object.entries(rest).forEach(([k, v]) => {
      if (v !== undefined && v !== null) httpParams = httpParams.set(k, String(v));
    });
    return this.apiService.get(`print/treatment/${idCustomer}`, { params: httpParams });
  }

  addLabel(item: OrderServiceUsedModel) {
    const itemWithDates = this.addFormattedTimestampFields(item, [
      'created_day',
      'dayComplete',
      'expires_at',
      'created_at',
      'updated_at',
    ]);
    const serviceInfo = this.servicesHelper.getServiceInfo(item.service_id!);
    return {
      ...itemWithDates,
      customerDisplayName: `${item?.customer?.fullname || ''}${item?.customer?.nickname ? ' - ' + item?.customer?.nickname : ''}`,
      orderPlanTypeLabel: ORDER_PLAN_TYPE_MAP[item.treatmentPlan?.orderPlanType!]!,
      branchLabel: this.branchHelper.getBranchName(item?.branch_id),
      revenueList: (item.listRevenues ?? []).map(id => this.userHelper.getUserInfo(id)),
      doctorList: (item.listDoctors ?? []).map(id => this.userHelper.getUserInfo(id)),
      createdByInfo: this.userHelper.getUserInfo(item.created_by),
      updatedByInfo: this.userHelper.getUserInfo(item.updated_by),
      countryInfo: this.locationCountryHelper.getCountryInfo(item.customer?.country_id),
      provinceLabel: this.locationProvinceHelper.getLabel(item?.customer?.province_id),
      sourceLabel: this.sourceHelper.getSourceName(item.source_id),
      sourceCustomerLabel: this.sourceHelper.getSourceName(item.customer?.source_id),
      serviceInfo: serviceInfo,
      unitInfo: this.unitHelper.getUnitInfo(serviceInfo?.unit_id),
      serviceTypeLabel: this.serviceTypeHelper.getLabel(serviceInfo?.service_type_id),
      appointmentTypeLabel: this.appointmentTypeHelper.getAppointmentTypeName(
        item.appointment?.appointment_type_id,
      ),
    };
  }

  async addLabels(items: OrderServiceUsedModel[]) {
    if (!Array.isArray(items)) return [];
    await this.ensureCacheLoaded();
    return items.map((item) => this.addLabel(item));
  }

  async ensureCacheLoaded() {
    await Promise.all([
      this.branchHelper.ensureBranchesLoaded(),
      this.userHelper.ensureUsersLoaded(),
      this.locationCountryHelper.ensureLocationCountriesLoaded(),
      this.locationProvinceHelper.ensureLocationProvincesLoaded(),
      this.sourceHelper.ensureSourcesLoaded(),
      this.servicesHelper.ensureServicesLoaded(),
      this.unitHelper.ensureUnitsLoaded(),
      this.serviceTypeHelper.ensureServiceTypesLoaded(),
      this.appointmentTypeHelper.ensureAppointmentTypesLoaded(),
    ]);
  }
}
