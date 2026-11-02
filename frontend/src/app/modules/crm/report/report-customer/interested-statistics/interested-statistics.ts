import {ChangeDetectionStrategy, Component, effect, inject, Input, OnInit, ViewChild, signal } from '@angular/core';
import {Table} from 'primeng/table';
import {map, Observable} from 'rxjs';
import {BaseListV2} from '@/share/base/base-list-v2';
import {FilterFieldDef} from '@/share/base/list-filter.model';
import {ListApiResponse} from '@/share/models/list-api-response';
import {ListShareImports} from '@/share/import/list-share-imports';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';
import {ReportCrmService} from "@/core/services/report/report-crm.service";

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-statistics-customer-interested-statistics',
  templateUrl: './interested-statistics.html',
  styles: [],
  imports: [
    FillHeightDirective,
    ...ListShareImports,

  ],
  standalone: true,
})
export class InterestedStatistics extends BaseListV2<any> implements OnInit {
  modelService = inject(ReportCrmService);
  @ViewChild('dt') dt!: Table;
  // khởi tạo ban đầu
  modelsTotal = signal<any>({
    totalLead: 0,
    totalAppointmentNew: 0,
    totalAppointment: 0,
    totalAppointmentDen: 0,
    totalAppointmentDongY: 0,
    appointmentNewLead: 0,
    denAppointment: 0,
    dongYDen: 0,
    totalOrderPlan: 0,
    totalRevenue: 0,
    totalPayment: 0,
    totalDeposit: 0,
    totalRefundDeposit: 0,
    totalRefundServiceUsed: 0,
    duKienThu: 0,
    tbKhach: 0,
  });
  protected override LS_SELECTED_COLUMNS_KEY =
    '/report/app-statistics-customer-interested-statistics';
  protected override URL_STATE_PREFIX = 'sin';

  constructor() {
    super();
    this.sort.set('-lead');
    effect(() => {
      this.padCols.set(10);
    });
  }

  _selectedCols!: any[];

  @Input() get selectedCols(): any[] {
    return this._selectedCols;
  }

  set selectedCols(val: any[]) {
    this._selectedCols = this.cols().filter((col: any) => val.includes(col));
  }

  ngOnInit() {
    this.initListV2();
  }

  onQuickSelect({start, end}: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({created_at: [start, end]});
  }

  /**
   * Lấy danh sách report theo nhóm dịch vụ quan tâm
   */

  /**
   * Khai báo filter — base tự sinh FormGroup, đấu valueChanges → dt.filter(),
   * dựng filters khởi tạo cho p-table, lo clearFilter() và đồng bộ URL.
   */
  protected filterFields(): FilterFieldDef[] {
    return [
      {
        name: 'created_at',
        kind: 'dateRange',
        required: true, // BE bắt buộc dải ngày (422 khi thiếu) → Xoá bộ lọc về mặc định
        // factory: clearFilter() tính lại theo hôm nay, không dính ngày lúc mở trang
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayEnd()],
      },
    ];
  }

  /**
   * Override `fetchList` chứ KHÔNG override `searchHandler`: base cho mọi lần tải đi qua một
   * stream có `switchMap` để huỷ request cũ, và tự lo `loading`/`models`/`paginationInfo`.
   */
  protected override fetchList(): Observable<ListApiResponse<any>> {
    return this.modelService.getInterestedStatistics({params: this.searchParams()}).pipe(
      map((response: any) => {
        const items = response.items ?? [];
        const totals = response.totals ?? {};

        const modelsTemp = items.map((item: any) => ({
          name: item.name,
          totalLead: item.totalLead,
          totalAppointmentNew: item.totalAppointmentNew,
          totalAppointment: item.totalAppointment,
          totalAppointmentDen: item.totalAppointmentDen,
          totalAppointmentDongY: item.totalAppointmentDongY,
          appointmentNewLead: +item.totalLead === 0 ? 0 : (+item.totalAppointmentNew * 100) / +item.totalLead,
          denAppointment: +item.totalAppointment === 0 ? 0 : (+item.totalAppointmentDen * 100) / +item.totalAppointment,
          dongYDen: +item.totalAppointmentDen === 0 ? 0 : (+item.totalAppointmentDongY * 100) / +item.totalAppointmentDen,
          totalOrderPlan: item.totalOrderPlan,
          totalRevenue: item.totalRevenue,
          totalPayment: item.totalPayment,
          totalDeposit: item.totalDeposit,
          totalRefundDeposit: item.totalRefundDeposit,
          totalRefundServiceUsed: item.totalRefundServiceUsed,
          duKienThu: Number(item.duKienThu),
          tbKhach: Number(item.tbKhach),
        }));

        this.modelsTotal.set({
          totalLead: +totals.totalLead || 0,
          totalAppointmentNew: +totals.totalAppointmentNew || 0,
          totalAppointment: +totals.totalAppointment || 0,
          totalAppointmentDen: +totals.totalAppointmentDen || 0,
          totalAppointmentDongY: +totals.totalAppointmentDongY || 0,
          appointmentNewLead: +totals.appointmentNewLead || 0,
          denAppointment: +totals.denAppointment || 0,
          dongYDen: +totals.dongYDen || 0,
          totalOrderPlan: +totals.totalOrderPlan || 0,
          totalRevenue: +totals.totalRevenue || 0,
          totalPayment: +totals.totalPayment || 0,
          totalDeposit: +totals.totalDeposit || 0,
          totalRefundDeposit: +totals.totalRefundDeposit || 0,
          totalRefundServiceUsed: +totals.totalRefundServiceUsed || 0,
          duKienThu: +totals.duKienThu || 0,
          tbKhach: +totals.tbKhach || 0,
        });

        return {
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

  /**
   * Nút xoá ngày KHÔNG được để bộ lọc rỗng: `dimensionStatistics` (BE, dùng chung 8 màn
   * `*-statistics`) nay bắt buộc có dải `created_at` và chặn trần 1 năm. Kể cả sau khi đã gộp
   * 7 lượt quét thành 3, ca không-lọc-ngày vẫn 759.208 dòng InnoDB / 0,79–1,16s @872.284 lịch
   * hẹn (bản `source-statistics` nặng hơn: 2.963.026 dòng / 6,1–14,4s). Xoá = quay về mặc định
   * (đầu tháng → hết hôm nay), đúng thứ người dùng thấy khi mới mở màn.
   */
  clearStartTime() {
    this.setFilterValue('created_at', [
      this.timezoneService.monthStart(),
      this.timezoneService.todayEnd(),
    ]);
  }
}
