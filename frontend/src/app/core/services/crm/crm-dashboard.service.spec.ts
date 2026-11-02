import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { describe, expect, it } from 'vitest';

// ⚠ Đứng TRƯỚC base-api (vòng `auth.service` ⇄ `base-api.service` có sẵn, dưới thứ tự nạp ESM của vitest) — khuôn
// `asset-disposal.service.spec.ts`.
import { AuthService } from '@/core/services/auth/auth.service';
import { SKIP_GLOBAL_403 } from '@/core/interceptors/response-handler.interceptor';
import { CrmDashboardService } from './crm-dashboard.service';

void AuthService;

function setup() {
  TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
  return { svc: TestBed.inject(CrmDashboardService), http: TestBed.inject(HttpTestingController) };
}

describe('CrmDashboardService', () => {
  it('query rỗng ⇒ URL không có tham số', () => {
    const { svc, http } = setup();
    svc.paymentInMonth({}).subscribe();
    const reqs = http.match(() => true);
    expect(reqs).toHaveLength(1);
    for (const r of reqs) expect(r.request.params.keys()).toEqual([]);
    expect(reqs[0].request.url).toMatch(/crm-dashboard\/payment-in-month$/);
  });

  it('gửi from/to/branch_id qua HttpParams (bản cũ truyền sai chỗ nên KHÔNG BAO GIỜ gửi)', () => {
    const { svc, http } = setup();
    svc.appointmentAcceptedStats({ from: 100, to: 200, branch_id: 3 }).subscribe();
    const r = http.expectOne((req) => req.url.endsWith('crm-dashboard/appointment-accepted-stats'));
    expect(r.request.params.get('from')).toBe('100');
    expect(r.request.params.get('to')).toBe('200');
    expect(r.request.params.get('branch_id')).toBe('3');
  });

  it('bỏ khoá rỗng/0 — chỉ chi nhánh ⇒ chỉ branch_id', () => {
    const { svc, http } = setup();
    svc.quickStats({ branch_id: 7, from: 0, to: undefined }).subscribe();
    const r = http.expectOne((req) => req.url.endsWith('crm-dashboard/quick-starts'));
    expect(r.request.params.keys()).toEqual(['branch_id']);
  });

  it('403 là lỗi của khối ⇒ tắt toast toàn cục (SKIP_GLOBAL_403) cho mọi endpoint', () => {
    const { svc, http } = setup();
    svc.paymentInMonth().subscribe();
    svc.overviewChartSource({ branch_id: 2 }).subscribe();
    for (const r of http.match(() => true)) expect(r.request.context.get(SKIP_GLOBAL_403)).toBe(true);
  });
});
