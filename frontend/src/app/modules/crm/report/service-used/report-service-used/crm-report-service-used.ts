import {ChangeDetectionStrategy, Component, effect, inject, OnInit, signal, ViewChild} from '@angular/core';
import {Table} from 'primeng/table';
import {finalize, Observable, switchMap} from 'rxjs';

import {BaseListV2} from '@/share/base/base-list-v2';
import {FilterFieldDef} from '@/share/base/list-filter.model';
import {ListApiResponse} from '@/share/models/list-api-response';
import {PERMISSION_SELECT} from '@/define';
import {ListShareImports} from '@/share/import/list-share-imports';
import {OrderServiceUsedService} from '@/core/services/crm/order-service-used.service';
import { ExportOptionsDialog, ExportMode } from '@/modules/utilities/export-options-dialog/export-options-dialog';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-service-used-report',
  templateUrl: './crm-report-service-used.html',
  styles: [],
  standalone: true,
  imports: [
    FillHeightDirective,
    ...ListShareImports,

  ],
})
export class CrmReportServiceUsed extends BaseListV2<any> implements OnInit {
  modelService = inject(OrderServiceUsedService);
  @ViewChild('dt') dt!: Table;
  displayCols: any = [
    {label: 'ID', value: 'id'},
    {label: 'Mã dịch vụ', labelKey: 'common.label.service_code', value: 'code'},
    {label: this.translate.instant('common.label.service'), value: 'service'},
    {label: 'Nhóm dịch vụ', labelKey: 'common.label.service_group', value: 'groupService'},
    {label: 'Số lượng khách hàng', labelKey: 'common.label.customer_count', value: 'customerCount'},
    {label: 'Số lượng dịch vụ', labelKey: 'common.label.service_count', value: 'numService'},
    {label: this.translate.instant('common.label.quantity'), value: 'numQuantity'},
    {label: 'Doanh số', labelKey: 'common.label.revenue', value: 'totalTotalEnd'},
    {label: this.translate.instant('crm.order_plan.form.total_to_pay'), value: 'totalPayment'},
    {label: 'Giảm dịch vụ', labelKey: 'common.label.service_discount', value: 'totalDiscountService'},
    {label: 'Giảm khác', labelKey: 'common.label.other_discount', value: 'totalDiscountOther'},
    {label: 'Hoàn dịch vụ', labelKey: 'common.label.service_refund', value: 'totalRefundServiceUsed'},
    {label: 'Trung bình 1 dịch vụ', labelKey: 'crm.report_service_used.average_per_service', value: 'mediumService'},
  ]; // dùng để hiển thị column table
  override selectedColumns = signal([
    'service',
    'customerCount',
    'numService',
    'numQuantity',
    'totalTotalEnd',
    'totalPayment',
    'totalDiscountService',
    'totalDiscountOther',
    'totalRefundServiceUsed',
    'mediumService',
  ]); // dùng để hiển thị column mặc định table
  numService = signal(0);
  numQuantity = signal(0);
  customerCount = signal(0);
  totalTotalEnd = signal(0);
  totalPayment = signal(0);
  totalDiscountService = signal(0);
  totalDiscountOther = signal(0);
  totalRefundServiceUsed = signal(0);
  keysToFilter = ['user'];
  protected override LS_SELECTED_COLUMNS_KEY =
    '/service-used/app-service-used-report';
  protected override URL_STATE_PREFIX = 'rsu';

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

  /** Lọc bỏ cột đã lưu localStorage không còn trong `displayCols` (vd `rank_discount_amount` đã gỡ 2026-11-01). */
  override loadSelectedColumns() {
    super.loadSelectedColumns();
    const valid = new Set<string>(this.displayCols.map((c: { value: string }) => c.value));
    this.selectedColumns.update((cols) => cols.filter((c) => valid.has(c)));
  }

  /**
   * Khai báo filter — base tự sinh FormGroup, đấu valueChanges → dt.filter(),
   * dựng filters khởi tạo cho p-table, lo clearFilter() và đồng bộ URL.
   *
   * `revenue_id` (V1 có control "Tư vấn viên") KHÔNG khai def: BE
   * `buildReportServiceUsedQuery` (crm/report/handler/treatment.go) không có
   * `query.ApplyFilter(c, q, "revenue_id", ...)` — control cũ đổi URL mà không đổi kết quả,
   * đã gỡ luôn khỏi template khi migrate (rule: BE không có ApplyFilter thì không để nút lọc).
   */
  protected filterFields(): FilterFieldDef[] {
    return [
      {name: '_keyword', kind: 'text'},
      {name: 'created_at', kind: 'dateRange'},
      {
        name: 'created_day',
        kind: 'dateRange',
        // factory: clearFilter() tính lại theo hôm nay, không dính ngày lúc mở trang
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayStart()],
      },
      {name: 'branch_id', kind: 'in'},
      {name: 'source_id', kind: 'in'},
      {name: 'sourceService_id', kind: 'in'},
      {name: 'service_type_id', kind: 'in'},
      {name: 'created_by', kind: 'in'},
    ];
  }

  /**
   * Override `fetchList` chứ KHÔNG override `searchHandler`: base cho mọi lần tải đi qua một
   * stream có `switchMap` để huỷ request cũ, và tự lo `loading`/`models`/`paginationInfo`.
   * Response BE bọc trong `object: {items, _meta}` + `totals` riêng — map lại đúng shape
   * `ListApiResponse`, cập nhật các signal tổng làm side-effect.
   */
  protected override fetchList(): Observable<ListApiResponse<any>> {
    return this.modelService.reportServiceUsed({params: this.searchParams()}).pipe(
      switchMap(async (response: any) => {
        const totals = response.totals ?? {};
        this.numService.set(totals.numService ?? 0);
        this.numQuantity.set(totals.numQuantity ?? 0);
        this.customerCount.set(totals.customerCount ?? 0);
        this.totalTotalEnd.set(totals.totalTotalEnd ?? 0);
        this.totalPayment.set(totals.totalPayment ?? 0);
        this.totalDiscountService.set(totals.totalDiscountService ?? 0);
        this.totalDiscountOther.set(totals.totalDiscountOther ?? 0);
        this.totalRefundServiceUsed.set(totals.totalRefundServiceUsed ?? 0);

        const items = response['object']?.items ?? [];
        const labeled = await this.modelService.addLabels(items);
        return {items: labeled, _meta: response['object']?._meta} as ListApiResponse<any>;
      }),
    );
  }

  onQuickSelect({start, end}: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({created_at: [start, end]});
  }

  onQuickSelectCreatedDay({start, end}: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({created_day: [start, end]});
  }

  exportReportServiceUsed() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionManageCrmReport)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
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
      this.doExportReportServiceUsed(mode);
    });
  }

  private doExportReportServiceUsed(mode: ExportMode) {
    this.isSubmitting.set(true);
    this.loadingService.showLoading('exportReportServiceUsed');
    const params = {...this.exportParams()} as any;
    if (mode === 'selected') {
      params['columns'] = this.selectedColumns().join(',');
    }
    this.modelService
      .exportReportServiceUsed({
        params,
      })
      .pipe(
        finalize(() => {
          this.isSubmitting.set(false);
          this.loadingService.hideLoading('exportReportServiceUsed');
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

  /**
   * Nút xoá ngày KHÔNG được để bộ lọc rỗng: BE nay bắt buộc có dải `created_day` và chặn trần
   * 1 năm (không dải ngày = aggregate TRỌN bảng `order_service_used`, chạy 3 lượt quét — đo
   * ~14–18s @898.720 dòng ngay cả sau khi đã tối ưu GROUP BY/COUNT), nên xoá về null chỉ đổi
   * được một màn treo lấy một toast 422. Xoá = quay về mặc định (đầu tháng → hôm nay), đúng
   * thứ người dùng thấy khi mới mở màn.
   */
  clearCreateDay() {
    this.setFilterValue('created_day', [
      this.timezoneService.monthStart(),
      this.timezoneService.todayStart(),
    ]);
  }

  clearCreateAt() {
    this.setFilterValue('created_at', null);
  }
}
