import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

/**
 * Khoá cấu nối story-09 bằng QUÉT MÃ NGUỒN thật (component PrimeNG nặng — dựng TestBed treo CPU, xem
 * executor-contract §2): đọc đúng file sản phẩm, bóc comment rồi khớp. Có test đối chứng "file tồn tại"
 * để lưới không xanh giả khi đổi tên file.
 */
// vitest luôn chạy từ `frontend/` (jsdom: import.meta.url không phải scheme file ⇒ không dùng được).
const ROOT = resolve(process.cwd(), 'src/app') + '/';
const FILES = {
  detailHtml: 'modules/crm/customer/customer-detail/customer-detail.html',
  detailTs: 'modules/crm/customer/customer-detail/customer-detail.ts',
  contactListTs: 'modules/crm/customer/contact-list/contact-list.ts',
  contactListHtml: 'modules/crm/customer/contact-list/contact-list.html',
  paymentHtml: 'modules/crm/payment/list/detail-customer/payment-of-customer.html',
  paymentTs: 'modules/crm/payment/list/detail-customer/payment-of-customer.ts',
  service: 'core/services/crm/customer.service.ts',
  model: 'core/models/crm/customer.model.ts',
  changer: 'modules/crm/customer/customer-detail/customer-stage-changer.ts',
  timelineTs: 'modules/crm/customer/timeline/timeline.ts',
  timelineHtml: 'modules/crm/customer/timeline/timeline.html',
};

function stripComments(src: string): string {
  return src
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:])\/\/.*$/gm, '$1');
}

function read(key: keyof typeof FILES): string {
  return stripComments(readFileSync(ROOT + FILES[key], 'utf8'));
}

describe('story-09 — cấu nối màn khách hàng theo bước cấu hình', () => {
  it('đối chứng: mọi file được quét đều tồn tại', () => {
    for (const rel of Object.values(FILES)) expect(existsSync(ROOT + rel), rel).toBe(true);
  });

  it('AC5: nút sửa ở chi tiết khách chỉ gate theo quyền, không còn canShowUpdateBtn', () => {
    const html = read('detailHtml');
    expect(html).toContain('*hasPerm="permission.permissionUpdateCustomer"');
    expect(html).not.toContain('canShowUpdateBtn');
    expect(read('detailTs')).not.toContain('canShowUpdateBtn');
    expect(read('detailTs')).not.toMatch(/LEAD_LIKE_STATUSES|CONVERTED_CUSTOMER_STATUSES|CUSTOMER_STATUS/);
  });

  it('AC5: nút tạo phiếu thu chỉ gate theo quyền, không còn đọc customer.status', () => {
    const html = read('paymentHtml');
    expect(html).toContain('*hasPerm="permission.permissionCreatePayment"');
    expect(html).not.toMatch(/customer\.status|CUSTOMER_STATUS/);
    expect(read('paymentTs')).not.toContain('CUSTOMER_STATUS');
  });

  it('AC6: badge chi nhánh/SĐT không còn mượn class statusCustomer-*', () => {
    const html = read('detailHtml');
    expect(html).not.toContain('statusCustomer-');
    expect(html).toContain('badge-neutral');
    expect(html).toContain('badge-positive');
  });

  it('AC2: badge tiến trình ở chi tiết là ô đổi bước nhận customerId/stageId', () => {
    const html = read('detailHtml');
    expect(html).toContain('<app-customer-stage-changer');
    expect(html).toMatch(/\[stageId\]="m\.stage_id"/);
    expect(html).toContain('(changed)="onStageChanged($event)"');
  });

  it('AC2/AC3: ô đổi bước chỉ gọi changeStage khi shouldChangeStage, và gate bằng quyền sửa khách', () => {
    const ts = read('changer');
    expect(ts).toMatch(/shouldChangeStage\(\s*this\.stageId\(\)\s*,\s*id\s*\)/);
    expect(ts).toMatch(/changeStage\(\s*this\.customerId\(\)\s*,\s*id\s*\)/);
    expect(ts).toContain('PERMISSION_SELECT.permissionUpdateCustomer');
  });

  it('AC1: danh sách lọc theo stage_id (không còn _status) và hiển thị tên + class màu bước', () => {
    const ts = read('contactListTs');
    expect(ts).toMatch(/\{\s*name\s*:\s*'stage_id'\s*,\s*kind\s*:\s*'in'\s*\}/);
    expect(ts).not.toMatch(/_status|CUSTOMER_STATUS/);
    const html = read('contactListHtml');
    expect(html).toContain('formControlName="stage_id"');
    expect(html).toContain('model.stageColorClass');
    expect(html).toContain('model.stageName');
    expect(html).not.toMatch(/statusClass|statusKey|statusLabel/);
  });

  it('service: changeStage PUT /:id/stage; code chết đã xoá; không còn đọc item.status', () => {
    const ts = read('service');
    expect(ts).toContain('`${this.path}/${id}/stage`');
    expect(ts).toMatch(/\{\s*stage_id\s*:\s*stageId\s*\}/);
    expect(ts).not.toMatch(/getCustomerDid|getPortraitStatus|get-customer-with-status|statusCustomer-/);
    expect(ts).not.toMatch(/item\.status/);
  });

  it('AC7: CustomerModel có stage_id, không còn is_patient', () => {
    const ts = read('model');
    expect(ts).toMatch(/stage_id\?: number/);
    expect(ts).not.toContain('is_patient');
  });
  // ── Mối nối (QA pass 1 P1): khớp CẢ biểu thức nối, không chuỗi rời ──────────────────────────────

  // Regex chịu định dạng: `\s*` quanh ngoặc/dấu phẩy/hai chấm, KHÔNG ép kiểu `as …` (prettier/đổi kiểu không làm đỏ giả).
  it('AC2 mối nối: changeStage gọi PUT (không phải POST) tới /:id/stage', () => {
    expect(read('service')).toMatch(
      /\.put<[^(]*>\(\s*`\$\{this\.path\}\/\$\{id\}\/stage`\s*,\s*\{\s*stage_id\s*:\s*stageId\s*\}\s*\)/,
    );
  });

  it('AC2/3 mối nối: ô đổi bước chỉ gọi API khi shouldChangeStage, chỉ tương tác khi canChange() = quyền sửa khách', () => {
    const ts = read('changer');
    expect(ts).toMatch(/shouldChangeStage\(\s*this\.stageId\(\)\s*,\s*id\s*\)/);
    expect(ts).toMatch(/\.changeStage\(\s*this\.customerId\(\)\s*,\s*id\s*\)/);
    expect(ts).toMatch(/@if\s*\(\s*canChange\(\s*\)\s*\)\s*\{/);
    expect(ts).toMatch(
      /canChange\s*=\s*computed(?:<[^>]*>)?\(\s*\(\s*\)\s*=>\s*\{?\s*(?:return\s+)?hasPermission\(\s*PERMISSION_SELECT\.permissionUpdateCustomer\s*\)/,
    );
  });

  it('AC2 mối nối: chỉ emit changed sau khi API trả về, kèm đúng payload', () => {
    expect(read('changer')).toMatch(
      /\.subscribe\(\s*\{\s*next\s*:\s*\(\s*res\s*\)\s*=>\s*\{\s*if\s*\(\s*res\?\.data\s*\)\s*\{?\s*this\.changed\.emit\(\s*res\.data\s*\)/,
    );
  });

  it('AC2 mối nối: onStageChanged gọi applyStageChange, bỏ qua khi không đổi, rồi set model đã gắn nhãn', () => {
    const ts = read('detailTs');
    expect(ts).toMatch(/const\s+merged\s*=\s*applyStageChange\(\s*current\s*,\s*result\s*\)/);
    expect(ts).toMatch(/if\s*\(\s*!merged\s*\|\|\s*merged\s*===\s*current\s*\)\s*return/);
    expect(ts).toMatch(/this\.model\.set\(\s*this\.modelService\.addLabel\(\s*merged\s*\)/);
  });

  it('AC4 mối nối: timeline gắn stageView từ toStageChangeView và template render theo stageView', () => {
    expect(read('timelineTs')).toMatch(/stageView\s*:\s*toStageChangeView\(\s*item\s*\)\s*,/);
    const html = read('timelineHtml');
    expect(html).toMatch(/@if\s*\(\s*event\.stageView\s*;\s*as\s+sv\s*\)\s*\{/);
    expect(html).toMatch(/stageChangeTime\(\s*event\.day\s*\)/);
  });
});
