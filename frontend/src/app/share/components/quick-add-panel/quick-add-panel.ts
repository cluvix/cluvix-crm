import { ChangeDetectionStrategy, Component, input } from '@angular/core';
import { TranslatePipe } from '@ngx-translate/core';

/**
 * Khung "nhập nhanh" phía trên bảng dòng (Thêm dịch vụ / Thêm thuốc): tiêu đề + gợi ý "Enter để thêm" + ruột do form
 * truyền vào (`ng-content`). Chỉ là vỏ trình bày — cơ chế Enter/reset dùng `QuickEntryController`.
 *
 *   <app-quick-add-panel titleKey="common.label.add_medicine"> …các ô… </app-quick-add-panel>
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-quick-add-panel',
  standalone: true,
  imports: [TranslatePipe],
  template: `
    <section class="mb-4 rounded-border border border-surface bg-[var(--p-content-hover-background)] p-3">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h3 class="m-0 flex items-center gap-2 text-sm font-semibold text-color">
          <i class="pi pi-plus-circle text-primary" aria-hidden="true"></i>
          <span>{{ titleKey() | translate }}</span>
        </h3>
        <span class="text-xs text-muted-color">{{ 'common.message.quick_add_enter_hint' | translate }}</span>
      </div>
      <ng-content />
    </section>
  `,
})
export class QuickAddPanel {
  /** i18n key của tiêu đề khung (vd `common.label.add_medicine`). */
  readonly titleKey = input.required<string>();
}
