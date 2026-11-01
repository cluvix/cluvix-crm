import { afterNextRender, Injector, signal } from '@angular/core';

import { isImeComposing } from '@/share/utils/ime.util';

/**
 * Cơ chế chung của khung "nhập nhanh rồi Enter để thêm dòng" (khung Thêm dịch vụ / Thêm thuốc): xử lý Enter, cờ
 * "Enter vừa chọn gợi ý", dựng lại khung sau mỗi lần thêm và focus lại ô chọn. Mỗi form tự giữ field + logic `commit`.
 *
 * - Enter đang chốt chữ của bộ gõ (IME) hoặc Shift+Enter (xuống dòng trong textarea) ⇒ KHÔNG chốt.
 * - Enter ở ô chọn (autocomplete) vừa dùng để CHỌN gợi ý cũng nổi lên handler ngay sau ⇒ `markPicked()` đánh dấu để
 *   lần Enter đó không chốt; chọn từ trước (chuột / quay lại ô) thì Enter ở ô chọn chốt như các ô khác.
 * - `rowKey`: Enter cập nhật model của ô rồi reset trong CÙNG nhịp ⇒ binding `[ngModel]` không thấy đổi, ô vẫn hiện
 *   số cũ (đo 2026-10-20). Template bọc khung trong `@for (k of [ctrl.rowKey()]; track k)` để dựng lại view khi reset.
 */
export interface QuickEntryOptions {
  /** Injector của component (cho `afterNextRender` gọi ngoài injection context). */
  injector: Injector;
  /** Chốt khung ⇒ thêm dòng. Gọi khi Enter hợp lệ. */
  commit: () => void;
  /** Đưa con trỏ về ô chọn — gọi sau lần render kế của `reset()` (ô là view MỚI). */
  focusPicker: () => void;
}

export class QuickEntryController {
  readonly rowKey = signal(0);
  private justPicked = false;

  constructor(private readonly opts: QuickEntryOptions) {}

  /** Gọi trong handler "đã chọn 1 gợi ý" của ô chọn. */
  markPicked(): void {
    this.justPicked = true;
    setTimeout(() => (this.justPicked = false), 0);
  }

  /** Enter ở ô chọn: phím vừa chọn gợi ý / chưa có giá trị ⇒ bỏ qua; đã chọn từ trước ⇒ chốt. */
  onPickerEnter(event: Event, hasValue: boolean): void {
    if (this.justPicked || !hasValue) return;
    this.onEnter(event);
  }

  /** Enter ở ô bất kỳ của khung ⇒ chốt (KHÔNG submit form). */
  onEnter(event: Event): void {
    if (isImeComposing(event) || (event as KeyboardEvent).shiftKey) return;
    event.preventDefault();
    event.stopPropagation();
    this.opts.commit();
  }

  /** Dựng lại khung (các ô về mặc định) rồi focus ô chọn sau lần render kế. */
  reset(): void {
    this.rowKey.update((k) => k + 1);
    afterNextRender(() => this.opts.focusPicker(), { injector: this.opts.injector });
  }

  /** Focus (và bôi đen) 1 ô sau 1 nhịp — đợi overlay gợi ý đóng. */
  focusLater(get: () => HTMLElement | null | undefined): void {
    setTimeout(() => {
      const el = get();
      el?.focus();
      (el as HTMLInputElement | null | undefined)?.select?.();
    }, 0);
  }
}
