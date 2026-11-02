// Danh mục bước tiến trình khách hàng (epic customer-stage-pipeline).
// ĐỔI Ở ĐÂY ⇒ PHẢI đổi cả BE `pkg/define/customer.go` (CustomerStageType*, CustomerStageColors).

/** Loại bước — khớp BE `define.CustomerStageType*`. */
export const CUSTOMER_STAGE_TYPE = {
  OPEN: 1, // đang mở
  WON: 2, // thành công
  LOST: 3, // thất bại
} as const;

export type CustomerStageType = (typeof CUSTOMER_STAGE_TYPE)[keyof typeof CUSTOMER_STAGE_TYPE];

export const CUSTOMER_STAGE_TYPE_OPTIONS: { value: CustomerStageType; labelKey: string }[] = [
  { value: CUSTOMER_STAGE_TYPE.OPEN, labelKey: 'config.customer_stage.type.open' },
  { value: CUSTOMER_STAGE_TYPE.WON, labelKey: 'config.customer_stage.type.won' },
  { value: CUSTOMER_STAGE_TYPE.LOST, labelKey: 'config.customer_stage.type.lost' },
];

/** Từ số lựa chọn này trở lên, dropdown chọn bước bật ô tìm kiếm. */
export const CUSTOMER_STAGE_FILTER_MIN_OPTIONS = 8;

/** Khoá màu hợp lệ — khớp BE `define.CustomerStageColors` (bảng màu cố định, không nhận mã hex). */
export const CUSTOMER_STAGE_COLORS = [
  'gray',
  'blue',
  'teal',
  'green',
  'amber',
  'orange',
  'red',
  'purple',
] as const;

export type CustomerStageColor = (typeof CUSTOMER_STAGE_COLORS)[number];

export const CUSTOMER_STAGE_COLOR_DEFAULT: CustomerStageColor = 'gray';

// labelKey viết LITERAL (không ghép chuỗi) để `npm run i18n:check` thấy key đang được dùng.
// Record<CustomerStageColor, …> ⇒ thiếu/thừa màu so với CUSTOMER_STAGE_COLORS là lỗi compile.
const CUSTOMER_STAGE_COLOR_LABEL_KEY: Record<CustomerStageColor, string> = {
  gray: 'config.customer_stage.color.gray',
  blue: 'config.customer_stage.color.blue',
  teal: 'config.customer_stage.color.teal',
  green: 'config.customer_stage.color.green',
  amber: 'config.customer_stage.color.amber',
  orange: 'config.customer_stage.color.orange',
  red: 'config.customer_stage.color.red',
  purple: 'config.customer_stage.color.purple',
};

/** Danh sách màu cho dropdown — suy từ CUSTOMER_STAGE_COLORS (cùng thứ tự). */
export const CUSTOMER_STAGE_COLOR_OPTIONS: { value: CustomerStageColor; labelKey: string }[] =
  CUSTOMER_STAGE_COLORS.map((value) => ({ value, labelKey: CUSTOMER_STAGE_COLOR_LABEL_KEY[value] }));

/** Khoá màu → i18n key tên màu (khoá lạ ⇒ tên màu xám). */
export function customerStageColorLabelKey(color: string | null | undefined): string {
  return (
    CUSTOMER_STAGE_COLOR_OPTIONS.find((o) => o.value === color) ?? CUSTOMER_STAGE_COLOR_OPTIONS[0]
  ).labelKey;
}

/**
 * Khoá màu → class Tailwind. Mỗi class viết ĐỦ chuỗi literal (Tailwind JIT quét nguồn, ghép chuỗi
 * động sẽ không sinh CSS). Nền = màu palette ở độ trong suốt thấp + chữ đậm/nhạt theo theme
 * (`dark:`) để đạt contrast cả sáng lẫn tối — dùng palette mặc định của Tailwind, KHÔNG màu brand mới.
 * - `badge`: huy hiệu có nền (tên bước trong list/chi tiết/bộ lọc)
 * - `dot`: chấm màu nhỏ (cạnh nhãn, trong dropdown)
 */
export const CUSTOMER_STAGE_COLOR_CLASS: Record<CustomerStageColor, { badge: string; dot: string }> = {
  gray: { badge: 'bg-gray-500/15 text-gray-700 dark:text-gray-300', dot: 'bg-gray-500' },
  blue: { badge: 'bg-blue-500/15 text-blue-700 dark:text-blue-300', dot: 'bg-blue-500' },
  teal: { badge: 'bg-teal-500/15 text-teal-700 dark:text-teal-300', dot: 'bg-teal-500' },
  green: { badge: 'bg-green-500/15 text-green-700 dark:text-green-300', dot: 'bg-green-500' },
  amber: { badge: 'bg-amber-500/15 text-amber-700 dark:text-amber-300', dot: 'bg-amber-500' },
  orange: { badge: 'bg-orange-500/15 text-orange-700 dark:text-orange-300', dot: 'bg-orange-500' },
  red: { badge: 'bg-red-500/15 text-red-700 dark:text-red-300', dot: 'bg-red-500' },
  purple: { badge: 'bg-purple-500/15 text-purple-700 dark:text-purple-300', dot: 'bg-purple-500' },
};

/** Khoá màu không hợp lệ/rỗng ⇒ xám (không throw). */
export function customerStageColorClass(color: string | null | undefined): {
  badge: string;
  dot: string;
} {
  return (
    CUSTOMER_STAGE_COLOR_CLASS[(color ?? '') as CustomerStageColor] ??
    CUSTOMER_STAGE_COLOR_CLASS[CUSTOMER_STAGE_COLOR_DEFAULT]
  );
}
