import { inject, Service} from '@angular/core';
import { format } from 'date-fns';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

import { ApiResponseOneModel } from '@/core/interface/ApiResponseOneModel';
import { CustomerModel } from '@/core/models/crm/customer.model';
import {
  CUSTOMER_STATUS,

  getCustomerConnectKey,
  getCustomerConnectLabel,
  getCustomerDataLabel,
  getCustomerStatusKey,
  getCustomerStatusLabel,
  getCustomerTypeKey,
  getCustomerTypeLabel,
  getGenderKey,
  getGenderLabel,
} from '@/define';
import { ListApiResponse } from '@/share/models/list-api-response';
import { BaseApiService } from '@/share/services/base-api.service';
import { BranchHelperService } from '@/indexeddb/config/components/branch';
import { UserHelperService } from '@/indexeddb/config/components/user';
import { ServiceTypeHelperService } from '@/indexeddb/config/components/service-type/service-type-helper.service';
import { LocationCountryHelperService } from '@/indexeddb/config/components/location-country/location-country-helper.service';
import { LocationProvinceHelperService } from '@/indexeddb/config/components/location-province/location-province-helper.service';
import { SourceHelperService } from '@/indexeddb/config/components/source';
import { AppointmentNotBookedReasonsHelperService } from '@/indexeddb/config/components/appointment-not-booked-reasons/appointment-not-booked-reasons-helper.service';
import { CustomerCategoryHelperService } from '@/indexeddb/config/components/customer-category';
import { UserOptionHelperService } from '@/indexeddb/config/components/user-option';
import { JobHelperService } from '@/indexeddb/config/components/job';
import { EthnicityHelperService } from '@/indexeddb/config/components/ethnicity';

@Service()
export class CustomerService extends BaseApiService<CustomerModel> {
  override endpoint = 'customer';
  public readonly VIETNAM = 237;
  protected override unixTimesTampFields = [
    'created_at',
    'updated_at',
    'created_day',
    'appointment_date',
    'emr_insurance_expiry_date',
      'emr_insurance_valid_from',
      'emr_insurance_valid_to',
  ];
  protected override dateFields = ['birthday'];
  protected override jsonFields = [];
  private appointmentNotBookedReasonsHelper = inject(AppointmentNotBookedReasonsHelperService);
  private locationCountryHelper = inject(LocationCountryHelperService);
  private locationProvinceHelper = inject(LocationProvinceHelperService);
  private sourceHelper = inject(SourceHelperService);
  private userHelper = inject(UserHelperService);
  private branchHelper = inject(BranchHelperService);
  private serviceTypeHelper = inject(ServiceTypeHelperService);
  private customerCategoryHelper = inject(CustomerCategoryHelperService);
  private userOptionHelper = inject(UserOptionHelperService);
  private jobHelper = inject(JobHelperService);
  private ethnicityHelper = inject(EthnicityHelperService);

  /**
   * Khởi tạo ban đầu
   */
  override getInitialObject() {
    return {
      created_day: this.timezoneService.now(),
    };
  }


  addTag(id: number, tag: string): Observable<string[]> {
    return this.apiService.post<string[]>(`${this.path}/${id}/add-tag?tag=${tag}`);
  }

  removeTag(id: number, tag: string): Observable<string[]> {
    return this.apiService.post<string[]>(`${this.path}/${id}/remove-tag?tag=${tag}`);
  }

  /**
   * API get khách hàng theo id
   * @param id
   * @param options
   */
  getCustomerById(id: number, options: any = {}): Observable<CustomerModel> {
    return this.apiService
      .get<any>(`${this.path}/${id}`, options)
      .pipe<CustomerModel>(
        map((res: any) => {
          const raw = res?.data ?? res;
          const cloned = typeof structuredClone === 'function'
            ? structuredClone(raw)
            : JSON.parse(JSON.stringify(raw));
          return this.afterLoad(cloned);
        }),
      );
  }

  /**
   * API hiển thị danh sách khách hàng
   * @param params
   */
  getCustomer(params: any = {}): Observable<ListApiResponse<CustomerModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get(`${this.path}`, params).pipe<ListApiResponse<CustomerModel>>(
      map((listApiResponse: ListApiResponse<CustomerModel>) => {
        const cloned = JSON.parse(JSON.stringify(listApiResponse));
        cloned.items.map((model: CustomerModel) => {
          return this.afterLoad(model);
        });
        return cloned;
      }),
    );
  }

  /**
   * API hiển thị danh sách tư vấn
   * @param params
   */
  getRevenue(params: any = {}): Observable<ListApiResponse<CustomerModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/revenue`, params)
      .pipe<ListApiResponse<CustomerModel>>(
        map((listApiResponse: ListApiResponse<CustomerModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned.items.map((model: CustomerModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  /**
   * API export csv khách hàng
   * @param params
   */
  exportCustomer(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}?_export=1`, params);
  }

  /**
   * API export csv Marketing Lead
   * @param params
   */
  exportLeadToExcel(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/lead?_export=1`, params);
  }

  exportTicket(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/ticket?_export=1`, params);
  }

  exportConvert(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/convert?_export=1`, params);
  }

  exportTrash(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/trash?_export=1`, params);
  }

  moveToTrash(id: number, params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/${id}/move-to-trash`, params);
  }

  /**
   * API update avatar khách hàng
   * @param id
   * @param form
   * @param params
   */
  updateAvatar(id: number, form: any, params: any = {}): Observable<any> {
    return this.apiService.post<any>(`${this.path}/${id}/create-avatar`, form, params);
  }

  /**
   * API trả về danh sách công nợ khách hàng trước và trong kỳ
   * @param params
   */
  getInfoDebtsRevenueCustomer(params: any = {}): Observable<ListApiResponse<CustomerModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/info-debts-revenue-customer`, params)
      .pipe<ListApiResponse<CustomerModel>>(
        map((listApiResponse: ListApiResponse<CustomerModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned.items?.map((model: CustomerModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  /**
   * API thêm nhãn khách hàng
   * @param id
   * @param params
   * @param options
   */
  updateTags(id: number, params: any, options: any = {}): Observable<CustomerModel> {
    return this.apiService.post<any>(`${this.path}/${id}/update-tags-customer`, params, options).pipe(map((res) => res?.data ?? res));
  }

  /**
   * API tạo mới khách hàng
   * @param paramsValue
   * @param options
   */
  createCustomerVisited(paramsValue: CustomerModel, options: any = {}): Observable<any> {
    const params = this.beforeSave(paramsValue);
    params.year_born = paramsValue['year_born']
      ? parseInt(format(paramsValue['year_born'], 'yyyy'))
      : paramsValue['year_born'];
    return this.apiService.post(`${this.path}/create-customer-visited`, params, options);
  }

  /**
   * API tạo mới khách hàng lead
   * @param paramsValue
   * @param options
   */
  createCustomer(paramsValue: CustomerModel, options: any = {}): Observable<any> {
    const params = this.beforeSave(paramsValue);
    params.year_born = paramsValue['year_born']
      ? parseInt(format(paramsValue['year_born'], 'yyyy'))
      : paramsValue['year_born'];
    return this.apiService.post(`${this.path}`, params, options);
  }

  /**
   * API phiếu in tổng hợp khách hàng
   * @param id
   * @param params
   */
  printfOverview(id: number, params: any = {}) {
    return this.apiService.get(`${this.path}/${id}/printf-overview`, params);
  }

  /**
   * API trả về danh sách khách hàng Hoàn thành dịch vụ
   * @param params
   */
  getCustomerDid(params: any = {}): Observable<ListApiResponse<CustomerModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/get-customer-with-status?status=${CUSTOMER_STATUS.COMPLETED}`, params)
      .pipe<ListApiResponse<CustomerModel>>(
        map((res: ListApiResponse<CustomerModel>) => {
          const cloned = JSON.parse(JSON.stringify(res));
          (res.items ?? []).map((value: any) => {
            return this.afterLoad(value);
          });
          return cloned;
        }),
      );
  }

  /**
   * API cập nhật thông tin khách hàng
   * @param id
   * @param updateParams
   * @param options
   */
  updateCustomer(
    id: number,
    updateParams: any,
    options: any = {},
  ): Observable<ApiResponseOneModel<CustomerModel>> {
    const params = this.beforeSave(updateParams);
    return this.apiService
      .put<
        ApiResponseOneModel<CustomerModel>
      >(`${this.path}/${id}`, params, options)
      .pipe(
        map((res: any) => {
          const cloned = JSON.parse(JSON.stringify(res));
          this.afterLoad(cloned.data);
          return cloned;
        }),
      );
  }

  /**
   * API tạo mới ticket
   * @param id
   * @param updateParams
   * @param options
   */
  createTicket(updateParams: any, options: any = {}): Observable<CustomerModel> {
    const params = this.beforeSave(updateParams);
    return this.apiService
      .put<CustomerModel>(`${this.path}/create-ticket`, params, options)
      .pipe<CustomerModel>(
        map((res: CustomerModel) => {
          const cloned = JSON.parse(JSON.stringify(res));
          this.afterLoad(cloned);
          return cloned;
        }),
      );
  }

  /**
   * API trả về timeline khách hàng
   * @param id
   */
  getTimeLine(id: number): Observable<any> {
    return this.apiService
      .get(`${this.path}/${id}/timeline-customer`)
      .pipe(map((res: any) => res?.data ?? res));
  }

  /**
   * API trả về danh sách khách hàng không có lịch hẹn
   * @param params
   */
  appointmentListNotSchedule(params: any = {}): Observable<ListApiResponse<CustomerModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/list-customer-not-schedule`, params)
      .pipe<ListApiResponse<CustomerModel>>(
        map((listApiResponse: ListApiResponse<CustomerModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned.items.map((value: CustomerModel) => {
            return this.afterLoad(value);
          });
          return cloned;
        }),
      );
  }

  /**
   * API trả về số điện thoại khách hàng để kiểm tra số điện thoại bị trùng khi tạo mới khách hàng
   * @param phone
   * @param params
   */
  getNumberCustomerByPhone(phone: any, params: any = {}): Observable<any> {
    return this.apiService.get(
      `${this.path}/get-number-customer-by-phone?phone=${phone}`,
      params,
    );
  }

  /**
   * API trả về khách hàng theo mã KH cũ để cảnh báo trùng khi tạo mới khách hàng
   * @param oldCode
   * @param params
   */
  getNumberCustomerByOldCode(oldCode: any, params: any = {}): Observable<any> {
    return this.apiService.get(
      `${this.path}/get-number-customer-by-old-code?old_code=${oldCode}`,
      params,
    );
  }

  /**
   * API trả về số điện thoại khách hàng để kiểm tra số điện thoại bị trùng khi tạo mới khách hàng
   * @param code
   * @param params
   */
  getCustomerByCode(code: string, params: any = {}): Observable<any> {
    return this.apiService.get(`${this.path}/get-customer-by-code?code=${code}`, params);
  }

  /**
   * API search khách hàng global
   * @param $keyWord
   * @param params
   */
  searchGlobal($keyWord: any, params: any = {}): Observable<any> {
    return this.apiService.get(`${this.path}/search-global?keyword=${encodeURIComponent($keyWord)}`, params);
  }

  /**
   * API trả về danh sách lead
   * @param params
   */
  getLead(params: any = {}): Observable<ListApiResponse<CustomerModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/lead`, params)
      .pipe<ListApiResponse<CustomerModel>>(
        map((listApiResponse: ListApiResponse<CustomerModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned['object'].items.map((model: any) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  getTicketWarehourse(params: any = {}): Observable<ListApiResponse<CustomerModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/ticket-warehourse`, params)
      .pipe<ListApiResponse<CustomerModel>>(
        map((listApiResponse: ListApiResponse<CustomerModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned['object'].items.map((model: any) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  getTrashWarehourse(params: any = {}): Observable<ListApiResponse<CustomerModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/trash`, params)
      .pipe<ListApiResponse<CustomerModel>>(
        map((listApiResponse: ListApiResponse<CustomerModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned['object'].items.map((model: any) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  override importData(params: any) {
    const formData: FormData = new FormData();
    formData.append('file', params, params.name);
    return this.apiService.post(`${this.path}/import-customer-from-excel`, formData);
  }

  /**
   * Update Macro Customer
   * @param id
   * @param macro
   */
  /**
   * Update Appointment Not Booked Reasons
   * @param id_customer
   * @param appointment_not_booked_reasons_id
   */
  updateAppointmentNotBookedReasons(
    id_customer: number,
    appointment_not_booked_reasons_id: number,
  ): Observable<string[]> {
    return this.apiService.post(
      `${this.path}/${id_customer}/update-appointment-not-booked-reasons?appointmentNotBookedReasons=${appointment_not_booked_reasons_id}`,
    );
  }

  getNotPlanOnOneMonthOfCustomer(params: any = {}) {
    params.params = {
      ...params.params,
    };
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get(`${this.path}/one-month`, params);
  }

  getNotPlanOnThreeMonthOfCustomer(params: any = {}) {
    params.params = {
      ...params.params,
    };
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get(`${this.path}/three-month`, params);
  }

  getNotPlanOnSixMonthOfCustomer(params: any = {}) {
    params.params = {
      ...params.params,
    };
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get(`${this.path}/six-month`, params);
  }

  getNotPlanOnOneYearOfCustomer(params: any = {}) {
    params.params = {
      ...params.params,
    };
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get(`${this.path}/one-year`, params);
  }

  /**
   *
   * @param params
   */
  getPortraitStatus(params: any = {}) {
    params.params = {
      ...params.params,
    };
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get(`${this.path}/status`, params);
  }

  /**
   *
   * @param params
   */
  getNotAppointment(params: any = {}) {
    params.params = {
      ...params.params,
    };
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get(`${this.path}/not-appointment`, params);
  }

  /**
   *
   * @param params
   */
  getGroupCustomer(params: any = {}) {
    params.params = {
      ...params.params,
    };
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get(`${this.path}/group-customer`, params);
  }

  /**
   *
   * @param params
   */
  getEvent(params: any = {}) {
    params.params = {
      ...params.params,
    };
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get(`${this.path}/event`, params);
  }

  addLabel(item: CustomerModel) {
    const anyItem = item as any;
    const itemWithFormattedDates = this.addFormattedTimestampFields(
      item,
      ['created_day', 'birthday', 'created_at', 'updated_at', 'insurance_expiry_date'],
      ['birthday'],
    );

    return {
      ...itemWithFormattedDates,
      customerDisplayName: `${item.fullname || ''}${item.nickname ? ' - ' + item.nickname : ''}`,
      statusClass: `statusCustomer-${item.status}`,
      genderLabel: getGenderLabel?.(item.gender!) ?? '',
      genderKey: getGenderKey(item.gender!),
      provinceLabel: this.locationProvinceHelper.getLabel(item.province_id),
      sourceLabel: this.sourceHelper.getSourceName(item.source_id),
      serviceTypeLabel: this.serviceTypeHelper.getLabel(item.type_service_id!),
      statusLabel: getCustomerStatusLabel?.(item.status) ?? '',
      statusKey: getCustomerStatusKey(item.status!),
      branchLabel: this.branchHelper.getBranchName(item?.branch_id),
      appointmentNotBookedReasonsLabel: this.appointmentNotBookedReasonsHelper.getReasonName(
        item.appointment_not_booked_reasons_id,
      ),
      createdByInfo: this.userHelper.getUserInfo(item.created_by),
      updatedByInfo: this.userHelper.getUserInfo(item.updated_by),
      paymentUserInfo: this.userHelper.getUserInfo(item.payment_user_id),
      countryInfo: this.locationCountryHelper.getCountryInfo(item.country_id),
      customerConnectLabel: getCustomerConnectLabel(item.connect_id!),
      customerConnectKey: getCustomerConnectKey(item.connect_id!),
      customerConnectClass: `customerConnect-${item.connect_id}`,
      customerDataLabel: getCustomerDataLabel(item.data_id!),
      customerDataClass: `customerData-${item.data_id}`,
      customerTypeLabel: getCustomerTypeLabel(item.customerType!),
      customerTypeKey: getCustomerTypeKey(item.customerType!),
      customerCategoryLabel: this.customerCategoryHelper.getCustomerCategoryName(item.category),
      jobLabel: this.jobHelper.getJobName(item.job_id),
      ethnicityLabel: this.ethnicityHelper.getEthnicityName(item.ethnicity_id),
      lastOrderAtFmt: anyItem.lastServiceDate
        ? this.formatTimestampFields(anyItem.lastServiceDate, true)
        : '',
      age: item.birthday
        ? Math.floor((Date.now() - new Date(item.birthday as any).getTime()) / (365.25 * 24 * 60 * 60 * 1000))
        : null,
    };
  }

  async addLabels(items: CustomerModel[]) {
    if (!Array.isArray(items)) return [];
    await this.ensureCacheLoaded();
    return items.map((item) => this.addLabel(item));
  }

  /**
   * Ensure all helper caches are loaded before processing labels
   */
  async ensureCacheLoaded() {
    await Promise.all([
      this.branchHelper.ensureBranchesLoaded(),
      this.userHelper.ensureUsersLoaded(),
      this.serviceTypeHelper.ensureServiceTypesLoaded(),
      this.locationCountryHelper.ensureLocationCountriesLoaded(),
      this.locationProvinceHelper.ensureLocationProvincesLoaded(),
      this.sourceHelper.ensureSourcesLoaded(),
      this.appointmentNotBookedReasonsHelper.ensureReasonsLoaded(),
      this.customerCategoryHelper.ensureCustomerCategoryLoaded(),
      this.userOptionHelper.ensureSettingLoaded(),
      this.jobHelper.ensureJobLoaded(),
      this.ethnicityHelper.ensureEthnicityLoaded(),
    ]);
  }
}
