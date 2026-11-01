import {ChangeDetectionStrategy, Component,
  effect,
  EventEmitter,
  inject,
  Input,
  OnChanges,
  OnInit,
  Output,
  signal,
  SimpleChanges,
  ViewChild,
} from '@angular/core';
import { Router } from '@angular/router';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize, Observable, Subject, switchMap } from 'rxjs';
import { throttleTime } from 'rxjs/operators';
import { Table } from 'primeng/table';

import { OrderPlanForm } from '@/modules/crm/order-plan/form/order-plan-form';
import { OrderPlanDetailPanel } from '@/modules/crm/order-plan/detail-panel/order-plan-detail-panel';
import { TreatmentServiceDetailPanel } from '@/modules/crm/order-plan/detail-panel/treatment-service-detail-panel/treatment-service-detail-panel';
import { FormPaymentOrder } from '@/modules/crm/payment/form/form-payment-order/form-payment-order';
import { FormRefundOrderPlan } from '@/modules/crm/payment/form/form-refund-order-plan/form-refund-order-plan';
import { CustomerModel } from '@/core/models/crm/customer.model';
import { OrderPlanModel } from '@/core/models/order-plan.model';
import { CustomerService } from '@/core/services/crm/customer.service';
import { OrderServiceUsedService } from '@/core/services/crm/order-service-used.service';
import { OrderPlanService } from '@/core/services/order-plan.service';
import { BaseListV2 } from '@/share/base/base-list-v2';
import { FilterFieldDef } from '@/share/base/list-filter.model';
import {CONFIG_OPTION_CRM_SELECT, PERMISSION_SELECT} from '@/define';
import { ListShareImports } from '@/share/import/list-share-imports';
import { FillHeightDirective } from '@/share/directive/fill-height.directive';
import { OldNewTypePipe } from '@/share/pipes/old-new-type.pipe';
import { userId } from '@/core/services/auth';
import { OrderServiceUsedModel } from '@/core/models/crm/order-service-used.model';
import { ListApiResponse } from '@/share/models/list-api-response';
import { UserHelperService } from '@/indexeddb/config/components/user';
import { UnitHelperService } from '@/indexeddb/config/components/unit';
import { UserOptionHelperService } from '@/indexeddb/config/components/user-option';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-order-treatment-plan-mini-list',
  templateUrl: './order-plan-in-customer.html',
  standalone: true,
  imports: [
    ...ListShareImports,
    OrderPlanDetailPanel,
    TreatmentServiceDetailPanel,
    OldNewTypePipe,
    FillHeightDirective,
  ],
})
export class OrderPlanInCustomer extends BaseListV2<OrderServiceUsedModel> implements OnInit, OnChanges {
  modelService = inject(OrderServiceUsedService);
  orderPlanService = inject(OrderPlanService);
  customerService = inject(CustomerService);
  router = inject(Router);
  @ViewChild('dt') dt!: Table<OrderServiceUsedModel>;
  skipNgOnChangeFirstTime: boolean = true;
  getList$ = new Subject();
  @Input({ required: true }) customer!: CustomerModel;
  @Output() saveOderTreatmentPlan: EventEmitter<boolean> = new EventEmitter<boolean>(); // load lại thông tin customer detail
  optionsUpdateOrderPlan: boolean = false; // biến lưu option cập nhật đơn hàng trong ngày
  displayCols: any = [
    { label: 'ID', value: 'id' },
    { label: 'Mã đơn hàng', labelKey: 'common.label.order_code', value: 'orderCode' },
    { label: 'Mã dịch vụ sử dụng', labelKey: 'common.label.treatment_service_code', value: 'code' },
    { label: 'Mã dịch vụ', labelKey: 'common.label.service_code', value: 'codeSerice' },
    { label: this.translate.instant('common.label.service'), value: 'orderServiceUseds' },
    { label: 'Đơn vị tính', labelKey: 'common.label.unit', value: 'unit' },
    { label: this.translate.instant('common.label.quantity'), value: 'quantity' },
    { label: this.translate.instant('common.label.subtotal'), value: 'totalEnd' },
    { label: 'Đã thanh toán', labelKey: 'common.status.paid', value: 'payment' },
    { label: 'Hoàn dịch vụ', labelKey: 'common.label.service_refund', value: 'refundServiceTotal' },
    { label: this.translate.instant('crm.order_plan.form.outstanding_balance'), value: 'conLai' },
    { label: 'Tư vấn viên', labelKey: 'common.label.consultant', value: 'tuVan' },
    { label: 'Nhân viên tư vấn', labelKey: 'common.label.consulting_doctor', value: 'doctorRevenue_id' },
    { label: this.translate.instant('common.label.note'), value: 'note' },
    { label: this.translate.instant('common.label.branch'), value: 'branch' },
    { label: 'Nguồn khách hàng', labelKey: 'common.label.source', value: 'source' },
    { label: 'Đơn hàng mới/cũ', labelKey: 'crm.order_plan.order_type', value: 'orderPlanType' },
    { label: 'Ngày lên đơn', labelKey: 'common.label.order_date', value: 'created_time' },
    { label: 'Người tạo', labelKey: 'common.label.user_created', value: 'userCreated' },
    { label: 'Ngày tạo', labelKey: 'common.label.created_at', value: 'created_at' },
    { label: 'Người cập nhật', labelKey: 'common.label.user_updated', value: 'updated_by' },
    { label: 'Ngày cập nhật', labelKey: 'common.label.updated_at', value: 'updated_at' },
  ]; // dùng để hiển thị column table
  override selectedColumns = signal([
    'orderServiceUseds',
    'quantity',
    'totalEnd',
    'payment',
    'conLai',
    'created_time',
  ]); // dùng để hiển thị column mặc định table
  protected override LS_SELECTED_COLUMNS_KEY = '/order-plan/app-list-in-customer';
  private userOptionHelper = inject(UserOptionHelperService);
  private userHelper: UserHelperService = inject(UserHelperService);
  private unitHelper: UnitHelperService = inject(UnitHelperService);

  constructor() {
    super();

    effect(() => {
      const len = this.selectedColumns()?.length ?? 0;
      this.padCols.set(len + 3);
    });
  }

  ngOnInit() {
    this.userOptionHelper.ensureSettingLoaded().then(() => {
      this.optionsUpdateOrderPlan = !!this.userOptionHelper.getUserOptionInfo(
        CONFIG_OPTION_CRM_SELECT.OPTION_UPDATE_ORDER_PLAN,
      )?.options;
    });
    this.initRequiredFilter();
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
   * Check xem có option được cập nhật trong ngày hay không, nếu có thì ghi đè quyền được cập nhật
   * Chỉ khi có quyền tạo mới thì mới được tính
   * @param createdAt
   * @param createdBy
   */
  checkOrderPlanUpdate(createdAt: number | Date, createdBy: number) {
    const createdAtDate = createdAt instanceof Date ? createdAt : new Date(Number(createdAt) * 1000);
    return (
      this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdatePlanOrder) ||
      (this.permissionsService.getPermission(PERMISSION_SELECT.permissionCreatePlanOrder) &&
        this.optionsUpdateOrderPlan &&
        userId() === createdBy &&
        this.timezoneService.now().toDateString() === createdAtDate.toDateString())
    );
  }

  ngOnChanges(_changes: SimpleChanges) {
    if (this.skipNgOnChangeFirstTime === true) {
      this.skipNgOnChangeFirstTime = false;
      return;
    }

    if (this.customer?.id) {
      this.initRequiredFilter();
      this.getList$.next(true);
    }
  }

  /**
   * Khởi tạo filter
   */
  initRequiredFilter() {
    this.requiredSearch.set({
      id: this.customer?.id,
    });
  }

  /**
   * Lấy danh sách đơn hàng or dịch vụ theo khách hàng
   */
  protected override fetchList(): Observable<ListApiResponse<OrderServiceUsedModel>> {
    if (!this.customer?.id) return this.emptyListResult();

    return this.modelService
      .listServiceUsedCustomer({
        params: this.searchParams(),
      })
      .pipe(
        switchMap(async (response: any) => {
          await this.unitHelper.ensureUnitsLoaded();
          if (response.items) {
            const items = response.items ?? [];
            response.items = await this.modelService.addLabels(items);
          }
          return response;
        }),
      );
  }

  /**
   * Hàm xử lý ta mới thanh toán hoặc cập nhật thanh toán
   * Fetch đơn đầy đủ TRƯỚC khi mở dialog (loading tím toàn cục qua GlobalLoadingService) —
   * dialog mở ra là có sẵn dữ liệu, không cần loading riêng bên trong (đúng UX pattern đã dùng
   * ở updateOrderPlan() cùng file).
   * @param orderPlanModel
   */
  createPaymentOrder(orderPlanModel: OrderPlanModel | null): void {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionCreatePayment)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    if (!orderPlanModel?.id) return;

    this.loadingService
      .track('createPaymentOrder', this.orderPlanService.get(orderPlanModel.id, {}))
      .subscribe((order: any) => {
        const dialogRef = this.dialogWrapperService.open(FormPaymentOrder, {
          data: { orderPlanModel: order },
          width: '700px',
          header: this.translate.instant('common.label.order_payment'),
        });

        dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: OrderPlanModel) => {
          if (result) {
            this.saveOderTreatmentPlan.emit(true);
          }
        });
      });
  }

  /**
   * Hoàn trả đơn hàng hoặc dịch vụ
   * @param event
   * @param idOrder
   * @param idService
   * @param payment
   */
  refundServiceUsed(event: Event, idOrder: number, idService: number, payment: number) {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionExtRefundService)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }

    this.confirmationService.confirm({
      target: (event.target ?? undefined),
      message: 'Bạn có muốn hoàn trả dịch vụ?',
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: {severity: 'danger', size: 'small'},
      accept: () => {
        this.dialogRef = this.dialogWrapperService.open(FormRefundOrderPlan, {
          width: '700px',
          header: this.translate.instant('crm.order_plan.dialog_refund'),
          data: {
            customer: this.customer,
            idOrderPlan: idOrder,
            idService: idService,
            payment: payment,
          },
        });

        this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
          if (result) {
            this.saveOderTreatmentPlan.emit(true);
          }
        });
      },
      reject: () => {},
    });
  }

  /**
   * Tạo mới đơn hàng hoặc dịch vụ
   */
  createOrderPlan(): void {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionCreatePlanOrder)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }

    const data: any = {
      customer: this.customer,
    };

    const width = '1200px';

    this.dialogRef = this.dialogWrapperService.open(OrderPlanForm, {
      width,
      header: this.translate.instant('crm.order_plan.dialog_create'),
      data,
    });

    this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
      if (result) {
        this.saveOderTreatmentPlan.emit(true);
      }
    });
  }

  /**
   * Cập nhật đơn hàng hoặc dịch vụ
   * @param id
   */
  updateOrderPlan(id: number): void {
    this.orderPlanService.get(id, {}).subscribe((model: any) => {
      const data: any = {
        model,
        customer: this.customer,
      };

      const width = '1200px';

      this.dialogRef = this.dialogWrapperService.open(OrderPlanForm, {
        width,
        header: this.translate.instant('common.label.update_order'),
        data,
      });

      this.dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
        if (result) {
          this.saveOderTreatmentPlan.emit(true);
        }
      });
    });
  }

  /**
   * Tổng thành tiền đơn hàng or dịch vụ
   * @param field
   */
  getTotal(field: any = null) {
    if (field === null) {
      return 0;
    }
    if (this.models()) {
      return this.models()
        .map((res: any) => res[field])
        .reduce((total: any, b: any) => {
          return parseInt(total + '') + parseInt(b + '');
        }, 0);
    }
  }

  /**
   * Tổng tiền thanh toán đơn hàng or dịch vụ
   * @param field
   */
  getPayment(field: any = null) {
    if (field === null) {
      return 0;
    }
    if (this.models()) {
      return this.models()
        .map((res: any) => res[field])
        .reduce((payment: any, b: any) => {
          if (b === null) {
            b = 0;
          }
          return parseInt(payment + '') + parseInt(b + '');
        }, 0);
    }
  }

  /**
   * Phiếu in đơn hàng or dịch vụ
   * @param id
   */
  printOrderPlan(id: number) {
    this.openPrint(`/print/print-order/${id}`, `print-order-${id}`, this.orderPlanService.printOrderPlan(id));
  }

  lockServiceUsed(event: Event, id: number, isLock: number, updatedAt: Date | number): void {
    this.confirmationService.confirm({
      target: (event.target ?? undefined),
      message:
        isLock == this.modelService.NOT_LOCK
          ? 'Bạn có muốn khoá dịch vụ này?'
          : 'Bạn có muốn mở khoá dịch vụ này?',
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: {severity: 'danger', size: 'small'},
      accept: () => {
        this.modelService.lockServiceUsed(id, { isLock: isLock, updated_at: updatedAt }).subscribe({
          next: (res: any) => {
            this.noti.successRes(res);
            this.saveOderTreatmentPlan.emit(true);
          },
          error: () => {},
        });
      },
      reject: () => {},
    });
  }

  /**
   * Xóa đơn hàng or dịch vụ
   * @param event
   * @param id
   */
  deleteRecordTreatmentPlan(event: Event, id: number): void {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionDeletePlanOrder)) {
      this.noti.warnMsg(this.translate.instant('common.message.no_permission'));
      return;
    }
    this.isSubmitting.set(true);
    this.confirmationService.confirm({
      target: (event.target ?? undefined),
      message: 'Bạn có muốn xóa?',
      icon: 'pi pi-exclamation-triangle',
      acceptButtonProps: {severity: 'danger', size: 'small'},
      accept: () => {
        this.loadingService.showLoading('deleteRecord');
        this.orderPlanService
          .delete(id)
          .pipe(
            finalize(() => {
              this.isSubmitting.set(false);
              this.loadingService.hideLoading('deleteRecord');
            }),
          )
          .subscribe({
            next: () => {
              this.noti.successMsg(this.translate.instant('crm.order_plan.noti_delete_success'));
              this.saveOderTreatmentPlan.emit(true);
            },
          });
      },
      reject: () => {
        this.isSubmitting.set(false);
      },
    });
  }

  /**
   * Tải lại trang
   */
  override resetPage() {
    this.getList$.next(true);
  }

  /**
   * Chi tiết đơn hàng
   * @param model
   */
  // SlideOver chi tiết đơn hàng
  readonly orderDetailVisible = signal(false);
  readonly orderDetailId = signal<number | null>(null);
  readonly orderDetailSeed = signal<OrderPlanModel | null>(null);

  // SlideOver chi tiết 1 dịch vụ con — mở CHỒNG lên panel đơn khi bấm "Xem chi tiết dịch vụ".
  readonly serviceDetailVisible = signal(false);
  readonly serviceDetailId = signal<number | null>(null);
  readonly serviceDetailSeed = signal<OrderServiceUsedModel | null>(null);

  treatmentPlanDetail(model) {
    const id = model?.treatmentPlan?.id ?? model?.order_plan_id;
    if (!id) return;
    // Dòng list là dòng DỊCH VỤ, nhưng có kèm quan hệ `treatmentPlan` → dùng làm seed header cho
    // panel đơn (mã đơn, loại đơn, ngày đặt hiện ngay). Số tiền cấp đơn vẫn chờ API.
    this.orderDetailSeed.set(model?.treatmentPlan ?? null);
    this.orderDetailId.set(Number(id));
    this.orderDetailVisible.set(true);
  }

  /** Từ panel đơn bấm sang 1 dịch vụ con — dịch vụ đã kèm nhãn nên dùng luôn làm seed. */
  openServiceFromOrder(service: OrderServiceUsedModel) {
    if (!service?.id) return;
    this.serviceDetailSeed.set(service);
    this.serviceDetailId.set(Number(service.id));
    this.serviceDetailVisible.set(true);
  }


  getUserInfo(userId: number) {
    return this.userHelper.getUserInfo(userId);
  }
}
