// ─── Tài liệu hướng dẫn ──────────────────────────────────────────────────────
// Link "Hướng dẫn sử dụng" trên AppTopbar mở đúng bài của màn hình đang đứng.
// Docs là Docusaurus: tiếng Việt ở gốc `/docs/...`, tiếng Anh ở `/en/docs/...`
// (D5 chỉ vi-VN + en-US — story-22).
// Docs thêm bài mới → thêm dòng vào DOCS_ROUTE_MAP; route không khớp dòng nào → trang giới thiệu.
// Self-host: đổi DOCS_BASE_URL sang site docs của bạn (để rỗng ⇒ ẩn link — xem topbar).

// RỖNG = chưa có site docs ⇒ topbar ẨN nút "Hướng dẫn" (O-5). Điền URL thật khi có.
export const DOCS_BASE_URL = '';

/** Docs chỉ coi là đã cấu hình khi URL thật (không rỗng, không placeholder example.*). */
export function isDocsConfigured(baseUrl: string = DOCS_BASE_URL): boolean {
  const u = baseUrl.trim();
  return u !== '' && !/^https?:\/\/([^/]*\.)?example\.(com|org|net)(\/|$)/i.test(u);
}

/** Ngôn ngữ app có bản docs ở GỐC site (không tiền tố). */
export const DOCS_ROOT_LANG = 'vi-VN';
/** Tiền tố locale cho mọi ngôn ngữ khác — hiện docs chỉ có thêm bản tiếng Anh. */
export const DOCS_FALLBACK_LANG_PREFIX = '/en';

/** Bài mặc định khi route không khớp prefix nào. */
export const DOCS_DEFAULT_PATH = 'intro';

/**
 * Prefix route app → đường dẫn bài docs (sau `/docs/`).
 * Khớp theo RANH GIỚI SEGMENT và lấy prefix DÀI NHẤT: `/config/service` KHÔNG ăn
 * `/config/service-type` (xem resolveDocsPath).
 */
export const DOCS_ROUTE_MAP: Record<string, string> = {
  // CRM
  '': 'crm',
  '/customer': 'crm/khach-hang/tao-moi-khach-hang',
  '/customer/detail': 'crm/khach-hang/chi-tiet-khach-hang',
  '/appointment': 'crm/lich-hen/cac-cach-xem-lich-hen',
  '/reminder': 'crm/khach-hang/tao-loi-nhac',

  // Cấu hình
  '/config': 'config',
  '/config/company': 'config/he-thong/thuong-hieu',
  '/config/branch': 'config/he-thong/chi-nhanh',
  '/config/unit': 'config/he-thong/don-vi-tinh',
  '/config/option': 'config/he-thong/lua-chon-tac-vu',
  '/config/customer-source': 'config/marketing/nguon-khach-hang',
  '/config/group-source': 'config/marketing/nhom-nguon-khach-hang',
  '/config/service': 'config/phong-kham/dich-vu-san-pham',
  '/config/service-type': 'config/phong-kham/nhom-dich-vu-san-pham',
  '/config/form-customer': 'config/customize-form/form-customer',
  '/config/form-lead': 'config/customize-form/form-lead',
};

/** Đường dẫn bài docs cho 1 URL app (đã hoặc chưa bỏ query/fragment). */
export function resolveDocsPath(appUrl: string): string {
  const path = appUrl.split(/[?#]/)[0].replace(/\/+$/, '');
  let best: string | null = null;
  for (const prefix of Object.keys(DOCS_ROUTE_MAP)) {
    // '' = dashboard ở gốc: chỉ khớp ĐÚNG '/', không làm prefix cho mọi route.
    const matches = path === prefix || (prefix !== '' && path.startsWith(prefix + '/'));
    if (matches && (best === null || prefix.length > best.length)) best = prefix;
  }
  return best !== null ? DOCS_ROUTE_MAP[best] : DOCS_DEFAULT_PATH;
}

/** URL đầy đủ tới bài docs theo route + ngôn ngữ app hiện tại. */
export function buildDocsUrl(appUrl: string, lang: string | undefined): string {
  const langPrefix = !lang || lang === DOCS_ROOT_LANG ? '' : DOCS_FALLBACK_LANG_PREFIX;
  return `${DOCS_BASE_URL}${langPrefix}/docs/${resolveDocsPath(appUrl)}`;
}
