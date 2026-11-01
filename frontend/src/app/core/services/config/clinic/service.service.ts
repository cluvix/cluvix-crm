import { inject, Service} from '@angular/core';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

import { ApiResponseOneModel } from '@/core/interface/ApiResponseOneModel';
import { ServiceModel } from '@/core/models/config/clinic/service.model';
import { ListApiResponse } from '@/share/models/list-api-response';
import { BaseApiService } from '@/share/services/base-api.service';
import { UnitHelperService } from '@/indexeddb/config/components/unit';
import { ServiceTypeHelperService } from '@/indexeddb/config/components/service-type/service-type-helper.service';

@Service()
export class ServiceService extends BaseApiService<ServiceModel> {
  protected override apiModule = 'config';
  protected override endpoint = 'service';
  protected override unixTimesTampFields = ['created_at'];
  private unitHelper = inject(UnitHelperService);
  private serviceTypeHelper = inject(ServiceTypeHelperService);

  override getInitialObject() {
    return {
      status: 1,
      isUpdateMaterial: 0,
    };
  }

  loadServiceByKeyWord(keyWord: any = null): Observable<any> {
    return this.apiService.get(
      `${this.path}/load-service?keyword=${keyWord}`,
    );
  }

  exportService(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}?_export=1`, params);
  }

  /**
   * File mẫu import là file TĨNH trong assets như mọi file mẫu khác của dự án (BE không có endpoint
   * phát file mẫu). File được sinh bằng `backend/cmd/gen-template -only=service` để bộ cột luôn
   * khớp file export; sheet Danh_muc trong đó chỉ chứa danh mục cố định của hệ thống, còn khoa/nhóm
   * dịch vụ/đơn vị/thương hiệu/thuế suất phải tra id qua chức năng Export (id khác nhau mỗi doanh
   * nghiệp nên file dùng chung không nhúng được).
   */
  downloadImportTemplate() {
    const a = document.createElement('a');
    a.href = '/assets/download/mau_import_dich_vu.xlsx';
    a.download = 'mau_import_dich_vu.xlsx';
    a.click();
  }

  importServiceFromExcel(file: File): Observable<any> {
    const formData = new FormData();
    formData.append('file', file, file.name);
    return this.apiService.post(`${this.path}/import-service-from-excel`, formData);
  }

  loadServiceByType(params: any): Observable<any> {
    let param = '';
    params.forEach((value: any, index: any) => {
      param += `listType[${index}]=${value}&`;
    });
    return this.apiService.get(`${this.path}/load-service-by-type?${param}`);
  }

  createService(createParams: any = {}, options: any = {}): Observable<any> {
    const params = this.beforeSave(createParams);
    return this.apiService.post(`${this.path}`, params, options);
  }

  updateService(
    id: number,
    updateParams: any = {},
    options: any = {},
  ): Observable<ApiResponseOneModel<ServiceModel>> {
    const params = this.beforeSave(updateParams);
    // PUT — `resolveLoadingLabelKey` (loading-label.util.ts) suy label theo HTTP method; POST luôn ra
    // "Đang tạo mới" kể cả khi đang sửa.
    const request$ = this.apiService.put<ApiResponseOneModel<ServiceModel>>(
      `${this.path}/${id}`,
      params,
      options,
    );
    return request$
      .pipe(
        map((res: any) => {
          const cloned = JSON.parse(JSON.stringify(res));
          cloned.data = this.afterLoad(cloned.data);
          return cloned;
        }),
      );
  }

  addLabel(item: ServiceModel) {
    return {
      ...item,
      unitLabel: this.unitHelper.getLabel(item.unit_id),
      serviceTypeLabel: this.serviceTypeHelper.getLabel(item.service_type_id!),
    };
  }

  async addLabels(items: ServiceModel[]) {
    if (!Array.isArray(items)) return [];
    await this.ensureCacheLoaded();
    return items.map((item) => this.addLabel(item));
  }

  async ensureCacheLoaded() {
    await Promise.all([
      this.unitHelper.ensureUnitsLoaded(),
      this.serviceTypeHelper.ensureServiceTypesLoaded(),
    ]);
  }
}
