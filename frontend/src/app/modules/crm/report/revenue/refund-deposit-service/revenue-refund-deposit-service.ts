import {ChangeDetectionStrategy, Component, effect, inject, OnInit, signal, ViewChild} from '@angular/core';
import {Table} from 'primeng/table';
import {finalize, Observable, switchMap} from 'rxjs';

import {PaymentModel} from '@/core/models/crm/payment.model';
import {PaymentService} from '@/core/services/crm/payment.service';
import {ReportRevenueCrmService} from '@/core/services/crm/report-revenue-crm.service';
import {CustomerTimeline} from '@/modules/crm/customer/contact-list/customer-timeline/customer-timeline';
import {ExportMode, ExportOptionsDialog} from '@/modules/utilities/export-options-dialog/export-options-dialog';
import {BaseListV2} from '@/share/base/base-list-v2';
import {FilterFieldDef} from '@/share/base/list-filter.model';
import {FillHeightDirective} from '@/share/directive/fill-height.directive';
import {ListShareImports} from '@/share/import/list-share-imports';
import {ListApiResponse} from '@/share/models/list-api-response';

/**
 * Giảm trừ doanh số — phiếu hoàn dịch vụ + hoàn cọc. BE `GET crm/report/revenue/refund-deposit-service`
 * (port ERP accountant `report/refund-deposit-service`).
 *
 * Khác ERP: export qua BE (`_export=1`, ExportOptionsDialog chọn cột) thay cho "tải hết rồi dựng
 * Excel ở FE" — bản ERP xin `per-page=100000` nhưng BE kẹp per-page ≤ 200 nên file chỉ có 20 dòng đầu.
 * Bỏ cột "Thương hiệu" (1 thương hiệu/DB). BE BẮT BUỘC dải ngày + trần 1 năm.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-crm-report-revenue-refund-deposit-service',
  templateUrl: './revenue-refund-deposit-service.html',
  imports: [FillHeightDirective, ...ListShareImports],
  standalone: true,
})
export class RevenueRefundDepositService extends BaseListV2<PaymentModel> implements OnInit {
  @ViewChild('dt') dt!: Table;
  modelService = inject(ReportRevenueCrmService);
  private readonly paymentService = inject(PaymentService);
  readonly totalRefund = signal(0);

  // `value` khớp key colDef của export.ExportRefundDepositService (BE).
  displayCols: any = [
    {label: 'ID', value: 'id'},
    {label: 'Mã khách hàng', labelKey: 'common.label.customer_code', value: 'customer_number'},
    {label: 'Mã khách cũ', labelKey: 'common.label.old_code', value: 'customer_old_code'},
    {label: 'Khách hàng', labelKey: 'common.label.customer', value: 'customer_id'},
    {label: 'Số điện thoại', labelKey: 'common.label.phone', value: 'phone'},
    {label: 'Mã phiếu giảm trừ', labelKey: 'crm.report.revenue.refund.code', value: 'code'},
    {label: 'Số tiền giảm trừ', labelKey: 'crm.report.revenue.refund.amount', value: 'deposit'},
    {label: 'Loại giảm trừ', labelKey: 'crm.report.revenue.refund.type', value: 'refundType'},
    {label: 'Nội dung giảm trừ', labelKey: 'crm.report.revenue.refund.content', value: 'content'},
    {label: 'Mã phiếu cọc', labelKey: 'crm.report.revenue.refund.code_deposit', value: 'codeDeposit'},
    {label: 'Nội dung phiếu cọc', labelKey: 'crm.report.revenue.refund.content_deposit', value: 'contentDeposit'},
    {label: 'Ngày giảm trừ', labelKey: 'crm.report.revenue.refund.date', value: 'payment_time'},
    {label: 'Chi nhánh', labelKey: 'common.label.branch', value: 'branch_id'},
    {label: 'Người tạo', labelKey: 'common.label.user_created', value: 'created_by'},
    {label: 'Ngày tạo', labelKey: 'common.label.created_at', value: 'created_at'},
    {label: 'Người cập nhật', labelKey: 'common.label.user_updated', value: 'updated_by'},
    {label: 'Ngày cập nhật', labelKey: 'common.label.updated_at', value: 'updated_at'},
  ];
  override selectedColumns = signal(['customer_id', 'phone', 'code', 'deposit', 'refundType', 'branch_id', 'payment_time']);
  protected override LS_SELECTED_COLUMNS_KEY = '/crm/report/revenue/refund-deposit-service';
  protected override URL_STATE_PREFIX = 'rrd';

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

  protected filterFields(): FilterFieldDef[] {
    return [
      {name: '_keyword', kind: 'text'},
      {
        name: 'payment_time',
        kind: 'dateRange',
        required: true, // BE bắt buộc dải ngày (422 khi thiếu) → Xoá bộ lọc về mặc định
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayEnd()],
      },
      {name: 'branch_id', kind: 'in'},
    ];
  }

  /** Response `{object: {items, _meta}, totalRefundDeposit}` — bóc tại đây. */
  protected override fetchList(): Observable<ListApiResponse<PaymentModel>> {
    return this.modelService.listRefundDepositService({params: this.searchParams()}).pipe(
      switchMap(async (response: any) => {
        this.totalRefund.set(Number(response?.totalRefundDeposit ?? 0));
        return {
          items: await this.paymentService.addLabels(response?.object?.items ?? []),
          _meta: response?.object?._meta,
        } as ListApiResponse<PaymentModel>;
      }),
    );
  }

  timelineCustomer(_event: Event, id: number, fullname: string) {
    this.dialogRef = this.dialogWrapperService.open(CustomerTimeline, {
      width: '960px',
      data: {customerId: id},
      header: fullname,
    });
  }

  onQuickSelect({start, end}: {start: Date; end: Date}) {
    this.rfFilterModel.patchValue({payment_time: [start, end]});
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
      if (mode) this.doExport(mode);
    });
  }

  private doExport(mode: ExportMode) {
    this.isSubmitting.set(true);
    this.loadingService.showLoading('exportRefundDepositService');
    const params: any = {...this.exportParams()};
    if (mode === 'selected') {
      params['columns'] = this.selectedColumns().join(',');
    }
    this.modelService
      .exportRefundDepositService({params})
      .pipe(
        finalize(() => {
          this.isSubmitting.set(false);
          this.loadingService.hideLoading('exportRefundDepositService');
        }),
      )
      .subscribe({
        next: (url: string) => {
          this.noti.successMsg(this.translate.instant('common.message.export_success'));
          this.downloadExport(url);
        },
        error: () => this.noti.errorMsg(this.translate.instant('common.message.export_failed')),
      });
  }
}
