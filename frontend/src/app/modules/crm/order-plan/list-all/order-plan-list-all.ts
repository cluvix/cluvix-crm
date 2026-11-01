import { ChangeDetectionStrategy, Component, effect, inject, OnInit, signal, ViewChild } from '@angular/core';
import { Table } from 'primeng/table';
import { finalize, Observable, switchMap } from 'rxjs';

import { CustomerTimeline } from '@/modules/crm/customer/contact-list/customer-timeline/customer-timeline';
import { OrderPlanModel } from '@/core/models/order-plan.model';
import { OrderPlanService } from '@/core/services/order-plan.service';
import { OrderPlanDetailPanel } from '@/modules/crm/order-plan/detail-panel/order-plan-detail-panel';
import { BaseListV2 } from '@/share/base/base-list-v2';
import { detailPanelHost } from '@/share/base/detail-panel-host';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { PERMISSION_SELECT } from '@/define';
import { ListShareImports } from '@/share/import/list-share-imports';
import { ListApiResponse } from '@/share/models/list-api-response';
import { UserHelperService } from '@/indexeddb/config/components/user';
import { ExportOptionsDialog, ExportMode } from '@/modules/utilities/export-options-dialog/export-options-dialog';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';

/**
 * Quản lý đơn hàng — danh sách toàn bộ đơn (không gắn 1 khách), port từ ERP
 * `accountant/order-plan/list-order-plan/list-for-plan`.
 *
 * BE: `GET /crm/order-plan` (list) + `?_export=1` (Excel). Khác ERP: bỏ cột Thương hiệu (1 thương
 * hiệu/DB), Bác sĩ tư vấn và Direct sale (schema CRM không có `listDoctors`/`listDirectSales`);
 * cột Tư vấn viên đọc `listRevenues` (CRM không có `revenue_id`). Quyền export dùng
 * `permissionExtPayment` — khớp BE `RequireExportPerm(extPayment)` ở `GET /crm/order-plan?_export=1`.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-order-plan-list-all',
  templateUrl: './order-plan-list-all.html',
  standalone: true,
  imports: [FillHeightDirective, ...ListShareImports, OrderPlanDetailPanel],
})
export class OrderPlanListAll extends BaseListV2<OrderPlanModel> implements OnInit {
  modelService = inject(OrderPlanService);
  @ViewChild('dt') dt!: Table;
  // SlideOver chi tiết đơn hàng — dùng chung panel với tab Đơn hàng trong hồ sơ khách.
  protected readonly detail = detailPanelHost<OrderPlanModel>();
  private readonly userHelper = inject(UserHelperService);

  // `value` khớp key cột của BE `exportHandlers.ExportOrderPlan` (export `?columns=`).
  displayCols: any = [
    { label: 'ID', value: 'id' },
    { label: this.translate.instant('common.label.customer_code'), value: 'customer_code' },
    { label: this.translate.instant('common.label.customer'), value: 'fullname' },
    { label: this.translate.instant('common.label.phone'), value: 'phone' },
    { label: 'Mã đơn hàng', labelKey: 'common.label.order_code', value: 'code' },
    { label: 'Giá trị đơn hàng', labelKey: 'crm.order_plan.list_all.total', value: 'total' },
    { label: 'Tiền VAT', labelKey: 'common.label.vat_amount', value: 'vat_total' },
    { label: 'Tổng sau VAT', labelKey: 'crm.order_plan.form.total_after_vat', value: 'total_after_vat' },
    { label: 'Đã thanh toán', labelKey: 'common.status.paid', value: 'payment' },
    { label: 'Còn nợ', labelKey: 'common.label.outstanding', value: 'report-debt' },
    { label: 'Tư vấn viên', labelKey: 'common.label.consultant', value: 'tuVan' },
    { label: 'Ngày lên đơn', labelKey: 'common.label.order_date', value: 'created_time' },
    { label: this.translate.instant('common.label.branch'), value: 'branch' },
    { label: 'Người tạo', labelKey: 'common.label.user_created', value: 'created_by' },
    { label: 'Ngày tạo', labelKey: 'common.label.created_at', value: 'created_at' },
    { label: 'Người cập nhật', labelKey: 'common.label.user_updated', value: 'updated_by' },
    { label: 'Ngày cập nhật', labelKey: 'common.label.updated_at', value: 'updated_at' },
  ];
  override selectedColumns = signal([
    'fullname',
    'code',
    'branch',
    'total',
    'vat_total',
    'payment',
    'report-debt',
    'created_time',
    'phone',
  ]);
  protected override LS_SELECTED_COLUMNS_KEY: string = 'app-order-plan-list-all/OrderPlanListAll';
  protected override URL_STATE_PREFIX = 'op';

  constructor() {
    super();
    effect(() => {
      const len = this.selectedColumns()?.length ?? 0;
      this.padCols.set(len + 3);
    });
  }

  ngOnInit() {
    this.initListV2();
  }

  /** BE `orderPlanFilterBase` có `_keyword` + ApplyFilter branch_id/created_by/created_time. */
  protected filterFields(): FilterFieldDef[] {
    return [
      { name: '_keyword', kind: 'text' },
      { name: 'branch_id', kind: 'in' },
      { name: 'created_by', kind: 'in' },
      {
        name: 'created_time',
        kind: 'dateRange',
        default: () => [this.timezoneService.monthStart(), this.timezoneService.todayEnd()],
      },
    ];
  }

  protected override fetchList(): Observable<ListApiResponse<OrderPlanModel>> {
    return this.modelService.list({ params: this.searchParams() }).pipe(
      switchMap(async (response: any) => {
        const items = response.items ?? [];
        return {
          items: await this.modelService.addLabels(items),
          _meta: response._meta,
        } as ListApiResponse<OrderPlanModel>;
      }),
    );
  }

  timelineCustomer(event: Event, id: number, fullname: string) {
    this.dialogRef = this.dialogWrapperService.open(CustomerTimeline, {
      width: '960px',
      data: {
        customerId: id,
      },
      header: fullname,
    });
  }

  exportPlanExcel() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionExtPayment)) {
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
      this.isSubmitting.set(true);
      this.loadingService.showLoading('exportPlan');
      const params = { ...this.exportParams() } as any;
      if (mode === 'selected') {
        params['columns'] = this.selectedColumns().join(',');
      }
      this.modelService
        .exportPlan({ params })
        .pipe(
          finalize(() => {
            this.isSubmitting.set(false);
            this.loadingService.hideLoading('exportPlan');
          }),
        )
        .subscribe({
          next: (res: any) => {
            this.noti.successMsg(this.translate.instant('common.message.export_success'));
            this.downloadExport(res);
          },
        });
    });
  }

  onQuickSelect({ start, end }: { start: Date; end: Date }) {
    this.rfFilterModel.patchValue({ created_time: [start, end] });
  }

  /** Xoá hẳn filter ngày — không còn `gte` lẫn `lte`. */
  clearCreateTime() {
    this.setFilterValue('created_time', null);
  }

  getUserInfo(userId: number) {
    return this.userHelper.getUserInfo(userId);
  }

  /** Mở SlideOver chi tiết đơn hàng. Dòng ở đây LÀ đơn nên dùng thẳng làm seed cho phần header. */
  openOrderDetail(model: OrderPlanModel) {
    this.detail.open(model);
  }
}
