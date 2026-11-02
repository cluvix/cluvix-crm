import {ChangeDetectionStrategy, Component, inject, OnInit, signal, ViewChild} from '@angular/core';
import {Table} from 'primeng/table';
import {finalize, map, Observable, tap} from 'rxjs';

import {ReportRevenueCrmService} from '@/core/services/crm/report-revenue-crm.service';
import {PrintService} from '@/prints/print.service';
import {BaseListV2} from '@/share/base/base-list-v2';
import {FilterFieldDef} from '@/share/base/list-filter.model';
import {FillHeightDirective} from '@/share/directive/fill-height.directive';
import {ListShareImports} from '@/share/import/list-share-imports';
import {ListApiResponse} from '@/share/models/list-api-response';
import {copyReportSummary, ReportSummaryConfig} from '@/share/utils/report-summary-export.util';

interface RevenueByDateSummary {
  dongY: number;
  doanhSo: number;
  thucThu: number;
  deposit: number;
  refundDepoit: number;
  refundService: number;
}

const EMPTY_SUMMARY: RevenueByDateSummary = {
  dongY: 0,
  doanhSo: 0,
  thucThu: 0,
  deposit: 0,
  refundDepoit: 0,
  refundService: 0,
};

/**
 * Doanh thu theo ngày thu tiền — BE `GET crm/report/revenue/by-date` (port ERP accountant
 * `report/revenue-date`). Mỗi dòng 1 ngày: số khách đồng ý, doanh số, thực thu, đặt cọc, hoàn cọc,
 * hoàn dịch vụ. BE BẮT BUỘC dải ngày + trần 1 năm ⇒ ô ngày không cho xoá trống (xoá bộ lọc = về mặc định).
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-crm-report-revenue-by-date',
  templateUrl: './revenue-by-date.html',
  imports: [FillHeightDirective, ...ListShareImports],
  standalone: true,
})
export class RevenueByDate extends BaseListV2<any> implements OnInit {
  @ViewChild('dt') dt!: Table;
  modelService = inject(ReportRevenueCrmService);
  private readonly printService = inject(PrintService);
  protected override URL_STATE_PREFIX = 'rbd';

  readonly summary = signal<RevenueByDateSummary>(EMPTY_SUMMARY);

  ngOnInit() {
    this.initListV2();
  }

  /** `_ngay_thang` bắt đầu bằng `_` ⇒ buildFilter sinh `_ngay_thang[gte]/[lte]` (không bọc `filter[]`). */
  protected filterFields(): FilterFieldDef[] {
    return [
      {
        name: '_ngay_thang',
        kind: 'dateRange',
        required: true, // BE bắt buộc dải ngày (422 khi thiếu) → Xoá bộ lọc về mặc định
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayEnd()],
      },
      {name: 'branch_id', kind: 'in'},
    ];
  }

  /** Response `{items, _meta, summary}` — bóc `summary` tại đây; reset trước mỗi lần tải để 0 kết quả thì dòng tổng về 0. */
  protected override fetchList(): Observable<ListApiResponse<any>> {
    this.summary.set(EMPTY_SUMMARY);
    return this.modelService.getPaymentByDate({params: this.searchParams()}).pipe(
      tap((response: any) => {
        const s = response?.summary ?? {};
        this.summary.set({
          dongY: Number(s.numAppOk ?? 0),
          doanhSo: Number(s.totalDoanhSo ?? 0),
          thucThu: Number(s.totalThucThu ?? 0),
          deposit: Number(s.totalDeposit ?? 0),
          refundDepoit: Number(s.totalRefundDepoit ?? 0),
          refundService: Number(s.totalRefundService ?? 0),
        });
      }),
      map((response: any) => ({
        ...response,
        items: (response?.items ?? []).map((item: any) => ({
          id: item.dayCreate,
          ngayThang: item.dayCreate,
          dongY: item.numAppOk,
          doanhSo: item.totalDoanhSo,
          thucThu: item.totalThucThu,
          deposit: item.totalDeposit,
          refundDepoit: item.totalRefundDepoit,
          refundService: item.totalRefundService,
        })),
      })),
    );
  }

  exportPaymentByDate(event: Event) {
    this.confirmationService.confirm({
      target: event.target ?? undefined,
      message: this.translate.instant('common.message.confirm_export'),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: {severity: 'danger', size: 'small'},
      accept: () => {
        this.isSubmitting.set(true);
        this.loadingService.showLoading('exportRevenueByDate');
        this.modelService
          .exportPaymentByDate({params: {...this.exportParams()}})
          .pipe(
            finalize(() => {
              this.isSubmitting.set(false);
              this.loadingService.hideLoading('exportRevenueByDate');
            }),
          )
          .subscribe({
            next: (url: string) => {
              this.noti.successMsg(this.translate.instant('common.message.export_success'));
              this.downloadExport(url);
            },
            error: () => this.noti.errorMsg(this.translate.instant('common.message.export_failed')),
          });
      },
    });
  }

  /** In phiếu thu trong ngày — trang in sẵn có `/print/print-payment-in-day/:datePrint`. */
  printPaymentInDay(datePrint: number) {
    const url = this.route.serializeUrl(this.route.createUrlTree([`/print/print-payment-in-day/${datePrint}`]));
    const fetch$ = this.printService.paymentInDay({
      params: {
        '_ngay_thang[gte]': datePrint,
        // dayCreate = 0h ngày theo timezone thương hiệu (BE tính) ⇒ cuối ngày = +86399, khớp trang in.
        '_ngay_thang[lte]': datePrint + 86399,
      },
    });
    this.openPrint(url, `print-payment-in-day-${datePrint}`, fetch$);
  }

  onQuickSelect({start, end}: {start: Date; end: Date}) {
    this.rfFilterModel.patchValue({_ngay_thang: [start, end]});
  }

  /** Copy bảng (client-side) — các dòng ĐANG HIỂN THỊ (trang hiện tại; BE luôn xếp ngày mới nhất trước). */
  copyData() {
    const cfg: ReportSummaryConfig = {
      items: this.models(),
      cols: [
        {value: 'dongY', labelKey: 'common.label.accepted'},
        {value: 'doanhSo', labelKey: 'common.label.sales'},
        {value: 'thucThu', labelKey: 'accountant.report_revenue_common.payment'},
        {value: 'deposit', labelKey: 'common.label.deposit'},
        {value: 'refundDepoit', labelKey: 'common.label.refund_deposit'},
        {value: 'refundService', labelKey: 'common.label.refund_service'},
      ],
      totals: {...this.summary()},
      labelField: 'ngayThang',
      labelFormatter: (v) => this.timezoneService.format(v, {day: '2-digit', month: '2-digit', year: 'numeric'}),
      firstColHeaderKey: 'common.label.date',
      titleKey: 'crm.report.revenue.by_date.title',
      filenameBase: 'doanh-thu-theo-ngay',
      permission: this.permission.permissionManageCrmReport,
      dateRange: this.rfFilterModel?.get('_ngay_thang')?.value,
    };
    copyReportSummary(cfg, {translate: this.translate, noti: this.noti, permissionsService: this.permissionsService});
  }
}
