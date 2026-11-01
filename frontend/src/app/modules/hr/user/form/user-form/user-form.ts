import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { finalize } from 'rxjs';
import {
  FormField,
  required,
  minLength,
  maxLength,
  validate,
  requiredError,
  SchemaPathTree,
} from '@angular/forms/signals';
import { extractValue } from '@angular/forms/signals/compat';

import { UserModel } from '@/core/models/hr/user.model';
import { UserService } from '@/core/services/hr/user.service';
import { BaseSignalForm } from '@/share/base/base-signal-form';
import { FormShareImports } from '@/share/import/form-share-imports';
import { DEPARTMENT_SELECT, GENDER_OPTIONS, USER_STATUS_OPTIONS } from '@/define';
import { translateOptions } from '@/share/utils/translate-options.util';
import { DepartementMultiselectFromControl } from '@/indexeddb/config/components/hr-department';
import { authUser } from '@/core/services/auth/auth.state';
import { userId } from '@/core/services/auth/auth.selectors';
import { UtilsService } from '@/share/services/utils.service';

/**
 * Form tạo/sửa nhân viên — cross-module (user + user_profile + department assignment). Đặc thù:
 * - `password` chỉ tồn tại thật ở create-mode bản gốc (`addControl` động) — Signal Forms field LUÔN có
 *   mặt (seed `''`), `required({when: !isEditMode()})`; PHẢI tự xoá khỏi payload khi edit-mode (xem
 *   `getPayload()`) — nếu không sẽ gửi `password: ''` lên BE lúc update, rủi ro ghi đè mật khẩu thật.
 * - `userProfile` là property THẬT trên `UserModel` (không phải `user_profile` — bản gốc chỉ đặt TÊN
 *   FormGroup tuỳ ý là `user_profile`, không liên quan tên field model) — Signal Forms bind theo đúng
 *   tên field thật `userProfile`, `getPayload()` tự đổi key thành `user_profile` lúc gửi BE (khớp shape
 *   API cũ).
 * - `allowed_ips`: BE `ListUser` KHÔNG trả field này (chỉ `GetUser` có) — sửa qua list sẽ hiện rỗng, bug
 *   PRE-EXISTING giống `CompPlanForm` (xem `deferred-work.md`), KHÔNG tự sửa trong story này.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-user-form',
  templateUrl: './user-form.html',
  styles: [
    `
      /* Ngăn border đỏ cascade từ formGroupName div (ng-dirty.ng-invalid)
         xuống các input không có validator */
      :host ::ng-deep .ng-dirty.ng-invalid .p-inputtext:not(.ng-invalid),
      :host ::ng-deep .ng-dirty.ng-invalid .p-select:not(.ng-invalid) {
        border-color: var(--p-inputtext-border-color);
        box-shadow: none;
      }
    `,
  ],
  imports: [
    ...FormShareImports,
    FormField,
    DepartementMultiselectFromControl,
  ],
  standalone: true,
})
export class UserForm extends BaseSignalForm<UserModel> {
  modelService = inject(UserService);
  private utilsService = inject(UtilsService);
  readonly blockSpace: RegExp = /[^\s]/;

  childRole = signal<any[]>([]);
  protected readonly genderOptions = translateOptions(GENDER_OPTIONS);
  protected readonly userStatusOptions = translateOptions(USER_STATUS_OPTIONS);
  protected readonly DEPARTMENT_SELECT = DEPARTMENT_SELECT;
  protected readonly isSelf = computed(() => this.isEditMode() && this.model()?.id === userId());

  constructor() {
    super();
    const pound = this.dialogConfig?.data?.childRole;
    this.childRole.set(pound ?? []);
  }

  get deptList(): number[] {
    const v = this.rootField.userProfile!.departmentList!().value();
    if (Array.isArray(v)) return v.map((x) => (typeof x === 'string' ? +x : x));
    if (v == null) return [];
    return [typeof v === 'string' ? +(v as any) : (v as any)];
  }

  /** "role" flat field (identity BE) seed từ `userRole.name` (nested relation, chỉ GET trả về) — mirror
   * bản gốc `buildForm()`: `role: [model.userRole?.name, ...]`. Chạy 1 lần sau khi model load. */
  override ngOnInit(): void {
    super.ngOnInit();
    if (this.isEditMode()) {
      const roleName = this.model()?.userRole?.name;
      if (roleName) this.model.update((m) => ({ ...m, role: roleName }));
    }
  }

  get allowedIpsText(): string {
    return this.ipsToText(this.model().allowed_ips);
  }

  onAllowedIpsInput(event: Event): void {
    const text = (event.target as HTMLTextAreaElement).value;
    this.rootField.allowed_ips!().value.set(this.parseIps(text) as any);
  }

  generatePassword() {
    this.rootField.password!().value.set(this.utilsService.randomString());
  }

  override onSubmit(): void {
    if (this.isSubmitting()) return;
    this.submitted.set(true);
    this.rootField().markAsTouched();
    if (this.rootField().invalid()) return;

    this.isSubmitting.set(true);
    const modelId = this.model()?.id;
    const payload = this.getPayload();

    const saveOp = this.isEditMode()
      ? this.modelService.updateUser(modelId!, payload, {})
      : this.modelService.create(payload, {});

    saveOp.pipe(finalize(() => this.isSubmitting.set(false))).subscribe({
      next: (res: any) => {
        if (this.isEditMode() && modelId === userId()) {
          this.syncAuthUserIfSelf();
          sessionStorage.setItem('post_reload_toast', this.translate.instant('hr.user_list_page.update_personal_success'));
          location.reload();
          return;
        }
        this.noti.successRes(res);
        this.dialogRef.close(res?.data ?? res);
      },
      error: () => {},
    });
  }

  hasDept = (code: number) => this.deptList.includes(code);

  /** Khi admin sửa chính mình, đồng bộ branchId vào authUser signal + localStorage. */
  private syncAuthUserIfSelf() {
    if (this.model()?.id !== userId()) return;
    const current = authUser();
    if (!current) return;
    const formVal = this.model();
    const updated = {
      ...current,
      branchId: Array.isArray(formVal.branchId) ? [...formVal.branchId] : (current.branchId ?? []),
    };
    authUser.set(updated);
    this.storageService.saveStorage('auth_user', updated);
  }

  protected override buildSchema(p: SchemaPathTree<UserModel>): void {
    required(p.username!);
    minLength(p.username!, 5);
    maxLength(p.username!, 20);
    required(p.status!);
    required(p.role!);
    // password chỉ bắt buộc ở create-mode (bản gốc addControl động) — field LUÔN tồn tại trong
    // FieldTree (gotcha #1), chỉ đổi điều kiện required; PHẢI tự xoá khỏi payload khi edit (getPayload()).
    required(p.password!, { when: () => !this.isEditMode() });

    required(p.userProfile!.fullname!);
    required(p.userProfile!.phone!);
    minLength(p.userProfile!.phone!, 10);
    maxLength(p.userProfile!.phone!, 12);
    required(p.userProfile!.gender!);
    // Mảng bắt buộc chọn ≥1 — required() coi [] hợp lệ (gotcha #10), phải validate() thủ công.
    validate(p.userProfile!.departmentList!, ({ value }) => ((value()?.length ?? 0) === 0 ? requiredError() : null));
  }

  /** JSON array/string allowed_ips → text mỗi dòng 1 entry. */
  private ipsToText(v: any): string {
    if (Array.isArray(v)) return v.join('\n');
    if (typeof v === 'string' && v) {
      try {
        const a = JSON.parse(v);
        return Array.isArray(a) ? a.join('\n') : v;
      } catch {
        return v;
      }
    }
    return '';
  }

  /** Text (xuống dòng/phẩy) → mảng IP/CIDR đã trim, bỏ rỗng. */
  private parseIps(text: any): string[] {
    if (Array.isArray(text)) return text;
    if (typeof text !== 'string') return [];
    return text
      .split(/[\n,]/)
      .map((s) => s.trim())
      .filter(Boolean);
  }

  /** `userProfile` (tên field THẬT trên model) → đổi key `user_profile` lúc gửi BE (khớp shape API cũ,
   * xem comment class). `password` PHẢI xoá khỏi payload khi edit-mode — field luôn tồn tại trong
   * FieldTree (gotcha #1) nhưng BE update không được nhận `password: ''` (rủi ro xoá mật khẩu thật).
   *
   * QA finding pass 1 (patch, high): `model().userProfile` sau khi merge với data GET/list thật chứa
   * ĐẦY ĐỦ ~20 field BE trả về (`email/birthday/avatar/cover/locale/facebook/telegram/
   * branchOffice_id/branchActive_id/companyActive_id/companyActive/branch...`), KHÔNG chỉ 8 field bản
   * gốc Reactive Forms từng khai trong `FormGroup('user_profile')`. Nếu spread nguyên `userProfile` vào
   * payload, BE (`UpdateUserInput.UserProfile` → map-based `Updates()`, xem `service.go` dòng 188-208)
   * sẽ GHI ĐÈ 12 cột SCALAR thật (email/birthday/about/address/avatar/cover/locale/facebook/telegram/
   * branchOffice_id/branchActive_id/companyActive_id) bằng giá trị round-trip CŨ — race condition/
   * lost-update nếu field nào trong số đó bị đổi ở nơi khác (vd tự đổi avatar) giữa lúc mở dialog và
   * lúc Lưu. Phải dựng `user_profile` THỦ CÔNG, CHỈ ĐÚNG các field form sở hữu — KHÔNG spread toàn bộ.
   * `nickname`/`code`/`id_pancake` đã bỏ khỏi form ⇒ không gửi nữa: BE chỉ ghi key có mặt
   * (`hrProfileEditableFields`), giá trị đã lưu giữ nguyên. */
  protected override getPayload(): any {
    const raw = extractValue(this.rootField) as any;
    const { userProfile, ...rest } = raw;
    const up = userProfile ?? {};
    const payload: any = {
      ...rest,
      user_profile: {
        fullname: up.fullname,
        phone: up.phone,
        gender: up.gender,
        departmentList: up.departmentList,
        updated_at: up.updated_at,
      },
    };
    if (this.isEditMode()) delete payload.password;
    return payload;
  }
}
