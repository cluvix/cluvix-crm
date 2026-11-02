import { inject, Service} from '@angular/core';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

import { NoteDisplayModel, NoteModel } from '@/core/models/crm/note.model';
import { ListApiResponse } from '@/share/models/list-api-response';
import { BaseApiService } from '@/share/services/base-api.service';
import { BranchHelperService } from '@/indexeddb/config/components/branch';
import { UserHelperService } from '@/indexeddb/config/components/user';
import { LocationProvinceHelperService } from '@/indexeddb/config/components/location-province/location-province-helper.service';
import { SourceHelperService } from '@/indexeddb/config/components/source';
import { CustomerStageHelperService } from '@/indexeddb/config/components/customer-stage';

@Service()
export class NoteService extends BaseApiService<NoteModel> {
  protected override endpoint = 'note';
  protected override dateFields = [];
  protected override unixTimesTampFields = ['pinned_at', 'created_at', 'updated_at'];
  private locationProvinceHelper: LocationProvinceHelperService = inject(
    LocationProvinceHelperService,
  );
  private sourceHelper: SourceHelperService = inject(SourceHelperService);
  private userHelper: UserHelperService = inject(UserHelperService);
  private branchHelper: BranchHelperService = inject(BranchHelperService);
  private customerStageHelper = inject(CustomerStageHelperService);

  togglePin(id: number, pinned: boolean) {
    return this.apiService.post(`${this.path}/${id}/pin-note?pinned=${pinned}`);
  }

  override list(
    params: any = {},
  ): Observable<ListApiResponse<NoteModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService.get(`${this.path}`, params).pipe<ListApiResponse<NoteModel>>(
      map((listApiResponse: ListApiResponse<NoteModel>) => {
        const cloned = JSON.parse(JSON.stringify(listApiResponse));
        cloned.items.map((model: NoteModel) => {
          return this.afterLoad(model);
        });
        return cloned;
      }),
    );
  }

  listNoteAdvise(params: any = {}): Observable<ListApiResponse<NoteModel>> {
    params.page = params.page ? (params.page < 1 ? 1 : params.page) : 1;
    return this.apiService
      .get(`${this.path}/list-advise`, params)
      .pipe<ListApiResponse<NoteModel>>(
        map((listApiResponse: ListApiResponse<NoteModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned.items.map((model: NoteModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  createNote(createParams: any = {}, options: any = {}): Observable<NoteModel> {
    const params = this.beforeSave(createParams);
    return this.apiService.post<any>(`${this.path}`, params, options).pipe<NoteModel>(
      map((res: any) => {
        return res?.data ?? res;
      }),
    );
  }

  deleteNote(id: number): Observable<string> {
    return this.apiService.delete(`${this.path}/${id}`);
  }

  getAllNote(params: any = {}) {
    return this.apiService
      .get(`${this.path}/get-all-note`, params)
      .pipe<ListApiResponse<NoteModel>>(
        map((listApiResponse: ListApiResponse<NoteModel>) => {
          const cloned = JSON.parse(JSON.stringify(listApiResponse));
          cloned.items.map((model: NoteModel) => {
            return this.afterLoad(model);
          });
          return cloned;
        }),
      );
  }

  exportNote(params: any = {}): Observable<string> {
    return this.apiService.get(`${this.path}/get-all-note`, { ...params, _export: 1 });
  }

  addLabel(item: NoteModel) {
    const itemWithDates = this.addFormattedTimestampFields(item, ['created_at']);
    return {
      ...itemWithDates,
      customerDisplayName: `${item?.customer?.fullname || ''}${item?.customer?.nickname ? ' - ' + item?.customer?.nickname : ''}`,
      branchLabel: this.branchHelper.getBranchName(item.customer?.branch_id),
      stageName: this.customerStageHelper.getName(item.customer?.stage_id),
      stageColorClass: this.customerStageHelper.getBadgeClass(item.customer?.stage_id),
      createdByInfo: this.userHelper.getUserInfo(item.created_by),
      provinceLabel: this.locationProvinceHelper.getLabel(item?.customer?.province_id),
      sourceCustomerLabel: this.sourceHelper.getSourceName(item.customer?.source_id),
    };
  }

  async addLabels(items: NoteModel[]): Promise<NoteDisplayModel[]> {
    if (!Array.isArray(items)) return [];
    await this.ensureCacheLoaded();
    return items.map((item) => this.addLabel(item)) as NoteDisplayModel[];
  }

  async ensureCacheLoaded() {
    await Promise.all([
      this.branchHelper.ensureBranchesLoaded(),
      this.userHelper.ensureUsersLoaded(),
      this.locationProvinceHelper.ensureLocationProvincesLoaded(),
      this.sourceHelper.ensureSourcesLoaded(),
      this.customerStageHelper.ensureLoaded(),
    ]);
  }
}
