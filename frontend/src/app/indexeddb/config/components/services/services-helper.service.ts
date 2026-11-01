import { inject, Service} from '@angular/core';
import { ServicesCacheService } from './services-cache.service';
import { ServiceModel } from '@/core/models/config/clinic/service.model';

@Service()
export class ServicesHelperService {
  private servicesCacheService = inject(ServicesCacheService);

  async ensureServicesLoaded(): Promise<void> {
    await this.servicesCacheService.ensureServicesLoaded();
  }

  getServiceName(id: number | null | undefined): string {
    if (!id) return '';
    const service = this.servicesCacheService.getById(id);
    return service?.name || '—';
  }

  getServiceInfo(id: number) {
    const service = this.servicesCacheService.getById(id);
    if (!service) {
      return null;
    }

    return this.buildServiceInfo(service);
  }

  private buildServiceInfo(service: any) {
    return {
      id: service?.id ?? null,
      code: service?.code ?? '',
      name: service?.name ?? '',
      unit_id: service?.unit_id ?? null,
      service_type_id: service?.service_type_id ?? null,
      isUpdateMaterial: service?.isUpdateMaterial ?? null,
      min_price: service?.min_price ?? null,
      max_price: service?.max_price ?? null,
      child: service?.child ?? '',
      servicePosition: service?.servicePosition ?? null,
      department_type: service?.department_type ?? null,
      serviceCategory_id: service?.serviceCategory_id ?? null,
      emr_department_id: service?.emr_department_id ?? null,
    };
  }

  mapServiceInfos(ids: number[]): ServiceModel[] {
    if (!Array.isArray(ids)) return [];
    return ids
      .filter((id) => id != null)
      .map((id) => this.servicesCacheService.getById(id))
      .filter((c) => c != null);
  }

  /**
   * Lấy danh sách services cho PickList
   * @param selectedIds Danh sách ID đã chọn
   */
  getServicesForPickList(selectedIds: number[] = []) {
    const allServices = this.getServices();

    const selectedSet = new Set(selectedIds);
    const available: ServiceModel[] = [];
    const selected: ServiceModel[] = [];

    allServices.forEach((service) => {
      if (selectedSet.has(service.id!)) {
        selected.push(service);
      } else {
        available.push(service);
      }
    });

    return { available, selected };
  }

  /**
   * Tìm kiếm services theo keyword (cho autocomplete)
   * @param keyword Từ khóa tìm kiếm
   * @returns Danh sách services match với keyword
   */
  searchServices(keyword: string): ServiceModel[] {
    const normalizedKeyword = keyword?.toLowerCase().trim() || '';

    return this.getServices()
      .filter((service) => {
        // Filter by status (only active)
        if (service.status !== 1) {
          return false;
        }

        // Search in name and code
        if (normalizedKeyword) {
          const name = service.name?.toLowerCase() || '';
          const code = service.code?.toLowerCase() || '';
          return name.includes(normalizedKeyword) || code.includes(normalizedKeyword);
        }

        return true;
      })
      .sort((a, b) => (b.SortOrder || 0) - (a.SortOrder || 0))
      .slice(0, 50); // Limit to 50 results for performance
  }

  /**
   * Dịch vụ ACTIVE dùng cho luồng "đổi dịch vụ" (order-service-change). D16: dịch vụ không còn
   * khoa (`emr_department_id` đã bỏ khỏi BE) → đổi dịch vụ tự do, KHÔNG lọc theo khoa nữa.
   * @param excludeId loại trừ dịch vụ hiện tại khỏi danh sách chọn
   */
  getServicesForChange(excludeId?: number): ServiceModel[] {
    return this.getServices().filter(
      (service: any) => service.status === 1 && service.id !== excludeId,
    );
  }

  /**
   * Dịch vụ ACTIVE — cho các ô chọn dịch vụ dạng dropdown đầy đủ
   * (story-12 emr-episode-flow D12: ô chọn dịch vụ ở tab Kết luận).
   * Khác `searchServices`: KHÔNG cắt 50 kết quả.
   */
  getActiveServicesForCompany(): ServiceModel[] {
    return this.getServices()
      .filter((service: any) => service.status === 1)
      .sort((a: any, b: any) => String(a?.name ?? '').localeCompare(String(b?.name ?? '')));
  }

  private getServices() {
    return this.servicesCacheService.getAllServicesUnfiltered();
  }
}
