import { ChangeDetectionStrategy, Component, effect, inject, OnInit, signal, ViewChild } from '@angular/core';
import { Table } from 'primeng/table';
import { finalize, Observable, switchMap } from 'rxjs';

import { CustomerTimeline } from '@/modules/crm/customer/contact-list/customer-timeline/customer-timeline';
import { BaseListV2 } from '@/share/base/base-list-v2';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { PERMISSION_SELECT } from '@/define';
import { ListShareImports } from '@/share/import/list-share-imports';
import { ListApiResponse } from '@/share/models/list-api-response';
import { ReportDebtCrmService } from '@/core/services/crm/report-debt-crm.service';
import { ExportOptionsDialog, ExportMode } from '@/modules/utilities/export-options-dialog/export-options-dialog';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';

/**
 * Công nợ theo dịch vụ — BE GET crm/report/debt/service: từng dòng dịch vụ/liệu trình còn nợ
 * (totalEnd + vat_amount ≠ payment; totalEnd là tiền TRƯỚC VAT nên cột VAT đứng ngay sau Phát sinh), lọc theo ngày lên đơn `_orderCreatedTime`.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-crm-service-debts',
  templateUrl: './service-debts.html',
  standalone: true,
  imports: [FillHeightDirective, ...ListShareImports],
})
export class ServiceDebts extends BaseListV2<any> implements OnInit {
  modelService = inject(ReportDebtCrmService);
  @ViewChild('dt') dt!: Table;
  totalEnd = signal(0);
  totalVat = signal(0);
  totalPayment = signal(0);
  totalDebt = signal(0);
  totalDeposit = signal(0);
  // key khớp colDef BE export.ExportServiceDebts (cột tiền là cột cố định, không bật/tắt)
  displayCols: any = [
    { label: 'ID', value: 'id' },
    { label: 'Khách hàng', labelKey: 'common.label.customer', value: 'customer_id' },
    { label: 'Mã khách hàng', labelKey: 'common.label.customer_code', value: 'customer_number' },
    { label: 'Mã khách cũ', labelKey: 'common.label.old_code', value: 'customer_old_code' },
    { label: 'Điện thoại', labelKey: 'common.label.phone', value: 'phone' },
    { label: 'Mã dịch vụ sử dụng', labelKey: 'common.label.treatment_service_code', value: 'serviceUsedCode' },
    { label: 'Dịch vụ', labelKey: 'common.label.service', value: 'serviceName' },
    { label: 'Ngày lên đơn', labelKey: 'common.label.order_date', value: 'serviceUsedCreate' },
    { label: 'Chi nhánh', labelKey: 'common.label.branch', value: 'branch_id' },
  ];
  override selectedColumns = signal([
    'customer_id',
    'serviceUsedCode',
    'serviceName',
    'serviceUsedCreate',
    'branch_id',
  ]);
  protected override LS_SELECTED_COLUMNS_KEY = '/crm/report/debt/service';
  protected override URL_STATE_PREFIX = 'dsv';

  constructor() {
    super();
    effect(() => {
      const len = this.selectedColumns()?.length ?? 0;
      this.padCols.set(len + 7); // # + 5 cột tiền cố định (phát sinh, VAT, thanh toán, nợ, cọc) + cột thao tác
    });
  }

  ngOnInit() {
    this.initListV2();
  }

  /** `_orderCreatedTime` bắt đầu bằng `_` ⇒ base sinh `_orderCreatedTime[gte]/[lte]` không bọc `filter[]` — đúng dạng BE đọc. */
  protected filterFields(): FilterFieldDef[] {
    return [
      { name: '_keyword', kind: 'text' },
      {
        name: '_orderCreatedTime',
        kind: 'dateRange',
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayEnd()],
      },
      { name: 'branch_id', kind: 'in' },
    ];
  }

  onQuickSelect({ start, end }: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({ _orderCreatedTime: [start, end] });
  }

  clearCreatedTime() {
    this.setFilterValue('_orderCreatedTime', null);
  }

  timelineCustomer(event: Event, id: number, fullname: string) {
    this.dialogWrapperService.open(CustomerTimeline, {
      width: '960px',
      data: { customerId: id },
      header: fullname,
    });
  }

  protected override fetchList(): Observable<ListApiResponse<any>> {
    return this.modelService.listServiceDebts({ params: this.searchParams() }).pipe(
      switchMap(async (response: any) => {
        const total = response?.total ?? {};
        this.totalEnd.set(total.totalEnd ?? 0);
        this.totalVat.set(total.vatAmount ?? 0);
        this.totalPayment.set(total.totalPayment ?? 0);
        this.totalDebt.set(total.totalDebt ?? 0);
        this.totalDeposit.set(total.totalDeposit ?? 0);
        const items = await this.modelService.addDebtLabels(response?.items ?? [], ['created_day']);
        return { items, _meta: response?._meta ?? null } as ListApiResponse<any>;
      }),
    );
  }

  exportToExcel() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionManageCrmReport)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    const ref = this.dialogWrapperService.open(ExportOptionsDialog, {
      header: this.translate.instant('utilities.export_options_dialog.header'),
      width: '520px',
      data: { selectedCount: this.selectedColumns().length, totalCount: this.displayCols.length },
    });
    ref.onClose.subscribe((mode: ExportMode | null) => {
      if (!mode) return;
      this.doExport(mode);
    });
  }

  private doExport(mode: ExportMode) {
    this.isSubmitting.set(true);
    this.loadingService.showLoading('exportServiceDebts');
    const params = { ...this.exportParams() } as any;
    if (mode === 'selected') {
      params['columns'] = this.selectedColumns().join(',');
    }
    this.modelService
      .exportServiceDebts({ params })
      .pipe(
        finalize(() => {
          this.isSubmitting.set(false);
          this.loadingService.hideLoading('exportServiceDebts');
        }),
      )
      .subscribe({
        next: (res: any) => {
          this.noti.successMsg(this.translate.instant('common.message.export_success'));
          this.downloadExport(res);
        },
        error: () => this.noti.errorMsg(this.translate.instant('common.message.export_failed')),
      });
  }
}
