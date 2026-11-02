/* =========================
 *  Cluvix Custom Service Worker (API = network-only)
 * ========================= */

// Giá trị này do prebuild `scripts/inject-version.js` ghi đè mỗi lần build (regex đòi hằng chuỗi
// kết thúc bằng dấu ; ngay sau nháy). TRƯỚC 2026-08-28 dòng này là `'2.2.' + Date.now()` nên regex
// KHÔNG khớp — replace im lặng không làm gì, và SW_VERSION bị tính lại MỖI LẦN service worker
// khởi động ⇒ STATIC_CACHE đổi tên liên tục ⇒ cache tĩnh không bao giờ dùng lại được.
// ⚠ Đừng thêm biểu thức vào sau chuỗi — làm vậy là tái hiện đúng bug đó.
const SW_VERSION = '2.2.1793620062216';
const DOMAIN_KEY = self.location.hostname.replace(/\./g, '-');
const STATIC_CACHE = `cluvix:${DOMAIN_KEY}:static:${SW_VERSION}`;
const ASSET_CACHE = `cluvix:${DOMAIN_KEY}:assets:v1`; // không gắn SW_VERSION → giữ cache assets qua các lần deploy

const ASSET_TTL_MS = 300_000; // 5 phút — giảm revalidate frequency cho asset tĩnh
const assetTouched = new Map(); // url -> lastFetchTs
const MAX_CACHE_ENTRIES = 300; // Giới hạn cache entries

/* ---------- Icons & manifest (prefetch) ---------- */
const ICON_PREFETCH = [
  '/assets/icons/apple-icon-180.png',
  '/assets/icons/favicon.svg',
  '/assets/icons/favicon-16x16.png',
  '/assets/icons/favicon-32x32.png',
  '/assets/icons/icon-192.png',
  '/assets/icons/icon-512.png',
  '/assets/icons/maskable-192.png',
  '/assets/icons/maskable-512.png',
  '/assets/icons/safari-pinned-tab.svg',
  '/manifest.webmanifest',
];

/* ---------- Static pre-cache ---------- */
const STATIC_ASSETS = [
  '/', // SPA shell
  '/index.html',
  '/assets/logo/icon.png',
  '/assets/logo/logo-ngang.png',
  ...ICON_PREFETCH,
];

/* ---------- Request classification ---------- */
const ORIGIN = self.location.origin;

const isApiRequest = (urlObj) =>
  urlObj.origin === ORIGIN && urlObj.pathname.startsWith('/api/');

/* ---------- S3 rule ---------- */
// ⚠ Repo public để RỖNG (không lộ CDN Cluvix). Khi build, scripts/inject-sw-origin.js thay giá trị
// này trong FILE OUTPUT theo biến môi trường SW_S3_ORIGIN (PHẢI khớp S3_CDN của backend/.env).
// SW là plain JS ngoài build Angular nên không đọc được environment.ts.
// ⚠ startsWith('') luôn true ⇒ phải chặn origin rỗng, không thì MỌI URL bị coi là S3.
const S3_ORIGIN = '';
const isS3Url = (href) => !!S3_ORIGIN && href.startsWith(S3_ORIGIN);
// Key license PrimeUI là CONFIG, KHÔNG phải asset → luôn đi thẳng mạng.
// Nó nằm trên S3_ORIGIN nên nếu không loại trừ ở đây sẽ rơi vào nhánh SWR bên dưới:
// (1) bị phục vụ bản cache cũ tới 5 phút sau khi đã gia hạn key trên CDN, và
// (2) cache-buster theo ngày làm ASSET_CACHE đẻ thêm 1 entry MỖI NGÀY, ăn dần
//     hạn mức MAX_CACHE_ENTRIES=300 vốn dùng chung với ảnh/js thật.
// ⚠ Khớp theo TÊN FILE, KHÔNG theo thư mục: URL đầy đủ được khai ở
// src/app/define/primeng.define.ts (PRIMEUI_LICENSE_CDN_URL) — file đó và file này KHÔNG
// import được nhau (SW là plain JS ngoài build Angular). Match theo path cứng thì đổi thư
// mục trên CDN mà quên sửa đây sẽ hỏng ÂM THẦM: license rơi lại vào nhánh SWR bên dưới,
// stale 5 phút + đẻ 1 entry/ngày trong ASSET_CACHE (giới hạn 300, dùng chung với ảnh/js).
// Đổi tên file license ⇒ PHẢI sửa cả 2 nơi.
const LICENSE_FILE = 'primeui-license.json';
const isLicenseConfig = (urlObj) =>
  isS3Url(urlObj.href) && urlObj.pathname.endsWith('/' + LICENSE_FILE);
const HEAVY_EXT = /\.(jpg|jpeg|webp)$/i;
const isS3CacheablePath = (pathname) => !HEAVY_EXT.test(pathname);

/* ---------- Hashed asset (immutable) ---------- */
// Angular build output: chunk-XXXXXXXX.js, chunk-XXXXXXXX.css, main-XXXXXXXX.js
const HASHED_ASSET = /[.-][A-Za-z0-9]{8,}\.(js|css)$/;
const isHashedAsset = (pathname) => HASHED_ASSET.test(pathname);

/* ---------- Non-SPA navigate paths (file download, export) ---------- */
// Các path này không phải Angular route → không serve index.html, để browser xử lý tự nhiên
const NON_SPA_PREFIX = ['/download/'];
const NON_SPA_EXT = /\.(xlsx|xls|csv|pdf|zip|doc|docx|rar|7z)$/i;
// Trang tĩnh độc lập, KHÔNG phải Angular route — iframe load cũng là mode 'navigate'. Thiếu dòng này thì
// trình duyệt đã cài SW (đã từng đăng nhập app) mở widget livechat trên website khách sẽ nhận index.html
// của Angular thay vì widget.html → router hiện "trang không tồn tại" trong khung chat (2026-09-04).
const NON_SPA_EXACT = ['/widget.html'];
const isNonSpaNavigate = (pathname) =>
  NON_SPA_EXACT.includes(pathname) ||
  NON_SPA_PREFIX.some((p) => pathname.startsWith(p)) ||
  NON_SPA_EXT.test(pathname);

/* ---------- Timeout fetch (chỉ dùng cho asset) ---------- */
const timeoutFetch = (request, timeout = 2000) =>
  Promise.race([
    fetch(request),
    new Promise((_, reject) => setTimeout(() => reject(new Error('timeout')), timeout)),
  ]);

/* =============== INSTALL =============== */
self.addEventListener('install', (event) => {
  self.skipWaiting();
  event.waitUntil(
    (async () => {
      const cache = await caches.open(STATIC_CACHE);
      // Parallel prefetch, don't block on individual failures
      await Promise.allSettled(
        STATIC_ASSETS.map(async (url) => {
          try {
            const res = await fetch(url, { cache: 'reload' });
            if (res.ok) await cache.put(url, res);
          } catch { /* skip */ }
        }),
      );
    })(),
  );
});

/* =============== ACTIVATE =============== */
self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      // Stale cache = cluvix:* khác STATIC_CACHE/ASSET_CACHE hiện tại → có nghĩa là SW cũ đang bị thay
      const staleKeys = keys.filter(
        (key) =>
          key !== STATIC_CACHE && key !== ASSET_CACHE && key.startsWith(`cluvix:${DOMAIN_KEY}:`),
      );
      const isUpdate = staleKeys.length > 0;
      await Promise.all(staleKeys.map((key) => caches.delete(key)));
      await self.clients.claim();
      // Chỉ post SW_UPDATED khi thực sự là update, không phải first install
      // (tránh trigger reload thừa lần đầu user vào trang)
      if (isUpdate) {
        const clients = await self.clients.matchAll();
        clients.forEach((client) =>
          client.postMessage({ type: 'SW_UPDATED', version: SW_VERSION }),
        );
      }
    })(),
  );
});

/* =============== FETCH =============== */
self.addEventListener('fetch', (event) => {
  const req = event.request;

  // ❌ Skip non-http(s) sớm nhất (chrome-extension, devtools, blob...)
  if (!req.url.startsWith('http')) return;

  const url = new URL(req.url);

  // ✅ API → NETWORK-ONLY (mọi method)
  if (isApiRequest(url)) return;

  // ✅ License config → NETWORK-ONLY (xem isLicenseConfig)
  if (isLicenseConfig(url)) return;

  // ❌ Non-GET không can thiệp
  if (req.method !== 'GET') return;

  // ❌ Cross-origin (trừ S3)
  const allowCross = isS3Url(url.href);
  if (url.origin !== ORIGIN && !allowCross) return;

  // ✅ Hashed chunk/asset (có hash trong tên) → cache-first (immutable)
  if (isHashedAsset(url.pathname)) {
    event.respondWith(cacheFirstImmutable(req));
    return;
  }

  // ✅ Asset tĩnh & S3 → SWR
  // i18n JSON cũng được cache qua SWR — eliminates re-fetch 108KB mỗi page load.
  // Translators update JSON sẽ propagate qua revalidate cycle (TTL 5 min).
  const isI18n = url.pathname.startsWith('/assets/i18n/') && url.pathname.endsWith('.json');
  const isStatic =
    ['script', 'style', 'image', 'font', 'audio'].includes(req.destination) ||
    url.pathname.endsWith('.css') ||
    url.pathname.endsWith('.js') ||
    isI18n;
  const shouldCacheS3Asset = allowCross && isS3CacheablePath(url.pathname);

  if (isStatic || shouldCacheS3Asset) {
    event.respondWith(staleWhileRevalidate(req, event));
    return;
  }

  // 🌐 SPA navigation fallback
  // Luôn fetch /index.html thay vì fetch(req) vì:
  // fetch(req) đổi Sec-Fetch-Mode từ "navigate" → "same-origin" → server auth middleware
  // có thể redirect deep routes (/crm/xxx) về /auth/accessdenied thay vì serve index.html.
  // Angular router đọc URL từ window.location (browser-managed), không từ response URL,
  // nên serve index.html cho mọi SPA route là đúng.
  //
  // Exception: non-SPA navigate (file download/export) → pass-through, không can thiệp.
  if (req.mode === 'navigate' && isNonSpaNavigate(url.pathname)) return;

  if (req.mode === 'navigate') {
    event.respondWith(
      (async () => {
        try {
          const response = await fetch('/index.html');
          if (response.ok) {
            const cache = await caches.open(STATIC_CACHE);
            cache.put('/index.html', response.clone()).catch(() => {});
            return response;
          }
        } catch { /* network error → serve cache */ }
        const cache = await caches.open(STATIC_CACHE);
        const cached = await cache.match('/index.html');
        if (cached) return cached;
        return new Response(
          '<!DOCTYPE html><html><head><title>Offline</title></head><body><h1>App đang offline</h1></body></html>',
          { headers: { 'Content-Type': 'text/html' } },
        );
      })(),
    );
    return;
  }
});

/* =========================
 *  Cache-first for immutable hashed assets
 *  (chunk-ABCD1234.js sẽ không bao giờ thay đổi nội dung)
 * ========================= */
async function cacheFirstImmutable(request) {
  const cache = await caches.open(ASSET_CACHE);
  const cached = await cache.match(request);
  if (cached) return cached;

  try {
    const res = await fetch(request);
    if (res.ok && (res.type === 'basic' || res.type === 'cors')) {
      cache.put(request, res.clone()).catch(() => {});
      trimCache(cache);
    } else if (res.status === 404) {
      // Chunk cũ bị xóa → version mới deploy, báo tất cả tab reload
      const clients = await self.clients.matchAll();
      clients.forEach((c) => c.postMessage({ type: 'SW_UPDATED', version: SW_VERSION }));
      return new Response(null, { status: 204 });
    }
    return res;
  } catch {
    return new Response(null, { status: 504 });
  }
}

/* =========================
 *  SWR for non-hashed assets
 * ========================= */
async function staleWhileRevalidate(request, fetchEvent) {
  const cache = await caches.open(ASSET_CACHE);
  const cached = await cache.match(request);

  const now = Date.now();
  const key = request.url;
  const lastTouch = assetTouched.get(key) || 0;
  const shouldRevalidate = !cached || now - lastTouch >= ASSET_TTL_MS;

  if (cached && !shouldRevalidate) return cached;

  const networkPromise = shouldRevalidate
    ? timeoutFetch(request, 2000)
        .then((res) => {
          if (res && res.ok && (res.type === 'basic' || res.type === 'cors')) {
            cache.put(request, res.clone()).catch(() => {});
            assetTouched.set(key, now);
            trimCache(cache);
          } else if (res && res.status === 404) {
            cache.delete(request).catch(() => {});
            assetTouched.delete(key);
          }
          return res;
        })
        .catch(() => null)
    : null;

  if (cached && networkPromise) {
    // Gia hạn SW lifetime cho background revalidate — không để bị kill giữa chừng
    fetchEvent.waitUntil(networkPromise);
    return cached;
  }

  // `networkPromise` LUÔN là Promise ở nhánh này (shouldRevalidate = !cached || ...
  // nên !cached ⇒ true), và `.catch(() => null)` phía trên cho nó quyền resolve ra
  // null. `networkPromise || fallback` kiểm ĐỐI TƯỢNG Promise — luôn truthy — chứ
  // KHÔNG kiểm giá trị nó resolve ra: fallback 504 thành nhánh chết, còn
  // `event.respondWith()` (dòng dispatch SWR) nhận null ⇒ Chrome ném TypeError ra
  // console và request thành network error không lần ngược được về SW.
  // Phải await rồi mới fallback.
  const res = await networkPromise;
  return res || new Response(null, { status: 504 });
}

/* =========================
 *  Trim cache khi quá nhiều entries
 *  Nhận cache object trực tiếp — gọi từ cả hashed và SWR path
 * ========================= */
let trimPending = false;
function trimCache(cache) {
  if (trimPending) return;
  trimPending = true;

  // Debounce: chờ 5s để gom nhiều put rồi trim 1 lần
  setTimeout(async () => {
    trimPending = false;
    try {
      const keys = await cache.keys();
      if (keys.length <= MAX_CACHE_ENTRIES) return;

      // Xóa entries theo thứ tự keys() (cũ nhất trước) cho đến khi đủ limit
      const toDelete = keys.length - MAX_CACHE_ENTRIES;
      await Promise.all(
        keys.slice(0, toDelete).map((req) => {
          assetTouched.delete(req.url);
          return cache.delete(req);
        }),
      );
    } catch { /* ignore */ }
  }, 5000);
}
