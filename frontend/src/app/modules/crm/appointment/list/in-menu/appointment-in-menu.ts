import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  EventEmitter,
  inject,
  Input,
  OnInit,
  Output,
  signal,
  ViewChild,
  viewChild,
} from '@angular/core';
import {takeUntilDestroyed} from '@angular/core/rxjs-interop';
import {Table} from 'primeng/table';
import {Observable, finalize, from, map, switchMap} from 'rxjs';

import {ExportOptionsDialog, ExportMode} from '@/modules/utilities/export-options-dialog/export-options-dialog';
import {AppointmentDetailPanel} from '@/modules/crm/appointment/detail-panel/appointment-detail-panel';
import {FormAppointmentCancel} from '@/modules/crm/appointment/form/appointment-cancel/form-appointment-cancel';
import {Checkin} from '@/modules/crm/appointment/form/check-in/checkin';
import { FormAppointmentV2 } from '@/modules/crm/appointment/form/form-appointment/form-appointment-v2';
import {CustomerTimeline} from '@/modules/crm/customer/contact-list/customer-timeline/customer-timeline';
import {AppointmentModel} from '@/core/models/crm/appointment.model';
import {AppointmentService} from '@/core/services/crm/appointment.service';
import {BaseListV2} from '@/share/base/base-list-v2';
import { detailPanelHost } from '@/share/base/detail-panel-host';
import {FilterFieldDef} from '@/share/base/list-filter.model';
import {
  APPOINTMENT_COLOR_STATUS_SELECT,
  APPOINTMENT_STATUS_OPTIONS,
  APPOINTMENT_STATUS_SELECT,
  CONFIRM_APPOINTMENT_SELECT,
  CUSTOMER_TYPE_OPTION,
  GENDER_OPTIONS,
  PERMISSION_SELECT,
} from '@/define';
import {WhitespaceControlDirective} from '@/share/directive/whitespace-control.directive';
import {ListShareImports} from '@/share/import/list-share-imports';
import {isCheckinAble} from '@/share/utils/appointment-checkin.util';
import {translateOptions} from '@/share/utils/translate-options.util';
import {ListApiResponse} from '@/share/models/list-api-response';
import {DepartementMultiselectFromControl} from '@/indexeddb/config/components/hr-department';
import {CustomerCategoryMultiselectFormControl} from '@/indexeddb/config/components/customer-category';
import {GroupSourceMultiselectFormControl} from '@/indexeddb/config/components/group-source';
import {GroupLocalMultiselectFromControl} from '@/indexeddb/config/components/group-local';
import {GroupCountryMultiselectFromControl} from '@/indexeddb/config/components/group-country';
import {CustomerStageMultiselectFormControl} from '@/indexeddb/config/components/customer-stage';
import {ConfirmCheckin} from '@/modules/crm/appointment/form/confirm-checkin/confirm-checkin';
import {FillHeightDirective} from "@/share/directive/fill-height.directive";

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-appointment-list-in-menu',
  templateUrl: './appointment-in-menu.html',
  standalone: true,
  imports: [
    ...ListShareImports,
    AppointmentDetailPanel,
    WhitespaceControlDirective,
    DepartementMultiselectFromControl,
    CustomerCategoryMultiselectFormControl,
    GroupSourceMultiselectFormControl,
    GroupLocalMultiselectFromControl,
    GroupCountryMultiselectFromControl,
    CustomerStageMultiselectFormControl,
    FillHeightDirective,
  ],
})
export class AppointmentInMenu extends BaseListV2<AppointmentModel> implements OnInit {
  modelService = inject(AppointmentService);
  @ViewChild('dt') dt!: Table<AppointmentModel>;
  @Input() appointmentForLead: boolean = false;
  @Output() updateAppointment: EventEmitter<AppointmentModel> =
    new EventEmitter<AppointmentModel>(); // EventEmitter khi thay đổi status
  displayCols: any = [
    {label: 'ID', value: 'id'},
    {label: this.translate.instant('common.label.customer_code'), value: 'codeCustomer'},
    {label: this.translate.instant('common.label.old_code'), value: 'codeOldCustomer'},
    {label: 'Thời gian', labelKey: 'common.label.time', value: 'appointment_time'},
    {label: this.translate.instant('common.label.customer'), value: 'fullname'},
    {label: 'Mã lịch hẹn', labelKey: 'common.label.appointment_code', value: 'appointment_code'},
    {label: this.translate.instant('common.label.phone'), value: 'phone'},
    {label: 'Phân loại', labelKey: 'common.label.category', value: 'bookType'},
    {label: 'Loại lịch hẹn', labelKey: 'common.label.appointment_type', value: 'appointmentType'},
    {label: 'Ngày hẹn', labelKey: 'common.label.appointment_date', value: 'appointment_date'},
    {label: 'Ngày hẹn đầu', labelKey: 'common.label.first_appointment_date', value: 'appointment_date_first'},
    {label: 'Ngày hẹn đã đổi', labelKey: 'common.label.changed_appointment_date', value: 'appointmentListDate'},
    {label: 'Xác nhận lịch', labelKey: 'crm.appointment_list.confirm', value: 'confirmAppointment'},
    {label: 'Lần hẹn', labelKey: 'common.label.appointment_count', value: 'appointment_number'},
    {label: 'Nội dung hẹn', labelKey: 'common.label.appointment_content', value: 'content'},
    {label: 'Ghi chú nội bộ', labelKey: 'common.label.internal_note', value: 'contentPrivate'},
    {label: 'Checkin', value: 'checkin_date'},
    {label: 'Checkout', value: 'checkout_date'},
    {
      label: 'Nhóm dịch vụ quan tâm',
      labelKey: 'common.label.service_group_of_interest',
      value: 'typeServiceInterested'
    },
    {label: 'Dịch vụ hẹn', labelKey: 'common.label.appointment_service', value: 'orderServiceUsed'},
    {label: 'Nguồn khách hàng', labelKey: 'common.label.source', value: 'source'},
    {label: this.translate.instant('common.label.convert'), value: 'convert_id'},
    {label: 'Tư vấn viên', labelKey: 'common.label.consultant', value: 'revenue_id'},
    {label: 'Trạng thái', labelKey: 'common.label.status', value: 'appointmentIsAccepted'},
    {label: 'Lý do', labelKey: 'common.label.reason', value: 'noteFail'},
    {label: 'Dịch vụ đồng ý', labelKey: 'crm.appointment_list.service_accepted', value: 'type_service_accepted_id'},
    {label: 'Tỉnh thành', labelKey: 'common.label.province', value: 'province'},
    {label: this.translate.instant('common.label.branch'), value: 'branch'},
    {label: 'Người tạo', labelKey: 'common.label.user_created', value: 'created_by'},
    {label: 'Ngày tạo', labelKey: 'common.label.created_at', value: 'created_at'},
  ]; // dùng để hiển thị column table

  // SlideOver chi tiết lịch hẹn
  protected readonly detail = detailPanelHost<AppointmentModel>();
  private readonly detailPanel = viewChild(AppointmentDetailPanel);
  override selectedColumns = signal([
    'appointment_time',
    'fullname',
    'appointmentType',
    'appointment_date',
    'note',
    'typeServiceInterested',
  ]);
  visibleColumns = computed(() => {
    const set = new Set(this.selectedColumns());
    return Object.fromEntries(
      this.displayCols.map((c: any) => [c.value, set.has(c.value)])
    ) as Record<string, boolean>;
  });
  protected override LS_SELECTED_COLUMNS_KEY = '/appointment/app-appointment-list-in-menu';

  /** Lọc bỏ cột đã lưu localStorage không còn trong `displayCols` (vd `marketing_id` đã gỡ 2026-10-18). */
  override loadSelectedColumns() {
    super.loadSelectedColumns();
    const valid = new Set<string>(this.displayCols.map((c: { value: string }) => c.value));
    this.selectedColumns.update((cols) => cols.filter((c) => valid.has(c)));
  }
  protected override URL_STATE_PREFIX = 'ap';
  protected readonly genderOptions = translateOptions(GENDER_OPTIONS);
  protected readonly customerTypeOptions = translateOptions(CUSTOMER_TYPE_OPTION);
  protected readonly appointmentStatusOptions = translateOptions(APPOINTMENT_STATUS_OPTIONS);
  protected readonly APPOINTMENT_STATUS_SELECT = APPOINTMENT_STATUS_SELECT;
  protected readonly APPOINTMENT_COLOR_STATUS_SELECT = APPOINTMENT_COLOR_STATUS_SELECT;
  protected readonly CONFIRM_APPOINTMENT_SELECT = CONFIRM_APPOINTMENT_SELECT;

  constructor() {
    super();
    this.sort.set('appointment_date');
    effect(() => {
      const len = this.selectedColumns()?.length ?? 0;
      this.padCols.set(len + 2);
    });
  }

  rowTrackBy = (_: number, r: AppointmentModel) => r.id;

  /**
   * Gắn flag _isCheckinAble cho 1 row sau update — searchHandler chỉ set lúc load đầu,
   * updateRow replace row → cần re-compute để giữ button "Khách đến" / cancel logic.
   */
  private attachCheckinAble<T extends AppointmentModel>(model: T): T {
    const today = this.timezoneService.now();
    (model as any)._isCheckinAble = isCheckinAble(
      model,
      today,
      APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_CANCEL,
      APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_CHECKED_OUT,
    );
    return model;
  }

  ngOnInit() {
    this.initListV2();
  }

  /**
   * Khai báo filter — base tự sinh FormGroup, đấu valueChanges → dt.filter(),
   * dựng filters khởi tạo cho p-table, lo clearFilter() và đồng bộ URL.
   */
  protected filterFields(): FilterFieldDef[] {
    return [
      {name: '_keyword', kind: 'text'},
      {
        name: 'appointment_date',
        kind: 'dateRange',
        urlKey: 'date',
        // factory: chỉ áp lúc mở màn / bấm lại menu (tính theo hôm nay); "Xoá bộ lọc" xoá về rỗng
        default: () => [this.timezoneService.todayStart(), this.timezoneService.todayEnd()],
      },
      {name: 'appointment_date_first', kind: 'dateRange', urlKey: 'date_first'},
      {name: 'created_at', kind: 'dateRange'},
      // 4 field dưới đây là p-select CHỌN MỘT (template không còn [multiple], có [showClear])
      // → kind 'equals' để value là scalar và BE nhận filter[x][eq] thay vì mảng.
      {name: 'customerType', kind: 'equals'},
      {name: 'branch_id', kind: 'in'},
      {name: 'convert_id', kind: 'in'},
      {name: 'is_accepted', kind: 'in'},
      {name: 'type_service_id', kind: 'in'},
      {name: 'type_service_accepted_id', kind: 'in'},
      {name: 'appointment_type_id', kind: 'in'},
      {name: 'status', kind: 'equals'},
      {name: 'source_id', kind: 'in'},
      {name: 'category', kind: 'in'},
      {name: '_groupLocal', kind: 'in'},
      {name: '_groupCountry', kind: 'in'},
      {name: '_groupSource', kind: 'in'},
      // Tiến trình khách = bước cấu hình (customer_stage), nhiều giá trị: gửi `_stage_id[i]=…`
      // (BE story-05, JOIN customer + lọc `customer.stage_id`). URL `ap_stage_id=…` theo quy ước urlKeyOf.
      {name: '_stage_id', kind: 'in'},
      {name: '_gender', kind: 'equals'},
      {name: '_department', kind: 'in'},
      {
        name: 'created_by',
        kind: 'in',
        toFilter: (value: any) =>
          value == null || (Array.isArray(value) && !value.length)
            ? null
            : {value, matchMode: Array.isArray(value) ? 'in' : 'equals'},
      },
      {
        name: 'revenue_id',
        kind: 'in',
        // id=0 là sentinel "chưa có tư vấn viên" → đổi thành điều kiện IS NULL
        toFilter: (value: any) => {
          if (value == null || (Array.isArray(value) && !value.length)) return null;
          if (Array.isArray(value) && value.includes(0)) return {value: '1', matchMode: 'is'};
          return {value, matchMode: Array.isArray(value) ? 'in' : 'equals'};
        },
      },
    ];
  }

  /**
   * Override `fetchList` chứ KHÔNG override `searchHandler`: base cho mọi lần tải đi qua một
   * stream có `switchMap` để huỷ request cũ, và tự lo `loading`/`models`/`paginationInfo`.
   */
  protected override fetchList(): Observable<ListApiResponse<AppointmentModel>> {
    const today = this.timezoneService.now();
    const cancel = APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_CANCEL;
    const checkout = APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_CHECKED_OUT;

    return this.modelService.list({params: this.searchParams()}).pipe(
      switchMap((response: any) =>
        from(this.modelService.addLabels(response.items ?? [])).pipe(
          map((items: any[]) => {
            items.forEach((item: any) => {
              item._isCheckinAble = isCheckinAble(item, today, cancel, checkout);
            });
            return {...response, items} as ListApiResponse<AppointmentModel>;
          }),
        ),
      ),
    );
  }

  /**
   * Cập nhật lịch hẹn
   * @param appointment
   */
  async updateAppointmentDialog(appointment: AppointmentModel) {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdateAppointment)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    await this.modelService.ensureCacheLoaded();
    this.dialogRef = this.dialogWrapperService.open(FormAppointmentV2, {
      width: '960px',
      header: this.translate.instant('common.label.update_appointment'),
      data: {appointment},
    });
    this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((res: any) => {
      if (res) {
        const dataWithLabels = this.modelService.addLabel(res);
        this.updateRow(this.attachCheckinAble(dataWithLabels as any));
        // SlideOver chi tiết đang mở phía sau form sửa → nạp lại cho khớp dữ liệu vừa lưu.
        if (this.detail.visible()) this.detailPanel()?.reload();
      }
    });
  }

  /**
   * Timeline của khách hàng
   * @param event
   * @param id
   * @param fullname
   */
  timelineCustomer(event: Event, id: number, fullname: string) {
    this.dialogRef = this.dialogWrapperService.open(CustomerTimeline, {
      width: '960px',

      data: {
        customerId: id,
      },
      header: fullname,
    });
  }


  /**
   * Export csv lịch hẹn — mở dialog chọn chế độ export trước
   */
  exportAppointment() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionExtAppointment)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    const ref = this.dialogWrapperService.open(ExportOptionsDialog, {
      header: this.translate.instant('utilities.export_options_dialog.header'),
      width: '520px',
      data: {
        selectedCount: this.selectedColumns().length,
        totalCount: this.displayCols.length,
      },
    });

    ref.onClose.subscribe((mode: ExportMode | null) => {
      if (!mode) return;
      this.doExportAppointment(mode);
    });
  }

  private doExportAppointment(mode: ExportMode) {
    this.isSubmitting.set(true);
    this.loadingService.showLoading('exportAppointment');
    const params = {...this.exportParams()} as any;
    if (mode === 'selected') {
      params['columns'] = this.selectedColumns().join(',');
    }
    this.modelService
      .exportAppointment({
        params,
      })
      .pipe(
        finalize(() => {
          this.isSubmitting.set(false);
          this.loadingService.hideLoading('exportAppointment');
        }),
      )
      .subscribe({
        next: (res: any) => {
          this.noti.successMsg(this.translate.instant('common.message.export_success'));
          this.downloadExport(res);
        },
      });
  }

  onQuickSelectAppointmentDate({start, end}: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({appointment_date: [start, end]});
  }

  onQuickSelectAppointmentDateFirst({start, end}: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({appointment_date_first: [start, end]});
  }

  onQuickSelectCreatedAt({start, end}: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({created_at: [start, end]});
  }

  clearStartTime() {
    this.setFilterValue('appointment_date', null);
  }

  clearCreateStartTime() {
    this.setFilterValue('created_at', null);
  }

  /** Mở SlideOver chi tiết lịch hẹn — row hiện có dùng làm seed để vẽ ngay, panel tự gọi API detail. */
  appointmentDetail(model: AppointmentModel) {
    this.detail.open(model);
  }

  async confirmAppointment(model: AppointmentModel) {
    await this.modelService.ensureCacheLoaded();
    this.dialogRef = this.dialogWrapperService.open(ConfirmCheckin, {
      header: this.translate.instant('common.label.confirm_customer_arrival'),
      width: '700px',
      data: {model},
    });
    this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((data: AppointmentModel) => {
      if (!data) return;
      this.updateRow(this.attachCheckinAble(this.modelService.addLabel(data) as any));
    });
  }

  cancelAppoint(model: any) {
    this.dialogRef = this.dialogWrapperService.open(FormAppointmentCancel, {
      header: this.translate.instant('common.label.cancel_appointment'),
      width: '700px',
      data: {appointment: model},
    });
    this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((data: AppointmentModel) => {
      if (!data) return;
      this.updateRow(this.attachCheckinAble(this.modelService.addLabel(data) as any));
    });
  }

  filterByPosition(field: string, value: number) {
    // setValue bắn valueChanges → base tự filter + reload, không cần resetPage()
    this.setFilterValue(field, [value]);
  }

  /**
   * Khách hàng checkin
   * @param model
   */
  async customerCheckin(model: AppointmentModel) {
    if (this.isSubmitting()) return;
    this.isSubmitting.set(true);
    await this.modelService.ensureCacheLoaded();
    this.modelService
      .get(model.id!)
      .pipe(takeUntilDestroyed(this.destroyRef), finalize(() => this.isSubmitting.set(false)))
      .subscribe({
        next: (full: AppointmentModel) => {
          this.dialogRef = this.dialogWrapperService.open(Checkin, {
            header: this.translate.instant('common.label.customer_arrived'),
            width: '700px',
            data: {model: full},
          });
          this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((data: AppointmentModel) => {
            if (!data) return;
            this.updateRow(this.attachCheckinAble(this.modelService.addLabel(data) as any));
          });
        },
        error: (err: any) => {
          this.noti.errorMsg(err?.message || this.translate.instant('common.message.error'));
        },
      });
  }

}
