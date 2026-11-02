import { PERMISSION_SELECT } from '@/define';

/**
 * Registry menu trang /config — NGUỒN DỮ LIỆU DUY NHẤT cho cả 3 cột panel lẫn thanh tìm kiếm.
 *
 * Trước đây 23 mảng nằm rải trong `Config` class còn thứ tự/cột nằm trong `config.html`, nên
 * không có chỗ nào để search đi qua. Thêm mục mới: chỉ sửa file này.
 */
export interface ConfigMenuItem {
  /** Nhãn tiếng Việt gốc — vẫn dùng làm fallback khi thiếu key i18n, và được đánh chỉ số search. */
  label: string;
  labelKey?: string;
  link: string;
  icon?: string;
  iconClass?: string;
  queryParams?: Record<string, unknown>;
  /** Quyền RIÊNG của item, chặt hơn quyền mở panel cha. Không set = theo quyền panel. */
  permission?: string | string[];
  /** Chỉ super-user thấy (server cũng enforce 403). */
  superUserOnly?: boolean;
  /** Từ khoá phụ cho search: tên gọi khác, viết tắt, tiếng Anh. KHÔNG hiển thị trên UI. */
  keywords?: string[];
}

export interface ConfigMenuGroup {
  key: string;
  title: string;
  titleKey: string;
  icon: string;
  /** Quyền mở cả panel. */
  permission: string | string[];
  /** Cột hiển thị khi KHÔNG tìm kiếm (giữ nguyên bố cục cũ của config.html). */
  column: 1 | 2 | 3;
  items: ConfigMenuItem[];
}

export const CONFIG_MENU: ConfigMenuGroup[] = [
  {
    key: 'system',
    title: 'Hệ thống',
    titleKey: 'config.nav.panel.system',
    icon: 'pi pi-cog',
    permission: PERMISSION_SELECT.permissionManageConfig,
    column: 1,
    items: [
      {
        label: 'Hồ sơ doanh nghiệp',
        labelKey: 'config.business_profile.title',
        link: '/config/company',
        icon: 'pi pi-building',
        iconClass: 'text-blue-600',
        keywords: ['thuong hieu', 'cong ty', 'doanh nghiep', 'brand', 'ho so'],
      },
      {
        label: 'Chi nhánh',
        labelKey: 'common.label.branch',
        link: '/config/branch',
        icon: 'pi pi-sitemap',
        iconClass: 'text-green-600',
        keywords: ['co so', 'chi nhanh', 'branch'],
      },
      {
        label: 'Mã định danh',
        labelKey: 'common.label.identifier_code',
        link: '/config/config_code',
        icon: 'pi pi-hashtag',
        iconClass: 'text-purple-600',
      },
      {
        label: 'Lựa chọn tác vụ',
        labelKey: 'config.nav.system.task_options',
        link: '/config/option',
        icon: 'pi pi-list',
        iconClass: 'text-indigo-600',
        keywords: ['user option', 'tuy chon', 'option'],
      },
      {
        label: 'Đơn vị tính',
        labelKey: 'common.label.unit',
        link: '/config/unit',
        icon: 'pi pi-calculator',
        iconClass: 'text-teal-600',
        keywords: ['dvt', 'unit'],
      },
      {
        label: 'Hằng số',
        labelKey: 'config.nav.system.constants',
        link: '/config/setting',
        icon: 'pi pi-sliders-h',
        iconClass: 'text-gray-600',
        keywords: ['cau hinh he thong', 'tham so', 'setting'],
      },
      {
        // Nhật ký bảo mật chỉ hiện cho super-user (server cũng enforce 403).
        label: 'Nhật ký bảo mật',
        labelKey: 'config.security_audit.title',
        link: '/config/security-audit',
        icon: 'pi pi-shield',
        iconClass: 'text-rose-600',
        superUserOnly: true,
        keywords: ['audit', 'security', 'log bao mat'],
      },

    ],
  },
  {
    key: 'appointment',
    title: 'Lịch hẹn',
    titleKey: 'common.label.appointment',
    icon: 'pi pi-calendar',
    permission: PERMISSION_SELECT.permissionManageConfig,
    column: 3,
    items: [
      {
        label: 'Loại lịch hẹn',
        labelKey: 'common.label.appointment_type',
        link: '/config/appointment-type',
        icon: 'pi pi-calendar',
        iconClass: 'text-emerald-600',
      },
      {
        label: 'Trạng thái tư vấn hẹn',
        labelKey: 'config.nav.appointment.consultation_status',
        link: '/config/appointment-is-accepted',
        icon: 'pi pi-check-circle',
        iconClass: 'text-green-600',
      },
      {
        label: 'Lí do hẹn không đồng ý',
        labelKey: 'config.nav.appointment.rejection_reason',
        link: '/config/appointment-reason',
        icon: 'pi pi-times-circle',
        iconClass: 'text-red-600',
      },
      {
        label: 'Lý do chưa đặt lịch',
        labelKey: 'common.label.not_booked_reason',
        link: '/config/appointment-not-booked-reasons',
        icon: 'pi pi-calendar-plus',
        iconClass: 'text-orange-600',
      },
    ],
  },
  {
    key: 'account',
    title: 'Kế toán',
    titleKey: 'common.label.accounting',
    icon: 'pi pi-wallet',
    permission: PERMISSION_SELECT.permissionManageConfig,
    column: 1,
    items: [
      {
        label: 'Phương thức thanh toán',
        labelKey: 'config.nav.account.payment_method',
        link: '/config/card-type',
        icon: 'pi pi-credit-card',
        iconClass: 'text-blue-600',
        keywords: ['quet the', 'the', 'payment'],
      },
      {
        label: 'Hình thức thanh toán',
        labelKey: 'config.nav.account.payment_form',
        link: '/config/payment-method',
        icon: 'pi pi-wallet',
        iconClass: 'text-green-600',
        keywords: ['tien mat', 'chuyen khoan', 'payment'],
      },
      {
        label: 'Tài khoản ngân hàng',
        labelKey: 'config.nav.account.bank_account',
        link: '/config/bank-account',
        icon: 'pi pi-building-columns',
        iconClass: 'text-purple-600',
      },
    ],
  },
  {
    key: 'customForm',
    title: 'Customize Form',
    titleKey: 'config.nav.panel.customize_form',
    icon: 'pi pi-sliders-h',
    permission: PERMISSION_SELECT.permissionManageConfig,
    column: 2,
    items: [
      {
        label: 'Form Customer',
        labelKey: 'config.nav.custom_form.customer',
        link: '/config/customize/form-customer',
        icon: 'pi pi-user',
        iconClass: 'text-indigo-600',
      },
      {
        label: 'Form Lead',
        labelKey: 'config.nav.custom_form.lead',
        link: '/config/customize/form-lead',
        icon: 'pi pi-user-plus',
        iconClass: 'text-purple-600',
      },
    ],
  },
  {
    key: 'customer',
    title: 'Khách hàng',
    titleKey: 'common.label.customer',
    icon: 'pi pi-users',
    permission: PERMISSION_SELECT.permissionManageConfig,
    column: 2,
    items: [
      {
        label: 'Mẫu ghi chú',
        labelKey: 'config.nav.marketing.note_template',
        link: '/config/note-template',
        icon: 'pi pi-book',
        iconClass: 'text-blue-600',
      },
      {
        label: 'Nguồn khách hàng',
        labelKey: 'common.label.source',
        link: '/config/customer-source',
        icon: 'pi pi-share-alt',
        iconClass: 'text-green-600',
        keywords: ['utm', 'kenh khach den', 'source', 'khach den tu dau'],
      },
      {
        label: 'Nhóm nguồn khách hàng',
        labelKey: 'common.label.customer_source_group',
        link: '/config/group-source',
        icon: 'pi pi-sitemap',
        iconClass: 'text-teal-600',
        keywords: ['nhom utm', 'source group'],
      },
      {
        label: 'Nghề nghiệp',
        labelKey: 'config.nav.marketing.customer_job',
        link: '/config/customer-job',
        icon: 'pi pi-briefcase',
        iconClass: 'text-yellow-600',
      },
      {
        label: 'Dân tộc',
        labelKey: 'config.nav.marketing.customer_ethnicity',
        link: '/config/customer-ethnicity',
        icon: 'pi pi-flag',
        iconClass: 'text-teal-600',
      },
      {
        label: 'Nhóm khách hàng',
        labelKey: 'common.label.customer_category',
        link: '/config/customer-category',
        icon: 'pi pi-users',
        iconClass: 'text-cyan-600',
      },
      {
        label: 'Tiến trình khách hàng',
        labelKey: 'config.customer_stage.title',
        link: '/config/customer-stage',
        icon: 'pi pi-flag',
        iconClass: 'text-purple-600',
        keywords: ['pipeline', 'buoc khach hang', 'giai doan', 'stage'],
      },
    ],
  },
  {
    key: 'medicalForm',
    title: 'Phiếu mẫu',
    titleKey: 'config.nav.panel.medical_form',
    icon: 'pi pi-file-edit',
    permission: PERMISSION_SELECT.permissionManageConfig,
    column: 1,
    items: [
      {
        label: 'Phiếu thanh toán',
        labelKey: 'common.label.payment_receipt',
        link: '/config/print-template',
        queryParams: { type: 'payment' },
        icon: 'pi pi-receipt',
        iconClass: 'text-green-600',
      },
    ],
  },
  {
    key: 'hr',
    title: 'Nhân sự',
    titleKey: 'common.label.hr',
    icon: 'pi pi-users',
    permission: PERMISSION_SELECT.permissionManageConfig,
    column: 2,
    items: [
      {
        label: 'Phòng ban',
        labelKey: 'common.label.department',
        link: '/config/departments',
        icon: 'pi pi-building',
        iconClass: 'text-yellow-600',
        keywords: ['bo phan', 'department'],
      },
    ],
  },
  {
    key: 'location',
    title: 'Location',
    titleKey: 'config.nav.panel.location',
    icon: 'pi pi-map',
    permission: PERMISSION_SELECT.permissionManageConfig,
    column: 3,
    items: [
      {
        label: 'Khu vực trong nước',
        labelKey: 'common.label.domestic_area',
        link: '/config/group-local',
        icon: 'pi pi-map',
        iconClass: 'text-green-600',
      },
      {
        label: 'Khu vực quốc tế',
        labelKey: 'common.label.international_area',
        link: '/config/group-country',
        icon: 'pi pi-globe',
        iconClass: 'text-blue-600',
      },
      {
        label: 'Quốc gia',
        labelKey: 'common.label.country',
        link: '/config/location-country',
        icon: 'pi pi-flag',
        iconClass: 'text-red-600',
      },
      {
        label: 'Tỉnh/Thành phố',
        labelKey: 'common.label.province_city',
        link: '/config/location-province',
        icon: 'pi pi-map-marker',
        iconClass: 'text-orange-600',
        keywords: ['tinh', 'thanh pho', 'province'],
      },
      {
        label: 'Phường/Xã',
        labelKey: 'common.label.ward_commune',
        link: '/config/location-ward',
        icon: 'pi pi-home',
        iconClass: 'text-purple-600',
        keywords: ['phuong', 'xa', 'ward'],
      },
    ],
  },
  {
    key: 'clinic',
    title: 'Dịch vụ',
    titleKey: 'common.label.service',
    icon: 'pi pi-briefcase',
    permission: PERMISSION_SELECT.permissionManageConfig,
    column: 3,
    items: [
      {
        label: 'Dịch vụ/ Sản phẩm',
        labelKey: 'common.label.service_product',
        link: '/config/service',
        icon: 'pi pi-briefcase',
        iconClass: 'text-blue-600',
        keywords: ['bang gia', 'gia dich vu', 'service', 'san pham'],
      },
      {
        label: 'Nhóm dịch vụ/ Sản phẩm',
        labelKey: 'common.label.service_product_group',
        link: '/config/service-type',
        icon: 'pi pi-th-large',
        iconClass: 'text-purple-600',
      },
    ],
  },
];
