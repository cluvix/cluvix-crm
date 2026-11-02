import {ChangeDetectionStrategy, Component, inject, OnInit, ViewChild, signal} from '@angular/core';
import {Table} from 'primeng/table';
import {Observable, map} from 'rxjs';

import {BaseListV2} from '@/share/base/base-list-v2';
import {FilterFieldDef} from '@/share/base/list-filter.model';
import {ListApiResponse} from '@/share/models/list-api-response';
import {ListShareImports} from '@/share/import/list-share-imports';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';
import {ReportCrmService} from "@/core/services/report/report-crm.service";

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-appointment-app-statistics',
  templateUrl: './app-statistics.html',
  styles: [' '],
  standalone: true,
  imports: [
    FillHeightDirective,
    ...ListShareImports,
  ],
})
export class AppStatistics extends BaseListV2<any> implements OnInit {
  modelService = inject(ReportCrmService);
  @ViewChild('dt') dt!: Table;
  protected override URL_STATE_PREFIX = 'as';

  // Khởi tạo giá trị ban đầu
  modelsTotal = signal<any>({
    numAppNew: 0,
    numAppNewPlus: 0,
    numAppDate: 0,
    numAppDen: 0,
    numAppCancel: 0,
    numAppDenOk: 0,
    numAppDenNumAppDate: 0,
    numAppDenOkNumAppDen: 0,
  });
  keysToFilter = ['user'];

  constructor() {
    super();
  }

  ngOnInit() {
    this.initListV2();
  }

  /**
   * Khai báo filter — base tự sinh FormGroup, đấu valueChanges → dt.filter(),
   * dựng filters khởi tạo cho p-table, lo clearFilter() và đồng bộ URL.
   *
   * `_created_at` có gạch dưới đầu → UtilsService.buildFilter tự xuất dạng
   * `_created_at[gte]`/`_created_at[lte]` (KHÔNG bọc `filter[]`), khớp đúng
   * `c.Query("_created_at[gte]")` BE đang đọc raw — giữ nguyên tên control, không đổi.
   */
  protected filterFields(): FilterFieldDef[] {
    return [
      {
        name: '_created_at',
        kind: 'dateRange',
        required: true, // BE bắt buộc dải ngày (422 khi thiếu) → Xoá bộ lọc về mặc định
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayStart()],
      },
      {name: 'branch_id', kind: 'in'},
      {name: 'appointment_type_id', kind: 'in'},
    ];
  }

  /**
   * Lấy danh sách thống kê
   */
  protected override fetchList(): Observable<ListApiResponse<any>> {
    return this.modelService
      .getStatisticsAppointment({
        params: this.searchParams(),
      })
      .pipe(
        map((response: any) => {
          const items = response.items ?? [];
          const totals = response.totals ?? {};

          const modelsTemp = items.map((item: any) => ({
            dayCreate: item.dayCreate,
            numAppNew: item.numAppNew,
            numAppNewPlus: item.numAppNewPlus,
            numAppDate: item.numAppDate,
            numAppDen: item.numAppDen,
            numAppCancel: item.numAppCancel,
            numAppDenOk: item.numAppDenOk,
            numAppDenNumAppDate: +item.numAppDate === 0 ? 0 : (+item.numAppDen * 100) / +item.numAppDate,
            numAppDenOkNumAppDen: +item.numAppDen === 0 ? 0 : (+item.numAppDenOk * 100) / +item.numAppDen,
          }));

          this.modelsTotal.set({
            numAppNew: +totals.numAppNew || 0,
            numAppNewPlus: +totals.numAppNewPlus || 0,
            numAppDate: +totals.numAppDate || 0,
            numAppDen: +totals.numAppDen || 0,
            numAppCancel: +totals.numAppCancel || 0,
            numAppDenOk: +totals.numAppDenOk || 0,
            numAppDenNumAppDate: +totals.numAppDate === 0 ? 0 : (+totals.numAppDen * 100) / +totals.numAppDate,
            numAppDenOkNumAppDen: +totals.numAppDen === 0 ? 0 : (+totals.numAppDenOk * 100) / +totals.numAppDen,
          });

          return {
            ...response,
            items: modelsTemp,
            _meta: response._meta ?? {
              totalCount: modelsTemp.length,
              pageCount: 1,
              currentPage: 1,
              perPage: modelsTemp.length,
            },
          } as ListApiResponse<any>;
        }),
      );
  }

  onQuickSelect({start, end}: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({_created_at: [start, end]});
  }

  /**
   * Nút xoá ngày KHÔNG được để bộ lọc rỗng: BE nay bắt buộc có dải ngày và chặn trần 1 năm
   * (không dải ngày = quét cạn bảng `appointment` — đo 2.729.935 dòng InnoDB @1,09M lịch hẹn),
   * nên xoá về null chỉ đổi được một màn chậm lấy một toast 422. Xoá = quay về mặc định
   * (đầu tháng → hôm nay), đúng thứ người dùng thấy khi mới mở màn.
   */
  clearStartTime() {
    this.rfFilterModel.patchValue({
      _created_at: [this.timezoneService.monthStart(), this.timezoneService.todayStart()],
    });
  }
}
