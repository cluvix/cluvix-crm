import {ChangeDetectionStrategy, Component, effect, inject, OnDestroy, OnInit, signal, ViewChild } from '@angular/core';
import { Table } from 'primeng/table';
import { Observable, finalize, switchMap } from 'rxjs';

import { CustomerTimeline } from '@/modules/crm/customer/contact-list/customer-timeline/customer-timeline';
import { ExportOptionsDialog, ExportMode } from '@/modules/utilities/export-options-dialog/export-options-dialog';
import { NoteDisplayModel, NoteModel } from '@/core/models/crm/note.model';
import { NoteService } from '@/core/services/crm/note.service';
import { BaseListV2 } from '@/share/base/base-list-v2';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { ListShareImports } from '@/share/import/list-share-imports';
import { CustomerStageMultiselectFormControl } from '@/indexeddb/config/components/customer-stage';
import { ListApiResponse } from '@/share/models/list-api-response';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';


@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-note-note-list',
  templateUrl: './note-list.html',
  styles: [
    `
      :host ::ng-deep .p-datatable .p-datatable-tbody > tr.p-highlight {
        background: #e3f2fd;
        color: #495057;
      }
    `,
  ],
  standalone: true,
  imports: [FillHeightDirective, CustomerStageMultiselectFormControl, ...ListShareImports],
})
export class NoteList extends BaseListV2<NoteDisplayModel> implements OnInit, OnDestroy {
  modelService = inject(NoteService);
  @ViewChild('dt') dt!: Table;
  keysToFilter = ['user'];
  displayCols: any = [
    { label: 'ID', value: 'id' },
    { label: this.translate.instant('common.label.customer_code'), value: 'code' },
    { label: this.translate.instant('common.label.customer'), value: 'fullname' },
    { label: this.translate.instant('common.label.phone'), value: 'phone' },
    { label: 'Nội dung ghi chú', labelKey: 'common.label.note_content', value: 'note' },
    { label: 'Trạng thái', labelKey: 'common.label.status', value: 'statusName' },
    { label: 'Người tạo', labelKey: 'common.label.user_created', value: 'created_by' },
    { label: 'Ngày tạo', labelKey: 'common.label.created_at', value: 'created_at' },
  ];
  override selectedColumns = signal([
    'fullname',
    'note',
    'statusName',
  ]);
  protected override LS_SELECTED_COLUMNS_KEY = '/note/app-note-note-list';
  protected override URL_STATE_PREFIX = 'nt';

  constructor() {
    super();
    effect(() => {
      const len = this.selectedColumns()?.length ?? 0;
      this.padCols.set(len + 2);
    });
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
        name: 'created_at',
        kind: 'dateRange',
        // factory: chỉ áp lúc mở màn / bấm lại menu (tính theo hôm nay); "Xoá bộ lọc" xoá về rỗng
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayStart()],
      },
      {name: 'created_by', kind: 'in'},
      {name: 'type_service_id', kind: 'in'},
      // Tiến trình khách = bước cấu hình (customer_stage), chọn nhiều: gửi `_stage_id[i]=…`
      // (BE story-05, lọc `customer.stage_id`). URL `nt_stage_id=…` theo quy ước urlKeyOf.
      {name: '_stage_id', kind: 'in'},
    ];
  }

  /**
   * Override `fetchList` chứ KHÔNG override `searchHandler`: base cho mọi lần tải đi qua một
   * stream có `switchMap` để huỷ request cũ, và tự lo `loading`/`models`/`paginationInfo`.
   */
  protected override fetchList(): Observable<ListApiResponse<NoteDisplayModel>> {
    return this.modelService.getAllNote({ params: this.searchParams() }).pipe(
      switchMap(async (response: any) => {
        if (response.items) {
          response.items = await this.modelService.addLabels(response.items);
        }
        return response;
      }),
    );
  }

  timelineCustomer(event: Event, id: number, fullname: string) {
    this.dialogRef = this.dialogWrapperService.open(CustomerTimeline, {
      width: '960px',

      data: {
        customerId: id,
      },
      header: fullname,
    });
  }

  onQuickSelect({ start, end }: { start: Date; end: Date }) {
    this.setFilterValue('created_at', [start, end]);
  }

  exportExcel() {
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
      this.isSubmitting.set(true);
      this.loadingService.showLoading('exportNote');
      const params = {...this.exportParams()} as any;
      if (mode === 'selected') {
        params['columns'] = this.selectedColumns().join(',');
      }
      this.modelService
        .exportNote({
          params,
        })
        .pipe(
          finalize(() => {
            this.isSubmitting.set(false);
            this.loadingService.hideLoading('exportNote');
          }),
        )
        .subscribe({
          next: (res: any) => {
            this.noti.successMsg(this.translate.instant('common.message.export_success'));
            this.downloadExport(res);
          },
        });
    });
  }

  clearStartTime() {
    this.setFilterValue('created_at', null);
  }
}
