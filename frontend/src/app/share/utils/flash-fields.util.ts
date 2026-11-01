/**
 * Nháy nền nhạt các ô VỪA ĐƯỢC ĐIỀN TỰ ĐỘNG (áp đơn mẫu, chọn mẫu lời dặn, chọn thuốc điền sẵn cách dùng…) — để mắt
 * thấy ô nào vừa đổi thay vì giá trị "bụp" hiện ra. Web Animations: tint màu primary ~18% → nền gốc của ô trong 900ms,
 * không đụng class/style của ô (chạy lại được ngay nếu điền tiếp).
 *
 * `prefers-reduced-motion` ⇒ bỏ qua: luật giảm chuyển động toàn cục trong `styles.scss` chỉ phủ CSS, KHÔNG phủ `el.animate`.
 * Gọi SAU khi view đã render giá trị mới (vd trong `afterNextRender`) để nháy đúng ô đang hiển thị.
 */
const FLASH_MS = 900;
const FLASH_ALPHA = 0.18;

export function flashFields(root: HTMLElement | null | undefined, selectors: readonly string[]): void {
  if (!root || !selectors.length || typeof window === 'undefined') return;
  if (window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) return;
  const tint = primaryTint();
  for (const sel of selectors) {
    root.querySelectorAll<HTMLElement>(sel).forEach((el) => {
      if (el.offsetWidth === 0 || typeof el.animate !== 'function') return;
      const base = getComputedStyle(el).backgroundColor;
      el.animate([{ backgroundColor: tint }, { backgroundColor: base }], { duration: FLASH_MS, easing: 'ease-out' });
    });
  }
}

/**
 * Màu primary ĐANG DÙNG ở độ đậm thấp. `--p-primary-color` của preset là `light-dark(sáng, tối)` (đo 2026-10-20) — đưa
 * nguyên biểu thức vào `color-mix` để trình duyệt tự chọn theo theme (sáng/tối), không tự tách hex.
 */
function primaryTint(): string {
  const raw = getComputedStyle(document.documentElement).getPropertyValue('--p-primary-color').trim() || '#7B3F9E';
  return `color-mix(in srgb, ${raw} ${FLASH_ALPHA * 100}%, transparent)`;
}
