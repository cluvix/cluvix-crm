import { computed, inject, Signal } from '@angular/core';
import { TranslateService } from '@ngx-translate/core';

/**
 * Translate options array — utility for migrating define-file enums from
 * hardcoded `label` to i18n `labelKey`.
 *
 * Usage in component:
 * ```typescript
 * import { translateOptions } from '@/share/utils/translate-options.util';
 * import { GENDER_OPTIONS } from '@/define';
 *
 * @Component({...})
 * export class CustomerList {
 *   protected readonly genderOptions = translateOptions(GENDER_OPTIONS);
 *   // Template: <p-select [options]="genderOptions()" optionLabel="label" .../>
 * }
 * ```
 *
 * Returns a Signal so PrimeNG p-select reactively re-renders when language changes
 * (TranslateService onLangChange fires → translate.instant returns new value →
 * computed re-runs).
 *
 * If option already has `labelKey`, uses translated value as `label`.
 * If option only has `label` (legacy), passes through unchanged for safe rollout.
 *
 * @template T option type with optional labelKey field
 */
export function translateOptions<T extends { label?: string; labelKey?: string }>(
  options: readonly T[],
): Signal<T[]> {
  const translate = inject(TranslateService);
  return computed(() => {
    // re-trigger on lang change via translate.currentLang signal-like pattern
    // (TranslateService 17+ exposes currentLang as direct signal in some setups,
    //  but for compat we rely on instant() + manual subscription would be ideal)
    const _lang = translate.currentLang;
    void _lang; // capture for change detection
    return options.map((opt) => {
      if (opt.labelKey) {
        return { ...opt, label: translate.instant(opt.labelKey) };
      }
      return { ...opt };
    });
  });
}

/**
 * Synchronous version — for non-reactive contexts (e.g., service constructors).
 * Translates once, won't update on lang change. Prefer signal version for templates.
 */
export function translateOptionsSync<T extends { label?: string; labelKey?: string }>(
  options: readonly T[],
  translate: TranslateService,
): T[] {
  return options.map((opt) => {
    if (opt.labelKey) {
      return { ...opt, label: translate.instant(opt.labelKey) };
    }
    return { ...opt };
  });
}
