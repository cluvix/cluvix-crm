import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormField, required, pattern, minLength, maxLength, disabled, SchemaPathTree } from '@angular/forms/signals';
import { finalize, take } from 'rxjs';

import { RoleModel } from '@/core/models/hr/role.model';
import { RbacService } from '@/core/services/auth/rbac.service';
import { BaseSignalForm } from '@/share/base/base-signal-form';
import { FormShareImports } from '@/share/import/form-share-imports';

const NAME_PATTERN = /^[a-z]+$/; // chỉ a–z

/**
 * Form tạo/sửa Role — ĐẶC BIỆT trong epic: identity thật là `name` (string), KHÔNG phải `id`
 * (`RoleModel.id` "khai cho có", không dùng thật). Nhận `name` qua `dialogConfig.data.name`
 * (KHÔNG qua `data.model`/`data.id` như mọi form khác), dùng API riêng `RbacService.getRole(name)`/
 * `createRole()`/`updateRole(name, ...)` — không theo khuôn CRUD `id`-based chuẩn của `BaseSignalForm`.
 * `ngOnInit()` override HOÀN TOÀN (không gọi `super.ngOnInit()`), giống cách `ServiceForm` tự làm.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-role-form',
  templateUrl: './role-form.html',
  styles: [],
  imports: [...FormShareImports, FormField],
  standalone: true,
})
export class RoleForm extends BaseSignalForm<RoleModel> {
  modelService = inject(RbacService);
  name!: string;

  constructor() {
    super();
    this.name = this.dialogConfig?.data?.name;
  }

  /**
   * Chuẩn hoá `name` (trim + lowercase) — CHỈ gọi lúc rời ô (`blur`) và lúc Lưu, KHÔNG theo từng phím.
   * Bản cũ ghi lại giá trị ngay trong lúc gõ (effect tự sửa) + chặn phím ngoài a–z ở `keypress`
   * (`alphabetOnly`): bộ gõ tiếng Việt (Unikey/EVKey/Telex) xoá-rồi-chèn ký tự có dấu, bị chặn/ghi đè
   * giữa chừng ⇒ lệch bộ đệm của bộ gõ, chữ bị lặp/rối. Ràng buộc chỉ a–z vẫn do `pattern()` báo lỗi.
   */
  normalizeName(): void {
    const state = this.rootField.name!();
    const val = state.value();
    if (val == null) return;
    const normalized = val.trim().toLowerCase();
    if (normalized !== val) state.value.set(normalized);
  }

  override ngOnInit(): void {
    if (this.name) {
      // Khởi tạo model ngay để body/footer hiển thị liền, không chờ API (mirror `initialForm()` gốc).
      this.model.set({ name: this.name, description: '' } as RoleModel);

      this.modelService
        .getRole(this.name)
        .pipe(take(1), takeUntilDestroyed(this.destroyRef))
        .subscribe((loaded: any) => {
          // Không đè description nếu user đã gõ trước khi API trả về (tránh race mất text) — mirror
          // check `.dirty` gốc, dùng field state `dirty()` của Signal Forms thay `FormControl.dirty`.
          const isDescDirty = this.rootField.description!().dirty();
          this.model.update((m) => ({
            ...m,
            name: loaded?.name ?? this.name,
            description: isDescDirty ? m.description : (loaded?.description ?? ''),
          }));
        });
    } else {
      const obj = { ...this.modelService.getInitialObject(), ...this.initialValue } as RoleModel;
      this.model.set(obj);
    }
  }

  override onSubmit(): void {
    if (this.isSubmitting()) return;
    this.normalizeName();
    this.submitted.set(true);
    this.rootField().markAsTouched();
    if (this.rootField().invalid()) return;

    this.isSubmitting.set(true);
    // extractValue() gồm cả field disabled (name khi edit) — tương đương `getRawValue()` bản gốc.
    const payload = this.getPayload();

    // ⚠ Dùng `this.name` (giá trị gốc lúc mở dialog, BẤT BIẾN) để phân biệt edit/create — KHÔNG dùng
    // `this.model().name` như bản gốc: `model` ở `BaseSignalForm` là nguồn sống 2 chiều cho `form()`
    // (field ghi thẳng vào `model`), nên `model().name` sẽ LUÔN truthy ngay khi user gõ tên ở CẢ 2 mode
    // — khác `BaseForm.model` (chỉ là snapshot tĩnh lúc `initialForm()`, không đổi theo `rfModel` sống).
    // Nếu giữ nguyên `this.model().name` sẽ vỡ nhánh create (luôn gọi nhầm `updateRole` sau khi gõ tên).
    if (this.name) {
      this.modelService
        .updateRole(this.name, payload)
        .pipe(finalize(() => this.isSubmitting.set(false)))
        .subscribe({ next: (res: any) => this.dialogRef.close(res), error: () => {} });
    } else {
      this.modelService
        .createRole(payload)
        .pipe(finalize(() => this.isSubmitting.set(false)))
        .subscribe({ next: (res: any) => this.dialogRef.close(res), error: () => {} });
    }
  }

  protected override buildSchema(p: SchemaPathTree<RoleModel>): void {
    required(p.name!);
    pattern(p.name!, NAME_PATTERN);
    minLength(p.name!, 2);
    maxLength(p.name!, 50);
    disabled(p.name!, { when: () => !!this.name });

    required(p.description!);
    maxLength(p.description!, 255);
  }
}
