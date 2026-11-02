import {
  ChangeDetectionStrategy,
  Component,
  computed,
  EventEmitter,
  forwardRef,
  inject,
  input,
  OnInit,
  Output,
  signal,
} from '@angular/core';
import { ControlValueAccessor, FormsModule, NG_VALUE_ACCESSOR } from '@angular/forms';
import { TranslatePipe } from '@ngx-translate/core';
import { MultiSelect } from 'primeng/multiselect';

import { CUSTOMER_STAGE_FILTER_MIN_OPTIONS, customerStageColorClass } from '@/define';
import { CustomerStageCacheService } from './customer-stage-cache.service';
import { buildCustomerStageOptions, CustomerStageOption } from './customer-stage-options.util';

/**
 * Chọn NHIỀU bước — dùng cho bộ lọc (list khách, lịch hẹn, báo cáo phễu). Mặc định liệt kê MỌI bước kể
 * cả ẩn (bước ẩn đánh dấu "đã ẩn") vì bản ghi cũ vẫn có thể đang ở bước đã ẩn. Giá trị là mảng `stage_id`.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-customer-stage-multiselect-form-control',
  standalone: true,
  imports: [FormsModule, MultiSelect, TranslatePipe],
  providers: [
    {
      provide: NG_VALUE_ACCESSOR,
      useExisting: forwardRef(() => CustomerStageMultiselectFormControl),
      multi: true,
    },
  ],
  template: `
    <p-multiselect
      [options]="options()"
      [ngModel]="selectedIds()"
      (ngModelChange)="onSelectionChange($event)"
      (onBlur)="onTouched()"
      (onClear)="onClear()"
      [disabled]="disabled()"
      [filter]="options().length >= filterMinOptions"
      [showClear]="true"
      [maxSelectedLabels]="2"
      [placeholder]="placeholder()"
      optionLabel="name"
      optionValue="id"
      appendTo="body"
      class="w-full"
    >
      <ng-template #item let-opt>
        <span class="inline-flex items-center gap-2">
          <span class="inline-block w-2.5 h-2.5 rounded-full" [class]="dotClass(opt)"></span>
          <span>{{ opt.name }}</span>
          @if (opt.hidden) {
            <span class="text-xs text-muted-color">({{ 'config.customer_stage.hidden_label' | translate }})</span>
          }
        </span>
      </ng-template>
    </p-multiselect>
  `,
})
export class CustomerStageMultiselectFormControl implements ControlValueAccessor, OnInit {
  /** false = như form chọn đơn: chỉ bước hiện + bước ẩn đang được chọn. Mặc định true (bộ lọc). */
  readonly includeHidden = input(true);
  readonly placeholder = input('');
  @Output() valueChange = new EventEmitter<number[]>();

  protected readonly selectedIds = signal<number[]>([]);
  protected readonly disabled = signal(false);
  private readonly cache = inject(CustomerStageCacheService);

  protected readonly options = computed<CustomerStageOption[]>(() =>
    buildCustomerStageOptions(this.cache.allUnfiltered(), {
      selectedIds: this.selectedIds(),
      includeHidden: this.includeHidden(),
    }),
  );

  protected readonly filterMinOptions = CUSTOMER_STAGE_FILTER_MIN_OPTIONS;

  protected dotClass(opt: CustomerStageOption): string {
    return customerStageColorClass(opt.color).dot;
  }

  async ngOnInit() {
    await this.cache.ensureCustomerStagesLoaded();
  }

  protected onSelectionChange(ids: number[] | null) {
    const next = Array.isArray(ids) ? ids : [];
    this.selectedIds.set(next);
    this.onChange(next);
    this.valueChange.emit(next);
  }

  protected onClear() {
    this.onSelectionChange([]);
  }

  writeValue(value: number[] | null): void {
    this.selectedIds.set(Array.isArray(value) ? value : []);
  }

  registerOnChange(fn: (value: number[]) => void): void {
    this.onChange = fn;
  }

  registerOnTouched(fn: () => void): void {
    this.onTouched = fn;
  }

  setDisabledState(isDisabled: boolean): void {
    this.disabled.set(isDisabled);
  }

  private onChange: (value: number[]) => void = () => {};

  protected onTouched: () => void = () => {};
}
