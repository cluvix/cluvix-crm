import { NgTemplateOutlet } from '@angular/common';
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
import { Select } from 'primeng/select';

import { CUSTOMER_STAGE_FILTER_MIN_OPTIONS, customerStageColorClass } from '@/define';
import { CustomerStageCacheService } from './customer-stage-cache.service';
import { buildCustomerStageOptions, CustomerStageOption } from './customer-stage-options.util';

/**
 * Chọn MỘT bước tiến trình khách (form tạo/sửa khách, đổi bước...). Liệt kê bước đang hiện theo
 * `sort_order`, CỘNG bước hiện tại của bản ghi nếu bước đó đã bị ẩn (hiện kèm nhãn "đã ẩn") để không
 * mất giá trị khi sửa. Giá trị là `stage_id` (number) hoặc null. Dùng được với `[formField]`,
 * `formControlName`, `[(ngModel)]`.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-customer-stage-select-form-control',
  standalone: true,
  imports: [FormsModule, NgTemplateOutlet, Select, TranslatePipe],
  providers: [
    {
      provide: NG_VALUE_ACCESSOR,
      useExisting: forwardRef(() => CustomerStageSelectFormControl),
      multi: true,
    },
  ],
  template: `
    <p-select
      [options]="options()"
      [ngModel]="value()"
      (ngModelChange)="onSelect($event)"
      (onBlur)="onTouched()"
      [disabled]="disabled()"
      [showClear]="showClear()"
      [filter]="options().length >= filterMinOptions"
      [placeholder]="placeholder()"
      optionLabel="name"
      optionValue="id"
      appendTo="body"
      class="w-full"
    >
      <ng-template #selectedItem let-opt>
        <ng-container *ngTemplateOutlet="row; context: { $implicit: opt }" />
      </ng-template>
      <ng-template #item let-opt>
        <ng-container *ngTemplateOutlet="row; context: { $implicit: opt }" />
      </ng-template>
    </p-select>

    <ng-template #row let-opt>
      <span class="inline-flex items-center gap-2">
        <span class="inline-block w-2.5 h-2.5 rounded-full" [class]="dotClass(opt)"></span>
        <span>{{ opt.name }}</span>
        @if (opt.hidden) {
          <span class="text-xs text-muted-color">({{ 'config.customer_stage.hidden_label' | translate }})</span>
        }
      </span>
    </ng-template>
  `,
})
export class CustomerStageSelectFormControl implements ControlValueAccessor, OnInit {
  readonly showClear = input(false);
  readonly placeholder = input('');
  @Output() valueChange = new EventEmitter<number | null>();

  protected readonly value = signal<number | null>(null);
  protected readonly disabled = signal(false);
  private readonly cache = inject(CustomerStageCacheService);

  protected readonly options = computed<CustomerStageOption[]>(() =>
    buildCustomerStageOptions(this.cache.allUnfiltered(), {
      selectedIds: this.value() ? [this.value() as number] : [],
    }),
  );

  protected readonly filterMinOptions = CUSTOMER_STAGE_FILTER_MIN_OPTIONS;

  protected dotClass(opt: CustomerStageOption): string {
    return customerStageColorClass(opt.color).dot;
  }

  async ngOnInit() {
    await this.cache.ensureCustomerStagesLoaded();
  }

  protected onSelect(id: number | null) {
    this.value.set(id ?? null);
    this.onChange(id ?? null);
    this.valueChange.emit(id ?? null);
  }

  writeValue(id: number | null): void {
    this.value.set(id ?? null);
  }

  registerOnChange(fn: (value: number | null) => void): void {
    this.onChange = fn;
  }

  registerOnTouched(fn: () => void): void {
    this.onTouched = fn;
  }

  setDisabledState(isDisabled: boolean): void {
    this.disabled.set(isDisabled);
  }

  private onChange: (value: number | null) => void = () => {};

  protected onTouched: () => void = () => {};
}
