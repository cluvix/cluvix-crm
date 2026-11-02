import { ChangeDetectionStrategy, Component, effect, inject, OnDestroy, OnInit, signal, ViewChild } from '@angular/core';
import { Table } from 'primeng/table';
import { finalize, Observable, switchMap } from 'rxjs';

import {
  AppointmentConsultationDisplayModel,
} from '@/core/models/crm/appointment-consultation-report.model';
import { AppointmentConsultationReportService } from '@/core/services/crm/appointment-consultation-report.service';
import {
  APPOINTMENT_STATUS_MAP,
  APPOINTMENT_STATUS_OPTIONS,
  APPOINTMENT_STATUS_SELECT,
} from '@/define/appointment.define';
import { BaseListV2 } from '@/share/base/base-list-v2';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { ListShareImports } from '@/share/import/list-share-imports';
import { ListApiResponse } from '@/share/models/list-api-response';
import { translateOptions } from '@/share/utils/translate-options.util';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-crm-report-appointment-consultation',
  templateUrl: './appointment-consultation.html',
  standalone: true,
  imports: [FillHeightDirective, ...ListShareImports],
})
export class AppointmentConsultation
  extends BaseListV2<AppointmentConsultationDisplayModel>
  implements OnInit, OnDestroy
{
  modelService = inject(AppointmentConsultationReportService);
  @ViewChild('dt') dt!: Table;

  displayCols: any = [
    { label: this.translate.instant('common.label.customer_code'), value: 'customer_code' },
    { label: this.translate.instant('common.label.customer'), value: 'customer_fullname' },
    { label: this.translate.instant('common.label.phone'), value: 'customer_phone' },
    { label: this.translate.instant('common.label.appointment_date'), value: 'appointment_date' },
    { label: this.translate.instant('config.appointment_type.appointment_type'), value: 'appointment_type' },
    { label: this.translate.instant('crm.appointment.label.checkin'), value: 'checkin_date' },
    { label: this.translate.instant('crm.appointment.label.checkout'), value: 'checkout_date' },
    { label: this.translate.instant('common.label.appointment_status'), value: 'status' },
    { label: this.translate.instant('common.label.accepted_service_group'), value: 'service_type_accepted' },
    { label: this.translate.instant('common.label.reason'), value: 'appointment_reason' },
    { label: this.translate.instant('common.label.note'), value: 'note' },
    { label: this.translate.instant('common.label.appointment_created_at'), value: 'created_at' },
    { label: this.translate.instant('common.label.user_created'), value: 'created_by' },
  ];
  override selectedColumns = signal([
    'customer_fullname',
    'customer_phone',
    'appointment_date',
    'appointment_type',
    'status',
    'service_type_accepted',
    'appointment_reason',
  ]);
  protected override LS_SELECTED_COLUMNS_KEY = '/crm/report/appointment-consultation';
  protected override URL_STATE_PREFIX = 'apc';
  protected readonly appointmentStatusOptions = translateOptions(APPOINTMENT_STATUS_OPTIONS);
  protected readonly appointmentStatusMap = APPOINTMENT_STATUS_MAP;

  constructor() {
    super();
    effect(() => {
      const len = this.selectedColumns()?.length ?? 0;
      this.padCols.set(len + 1);
    });
  }

  ngOnInit() {
    this.initListV2();
  }

  protected filterFields(): FilterFieldDef[] {
    return [
      {
        name: 'appointment_date',
        kind: 'dateRange',
        // factory: chỉ áp lúc mở màn / bấm lại menu (tính theo hôm nay); "Xoá bộ lọc" xoá về rỗng
        // Chốt ở HÔM NAY, không phải cuối tháng: đây là report kết quả tư vấn — lịch hẹn
        // tương lai chưa diễn ra nên luôn rơi vào "Chưa đến", làm loãng tỉ lệ chốt.
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayEnd()],
      },
      { name: 'branch_id', kind: 'in' },
      {
        name: 'status',
        kind: 'in',
        // Mặc định chỉ khách ĐÃ ĐẾN — report này đo kết quả tư vấn nên khách chưa tới
        // quầy thì chưa có kết quả để đọc. KHÔNG lọc cứng ở BE: bỏ chọn là xem lại được
        // "Chưa đến" (= no-show trong khoảng đã qua) và "Hủy".
        default: () => [
          APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_CHECKED_IN,
          APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_CHECKED_OUT,
          APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_ADVISE,
          APPOINTMENT_STATUS_SELECT.APPOINTMENT_STATUS_TREATMENT,
        ],
      },
    ];
  }

  /**
   * Override `fetchList` chứ KHÔNG override `searchHandler`: base cho mọi lần tải đi qua một
   * stream có `switchMap` để huỷ request cũ, và tự lo `loading`/`models`/`paginationInfo`.
   */
  protected override fetchList(): Observable<ListApiResponse<AppointmentConsultationDisplayModel>> {
    return this.modelService.list({ params: this.searchParams() }).pipe(
      switchMap(async (response: any) => {
        if (response.items) {
          response.items = await this.modelService.addLabels(response.items);
        }
        return response;
      }),
    );
  }

  onQuickSelect({ start, end }: { start: Date; end: Date }) {
    this.setFilterValue('appointment_date', [start, end]);
  }

  clearAppointmentDate() {
    this.setFilterValue('appointment_date', null);
  }

  /**
   * Export full — KHÔNG gửi `columns=`: bộ cột export do BE (`ExportAppointment`)
   * tự quyết định, không nhất thiết trùng 1-1 với cột đang hiển thị trên UI
   * (xem architecture.md "Rủi ro & đánh đổi") — tránh bug lệch cột dạng
   * `feedback_export_core_columns`.
   */
  exportExcel() {
    this.isSubmitting.set(true);
    this.loadingService.showLoading('exportAppointmentConsultation');
    const params = {...this.exportParams()} as any;
    this.modelService
      .exportAppointmentConsultation({ params })
      .pipe(
        finalize(() => {
          this.isSubmitting.set(false);
          this.loadingService.hideLoading('exportAppointmentConsultation');
        }),
      )
      .subscribe({
        next: (res: any) => {
          this.noti.successMsg(this.translate.instant('common.message.export_success'));
          this.downloadExport(res);
        },
      });
  }
}
