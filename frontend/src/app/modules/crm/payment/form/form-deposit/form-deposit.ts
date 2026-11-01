import { ChangeDetectionStrategy, Component, effect, inject, untracked } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { catchError, finalize, of } from 'rxjs';
import { FormField, minError, required, requiredError, SchemaPathTree, validate } from '@angular/forms/signals';
import {
  PaymentTypeAutocompleteFormControl,
  PaymentTypeCacheService,
} from '@/indexeddb/config/components/payment-type';

import { CustomerModel } from '@/core/models/crm/customer.model';
import { branchActiveId, isAdmin, isDepartmentTelesale, isDevelop } from '@/core/services/auth';
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
import { UserOptionHelperService } from '@/indexeddb/config/components/user-option';

/** ĐÚNG 13 control của `buildForm()` cũ. */
const PAYLOAD_FIELDS = [
  'customer_id', 'branch_id', 'method', 'card_type_id', 'bank_code', 'payment_time',
  'note', 'payment_for', 'created_at', 'amount', 'deposit_type', 'updated_at',
] as const;

/** Field bind `<input pInputText type="text">` → seed `''` (gotcha #9), kiểu `string` thuần (gotcha #28).
 * `note` là `<textarea pTextarea>` nên KHÔNG nằm đây. */
const TEXT_INPUT_FIELDS = ['bank_code'] as const;

interface DepositFormModel {
  id?: number;
  customer_id: number | null;
  branch_id: number | null;
  method: number | null;
  card_type_id: number | null;
  bank_code: string;
  payment_time: Date | number | null;
  note: string | null;
  payment_for: number | null;
  created_at: number | null;
  amount: number | null;
  deposit_type: number | null;
  updated_at: Date | number | null;
}

/** Seed đủ mọi field sẽ bind, không field nào `undefined` (gotcha #1). */
function emptyDepositModel(): DepositFormModel {
  return {
    customer_id: null, branch_id: null, method: null, card_type_id: null,
    bank_code: '', payment_time: null, note: null, payment_for: PAYMENT_FOR.DEPOSIT,
    created_at: null, amount: null, deposit_type: null, updated_at: null,
  };
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-payment-form-deposit',
  templateUrl: './form-deposit.html',
  styles: [],
  standalone: true,
  imports: [
    ...FormShareImports,
    FormField,
    PaymentMethodAutocompleteFormControl,
    PaymentTypeAutocompleteFormControl,
  ],
})
export class FormDeposit extends BaseSignalForm<DepositFormModel> {
  modelService = inject(PaymentService) as unknown as BaseApiService<DepositFormModel> & PaymentService;
  customer!: CustomerModel;
  maxPaymentTime: Date = this.timezoneService.todayEnd();
  minPaymentTime!: Date;
  optionsTimeBefore: boolean = false; // biến lưu option thời gian trước đó hay không
  optionsAllowPreviousDay: boolean = false;
  typeDeposit: any[] = [
    { name: 'Cọc tạm', nameKey: 'common.label.temporary_deposit', value: 2, key: 'cTa' },
    { name: 'Cọc giữ khuyến mại', nameKey: 'crm.payment.form.deposit_keep_promotion', value: 1, key: 'gCh' },
  ]; // loại đặt cọc
  private userOptionHelper = inject(UserOptionHelperService);
  private paymentTypeCache = inject(PaymentTypeCacheService);

  constructor() {
    super();
    this.customer = this.dialogConfig?.data?.customer;
    this.optionsTimeBefore = !!this.userOptionHelper.getUserOptionInfo(
      CONFIG_OPTION_CRM_SELECT.OPTION_TIME_BEFORE,
    )?.options; // Check option có được chọn thời gian trước đó hay không
    this.optionsAllowPreviousDay = !!this.userOptionHelper.getUserOptionInfo(
      CONFIG_OPTION_CRM_SELECT.OPTION_ALLOW_PREVIOUS_DAY,
    )?.options;
    // [QA pass 1 — finding high] Bản gốc có `method.valueChanges` → reset `card_type_id`, nằm TRONG
    // `buildForm()` nên bị xoá cùng khi convert (gotcha #45). Template form này KHÔNG gọi handler nào →
    // phải đăng ký lại ở đây, nếu không đổi phương thức thanh toán sẽ giữ nguyên loại thẻ của phương
    // thức CŨ và lọt vào payload (không validator nào chặn vì `cardTypeRequired()` lúc đó là false).
    effectOnChangeLocal(
      () => this.rootField.method?.().value() ?? null,
      () => untracked(() => this.rootField.card_type_id!().value.set(null)),
    );

  }

  override ngOnInit() {
    const dialogModel = this.model() as any;
    if (dialogModel && Object.keys(dialogModel).length > 0) {
      // Nếu option cho phép sửa trong ngày
      if (!this.permissionsService.getPermission(PERMISSION_SELECT.permissionUpdatePayment)) {
        if (!this.optionsTimeBefore) {
          const createdDate = new Date(dialogModel.payment_time!);

          // Đầu ngày
          this.minPaymentTime = new Date(createdDate);
          this.minPaymentTime.setHours(0, 0, 0, 0);

          // Cuối ngày
          this.maxPaymentTime = new Date(createdDate);
          this.maxPaymentTime.setHours(23, 59, 59, 999);
        }
      }
      this._applyModel(dialogModel);
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
        deposit_type: this.modelService.TYPE_DEPOSIT_TEMP,
        payment_time: this.today,
      } as any;

      if (isDepartmentTelesale()) {
        this.initialValue = {
          ...this.initialValue,
          deposit_type: this.modelService.TYPE_DEPOSIT_RESERVE,
        } as any;
      }

      this._applyModel({
        ...this.modelService.getInitialObject(),
        ...this.initialValue,
      } as any);
    }
  }

  /** Merge template đủ key (gotcha #1); `payment_for` là hằng số cứng của bản cũ, không lấy từ model. */
  private _applyModel(model: any): void {
    const merged: any = {
      ...emptyDepositModel(),
      ...model,
      payment_for: PAYMENT_FOR.DEPOSIT,
    };
    for (const key of TEXT_INPUT_FIELDS) {
      if (merged[key] == null) merged[key] = '';
    }
    this.model.set(merged as DepositFormModel);
  }

  /**
   * Check branch nhân viên với khách hành trước khi submit lưu data
   */
  checkBeforeSubmit() {
    // nếu như branch nhân viên khác với branch khách hàng thì show popup cảnh báo, trừ admin và develop
    if (
      !isAdmin &&
      !isDevelop &&
      branchActiveId() != null &&
      branchActiveId() != this.rootField.branch_id!().value()
    ) {
      const dialogOtherBranch = this.dialogWrapperService.open(ShowOtherBranch, {
        header: this.translate.instant('common.label.notice'),
        data: {
          text: this.translate.instant('crm.payment.message.text_add_deposit'),
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

    const payload = this.getPayload();

    if (this.isEditMode()) {
      this.modelService
        .updateDepositCustomer(this.model().id!, payload, {})
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
      this.modelService
        .createDepositCustomer(payload, {})
        .pipe(finalize(() => this.isSubmitting.set(false)), catchError(() => of(null)))
        .subscribe({
          next: (res: any) => {
            if (res) {
              this.noti.successMsg(this.translate.instant('crm.payment.message.create_deposit_success'));
              this.dialogRef.close(res);
            }
          },
        });
    }
  }

  /**
   * [AC15 — trọng tâm nhóm Payment] Field text rỗng: phân biệt "CHƯA BAO GIỜ chạm" với "user CỐ Ý xoá".
   *
   * BE `UpdatePaymentInput.ToParams()` (`crm/payment/handler/dto.go`) chỉ đưa field con trỏ vào map khi
   * khác `nil`: `if in.BankCode != nil { m["bank_code"] = *in.BankCode }`. Go unmarshal JSON `null` →
   * con trỏ `nil` ⇒ **gửi `null` ≡ không gửi key ≡ giữ nguyên giá trị DB**; còn `""` là con trỏ trỏ tới
   * chuỗi rỗng ⇒ **GHI ĐÈ DB thành rỗng**.
   *
   * Bản Reactive Forms phân biệt được 2 tình huống một cách tự nhiên:
   *   - control chưa từng chạm, model là `null` → `rfModel.value` trả `null` → giữ DB;
   *   - user xoá trắng ô     → `''`            → ghi đè rỗng (đúng ý người dùng).
   * Signal Forms buộc seed `''` (gotcha #9) nên **cả 2 tình huống đều ra `''`** → mất khả năng phân biệt.
   * Quy `'' → null` vô điều kiện thì an toàn dữ liệu nhưng **user không xoá được ghi chú/mã NH nữa**.
   *
   * → Dùng `dirty()` của chính field để khôi phục đúng ngữ nghĩa cũ:
   *   `''` + field CHƯA dirty  → `null` (chưa chạm → giữ DB)
   *   `''` + field ĐÃ dirty    → `''`   (user cố ý xoá → ghi đè)
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

  /** `card_type_id` bắt buộc khi phương thức thanh toán đang chọn CÓ loại thẻ trong cache. */
  private cardTypeRequired(): boolean {
    const methodId = this.rootField.method?.().value();
    if (!methodId) return false;
    return this.paymentTypeCache.getByPaymentMethodId(methodId).length > 0;
  }

  protected override buildSchema(p: SchemaPathTree<DepositFormModel>): void {
    required(p.customer_id!);
    required(p.branch_id!);
    required(p.method!);
    required(p.note!);
    required(p.amount!);
    // Bản cũ: `greaterThanNumber(0)` — số tiền phải > 0. KHÔNG dùng `min()` của Signal Forms: nó đẩy
    // metadata xuống `@Input min` của `p-inputnumber` → ô số CLAMP thay vì báo đỏ (gotcha #20).
    validate(p.amount!, ({ value }) => {
      const v = value();
      return v == null || Number(v) > 0 ? null : minError(0);
    });
    // Validator ĐỘNG cho `card_type_id` — điều kiện `cardTypeRequired()`; phần RESET giá trị do
    // cascade `effectOnChangeLocal` trong constructor lo.
    validate(p.card_type_id!, ({ value }) =>
      this.cardTypeRequired() && value() == null ? requiredError() : null,
    );
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
