import {ChangeDetectionStrategy, Component, effect, inject, Input, OnInit, ViewChild, signal} from '@angular/core';
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
  selector: 'app-report-marketing-revenue',
  templateUrl: './revenue.html',
  styles: [],
  imports: [
    FillHeightDirective,
    ...ListShareImports,
  ],
  standalone: true,
})
export class Revenue extends BaseListV2<any> implements OnInit {
  @ViewChild('dt') dt!: Table;
  protected override URL_STATE_PREFIX = 'rev';
  modelsTotal = signal<any>({
    totalAppointment: 0,
    totalAppointmentDen: 0,
    totalAppointmentDongY: 0,
    denLichHen: 0,
    dongYDen: 0,
    totalOrderPlan: 0,
    totalThanhToan: 0,
    totalDepositMoney: 0,
    tbKhach: 0,
  }); // khởi tạo giá trị ban đầu
  modelService = inject(ReportCrmService);
  protected override LS_SELECTED_COLUMNS_KEY = '/statistic/app-report-marketing-revenue';

  constructor() {
    super();
    this.sort.set('-totalAppointment');
    effect(() => {
      this.padCols.set(12);
    });
  }

  _selectedCols!: any[];

  @Input() get selectedCols(): any[] {
    return this._selectedCols;
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
      {
        name: 'created_at',
        kind: 'dateRange',
        required: true, // BE bắt buộc dải ngày (422 khi thiếu) → Xoá bộ lọc về mặc định
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayStart()],
      },
    ];
  }

  /**
   * Lấy danh sách hiệu quả telesale
   */
  protected override fetchList(): Observable<ListApiResponse<any>> {
    return this.modelService
      .revenue({
        params: this.searchParams(),
      })
      .pipe(
        map((response: any) => {
          const items = response.items ?? [];
          const totals = response.totals ?? {};

          const modelsTemp = items.map((item: any) => ({
            id: item.id,
            fullname: item.fullname,
            totalAppointment: item.totalAppointment,
            totalAppointmentDen: item.totalAppointmentDen,
            totalAppointmentDongY: item.totalAppointmentDongY,
            denLichHen: +item.totalAppointment == 0 ? 0 : (+item.totalAppointmentDen * 100) / +item.totalAppointment,
            dongYDen: +item.totalAppointmentDen == 0 ? 0 : (+item.totalAppointmentDongY * 100) / +item.totalAppointmentDen,
            totalOrderPlan: item.totalOrderPlan,
            totalThanhToan: item.totalThanhToan,
            totalDepositMoney: item.totalDepositMoney,
            tbKhach: +item.totalAppointmentDongY == 0 ? 0 : +item.totalOrderPlan / +item.totalAppointmentDongY,
          }));

          this.modelsTotal.set({
            totalAppointment: +totals.totalAppointment || 0,
            totalAppointmentDen: +totals.totalAppointmentDen || 0,
            totalAppointmentDongY: +totals.totalAppointmentDongY || 0,
            totalOrderPlan: +totals.totalOrderPlan || 0,
            totalThanhToan: +totals.totalThanhToan || 0,
            totalDepositMoney: +totals.totalDepositMoney || 0,
            denLichHen: +totals.denLichHen || 0,
            dongYDen: +totals.dongYDen || 0,
            tbKhach: +totals.tbKhach || 0,
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
    this.rfFilterModel.patchValue({created_at: [start, end]});
  }

  /**
   * Nút xoá ngày KHÔNG được để bộ lọc rỗng: BE nay bắt buộc có dải `created_at` và chặn trần
   * 1 năm — không dải ngày thì CẢ 4 nhánh của báo cáo đều quét cạn bảng của mình (đo
   * 1.218.228 dòng InnoDB + 743.786 dòng bảng tạm @872.284 lịch hẹn / 596.680 đơn liệu trình /
   * 372.504 payment), nên xoá về null chỉ đổi một màn treo lấy một toast 422. Xoá = quay về
   * mặc định (đầu tháng → hôm nay), đúng thứ người dùng thấy khi mới mở màn.
   */
  clearStartTime() {
    this.setFilterValue('created_at', [
      this.timezoneService.monthStart(),
      this.timezoneService.todayStart(),
    ]);
  }
}
