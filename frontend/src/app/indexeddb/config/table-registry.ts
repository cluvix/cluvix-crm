import { InjectionToken } from '@angular/core';

export interface TableIndexConfig {
  name: string;
  field: string;
  unique?: boolean;
}

export interface TableConfig {
  name: string;
  keyPath: string;
  /** Indexes tạo lúc onupgradeneeded — dùng cho query nhanh qua store.index(...) */
  indexes?: TableIndexConfig[];
  /**
   * Store CHỈ sống ở client — KHÔNG có bên tương ứng trong registry BE (`configTables`), KHÔNG bao
   * giờ được nạp/ghi đè bởi `/crm/indexeddb/sync` (`MasterDataRegistryService.syncAllTables` /
   * `syncSingleTable`). Dùng cho dữ liệu per-user (thông báo của chuông) — sync nhầm là xoá trắng
   * dữ liệu của đúng người đang dùng mà không có lỗi nào.
   */
  localOnly?: boolean;
}

export const TABLE_CONFIGS: TableConfig[] = [
  { name: 'users', keyPath: 'id' },
  { name: 'appointmentTypes', keyPath: 'id' },
  { name: 'services', keyPath: 'id' },
  { name: 'branches', keyPath: 'id' },
  { name: 'units', keyPath: 'id' },
  { name: 'serviceTypes', keyPath: 'id' },
  { name: 'locationCountries', keyPath: 'id' },
  { name: 'locationProvinces', keyPath: 'id' },
  { name: 'appointmentIsAccepteds', keyPath: 'id' },
  { name: 'paymentMethods', keyPath: 'id' },
  { name: 'paymentTypes', keyPath: 'id' },
  { name: 'sources', keyPath: 'id' },
  { name: 'appointmentNotBookedReasons', keyPath: 'id' },
  { name: 'noteTemplates', keyPath: 'id' },
  { name: 'appointmentReasons', keyPath: 'id' },
  { name: 'customerCategories', keyPath: 'id' },
  { name: 'customerStages', keyPath: 'id' },
  { name: 'hrDepartments', keyPath: 'id' },
  { name: 'groupSources', keyPath: 'id' },
  { name: 'groupLocals', keyPath: 'id' },
  { name: 'groupCountries', keyPath: 'id' },
  { name: 'settings', keyPath: 'id' },
  { name: 'userOptions', keyPath: 'id' },
  { name: 'controlFieldInTables', keyPath: 'id' },
  { name: 'job', keyPath: 'id' },
  { name: 'ethnicity', keyPath: 'id' },
  // BE `config/indexeddb/handler/sync.go` "roles" — danh sách vai trò (rbac_auth_item type=1,
  // trừ admin/develop/partner) — SELECT trả `value`/`label` (không có cột `id`) nên keyPath='value'.
  { name: 'roles', keyPath: 'value' },

  // Per-user auth session — keyPath userId, KHÔNG sync qua Dexie global mechanism.
  // Lưu user profile + permissions riêng từng user, admin sửa chỉ user đó sync.
  { name: 'userAuthSession', keyPath: 'userId' },
];

export const TABLE_REGISTRY = new InjectionToken<TableConfig[]>('TABLE_REGISTRY', {
  providedIn: 'root',
  factory: () => TABLE_CONFIGS,
});
