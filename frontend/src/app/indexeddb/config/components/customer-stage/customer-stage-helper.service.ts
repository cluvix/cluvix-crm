import { inject, Service } from '@angular/core';

import { customerStageColorClass } from '@/define';
import { CustomerStageCacheService } from './customer-stage-cache.service';
import { CUSTOMER_STAGE_STATUS_VISIBLE } from './customer-stage-options.util';

/**
 * Tra tên/màu/loại bước theo `stage_id` (đọc cache IndexedDB, KỂ CẢ bước đã ẩn). Mọi hàm đọc signal
 * nên dùng thẳng trong template OnPush — tự cập nhật khi cache được sync lại. Id không có trong cache
 * ⇒ trả rỗng/`-`/null, KHÔNG throw. Màn dùng helper phải `await ensureLoaded()` ở `ngOnInit`
 * (F5 xong store chưa nạp vào bộ nhớ).
 */
@Service()
export class CustomerStageHelperService {
  private cache = inject(CustomerStageCacheService);

  async ensureLoaded(): Promise<void> {
    await this.cache.ensureCustomerStagesLoaded();
  }

  /** Không có id ⇒ ''; id không còn trong cache ⇒ '-'. */
  getName(id: number | null | undefined): string {
    if (!id) return '';
    return this.cache.stageById(id)?.name || '-';
  }

  /** Khoá màu (`gray`…`purple`); không biết ⇒ ''. Dùng `getBadgeClass`/`getDotClass` để lấy class. */
  getColor(id: number | null | undefined): string {
    return this.cache.stageById(id)?.color ?? '';
  }

  /** Loại bước (`CUSTOMER_STAGE_TYPE`); không biết ⇒ null. */
  getType(id: number | null | undefined): number | null {
    return this.cache.stageById(id)?.type ?? null;
  }

  /** Bước đã bị ẩn (status != 1). Không biết ⇒ false. */
  isHidden(id: number | null | undefined): boolean {
    const s = this.cache.stageById(id);
    return !!s && s.status !== CUSTOMER_STAGE_STATUS_VISIBLE;
  }

  /** Class Tailwind huy hiệu có nền theo màu bước (id lạ ⇒ xám). */
  getBadgeClass(id: number | null | undefined): string {
    return customerStageColorClass(this.getColor(id)).badge;
  }

  /** Class chấm màu nhỏ theo màu bước (id lạ ⇒ xám). */
  getDotClass(id: number | null | undefined): string {
    return customerStageColorClass(this.getColor(id)).dot;
  }
}
