import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { describe, expect, it } from 'vitest';

// ⚠ Đứng TRƯỚC base-api (vòng `auth.service` ⇄ `base-api.service` có sẵn) — khuôn crm-dashboard.service.spec.ts.
import { AuthService } from '@/core/services/auth/auth.service';
import { SKIP_GLOBAL_403, SKIP_GLOBAL_404 } from '@/core/interceptors/response-handler.interceptor';
import { CrmDashboardLayoutService } from './crm-dashboard-layout.service';
import { CrmDashboardService } from './crm-dashboard.service';

void AuthService;

function setup() {
  TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
  return {
    layout: TestBed.inject(CrmDashboardLayoutService),
    data: TestBed.inject(CrmDashboardService),
    http: TestBed.inject(HttpTestingController),
  };
}

// Sự cố 2026-10-19: BE dev chạy binary cũ chưa có /layout ⇒ GET 404 ⇒ interceptor đá cả trang chủ sang
// /auth/notfound. Lỗi đọc bố cục / dữ liệu khối phải được xử lý TẠI dashboard (store rơi về mặc định,
// khối hiện lỗi + Thử lại), không bao giờ điều hướng toàn trang.
describe('Dashboard — lỗi API không điều hướng toàn trang', () => {
  it('getLayout tắt chuyển trang khi 404 và 403', () => {
    const { layout, http } = setup();
    layout.getLayout().subscribe({ error: () => undefined });
    const r = http.expectOne((req) => req.url.endsWith('crm-dashboard/layout'));
    expect(r.request.context.get(SKIP_GLOBAL_404)).toBe(true);
    expect(r.request.context.get(SKIP_GLOBAL_403)).toBe(true);
  });

  it('request dữ liệu khối tắt chuyển trang khi 404 và 403', () => {
    const { data, http } = setup();
    data.paymentInMonth().subscribe({ error: () => undefined });
    const r = http.expectOne((req) => req.url.endsWith('crm-dashboard/payment-in-month'));
    expect(r.request.context.get(SKIP_GLOBAL_404)).toBe(true);
    expect(r.request.context.get(SKIP_GLOBAL_403)).toBe(true);
  });
});
