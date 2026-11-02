import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

/**
 * Khoá cấu nối story-10 (lịch hẹn / báo cáo ghi chú / phễu dashboard / gỡ tiến trình cũ) bằng QUÉT MÃ NGUỒN
 * thật — component PrimeNG nặng, dựng TestBed treo CPU (executor-contract §2). Đọc đúng file sản phẩm, bóc comment
 * rồi khớp. Regex khớp CẢ biểu thức nối (`'statusCustomer-' + x`, `` `statusCustomer-${x}` ``) bằng cách bắt gốc
 * `statusCustomer`, không bắt chuỗi rời có dấu `-`. Có test đối chứng "file tồn tại" + "regex bắt được mã cũ".
 */
// vitest luôn chạy từ `frontend/` (jsdom: import.meta.url không phải scheme file ⇒ không dùng được).
const SRC = resolve(process.cwd(), 'src') + '/';
const APP = SRC + 'app/';
const FILES = {
  apptTs: 'modules/crm/appointment/list/in-menu/appointment-in-menu.ts',
  apptHtml: 'modules/crm/appointment/list/in-menu/appointment-in-menu.html',
  noteTs: 'modules/crm/report/note/note-list/note-list.ts',
  noteHtml: 'modules/crm/report/note/note-list/note-list.html',
  noteService: 'core/services/crm/note.service.ts',
  noteModel: 'core/models/crm/note.model.ts',
  customerModel: 'core/models/crm/customer.model.ts',
  customerDefine: 'define/customer.define.ts',
  funnelTs: 'modules/crm/dashboard/component/customer-status-chart-overview.ts',
  widgets: 'modules/crm/dashboard/shell/dashboard-widgets.ts',
  presets: 'modules/crm/dashboard/shell/layout-presets.ts',
};
const SCSS = 'assets/overrides/custom.scss';
const I18N = ['assets/i18n/vi-VN.json', 'assets/i18n/en-US.json'];

function stripComments(src: string): string {
  return src
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:])\/\/.*$/gm, '$1');
}

function read(key: keyof typeof FILES): string {
  return stripComments(readFileSync(APP + FILES[key], 'utf8'));
}

/** Gốc từ khoá tiến trình cũ — không kèm `-`/`(` để bắt luôn dạng nối chuỗi/template literal. */
// `statusCustomer(?![A-Za-z])` loại tên hàm `statusCustomerOverview` (endpoint phễu, được giữ) nhưng vẫn bắt `statusCustomer-`, `statusCustomer${…}`, `'statusCustomer' + …`.
const OLD_PROGRESS = /CUSTOMER_STATUS|getCustomerStatus|statusCustomer(?![A-Za-z])|define\.customer_status|is_patient/;

function walk(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else out.push(p);
  }
  return out;
}

describe('story-10 — gỡ tiến trình cũ, dùng bước cấu hình', () => {
  it('đối chứng: mọi file được quét đều tồn tại; regex bắt được mã tiến trình cũ', () => {
    for (const rel of Object.values(FILES)) expect(existsSync(APP + rel), rel).toBe(true);
    expect(existsSync(SRC + SCSS)).toBe(true);
    for (const old of [
      'export const CUSTOMER_STATUS = {',
      'getCustomerStatusLabel(x)',
      "'statusCustomer-' + item.status",
      '`statusCustomer-${item.customer?.status}`',
      "labelKey: 'define.customer_status.zalo'",
      'is_patient: boolean',
    ]) {
      expect(OLD_PROGRESS.test(old), old).toBe(true);
    }
  });

  it('AC1: lịch hẹn lọc `_stage_id` (nhiều giá trị) bằng control bước, không còn `_status`/p-select cũ', () => {
    const ts = read('apptTs');
    const html = read('apptHtml');
    expect(ts).toMatch(/\{\s*name:\s*['"]_stage_id['"]\s*,\s*kind:\s*['"]in['"]\s*,?\s*\}/);
    expect(html).toMatch(/<app-customer-stage-multiselect-form-control[^>]*formControlName="_stage_id"/);
    expect(ts).toContain('CustomerStageMultiselectFormControl');
    expect(ts).not.toMatch(/['"]_status['"]|customerStatusOptions|CUSTOMER_STATUS/);
    expect(html).not.toMatch(/formControlName="_status"|customerStatusOptions/);
    // filter `status` của CHÍNH lịch hẹn giữ nguyên
    expect(ts).toMatch(/\{\s*name:\s*['"]status['"]\s*,\s*kind:\s*['"]equals['"]\s*,?\s*\}/);
  });

  it('AC2: báo cáo ghi chú lọc `_stage_id`, cột tên + màu bước map từ customer.stage_id', () => {
    const ts = read('noteTs');
    const html = read('noteHtml');
    const svc = read('noteService');
    expect(ts).toMatch(/\{\s*name:\s*['"]_stage_id['"]\s*,\s*kind:\s*['"]in['"]\s*,?\s*\}/);
    expect(ts).not.toMatch(/name:\s*['"]status['"]|customerStatusOptions|CUSTOMER_STATUS/);
    expect(html).toMatch(/<app-customer-stage-multiselect-form-control[^>]*formControlName="_stage_id"/);
    expect(html).not.toMatch(/formControlName="status"/);
    expect(html).toMatch(/\[class\]="model\.stageColorClass"/);
    expect(html).toMatch(/\{\{\s*model\.stageName\s*\}\}/);
    expect(html).not.toMatch(/statusClass|statusKey|statusLabel/);
    expect(svc).toMatch(/stageName:\s*this\.customerStageHelper\.getName\(\s*item\??\.customer\??\.stage_id\s*,?\s*\)/);
    expect(svc).toMatch(/stageColorClass:\s*this\.customerStageHelper\.getBadgeClass\(\s*item\??\.customer\??\.stage_id\s*,?\s*\)/);
    expect(svc).toMatch(/customerStageHelper\.ensureLoaded\(\)/);
    expect(read('noteModel')).toMatch(/stageName\?:\s*string/);
    expect(read('noteModel')).not.toMatch(/statusClass|statusKey|statusLabel/);
  });

  it('AC3: phễu dashboard dựng từ response bước, key widget + preset giữ nguyên', () => {
    const ts = read('funnelTs');
    // Mối nối: phễu nhận ĐÚNG `v.tiers` (không cắt/đổi), view dựng từ ĐÚNG response (không thay `[]`)
    expect(ts).toMatch(/return\s+rows\s*\?\s*buildCustomerStageFunnelView\(\s*rows\s*,?\s*\)\s*:\s*null/);
    expect(ts).toMatch(/\[data\]="v\.tiers"/);
    expect(ts).toMatch(/const\s+rows\s*=\s*this\.data\.value\(\)/);
    expect(ts).toMatch(/@for\s*\(\s*l\s+of\s+v\.lost\b/);
    expect(ts).toMatch(/createWidgetData<CustomerStageFunnelRow\[\]>\(\s*['"]customer_status_overview['"]/);
    expect(ts).toMatch(/statusCustomerOverview/);
    expect(ts).toContain('appFunnelChart');
    expect(read('widgets')).toMatch(/key:\s*['"]customer_status_overview['"]/);
    expect(read('presets')).toMatch(/\[\s*['"]customer_status_overview['"]/);
  });

  it('AC4: toàn cây src (trừ spec) không còn từ khoá tiến trình cũ', () => {
    const files = walk(APP).filter((f) => /\.(ts|html|scss)$/.test(f) && !/\.spec\.ts$/.test(f));
    expect(files.length).toBeGreaterThan(500); // đối chứng: quét thật chứ không phải thư mục rỗng
    const hits = files
      .filter((f) => OLD_PROGRESS.test(stripComments(readFileSync(f, 'utf8'))))
      .map((f) => f.replace(APP, ''));
    expect(hits).toEqual([]);
    expect(stripComments(readFileSync(SRC + SCSS, 'utf8'))).not.toMatch(/statusCustomer/);
    expect(read('customerModel')).not.toMatch(/^\s*status\??\s*:/m);
    expect(read('customerDefine')).toMatch(/CUSTOMER_TYPE_SELECT/); // phần còn lại của file được giữ
  });

  it('AC5: selector dùng chung trong custom.scss vẫn còn hiệu lực (paymentFor/status-*)', () => {
    const scss = stripComments(readFileSync(SRC + SCSS, 'utf8'));
    for (const sel of ['.paymentFor-1', '.paymentFor-2', '.paymentFor-3', '.paymentFor-4', '.paymentFor-5', '.paymentFor-6',
      '.status-active', '.status-inactive', '.status-suspended', '.status-1', '.status-2', '.status-3',
      '.badge-neutral', '.badge-positive']) {
      expect(scss, sel).toContain(sel);
    }
  });

  it('AC6: i18n vi/en không còn define.customer_status.*; key chung còn nơi dùng thì giữ', () => {
    for (const rel of I18N) {
      const j = JSON.parse(readFileSync(SRC + rel, 'utf8'));
      expect(j.define?.customer_status, rel).toBeUndefined();
      for (const k of ['lead', 'deposit', 'agreed', 'disagreed', 'in_treatment', 'contact', 'progress']) {
        expect(j.common.label[k], `${rel} common.label.${k}`).toBeTruthy();
      }
      expect(j.common.status.arrived, `${rel} common.status.arrived`).toBeTruthy();
      expect(j.config.customer_stage.type.lost, `${rel} config.customer_stage.type.lost`).toBeTruthy();
      expect(j.dashboard.crm.widgets.customer_status_overview.title, rel).toBeTruthy();
    }
  });
});
