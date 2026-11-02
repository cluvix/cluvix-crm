import {ChangeDetectionStrategy, Component, inject, Input, OnInit, ViewChild} from '@angular/core';
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
  selector: 'app-report-marketing-effective-advise',
  templateUrl: './effective-advise.html',
  styles: [],
  imports: [
    FillHeightDirective,
    ...ListShareImports,
  ],
  standalone: true,
})
export class EffectiveAdvise extends BaseListV2<any> implements OnInit {
  modelService = inject(ReportCrmService);
  @ViewChild('dt') dt!: Table;
  protected override URL_STATE_PREFIX = 'efa';

  totalIsAccepted: number = 0; // tổng đồng ý

  constructor() {
    super();
    this.sort.set('-isAccepted');
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
   * Lấy danh sách report theo hiệu quả tư vấn
   */
  protected override fetchList(): Observable<ListApiResponse<any>> {
    return this.modelService
      .effectiveAdvise({
        params: this.searchParams(),
      })
      .pipe(
        map((response: any) => {
          const items = response.items ?? [];
          const totals = response.totals ?? {};
          this.totalIsAccepted = +totals.totalIsAccepted || 0;

          const modelsTemp = items.map((item: any) => ({
            name: item.name,
            isAccepted: item.isAccepted,
            tyle: this.totalIsAccepted === 0 ? 0 : (+item.isAccepted * 100) / this.totalIsAccepted,
          }));

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
   * 1 năm. Bỏ dải ngày làm optimizer rời chỉ mục ngày và fetch row ngẫu nhiên 57.384 lần —
   * đo 0,18s lúc cache nóng nhưng **6,8–14,4s lúc cache lạnh**, mà báo cáo mở thưa nên cache
   * lạnh mới là trạng thái bình thường. Xoá = quay về mặc định (đầu tháng → hôm nay).
   */
  clearStartTime() {
    this.setFilterValue('created_at', [
      this.timezoneService.monthStart(),
      this.timezoneService.todayStart(),
    ]);
  }
}
