import { HttpClient } from '@angular/common/http';
import { TranslateLoader } from '@ngx-translate/core';
import { Observable, of, tap } from 'rxjs';

import viVN from '@assets/i18n/vi-VN.json';

const LS_KEY_PREFIX = 'i18n_';
// Auto-injected bởi scripts/inject-version.js khi prebuild (format: "<pkg.version>.<timestamp>").
// Mỗi build → version mới → localStorage cache cũ tự invalidate, user nhận JSON mới ngay lần load đầu.
const LS_VERSION = '2.2.1793620062216';

// Inline fallback cho default lang — đảm bảo translations luôn có ngay lập tức
// kể cả khi localStorage trống (incognito, fresh install). HTTP fetch chạy nền để
// update cache cho lần reload tiếp theo (translator workflow vẫn hoạt động).
const INLINE_FALLBACKS: Record<string, any> = {
  'vi-VN': viVN,
};

interface CachedEntry {
  v: string; // version
  t: number; // timestamp
  d: any;    // data (translation map)
}

// Stale-while-revalidate: trả localStorage ngay lập tức, fetch HTTP nền
// để update cache cho lần load sau. Giúp:
//  - Cold-start subsequent: 0 network blocking, render i18n instant
//  - Translators update JSON: thấy trong lần reload tiếp theo
//  - Offline: vẫn hoạt động được
export class CachedTranslateLoader implements TranslateLoader {
  constructor(
    private http: HttpClient,
    private prefix: string = './assets/i18n/',
    private suffix: string = '.json',
  ) {}

  getTranslation(lang: string): Observable<any> {
    const lsKey = LS_KEY_PREFIX + lang;
    const cached = this.readCache(lsKey);

    // ?v=<LS_VERSION> → URL đổi mỗi build, nên file JSON có thể cache `immutable` ở nginx:
    // cold-load lần 2 trở đi tốn 0 byte thay vì tải lại ~109 KB gzip mỗi lần (đo 2026-08-28).
    // ⚠ Đánh đổi: sửa JSON mà KHÔNG rebuild FE thì client không thấy — quy trình dịch phải đi kèm build
    // (prebuild `inject-version.js` tự sinh version mới mỗi lần build nên bình thường không phải lo).
    const fetch$ = this.http.get(`${this.prefix}${lang}${this.suffix}?v=${LS_VERSION}`).pipe(
      tap((data) => this.writeCache(lsKey, data)),
    );

    if (cached) {
      // Background revalidate (fire & forget) — không block render với cached data
      fetch$.subscribe({
        next: () => {},
        error: () => {},
      });
      return of(cached);
    }

    // Cache miss (incognito / fresh install): trả inline ngay nếu có,
    // đồng thời fetch HTTP nền để update cache cho lần reload sau.
    const inline = INLINE_FALLBACKS[lang];
    if (inline) {
      fetch$.subscribe({ next: () => {}, error: () => {} });
      return of(inline);
    }

    return fetch$;
  }

  private readCache(key: string): any | null {
    try {
      const raw = localStorage.getItem(key);
      if (!raw) return null;
      const entry = JSON.parse(raw) as CachedEntry;
      if (entry.v !== LS_VERSION) {
        localStorage.removeItem(key);
        return null;
      }
      return entry.d;
    } catch {
      return null;
    }
  }

  private writeCache(key: string, data: any): void {
    try {
      const entry: CachedEntry = { v: LS_VERSION, t: Date.now(), d: data };
      localStorage.setItem(key, JSON.stringify(entry));
    } catch {
      // localStorage quota exceeded → silent fail, sẽ fetch HTTP lần sau
    }
  }
}
