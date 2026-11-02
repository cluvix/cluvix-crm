import {ChangeDetectionStrategy, Component, effect, inject, OnInit, signal, ViewChild} from '@angular/core';
import {Table} from 'primeng/table';
import {finalize, Observable, switchMap} from 'rxjs';

import {PaymentModel} from '@/core/models/crm/payment.model';
import {PaymentService} from '@/core/services/crm/payment.service';
import {ReportRevenueCrmService} from '@/core/services/crm/report-revenue-crm.service';
import {PAYMENT_FOR_OPTION} from '@/define';
import {PaymentMethodMultiselectFormControl} from '@/indexeddb/config/components/payment-method';
import {PaymentTypeMultiselectFormControl} from '@/indexeddb/config/components/payment-type';
import {CustomerTimeline} from '@/modules/crm/customer/contact-list/customer-timeline/customer-timeline';
import {ExportMode, ExportOptionsDialog} from '@/modules/utilities/export-options-dialog/export-options-dialog';
import {BaseListV2} from '@/share/base/base-list-v2';
import {FilterFieldDef} from '@/share/base/list-filter.model';
import {FillHeightDirective} from '@/share/directive/fill-height.directive';
import {ListShareImports} from '@/share/import/list-share-imports';
import {ListApiResponse} from '@/share/models/list-api-response';

/**
 * Phiếu thu theo chi nhánh — BE `GET crm/report/revenue/branch-receipts` (port ERP accountant
 * `report/branch-receipts`). Export Excel qua BE (`_export=1`, nhiều sheet theo chi nhánh, chọn cột
 * bằng ExportOptionsDialog — `displayCols[].value` khớp key colDef của export.ExportPaymentBranch).
 * Bỏ so với ERP: lọc/cột "Thương hiệu" (CRM = 1 thương hiệu/DB). BE BẮT BUỘC dải ngày + trần 1 năm.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-crm-report-revenue-branch-receipts',
  templateUrl: './revenue-branch-receipts.html',
  standalone: true,
  imports: [FillHeightDirective, ...ListShareImports, PaymentMethodMultiselectFormControl, PaymentTypeMultiselectFormControl],
})
export class RevenueBranchReceipts extends BaseListV2<PaymentModel> implements OnInit {
  @ViewChild('dt') dt!: Table;
  modelService = inject(ReportRevenueCrmService);
  private readonly paymentService = inject(PaymentService);
  readonly amountTotal = signal(0);

  displayCols: any = [
    {label: 'Mã khách hàng', labelKey: 'common.label.customer_code', value: 'customer_code'},
    {label: 'Khách hàng', labelKey: 'common.label.customer', value: 'customer_id'},
    {label: 'Mã dịch vụ', labelKey: 'common.label.treatment_service_code', value: 'serviceUsedCode'},
    {label: 'Dịch vụ', labelKey: 'common.label.service', value: 'service'},
    {label: 'Số lượng', labelKey: 'common.label.quantity', value: 'quantity'},
    {label: 'Hình thức thanh toán', labelKey: 'common.label.payment_method', value: 'method'},
    {label: 'Tiền thu/chi', labelKey: 'crm.report.revenue.amount_in_out', value: 'amount'},
    {label: 'Ngày tạo', labelKey: 'common.label.created_at', value: 'created_at'},
    {label: 'Chi nhánh', labelKey: 'common.label.branch', value: 'branch_id'},
    {label: 'Địa chỉ', labelKey: 'common.label.address', value: 'address'},
  ];
  override selectedColumns = signal([
    'customer_code',
    'customer_id',
    'serviceUsedCode',
    'service',
    'quantity',
    'method',
    'amount',
    'created_at',
    'branch_id',
  ]);
  protected override LS_SELECTED_COLUMNS_KEY = '/crm/report/revenue/branch-receipts';
  protected override URL_STATE_PREFIX = 'rbr';
  protected readonly PAYMENT_FOR_OPTION = PAYMENT_FOR_OPTION;

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
      {name: 'branch_id', kind: 'in'},
      {name: 'method', kind: 'in'},
      {name: 'card_type_id', kind: 'in'},
      {name: 'payment_for', kind: 'in'},
      {
        name: 'payment_time',
        kind: 'dateRange',
        required: true, // BE bắt buộc dải ngày (422 khi thiếu) → Xoá bộ lọc về mặc định
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayEnd()],
      },
    ];
  }

  /** Response `{object: {items, _meta}, totalSum}` — bóc tại đây, nhãn (hình thức, chi nhánh…) qua PaymentService.addLabels. */
  protected override fetchList(): Observable<ListApiResponse<PaymentModel>> {
    return this.modelService.listBranchReceipts({params: this.searchParams()}).pipe(
      switchMap(async (response: any) => {
        this.amountTotal.set(Number(response?.totalSum ?? 0));
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
    this.loadingService.showLoading('exportBranchReceipts');
    const params: any = {...this.exportParams()};
    if (mode === 'selected') {
      params['columns'] = this.selectedColumns().join(',');
    }
    this.modelService
      .exportBranchReceipts({params})
      .pipe(
        finalize(() => {
          this.isSubmitting.set(false);
          this.loadingService.hideLoading('exportBranchReceipts');
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

  onQuickSelect({start, end}: {start: Date; end: Date}) {
    this.rfFilterModel.patchValue({payment_time: [start, end]});
  }
}
