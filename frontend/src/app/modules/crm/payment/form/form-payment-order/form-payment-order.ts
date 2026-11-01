import { ChangeDetectionStrategy, Component, effect, inject, untracked } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormField, minError, required, requiredError, SchemaPathTree, validate } from '@angular/forms/signals';

import { catchError, finalize, of } from 'rxjs';

import { OrderPlanModel } from '@/core/models/order-plan.model';
import { PaymentModel } from '@/core/models/crm/payment.model';
import { branchActiveId, isAdmin, isDevelop } from '@/core/services/auth';
import { PaymentService } from '@/core/services/crm/payment.service';
import { OrderServiceUsedService } from '@/core/services/crm/order-service-used.service';
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
import { UserOptionHelperService } from '@/indexeddb/config/components/user-option';

/** ĐÚNG 12 control của `buildForm()` cũ. */
const PAYLOAD_FIELDS = [
  'parent_id', 'amount', 'payment_time', 'payment_of_date', 'branch_id', 'method',
  'card_type_id', 'bank_code', 'note', 'payment_for',
] as const;

/** Field bind `<input pInputText>` → seed `''` (gotcha #9), kiểu `string` thuần (gotcha #28). */
const TEXT_INPUT_FIELDS = ['bank_code'] as const;

interface PaymentOrderFormModel {
  id?: number;
  parent_id: number | null;
  amount: number | null;
  payment_time: Date | number | null;
  payment_of_date: Date | number | null;
  branch_id: number | null;
  method: number | null;
  card_type_id: number | null;
  bank_code: string;
  note: string | null;
  payment_for: number | null;
}

/** Seed đủ mọi field sẽ bind, không field nào `undefined` (gotcha #1). */
function emptyPaymentOrderModel(): PaymentOrderFormModel {
  return {
    parent_id: null, amount: null, payment_time: null, payment_of_date: null,
    branch_id: null, method: null, card_type_id: null, bank_code: '', note: null, payment_for: null,
  };
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  templateUrl: './form-payment-order.html',
  styles: [],
  standalone: true,
  imports: [
    ...FormShareImports,
    FormField,
    PaymentMethodAutocompleteFormControl,
    PaymentTypeAutocompleteFormControl,
  ],
})
export class FormPaymentOrder extends BaseSignalForm<PaymentOrderFormModel> {
  modelService = inject(PaymentService) as unknown as BaseApiService<PaymentOrderFormModel> & PaymentService;
  orderPlanModel: OrderPlanModel;
  maxPaymentTime: Date = this.timezoneService.todayEnd();
  minPaymentTime!: Date;
  optionsTimeBefore: boolean = false; // biến lưu option thời gian trước đó hay không
  optionsAllowPreviousDay: boolean = false;
  protected readonly PAYMENT_FOR = PAYMENT_FOR;
  private userOptionHelper = inject(UserOptionHelperService);
  private paymentTypeCache = inject(PaymentTypeCacheService);
  private orderServiceUsedService = inject(OrderServiceUsedService);

  constructor() {
    super();
    this.orderPlanModel = this.dialogConfig?.data?.orderPlanModel;
    this.minPaymentTime = new Date(this.orderPlanModel.created_time * 1000);
    this.optionsTimeBefore = !!this.userOptionHelper.getUserOptionInfo(
      CONFIG_OPTION_CRM_SELECT.OPTION_TIME_BEFORE,
    )?.options; // Check option có được chọn thời gian trước đó hay không
    this.optionsAllowPreviousDay = !!this.userOptionHelper.getUserOptionInfo(
      CONFIG_OPTION_CRM_SELECT.OPTION_ALLOW_PREVIOUS_DAY,
    )?.options;
    // Bản cũ: `method.valueChanges` → reset `card_type_id` (phần đổi validator nay là điều kiện tĩnh
    // `cardTypeRequired()` trong `buildSchema()`). Template form này KHÔNG gọi handler nào nên cascade
    // phải đăng ký ở đây, nếu không đổi phương thức sẽ giữ nguyên loại thẻ cũ.
    effectOnChangeLocal(
      () => this.rootField.method?.().value() ?? null,
      () => untracked(() => this.rootField.card_type_id!().value.set(null)),
    );

  }

  override ngOnInit() {
    if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdatePayment)) {
      if (!this.optionsTimeBefore) {
        this.minPaymentTime = this.optionsAllowPreviousDay
          ? this.timezoneService.yesterdayStart()
          : this.timezoneService.todayStart();
      }
    }

    // `orderPlanModel` đã được list cha (order-plan-in-customer.ts createPaymentOrder())
    // fetch đầy đủ qua OrderPlanService.get() TRƯỚC khi mở dialog này (loading tím
    // toàn cục) — ở đây chỉ tính toán đồng bộ, không cần gọi API/loading riêng nữa.
    // amount = tổng nợ các dịch vụ CHƯA KHOÁ — khớp đúng logic BE (CreatePaymentOrderFromPayment
    // luôn trả hết nợ mọi dịch vụ isLock=NOT_LOCK, bỏ qua giá trị amount FE gửi).
    // Nợ mỗi dòng = phải thu (totalEnd chưa VAT + vat_amount) − đã thu.
    const payableAmount = (this.orderPlanModel.orderServiceUseds ?? [])
      .filter((svc: any) => svc.isLock === this.orderServiceUsedService.NOT_LOCK)
      .reduce(
        (sum: number, svc: any) => sum + Math.max(+svc.totalEnd + (Number(svc.vat_amount) || 0) - +svc.payment, 0),
        0,
      );

    this.initialValue = {
      ...this.initialValue,
      branch_id: this.orderPlanModel?.branch_id,
      payment_for: PAYMENT_FOR.ORDER,
      amount: payableAmount,
      parent_id: this.orderPlanModel?.id,
    } as any;

    this._applyModel({ ...this.modelService.getInitialObject(), ...this.initialValue } as any);
  }

  /** Merge template đủ key (gotcha #1) + 2 fallback `now()` của bản cũ. */
  private _applyModel(model: any): void {
    const merged: any = {
      ...emptyPaymentOrderModel(),
      ...model,
      payment_time: model.payment_time || this.today,
      payment_of_date: model.payment_of_date || this.today,
    };
    for (const key of TEXT_INPUT_FIELDS) if (merged[key] == null) merged[key] = '';
    this.model.set(merged as PaymentOrderFormModel);
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
        data: { text: this.translate.instant('common.label.payment') },
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

    this.modelService
      .createPaymentOrder(this.getPayload(), {})
      .pipe(
        finalize(() => {
          this.isSubmitting.set(false);
        }),
        catchError(() => of(null)),
      )
      .subscribe({
        next: (res) => {
          if (res) {
            this.noti.successRes(res);
            this.dialogRef.close(true);
          }
        },
      });
  }

  /**
   * [AC15] `''` + field CHƯA dirty → `null` (giữ DB); `''` + ĐÃ dirty → `''` (user cố ý xoá).
   * Chi tiết: `form-deposit.ts` (pilot nhóm A).
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

  protected override buildSchema(p: SchemaPathTree<PaymentOrderFormModel>): void {
    required(p.parent_id!);
    required(p.amount!);
    required(p.branch_id!);
    required(p.method!);
    // `greaterThanNumber(0)` — KHÔNG `min()` (gotcha #20: đẩy metadata → ô số CLAMP).
    validate(p.amount!, ({ value }) => {
      const v = value();
      return v == null || Number(v) > 0 ? null : minError(0);
    });
    validate(p.card_type_id!, ({ value }) => (this.cardTypeRequired() && value() == null ? requiredError() : null));
  }
}


/** Copy helper `effectOnChange` (quyết định epic: KHÔNG import chung) — bỏ qua lần chạy đầu để không
 * xoá `card_type_id` vừa nạp ở chế độ Sửa. */
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
