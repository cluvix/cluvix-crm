import {
  ChangeDetectionStrategy,
  Component,
  computed,
  DestroyRef,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { TranslatePipe } from '@ngx-translate/core';
import { Popover } from 'primeng/popover';
import { TooltipModule } from 'primeng/tooltip';
import { finalize } from 'rxjs';

import { CustomerStageChangeResult } from '@/core/models/crm/customer.model';
import { CustomerService } from '@/core/services/crm/customer.service';
import { hasPermission } from '@/core/services/auth/auth.selectors';
import { PERMISSION_SELECT } from '@/define';
import {
  buildCustomerStageOptions,
  CustomerStageCacheService,
  CustomerStageHelperService,
} from '@/indexeddb/config/components/customer-stage';
import { shouldChangeStage } from './customer-stage-change.util';

/**
 * Badge tiến trình của khách ở đầu trang chi tiết. Có quyền sửa khách ⇒ bấm để chọn bước (chỉ bước đang
 * hiện + bước hiện tại, dùng chung `buildCustomerStageOptions` với form chọn bước của story-08) rồi gọi
 * `PUT /crm/customer/:id/stage`; không có quyền ⇒ chỉ hiển thị. Chỉ phát `changed` khi API thành công —
 * lỗi 422 (bước vừa bị ẩn ở tab khác) do interceptor hiện toast, badge giữ nguyên bước cũ.
 * Màn cha phải `await CustomerStageHelperService.ensureLoaded()` trước khi hiển thị (đã nằm trong
 * `CustomerService.ensureCacheLoaded`).
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-customer-stage-changer',
  standalone: true,
  imports: [Popover, TooltipModule, TranslatePipe],
  template: `
    @if (canChange()) {
      <button
        type="button"
        class="px-1.5 py-1 rounded font-semibold inline-flex items-center gap-1 text-sm cursor-pointer disabled:opacity-60 disabled:cursor-wait"
        [class]="badgeClass()"
        [disabled]="saving()"
        [pTooltip]="'crm.customer.progress_tooltip' | translate"
        tooltipPosition="top"
        [attr.aria-label]="'crm.customer.stage_change_aria' | translate"
        aria-haspopup="listbox"
        (click)="stagePopover.toggle($event)"
      >
        <i class="pi pi-flag"></i>
        <span>{{ stageName() }}</span>
        <i class="pi pi-angle-down text-xs"></i>
      </button>
      <p-popover #stagePopover>
        <div class="flex flex-col gap-1 min-w-48" role="listbox">
          @for (opt of options(); track opt.id) {
            <button
              type="button"
              role="option"
              class="flex items-center gap-2 px-2 py-1.5 rounded text-left cursor-pointer hover:bg-[var(--p-content-hover-background)]"
              [attr.aria-selected]="opt.id === stageId()"
              (click)="select(opt.id, stagePopover)"
            >
              <span class="inline-block w-2.5 h-2.5 rounded-full shrink-0" [class]="dotClass(opt.id)"></span>
              <span class="flex-1">{{ opt.name }}</span>
              @if (opt.hidden) {
                <span class="text-xs text-muted-color">({{ 'config.customer_stage.hidden_label' | translate }})</span>
              }
              @if (opt.id === stageId()) {
                <i class="pi pi-check text-xs"></i>
              }
            </button>
          }
        </div>
      </p-popover>
    } @else {
      <span
        class="px-1.5 py-1 rounded font-semibold inline-flex items-center gap-1 text-sm"
        [class]="badgeClass()"
        [pTooltip]="'crm.customer.progress_tooltip' | translate"
        tooltipPosition="top"
      >
        <i class="pi pi-flag"></i>
        <span>{{ stageName() }}</span>
      </span>
    }
  `,
})
export class CustomerStageChanger {
  readonly customerId = input.required<number>();
  readonly stageId = input<number | null | undefined>(null);
  /** Phát sau khi BE xác nhận đổi bước (`updated_at` mới nằm trong payload). */
  readonly changed = output<CustomerStageChangeResult>();

  protected readonly saving = signal(false);

  private readonly customerService = inject(CustomerService);
  private readonly cache = inject(CustomerStageCacheService);
  private readonly helper = inject(CustomerStageHelperService);
  private readonly destroyRef = inject(DestroyRef);

  protected readonly canChange = computed(() =>
    hasPermission(PERMISSION_SELECT.permissionUpdateCustomer),
  );
  protected readonly stageName = computed(() => this.helper.getName(this.stageId()));
  protected readonly badgeClass = computed(() => this.helper.getBadgeClass(this.stageId()));
  protected readonly options = computed(() => {
    const current = this.stageId();
    return buildCustomerStageOptions(this.cache.allUnfiltered(), {
      selectedIds: current ? [current] : [],
    });
  });

  protected dotClass(id: number): string {
    return this.helper.getDotClass(id);
  }

  protected select(id: number, popover: Popover): void {
    popover.hide();
    if (this.saving() || !shouldChangeStage(this.stageId(), id)) return;

    this.saving.set(true);
    this.customerService
      .changeStage(this.customerId(), id)
      .pipe(
        finalize(() => this.saving.set(false)),
        takeUntilDestroyed(this.destroyRef),
      )
      .subscribe({
        next: (res) => {
          if (res?.data) this.changed.emit(res.data);
        },
      });
  }
}
