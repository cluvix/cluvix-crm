// Kiểu dữ liệu module Group Facebook — khớp JSON của BE /api/fbgroup/* (backend/internal/modules/webapp/fbgroup).

export interface FbGroupTopicRef {
  topic_id: number;
  name: string;
  color: string;
  branch_id: number | null;
  branch_name: string | null;
  source: number;
}

export interface FbGroup {
  id: number;
  facebook_id: string;
  name: string;
  url: string;
  member_count: number | null;
  member_rounded: boolean;
  posts_per_day: number | null;
  privacy: number | null;
  province: string | null;
  found_keyword: string | null;
  ncb_quality: number | null;
  work_status: number;
  last_seen_at: number | null;
  synced_at: number | null;
  topics: FbGroupTopicRef[];
}

export interface FbGroupDetail extends FbGroup {
  slug: string | null;
  ncb_region: string | null;
  ncb_category: string | null;
  found_by_profile: string | null;
  note: string | null;
  first_seen_at: number | null;
  created_at: number;
  updated_at: number;
}

export interface FbGroupTopicBranch {
  id: number;
  topic_id?: number;
  name: string;
  keywords: string[];
  sort_order: number;
}

export interface FbGroupTopic {
  id: number;
  name: string;
  color: string;
  include_keywords: string[];
  exclude_keywords: string[];
  min_members: number;
  min_posts_per_day: number;
  public_only: boolean;
  sort_order: number;
  status: number;
  branches: FbGroupTopicBranch[];
  group_count?: number;
  qualified_count?: number;
}

/** Thân POST/PUT /fbgroup/topic — branch id 0 = nhánh mới. */
export type FbGroupTopicInput = Omit<FbGroupTopic, 'id' | 'group_count' | 'qualified_count' | 'branches'> & {
  branches: FbGroupTopicBranch[];
};

export interface FbGroupPreview {
  matched: number;
  excluded: number;
  samples: string[];
}

export interface FbGroupStatsOverview {
  overview: {
    total_groups: number;
    on_topic_groups: number;
    qualified: number;
    members: number;
    big_groups: number;
    new_7d: number;
    excluded: number;
    last_synced_at: number;
  };
  by_topic: {
    topic_id: number;
    name: string;
    color: string;
    groups: number;
    members: number;
    qualified: number;
    big_groups: number;
  }[];
  by_work_status: { work_status: number; groups: number }[];
}

export interface FbGroupStatsBranch {
  topic_id: number;
  topic_name: string;
  topic_color: string;
  branch_id: number | null;
  branch_name: string | null;
  groups: number;
  members: number;
  over_10k: number;
  active_10_per_day: number;
}

export interface FbGroupStatsProvince {
  province: string;
  groups: number;
  members: number;
}

export interface FbGroupStatsKeyword {
  keyword: string;
  on_topic: number;
  off_topic: number;
  avg_members: number;
  over_10k: number;
}

export interface FbGroupUpsertResult {
  received: number;
  created: number;
  updated: number;
  skipped: number;
}

export interface FbGroupIngestKeyInfo {
  has_key: boolean;
  key_hint: string;
  last_used_at: number | null;
  last_batch_count: number | null;
  total_received: number;
  created_at: number;
  ingest_path: string;
}

/** Bộ lọc màn danh sách — tên trùng query param BE nhận. */
export interface FbGroupListQuery {
  page: number;
  'per-page': number;
  sort?: string;
  _keyword?: string;
  topicId?: number | null;
  branchId?: number | null;
  workStatus?: string;
  privacy?: number | null;
  minMembers?: number | null;
  minPostsPerDay?: number | null;
  province?: string | null;
  qualified?: string;
}
