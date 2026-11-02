import { Observable } from 'rxjs';
import { Service } from '@angular/core';

import { ApiResponseOneModel } from '@/core/interface/ApiResponseOneModel';
import { CustomerStageModel } from '@/core/models/config/customer-stage.model';
import { CUSTOMER_STAGE_COLOR_DEFAULT, CUSTOMER_STAGE_TYPE } from '@/define';
import { BaseApiService } from '@/share/services/base-api.service';

/** Số dòng tối đa BE cho 1 trang list (`pagination.Paginate`) — danh mục bước ít, lấy 1 lần đủ. */
export const CUSTOMER_STAGE_LIST_PAGE_SIZE = 200;

@Service()
export class CustomerStageService extends BaseApiService<CustomerStageModel> {
  protected override apiModule = 'config';
  protected override endpoint = 'customer-stage';

  /** Seed ĐỦ key, không `undefined` (Signal Forms) — name là `pInputText` ⇒ '' (không `null`). */
  override getInitialObject(): CustomerStageModel {
    return {
      name: '',
      type: CUSTOMER_STAGE_TYPE.OPEN,
      color: CUSTOMER_STAGE_COLOR_DEFAULT,
      status: 1,
    };
  }

  /** Đổi thứ tự: `ids` PHẢI đủ MỌI bước theo thứ tự mới (BE trả 422 `reorder_invalid` nếu thiếu/thừa). */
  reorder(ids: number[]): Observable<ApiResponseOneModel<{ ids: number[] }>> {
    return this.apiService.put<ApiResponseOneModel<{ ids: number[] }>>(`${this.path}/reorder`, { ids });
  }

  /** Đặt bước mặc định cho khách mới (BE: chỉ bước loại "đang mở" và đang hiện). */
  setDefault(id: number): Observable<ApiResponseOneModel<CustomerStageModel>> {
    return this.apiService.put<ApiResponseOneModel<CustomerStageModel>>(
      `${this.path}/${id}/set-default`,
      {},
    );
  }
}
