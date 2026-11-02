import { ChangeDetectionStrategy, Component, effect, inject, OnInit, signal, ViewChild } from '@angular/core';
import { Table } from 'primeng/table';
import { Observable, map } from 'rxjs';

import { BaseListV2 } from '@/share/base/base-list-v2';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { ListApiResponse } from '@/share/models/list-api-response';
import { ListShareImports } from '@/share/import/list-share-imports';
import { ReportCrmService } from '@/core/services/report/report-crm.service';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-report-service-type',
  templateUrl: './crm-report-service-type.html',
  styles: [],
  standalone: true,
  imports: [FillHeightDirective, ...ListShareImports],
})
export class CrmReportServiceType extends BaseListV2<any> implements OnInit {
  modelService = inject(ReportCrmService);
  @ViewChild('dt') dt!: Table;

  displayCols: any[] = [
    { label: 'Số KH', labelKey: 'common.label.customer_count', value: 'customerCount' },
    { label: 'Số DV', labelKey: 'common.label.service_count', value: 'numService' },
    { label: 'Số lượng', labelKey: 'common.label.quantity', value: 'quantity' },
    { label: 'Doanh số', labelKey: 'common.label.revenue', value: 'totalTotalEnd' },
    { label: 'Thực thu', labelKey: 'common.label.actual_revenue', value: 'totalPayment' },
    { label: 'Giảm dịch vụ', labelKey: 'common.label.service_discount', value: 'totalDiscountService' },
    { label: 'Giảm khác', labelKey: 'common.label.other_discount', value: 'totalDiscountOther' },
    { label: 'Hoàn DV', labelKey: 'common.label.service_refund', value: 'totalRefundServiceUsed' },
    { label: 'TB/DV', labelKey: 'crm.report_service_used.average_per_service', value: 'mediumService' },
  ];

  override selectedColumns = signal([
    'customerCount', 'numService', 'quantity',
    'totalTotalEnd', 'totalPayment', 'totalDiscountService',
    'totalDiscountOther', 'totalRefundServiceUsed', 'mediumService',
  ]);

  customerCount = signal(0);
  numService = signal(0);
  numQuantity = signal(0);
  totalTotalEnd = signal(0);
  totalPayment = signal(0);
  totalDiscountService = signal(0);
  totalDiscountOther = signal(0);
  totalRefundServiceUsed = signal(0);

  protected override LS_SELECTED_COLUMNS_KEY = '/report/app-report-service-type';
  protected override URL_STATE_PREFIX = 'rst';

  constructor() {
    super();
    this.sort.set('-totalEnd');
    effect(() => {
      this.padCols.set((this.selectedColumns()?.length ?? 0) + 2);
    });
  }

  ngOnInit() {
    this.initListV2();
  }

  /** Lọc bỏ cột đã lưu localStorage không còn trong `displayCols` (vd `rank_discount_amount` đã gỡ 2026-11-01). */
  override loadSelectedColumns() {
    super.loadSelectedColumns();
    const valid = new Set<string>(this.displayCols.map((c: { value: string }) => c.value));
    this.selectedColumns.update((cols) => cols.filter((c) => valid.has(c)));
  }

  /**
   * Khai báo filter — chỉ 1 field `created_day` (between), default tháng hiện tại → hôm nay.
   */
  protected filterFields(): FilterFieldDef[] {
    return [
      {
        name: 'created_day',
        kind: 'dateRange',
        // factory: chỉ áp lúc mở màn / bấm lại menu (tính theo hôm nay); "Xoá bộ lọc" xoá về rỗng
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayStart()],
      },
    ];
  }

  /**
   * Override `fetchList` chứ KHÔNG override `searchHandler`: base cho mọi lần tải đi qua một
   * stream có `switchMap` để huỷ request cũ, và tự lo `loading`/`models`/`paginationInfo`.
   */
  protected override fetchList(): Observable<ListApiResponse<any>> {
    return this.modelService.getServiceTypeStatistics({ params: this.searchParams() }).pipe(
      map((response: any) => {
        const t = response.totals ?? {};
        this.customerCount.set(+t.customerCount || 0);
        this.numService.set(+t.numService || 0);
        this.numQuantity.set(+t.quantity || 0);
        this.totalTotalEnd.set(+t.totalEnd || 0);
        this.totalPayment.set(+t.payment || 0);
        this.totalDiscountService.set(+t.service_discount_amount || 0);
        this.totalDiscountOther.set(+t.discount_other || 0);
        this.totalRefundServiceUsed.set(+t.refundServiceUsed || 0);

        const items = response.items ?? [];
        return {
          items,
          _meta: response._meta ?? {
            totalCount: items.length,
            pageCount: 1,
            currentPage: 1,
            perPage: items.length,
          },
        } as ListApiResponse<any>;
      }),
    );
  }

  onQuickSelect({ start, end }: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({ created_day: [start, end] });
  }

  clearStartTime() {
    this.setFilterValue('created_day', null);
  }
}
