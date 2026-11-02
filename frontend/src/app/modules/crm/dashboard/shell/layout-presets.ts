import type { DashboardSize, DashboardWidgetDef, DashboardZone } from './dashboard-widgets';
import { type DashboardLayoutConfig, DASHBOARD_LAYOUT_VERSION, fullLayout } from './layout.util';

/**
 * Mẫu bố cục "đủ khối" theo vai trò (port ERP, cắt 6 khối CRM không có) — chọn trong chế độ Tuỳ chỉnh, áp vào BẢN
 * NHÁP (Huỷ để quay lại, Lưu/Lưu mặc định thương hiệu như thường). Dựng trên registry ĐÃ LỌC QUYỀN ⇒ khối user không
 * được thấy không bao giờ vào mẫu. Mẫu nào cũng hiện ĐỦ mọi khối, chỉ khác thứ tự/vùng:
 *  - khối nhóm trọng tâm của vai trò lên đầu;
 *  - cột chính: khối ½ luôn đi CẶP cùng chiều cao biểu đồ (400+400, 320+320) ⇒ không hở nửa hàng;
 *  - cột phụ chỉ có cỡ đầy đủ; tổng chiều cao 2 cột xấp xỉ nhau.
 * `overview` = bố cục chuẩn của registry (`fullLayout`).
 */
export type DashboardPresetId = 'overview' | 'revenue' | 'front_desk' | 'marketing';

type Slot = readonly [key: string, size?: DashboardSize];

interface PresetSlots {
  top: readonly Slot[];
  main: readonly Slot[];
  side: readonly Slot[];
}

export interface DashboardLayoutPreset {
  id: DashboardPresetId;
  labelKey: string;
  icon: string;
}

export const DASHBOARD_LAYOUT_PRESETS: readonly DashboardLayoutPreset[] = [
  { id: 'overview', labelKey: 'dashboard.crm.layout.preset.overview', icon: 'pi pi-th-large' },
  { id: 'revenue', labelKey: 'dashboard.crm.layout.preset.revenue', icon: 'pi pi-wallet' },
  { id: 'front_desk', labelKey: 'dashboard.crm.layout.preset.front_desk', icon: 'pi pi-calendar' },
  { id: 'marketing', labelKey: 'dashboard.crm.layout.preset.marketing', icon: 'pi pi-megaphone' },
];

const H: DashboardSize = 'half';

const SLOTS: Record<Exclude<DashboardPresetId, 'overview'>, PresetSlots> = {
  // Chủ phòng khám / kế toán: tiền và dịch vụ trước, vận hành trong ngày ở cột phụ.
  revenue: {
    top: [['payment_in_month']],
    main: [
      ['payment_branch_overview'], ['service_statistics'],
      ['service_type_accepted', H], ['service_type_interest', H],
      ['quick_stats'], ['appointment_accepted_stats'],
      ['customer_region', H], ['appointment_status', H],
    ],
    side: [
      ['overview_appointment'], ['additional_stats'], ['customer_status_overview'], ['source_overview'],
    ],
  },
  // Lễ tân / quản lý ca: lịch hẹn và trạng thái tư vấn trong ngày trước, tiền xuống dưới.
  front_desk: {
    top: [['quick_stats']],
    main: [
      ['appointment_accepted_stats'], ['overview_appointment'],
      ['appointment_status', H], ['customer_status_overview', H],
      ['payment_in_month'], ['payment_branch_overview'],
      ['source_overview', H], ['service_type_interest', H],
      ['service_statistics'],
    ],
    side: [
      ['additional_stats'], ['customer_region'], ['service_type_accepted'],
    ],
  },
  // Marketing: nguồn khách, nhóm dịch vụ quan tâm/đồng ý và tiến trình khách trước; doanh thu và lịch hẹn làm bối cảnh.
  marketing: {
    top: [['source_overview']],
    main: [
      ['service_type_interest', H], ['service_type_accepted', H],
      ['customer_status_overview', H], ['customer_region', H],
      ['quick_stats'], ['appointment_accepted_stats'], ['payment_in_month'],
      ['service_statistics'], ['payment_branch_overview'],
    ],
    side: [
      ['appointment_status'], ['overview_appointment'], ['additional_stats'],
    ],
  },
};

/** Bố cục của 1 mẫu: đủ mọi khối, đều hiện. Khối registry có mà mẫu chưa liệt kê (khối thêm sau) ⇒ cuối vùng mặc định. */
export function presetLayout(id: DashboardPresetId, registry: readonly DashboardWidgetDef[]): DashboardLayoutConfig {
  if (id === 'overview') return fullLayout(registry);
  const slots = SLOTS[id];
  const known = new Set(registry.map((w) => w.key));
  // Gom MỌI key có slot trước: khối mặc định ở main nhưng mẫu xếp ở side không được bị vòng "khối thêm sau" chèn vào main.
  const slotted = new Set([...slots.top, ...slots.main, ...slots.side].map(([k]) => k));
  const placed = new Set<string>();
  const items: DashboardLayoutConfig['items'] = [];
  for (const zone of ['top', 'main', 'side'] as const satisfies readonly DashboardZone[]) {
    for (const [key, size] of slots[zone]) {
      if (!known.has(key) || placed.has(key)) continue;
      placed.add(key);
      items.push({ key, zone, visible: true, size: zone === 'main' ? (size ?? 'full') : 'full' });
    }
    for (const w of registry) {
      if (slotted.has(w.key) || placed.has(w.key) || w.defaultZone !== zone) continue;
      placed.add(w.key);
      items.push({ key: w.key, zone, visible: true, size: w.defaultSize });
    }
  }
  return { version: DASHBOARD_LAYOUT_VERSION, items };
}

/** Mã slot thô của mẫu — chỉ cho test kiểm đủ khối / cặp ½. */
export function presetSlots(id: Exclude<DashboardPresetId, 'overview'>): PresetSlots {
  return SLOTS[id];
}
