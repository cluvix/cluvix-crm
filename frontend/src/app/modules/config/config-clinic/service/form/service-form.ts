import { ChangeDetectionStrategy, Component, effect, inject, untracked } from '@angular/core';
import {
  FormField,
  max,
  min,
  required,
  SchemaPath,
  SchemaPathTree,
} from '@angular/forms/signals';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { finalize } from 'rxjs';

import { ServiceModel } from '@/core/models/config/clinic/service.model';
import { ServiceService } from '@/core/services/config/clinic/service.service';
import { BaseSignalForm } from '@/share/base/base-signal-form';
import { FormShareImports } from '@/share/import/form-share-imports';
import {
  SERVICE_CATEGORY_OPTIONS,
  SERVICE_CATEGORY_SELECT,
  SERVICE_DISCOUNT_TYPE_SELECT,
  UNIT_CATEGORY_SELECT,
} from '@/define';

/**
 * Signal Forms cần KEY tồn tại thật trên object model để dựng sub-field (`rootField.x` trả `undefined`,
 * không phải FieldTree, nếu key không có mặt — kể cả khi TypeScript coi field là optional). Khác hẳn
 * `FormGroup` gốc (`fb.group({name: [model.name, ...]})` luôn tạo control bất kể `model.name` có hay
 * không). `getInitialObject()`/BE response (Go `omitempty`) có thể thiếu key cho field rỗng — merge
 * qua template này TRƯỚC khi set vào `model` để đảm bảo đủ key cho mọi field `buildSchema()`/template
 * đụng tới (cả create lẫn edit mode).
 */
function emptyServiceModel(): ServiceModel {
  // TẤT CẢ field dùng `null` (KHÔNG `undefined`) — đã verify thực nghiệm qua console trên browser
  // thật: Signal Forms coi `undefined` là "field không tồn tại", `rootField.x` trả `undefined` (không
  // callable) VĨNH VIỄN cho tới khi field có giá trị THẬT SỰ khác undefined — dù object CÓ key đó.
  // `0` KHÔNG dùng được cho field số có `required()` vì `required()` coi `0` là hợp lệ (giống hệt
  // gotcha `Validators.required` của Reactive Forms, `fe-patterns-gotchas-form.md`) — `null` vừa giữ
  // field bind được vừa đúng ngữ nghĩa "rỗng" cho `required()`. Field mảng dùng `[]` (không `null`).
  // NGOẠI LỆ — field text bind `<input pInputText type="text">` (name/code_old) PHẢI
  // seed `''`, KHÔNG `null` (gotcha #9 `angular22.md`): Signal Forms native-control adapter coi
  // `<input type="text">` có giá trị hiện tại là `null` là field SỐ, ép `Number(text)` mỗi lần gõ —
  // gõ chữ luôn lỗi `parse`, gõ số mất số 0 đầu. `''` giữ đúng `typeof === 'string'`, tránh nhánh đó.
  return {
    name: '', code_old: '', min_price: null, max_price: null,
    unit_id: null, serviceCategory_id: null, service_type_id: null,
    status: null, time_reminder: null,
    commission_telesale: null, commission_telesale_new: null, commission_telesale_type: null,
  } as unknown as ServiceModel;
}

/**
 * Đăng ký 1 `effect()` CHỈ phản ứng khi giá trị THAY ĐỔI so với lần đọc trước — bỏ qua lần chạy đầu
 * tiên (Angular `effect()` luôn chạy ngay lúc tạo, khác `valueChanges.subscribe()` của Reactive Forms
 * chỉ bắn khi có thay đổi THẬT SỰ sau khi subscribe). Không skip lần đầu sẽ khiến cascade-reset
 * chạy ngay khi mở form edit, xoá mất dữ liệu vừa load.
 * Pattern giống `fe-patterns-gotchas-signals.md` mục "model() 2 chiều".
 */
function effectOnChange<V>(read: () => V, action: (value: V) => void): void {
  let initialized = false;
  let prev: V;
  effect(() => {
    const value = read();
    if (!initialized) {
      initialized = true;
      prev = value;
      return;
    }
    if (value === prev) return;
    prev = value;
    action(value);
  });
}

/**
 * Template không truyền được `SchemaPath` nên `changeCommissionType`/`getMaxCommissionByType` phải
 * nhận TÊN field dạng string. Ràng buộc `& keyof ServiceModel` để đổi tên field trong model là lỗi
 * biên dịch ngay tại template — thay vì `string` + `as any` không ai bắt được.
 */
type CommissionGroup = 'telesale';
type CommissionTypeKey = `commission_${CommissionGroup}_type` & keyof ServiceModel;
type CommissionValueKey = (`commission_${CommissionGroup}` | `commission_${CommissionGroup}_new`) & keyof ServiceModel;

/** min(0) + max(100 khi percent) cho 1 cặp field hoa hồng — thay `wireCommissionValidators()` gốc. */
function wireCommissionRules(value: SchemaPath<number | undefined>, type: SchemaPath<number | undefined>): void {
  min(value as SchemaPath<number>, 0);
  max(value as SchemaPath<number>, ({ valueOf }) =>
    valueOf(type) === SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT ? 100 : undefined,
  );
}

/**
 * Key CÓ trong model (đến từ response `GET /config/service/:id`) nhưng KHÔNG được nằm trong payload
 * gửi lên BE. Signal Forms `extractValue()` trả về MỌI key của object model — khác hẳn `FormGroup`
 * cũ chỉ trả các control đã khai báo tường minh — nên field quan hệ/read-only lọt thẳng vào payload.
 *
 * Các key này là field kỹ thuật server tự quản (BE cũng đã tự `delete` phần lớn) — loại từ FE để
 * payload chỉ mang đúng thứ form thật sự sở hữu. `updated_at` PHẢI giữ lại (optimistic lock — BE
 * story-17 đổi khoá lock từ `version` sang `updated_at`).
 */
const NON_SUBMIT_KEYS = [
  'code',
  'id_old',
  'SortOrder',
  'created_at',
  'created_by',
  'updated_by',
  // Field runtime do `ServiceService.addLabel()` gắn thêm — chỉ có ở luồng list, strip phòng xa vì
  // nếu lọt vào payload sẽ gây lỗi "unknown column".
  'unitLabel',
  'serviceTypeLabel',
  // Form không còn quản lý nhóm tuổi + trả góp — không gửi để giữ nguyên giá trị đã lưu (sửa) /
  // nhận default của DB (tạo mới: `child` = 0 không phân biệt, `is_installment` = 0).
  'child',
  'is_installment',
  'installment_config',
] as const;

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-service-form',
  templateUrl: './service-form.html',
  imports: [
    ...FormShareImports,
    FormField,
  ],
  standalone: true,
})
export class ServiceForm extends BaseSignalForm<ServiceModel> {
  modelService = inject(ServiceService);
  protected readonly UNIT_CATEGORY_SELECT = UNIT_CATEGORY_SELECT;
  protected readonly SERVICE_CATEGORY_OPTIONS = SERVICE_CATEGORY_OPTIONS;
  protected readonly SERVICE_DISCOUNT_TYPE_SELECT = SERVICE_DISCOUNT_TYPE_SELECT;

  constructor() {
    super();

    // Auto nâng max_price theo min_price (đối chiếu dòng 507-515 gốc).
    effectOnChange(
      () => this.rootField.min_price!().value(),
      (minVal) => {
        const maxState = this.rootField.max_price!();
        const maxVal = untracked(() => maxState.value());
        if (maxVal == null || (minVal != null && maxVal < minVal)) {
          untracked(() => maxState.value.set(minVal ?? null));
        }
      },
    );
  }

  override ngOnInit() {
    const dialogModel = this.dialogConfig?.data?.model as ServiceModel | undefined;

    if (dialogModel) {
      this._applyModel(dialogModel);
    } else {
      const seeded: Partial<ServiceModel> = {
        ...this.initialValue,
        serviceCategory_id: SERVICE_CATEGORY_SELECT.SERVICE_CATEGORY_SERVICE || null,
      };
      const obj = { ...this.modelService.getInitialObject(), ...seeded } as ServiceModel;
      this._applyModel(obj);
    }
  }

  /** Bù đủ key cho FieldTree trước khi set model (xem `emptyServiceModel`). */
  private _applyModel(model: ServiceModel): void {
    const merged = { ...emptyServiceModel(), ...model };

    this.model.set(merged);
  }

  protected override buildSchema(p: SchemaPathTree<ServiceModel>): void {
    required(p.name!);
    required(p.min_price!);
    required(p.max_price!);
    required(p.unit_id!);
    required(p.serviceCategory_id!);
    required(p.service_type_id!);

    wireCommissionRules(p.commission_telesale!, p.commission_telesale_type!);
    wireCommissionRules(p.commission_telesale_new!, p.commission_telesale_type!);
  }

  /**
   * `ServiceService` có `createService()`/`updateService()` riêng (không phải `create()`/`update()`
   * chung của `BaseApiService`). Phải override thay vì dùng `BaseSignalForm.onSubmit()` chung.
   */
  override onSubmit(): void {
    if (this.isSubmitting()) return;

    this.submitted.set(true);
    this.rootField().markAsTouched();
    if (this.rootField().invalid()) {
      this.noti.warnMsg(this.translate.instant('common.validation.please_check_required_fields'));
      return;
    }

    this.isSubmitting.set(true);

    const formValue = this.getPayload();

    let responseData: any = null;

    const handleSuccess = (res: any) => {
      responseData = res;
      this.noti.successMsg(this.model().id ? res.message : this.translate.instant('common.message.create_success'));
    };

    const handleFinalize = () => {
      this.isSubmitting.set(false);
      if (responseData) {
        this.dialogRef.close(this.model().id ? responseData.data : responseData);
      }
    };

    if (this.model().id && !this.dialogConfig?.data?.clone) {
      this.modelService
        .updateService(this.model().id!, formValue)
        .pipe(finalize(handleFinalize))
        .subscribe({ next: handleSuccess });
    } else {
      this.modelService
        .createService(formValue)
        .pipe(finalize(handleFinalize))
        .subscribe({ next: handleSuccess });
    }
  }

  /**
   * `extractValue()` trả mọi key của model — strip field quan hệ/read-only trước khi gửi BE.
   * Xem `NON_SUBMIT_KEYS` cho lý do từng key.
   */
  protected override getPayload(): any {
    const payload = { ...super.getPayload() };
    for (const key of NON_SUBMIT_KEYS) {
      delete payload[key];
    }
    return payload;
  }

  getMaxCommissionByType(typeField: CommissionTypeKey): number | null {
    const value = (this.rootField as any)[typeField]?.()?.value?.();
    return value === SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT ? 100 : null;
  }

  changeCommissionType(typeField: CommissionTypeKey, ...valueFields: CommissionValueKey[]): void {
    const typeState = (this.rootField as any)[typeField]();
    const current = typeState.value();
    const isPercent = current === SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT;
    const newType = isPercent
      ? SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_CURRENCY
      : SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT;
    typeState.value.set(newType);

    if (!isPercent) {
      valueFields.forEach((key) => {
        const state = (this.rootField as any)[key]();
        if (state.value() > 100) state.value.set(100);
      });
    }
  }
}
