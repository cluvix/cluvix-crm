import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize } from 'rxjs';
import { FormField, required, SchemaPathTree, validate, requiredError } from '@angular/forms/signals';

import { DuplicateCode } from '@/modules/crm/customer/form/duplicate-code/duplicate-code';
import { CustomerModel } from '@/core/models/crm/customer.model';
import { isDepartmentReceptionist } from '@/core/services/auth';
import { CustomerService } from '@/core/services/crm/customer.service';
import { BaseApiService } from '@/share/services/base-api.service';
import { BaseSignalForm } from '@/share/base/base-signal-form';
import { FormShareImports } from '@/share/import/form-share-imports';

/** Các control của `buildForm()` bản Reactive Forms cũ, THEO ĐÚNG THỨ TỰ khai báo (trừ
 * `schedule_time`/`bookType` đã bỏ khỏi form 2026-11-01).
 *
 * ⚠ Whitelist ở đây quan trọng hơn mọi form trước: BE `CreateCustomerSwitch`
 * (`crm/customer/handler/customer.go:458`) **clone toàn bộ bản ghi khách cũ rồi override theo TỪNG KEY
 * của body**:
 * ```go
 * new := clone(old); for k, v := range body { new[k] = v }
 * ```
 * ⇒ **key vắng = kế thừa giá trị của khách cũ; key có (kể cả `null`) = GHI ĐÈ**. Payload phình thêm 1 key
 * là mất 1 giá trị kế thừa. */
const PAYLOAD_FIELDS_ALWAYS = [
  'payment_user_id',
  'created_day',
  'note',
  'appointment_date',
  'content',
  // story-43 (E6 bỏ company): `source_id`/`type_service_id` không còn khoá theo "đã chọn thương
  // hiệu" (xem lịch sử ở PAYLOAD_FIELDS_AFTER_COMPANY, epic fe-remove-company-picker) — luôn gửi.
  'source_id',
  'type_service_id',
] as const;

/** Bản parse THUẦN của `BaseForm.handleSelectedDate()` + `formatDate()` — 2 hàm này CHỈ có ở `BaseForm`
 * (gotcha #22). */
function parseTypedDate(event: Event, typed: unknown): Date | null {
  if ((event as unknown as { keyCode?: number }).keyCode !== 13) return null;
  let s = typeof typed === 'string' ? typed : '';
  if (!s) return null;
  if (!s.includes('/')) s = `${s.slice(0, 2)}/${s.slice(2, 4)}/${s.slice(4, 8)}`;
  const [day, month, year] = s.split('/');
  if (year && +month <= 12 && +day <= 31) return new Date(+year, +month - 1, +day);
  return null;
}

interface SwitchFormModel {
  id?: number;
  source_id: number | null;
  type_service_id: number | null;
  payment_user_id: number | null;
  created_day: Date | number | null;
  /** `<textarea pTextarea>` → seed `null` (gotcha #9 không áp cho TEXTAREA) */
  note: string | null;
  appointment_date: Date | null;
  content: string | null;
}

/** Seed đủ mọi field sẽ bind, không field nào `undefined` (gotcha #1). */
function emptySwitchModel(): SwitchFormModel {
  return {
    source_id: null,
    type_service_id: null,
    payment_user_id: null,
    created_day: null,
    note: null,
    appointment_date: null,
    content: null,
  };
}

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-customer-switch-form',
  templateUrl: './switch-form.html',
  styleUrls: ['../form.scss'],
  standalone: true,
  imports: [...FormShareImports, FormField],
})
export class SwitchForm extends BaseSignalForm<SwitchFormModel> {
  modelService = inject(CustomerService) as unknown as BaseApiService<SwitchFormModel> &
    Pick<CustomerService, 'createCustomerSwitch' | 'getCustomerByCode'>;
  selectedDateCreatedDay!: any;
  minDate: Date = this.timezoneService.todayStart();
  maxDate: Date = this.timezoneService.todayEnd();
  customer!: CustomerModel;

  /** Bản cũ thêm `Validators.required` cho `appointment_date` khi user thuộc bộ phận Lễ tân. */
  private readonly isReceptionist = isDepartmentReceptionist();

  constructor() {
    super();
    this.customer = this.dialogConfig?.data?.customer;
    // Story-61: 3 dropdown (Nguồn, Bác sĩ hẹn, Direct sale) đã bỏ input `companyId`/`trackScopeFields`
    // company — form-control tự đọc toàn bộ danh mục qua `CacheService`, không còn lọc theo thương hiệu.
  }

  /** Override HOÀN TOÀN — bản cũ `initialForm(this.customer)` rồi `buildForm(model)` chỉ đọc `created_day`
   * từ khách cũ, mọi control khác khởi tạo `null` cứng. (`bookType`/`schedule_time` đã bỏ khỏi form.) */
  override ngOnInit(): void {
    const cust = (this.customer ?? {}) as any;
    const obj: SwitchFormModel = {
      ...emptySwitchModel(),
      created_day: cust.created_day ?? null,
    };
    this.model.set(obj);
  }

  override onSubmit() {
    if (this.isSubmitting()) return;
    this.submitted.set(true);
    this.rootField().markAsTouched();

    if (this.rootField().invalid()) return;

    this.isSubmitting.set(true);

    if (this.customer != null) {
      // kiểm tra mã khách này đã là mã khách cũ của khách hàng nào chưa
      this.modelService
        .getCustomerByCode(this.customer.code!, {
          params: {},
        })
        .pipe(finalize(() => this.isSubmitting.set(false)))
        .subscribe({
          next: (res: any) => {
            if (res.num > 0) {
              // nếu như đã tồn tại thì show popup confirm
              const dialogRef = this.dialogWrapperService.open(DuplicateCode, {
                width: '700px',
                autoZIndex: true,
                showHeader: true,
                closeOnEscape: false,
                contentStyle: { overflow: 'auto', padding: '0' },
                header: this.translate.instant('common.label.possible_duplicate_customer'),
                data: { duplicateModels: res.data },
              });
              dialogRef.onClose.pipe(takeUntilDestroyed(this.destroyRef)).subscribe((isConfirmCreate: any) => {
                if (isConfirmCreate) {
                  this.createCustomer();
                }
              });
            } else {
              this.createCustomer();
            }
          },
        });
    }
  }

  /**
   * Hàm tạo mới switch
   */
  createCustomer() {
    this.modelService
      .createCustomerSwitch(this.customer?.id!, this.getPayload(), {})
      .pipe(
        finalize(() => {
          this.isSubmitting.set(false);
        }),
      )
      .subscribe({
        next: (res: any) => {
          this.noti.successMsg(this.translate.instant('crm.customer.create_customer_success'));
          this.dialogRef.close(res?.data ?? res);
        },
      });
  }

  protected override getPayload(): any {
    const raw = super.getPayload() as any;
    const payload: any = {};
    for (const key of PAYLOAD_FIELDS_ALWAYS) payload[key] = raw[key];
    return payload;
  }

  /**
   * Xử lý format data date khi nhập tay vào calendar ngày sinh
   */
  inputDateCreatedDay(value: any) {
    this.selectedDateCreatedDay = value;
  }

  /**
   * Khi nhấn phím enter thì ngày sinh sẽ đc fill vào calendar ngày sinh
   */
  handleKeyPressCreatedDay(event: Event) {
    const parsed = parseTypedDate(event, this.selectedDateCreatedDay);
    if (!parsed) {
      this.noti.warnMsg(this.translate.instant('crm.customer.invalid_collected_date_warn'));
      return;
    }
    this.rootField.created_day!().value.set(parsed);
  }

  protected override buildSchema(p: SchemaPathTree<SwitchFormModel>): void {
    required(p.created_day!);

    // story-43 (E6 bỏ company): 2 field này trước đây khoá tới khi "chọn thương hiệu" — không còn
    // khái niệm đó nữa nên luôn bắt buộc, không còn `disabled`.
    required(p.source_id!);
    required(p.type_service_id!);

    // Lễ tân → bắt buộc nhập lịch hẹn (bản cũ: `addValidators()` sau khi dựng group).
    validate(p.appointment_date!, ({ value }) =>
      this.isReceptionist && value() == null ? requiredError() : null,
    );
  }
}
