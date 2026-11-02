export const INDEXEDDB_SELECT = {
  INDEXEDDB_DB_NAME_CONFIG: 'cluvixData',
  INDEXEDDB_DB_VERSION: 76, // 76 = thêm store `customerStages` (pipeline tiến trình khách; BE sync key `customerStages`). 75 = gỡ hạng thành viên: bỏ store `customerRanks`/`customerRankServices` (BE bỏ bảng + sync key; store cũ mồ côi vô hại như `taxRates`). 74 = CRM thuần SME: bỏ store `notifications` (chuông/SSE đã xoá; upgrade không xoá store cũ — như `taxRates`, store mồ côi vô hại). 73 = story-30 QA crm-standalone: bỏ bảng `taxRates` (domain kế toán đã cắt, BE không serve). 72 = story-22 crm-standalone: thêm bảng `roles` (BE config/indexeddb/handler/sync.go có serve, FE table-registry thiếu — deferred-work story-54 QA-A)
} as const;
