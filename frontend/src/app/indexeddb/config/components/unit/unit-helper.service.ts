import { inject, Service} from '@angular/core';
import { UnitCacheService } from './unit-cache.service';

@Service()
export class UnitHelperService {
  private unitCache = inject(UnitCacheService);

  /**
   * Đảm bảo đã load dữ liệu unit vào cache trước khi dùng các hàm đồng bộ bên dưới.
   * Gọi await ensureUnitsLoaded() ở component/service trước khi dùng getLabel.
   */
  async ensureUnitsLoaded() {
    await this.unitCache.loadUnits();
  }

  /**
   * Lấy tên đơn vị theo id. Yêu cầu cache đã được load trước đó.
   */
  getLabel(id: number | null | undefined): string {
    if (!id) return '';
    const unit = this.unitCache.getById(id);
    return unit?.name ?? '';
  }

  getUnitInfo(id: number) {
    const item = this.unitCache.getById(id);
    if (!item) return null;

    return {
      id: item?.id ?? null,
      name: item?.name ?? '',
    };
  }

  /**
   * Lấy tên đơn vị theo id, đảm bảo cache đã load (dùng khi chưa chắc cache đã load).
   */
  async getLabelAsync(id: number | null | undefined): Promise<string> {
    if (!id) return '';
    await this.ensureUnitsLoaded();
    const unit = this.unitCache.getById(id);
    return unit?.name ?? '';
  }
}
