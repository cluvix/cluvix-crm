import { FilterMetadata } from 'primeng/api';

/**
 * Loại filter — quyết định 3 thứ cùng lúc: matchMode gửi p-table,
 * giá trị rỗng của control, và cách encode/decode ra URL.
 *
 * - text       : ô tìm kiếm tự do        → matchMode 'contains'
 * - in         : multiselect (mảng id)   → matchMode 'in'
 * - equals     : select 1 giá trị        → matchMode 'equals'
 * - dateRange  : p-datePicker selectionMode="range" → matchMode 'between', value là cặp unix
 * - dateSingle : p-datePicker chọn 1 ngày → gửi BE chuỗi `yyyy-MM-dd`, matchMode mặc định 'custom'
 * - monthDayRange : khoảng ngày-tháng KHÔNG có năm (`MM-DD`…`MM-DD`, vd lọc sinh nhật)
 *                   → matchMode 'between', value là cặp chuỗi `MM-DD`
 * - dateRangeSplit: khoảng ngày mà BE đọc bằng HAI param TÊN RIÊNG do component tự ghép
 *                   (vd `_dueDateGte`/`_dueDateLte`, chuỗi `yyyy-MM-dd`). Base KHÔNG sinh param
 *                   (`toFilterMeta` trả null) — component ghép trong `fetchList()`. Base chỉ lo
 *                   FormControl + đồng bộ URL, nhờ đó filter sống sót qua Back/F5.
 * - custom     : tự lo bằng def.toFilter()
 *
 * `matchMode: 'custom'` (trả từ `toFilter`) luôn gửi BE dạng raw `field=value` (không bọc
 * `filter[field][...]`), bất kể `name` có bắt đầu bằng `_` hay không (`UtilsService.buildFilter`).
 * Field BE đọc qua `c.Query("field")` (không qua `query.ApplyFilter`) mới dùng `kind: 'custom'`.
 */
export type FilterKind =
  | 'text'
  | 'in'
  | 'equals'
  | 'dateRange'
  | 'dateSingle'
  | 'monthDayRange'
  | 'dateRangeSplit'
  | 'custom';

/**
 * Chặn trên độ dài filter `kind: 'text'` (vd `_keyword`) — giá trị đi thẳng vào query string
 * GET (`base-list-v2.ts` → `searchParams()`), gõ/paste vài nghìn ký tự làm request vượt giới hạn
 * độ dài dòng request của backend/reverse-proxy → HTTP 414 Request URI Too Long, list trắng không
 * rõ lý do (sự cố thật 2026-08-05, `ServiceList`). 200 ký tự đủ rộng cho mọi nhu cầu tìm theo
 * tên/mã/ghi chú thực tế, không có nghiệp vụ nào cần search string dài hơn.
 */
export const MAX_TEXT_FILTER_LENGTH = 200;

/**
 * Date → `yyyy-MM-dd` theo wall-clock CỤC BỘ của Date đó.
 *
 * KHÔNG chuyển timezone ở đây: `p-datePicker` trả Date theo giờ máy, và `BaseApp.today`
 * (= `timezoneService.now()`) vốn đã là Date có wall-clock bằng giờ company. Chuyển thêm lần nữa
 * sẽ lệch ngày. Cho ra đúng chuỗi mà `formatDate(d, 'yyyy-MM-dd', 'en-US')` của V1 sinh ra.
 */
export function toDateOnlyString(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** `yyyy-MM-dd` → Date lúc 00:00 giờ máy — hàm ngược của `toDateOnlyString` */
export function fromDateOnlyString(s: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
  if (!m) return null;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  return isNaN(d.getTime()) ? null : d;
}

/**
 * Năm nhuận cố định dùng khi ngày-tháng chỉ tồn tại ở năm nhuận (`02-29`).
 * Năm không mang ý nghĩa với `monthDayRange` (control chỉ hiện `dd/mm`, BE so cột `MM-DD`).
 */
const LEAP_YEAR_FALLBACK = 2024;

/**
 * Date → `MM-DD` theo wall-clock CỤC BỘ của Date đó.
 *
 * Cùng lý do như `toDateOnlyString`: `p-datePicker` trả Date theo giờ máy và `BaseApp.today`
 * vốn đã là Date có wall-clock bằng giờ company — chuyển timezone thêm lần nữa sẽ lệch ngày.
 */
export function toMonthDayString(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/**
 * `MM-DD` → Date lúc 00:00 giờ máy — hàm ngược của `toMonthDayString`.
 *
 * Năm không mang ý nghĩa nên lấy năm hiện tại. Riêng `02-29` ở năm KHÔNG nhuận thì
 * `new Date(y, 1, 29)` tự trôi sang 01/03 → phát hiện bằng cách so lại month/date sau khi dựng,
 * rồi lùi về một năm nhuận cố định để giữ đúng ngày-tháng người dùng chọn.
 *
 * Cùng cơ chế so lại đó loại luôn ngày không tồn tại (`02-30`, `04-31`) → trả null.
 */
export function fromMonthDayString(s: string): Date | null {
  const m = /^(\d{2})-(\d{2})$/.exec(s);
  if (!m) return null;
  const month = Number(m[1]);
  const day = Number(m[2]);
  if (month < 1 || month > 12 || day < 1 || day > 31) return null;

  const build = (year: number): Date | null => {
    const d = new Date(year, month - 1, day);
    // Date trôi sang tháng sau khi ngày không tồn tại trong tháng đó → coi như không dựng được.
    return d.getMonth() === month - 1 && d.getDate() === day ? d : null;
  };

  return build(new Date().getFullYear()) ?? build(LEAP_YEAR_FALLBACK);
}

/**
 * Khai báo 1 filter của list. Đây là nguồn duy nhất sinh ra:
 * FormControl + giá trị mặc định, subscription valueChanges → dt.filter(),
 * object filters khởi tạo cho p-table, hành vi clearFilter, và tham số URL.
 *
 * Thêm 1 filter = thêm 1 dòng ở đây, không phải sửa 4 chỗ như BaseList cũ.
 */
export interface FilterFieldDef {
  /** Tên control, đồng thời là tên field filter gửi BE */
  name: string;

  kind: FilterKind;

  /**
   * Giá trị mặc định của control. Truyền factory `() => ...` cho giá trị phụ thuộc
   * thời điểm (vd `() => [tz.todayStart(), tz.todayEnd()]`) để lần reset về mặc định (bấm lại
   * menu, hoặc `clearFilter()` với field `required`) tính lại theo hôm nay chứ không dính ngày lúc mở trang.
   * Bỏ trống → `[]` với kind 'in', `null` với các kind còn lại.
   */
  default?: any | (() => any);

  /**
   * Filter BẮT BUỘC có giá trị — BE trả 422 khi thiếu (vd báo cáo bắt buộc dải ngày + trần 1 năm).
   * "Xoá bộ lọc" và URL đã-lọc-mà-thiếu-param trả field này về `default` thay vì rỗng.
   * Bỏ trống → "Xoá bộ lọc" xoá field về rỗng như mọi field khác, KỂ CẢ field có `default`
   * (`default` chỉ áp lúc mới mở màn).
   */
  required?: boolean;

  /** ms debounce trước khi bắn filter. Mặc định: 500 cho 'text', 300 cho các kind khác. */
  debounce?: number;

  /** Ghi đè matchMode suy ra từ kind (hiếm khi cần) */
  matchMode?: string;

  /**
   * Tên tham số URL (đã bỏ prefix). Bỏ trống → suy theo quy ước ở `urlKeyOf()`.
   * Chỉ khai khi muốn tên khác quy ước (vd `appointment_date` → `date` cho URL ngắn).
   */
  urlKey?: string;

  /** false = filter này không đẩy lên URL (vd filter nội bộ theo context) */
  urlSync?: boolean;

  /** Kiểu phần tử khi parse ngược từ URL. Mặc định 'number' cho 'in'/'equals', 'string' cho 'text'. */
  itemType?: 'number' | 'string';

  /**
   * Map giá trị control → FilterMetadata. Chỉ khai khi kind='custom' hoặc cần
   * hành vi đặc biệt (vd sentinel id=0 nghĩa là IS NULL).
   * Trả null = coi như không lọc.
   */
  toFilter?: (value: any) => FilterMetadata | null;
}

/**
 * Tên param URL của 1 field theo quy ước chung: bỏ gạch dưới đứng đầu + camelCase → snake_case.
 *
 *   _keyword     → keyword          _groupLocal → group_local
 *   customerType → customer_type    branch_id   → branch_id
 *
 * Có quy ước thì cùng một field ở list khác nhau ra cùng một tên param, URL toàn app đồng nhất
 * mà không cần catalog dùng chung — mỗi list vẫn khai riêng, tường minh tại chỗ.
 *
 * Trường hợp quy ước cho ra tên trùng nhau (kinh điển: `_status` và `status` đều ra `status`)
 * thì PHẢI khai `urlKey` tường minh; `BaseListV2` chặn ngay lúc khởi tạo chứ không để URL hỏng âm thầm.
 */
export function urlKeyOf(def: FilterFieldDef): string {
  if (def.urlKey) return def.urlKey;
  return def.name
    .replace(/^_+/, '')
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .toLowerCase();
}

/** matchMode suy ra từ kind */
export function matchModeOf(def: FilterFieldDef): string {
  if (def.matchMode) return def.matchMode;
  switch (def.kind) {
    case 'text':
      return 'contains';
    case 'in':
      return 'in';
    case 'dateRange':
      return 'between';
    // Bắt buộc: thiếu case này thì rơi vào 'equals' → buildFilter xuất `filter[x][eq]`
    // trong khi BE đọc `filter[x][gte]`/`[lte]` → filter chết âm thầm.
    case 'monthDayRange':
      return 'between';
    // Vô hại ở luồng hiện tại (`toFilterMeta` trả `null` vô điều kiện cho kind này nên
    // matchMode không bao giờ thực sự được lưu vào `dt.filters`), nhưng khai đúng 'between'
    // để nhất quán với các kind khoảng-ngày khác và tránh bẫy nếu sau này có chỗ khác
    // gọi `matchModeOf()` cho field này.
    case 'dateRangeSplit':
      return 'between';
    default:
      return 'equals';
  }
}

/** Giá trị "không lọc" của 1 control */
export function emptyValueOf(def: FilterFieldDef): any {
  return def.kind === 'in' ? [] : null;
}

/** Giá trị mặc định (giải factory nếu có) */
export function defaultValueOf(def: FilterFieldDef): any {
  if (def.default === undefined) return emptyValueOf(def);
  return typeof def.default === 'function' ? (def.default as () => any)() : def.default;
}

/** Giá trị sau khi "Xoá bộ lọc": rỗng, trừ field `required` thì về mặc định */
export function clearedValueOf(def: FilterFieldDef): any {
  return def.required ? defaultValueOf(def) : emptyValueOf(def);
}

/** Rỗng theo nghĩa "không lọc" — khớp với UtilsService.isFilterValueBlank */
export function isBlankFilterValue(value: any): boolean {
  if (value === null || value === undefined) return true;
  if (typeof value === 'string' && value.trim().length === 0) return true;
  if (Array.isArray(value)) return value.length === 0 || value.every((v) => v === null || v === undefined);
  return false;
}
