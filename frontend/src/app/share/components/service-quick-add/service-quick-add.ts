import { ChangeDetectionStrategy, Component, computed, inject, Injector, input, output, signal, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { disabled, form, FormField } from '@angular/forms/signals';
import { TranslatePipe } from '@ngx-translate/core';
import { ButtonDirective } from 'primeng/button';
import { FloatLabel } from 'primeng/floatlabel';
import { InputGroup } from 'primeng/inputgroup';
import { InputNumber } from 'primeng/inputnumber';
import { Tooltip } from 'primeng/tooltip';

import { ServiceModel } from '@/core/models/config/clinic/service.model';
import { SERVICE_DISCOUNT_TYPE_SELECT } from '@/define';
import { ServicesAutocompleteFormControl, ServicesCacheService } from '@/indexeddb/config/components/services';
import { UnitHelperService } from '@/indexeddb/config/components/unit';
import { QuickAddPanel } from '@/share/components/quick-add-panel/quick-add-panel';
import { CurrencyFormatPipe } from '@/share/pipes/currency-format/currency-format.pipe';
import { QuickEntryController } from '@/share/utils/quick-entry.controller';

/** Dòng vừa chốt ở khung — form cha tự dựng dòng bảng (chèn đầu) rồi chạy đúng luồng chọn dịch vụ của mình. */
export interface ServiceQuickAddEvent {
  service: ServiceModel;
  /** Số nguyên ≥1 (trống/≤0 ⇒ 1). */
  quantity: number;
  /** Đơn hàng: đã kẹp trong [min_price, max_price]; giá cố định ⇒ đúng giá catalog. */
  unit_price: number;
  service_discount: number;
  service_discount_type: number;
}

/** Khoảng giá được phép sửa của dịch vụ đang chọn (chỉ khi `enforcePriceRange`). */
interface PriceRange {
  min: number;
  max: number;
  /** Giá cố định (thiếu min/max hoặc min = max) ⇒ khoá ô — cùng luật `unitPriceLocked` của dòng Đơn hàng. */
  locked: boolean;
}

/**
 * Giá trị các ô của khung — đi qua Signal Forms `[formField]` (KHÔNG `[ngModel]` một chiều): NgModel ghi giá trị xuống
 * `p-inputnumber` trong 1 microtask SAU change detection, ô giá bị blur đúng khe đó (focus nhảy ô sau khi chọn dịch vụ)
 * ⇒ `onInputBlur` đọc lại chữ cũ trong DOM và ghi ĐÈ ngược lên model (đo ở ERP bằng spec). Mọi field seed giá trị cụ thể
 * (gotcha #1 building-angular-signal-forms). Khung không có Giảm khác / Ghi chú — nhập trên dòng bảng.
 */
interface QuickEntryModel {
  quantity: number | null;
  unit_price: number | null;
  service_discount: number | null;
}

const emptyEntry = (): QuickEntryModel => ({ quantity: 1, unit_price: null, service_discount: 0 });

let instanceCounter = 0;

/** Trần chiết khấu dịch vụ: theo tiền ⇒ số lượng × đơn giá; theo % ⇒ 100 (cùng luật ô chiết khấu trên dòng). */
function discountMax(type: number, quantity: number, unitPrice: number | null): number {
  if (type === SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_CURRENCY) return quantity * (unitPrice ?? 0);
  return 100;
}

/**
 * "Tạm tính" của khung — CÙNG công thức dòng (`totalTotalService` của Đơn hàng): sub = SL × giá; trừ chiết khấu DV
 * (% ⇒ làm tròn). Khung không có Giảm khác (luôn 0). Giảm theo hạng khách KHÔNG tính ở đây (Đơn hàng tự tính trên dòng).
 */
function lineSubtotal(quantity: number, unitPrice: number | null, discount: number | null, discountType: number): number {
  const sub = quantity * (Number(unitPrice) || 0);
  const d = Number(discount) || 0;
  const disc = discountType === SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT ? Math.round((d * sub) / 100) : d;
  return Math.round(disc > 0 ? sub - disc : sub);
}

const clamp = (v: number, min: number, max: number) => Math.min(Math.max(v, min), max);

/**
 * Khung "Thêm dịch vụ" phía trên bảng dòng của form Đơn hàng (port từ ERP `share/components/service-quick-add`, bỏ nhánh
 * dịch vụ răng + lọc thương hiệu — CRM không có): Dịch vụ · Số lượng · Đơn giá · Chiết khấu DV (đ/%) · Tạm tính · nút
 * Thêm. Khung KHÔNG biết bảng: chốt ⇒ phát `(add)`, form cha chèn dòng lên đầu bảng.
 *
 * - Gõ tên + Enter (gợi ý đầu tự focus) ⇒ chọn dịch vụ, điền đơn giá như chọn trên dòng (`min_price ?? max_price ?? 0`)
 *   rồi nhảy sang Số lượng. Enter ở ô bất kỳ / nút Thêm ⇒ chốt; Enter đang ghép chữ IME ⇒ bỏ qua.
 *   Chốt xong khung về mặc định (giữ loại chiết khấu đ/% vừa dùng), con trỏ về ô dịch vụ.
 * - `enforcePriceRange` (Đơn hàng): đơn giá kẹp trong [min_price, max_price]; giá cố định ⇒ khoá ô giá.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-service-quick-add',
  standalone: true,
  imports: [
    FormsModule,
    FormField,
    TranslatePipe,
    ButtonDirective,
    FloatLabel,
    InputGroup,
    InputNumber,
    Tooltip,
    CurrencyFormatPipe,
    ServicesAutocompleteFormControl,
    QuickAddPanel,
  ],
  templateUrl: './service-quick-add.html',
})
export class ServiceQuickAdd {
  /** Đơn hàng: bật luật giá [min, max] + khoá giá cố định (giống ô đơn giá trên dòng). */
  readonly enforcePriceRange = input(false);
  /** Đơn hàng: ghi chú cạnh "Tạm tính" rằng số này CHƯA trừ giảm theo hạng khách (tính trên dòng sau khi thêm). */
  readonly subtotalBeforePromotions = input(false);
  readonly add = output<ServiceQuickAddEvent>();

  private readonly servicesCache = inject(ServicesCacheService);
  private readonly unitHelper = inject(UnitHelperService);

  private readonly picker = viewChild<ServicesAutocompleteFormControl>('picker');
  private readonly qtyInput = viewChild<InputNumber>('qty');

  /** id riêng mỗi instance — 2 khung cùng trang (dialog chồng) không trùng id/nhãn. */
  protected readonly ids = (() => {
    const n = ++instanceCounter;
    return { qty: `sqa_qty_${n}`, price: `sqa_price_${n}`, discount: `sqa_discount_${n}` };
  })();
  protected readonly SERVICE_DISCOUNT_TYPE_SELECT = SERVICE_DISCOUNT_TYPE_SELECT;
  /** Key ở dạng `titleKey: '…'` để `i18n:check` thấy đang dùng (truyền qua input của `app-quick-add-panel`). */
  protected readonly panel = { titleKey: 'crm.order_plan.form.add_service' } as const;

  readonly service = signal<ServiceModel | null>(null);
  readonly entry = signal<QuickEntryModel>(emptyEntry());
  readonly discountType = signal<number>(SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT);

  readonly unitName = computed(() => this.unitHelper.getLabel(this.service()?.unit_id) ?? '');

  readonly priceRange = computed<PriceRange | null>(() => {
    const s = this.service();
    if (!this.enforcePriceRange() || !s) return null;
    const locked = s.min_price == null || s.max_price == null || Number(s.min_price) === Number(s.max_price);
    return { min: Number(s.min_price ?? 0), max: Number(s.max_price ?? 0), locked };
  });
  /**
   * Khoá ô khai trong schema, KHÔNG `[disabled]` trên template (FormField ghi đè input đó — gotcha
   * building-angular-signal-forms #11). Giá cố định (Đơn hàng) ⇒ khoá ô giá.
   */
  protected readonly f = form(this.entry, (p) => {
    disabled(p.unit_price, { when: () => !!this.priceRange()?.locked });
  });

  /** Số lượng thực sẽ dùng: số nguyên ≥1. */
  readonly effectiveQuantity = computed(() => {
    const q = this.entry().quantity;
    return q != null && q >= 1 ? Math.round(q) : 1;
  });
  readonly discountMax = computed(() => discountMax(this.discountType(), this.effectiveQuantity(), this.entry().unit_price));
  readonly subtotal = computed(() => {
    const e = this.entry();
    return lineSubtotal(this.effectiveQuantity(), e.unit_price, e.service_discount, this.discountType());
  });

  protected readonly ctrl = new QuickEntryController({
    injector: inject(Injector),
    commit: () => this.commit(),
    focusPicker: () => this.picker()?.focus(),
  });

  /** Chọn dịch vụ ⇒ điền đơn giá như chọn trên dòng rồi nhảy sang Số lượng. */
  onServiceChange(serviceId: number | null): void {
    const service = serviceId ? (this.servicesCache.getById(serviceId) ?? null) : null;
    this.service.set(service);
    if (!service) return;
    this.ctrl.markPicked();
    const currency = this.discountType() === SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_CURRENCY;
    this.entry.update((e) => ({
      ...e,
      quantity: e.quantity != null && e.quantity >= 1 ? e.quantity : 1,
      unit_price: service.min_price ?? service.max_price ?? 0,
      service_discount: currency ? 0 : e.service_discount,
    }));
    this.focusAfterPick(() => this.qtyInput()?.input()?.nativeElement);
  }

  /**
   * Focus ô kế tiếp SAU khi `p-autocomplete` đóng gợi ý. Chọn bằng chuột: PrimeNG `onOptionSelect` phát `onSelect` RỒI mới
   * `hide(true)` — hẹn `setTimeout(0)` focus LẠI ô autocomplete. Hẹn 1 lớp (như `QuickEntryController.focusLater`) chạy
   * TRƯỚC timer đó ⇒ con trỏ bị kéo về ô dịch vụ (đo ở ERP bằng spec). Hẹn 2 lớp ⇒ chạy sau nó.
   */
  private focusAfterPick(get: () => HTMLElement | null | undefined): void {
    setTimeout(() => this.ctrl.focusLater(get), 0);
  }

  toggleDiscountType(): void {
    this.discountType.update((t) =>
      t === SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_CURRENCY
        ? SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT
        : SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_CURRENCY,
    );
    this.entry.update((e) => ({ ...e, service_discount: 0 }));
  }

  /** Chốt khung ⇒ phát `(add)`. Chưa chọn dịch vụ ⇒ focus ô dịch vụ. */
  commit(): void {
    const service = this.service();
    if (!service?.id) {
      this.picker()?.focus();
      return;
    }
    const quantity = this.effectiveQuantity();
    const e = this.entry();
    let unitPrice = Number(e.unit_price ?? 0) || 0;
    const range = this.priceRange();
    if (range) unitPrice = range.locked ? Number(service.min_price ?? service.max_price ?? 0) : clamp(unitPrice, range.min, range.max);
    const type = this.discountType();
    const maxDiscount = discountMax(type, quantity, unitPrice);
    this.add.emit({
      service,
      quantity,
      unit_price: unitPrice,
      service_discount: clamp(Number(e.service_discount) || 0, 0, maxDiscount),
      service_discount_type: type,
    });
    this.service.set(null);
    this.entry.set(emptyEntry());
    this.ctrl.reset();
  }
}
