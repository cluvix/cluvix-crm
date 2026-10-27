import { HttpParams } from '@angular/common/http';
import { inject, Service } from '@angular/core';
import { Observable } from 'rxjs';
import { map } from 'rxjs/operators';

import {
  FbGroup,
  FbGroupDetail,
  FbGroupIngestKeyInfo,
  FbGroupListQuery,
  FbGroupPreview,
  FbGroupStatsBranch,
  FbGroupStatsKeyword,
  FbGroupStatsOverview,
  FbGroupStatsProvince,
  FbGroupTopic,
  FbGroupTopicInput,
  FbGroupUpsertResult,
} from '@/core/models/fbgroup/fbgroup.model';
import { ApiService } from '@/share/services/api.service';
import { ListApiResponse } from '@/share/models/list-api-response';

/** Bỏ giá trị rỗng (null/undefined/'') rồi dựng HttpParams — BE coi param vắng là "không lọc". */
function toParams(obj: Record<string, unknown>): HttpParams {
  const clean: Record<string, string> = {};
  for (const [k, v] of Object.entries(obj)) {
    if (v === null || v === undefined || v === '') continue;
    clean[k] = String(v);
  }
  return new HttpParams({ fromObject: clean });
}

/**
 * API module Group Facebook (/api/fbgroup/*). Tham số lọc là query param thường (topicId,
 * workStatus...) — KHÔNG phải cú pháp filter[...] của BaseListV2, nên service riêng, không kế thừa
 * BaseApiService.
 */
@Service()
export class FbGroupService {
  private readonly api = inject(ApiService);
  private readonly base = 'fbgroup';

  // ─── Group ───
  listGroups(q: FbGroupListQuery): Observable<ListApiResponse<FbGroup>> {
    return this.api
      .get<ListApiResponse<FbGroup>>(`${this.base}/group`, { params: toParams(q as unknown as Record<string, unknown>) })
      .pipe(map((res) => res ?? { items: [] }));
  }

  /** Xuất Excel theo bộ lọc hiện tại (`?_export=1`) — BE trả URL tải (cần JWT, tải qua downloadExport). */
  exportGroups(q: FbGroupListQuery): Observable<string> {
    const { page: _page, 'per-page': _perPage, ...filters } = q;
    return this.api.get<string>(`${this.base}/group`, {
      params: toParams({ ...(filters as unknown as Record<string, unknown>), _export: 1 }),
    });
  }

  getGroup(id: number): Observable<FbGroupDetail> {
    return this.api.get<{ data: FbGroupDetail }>(`${this.base}/group/${id}`).pipe(map((res) => res.data));
  }

  updateGroup(id: number, body: { work_status?: number; note?: string }): Observable<{ data: FbGroupDetail; message: string }> {
    return this.api.put(`${this.base}/group/${id}`, body);
  }

  bulkGroups(body: {
    ids: number[];
    work_status?: number;
    add_topic_id?: number;
    remove_topic_id?: number;
  }): Observable<{ data: { affected: number; ignored: number }; message: string }> {
    return this.api.post(`${this.base}/group/bulk`, body);
  }

  importCsv(file: File): Observable<{ data: FbGroupUpsertResult; message: string }> {
    const fd = new FormData();
    fd.append('file', file, file.name);
    return this.api.post(`${this.base}/group/import-csv`, fd as unknown as Record<string, unknown>);
  }

  // ─── Chủ đề ───
  listTopics(): Observable<FbGroupTopic[]> {
    return this.api.get<{ data: FbGroupTopic[] }>(`${this.base}/topic`).pipe(map((res) => res?.data ?? []));
  }

  createTopic(body: FbGroupTopicInput): Observable<{ data: FbGroupTopic; message: string }> {
    return this.api.post(`${this.base}/topic`, body);
  }

  updateTopic(id: number, body: FbGroupTopicInput): Observable<{ data: FbGroupTopic; message: string }> {
    return this.api.put(`${this.base}/topic/${id}`, body);
  }

  deleteTopic(id: number): Observable<{ message: string }> {
    return this.api.delete(`${this.base}/topic/${id}`);
  }

  previewTopic(include: string[], exclude: string[]): Observable<FbGroupPreview> {
    return this.api
      .post<{ data: FbGroupPreview }>(`${this.base}/topic/preview`, { include_keywords: include, exclude_keywords: exclude })
      .pipe(map((res) => res.data));
  }

  reclassify(): Observable<{ data: { processed: number }; message: string }> {
    return this.api.post(`${this.base}/topic/reclassify`, {});
  }

  // ─── Thống kê ───
  statsOverview(topicId: number | null): Observable<FbGroupStatsOverview> {
    return this.api
      .get<{ data: FbGroupStatsOverview }>(`${this.base}/stats/overview`, { params: toParams({ topicId }) })
      .pipe(map((res) => res.data));
  }

  statsBranches(topicId: number | null): Observable<FbGroupStatsBranch[]> {
    return this.api
      .get<{ data: FbGroupStatsBranch[] }>(`${this.base}/stats/branches`, { params: toParams({ topicId }) })
      .pipe(map((res) => res?.data ?? []));
  }

  statsProvinces(topicId: number | null): Observable<FbGroupStatsProvince[]> {
    return this.api
      .get<{ data: FbGroupStatsProvince[] }>(`${this.base}/stats/provinces`, { params: toParams({ topicId }) })
      .pipe(map((res) => res?.data ?? []));
  }

  statsKeywords(): Observable<FbGroupStatsKeyword[]> {
    return this.api
      .get<{ data: FbGroupStatsKeyword[] }>(`${this.base}/stats/keywords`)
      .pipe(map((res) => res?.data ?? []));
  }

  // ─── Kết nối NextCore ───
  getIngestKey(): Observable<FbGroupIngestKeyInfo> {
    return this.api.get<{ data: FbGroupIngestKeyInfo }>(`${this.base}/ingest-key`).pipe(map((res) => res.data));
  }

  createIngestKey(): Observable<{ data: { key: string; key_hint: string; ingest_path: string }; message: string }> {
    return this.api.post(`${this.base}/ingest-key`, {});
  }
}
