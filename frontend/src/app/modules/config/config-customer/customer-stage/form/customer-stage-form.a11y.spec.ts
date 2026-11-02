import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, it } from 'vitest';

/**
 * Smoke S3 (epic customer-stage-pipeline): `p-select` lấy accessible name từ `ariaLabel` rồi mới tới nhãn option đang
 * chọn; option của form này là `{ value, labelKey }` (không có `label`) nên thiếu `ariaLabel` ⇒ PrimeNG trả CHÍNH OBJECT
 * option ⇒ tên truy cập "[object Object]". Component nặng PrimeNG không dựng TestBed được (treo CPU) ⇒ khoá ở mức template.
 */
const html = readFileSync(
  resolve(process.cwd(), 'src/app/modules/config/config-customer/customer-stage/form/customer-stage-form.html'),
  'utf8',
);

function selectTag(inputId: string): string {
  const m = html.match(new RegExp(`<p-select[^>]*inputId="${inputId}"[^>]*>`, 's'));
  expect(m, `không thấy p-select inputId="${inputId}"`).not.toBeNull();
  return m![0];
}

describe('customer-stage-form — accessible name của combobox', () => {
  it.each([
    ['type', 'common.label.type'],
    ['color', 'common.label.color'],
  ])('p-select %s có [ariaLabel] bằng nhãn i18n %s (không để PrimeNG tự lấy object option)', (inputId, key) => {
    expect(selectTag(inputId)).toContain(`[ariaLabel]="'${key}' | translate"`);
  });

  it('mọi p-select trong form đều khai ariaLabel', () => {
    const tags = html.match(/<p-select\b[^>]*>/gs) ?? [];
    expect(tags.length).toBe(2);
    for (const t of tags) expect(t).toContain('[ariaLabel]');
  });
});
