import { ChangeDetectionStrategy, Component, computed, effect, inject, signal, untracked } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import {
  disabled,
  emailError,
  FormField,
  maxLength,
  required,
  SchemaPath,
  SchemaPathTree,
  validate,
} from '@angular/forms/signals';
import { catchError, finalize, of, switchMap } from 'rxjs';

import { DuplicatePhone } from '@/modules/crm/customer/form/duplicate-phone/duplicate-phone';
import { CustomerModel } from '@/core/models/crm/customer.model';
import {
  authUser,
  branchActiveId,
  branchListId,
  isDepartmentBOD,
  isDepartmentConvert,
  isDepartmentCustomerCare,
  isDepartmentTelesale,
  isMarketing,
  isSaleStaff,
  userId,
} from '@/core/services/auth';
import { LocationWardService } from '@/core/services/config/location/location-ward.service';
import { CustomerService } from '@/core/services/crm/customer.service';
import { BaseSignalForm } from '@/share/base/base-signal-form';
import { BaseApiService } from '@/share/services/base-api.service';
import { UtilsService } from '@/share/services/utils.service';
import {
  CUSTOMER_CONNECT_OPTION,
  GENDER_OPTIONS,
} from '@/define';

import { FormShareImports } from '@/share/import/form-share-imports';
import { translateOptions } from '@/share/utils/translate-options.util';
import { CustomerCategoryAutocompleteFormControl } from '@/indexeddb/config/components/customer-category';
import { ControlFieldInTableCacheService } from '@/indexeddb/config/components/control-field-in-table';
import { JobAutocompleteFormControl } from '@/indexeddb/config/components/job';
import { EthnicityAutocompleteFormControl } from '@/indexeddb/config/components/ethnicity';

/**
 * Model của FORM (khác `CustomerModel` thuần): thêm các field chỉ tồn tại trên form gốc
 * (`customer-form.ts` `buildForm()`) chứ không có trong `CustomerModel` — field lịch hẹn nhanh,
 * ghi chú khách khi tạo.
 */
export interface CustomerFormModel extends CustomerModel {
  appointment_type_id?: number | null;
  content?: string | null;
  noteCustomer?: string | null;
}

/**
 * Danh sách field gửi lên BE — đúng bằng danh sách control của `buildForm()` gốc
 * (`customer-form.ts` dòng 556-633), TRỪ các field đã gỡ khỏi form (2026-10-18): `marketing_id`
 * (BE drop cột), tab "Thông tin bổ sung" (`emr_family_history`) và tab "Người giám hộ"
 * (`relatives` + field lẻ `codeCustomer/nameRelation/relation/phoneRelation/noteRelation`).
 * KHÔNG gửi các key đó là CỐ Ý: BE `PUT /crm/customer/:id` update theo map key nhận được
 * (`optlock.Updates(..., filtered)`) ⇒ key vắng mặt thì cột giữ nguyên — dữ liệu giám hộ/tiền sử
 * của khách cũ KHÔNG bị ghi đè rỗng. Đừng thêm lại các key này với giá trị `null`.
 *
 * Phải liệt kê tường minh vì `extractValue(rootField)` trả về TOÀN BỘ object model (kể cả field
 * BE trả về kèm như `code`, `notes`, `appointments`, `*Label`...), khác `rfModel.value` của
 * Reactive Forms vốn chỉ chứa các control đã khai.
 */
const PAYLOAD_FIELDS = [
  'fullname', 'nickname', 'phone', 'other_phone', 'gender', 'category', 'birthday', 'source_id',
  'job_id', 'ethnicity_id', 'branch_id', 'address', 'address_other', 'type_service_id',
  'country_id', 'province_id', 'ward_id', 'facebook', 'loai_message',
  'chat_message', 'instagram', 'zalo', 'email', 'viber',
  'payment_user_id', 'old_code', 'created_day',
  'connect_id',
  'appointment_date', 'appointment_type_id',
  'content', 'updated_at',
  'emr_id_card_number',
  'emr_permanent_address', 'emr_house_number', 'noteCustomer',
] as const;

/**
 * Field có thể bị `control_field_in_table` khống chế required/disable (AC3).
 * LẤY TỪ DỮ LIỆU THẬT: `SELECT control FROM control_field_in_table` (id=1 "Bảng khách hàng",
 * id=2 "Bảng khách hàng (Lead)") — hợp của 2 bản ghi, BỎ `link_message` vì form khách hàng không có
 * control đó (bản gốc cũng bỏ qua: `if (!formControl) return`).
 * Field lạ (deploy khác cấu hình thêm field mới) → không có trong danh sách này thì không áp rule,
 * đúng hành vi "không rule = không required/không disable" của bản gốc khi control không tồn tại.
 */
const DYNAMIC_VALIDATED_FIELDS = [
  'fullname', 'nickname', 'gender', 'phone', 'category', 'type_service_id',
  'branch_id', 'source_id', 'payment_user_id',
  'birthday', 'facebook', 'zalo', 'viber', 'email',
  'instagram', 'chat_message', 'loai_message', 'country_id', 'province_id', 'ward_id', 'address',
  'created_day',
] as const;

/**
 * Signal Forms cần KEY tồn tại thật + giá trị KHÁC `undefined` để dựng sub-field
 * (`skill/frontend/angular22.md` gotcha #1) — mọi field template/buildSchema đụng tới phải seed
 * `null` (scalar) hoặc `[]` (mảng). BE (Go `omitempty`) không trả key cho field rỗng nên bắt buộc
 * merge qua template này trước khi `model.set()`.
 *
 * NGOẠI LỆ — field text bind `<input pInputText type="text">` (KHÔNG phải `<textarea>`/`p-inputnumber`/
 * `p-select`/custom control) PHẢI seed `''` (chuỗi rỗng), KHÔNG `null` (gotcha #9 `angular22.md`):
 * Signal Forms native-control adapter coi field `<input type="text">` có giá trị HIỆN TẠI là `null` là
 * field SỐ, ép `Number(text)` mỗi lần gõ — gõ chữ luôn lỗi `parse`, gõ số mất số 0 đầu. `''` giữ đúng
 * `typeof === 'string'` nên tránh nhánh đó. `required()` vẫn coi `''` là invalid, không đổi ngữ nghĩa.
 */
const TEXT_INPUT_FIELDS = [
  'fullname', 'phone', 'nickname', 'other_phone', 'facebook', 'loai_message', 'chat_message',
  'instagram', 'zalo', 'email', 'viber', 'old_code',
  'emr_id_card_number',
  'emr_house_number',
] as const;

function emptyCustomerModel(): CustomerFormModel {
  return {
    // Text field bind `<input pInputText>` — seed '' (gotcha #9), KHÔNG null.
    fullname: '', phone: '', nickname: '', other_phone: '', facebook: '', loai_message: '',
    chat_message: '', instagram: '', zalo: '', email: '', viber: '',
    old_code: '',
    emr_id_card_number: '', emr_house_number: '',
    // Field khác (số/reference/textarea/custom control) — giữ null như cũ, không bị gotcha #9.
    gender: null, category: null,
    birthday: null, source_id: null, job_id: null, ethnicity_id: null, branch_id: null,
    address: null, address_other: null, type_service_id: null, country_id: null,
    province_id: null, ward_id: null,
    payment_user_id: null, created_day: null,
    connect_id: null,
    appointment_date: null,
    appointment_type_id: null,
    content: null, updated_at: null,
    emr_permanent_address: null,
    noteCustomer: null,
  } as unknown as CustomerFormModel;
}

/**
 * Đăng ký 1 `effect()` CHỈ phản ứng khi giá trị THAY ĐỔI so với lần đọc trước — bỏ qua lần chạy
 * đầu (Angular `effect()` luôn chạy ngay lúc tạo, khác `valueChanges.subscribe()` chỉ bắn khi có
 * thay đổi thật). Không skip lần đầu thì cascade-reset (country/province) sẽ xoá dữ liệu vừa load
 * ở edit mode. Copy nguyên từ `service-form.ts` (quyết định "copy riêng" đã chốt ở spike).
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

@Component({
  selector: 'app-customer-edit-form-v2',
  templateUrl: './customer-form-v2.html',
  styleUrls: ['../form.scss'],
  standalone: true,
  imports: [
    ...FormShareImports,
    FormField,
    CustomerCategoryAutocompleteFormControl,
    JobAutocompleteFormControl,
    EthnicityAutocompleteFormControl,
  ],
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class CustomerFormV2 extends BaseSignalForm<CustomerFormModel> {
  modelService = inject(CustomerService) as unknown as CustomerService & BaseApiService<CustomerFormModel>;
  wardService = inject(LocationWardService);
  maxDate!: Date;
  selectedDateBirthday!: any;
  selectedDateCreatedDay!: any;
  customer!: CustomerFormModel;
  readonly blockSpace: RegExp = /[^\s]/;
  readonly locationFindWard = signal<{ value: number; label: string }[]>([]);
  readonly currentCountryId = signal<number | undefined>(undefined);
  readonly canEditBranch = signal(true);
  readonly CUSTOMIZE_FORM_ID = 1;
  readonly CUSTOMIZE_FORM_LEAD_ID = 2;
  readonly activeTab = signal<string | number>('0');
  protected readonly genderOptions = translateOptions(GENDER_OPTIONS);
  protected readonly CUSTOMER_CONNECT_OPTION = CUSTOMER_CONNECT_OPTION;
  private utilsService = inject(UtilsService);
  private controlFieldInTableCache = inject(ControlFieldInTableCacheService);
  /** Rule động từ `control_field_in_table` — `{}` cho tới khi cache load xong (không block render). */
  private readonly dynamicFieldRules =
    signal<Record<string, { create?: boolean; update?: boolean; edit?: boolean }>>({});
  private readonly wardCache = new Map<number, { value: number; label: string }[]>();

  constructor() {
    super();
    this.customer = this.dialogConfig?.data?.customer;
    this.maxDate = this.timezoneService.todayEnd();

    // AC9 — hook có sẵn ở BaseSignalForm, chỉ gọi (không tự viết lại). Truyền TÊN field (không phải
    // rootField.branch_id trực tiếp) — model chưa seed lúc constructor chạy (seed ở ngOnInit qua
    // _applyModel()), rootField.branch_id lúc này là undefined (gotcha #1); trackScopeFields() tự
    // tra field bên trong effect() nên an toàn với thứ tự này.
    this.trackScopeFields('branch_id');

    // AC5 — country đổi thì reset province + ward (đối chiếu dòng 688-697 gốc).
    effectOnChange(
      () => this.rootField.country_id!().value(),
      (countryId) => {
        const prevCountryId = this.currentCountryId();
        this.currentCountryId.set(countryId ?? undefined);
        if (prevCountryId !== null && prevCountryId !== countryId) {
          untracked(() => {
            this.rootField.province_id!().value.set(null as any);
            this.rootField.ward_id!().value.set(null as any);
          });
        }
      },
    );

    // AC5 — province đổi thì load ward, xoá province thì clear danh sách + ward (dòng 700-709 gốc).
    effectOnChange(
      () => this.rootField.province_id!().value(),
      (provinceId) => {
        if (provinceId) {
          this.loadWard(provinceId);
        } else {
          this.locationFindWard.set([]);
          untracked(() => this.rootField.ward_id!().value.set(null as any));
        }
      },
    );
  }

  /**
   * Cờ lỗi trên đầu mỗi tab (3 tab, 2026-10-18): `info` (tab 0), `contactExtended` (tab 1 — gộp
   * Liên hệ + Mở rộng cũ, kể cả các field chỉ có maxLength như CCCD/số nhà/địa chỉ thường trú),
   * `appointment` (tab 2, chỉ create).
   */
  readonly tabErrors = computed(() => ({
    info:
      this.isFieldInvalid(this.rootField.fullname!) ||
      this.isFieldInvalid(this.rootField.nickname!) ||
      this.isFieldInvalid(this.rootField.phone!) ||
      this.isFieldInvalid(this.rootField.source_id!) ||
      this.isFieldInvalid(this.rootField.payment_user_id!) ||
      this.isFieldInvalid(this.rootField.branch_id!) ||
      this.isFieldInvalid(this.rootField.birthday!) ||
      this.isFieldInvalid(this.rootField.gender!) ||
      this.isFieldInvalid(this.rootField.category!) ||
      this.isFieldInvalid(this.rootField.type_service_id!) ||
      this.isFieldInvalid(this.rootField.email!) ||
      this.isFieldInvalid(this.rootField.country_id!) ||
      this.isFieldInvalid(this.rootField.province_id!) ||
      this.isFieldInvalid(this.rootField.ward_id!) ||
      this.isFieldInvalid(this.rootField.emr_house_number!) ||
      this.isFieldInvalid(this.rootField.address!),
    contactExtended:
      this.isFieldInvalid(this.rootField.old_code!) ||
      this.isFieldInvalid(this.rootField.emr_id_card_number!) ||
      this.isFieldInvalid(this.rootField.created_day!) ||
      this.isFieldInvalid(this.rootField.loai_message!) ||
      this.isFieldInvalid(this.rootField.chat_message!) ||
      this.isFieldInvalid(this.rootField.job_id!) ||
      this.isFieldInvalid(this.rootField.ethnicity_id!) ||
      this.isFieldInvalid(this.rootField.connect_id!) ||
      this.isFieldInvalid(this.rootField.emr_permanent_address!) ||
      this.isFieldInvalid(this.rootField.facebook!) ||
      this.isFieldInvalid(this.rootField.instagram!) ||
      this.isFieldInvalid(this.rootField.other_phone!) ||
      this.isFieldInvalid(this.rootField.zalo!) ||
      this.isFieldInvalid(this.rootField.viber!) ||
      this.isFieldInvalid(this.rootField.address_other!),
    appointment:
      this.isFieldInvalid(this.rootField.appointment_date!) ||
      this.isFieldInvalid(this.rootField.content!),
  }));

  override ngOnInit(): void {
    if (this.customer) {
      this._applyModel(this.customer);
    } else {
      const activeBranch = branchActiveId();
      const initialValue: any = {
        branch_id: activeBranch && branchListId().includes(activeBranch) ? activeBranch : null,
        old_code: null,
        country_id: this.modelService.VIETNAM,
        phone: this.dialogConfig?.data?.phone || null,
      };

      if (isDepartmentTelesale()) {
        initialValue.payment_user_id = userId();
      }

      const obj: any = { ...this.modelService.getInitialObject(), ...initialValue };

      this._applyModel(obj);
    }

    void this.loadDynamicFieldRules();
  }

  /**
   * Chuẩn hoá model trước khi `model.set()` — merge template đủ key (gotcha #1), reset các field
   * bản gốc luôn khởi tạo `null` (lịch hẹn nhanh, ghi chú).
   */
  private _applyModel(model: CustomerFormModel): void {
    const merged: any = {
      ...emptyCustomerModel(),
      ...model,
      // Bản gốc khai các control này bằng `null` cứng, KHÔNG lấy từ model (dòng 598-609, 632).
      appointment_date: null,
      appointment_type_id: null,
      content: null,
      noteCustomer: null,
    };

    // Lưới an toàn gotcha #9: `...model` (từ BE) có thể ghi đè seed
    // `''` bằng `null`/`undefined` cho field `<input pInputText>` → field lại bị coi là field SỐ.
    // Chuẩn hoá 1 chỗ ở đây thay vì rải `?? ''` khắp nơi.
    for (const key of TEXT_INPUT_FIELDS) {
      if (merged[key] == null) merged[key] = '';
    }

    this.currentCountryId.set(model.country_id || undefined);
    this.canEditBranch.set(this.isEditableOnUpdate(model.id, model.branch_id));

    this.model.set(merged as CustomerFormModel);

    if (merged.province_id) {
      this.loadWard(merged.province_id);
    }
  }

  protected override buildSchema(p: SchemaPathTree<CustomerFormModel>): void {
    // Email — dùng đúng luật của `customValidationService.email()` (regex `UtilsService.isEmail`);
    // service inject được đọc trong closure LogicFn nên an toàn với ràng buộc của BaseSignalForm.
    validate(p.email! as SchemaPath<string>, ({ value }) => {
      const v = value();
      return !v || this.utilsService.isEmail(v) ? null : emailError();
    });

    // story-43 (E6 bỏ company): `connect_id` required theo `convertIdAtLoad` (customer.convert_id,
    // D26) đã bỏ — tính năng "Convert" gắn omnichat đã bỏ (D8), không còn nguồn để chốt điều kiện này.
    // COMPANY_DEPENDENT_FIELDS (source_id/type_service_id khoá tới khi chọn company) cũng bỏ — không
    // còn khái niệm company để khoá theo.

    // AC8 — branch_id khoá khi user không có quyền trên branch của bản ghi (dòng 672-676 gốc).
    disabled(p.branch_id!, { when: () => !this.canEditBranch() });

    // AC3 — rule động từ `control_field_in_table`. Đăng ký 1 LẦN; LogicFn đọc `dynamicFieldRules()`
    // trong closure nên tự re-evaluate khi cache load xong (không cần rebuild schema).
    for (const fieldName of DYNAMIC_VALIDATED_FIELDS) {
      const path = (p as any)[fieldName];
      if (!path) continue;
      disabled(path, {
        when: () =>
          this.isEditMode() &&
          this.dynamicFieldRules()[fieldName]?.edit === true &&
          !isDepartmentBOD(),
      });
      required(path, {
        when: () => {
          const rule = this.dynamicFieldRules()[fieldName];
          if (!rule) return false;
          // Edit mode: `edit` (đã disable ở trên) chặn hẳn nhánh `update` — trừ BOD, vốn không bị
          // disable nên vẫn phải chịu required của `update` (đối chiếu `handleEditMode()` gốc).
          if (this.isEditMode()) {
            return (!rule.edit || isDepartmentBOD()) && !!rule.update;
          }
          return !!rule.create;
        },
      });
    }

    // `[formField]` cấm gán thẳng attribute `maxlength` trên node (NG8022) — phải khai qua schema để
    // directive tự quản lý, khớp `maxlength` HTML gốc từng field.
    maxLength(p.emr_id_card_number! as SchemaPath<string>, 20);
    maxLength(p.emr_house_number! as SchemaPath<string>, 100);
    maxLength(p.address! as SchemaPath<string>, 500);
    maxLength(p.emr_permanent_address! as SchemaPath<string>, 500);
  }

  /** AC7 — loại 2 field UI-only + chỉ gửi đúng danh sách control của form gốc. */
  protected override getPayload(): any {
    const raw = super.getPayload() as any;
    const payload: any = {};
    for (const key of PAYLOAD_FIELDS) payload[key] = raw[key];
    // Field text seed `''` (gotcha #9) — trả về `null` khi rỗng để payload GIỐNG HỆT bản Reactive
    // Forms gốc (`rfModel.value` trả `null` cho control text chưa nhập). Quan trọng với `phone`:
    // `onSubmit()` dựa vào `payload.phone == null` để quyết định có gọi API check trùng SĐT hay không.
    for (const key of TEXT_INPUT_FIELDS) {
      if (payload[key] === '') payload[key] = null;
    }
    return payload;
  }

  /** Xử lý format data date khi nhập tay vào calendar ngày sinh */
  inputDateBirthday(value: any) {
    this.selectedDateBirthday = value;
  }

  /** Khi nhấn phím enter thì ngày sinh sẽ đc fill vào calendar ngày sinh */
  handleKeyPressBirthday(_event: KeyboardEvent) {
    const date = this.parseTypedDate(this.selectedDateBirthday);
    if (!date) {
      this.noti.warnMsg(this.translate.instant('crm.customer.invalid_birthday'));
      return;
    }
    this.rootField.birthday!().value.set(date as any);
  }

  /** Xử lý format data date khi nhập tay vào calendar ngày thu thập */
  inputDateCreatedDay(value: any) {
    this.selectedDateCreatedDay = value;
  }

  /** Khi nhấn phím enter thì ngày thu thập sẽ đc fill vào calendar */
  handleKeyPressCreatedDay(_event: KeyboardEvent) {
    const date = this.parseTypedDate(this.selectedDateCreatedDay);
    if (!date) {
      this.noti.warnMsg(this.translate.instant('crm.customer.invalid_collected_date'));
      return;
    }
    this.rootField.created_day!().value.set(date as any);
  }

  /**
   * Lấy ra danh sách phường xã theo tỉnh/thành để hiển thị ở dropdown
   * @param provinceId
   */
  loadWard(provinceId: any) {
    if (!provinceId) return;
    const cached = this.wardCache.get(+provinceId);
    if (cached) {
      this.locationFindWard.set(cached);
      return;
    }
    this.wardService
      .getProvince(provinceId)
      .pipe(takeUntilDestroyed(this.destroyRef))
      .subscribe((res: any) => {
        this.wardCache.set(+provinceId, res.data);
        this.locationFindWard.set(res.data);
      });
  }

  override onSubmit(): void {
    if (this.isSubmitting()) return;
    this.submitted.set(true);
    this.rootField().markAsTouched();

    if (this.rootField().invalid()) return;

    this.isSubmitting.set(true);

    const payload = this.getPayload();

    if (this.isEditMode()) {
      this.modelService
        .updateCustomer(this.model().id!, payload, {})
        .pipe(finalize(() => this.isSubmitting.set(false)), catchError(() => of(null)))
        .subscribe({
          next: (res: any) => {
            if (res) {
              this.noti.successRes(res);
              requestAnimationFrame(() => this.dialogRef.close(res.data));
            }
          },
        });
      return;
    }

    if (payload.phone == null) {
      this.checkOldCodeDuplicateThenCreate(payload);
      return;
    }

    // kiểm tra số điện thoại bị trùng hay không
    this.modelService
      .getNumberCustomerByPhone(payload.phone, {})
      .pipe(
        switchMap(async (response: any) => {
          if (response.data) {
            const items = response.data ?? [];
            response.data = await this.modelService.addLabels(items);
          }
          return response;
        }),
        finalize(() => {
          this.isSubmitting.set(false);
        }),
        catchError(() => of(null)),
      )
      .subscribe({
        next: (res: any) => {
          if (!res) return;
          if (res.num > 0) {
            // D-3: SĐT là duy nhất trong 1 DB (BE chặn bằng UNIQUE) ⇒ trùng SĐT là CHẶN: popup chỉ để xem
            // khách đang giữ số này, không còn lựa chọn "Lưu trùng".
            this.openDuplicateDialog(res.data, true);
          } else {
            this.checkOldCodeDuplicateThenCreate(payload);
          }
        },
      });
  }

  /**
   * Hàm tạo mới khách hàng
   * @param params
   */
  createCustomer(params: any) {
    this.modelService
      .createCustomer(params, {})
      .pipe(
        finalize(() => {
          this.isSubmitting.set(false);
        }),
        catchError(() => of(null)),
      )
      .subscribe({
        next: (res: any) => {
          if (res) {
            this.noti.successMsg(res.message ?? '');
            requestAnimationFrame(() => this.dialogRef.close(res?.data));
          }
        },
      });
  }

  /**
   * Kiểm tra trùng mã KH cũ trước khi tạo mới — chỉ cảnh báo, không chặn.
   * @param params
   */
  private checkOldCodeDuplicateThenCreate(params: any) {
    const oldCode = params.old_code;
    if (oldCode == null || oldCode === '') {
      this.createCustomer(params);
      return;
    }
    this.modelService
      .getNumberCustomerByOldCode(oldCode, {})
      .pipe(
        switchMap(async (response: any) => {
          if (response.data) {
            const items = response.data ?? [];
            response.data = await this.modelService.addLabels(items);
          }
          return response;
        }),
        finalize(() => {
          this.isSubmitting.set(false);
        }),
        catchError(() => of(null)),
      )
      .subscribe({
        next: (res: any) => {
          if (!res || !(res.num > 0)) {
            this.createCustomer(params);
            return;
          }
          // nếu như bị trùng mã khách hàng cũ thì show popup cảnh báo, không chặn tạo mới
          const dialogRef = this.openDuplicateDialog(res.data);
          dialogRef.onClose
            .pipe(takeUntilDestroyed(this.destroyRef))
            .subscribe((isConfirmCreate: any) => {
              if (isConfirmCreate) {
                this.createCustomer(params);
              }
            });
        },
      });
  }

  private openDuplicateDialog(duplicateModels: any, blocking = false) {
    return this.dialogWrapperService.open(DuplicatePhone, {
      width: '840px',
      autoZIndex: true,
      showHeader: true,
      closeOnEscape: false,
      contentStyle: { overflow: 'auto', padding: '0' },
      header: this.translate.instant('common.label.possible_duplicate_customer'),
      data: {
        duplicateModels,
        blocking,
      },
    });
  }

  /**
   * Load rule động async (không block render) — `dynamicFieldRules` mặc định `{}` nên field hiển thị
   * trạng thái "không có rule" trong lúc chờ, tự cập nhật ngay khi cache load xong.
   */
  private async loadDynamicFieldRules(): Promise<void> {
    await this.controlFieldInTableCache.ensureControlFieldInTableLoaded();
    const departmentId =
      isSaleStaff() ||
      isDepartmentTelesale() ||
      isDepartmentConvert() ||
      isDepartmentCustomerCare() ||
      isMarketing()
        ? this.CUSTOMIZE_FORM_LEAD_ID
        : this.CUSTOMIZE_FORM_ID;

    const raw = (this.controlFieldInTableCache.getById(departmentId) as any)?.control;
    if (!raw) return;

    let controls: any;
    try {
      controls = typeof raw === 'string' ? JSON.parse(raw) : raw;
    } catch {
      return;
    }
    // DB có thể lưu {"control": [...]} hoặc [...] trực tiếp
    if (!Array.isArray(controls) && Array.isArray(controls?.control)) {
      controls = controls.control;
    }
    if (!Array.isArray(controls)) return;

    this.dynamicFieldRules.set(
      Object.fromEntries(controls.filter((c: any) => c?.field_name).map((c: any) => [c.field_name, c])),
    );
  }

  /** Copy `BaseForm.isEditableOnUpdate()` (BaseSignalForm không kế thừa BaseForm). */
  private isEditableOnUpdate(modelId: number | undefined, branchId: any): boolean {
    // Create: luôn cho chọn
    if (!modelId) return true;
    // Update: nếu chưa có branch => vẫn cho chọn
    if (branchId == null || branchId === '') return true;
    const group = (authUser()?.branchId ?? []).map((x: any) => +x);
    return group.includes(+branchId);
  }

  /** Copy `BaseForm.handleSelectedDate()`/`formatDate()` — trả Date, không đụng Reactive Forms. */
  private parseTypedDate(typed: any): Date | null {
    if (!typed) return null;
    let value = String(typed);
    if (!value.includes('/')) {
      value = `${value.slice(0, 2)}/${value.slice(2, 4)}/${value.slice(4, 8)}`;
    }
    const [day, month, year] = value.split('/');
    if (year && +month <= 12 && +day <= 31) {
      return new Date(+year, +month - 1, +day);
    }
    return null;
  }
}
