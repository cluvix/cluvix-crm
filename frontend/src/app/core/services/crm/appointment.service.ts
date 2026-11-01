import {inject, Service} from '@angular/core';
import {Observable} from 'rxjs';
import {map} from 'rxjs/operators';

import {ApiResponseOneModel} from '@/core/interface/ApiResponseOneModel';
import {AppointmentModel} from '@/core/models/crm/appointment.model';
import {ListApiResponse} from '@/share/models/list-api-response';
import {BaseApiService} from '@/share/services/base-api.service';
import {getCurrentDateInTimeStamp} from '@/share/services/utils.service';
import {BranchHelperService} from '@/indexeddb/config/components/branch';
import {UserHelperService} from '@/indexeddb/config/components/user';
import {ServiceTypeHelperService} from '@/indexeddb/config/components/service-type/service-type-helper.service';
import {
  LocationProvinceHelperService
} from '@/indexeddb/config/components/location-province/location-province-helper.service';
import {
  APPOINTMENT_BOOK_TYPE_MAP,
  APPOINTMENT_STATUS_MAP,
  APPOINTMENT_STATUS_SELECT,
  CONFIRM_APPOINTMENT_MAP,
  getGenderKey,
  getGenderLabel,
} from '@/define';
import {AppointmentIsAcceptedHelperService} from '@/indexeddb/config/components/appointment-is-accepted';
import {AppointmentTypeHelperService} from '@/indexeddb/config/components/appointment-type';
import {SourceHelperService} from '@/indexeddb/config/components/source';
import {ServicesHelperService} from '@/indexeddb/config/components/services';
import {AppointmentReasonHelperService} from '@/indexeddb/config/components/appointment-reason';

@Service()
export class AppointmentService extends BaseApiService<AppointmentModel> {
  protected override endpoint = 'appointment';
  protected override dateFields = [];
  protected override unixTimesTampFields = [
    'appointment_date_first',
    'appointment_date',
    'checkin_date',
    'checkout_date',
    'created_at',
    'updated_at',
  ];
  protected override autoCompleteFields = ['customer_id'];
  public locationProvinceHelper = inject(LocationProvinceHelperService);
  public appointmentTypeHelper = inject(AppointmentTypeHelperService);
  public sourceHelper = inject(SourceHelperService);
  public userHelper = inject(UserHelperService);
  public branchHelper = inject(BranchHelperService);
  public serviceTypeHelper = inject(ServiceTypeHelperService);
  public appointmentIsAcceptedHelper = inject(AppointmentIsAcceptedHelperService);
  public servicesHelperService = inject(ServicesHelperService);
  public appointmentReasonHelperService = inject(AppointmentReasonHelperService);

  constructor() {
    super();
  }

  /**
   * Seed cho form Signal Forms (`FormReception` — đặt/sửa lịch hẹn ở màn Tiếp nhận). BẮT BUỘC trả đủ
   * key mà form bind tới, giá trị rỗng dùng `null`: Signal Forms coi field giá trị `undefined` là
   * "không tồn tại" (`rootField.x` trả `undefined` chứ không phải FieldTree) → field vỡ vĩnh viễn.
   *
   * Chỉ gồm đúng 12 field của `FormReception` — KHÔNG liệt kê mọi cột của `appointment` (form khác
   * dùng service này vẫn đi đường Reactive Forms cũ, không phụ thuộc hàm này).
   */
  override getInitialObject() {
    return {
      content: null,
      contentPrivate: null,
      service_id: null,
      customer_id: null,
      appointment_type_id: null,
      branch_id: null,
      appointment_date: null,
      type_service_id: null,
    };
  }

  /**
   * Lấy ra danh sách lịch hẹn trong ngày của khách hàng
   * @param date
   * @param idCustomer
   * @param params
   */
  appointmentDateInDay(
    date: Date,
    idCustomer: number,
    params: any = {},
  ): Observable<AppointmentModel[]> {
    const appointmentDate = this.timezoneService.dateHmsToUnix(date, date.getHours(), date.getMinutes(), date.getSeconds());
    return this.apiService.get(
      `${this.path}/appointment-date-in-day?appointmentDate=${appointmentDate}&idCustomer=${idCustomer}`,
      params,
    );
  }

  override get(id: number | string, options: any = {}): Observable<AppointmentModel> {
    return this.apiService.get<{ data: AppointmentModel }>(`${this.path}/${id}`, options).pipe(
      map((res: any) => {
        const raw = res?.data ?? res;
        const cloned = JSON.parse(JSON.stringify(raw));
        return this.afterLoad(cloned) as AppointmentModel;
      }),
    );
  }

  getAppointmentTomorrowForUserSale(
    params: any = {},
  ): Observable<ListApiResponse<AppointmentModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/get-appointment-tomorrow-for-user-sale`, params)
      .pipe<ListApiResponse<AppointmentModel>>(
        map((listApiResponse: ListApiResponse<AppointmentModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned.items.map((model: AppointmentModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  getAppointmentScheduleByDate(params: any = {}): Observable<any> {
    return this.apiService.get(`${this.path}/appointment-schedule`, params);
  }

  getAppointmentScheduleByHour(params: any = {}): Observable<any> {
    return this.apiService.get(`${this.path}/appointment-schedule-byhour`, params);
  }

  appointmentScheduleByHourList(params: any = {}): Observable<any[]> {
    return this.apiService.get(`${this.path}/appointment-schedule-byhour-list`, params);
  }

  // Phân trang theo chi nhánh (infinite scroll) — dùng cho ScheduleByhourList thay vì load hết 1 lần
  appointmentScheduleByHourGrouped(params: any = {}): Observable<any> {
    return this.apiService.get(`${this.path}/appointment-schedule-byhour-grouped`, params);
  }

  // API print danh sách lịch hẹn theo giờ (đã chuyển sang module print)
  printAppointmentByhourList(params: any = {}): Observable<any> {
    return this.apiService.get('print/appointment-byhour-list', params);
  }

  exportAppointmentByhourList(params: any = {}): Observable<string> {
    return this.apiService.get('print/appointment-byhour-list-export', params);
  }

  /**
   * Danh sách Tiếp nhận - Khám bệnh EMR
   * @param params
   */
  listReception(params: any = {}): Observable<ListApiResponse<AppointmentModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get<ListApiResponse<AppointmentModel>>(`${this.path}/reception`, params).pipe(
      map((listApiResponse: ListApiResponse<AppointmentModel>) => {
        const cloned = JSON.parse(JSON.stringify(listApiResponse));
        cloned.items.map((model: AppointmentModel) => {
          return this.afterLoad(model);
        });
        return cloned;
      }),
    );
  }

  override list(params: any = {}): Observable<ListApiResponse<AppointmentModel>> {

    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get<ListApiResponse<AppointmentModel>>(`${this.path}`, params).pipe(
      map((listApiResponse: ListApiResponse<AppointmentModel>) => {
        const cloned = JSON.parse(JSON.stringify(listApiResponse));
        cloned.items.map((model: AppointmentModel) => {
          return this.afterLoad(model);
        });
        return cloned;
      }),
    );
  }

  /**
   * API export csv lịch hẹn
   * @param params
   */
  exportAppointment(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}?_export=1`, params);
  }

  /**
   * API tạo mới lịch hẹn
   * @param paramsValue
   * @param options
   */
  createAppointment(
    paramsValue: any,
    options: any = {},
  ): Observable<ApiResponseOneModel<AppointmentModel>> {
    const params = this.beforeSave(paramsValue);
    params.customer_id = paramsValue['customer_id'];
    return this.apiService
      .post<ApiResponseOneModel<AppointmentModel>>(`${this.path}`, params, options)
      .pipe(
        map((res) => {
          const cloned = JSON.parse(JSON.stringify(res));
          cloned.data = this.afterLoad(cloned.data);
          return cloned;
        }),
      );
  }

  updateConfirmAppointment(
    id: any,
    param: any = {},
    options: any = {},
  ): Observable<ApiResponseOneModel<AppointmentModel>> {
    const params = this.beforeSave(param);
    return this.apiService
      .put<
        ApiResponseOneModel<AppointmentModel>
      >(`${this.path}/${id}/update-confirm-appointment`, params, options)
      .pipe(
        map((res) => {
          const cloned = JSON.parse(JSON.stringify(res));
          cloned.data = this.afterLoad(cloned.data);
          return cloned;
        }),
      );
  }

  /**
   * API update lịch hẹn
   * @param id
   * @param param
   * @param options
   */
  updateAppointment(
    id: any,
    param: any,
    options: any = {},
  ): Observable<ApiResponseOneModel<AppointmentModel>> {
    const params = this.beforeSave(param);

    return this.apiService
      .put<
        ApiResponseOneModel<AppointmentModel>
      >(`${this.path}/${id}/update-appointment`, params, options)
      .pipe(
        map((res) => {
          const cloned = JSON.parse(JSON.stringify(res));
          cloned.data = this.afterLoad(cloned.data);
          return cloned;
        }),
      );
  }

  /**
   * API trả về lịch hẹn tiếp theo của khách hàng
   * @param customerId
   */
  getNextAppointmentsByCustomer(customerId: number): Observable<AppointmentModel | null> {
    const params = {
      sort: 'appointment_date',
      'per-page': 1,
      'filter[customer_id][eq]': customerId,
      'filter[status][eq]': APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_NOT_CHECKIN,
      'filter[appointment_date][gte]': getCurrentDateInTimeStamp(),
    };

    return this.list({params}).pipe(
      map((listApiResponse: any) => {
        return listApiResponse.items?.length ? listApiResponse.items[0] : null;
      }),
    );
  }

  /**
   * API update lịch hẹn hủy của khách hàng
   * @param id
   * @param params
   * @param options
   */
  updateAppointmentCancel(
    id: any,
    params: any = {},
    options: any = {},
  ): Observable<ApiResponseOneModel<AppointmentModel>> {
    params = this.beforeSave(params);

    return this.apiService
      .put<
        ApiResponseOneModel<AppointmentModel>
      >(`${this.path}/${id}/update-appointment-cancel`, params, options)
      .pipe(
        map((res) => {
          const cloned = JSON.parse(JSON.stringify(res));
          cloned.data = this.afterLoad(cloned.data);
          return cloned;
        }),
      );
  }

  /**
   * API update lịch hẹn checkin của khách hàng
   * @param id
   * @param param
   * @param options
   */
  customerCheckin(
    id: any,
    param: any = {},
    options: any = {},
  ): Observable<ApiResponseOneModel<AppointmentModel>> {
    const params = this.beforeSave(param);

    return this.apiService
      .put<
        ApiResponseOneModel<AppointmentModel>
      >(`${this.path}/${id}/customer-checkin`, params, options)
      .pipe(
        map((res) => {
          const cloned = JSON.parse(JSON.stringify(res));
          cloned.data = this.afterLoad(cloned.data);
          return cloned;
        }),
      );
  }

  /**
   * API trả về danh sách lịch hẹn theo khách hàng
   * @param idCustomer
   * @param params
   */
  getAppointmentCustomer(
    idCustomer: any,
    params: any = {},
  ): Observable<ListApiResponse<AppointmentModel>> {
    return this.apiService
      .get(`${this.path}/get-by-customer?idCustomer=${idCustomer}`, params)
      .pipe<ListApiResponse<AppointmentModel>>(
        map((listAppointment: ListApiResponse<AppointmentModel>) => {
          const cloned = JSON.parse(JSON.stringify(listAppointment));
          cloned.items.map((value: AppointmentModel) => {
            return this.afterLoad(value);
          });
          return cloned;
        }),
      );
  }

  /**
   * API trả về data để hiển thị lên biểu đổ lịch hẹn
   * @param $params
   */
  getAppointmentChartStatus($params: any): Observable<any> {
    return this.apiService.get(`${this.path}/get-appointment-chart-status`, $params);
  }

  /**
   * API trả về danh sách lịch hẹn chưa đến
   * @param params
   */
  notCome(params: any = {}): Observable<ListApiResponse<AppointmentModel>> {
    return this.apiService
      .get(`${this.path}/not-come`, params)
      .pipe<ListApiResponse<AppointmentModel>>(
        map((listAppointment: ListApiResponse<AppointmentModel>) => {
          const cloned = JSON.parse(JSON.stringify(listAppointment));
          cloned.items.map((value: AppointmentModel) => {
            return this.afterLoad(value);
          });
          return cloned;
        }),
      );
  }

  dontWork(params: any = {}): Observable<ListApiResponse<AppointmentModel>> {
    return this.apiService
      .get(`${this.path}/dont-work`, params)
      .pipe<ListApiResponse<AppointmentModel>>(
        map((listAppointment: ListApiResponse<AppointmentModel>) => {
          const cloned = JSON.parse(JSON.stringify(listAppointment));
          cloned.items.map((value: AppointmentModel) => {
            return this.afterLoad(value);
          });
          return cloned;
        }),
      );
  }

  appointmentCancel(params: any = {}): Observable<ListApiResponse<AppointmentModel>> {
    return this.apiService
      .get(`${this.path}/appointment-cancel`, params)
      .pipe<ListApiResponse<AppointmentModel>>(
        map((listAppointment: ListApiResponse<AppointmentModel>) => {
          const cloned = JSON.parse(JSON.stringify(listAppointment));
          cloned.items.map((value: AppointmentModel) => {
            return this.afterLoad(value);
          });
          return cloned;
        }),
      );
  }

  getAppointmentNotArrivedForUserSale(
    params: any = {},
  ): Observable<ListApiResponse<AppointmentModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/get-appointment-not-arrived-for-user-sale`, params)
      .pipe<ListApiResponse<AppointmentModel>>(
        map((listApiResponse: ListApiResponse<AppointmentModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned.items.map((model: AppointmentModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  /**
   * Thống kê lịch hẹn tư vấn
   * @param year
   */
  appointmentOverview(year?: number) {
    return this.apiService.get(`${this.path}/appointment-overview?year=${year}`);
  }

  addNote(id: number, note: string): Observable<any> {
    return this.apiService.post(`${this.path}/${id}/add-note`, {note});
  }

  addLabel(item: AppointmentModel) {
    const itemWithDates = this.addFormattedTimestampFields(item, [
      'appointment_date',
      'appointment_date_first',
      'checkin_date',
      'checkout_date',
      'created_at',
    ]);
    const formattedList = item?.appointmentListDate?.map(item =>
      this.addFormattedTimestampFields(item, ['new', 'old'])
    );

    return {
      ...itemWithDates,
      customerDisplayName: `${item?.customer?.fullname || ''}${item?.customer?.nickname ? ' - ' + item?.customer?.nickname : ''}`,
      genderLabel: getGenderLabel?.(item?.customer?.gender!) ?? '',
      genderKey: getGenderKey(item?.customer?.gender!),
      confirmAppointmentInfo: CONFIRM_APPOINTMENT_MAP[item?.confirmAppointment!]!,
      branchLabel: this.branchHelper.getBranchName(item?.branch_id),
      createdByInfo: this.userHelper.getUserInfo(item?.created_by),
      updatedByInfo: this.userHelper.getUserInfo(item?.updated_by),
      paymentUser: this.userHelper.getUserInfo(item?.customer?.payment_user_id),
      convertInfo: this.userHelper.getUserInfo(item?.convert_id),
      revenue: this.userHelper.getUserInfo(item?.revenue_id),
      serviceTypeLabel: this.serviceTypeHelper.getLabel(item?.type_service_id!),
      serviceTypeAcceptedLabel: this.serviceTypeHelper.getLabel(item?.type_service_accepted_id!),
      provinceLabel: this.locationProvinceHelper.getLabel(item?.customer?.province_id),
      bookTypeInfo: APPOINTMENT_BOOK_TYPE_MAP[item?.bookType!]!,
      statusInfo: APPOINTMENT_STATUS_MAP[item?.status],
      appointmentIsAcceptedInfo: this.appointmentIsAcceptedHelper.getAppointmentIsAcceptedInfo(
        item?.is_accepted,
      ),
      appointmentTypeLabel: this.appointmentTypeHelper.getAppointmentTypeName(
        item?.appointment_type_id,
      ),
      sourceLabel: this.sourceHelper.getSourceName(item?.source_id),
      serviceInfo: this.servicesHelperService.getServiceInfo(item?.service_id!),
      appointmentReasonInfo: this.appointmentReasonHelperService.getAppointmentReasonInfo(
        item?.appointment_reason_id!,
      ),
      appointmentListDate: formattedList,
    };
  }

  async addLabels(items: AppointmentModel[]) {
    if (!Array.isArray(items)) return [];
    await this.ensureCacheLoaded();
    return items.map((item) => this.addLabel(item));
  }

  async ensureCacheLoaded() {
    await Promise.all([
      this.branchHelper.ensureBranchesLoaded(),
      this.userHelper.ensureUsersLoaded(),
      this.serviceTypeHelper.ensureServiceTypesLoaded(),
      this.locationProvinceHelper.ensureLocationProvincesLoaded(),
      this.appointmentIsAcceptedHelper.ensureAppointmentIsAcceptedsLoaded(),
      this.appointmentTypeHelper.ensureAppointmentTypesLoaded(),
      this.sourceHelper.ensureSourcesLoaded(),
      this.servicesHelperService.ensureServicesLoaded(),
      this.appointmentReasonHelperService.ensureAppointmentReasonLoaded(),
    ]);
  }
}
