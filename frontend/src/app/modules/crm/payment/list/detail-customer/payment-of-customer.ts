import {ChangeDetectionStrategy, Component,
  effect,
  EventEmitter,
  inject,
  Input,
  OnChanges,
  OnInit,
  Output,
  signal,
  ViewChild,
} from '@angular/core';
import { Router } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize, Observable, Subject, switchMap } from 'rxjs';
import { throttleTime } from 'rxjs/operators';
import { Table } from 'primeng/table';

import { FormDeposit } from '@/modules/crm/payment/form/form-deposit/form-deposit';
import { FormMoveDeposit } from '@/modules/crm/payment/form/form-move-deposit/form-move-deposit';
import { FormPayment } from '@/modules/crm/payment/form/form-payment/form-payment';
import { FormRefundDeposit } from '@/modules/crm/payment/form/form-refund-deposit/form-refund-deposit';
import { CustomerModel } from '@/core/models/crm/customer.model';
import { PaymentModel } from '@/core/models/crm/payment.model';
import { CustomerService } from '@/core/services/crm/customer.service';
import { OrderServiceUsedService } from '@/core/services/crm/order-service-used.service';
import { PaymentService } from '@/core/services/crm/payment.service';
import { PaymentDetailPanel } from '@/modules/crm/payment/detail-panel/payment-detail-panel';
import { BaseListV2 } from '@/share/base/base-list-v2';
import { detailPanelHost } from '@/share/base/detail-panel-host';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import { ListApiResponse } from '@/share/models/list-api-response';
import {
  CONFIG_OPTION_CRM_SELECT,
  CUSTOMER_STATUS,
  PAYMENT_FOR,
  PERMISSION_SELECT,
} from '@/define';
import { ListShareImports } from '@/share/import/list-share-imports';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';
import { userId } from '@/core/services/auth';
import { UserOptionHelperService } from '@/indexeddb/config/components/user-option';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-payment-list-detail-customer',
  templateUrl: './payment-of-customer.html',
  standalone: true,
  imports: [...ListShareImports, PaymentDetailPanel, FillHeightDirective],
})
export class PaymentOfCustomer extends BaseListV2<PaymentModel> implements OnInit, OnChanges {
  modelService = inject(PaymentService);

  // SlideOver chi tiết phiếu thu
  protected readonly detail = detailPanelHost<PaymentModel>();
  customerService = inject(CustomerService);
  router = inject(Router);
  orderServiceUsedService = inject(OrderServiceUsedService);
  @ViewChild('dt') dt!: Table<PaymentModel>;
  skipNgOnChangeFirstTime: boolean = true;
  getList$ = new Subject();
  dataSelected: any[] = []; // data đã chọn
  @Input({ required: true }) customer!: CustomerModel;
  @Output() savePayment: EventEmitter<boolean> = new EventEmitter<boolean>(); // load lại thông tin customer detail
  optionsUpdatePayment: boolean = false; // biến lưu option cập nhật thanh toán trong ngày hay không
  displayCols: any = [
    { label: 'ID', value: 'id' },
    { label: 'In', value: 'printTotal' },
    { label: 'Mã đơn hàng', labelKey: 'common.label.order_code', value: 'treatmentPlanCode' },
    { label: 'Mã dịch vụ sử dụng', labelKey: 'common.label.treatment_service_code', value: 'serviceUsedCode' },
    { label: this.translate.instant('common.label.service'), value: 'serviceName' },
    { label: 'Mã phiếu', labelKey: 'common.label.receipt_code', value: 'code' },
    { label: 'Loại thanh toán', labelKey: 'common.label.payment_type', value: 'paymentFor' },
    { label: 'Số tiền', labelKey: 'common.label.amount', value: 'payment_info' },
    { label: 'Hình thức', labelKey: 'common.label.method', value: 'paymentMethod' },
    { label: this.translate.instant('common.label.payment_method'), value: 'paymentCard' },
    { label: 'Nội dung', labelKey: 'common.label.content', value: 'note' },
    { label: 'Nguồn khách hàng', labelKey: 'common.label.source', value: 'source' },
    { label: this.translate.instant('common.label.branch'), value: 'branch' },
    { label: 'Ngày thanh toán', labelKey: 'common.label.payment_date', value: 'payment_of_date' },
    { label: 'Ngày thu/hoàn tiền', labelKey: 'crm.payment_of_customer.payment_time', value: 'payment_time' },
    { label: 'Người tạo', labelKey: 'common.label.user_created', value: 'userCreated' },
    { label: 'Ngày tạo', labelKey: 'common.label.created_at', value: 'created_at' },
    { label: 'Người cập nhật', labelKey: 'common.label.user_updated', value: 'updated_by' },
    { label: 'Ngày cập nhật', labelKey: 'common.label.updated_at', value: 'updated_at' },
  ]; // dùng để hiển thị column table
  override selectedColumns = signal([
    'printTotal',
    'serviceName',
    'paymentMethod',
    'payment_info',
    'paymentFor',
    'payment_of_date',
    'payment_time',
  ]); // dùng để hiển thị column mặc định table
  protected override LS_SELECTED_COLUMNS_KEY = '/payment/app-payment-list-detail-customer';
  protected readonly PAYMENT_FOR = PAYMENT_FOR;
  protected readonly CUSTOMER_STATUS = CUSTOMER_STATUS;
  private userOptionHelper = inject(UserOptionHelperService);

  constructor() {
    super();
    effect(() => {
      const len = this.selectedColumns()?.length ?? 0;
      this.padCols.set(len + 2);
    });
  }

  ngOnInit() {
    this.userOptionHelper.ensureSettingLoaded().then(() => {
      this.optionsUpdatePayment = !!this.userOptionHelper.getUserOptionInfo(
        CONFIG_OPTION_CRM_SELECT.OPTION_UPDATE_PAYMENT,
      )?.options;
    });
    this.getList$.pipe(throttleTime(500)).subscribe({
      next: () => {
        this.getList();
      },
    });
    this.initListV2();
  }

  /** List nhúng trong tab customer-detail — không có filter panel. */
  protected filterFields(): FilterFieldDef[] {
    return [];
  }

  /**
   * Lấy danh sách thanh toán theo khách hàng
   */
  protected override fetchList(): Observable<ListApiResponse<PaymentModel>> {
    this.dataSelected = [];
    if (!this.customer?.id) return this.emptyListResult();

    return this.modelService
      .getListPaymentDetailCustomer({
        params: {
          page: this.page(),
          sort: this.sort(),
          'per-page': this.perPage(),
          customer_id: this.customer.id,
        },
      })
      .pipe(
        switchMap(async (response: any) => {
          const items = response.items ?? [];
          // Pre-compute labels ngay khi nhận data
          response.items = await this.modelService.addLabels(items);
          return response;
        }),
      );
  }

  /**
   * User không có permissionUpdatePayment chỉ được sửa nếu:
   *   - option OPTION_UPDATE_PAYMENT bật
   *   - là người tạo ra thanh toán đó
   *   - vẫn trong cùng ngày tạo
   */
  checkPaymentUpdate(createdAt: number | Date, createdBy: number) {
    if (this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdatePayment)) {
      return true;
    }
    const createdAtDate = createdAt instanceof Date ? createdAt : new Date(Number(createdAt) * 1000);
    return (
      this.permissionsService.getPermission(PERMISSION_SELECT.permissionCreatePayment) &&
      this.optionsUpdatePayment &&
      userId() === createdBy &&
      this.timezoneService.now().toDateString() === createdAtDate.toDateString()
    );
  }

  checkDepositUpdate(createdAt: number, createdBy: number) {
    return this.checkPaymentUpdate(createdAt, createdBy);
  }

  ngOnChanges() {
    if (this.skipNgOnChangeFirstTime) {
      this.skipNgOnChangeFirstTime = false;
      return;
    }

    if (this.customer?.id) {
      this.getList$.next(true);
    }
  }

  /**
   * Hàm xử lý tạo mới đặt cọc
   */
  async createDeposit() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionExtDeposit)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    await this.modelService.ensureCacheLoaded();
    const dialogRef = this.dialogWrapperService.open(FormDeposit, {
      header: this.translate.instant('crm.payment.dialog.create_deposit'),
      data: {
        customer: this.customer,
      },
      width: '700px',
    });
    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
      if (result) {
        this.savePayment.emit(true);
      }
    });
  }

  /**
   * Hàm xử lý cập nhật đặt cọc
   * @param model
   */
  async updateDeposit(model: PaymentModel) {
    await this.modelService.ensureCacheLoaded();
    const dialogRef = this.dialogWrapperService.open(FormDeposit, {
      header: this.translate.instant('common.label.update_deposit'),
      data: {
        model,
        customer: this.customer,
      },
      width: '700px',
    });
    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((data: PaymentModel) => {
      if (data) {
        this.savePayment.emit(true);
      }
    });
  }

  /**
   * Hàm xử lý hoàn cọc
   * @param id
   */
  async refundDeposit(id: number) {
    await this.modelService.ensureCacheLoaded();
    const dialogRef = this.dialogWrapperService.open(FormRefundDeposit, {
      header: this.translate.instant('common.label.refund_deposit'),
      data: {
        customer: this.customer,
        idDeposit: id,
      },
      width: '700px',
    });
    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
      if (result) {
        this.savePayment.emit(true);
      }
    });
  }

  /**
   * Hàm xử lý xóa cọc
   * @param event
   * @param id
   */
  deleteDeposit(event: Event, id: number) {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionExtDeposit)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    this.isSubmitting.set(true);
    this.confirmationService.confirm({
      target: (event.target ?? undefined),
      message: this.translate.instant('common.label.do_you_want_to_delete'),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: {severity: 'danger', size: 'small'},
      accept: () => {
        this.loadingService.showLoading('deleteRecord');
        this.modelService
          .deleteDepositCustomer(id)
          .pipe(
            finalize(() => {
              this.isSubmitting.set(false);
              this.loadingService.hideLoading('deleteRecord');
            }),
          )
          .subscribe({
            next: (res: any) => {
              this.noti.successRes(res);
              this.savePayment.emit(true);
            },
          });
      },
      reject: () => {
        this.isSubmitting.set(false);
      },
    });
  }

  /**
   * Hàm xử lý xóa hoàn cọc
   * @param event
   * @param id
   */
  deleteRefund(event: Event, id: number) {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionExtRefundDeposit)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    this.isSubmitting.set(true);
    this.confirmationService.confirm({
      target: (event.target ?? undefined),
      message: this.translate.instant('common.label.do_you_want_to_delete'),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: {severity: 'danger', size: 'small'},
      accept: () => {
        this.loadingService.showLoading('deleteRecord');
        this.modelService
          .deleteRefundCustomer(id)
          .pipe(
            finalize(() => {
              this.isSubmitting.set(false);
              this.loadingService.hideLoading('deleteRecord');
            }),
          )
          .subscribe({
            next: (res: any) => {
              this.noti.successRes(res);
              this.savePayment.emit(true);
            },
          });
      },
      reject: () => {
        this.isSubmitting.set(false);
      },
    });
  }

  /**
   * Hàm xử lý xóa hoàn dịch vụ
   * @param event
   * @param id
   */
  deleteRefundOrderPlan(event: Event, id: number) {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionExtRefundService)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    this.isSubmitting.set(true);
    this.confirmationService.confirm({
      target: (event.target ?? undefined),
      message: this.translate.instant('common.label.do_you_want_to_delete'),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: {severity: 'danger', size: 'small'},
      accept: () => {
        this.loadingService.showLoading('deleteRecord');
        this.modelService
          .deleteRefundOrderPlan(id)
          .pipe(
            finalize(() => {
              this.isSubmitting.set(false);
              this.loadingService.hideLoading('deleteRecord');
            }),
          )
          .subscribe({
            next: (res: any) => {
              this.noti.successRes(res);
              this.savePayment.emit(true);
            },
          });
      },
      reject: () => {
        this.isSubmitting.set(false);
      },
    });
  }

  /**
   * Hàm xử lý chuyển cọc
   * @param event
   * @param id
   * @param depositAfter
   */
  moveDeposit(event: Event, id: number, depositAfter: number) {
    // API load dropdown dịch vụ
    this.orderServiceUsedService
      .serviceUsedOfCustomer(this.customer.id!)
      .subscribe(async (res: any) => {
        try {
          if (res.length === 0) {
            this.noti.infoMsg(this.translate.instant('crm.payment.message.no_service_to_pay'));
            return;
          }
          if (this.isOpeningForm()) return;

          this.isOpeningForm.set(true);
          await this.modelService.ensureCacheLoaded();
          const dialogRef = this.dialogWrapperService.open(FormMoveDeposit, {
            data: {
              idDeposit: id,
              depositAfter: depositAfter,
              customer: this.customer,
              serviceUsed: res,
            },
            header: this.translate.instant('common.label.transfer_deposit'),
            width: '700px',
          });

          dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
            if (result) {
              this.savePayment.emit(true);
            }
            this.isOpeningForm.set(false);
          });
        } catch (err) {
          console.error('[PaymentOfCustomer] moveDeposit async subscribe error:', err);
          this.isOpeningForm.set(false);
        }
      });
  }

  /**
   * Cập nhật chuyển cọc
   * @param model
   * @param treatmentPlanId
   */
  async updateMovePayment(model: PaymentModel, treatmentPlanId: number) {
    await this.modelService.ensureCacheLoaded();
    const dialogRef = this.dialogWrapperService.open(FormMoveDeposit, {
      header: this.translate.instant('common.label.update_transfer_deposit'),
      data: {
        model,
        parent_id: treatmentPlanId,
        customer: this.customer,
      },
      width: '700px',
      contentStyle: { overflow: 'auto' },
    });

    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
      if (result) {
        this.savePayment.emit(true);
      }
    });
  }

  /**
   * Hàm xử lý xóa thanh toán
   * @param event
   * @param id
   */
  deletePayment(event: Event, id: number): void {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionDeletePayment)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    this.isSubmitting.set(true);
    this.confirmationService.confirm({
      target: (event.target ?? undefined),
      message: this.translate.instant('common.label.do_you_want_to_delete'),
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: {severity: 'danger', size: 'small'},
      accept: () => {
        this.loadingService.showLoading('deleteRecord');
        this.modelService
          .deletePaymentCustomer(id)
          .pipe(
            finalize(() => {
              this.isSubmitting.set(false);
              this.loadingService.hideLoading('deleteRecord');
            }),
          )
          .subscribe({
            next: (res: any) => {
              this.noti.successMsg(res.message);
              this.savePayment.emit(true);
            },
          });
      },
      reject: () => {
        this.isSubmitting.set(false);
      },
    });
  }

  /**
   * Hàm xử lý ta mới thanh toán hoặc cập nhật thanh toán
   * @param model
   */
  async createOrUpdatePayment(model: PaymentModel | null) {
    await this.modelService.ensureCacheLoaded();
    const dialogRef = this.dialogWrapperService.open(FormPayment, {
      data: {
        model,
        customer: this.customer,
      },
      width: '960px',
      header: this.translate.instant(model ? 'common.action.update' : 'common.action.create'),
    });

    dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: PaymentModel) => {
      if (result) {
        this.savePayment.emit(true);
      }
    });
  }

  /**
   * In phiếu thanh toán
   * @param id
   */
  printPayment(id: number) {
    this.openPrint(`/print/print-payment/${id}`, `print-payment-${id}`, this.modelService.printPayment(id));
  }

  printDeposit(id: number) {
    this.openPrint(`/print/print-deposit/${id}`, `print-deposit-${id}`, this.modelService.printDeposit(id));
  }

  override resetPage() {
    this.getList$.next(true);
  }

  /**
   * In phiếu hoàn cọc
   * @param id
   */
  printRefundDeposit(id: number) {
    this.openPrint(`/print/print-refund-deposit/${id}`, `print-refund-deposit-${id}`, this.modelService.printRefundDeposit(id));
  }

  printRefundService(id: number) {
    this.openPrint(`/print/print-refund-service/${id}`, `print-refund-svc-${id}`, this.modelService.printRefundService(id));
  }

  onCheckboxChange(event: any, paymentSelect: any) {
    if (event.checked[0]) {
      this.dataSelected.push(event.checked[0]);
      this.dataSelected = [...new Set(this.dataSelected)];
    } else {
      const paymentIdRemove = '' + paymentSelect;
      const index = this.dataSelected.indexOf(paymentIdRemove);
      if (index > -1) {
        // only splice array when item is found
        this.dataSelected.splice(index, 1); // 2nd parameter means remove one item only
      }
    }
  }

  printCustomPayment() {
    if (this.dataSelected.length === 0) {
      this.noti.warnMsg(this.translate.instant('crm.payment.message.select_payment_to_print'));
      return;
    }
    const ids = this.dataSelected.join(',');
    const customerId = this.customer.id;
    this.openPrint(
      `/print/print-multi-payment?ids=${ids}&customerId=${customerId}`,
      `print-multi-payment-${customerId}`,
      this.modelService.printMultiPayment({ ids, customerId }),
    );
  }

  /** Mở SlideOver chi tiết phiếu thu — dòng đã qua `addLabels` nên dùng thẳng làm seed. */
  openPaymentDetail(model: PaymentModel) {
    this.detail.open(model);
  }

}
