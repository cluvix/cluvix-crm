import { computed, Service } from '@angular/core';

import { BaseCacheService } from '@/indexeddb/config/components/base';
import { CustomerStageModel } from '@/core/models/config/customer-stage.model';
import {
  CUSTOMER_STAGE_STATUS_VISIBLE,
  sortCustomerStages,
} from './customer-stage-options.util';

/**
 * Cache bước tiến trình khách (Dexie `customerStages`, BE sync KHÔNG lọc status — có cả bước ẩn).
 * - `getAll()`: chỉ bước đang hiện, theo `sort_order` — dùng cho selector tạo/sửa.
 * - `getAllUnfiltered()`: mọi bước kể cả ẩn — dùng để tra tên/màu bản ghi cũ và liệt kê ở bộ lọc.
 * - `stageById()`/`allUnfiltered`: bản SIGNAL (đọc registry) — computed/template gọi sẽ tự cập nhật
 *   sau khi sync, khác `getById()` của base đọc thuộc tính thường (đóng băng trong computed).
 */
@Service()
export class CustomerStageCacheService extends BaseCacheService<CustomerStageModel> {
  protected tableName = 'customerStages';
  protected activeStatus = CUSTOMER_STAGE_STATUS_VISIBLE;
  protected allowedStatuses = [CUSTOMER_STAGE_STATUS_VISIBLE];

  /** Toàn bộ bước (cả ẩn) đã sắp theo `sort_order` — reactive theo dữ liệu registry. */
  readonly allUnfiltered = computed(() =>
    sortCustomerStages(
      (this.registry.getTableService(this.tableName)?.getAll()() ?? []) as CustomerStageModel[],
    ),
  );

  private readonly byId = computed(() => {
    const map = new Map<number, CustomerStageModel>();
    for (const s of this.allUnfiltered()) if (s.id !== undefined) map.set(s.id, s);
    return map;
  });

  private selfHeal: Promise<void> | null = null;

  async ensureCustomerStagesLoaded(forceReload = false): Promise<void> {
    await this.loadItems(forceReload);
    await this.syncIfStoreEmpty();
  }

  /**
   * Store `customerStages` mới tạo (IDB nâng version) thì RỖNG, mà header `X-Dexie-Versions` chỉ báo
   * lệch khi BE đã bump `customerStages` (chỉ xảy ra sau lần sửa danh mục đầu tiên) ⇒ người đã đăng
   * nhập từ trước sẽ không bao giờ nạp được. Danh mục luôn có ≥ 1 bước (bước mặc định) nên rỗng nghĩa
   * là CHƯA nạp, không phải "chưa cấu hình": tự đồng bộ 1 lần. Lỗi mạng ⇒ cho phép thử lại lần sau.
   */
  private syncIfStoreEmpty(): Promise<void> {
    if (this.allUnfiltered().length > 0) return Promise.resolve();
    if (!this.selfHeal) {
      this.selfHeal = this.registry
        .invalidateAndSync(this.tableName)
        .catch((err) => console.warn('[CustomerStage] tự đồng bộ store rỗng thất bại:', err))
        .finally(() => {
          this.selfHeal = null;
        });
    }
    return this.selfHeal;
  }

  /** Bước đang hiện, sắp theo `sort_order` tăng dần. */
  getAll(): CustomerStageModel[] {
    return sortCustomerStages(this.getAllItems()).filter(
      (s) => s.status === CUSTOMER_STAGE_STATUS_VISIBLE,
    );
  }

  /** Mọi bước kể cả ẩn, sắp theo `sort_order` tăng dần (bản không-lọc cho form giữ bước ẩn đang chọn). */
  getAllUnfiltered(): CustomerStageModel[] {
    return sortCustomerStages(this.getAllItems());
  }

  /** Tra bước theo id, reactive. Không có ⇒ undefined (không throw). */
  stageById(id: number | null | undefined): CustomerStageModel | undefined {
    if (!id) return undefined;
    return this.byId().get(id);
  }

  protected override generateCacheKey(): string {
    return 'all';
  }
}
