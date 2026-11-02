export const TREATMENT_FLOW_TYPE = {
  PERCENT_BASED: 1,
  COUNT_BASED: 2,
  VISIT_BASED: 3,
} as const;

export const BUSINESS_TYPE = {
  PRIVATE: 1,
  PUBLIC: 2,
  CLINIC: 3,
  CLINIC_GENERAL: 4,
  CLINIC_SPECIAL: 5,
  HOSPITAL_PRIVATE: 6,
  MEDICAL_CENTER: 7,
  BEAUTY_CENTER: 8,
  SPA: 9,
} as const;
export const BUSINESS_TYPE_OPTIONS = [
  { value: 1, label: 'Tư nhân', labelKey: 'config.options.business_type.private' },
  { value: 2, label: 'Bệnh viện công', labelKey: 'config.options.business_type.public' },
  { value: 3, label: 'Cơ sở đơn lẻ', labelKey: 'common.label.clinic' },
  { value: 4, label: 'Chuỗi nhiều cơ sở', labelKey: 'common.label.general_clinic' },
  { value: 5, label: 'Cơ sở chuyên biệt', labelKey: 'config.options.business_type.clinic_special' },
  { value: 6, label: 'Bệnh viện tư nhân', labelKey: 'config.options.business_type.hospital_private' },
  { value: 7, label: 'Trung tâm dịch vụ', labelKey: 'config.options.business_type.medical_center' },
  { value: 8, label: 'Thẩm mỹ viện', labelKey: 'config.options.business_type.beauty_center' },
  { value: 9, label: 'Spa', labelKey: 'config.options.business_type.spa' },
];
export const perPageOptionsTable = [15, 30, 50, 100]; // option cho table select num row
export const rowsTable = 15; // // số row mặc định
export const CONFIG_LOCALE_LANGUAGE_OPTIONS = [
  { value: 'vi-VN', label: 'Tiếng việt', labelKey: 'config.options.locale_language.vi' },
  { value: 'en-US', label: 'English', labelKey: 'config.options.locale_language.en' },
];

export const CONFIG_FILE_SELECT = {
  CONFIG_FILE_MAX_SIZE: 10485760, // 10MB — dùng chung nhiều module; crm/customer CreateAvatar BE nén còn ~100KB (Options riêng, không dùng OptFill500 — xem skill/backend/modules/crm/customer.md)
  CONFIG_FILE_ALLOW_EXT: ['jpg', 'png', 'jpeg'],

  // Quality khi encode ảnh chụp từ camera (canvas → JPEG) TRƯỚC khi upload.
  // Chỉ ảnh hưởng dung lượng UPLOAD, không ảnh hưởng chất lượng lưu trữ: BE luôn nén lại
  // (avatar → 500×500 ~100KB; ảnh liệu trình → 2048px Q88-95). Gửi quality cao hơn chỉ tốn
  // băng thông của người dùng, kết quả cuối không đổi.
  CONFIG_CAMERA_JPEG_QUALITY: 0.85,
};

export const CONFIG_OPTION_CRM_SELECT = {
  OPTION_UPDATE_PAYMENT: 1,
  OPTION_UPDATE_TREATMENT: 2,
  OPTION_UPDATE_ORDER_PLAN: 3,
  OPTION_VIEW_LEAD_OF_TELE: 5,
  OPTION_DELETE_NOTE: 6,
  OPTION_UPDATE_CUSTOMER_FOR_TELESALE: 12,
  OPTION_HIDE_NUMBER_PHONE: 14,
  OPTION_TIME_BEFORE: 16,
  OPTION_ALLOW_PREVIOUS_DAY: 17,
  OPTION_CUSTOMER_CROSS_BRANCH: 18,
  OPTION_AUTO_CARE_NEXT_DAY: 19,
  OPTION_EXPORT_PHONE: 20,
  // id 21 (tính số lần dùng theo tổng gói), 22 (bỏ qua kiểm tra thanh toán), 23 (dùng dịch vụ ở chi nhánh
  // khác) đã gỡ cả BE + DB — BE nay luôn cho dùng đơn hàng/dịch vụ/thanh toán của khách ở mọi chi nhánh.
  // story-14 (omnichat-core): id KHỚP BE `pkg/define/omni_channel.go` OptionOmnichat* — KHÔNG đoán.
  OPTION_OMNICHAT_VIEW_MODE: 24,
  OPTION_OMNICHAT_HUMAN_AGENT: 25,
  OPTION_OMNICHAT_AUTO_LINK_PHONE: 26,
  OPTION_OMNICHAT_AUTO_CREATE_LEAD: 27,
  // omnichat-settings story-01: id KHỚP BE OptionOmnichatFaqAutoReply=28 (default TẮT).
  OPTION_OMNICHAT_FAQ_AUTO_REPLY: 28,
  // omnichat-dashboard story-02: ngưỡng "stale" health badge (giờ, default 72).
  // id KHỚP BE OptionOmnichatStaleHours=32 (pkg/define/omni_channel.go) — 29-31 đã bị
  // omnichat-assignment story-02 (OptionOmnichatAssignPush/Pull/WipCapDefault) lấy trước,
  // FIX id collision: 29 (đoán ban đầu) → 32 (verify MAX(id) runtime = 31 trên DB 1 khách, 2026-07-18).
  OPTION_OMNICHAT_STALE_HOURS: 32,
  // story-18 (fhir-compliance-2026): bật/tắt form CLS structured (bảng items theo template) per-tenant,
  // mặc định BẬT. id=40 chọn theo MAX id hiện có trong migrations/*.sql (39 = med_dosage_structured,
  // story chạy song song 2026-07-21) + 1 — CHƯA có migration BE seed dòng `user_option id=40` (ngoài
  // phạm vi story-18 FE, KHÔNG đụng file BE). UserOptionHelperService.getUserOptionInfo(40) trả `null`
  // khi chưa seed → FE code phải coi null = BẬT (đúng yêu cầu "mặc định bật"); toggle TẮT qua màn
  // Tuỳ chọn chỉ hoạt động thật sau khi BE seed dòng này (follow-up cần làm, xem story-18 Log).
  OPTION_EMR_CLS_STRUCTURED: 40,
  // story-23 (fhir-compliance-2026): bật/tắt dosage builder structured trong form kê đơn.
  // id=39 KHỚP BE define.UserOptionMedDosageStructured (pkg/define/user.go, story-22), migration
  // ĐÃ seed mặc định BẬT (`SELECT ... WHERE NOT EXISTS`, xem story-22 Log) — vẫn coi null = BẬT
  // (an toàn nếu migration chưa `migrate up` ở môi trường nào đó, khớp pattern OPTION_EMR_CLS_STRUCTURED).
  OPTION_MED_DOSAGE_STRUCTURED: 39,
  // OPTION_TEAMCHAT_TYPING (id=42) đã xoá (story-22): module teamchat không còn ở FE CRM standalone,
  // 0 caller. Cột `user_option` id=42 trên DB có thể vẫn còn (BE ngoài phạm vi FE story này).
};
export const STATUS_OPTION = [
  { value: 1, label: 'Hiển thị', labelKey: 'common.label.visible' },
  { value: 0, label: 'Ẩn', labelKey: 'common.label.hidden' },
];

export const SEVICE_DISCOUNT_TYPE_OPTION = [
  { value: 1, label: 'VND', labelKey: 'define.discount_type.vnd' },
  { value: 2, label: '%', labelKey: 'common.label.item' },
];
export const TABLE_INDB_TELESALE = 185;

export enum STORAGE_KEY_SELECT {
  Recent = 'global_search_recent',
  Favourite = 'global_search_favourite',
  // Trang /config: link các mục cấu hình vừa mở (tối đa CONFIG_RECENT_LIMIT). Dữ liệu theo TÀI
  // KHOẢN (mục nào thấy được phụ thuộc quyền) nên nằm trong LOGOUT_KEYS, khác `Lang`.
  ConfigMenuRecent = 'config_menu_recent',
  KEY_ROWS_TABLE = 'KEY_ROWS_TABLE',
  Lang = 'lng',
  ConfigDataDropdown = 'config_data_dropdown',
  ConfigPermission = 'config_permission',
  LayoutConfig = 'layout_config',
  AuthUser = 'auth_user',
  ScheduleViewMode = 'schedule-view-mode',
  // Dashboard CRM tuỳ biến: bản TẠM của bố cục (chỉ bố cục, KHÔNG số liệu) — `{ "<uid>": {...} }`.
  // Theo TÀI KHOẢN ⇒ nằm trong LOGOUT_KEYS.
  CrmDashboardLayout = 'crm_dashboard_layout',
  // Đã xem gợi ý "Tuỳ chỉnh dashboard" lần đầu — cấu hình THIẾT BỊ (như Lang) ⇒ KHÔNG nằm trong LOGOUT_KEYS.
  CrmDashboardHintSeen = 'crm_dashboard_hint_seen',
  // Đã bỏ qua màn "Chọn bố cục khởi đầu" (4 mẫu) — cấu hình THIẾT BỊ như HintSeen ⇒ KHÔNG nằm trong LOGOUT_KEYS.
  CrmDashboardPresetChooserSeen = 'crm_dashboard_preset_chooser_seen',
  // Key PrimeUI lấy từ CDN, cache lại để lần mở sau dùng ngay (không chờ mạng).
  // CỐ Ý không nằm trong LOGOUT_KEYS: đây là cấu hình THIẾT BỊ (như Lang), không phải
  // dữ liệu auth theo tài khoản — xoá khi logout thì mỗi lần đăng xuất lại rơi về key
  // build-time cũ cho tới lần fetch CDN kế tiếp.
  PrimeuiLicense = 'primeui_license',
  // Mốc lần cuối hỏi CDN — chặn fetch lặp mỗi lần mở app (PRIMEUI_LICENSE_CHECK_INTERVAL_MS).
  PrimeuiLicenseCheckedAt = 'primeui_license_checked_at',
}

export const CURRENCY_OPTION = [
  { value: 'VND', label: 'Việt nam đồng', labelKey: 'config.options.currency.vnd' },
  { value: 'USD', label: 'Dollar mỹ', labelKey: 'config.options.currency.usd' },
  { value: 'EUR', label: 'Euro', labelKey: 'config.options.currency.eur' },
  { value: 'GBP', label: 'Bảng anh', labelKey: 'config.options.currency.gbp' },
  { value: 'RUB', label: 'Ruble nga', labelKey: 'config.options.currency.rub' },
  { value: 'CAD', label: 'Dollar canada', labelKey: 'config.options.currency.cad' },
  { value: 'AUD', label: 'Dollar úc', labelKey: 'config.options.currency.aud' },
  { value: 'CNY', label: 'Nhân dân tệ', labelKey: 'config.options.currency.cny' },
  { value: 'JPY', label: 'Nhân dân tệ', labelKey: 'config.options.currency.cny' },
  { value: 'CNY', label: 'Yên nhật', labelKey: 'config.options.currency.jpy' },
  { value: 'SGD', label: 'Dollar Sing', labelKey: 'config.options.currency.sgd' },
  { value: 'BTC', label: 'Bitcoin', labelKey: 'config.options.currency.btc' },
  { value: 'ETH', label: 'Ethereum', labelKey: 'config.options.currency.eth' },
  { value: 'XAU', label: 'Cây vàng', labelKey: 'config.options.currency.xau' },
];
export const WARD_TYPE_OPTION = [
  { value: 1, label: 'Phường', labelKey: 'config.options.ward_type.ward' },
  { value: 2, label: 'Xã', labelKey: 'config.options.ward_type.commune' },
];
export const TYPE_MILK_SELECT = {
  TYPE_MILK: 1,
  TYPE_ADULT: 2,
} as const;
export const TOOTH_TYPE_OPTION = [
  { value: TYPE_MILK_SELECT.TYPE_ADULT, label: 'Cố định', labelKey: 'config.options.tooth_type.permanent' },
  { value: TYPE_MILK_SELECT.TYPE_MILK, label: 'Tạm thời', labelKey: 'config.options.tooth_type.milk' },
];
export const TAX_TYPE_OPTION = [
  { value: 'none', label: 'KCT', labelKey: 'config.options.tax_type.none' },
  { value: 0, label: '0%' },
  { value: 5, label: '5%' },
  { value: 8, label: '8%' },
  { value: 10, label: '10%' },
];
export const PROVINCE_TYPE_OPTION = [
  { value: 1, label: 'Thành Phố', labelKey: 'config.options.province_type.city' },
  { value: 2, label: 'Tỉnh', labelKey: 'config.options.province_type.province' },
  { value: 3, label: 'Khác', labelKey: 'common.label.other' },
];
export const KPI_CATEGORY_SELECT = {
  KPI_MONTH: 1,
  KPI_PERSON: 2,
  KPI_GROUP: 3,
  KPI_OTHER: 4,
  KPI_YEAR: 5,
};

export const KPI_CATEGORY_OPTION = [
  { value: KPI_CATEGORY_SELECT.KPI_MONTH, label: 'Mục tiêu tháng', labelKey: 'config.options.kpi_category.month' },
  { value: KPI_CATEGORY_SELECT.KPI_PERSON, label: 'Mục tiêu cá nhân', labelKey: 'config.options.kpi_category.person' },
  { value: KPI_CATEGORY_SELECT.KPI_GROUP, label: 'Mục tiêu nhóm', labelKey: 'config.options.kpi_category.group' },
  { value: KPI_CATEGORY_SELECT.KPI_OTHER, label: 'Mục tiêu khác', labelKey: 'config.options.kpi_category.other' },
  { value: KPI_CATEGORY_SELECT.KPI_YEAR, label: 'Mục tiêu năm', labelKey: 'config.options.kpi_category.year' },
];

export const DEFAULT_TIMEZONE = 'Asia/Ho_Chi_Minh';

export const TIMEZONE_OPTIONS = [
  { label: '(UTC-12:00) Baker Island', value: 'Etc/GMT+12' },
  { label: '(UTC-11:00) Pago Pago', value: 'Pacific/Pago_Pago' },
  { label: '(UTC-10:00) Honolulu', value: 'Pacific/Honolulu' },
  { label: '(UTC-09:00) Anchorage', value: 'America/Anchorage' },
  { label: '(UTC-08:00) Los Angeles', value: 'America/Los_Angeles' },
  { label: '(UTC-07:00) Denver', value: 'America/Denver' },
  { label: '(UTC-06:00) Chicago', value: 'America/Chicago' },
  { label: '(UTC-05:00) New York', value: 'America/New_York' },
  { label: '(UTC-04:00) Santiago', value: 'America/Santiago' },
  { label: '(UTC-03:00) São Paulo', value: 'America/Sao_Paulo' },
  { label: '(UTC-02:00) South Georgia', value: 'Atlantic/South_Georgia' },
  { label: '(UTC-01:00) Azores', value: 'Atlantic/Azores' },
  { label: '(UTC+00:00) London', value: 'Europe/London' },
  { label: '(UTC+01:00) Paris', value: 'Europe/Paris' },
  { label: '(UTC+02:00) Cairo', value: 'Africa/Cairo' },
  { label: '(UTC+03:00) Moscow', value: 'Europe/Moscow' },
  { label: '(UTC+03:30) Tehran', value: 'Asia/Tehran' },
  { label: '(UTC+04:00) Dubai', value: 'Asia/Dubai' },
  { label: '(UTC+04:30) Kabul', value: 'Asia/Kabul' },
  { label: '(UTC+05:00) Karachi', value: 'Asia/Karachi' },
  { label: '(UTC+05:30) Kolkata', value: 'Asia/Kolkata' },
  { label: '(UTC+05:45) Kathmandu', value: 'Asia/Kathmandu' },
  { label: '(UTC+06:00) Dhaka', value: 'Asia/Dhaka' },
  { label: '(UTC+06:30) Yangon', value: 'Asia/Yangon' },
  { label: '(UTC+07:00) Hồ Chí Minh', value: 'Asia/Ho_Chi_Minh' },
  { label: '(UTC+07:00) Bangkok', value: 'Asia/Bangkok' },
  { label: '(UTC+07:00) Jakarta', value: 'Asia/Jakarta' },
  { label: '(UTC+08:00) Singapore', value: 'Asia/Singapore' },
  { label: '(UTC+08:00) Taipei', value: 'Asia/Taipei' },
  { label: '(UTC+08:00) Hong Kong', value: 'Asia/Hong_Kong' },
  { label: '(UTC+08:00) Shanghai', value: 'Asia/Shanghai' },
  { label: '(UTC+08:00) Perth', value: 'Australia/Perth' },
  { label: '(UTC+09:00) Tokyo', value: 'Asia/Tokyo' },
  { label: '(UTC+09:00) Seoul', value: 'Asia/Seoul' },
  { label: '(UTC+09:30) Adelaide', value: 'Australia/Adelaide' },
  { label: '(UTC+10:00) Sydney', value: 'Australia/Sydney' },
  { label: '(UTC+11:00) Noumea', value: 'Pacific/Noumea' },
  { label: '(UTC+12:00) Auckland', value: 'Pacific/Auckland' },
  { label: '(UTC+13:00) Tongatapu', value: 'Pacific/Tongatapu' },
];

// Ngày trong tuần — value = weekday số theo chuẩn BE (0=CN..6=T7), dùng cho weekly_rest_days (company)
export const WEEKDAY_OPTIONS: { label: string; value: number; labelKey: string }[] = [
  { value: 0, label: 'Chủ nhật', labelKey: 'common.weekday.sunday' },
  { value: 1, label: 'Thứ Hai', labelKey: 'common.weekday.monday' },
  { value: 2, label: 'Thứ Ba', labelKey: 'common.weekday.tuesday' },
  { value: 3, label: 'Thứ Tư', labelKey: 'common.weekday.wednesday' },
  { value: 4, label: 'Thứ Năm', labelKey: 'common.weekday.thursday' },
  { value: 5, label: 'Thứ Sáu', labelKey: 'common.weekday.friday' },
  { value: 6, label: 'Thứ Bảy', labelKey: 'common.weekday.saturday' },
];

export const DEFAULT_CURRENCY = 'VND';
export const DEFAULT_CURRENCY_SYMBOL = '₫';
export const DEFAULT_CURRENCY_LOCALE = 'vi-VN';

// Các loại tiền tệ phổ biến — value là currency code (ISO 4217)
export const CURRENCY_OPTIONS: {
  label: string;
  value: string;
  symbol: string;
  locale: string;
}[] = [
  { label: 'VND — Việt Nam Đồng (₫)',     value: 'VND', symbol: '₫',   locale: 'vi-VN' },
  { label: 'USD — US Dollar ($)',          value: 'USD', symbol: '$',   locale: 'en-US' },
  { label: 'EUR — Euro (€)',               value: 'EUR', symbol: '€',   locale: 'de-DE' },
  { label: 'JPY — Japanese Yen (¥)',       value: 'JPY', symbol: '¥',   locale: 'ja-JP' },
  { label: 'KRW — Korean Won (₩)',         value: 'KRW', symbol: '₩',   locale: 'ko-KR' },
  { label: 'CNY — Chinese Yuan (¥)',       value: 'CNY', symbol: '¥',   locale: 'zh-CN' },
  { label: 'THB — Thai Baht (฿)',          value: 'THB', symbol: '฿',   locale: 'th-TH' },
  { label: 'SGD — Singapore Dollar (S$)', value: 'SGD', symbol: 'S$',  locale: 'en-SG' },
  { label: 'MYR — Malaysian Ringgit (RM)',value: 'MYR', symbol: 'RM',  locale: 'ms-MY' },
  { label: 'IDR — Indonesian Rupiah (Rp)',value: 'IDR', symbol: 'Rp',  locale: 'id-ID' },
  { label: 'PHP — Philippine Peso (₱)',   value: 'PHP', symbol: '₱',   locale: 'en-PH' },
  { label: 'TWD — Taiwan Dollar (NT$)',   value: 'TWD', symbol: 'NT$', locale: 'zh-TW' },
  { label: 'HKD — Hong Kong Dollar (HK$)',value: 'HKD', symbol: 'HK$', locale: 'zh-HK' },
  { label: 'AUD — Australian Dollar (A$)',value: 'AUD', symbol: 'A$',  locale: 'en-AU' },
  { label: 'GBP — British Pound (£)',     value: 'GBP', symbol: '£',   locale: 'en-GB' },
  { label: 'CAD — Canadian Dollar (C$)', value: 'CAD', symbol: 'C$',  locale: 'en-CA' },
  { label: 'CHF — Swiss Franc (CHF)',     value: 'CHF', symbol: 'CHF', locale: 'de-CH' },
  { label: 'INR — Indian Rupee (₹)',      value: 'INR', symbol: '₹',   locale: 'en-IN' },
  { label: 'AED — UAE Dirham (د.إ)',      value: 'AED', symbol: 'د.إ', locale: 'ar-AE' },
  { label: 'SAR — Saudi Riyal (﷼)',       value: 'SAR', symbol: '﷼',   locale: 'ar-SA' },
];

/** `config_code.parent_table` của mã khách hàng — định dạng có thể do env CUSTOMER_CODE_OPTION (BE) quyết định thay cho dòng cấu hình. */
export const CONFIG_CODE_PARENT_TABLE_CUSTOMER = 'customer';
