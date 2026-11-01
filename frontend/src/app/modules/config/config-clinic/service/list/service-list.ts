import {ChangeDetectionStrategy, Component, effect, inject, OnInit, signal, ViewChild } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { Table } from 'primeng/table';
import { Observable, finalize, switchMap } from 'rxjs';

import { ServiceForm } from '@/modules/config/config-clinic/service/form/service-form';
import { ServiceImportDialog } from '@/modules/config/config-clinic/service/import/service-import-dialog';
import { ServiceModel } from '@/core/models/config/clinic/service.model';
import { ServiceService } from '@/core/services/config/clinic/service.service';
import { BaseListV2 } from '@/share/base/base-list-v2';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { ListApiResponse } from '@/share/models/list-api-response';
import { PERMISSION_SELECT, SERVICE_CATEGORY_OPTIONS } from '@/define';
import { ListShareImports } from '@/share/import/list-share-imports';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-service-list',
  templateUrl: './service-list.html',
  styles: [
    `
      @media screen and (max-width: 40em) {
        :host ::ng-deep.p-menu.p-menu-overlay {
          top: 22px !important;
          left: 18px !important;
        }
      }
    `,
  ],
  imports: [FillHeightDirective, ...ListShareImports],
  standalone: true,
})
export class ServiceList extends BaseListV2<ServiceModel> implements OnInit {
  modelService = inject(ServiceService);
  @ViewChild('dt') dt!: Table;
  // labelKey duy nhất được AppColumnToggle dùng (col.labelKey || col.label) — không giữ `label`
  // tiếng Việt hardcode song song (dead field, không bao giờ render khi labelKey đã có).
  displayCols: any = [
    { labelKey: 'common.label.id', value: 'id' },
    { labelKey: 'common.label.service_code', value: 'code' },
    { labelKey: 'common.label.old_service_code', value: 'code_old' },
    { labelKey: 'common.label.service_name', value: 'name' },
    { labelKey: 'common.label.service_group', value: 'service_type_id' },
    { labelKey: 'common.label.category', value: 'serviceCategory_id' },
    { labelKey: 'common.label.sort_order', value: 'SortOrder' },
    { labelKey: 'common.label.price_info_min_max', value: 'min_price' },
    { labelKey: 'common.label.re_engagement', value: 'time_reminder' },
    { labelKey: 'common.label.unit', value: 'unit' },
    { labelKey: 'common.label.telesale_commission', value: 'commission_telesale' },
    { labelKey: 'common.label.status', value: 'status' },
  ];
  override selectedColumns = signal([
    'id',
    'name',
    'min_price',
    'time_reminder',
    'status',
    'SortOrder',
  ]);
  protected override LS_SELECTED_COLUMNS_KEY = '/config/service';
  protected override URL_STATE_PREFIX = 'se';
  protected readonly SERVICE_CATEGORY_OPTIONS = SERVICE_CATEGORY_OPTIONS;

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

  /**
   * Khai báo filter — base tự sinh FormGroup, đấu valueChanges → dt.filter(),
   * dựng filters khởi tạo cho p-table, lo clearFilter() và đồng bộ URL.
   * `service_type_id`/`serviceCategory_id` là multiselect thật trong V1 (fb.control([]) mặc định mảng rỗng,
   * control component/`p-select [multiple]="true"`) → giữ nguyên `kind: 'in'`.
   */
  protected filterFields(): FilterFieldDef[] {
    return [
      {name: '_keyword', kind: 'text'},
      {name: 'service_type_id', kind: 'in'},
      {name: 'serviceCategory_id', kind: 'in'},
    ];
  }

  /**
   * Override `fetchList` chứ KHÔNG override `searchHandler`: base cho mọi lần tải đi qua một
   * stream có `switchMap` để huỷ request cũ, và tự lo `loading`/`models`/`paginationInfo`.
   */
  protected override fetchList(): Observable<ListApiResponse<ServiceModel>> {
    return this.modelService
      .list({
        params: this.searchParams(),
      })
      .pipe(
        switchMap(async (response: any) => {
          if (response.items) {
            const items = response.items ?? [];
            response.items = await this.modelService.addLabels(items);
          }
          return response;
        }),
      );
  }

  createService() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionManageConfig)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    this.dialogRef = this.dialogWrapperService.open(ServiceForm, {
      width: '960px',
      header: this.translate.instant('common.action.create'),
    });

    this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
      if (result) {
        this.getList();
      }
    });
  }

  updateService(model: ServiceModel) {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionManageConfig)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    if (this.isOpeningForm()) return;
    this.isOpeningForm.set(true);
    this.loadingService.showLoading('updateService');
    // Load full detail (kèm serviceMaterial) trước khi mở form
    this.modelService
      .get(model.id!)
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.loadingService.hideLoading('updateService')),
      )
      .subscribe({
        next: (detail: ServiceModel) => {
          this.dialogRef = this.dialogWrapperService.open(ServiceForm, {
            width: '960px',
            header: this.translate.instant('common.action.update'),
            data: {
              model: detail,
              clone: false,
            },
          });

          this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: ServiceModel) => {
            if (result) {
              this.getList();
            }
            this.isOpeningForm.set(false);
          });
        },
        error: (err: any) => {
          this.noti.errorMsg(err?.error?.message || this.translate.instant('common.message.error'));
          this.isOpeningForm.set(false);
        },
      });
  }

  cloneService(model: ServiceModel) {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionManageConfig)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    if (this.isOpeningForm()) return;
    this.isOpeningForm.set(true);
    this.loadingService.showLoading('cloneService');
    // Load full detail (kèm serviceMaterial) trước khi mở form clone
    this.modelService
      .get(model.id!)
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.loadingService.hideLoading('cloneService')),
      )
      .subscribe({
        next: (detail: ServiceModel) => {
          this.dialogRef = this.dialogWrapperService.open(ServiceForm, {
            width: '960px',
            header: this.translate.instant('config.config_clinic.service.clone_dialog'),
            data: {
              model: detail,
              clone: true,
            },
          });

          this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
            if (result) {
              this.getList();
            }
            this.isOpeningForm.set(false);
          });
        },
        error: (err: any) => {
          this.noti.errorMsg(err?.error?.message || this.translate.instant('common.message.error'));
          this.isOpeningForm.set(false);
        },
      });
  }

  openImportDialog() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionManageConfig)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    this.dialogWrapperService.open(ServiceImportDialog, {
      width: '700px',
      header: this.translate.instant('config.service.import_dialog_title'),
      data: { onImported: () => this.getList() },
    });
  }

  /**
   * Xuất danh sách dịch vụ ra file
   * - Kiểm tra quyền export trước khi thực hiện
   * - Hiển thị hộp thoại xác nhận trước khi tải
   * - Gộp các tham số tìm kiếm hiện tại vào request
   * - Chuyển hướng đến URL file sau khi export thành công
   * @param event - DOM event dùng để định vị hộp thoại xác nhận
   */
  exportService(event: Event) {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionManageConfig)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    this.isSubmitting.set(true);
    this.confirmationService.confirm({
      target: (event.target ?? undefined),
      message: this.translate.instant('common.message.confirm_export'),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: {severity: 'danger', size: 'small'},
      accept: () => {
        this.loadingService.showLoading('exportService');
        const params = {...this.exportParams()} as any;
        this.modelService
          .exportService({
            params,
          })
          .pipe(
            finalize(() => {
              this.isSubmitting.set(false);
              this.loadingService.hideLoading('exportService');
            }),
          )
          .subscribe({
            next: (res: any) => {
              this.noti.successMsg(this.translate.instant('common.message.export_success'));
              this.downloadExport(res);
            },
          });
      },
      reject: () => {
        this.isSubmitting.set(false);
      },
    });
  }

  checkUpdatePermission() {
    return !this.permissionsService.getPermission(PERMISSION_SELECT.permissionManageConfig);
  }
}
