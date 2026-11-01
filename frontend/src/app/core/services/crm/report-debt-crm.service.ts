import { inject, Service } from '@angular/core';
import { Observable } from 'rxjs';

import { BaseApiService } from '@/share/services/base-api.service';
import { BranchHelperService } from '@/indexeddb/config/components/branch';
import { ServicesHelperService } from '@/indexeddb/config/components/services/services-helper.service';

/**
 * Báo cáo công nợ CRM — BE `crm/report/debt/*` (customer · order-plan · service).
 * Mỗi báo cáo: `list*` trả `{items, _meta, total...}`, `export*` (?_export=1) trả URL file.
 */
@Service()
export class ReportDebtCrmService extends BaseApiService<any> {
  protected override endpoint = 'report/debt';
  private branchHelper = inject(BranchHelperService);
  private servicesHelper = inject(ServicesHelperService);

  listCustomerDebts(params: any = {}): Observable<any> {
    return this.apiService.get(`${this.path}/customer`, params);
  }

  exportCustomerDebts(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/customer?_export=1`, params);
  }

  listOrderPlanDebts(params: any = {}): Observable<any> {
    return this.apiService.get(`${this.path}/order-plan`, params);
  }

  exportOrderPlanDebts(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/order-plan?_export=1`, params);
  }

  listServiceDebts(params: any = {}): Observable<any> {
    return this.apiService.get(`${this.path}/service`, params);
  }

  exportServiceDebts(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/service?_export=1`, params);
  }

  /**
   * Gắn nhãn hiển thị từ IndexedDB: chi nhánh, tên dịch vụ, ngày đã format (`<field>Formatted`).
   * @param timestampFields cột Unix cần format (vd `created_time`, `created_day`)
   */
  async addDebtLabels(items: any[], timestampFields: string[] = []): Promise<any[]> {
    if (!Array.isArray(items)) return [];
    await Promise.all([this.branchHelper.ensureBranchesLoaded(), this.servicesHelper.ensureServicesLoaded()]);
    return items.map((item) => ({
      ...this.addFormattedTimestampFields(item, timestampFields),
      branchLabel: this.branchHelper.getBranchName(item?.branch_id),
      serviceName: this.servicesHelper.getServiceName(item?.service_id),
    }));
  }
}
