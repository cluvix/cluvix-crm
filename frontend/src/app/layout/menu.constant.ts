import { PERMISSION_SELECT } from '@/define';
import { isSuperUser } from '@/core/services/auth';
import { userPermissions } from '@/core/services/auth/auth.state';
import { CONFIG_MENU, ConfigMenuGroup } from '@/modules/config/config/config-menu.registry';
import { isItemVisible, PermissionContext } from '@/modules/config/config/config-menu-search';
import { environment } from '@env/environment';

/**
 * Menu tree gốc — dùng chung cho AppMenu (sidebar) và CommandMenu (Cmd/Ctrl+K).
 *
 * 2026-10-17: CRM thuần cho SME — AppLayout là layout DUY NHẤT (bỏ trang home lưới module và
 * HrLayout/LogLayout/ConfigLayout). Các nhóm Nhân sự / Lịch sử / Cấu hình trước nằm ở sidebar riêng
 * của từng layout đó, nay gộp vào đây. Hàm đọc signal quyền (isSuperUser, userPermissions) nên PHẢI
 * được gọi trong reactive context (effect/computed) để menu tự dựng lại khi quyền đổi.
 */
export function buildMenuTree(): any[] {
  return [
    {
      label: 'Bảng điều khiển',
      labelKey: 'common.label.dashboard',
      icon: 'pi pi-home',
      iconColor: '#7B3F9E',
      routerLinkExact: false,
      items: [
        {
          label: 'Bảng điều khiển',
          labelKey: 'common.label.dashboard',
          icon: 'pi pi-home',
          iconColor: '#7B3F9E',
          routerLink: ['/'],
          routerLinkExact: true,
        },
      ],
    },
    {
      label: 'Lịch hẹn',
      labelKey: 'common.label.appointment',
      icon: 'pi pi-calendar',
      iconColor: '#3B82F6',
      routerLinkExact: false,
      items: [
        {
          label: 'Danh sách lịch hẹn',
          labelKey: 'layout.app_menu.appointment_list',
          icon: 'pi pi-list',
          iconColor: '#3B82F6',
          routerLink: ['/appointment'],
          permission: PERMISSION_SELECT.permissionViewAppointment,
        },
        {
          label: 'Phân bố theo ngày',
          labelKey: 'layout.app_menu.appointment_by_day',
          icon: 'pi pi-calendar-plus',
          iconColor: '#6366F1',
          routerLink: ['/appointment/appointment-schedule'],
          permission: PERMISSION_SELECT.permissionViewAppointment,
        },
        {
          label: 'Phân bố theo giờ',
          labelKey: 'layout.app_menu.appointment_by_hour',
          icon: 'pi pi-clock',
          iconColor: '#8B5CF6',
          routerLink: ['/appointment/appointment-schedule-byhour'],
          permission: PERMISSION_SELECT.permissionViewAppointment,
        },
      ]
    },
    {
      label: 'Khách hàng',
      labelKey: 'common.label.customer',
      icon: 'pi pi-th-large',
      iconColor: '#F59E0B',
      routerLinkExact: false,
      items: [
        {
          label: 'Quản lý khách hàng',
          labelKey: 'common.label.customer_management',
          icon: 'pi pi-users',
          iconColor: '#F59E0B',
          routerLink: ['/customer'],
          permission: PERMISSION_SELECT.permissionViewCustomer,
        },
        {
          label: 'Quản lý đơn hàng',
          labelKey: 'layout.app_menu.order_management',
          icon: 'pi pi-shopping-cart',
          iconColor: '#F59E0B',
          routerLink: ['/order-plan'],
          permission: PERMISSION_SELECT.permissionViewPlanOrder,
        },
        {
          label: 'Thanh toán',
          labelKey: 'common.label.payment',
          icon: 'pi pi-wallet',
          iconColor: '#10B981',
          routerLinkExact: false,
          items: [
            {
              label: 'Phiếu thanh toán',
              labelKey: 'crm.payment_list.title.payment',
              icon: 'pi pi-wallet',
              iconColor: '#10B981',
              routerLink: ['/payment'],
              permission: PERMISSION_SELECT.permissionViewPayment,
            },
            {
              label: 'Phiếu thu/chi',
              labelKey: 'crm.payment_list.title.receipts',
              icon: 'pi pi-arrow-right-arrow-left',
              iconColor: '#3B82F6',
              routerLink: ['/payment/receipts'],
              permission: PERMISSION_SELECT.permissionViewPayment,
            },
            {
              label: 'Phiếu đặt cọc',
              labelKey: 'crm.payment_list.title.deposit',
              icon: 'pi pi-money-bill',
              iconColor: '#F59E0B',
              routerLink: ['/payment/deposit'],
              permission: PERMISSION_SELECT.permissionViewPayment,
            },
            {
              label: 'Phiếu ký gửi',
              labelKey: 'crm.payment_list.title.consignment',
              icon: 'pi pi-box',
              iconColor: '#8B5CF6',
              routerLink: ['/payment/consignment'],
              permission: PERMISSION_SELECT.permissionViewPayment,
            },
            {
              label: 'Phiếu hoàn cọc',
              labelKey: 'crm.payment_list.title.refund_deposit',
              icon: 'pi pi-replay',
              iconColor: '#EF4444',
              routerLink: ['/payment/refund-deposit'],
              permission: PERMISSION_SELECT.permissionViewPayment,
            },
            {
              label: 'Phiếu hoàn dịch vụ',
              labelKey: 'crm.payment_list.title.refund_service',
              icon: 'pi pi-undo',
              iconColor: '#EC4899',
              routerLink: ['/payment/refund-service'],
              permission: PERMISSION_SELECT.permissionViewPayment,
            },
          ],
        },
        {
          label: 'Quản lý lời nhắc',
          labelKey: 'layout.app_menu.reminder_management',
          icon: 'pi pi-bell',
          iconColor: '#D97706',
          routerLink: ['/reminder'],
          routerLinkExact: false,
          permission: PERMISSION_SELECT.permissionViewReminder,
        },
      ],
    },
    // Group Facebook (bmad/fb-group/thiet-ke.md) — màu kênh Facebook có sẵn ở token --cx-ch-fb.
    {
      label: 'Group Facebook',
      labelKey: 'fbgroup.menu.root',
      icon: 'pi pi-facebook',
      iconColor: '#1877F2',
      routerLinkExact: false,
      items: [
        {
          label: 'Thống kê',
          labelKey: 'fbgroup.menu.stats',
          icon: 'pi pi-chart-bar',
          iconColor: '#7B3F9E',
          routerLink: ['/fb-group/stats'],
          permission: PERMISSION_SELECT.permissionViewFbGroup,
        },
        {
          label: 'Danh sách group',
          labelKey: 'fbgroup.menu.groups',
          icon: 'pi pi-list',
          iconColor: '#1877F2',
          routerLink: ['/fb-group/groups'],
          permission: PERMISSION_SELECT.permissionViewFbGroup,
        },
        {
          label: 'Chủ đề',
          labelKey: 'fbgroup.menu.topics',
          icon: 'pi pi-tags',
          iconColor: '#FF5722',
          routerLink: ['/fb-group/topics'],
          permission: PERMISSION_SELECT.permissionViewFbGroup,
        },
      ],
    },
    // Chuyển từ layout/hr/hr.menu.ts (HrLayout đã xoá).
    {
      label: 'Quản lý nhân sự',
      labelKey: 'layout.hr_menu.staff_root',
      icon: 'pi pi-users',
      iconColor: '#818CF8',
      routerLinkExact: false,
      items: [
        {
          label: 'Danh sách nhân viên',
          labelKey: 'common.label.staff_list',
          icon: 'pi pi-list',
          iconColor: '#6366F1',
          routerLink: ['/hr/users'],
          permission: PERMISSION_SELECT.permissionViewUser,
        },
        {
          label: 'Phân quyền',
          labelKey: 'common.label.permission',
          icon: 'pi pi-id-card',
          iconColor: '#8B1538',
          routerLink: ['/hr/roles'],
          permission: PERMISSION_SELECT.permissionViewRole,
        },
      ],
    },
    buildAdminMenu(),
  ];
}

/**
 * Nhóm "Quản trị" (user chốt 2026-10-18): gộp Báo cáo + Cấu hình hệ thống + Lịch sử vào 1 nhóm
 * thay vì 3 nhóm riêng. Lịch sử là menu con (7 mục). Mục nào user không có quyền bị lọc bởi
 * menu-permission.util; nhóm rỗng hoàn toàn thì bị ẩn.
 */
function buildAdminMenu(): any {
  return {
    label: 'Quản trị',
    labelKey: 'layout.app_menu.admin_group',
    icon: 'pi pi-briefcase',
    iconColor: '#64748B',
    routerLinkExact: false,
    items: [
      {
        label: 'Danh sách báo cáo',
        labelKey: 'common.label.report_list',
        icon: 'pi pi-chart-bar',
        iconColor: '#E53935',
        routerLink: ['/report'],
        // Sáng cả khi đang xem 1 báo cáo con /report/... (menuitem mặc định so khớp exact).
        routerLinkActiveOptions: { paths: 'subset', queryParams: 'ignored', matrixParams: 'ignored', fragment: 'ignored' },
        permission: PERMISSION_SELECT.permissionManageCrmReport,
      },
      ...buildConfigMenu(),
      buildLogMenu(),
    ],
  };
}

/**
 * Mục Cấu hình (trong nhóm Quản trị) — CHỈ 1 mục mở trang /config (user chốt 2026-10-17, giống ERP): danh sách đầy đủ
 * các cấu hình + ô tìm kiếm nằm ở trang đó, KHÔNG trải từng mục lên sidebar.
 * Mục chỉ hiện khi user thấy được ít nhất 1 cấu hình — lọc bằng chính `isItemVisible` của trang
 * /config (quyền panel + quyền item + superUserOnly) để sidebar và trang không lệch nhau.
 * Gate `environment.option >= 1` giữ nguyên từ nút /config trong lưới module cũ.
 */
function buildConfigMenu(): any[] {
  if (environment.option < 1) return [];

  const ctx: PermissionContext = { isSuperUser: isSuperUser(), granted: userPermissions() };
  const hasVisibleConfig = CONFIG_MENU.some((group: ConfigMenuGroup) =>
    group.items.some((item) => isItemVisible(group, item, ctx)),
  );
  if (!hasVisibleConfig) return [];

  return [
    {
      label: 'Cấu hình hệ thống',
      labelKey: 'common.label.system_configuration',
      icon: 'pi pi-cog',
      iconColor: '#8B1538',
      routerLink: ['/config'],
      // Sáng cả khi đang ở trang con /config/... (menuitem mặc định so khớp exact).
      routerLinkActiveOptions: { paths: 'subset', queryParams: 'ignored', matrixParams: 'ignored', fragment: 'ignored' },
    },
  ];
}

/**
 * Menu con Lịch sử (trong nhóm Quản trị) — chuyển từ layout/log/log.menu.ts (LogLayout đã xoá).
 * Lịch sử đăng nhập chỉ hiện cho super-user (BE cũng enforce 403). Không gắn `permission` để tránh
 * non-super có permissionViewHistory thấy menu rồi bị 403.
 */
function buildLogMenu(): any {
  const loginHistoryItem = isSuperUser()
    ? [
        {
          label: 'Lịch sử đăng nhập',
          labelKey: 'layout.log_menu.login',
          icon: 'pi pi-sign-in',
          iconColor: '#EF4444',
          routerLink: ['/log/login'],
        },
      ]
    : [];

  return {
    label: 'Lịch sử',
    labelKey: 'common.label.history',
    icon: 'pi pi-eject',
    iconColor: '#64748B',
    routerLinkExact: false,
    items: [
      {
        label: 'Lịch sử export',
        labelKey: 'layout.log_menu.export',
        icon: 'pi pi-file-export',
        iconColor: '#64748B',
        routerLink: ['/log/export'],
        permission: PERMISSION_SELECT.permissionViewHistory,
      },
      ...loginHistoryItem,
      {
        label: 'Lịch sử khách hàng',
        labelKey: 'layout.log_menu.customer',
        icon: 'pi pi-history',
        iconColor: '#3B82F6',
        routerLink: ['/log/customer'],
        permission: PERMISSION_SELECT.permissionViewHistory,
      },
      {
        label: 'Lịch sử lịch hẹn',
        labelKey: 'layout.log_menu.appointment',
        icon: 'pi pi-history',
        iconColor: '#8B5CF6',
        routerLink: ['/log/appointment'],
        permission: PERMISSION_SELECT.permissionViewHistory,
      },
      {
        label: 'Lịch sử thanh toán',
        labelKey: 'common.label.payment_history',
        icon: 'pi pi-history',
        iconColor: '#10B981',
        routerLink: ['/log/payment'],
        permission: PERMISSION_SELECT.permissionViewHistory,
      },
      {
        label: 'Lịch sử đơn hàng',
        labelKey: 'layout.log_menu.order_service_used',
        icon: 'pi pi-history',
        iconColor: '#F59E0B',
        routerLink: ['/log/order-service-used'],
        permission: PERMISSION_SELECT.permissionViewHistory,
      },
      {
        label: 'Lịch sử người dùng',
        labelKey: 'layout.log_menu.user',
        icon: 'pi pi-history',
        iconColor: '#0EA5E9',
        routerLink: ['/log/user'],
        permission: PERMISSION_SELECT.permissionViewHistory,
      },
    ],
  };
}
