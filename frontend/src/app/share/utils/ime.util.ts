/**
 * Phím đang thuộc một lượt SOẠN CHỮ của bộ gõ (IME có composition: Telex/VNI của macOS/iOS/Windows 11, bộ gõ CJK…).
 *
 * Enter lúc đang soạn là để CHỐT CHỮ đang soạn, không phải thao tác của app: `keydown` mang `isComposing = true`
 * (Safari cũ: `keyCode = 229`) và bắn TRƯỚC `compositionend` — `ngModel`/`DefaultValueAccessor` còn đang đệm chữ đang
 * soạn nên model CHƯA có từ cuối. Handler "Enter = thêm dòng" chạy lúc này sẽ chốt giá trị thiếu chữ rồi xoá trắng ô.
 * Gọi ở đầu mọi handler Enter-để-hành-động.
 */
export function isImeComposing(event: Event): boolean {
  const e = event as KeyboardEvent;
  return e.isComposing === true || e.keyCode === 229;
}
