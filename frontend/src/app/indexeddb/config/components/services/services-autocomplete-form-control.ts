import {
  ChangeDetectionStrategy,
  ChangeDetectorRef,
  Component,
  EventEmitter,
  forwardRef,
  inject,
  Input,
  OnChanges,
  OnInit,
  Output,
  signal,
  SimpleChanges,
  viewChild,
} from '@angular/core';
import { ControlValueAccessor, FormsModule, NG_VALUE_ACCESSOR } from '@angular/forms';

import { AutoComplete } from 'primeng/autocomplete';
import { ServicesCacheService } from './services-cache.service';
import { ServiceModel } from '@/core/models/config/clinic/service.model';
import { TranslateService, TranslatePipe } from '@ngx-translate/core';

@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-services-autocomplete-form-control',
  standalone: true,
  imports: [FormsModule, AutoComplete],
  providers: [
    {
      provide: NG_VALUE_ACCESSOR,
      useExisting: forwardRef(() => ServicesAutocompleteFormControl),
      multi: true,
    },
  ],
  template: `
    <p-autocomplete
      [(ngModel)]="selectedItem"
      [suggestions]="filteredItems()"
      (completeMethod)="filterItems($event)"
      (onSelect)="onSelect($event)"
      (onClear)="onClear()"
      (onBlur)="onBlur()"
      [dropdown]="true"
      [disabled]="disabled"
      [placeholder]="placeholder"
      [virtualScroll]="filteredItems().length > 20"
      [virtualScrollItemSize]="38"
      optionLabel="name"
      [showClear]="true"
      appendTo="body"
      [emptyMessage]="emptyMessage"
      [autoOptionFocus]="autoOptionFocus"
      class="w-full"
    />
  `,
})
export class ServicesAutocompleteFormControl implements ControlValueAccessor, OnInit, OnChanges {
  protected emptyMessage = '';
  private translate = inject(TranslateService);
  private cdr = inject(ChangeDetectorRef);
  @Input() serviceTypeId: number | null = null;
  @Input() status: number | null = null;
  @Output() valueChange = new EventEmitter<number | null>();
  @Output() selectService = new EventEmitter<{ value: any; index: number }>();
  @Input() index?: number;
  @Input() disabled: boolean = false;
  @Input() placeholder: string = '';
  /** Tự focus gợi ý đầu tiên khi danh sách mở ⇒ gõ tên rồi Enter là chọn luôn (ô nhập nhanh dịch vụ). Mặc định tắt. */
  @Input() autoOptionFocus: boolean = false;
  private readonly autoComplete = viewChild(AutoComplete);
  selectedItem: ServiceModel | null = null;
  filteredItems = signal<ServiceModel[]>([]);
  private cache = inject(ServicesCacheService);
  private cachedFilteredItems: ServiceModel[] = [];
  private lastFilterKey = '';
  private pendingId: number | null = null;
  private readonly MAX_RESULTS = 100;

  ngOnChanges(changes: SimpleChanges) {
    const hasRelevantChange =
      (changes['serviceTypeId'] && !changes['serviceTypeId'].firstChange) ||
      (changes['status'] && !changes['status'].firstChange);

    if (hasRelevantChange) {
      this.lastFilterKey = '';
      this.cachedFilteredItems = [];

      // Only clear selection if it doesn't match new filters
      if (this.selectedItem) {
        const newFilteredItems = this.getFilteredItems();
        if (!newFilteredItems.find((item) => item?.id === this.selectedItem!.id)) {
          this.selectedItem = null;
          this.onChange(null);
        }
      }
    }
  }

  async ngOnInit() {
    this.emptyMessage = this.translate.instant('common.label.no_results');
    await this.cache.ensureServicesLoaded();
    if (this.pendingId && !this.selectedItem) {
      this.selectedItem = this.cache.getById(this.pendingId) || null;
      this.cdr.markForCheck();
    }
  }

  filterItems(event: any) {
    const query = (event?.query ?? '').toString().trim().toLowerCase();
    let items = this.getFilteredItems();
    const safe = (v: any) => (v == null ? '' : String(v)).toLowerCase();

    // Filter theo query
    if (query && query.length >= 3) {
      // Tách query thành các từ (hỗ trợ search: "10b carbon" => ["10b", "carbon"])
      const keywords = query.split(/\s+/).filter(k => k.length > 0);

      items = items.filter((item) => {
        if (!item) return false;
        const name = safe(item.name);
        const code = safe(item.code);
        const searchText = `${name} ${code}`;

        // Tất cả keywords phải có trong searchText
        return keywords.every(keyword => searchText.includes(keyword));
      }).slice(0, this.MAX_RESULTS);
    } else if (query && query.length < 3) {
      items = items.slice(0, 50);
    }
    // No query: show all — virtualScroll handles rendering

    // Đảm bảo selectedItem luôn có trong list
    if (this.selectedItem && !items.find((item) => item?.id === this.selectedItem!.id)) {
      if (!query || query.length >= 3) {
        const keywords = query ? query.split(/\s+/).filter(k => k.length > 0) : [];
        const name = safe(this.selectedItem.name);
        const code = safe(this.selectedItem.code);
        const searchText = `${name} ${code}`;

        if (!query || keywords.every(keyword => searchText.includes(keyword))) {
          items = [this.selectedItem, ...items];
        }
      }
    }

    this.filteredItems.set(items);
  }

  onSelect(event: any) {
    const value = event.value || null;
    if (this.index !== undefined) {
      this.selectService.emit({ value, index: this.index });
    }
    const id = value?.id || null;
    this.valueChange.emit(id);
    this.onChange(id);
    this.onTouched();
  }

  /** Đưa con trỏ vào ô tìm — khung "Thêm dịch vụ" gọi sau khi thêm dòng để chọn dịch vụ kế tiếp. */
  focus(): void {
    (this.autoComplete()?.inputEL()?.nativeElement as HTMLInputElement | undefined)?.focus();
  }

  onClear() {
    this.selectedItem = null;
    this.valueChange.emit(null);
    this.onChange(null);
    this.onTouched();
  }

  onBlur() {
    if (this.selectedItem && !this.selectedItem.id) {
      this.selectedItem = null;
      this.onChange(null);
      this.onTouched();
    }
  }

  writeValue(id: number | null): void {
    this.pendingId = id;
    this.selectedItem = id ? this.cache.getById(id) || null : null;
    // OnPush: patchValue() từ bên ngoài (vd sau khi đổi dịch vụ ở order-plan-form) gọi thẳng
    // writeValue() mà không đi qua onSelect()/event — không markForCheck thì view không re-render,
    // service_id đổi đúng nhưng tên hiển thị vẫn đứng yên cho tới khi có CD trigger khác.
    this.cdr.markForCheck();
  }

  registerOnChange(fn: (value: number | null) => void): void {
    this.onChange = fn;
  }

  registerOnTouched(fn: () => void): void {
    this.onTouched = fn;
  }

  setDisabledState(isDisabled: boolean): void {
    this.disabled = isDisabled;
  }

  private onChange = (value: number | null) => {};

  private onTouched = () => {};

  private getFilteredItems(): ServiceModel[] {
    const filterKey = `${this.serviceTypeId || ''}_${this.status ?? ''}_v${this.cache.dataVersion}`;

    if (this.lastFilterKey !== filterKey) {
      const base = this.cache.getAll();
      this.cachedFilteredItems = base.filter((item) => {
        if (this.serviceTypeId && item.service_type_id !== this.serviceTypeId) return false;
        if (this.status !== null && this.status !== undefined && item.status !== this.status) return false;
        return true;
      });
      this.lastFilterKey = filterKey;
    }

    return this.cachedFilteredItems;
  }
}
