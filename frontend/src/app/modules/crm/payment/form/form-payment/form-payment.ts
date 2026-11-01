import { ChangeDetectionStrategy, Component, computed, effect, inject, Injector, signal, untracked } from '@angular/core';
import { takeUntilDestroyed, toObservable } from '@angular/core/rxjs-interop';
import { FormField, minError, required, requiredError, SchemaPathTree, validate } from '@angular/forms/signals';
import { catchError, debounceTime, finalize, of } from 'rxjs';

import { CustomerModel } from '@/core/models/crm/customer.model';
import { OrderServiceUsedModel } from '@/core/models/crm/order-service-used.model';
import { PaymentModel } from '@/core/models/crm/payment.model';
import { branchActiveId, isAdmin, isDevelop } from '@/core/services/auth';
import { CustomerService } from '@/core/services/crm/customer.service';
import { OrderServiceUsedService } from '@/core/services/crm/order-service-used.service';
import { PaymentService } from '@/core/services/crm/payment.service';
import { ShowOtherBranch } from '@/modules/utilities/popup-show-other-branch/show-other-branch';
import { BaseApiService } from '@/share/services/base-api.service';
import { BaseSignalForm } from '@/share/base/base-signal-form';
import {
  CONFIG_OPTION_CRM_SELECT,

  PAYMENT_FOR,
  PERMISSION_SELECT,
} from '@/define';
import { FormShareImports } from '@/share/import/form-share-imports';
import { PaymentMethodAutocompleteFormControl } from '@/indexeddb/config/components/payment-method';
import {
  PaymentTypeAutocompleteFormControl,
  PaymentTypeCacheService,
} from '@/indexeddb/config/components/payment-type';
import { Listbox } from 'primeng/listbox';
import { UserOptionHelperService } from '@/indexeddb/config/components/user-option';

/** ĐÚNG 15 control của `buildForm()` cũ. Bản cũ gửi `getRawValue()` ở CẢ 2 nhánh; form không có control
 * `disabled` nào nên tập key không đổi theo trạng thái. */
const PAYLOAD_FIELDS = [
  'customer_id', 'branch_id', 'serviceUsed_idArr', 'method', 'card_type_id', 'bank_code',
  'payment_time', 'payment_of_date', 'note', 'payment_for', 'created_at', 'amount', 'updated_at',
] as const;

/** Field bind `<input pInputText>` → seed `''` (gotcha #9), kiểu `string` thuần (gotcha #28).
 * BE `UpdatePaymentInput` khai `BankCode *string` → `''` GHI ĐÈ DB, `null` giữ DB (xem `getPayload()`). */
const TEXT_INPUT_FIELDS = ['bank_code'] as const;

interface PaymentFormModel {
  id?: number;
  customer_id: number | null;
  branch_id: number | null;
  serviceUsed_idArr: number[];
  method: number | null;
  card_type_id: number | null;
  bank_code: string;
  payment_time: Date | number | null;
  payment_of_date: Date | number | null;
  note: string | null;
  payment_for: number | null;
  created_at: number | null;
  amount: number | null;
  updated_at: Date | number | null;
  /** 2 field CHỈ ĐỌC do BE trả về ở chế độ Sửa (không nằm trong `PAYLOAD_FIELDS`, không bao giờ gửi lên). */
  serviceUsed_id?: number | null;
  orderServiceUsed?: any;
  serviceInfo?: any;
}

/** Seed đủ mọi field sẽ bind, không field nào `undefined` (gotcha #1); field mảng seed `[]`. */
function emptyPaymentModel(): PaymentFormModel {
  return {
    customer_id: null, branch_id: null, serviceUsed_idArr: [], method: null,
    card_type_id: null, bank_code: '', payment_time: null, payment_of_date: null, note: null,
    payment_for: null, created_at: null, amount: null, updated_at: null,
  };
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-payment-form-payment',
  templateUrl: './form-payment.html',
  styleUrl: 'form-payment.scss',
  standalone: true,
  imports: [
    ...FormShareImports,
    FormField,
    PaymentMethodAutocompleteFormControl,
    PaymentTypeAutocompleteFormControl,
    Listbox,
  ],
})
export class FormPayment extends BaseSignalForm<PaymentFormModel> {
  modelService = inject(PaymentService) as unknown as BaseApiService<PaymentFormModel> & PaymentService;
  // Tên riêng: `BaseSignalForm` đã có `private injector` — trùng tên ở lớp con là `TS2415`/`TS4114`.
  private paymentInjector = inject(Injector);
  customerService = inject(CustomerService);
  orderServiceUsedService = inject(OrderServiceUsedService);
  payment!: PaymentModel;
  customer!: CustomerModel;
  private appointmentId: number | null = null;
  maxAmount: number = 0; // số tiền thanh toán lớn nhất
  serviceUsed!: OrderServiceUsedModel;
  maxPaymentTime: Date = this.timezoneService.todayEnd();
  minPaymentTime!: Date;
  optionsTimeBefore: boolean = false; // biến lưu option thời gian trước đó hay không
  optionsAllowPreviousDay: boolean = false;
  soTien: boolean = true;
  loadingTreatmentPlan: boolean = false;
  readonly loadingServiceList = signal<boolean>(true);
  readonly serviceUsedData = signal<{ value: number; label: string; branch_id: number }[]>([]);
  // Preview phân bổ waterfall khi thanh toán 1 phần cho nhiều dịch vụ — chỉ hiển thị khi
  // chọn >1 service. Thứ tự + công thức phải khớp CHÍNH XÁC allocatePaymentWaterfall (BE)
  // để preview không lệch với số tiền BE thực sự ghi nhận.
  readonly allocationPreview = signal<{ id: number; label: string; due: number; pay: number }[]>([]);
  private serviceQueue: { id: number; label: string; due: number; created_day: number }[] = [];
  selectedItems!: any[];
  selectAll = false;
  protected readonly PAYMENT_FOR = PAYMENT_FOR;
  private userOptionHelper = inject(UserOptionHelperService);
  private paymentTypeCache = inject(PaymentTypeCacheService);

  constructor() {
    super();
    this.customer = this.dialogConfig?.data?.customer;
    // Thu ngân EMR truyền appointment_id qua dialog data để gắn payment vào lượt khám;
    // thiếu forward vào payload → payment.appointment_id NULL → công nợ lượt khám luôn "unpaid".
    this.appointmentId = this.dialogConfig?.data?.appointment_id ?? null;
    this.optionsTimeBefore = !!this.userOptionHelper.getUserOptionInfo(
      CONFIG_OPTION_CRM_SELECT.OPTION_TIME_BEFORE,
    )?.options; // Check option có được chọn thời gian trước đó hay không
    this.optionsAllowPreviousDay = !!this.userOptionHelper.getUserOptionInfo(
      CONFIG_OPTION_CRM_SELECT.OPTION_ALLOW_PREVIOUS_DAY,
    )?.options;
    // 3 cascade bản cũ đăng ký trong `buildForm()` bằng `valueChanges`. Template CHỈ gọi `changeService`
    // (cho `serviceUsed_idArr`), 2 cascade còn lại không có handler nào ở template nên phải đăng ký ở đây.
    effectOnChangeLocal(
      () => this.rootField.method?.().value() ?? null,
      () => untracked(() => this.rootField.card_type_id!().value.set(null)),
    );
    effectOnChangeLocal(
      () => this.rootField.serviceUsed_idArr?.().value() ?? [],
      value => {
        this.soTien = false;
        this.getServiceUsed(value);
      },
    );
    // Preview phân bổ waterfall khi user gõ amount (multi-service) — debounce 300ms, KHÔNG tính lại theo
    // từng keystroke (B.19: gõ dở số sẽ ra preview sai/nhảy liên tục).
    toObservable(computed(() => this.rootField.amount?.().value() ?? null), { injector: this.paymentInjector })
      .pipe(debounceTime(300), takeUntilDestroyed(this.destroyRef))
      .subscribe((value: number | null) => this.updateAllocationPreview(value ?? 0));

  }

  override ngOnInit() {
    // `BaseSignalForm.model` khởi tạo `{}` (KHÔNG phải `null`/`undefined` như `BaseForm`) → `if (this.model())`
    // LUÔN true, create-mode chạy nhầm nhánh Sửa: khối `initialValue` (customer_id, payment_for...) không bao
    // giờ chạy → 2 field required KHÔNG render (`customer_id`, `payment_for`) rỗng → `rootField().invalid()`
    // luôn true → nút Lưu chết câm lặng, không báo lỗi ở đâu (gotcha #1). Phải đo bằng số key như base class.
    const loadedModel = this.model();
    if (loadedModel && Object.keys(loadedModel).length > 0) {
      if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdatePayment)) {
        if (!this.optionsTimeBefore) {
          // Giới hạn chọn ngày trong ngày tạo service (nếu có)
          const createdDay = this.model()?.orderServiceUsed?.created_day;
          if (createdDay && createdDay > 0) {
            const createdDate = new Date(createdDay * 1000);
            this.minPaymentTime = new Date(createdDate);
            this.minPaymentTime.setHours(0, 0, 0, 0);
          }

          const paymentDate = new Date(this.model().payment_time!);

          // Đầu ngày
          this.minPaymentTime = new Date(paymentDate);
          this.minPaymentTime.setHours(0, 0, 0, 0);

          // Cuối ngày
          this.maxPaymentTime = new Date(paymentDate);
          this.maxPaymentTime.setHours(23, 59, 59, 999);
        }
      }

      // Cho phép sửa số tiền khi update: listbox dịch vụ bị ẩn ở edit mode nên
      // serviceUsed_idArr.valueChanges (nơi soTien/maxAmount được set) không bao giờ fire.
      this.soTien = false;
      const editServiceId = this.model().serviceUsed_id;
      if (editServiceId) {
        this.orderServiceUsedService
          .get(editServiceId, {})
          .pipe(takeUntilDestroyed(this.destroyRef), catchError(() => of(null)))
          .subscribe((serviceUsed: any) => {
            if (serviceUsed) {
              this.maxAmount = this.getMaxAmount(serviceUsed);
            }
          });
      }

      this._applyModel(this.model() as any);
    } else {
      if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdatePayment)) {
        if (!this.optionsTimeBefore) {
          this.minPaymentTime = this.optionsAllowPreviousDay
            ? this.timezoneService.yesterdayStart()
            : this.timezoneService.todayStart();
        }
      }

      this.initialValue = {
        ...this.initialValue,
        customer_id: this.customer.id,
        branch_id: branchActiveId(),
        payment_for: PAYMENT_FOR.ORDER,
      } as any;

      this._applyModel({ ...this.modelService.getInitialObject(), ...this.initialValue } as any);
    }
    this.loadServiceUsedList();
  }

  /** Merge template đủ key (gotcha #1) + 2 fallback `now()`; `serviceUsed_id` (số ít, từ BE) trải thành
   * mảng `serviceUsed_idArr` đúng như bản cũ. */
  private _applyModel(model: any): void {
    const merged: any = {
      ...emptyPaymentModel(),
      ...model,
      serviceUsed_idArr: model.serviceUsed_id ? [model.serviceUsed_id] : (model.serviceUsed_idArr ?? []),
      payment_time: model.payment_time || this.today,
      payment_of_date: model.payment_of_date || this.today,
    };
    for (const key of TEXT_INPUT_FIELDS) if (merged[key] == null) merged[key] = '';
    this.model.set(merged as PaymentFormModel);
  }

  /** Bản cũ gọi trong `buildForm()`. */
  private loadServiceUsedList(): void {
    this.orderServiceUsedService
      .serviceUsedOfCustomer(this.customer.id!)
      .pipe(
        takeUntilDestroyed(this.destroyRef),
        finalize(() => this.loadingServiceList.set(false)),
        catchError(() => of(null)),
      )
      .subscribe((res: any) => {
        if (!res) return;
        const serviceUsed = Object.keys(res).map((key: any) => ({
          label: res[key].label,
          value: res[key].value,
          branch_id: res[key].branch_id,
        }));
        this.serviceUsedData.set(serviceUsed);
      });
  }

  /**
   * Hàm xử lý dịch vụ để thanh toán
   * @param ids
   */
  getServiceUsed(ids: number[] | null = null) {
    if (!ids || ids.length === 0) {
      this.maxAmount = 0;
      this.serviceQueue = [];
      this.allocationPreview.set([]);
      this.rootField.amount!().value.set(0);
      return;
    }

    let amount = 0;
    this.loadingTreatmentPlan = true;

    // Sử dụng Promise.all để chờ tất cả requests hoàn thành
    const requests = ids.map((id) => this.orderServiceUsedService.get(id, {}).toPromise());

    Promise.all(requests)
      .then((serviceUsedList: any[]) => {
        const queue: { id: number; label: string; due: number; created_day: number }[] = [];

        serviceUsedList.forEach((serviceUsed) => {
          if (!this.optionsTimeBefore && isAdmin()) {
            // `created_day` = ngày tạo dịch vụ điều trị, có sẵn trong response của
            // `GET /crm/order-service-used/{id}`. Trước đây chỗ này đọc `treatmentPlan.created_time`
            // — endpoint đó KHÔNG trả quan hệ `treatmentPlan` nên nhánh này chưa bao giờ chạy, giới
            // hạn ngày thu tiền của admin âm thầm không có hiệu lực.
            const createdTime = serviceUsed.created_day;
            if (createdTime && createdTime > 0) {
              this.minPaymentTime = new Date(createdTime * 1000);
            }
          }
          const due = this.getMaxAmount(serviceUsed);
          amount += due;

          const found = this.serviceUsedData().find((item) => item.value === serviceUsed.id);
          queue.push({
            id: serviceUsed.id,
            label: found?.label ?? '',
            due,
            created_day: serviceUsed.created_day ?? 0,
          });
        });

        this.maxAmount = amount;
        // Sort waterfall: created_day cũ hơn trả trước, id làm tie-break — PHẢI khớp
        // allocatePaymentWaterfall bên BE (payment_crud.go) để preview đúng số BE ghi nhận.
        this.serviceQueue = [...queue].sort((a, b) =>
          a.created_day !== b.created_day ? a.created_day - b.created_day : a.id - b.id,
        );

        this.rootField.amount!().value.set(amount);
        this.updateAllocationPreview(amount);
      })
      .catch(() => {
        this.noti.errorMsg(this.translate.instant('crm.payment.message.load_service_failed'));
      })
      .finally(() => {
        this.loadingTreatmentPlan = false;
      });
  }

  /**
   * Tính preview phân bổ waterfall: trả đủ dịch vụ cũ trước, dư chảy qua dịch vụ sau.
   * Chỉ hiển thị khi chọn >1 dịch vụ — chọn 1 dịch vụ thì amount đã rõ ràng đi thẳng vào nó.
   * Logic PHẢI đồng bộ allocatePaymentWaterfall (BE) — đổi 1 bên phải đổi bên còn lại.
   */
  private updateAllocationPreview(amount: number) {
    if (this.serviceQueue.length <= 1) {
      this.allocationPreview.set([]);
      return;
    }

    let remaining = amount || 0;
    const result: { id: number; label: string; due: number; pay: number }[] = [];
    for (const svc of this.serviceQueue) {
      if (remaining <= 0) break;
      if (svc.due <= 0) continue;
      const pay = Math.min(remaining, svc.due);
      result.push({ id: svc.id, label: svc.label, due: svc.due, pay });
      remaining -= pay;
    }
    this.allocationPreview.set(result);
  }

  /**
   * Chọn hết các dịch vụ cho thanh toán
   * @param event
   */
  onSelectAllChange(event: any) {
    const allServiceIds = this.serviceUsedData().map((item) => item.value);
    this.selectAll = event.checked;

    if (event.checked) {
      // Chọn tất cả dịch vụ
      this.rootField.serviceUsed_idArr!().value.set(allServiceIds);
      this.rootField.branch_id!().value.set(branchActiveId() ?? null);
      this.getServiceUsed(allServiceIds);
    } else {
      // Bỏ chọn tất cả
      this.rootField.serviceUsed_idArr!().value.set([]);
      this.maxAmount = 0;
      this.rootField.amount!().value.set(0);
      this.loadingTreatmentPlan = false;
    }
  }

  /**
   * Hàm tính số tiền thanh toán còn lại — khớp 100% guard BE (crm/service/payment_crud.go):
   * tạo: nợ = (totalEnd + vat_amount) − payment; sửa: cộng thêm số tiền đang lưu của phiếu.
   * `totalEnd` là Thành tiền CHƯA VAT, tiền khách trả (`payment`) đã gồm VAT ⇒ phải thu = totalEnd + vat_amount
   * (dòng cũ không VAT: vat_amount = 0/thiếu ⇒ số như trước).
   * `payment` (order_service_used) là tổng thu GỘP (thanh toán + chuyển cọc), KHÔNG trừ hoàn:
   * hoàn = huỷ bớt dịch vụ, công nợ không đổi và vẫn thu tiếp được (R-2) — KHÔNG trừ refundServiceUsed.
   * @param serviceUsed
   */
  getMaxAmount(serviceUsed: OrderServiceUsedModel | null = null) {
    let maxAmount = 0;
    if (serviceUsed !== null) {
      const payable = (serviceUsed?.totalEnd ?? 0) + (Number(serviceUsed?.vat_amount) || 0);
      const due = parseFloat(payable - (serviceUsed?.payment ?? 0) + '');
      maxAmount = (this.model().id ? due + parseFloat(this.model().amount + '') : due) || 0;
    } else {
      maxAmount = 0;
    }
    return maxAmount;
  }

  /**
   * Function tự động điền max tiền thanh toán
   */
  maxPayment() {
    this.rootField.amount!().value.set(this.maxAmount);
  }


  /**
   * Thay đổi dịch vụ khi thanh toán
   * @param value
   */
  changeService(value) {
    if (value && value.length > 0) {
      const currentBranchId = this.rootField.branch_id!().value();
      this.rootField.amount!().value.set(0);

      if (!currentBranchId) {
        const found = this.serviceUsedData().find((item) => item.value === value[0]);
        this.rootField.branch_id!().value.set(((found ? found.branch_id : branchActiveId()) ?? 0) as number);
      }
    } else {
      this.rootField.amount!().value.set(0);
      this.rootField.branch_id!().value.set(null);
    }
  }

  /**
   * Check branch user với khách hành trước khi submit lưu data
   */
  checkBeforeSubmit() {
    if (
      !isAdmin &&
      !isDevelop &&
      branchActiveId() != null &&
      branchActiveId() != this.rootField.branch_id!().value()
    ) {
      const dialogOtherBranch = this.dialogWrapperService.open(ShowOtherBranch, {
        header: this.translate.instant('common.label.notice'),
        data: {
          text: this.translate.instant('common.label.payment'),
        },
        width: '30%',
      });

      dialogOtherBranch.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((result: any) => {
        if (result) {
          this.onSubmit();
        }
      });
    } else {
      this.onSubmit();
    }
  }

  override onSubmit() {
    if (this.isSubmitting()) return;
    this.submitted.set(true);
    this.rootField().markAsTouched();
    if (this.rootField().invalid()) return;

    this.isSubmitting.set(true);

    if (this.isEditMode()) {
      this.modelService
        .updatePaymentCustomer(this.model().id!, this.getPayload(), {})
        .pipe(finalize(() => this.isSubmitting.set(false)), catchError(() => of(null)))
        .subscribe({
          next: (res: any) => {
            if (res) {
              this.noti.successRes(res);
              this.dialogRef.close(res.data);
            }
          },
        });
    } else {
      const createPayload = this.appointmentId
        ? { ...this.getPayload(), appointment_id: this.appointmentId }
        : this.getPayload();
      this.modelService
        .createPaymentCustomer(createPayload, {})
        .pipe(finalize(() => this.isSubmitting.set(false)), catchError(() => of(null)))
        .subscribe({
          next: (res: any) => {
            if (res) {
              this.noti.successMsg(this.translate.instant('crm.payment.message.create_payment_success'));
              this.dialogRef.close(res);
            }
          },
        });
    }
  }

  /**
   * [AC15 — nhóm Payment] `''` + field CHƯA dirty → `null` (chưa chạm → BE giữ DB vì `ToParams()` bỏ qua
   * con trỏ `nil`); `''` + ĐÃ dirty → `''` (user cố ý xoá → ghi đè). Chi tiết đầy đủ: `form-deposit.ts`.
   */
  protected override getPayload(): any {
    const raw = super.getPayload() as any;
    const payload: any = {};
    for (const key of PAYLOAD_FIELDS) payload[key] = raw[key];
    for (const key of TEXT_INPUT_FIELDS) {
      if (payload[key] === '' && !this.rootField[key]?.().dirty()) payload[key] = null;
    }
    return payload;
  }

  /** `card_type_id` bắt buộc khi phương thức đang chọn CÓ loại thẻ trong cache. */
  private cardTypeRequired(): boolean {
    const methodId = this.rootField.method?.().value();
    if (!methodId) return false;
    return this.paymentTypeCache.getByPaymentMethodId(methodId).length > 0;
  }

  protected override buildSchema(p: SchemaPathTree<PaymentFormModel>): void {
    required(p.customer_id!);
    required(p.branch_id!);
    required(p.method!);
    required(p.payment_for!);
    required(p.amount!);
    // `greaterThanNumber(0)` — KHÔNG `min()` (gotcha #20: đẩy metadata → ô số CLAMP thay vì báo đỏ).
    validate(p.amount!, ({ value }) => {
      const v = value();
      return v == null || Number(v) > 0 ? null : minError(0);
    });
    // Field MẢNG bắt buộc: `required()` KHÔNG coi `[]` là rỗng (gotcha #10) — phải viết tay, nếu không
    // submit được khi chưa chọn dịch vụ nào.
    validate(p.serviceUsed_idArr!, ({ value }) => ((value()?.length ?? 0) === 0 ? requiredError() : null));
    validate(p.card_type_id!, ({ value }) => (this.cardTypeRequired() && value() == null ? requiredError() : null));
  }
}

/** Copy helper `effectOnChange` — bỏ qua lần chạy đầu để cascade không đè dữ liệu vừa nạp khi Sửa. */
function effectOnChangeLocal<V>(read: () => V, action: (value: V) => void): void {
  let initialized = false;
  let prev: V;
  effect(() => {
    const value = read();
    if (!initialized) { initialized = true; prev = value; return; }
    if (value === prev) return;
    prev = value;
    action(value);
  });
}
