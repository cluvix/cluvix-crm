import { PERMISSION_SELECT, type PermissionMatrixModule } from './permission.define';

// Tách khỏi `permission.define.ts` (2026-08-28) để KHÔNG vào bundle eager.
// story-22 (+2026-10-19 bỏ 6 quyền thừa): lọc còn đúng 46 quyền catalog BE (permissions.gen.json) — module/dòng không
// còn quyền thật đã bị xoá; dòng CRUD lẻ vẫn giữ `value: null` (placeholder canh lưới) khi
// action đó không có quyền catalog tương ứng nhưng module vẫn còn ít nhất 1 quyền khác.
//
// ⚠ Đừng re-export file này từ barrel `@/define`, và đừng import nó từ code chạy lúc boot —
// làm vậy là matrix quay lại main. Import trực tiếp theo đường dẫn như user-role.ts đang làm.
// Script `perm-audit`/`perm-codegen` đọc CẢ HAI file (xem scripts/perm-define-parse.mjs).

export const PERMISSION_MATRIX: PermissionMatrixModule[] = [
  {
    name: "Khách hàng",
    nameKey: "common.label.customer",
    id: 1,
    appModule: "cxCrm",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewCustomer,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 1
      },
      {
        value: PERMISSION_SELECT.permissionCreateCustomer,
        label: "Tạo mới",
        labelKey: "common.action.create",
        type: 2,
        parent: 1
      },
      {
        value: PERMISSION_SELECT.permissionUpdateCustomer,
        label: "Cập nhật",
        labelKey: "common.action.update",
        type: 3,
        parent: 1
      },
      {
        value: PERMISSION_SELECT.permissionExtCustomer,
        label: "Mở rộng",
        labelKey: "define.permission.action.extend",
        type: 4,
        parent: 1
      },
      {
        value: PERMISSION_SELECT.permissionDeleteCustomer,
        label: "Xóa",
        labelKey: "common.action.delete",
        type: 5,
        parent: 1
      },
    ]
  },
  {
    name: "Lịch hẹn",
    nameKey: "common.label.appointment",
    id: 2,
    appModule: "cxCrm",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewAppointment,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 2
      },
      {
        value: PERMISSION_SELECT.permissionCreateAppointment,
        label: "Tạo mới",
        labelKey: "common.action.create",
        type: 2,
        parent: 2
      },
      {
        value: PERMISSION_SELECT.permissionUpdateAppointment,
        label: "Cập nhật",
        labelKey: "common.action.update",
        type: 3,
        parent: 2
      },
      {
        value: PERMISSION_SELECT.permissionExtAppointment,
        label: "Mở rộng",
        labelKey: "define.permission.action.extend",
        type: 4,
        parent: 2
      },
      {
        value: PERMISSION_SELECT.permissionDeleteAppointment,
        label: "Xóa",
        labelKey: "common.action.delete",
        type: 5,
        parent: 2
      },
    ]
  },
  {
    name: "Đơn hàng - Dịch vụ",
    nameKey: "define.permission.module.order_plan",
    id: 119,
    appModule: "cxCrm",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewPlanOrder,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 119
      },
      {
        value: PERMISSION_SELECT.permissionCreatePlanOrder,
        label: "Tạo mới",
        labelKey: "common.action.create",
        type: 2,
        parent: 119
      },
      {
        value: PERMISSION_SELECT.permissionUpdatePlanOrder,
        label: "Cập nhật",
        labelKey: "common.action.update",
        type: 3,
        parent: 119
      },
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 119},
      {
        value: PERMISSION_SELECT.permissionDeletePlanOrder,
        label: "Xóa",
        labelKey: "common.action.delete",
        type: 5,
        parent: 119
      },
    ]
  },
  {
    name: "Thanh toán",
    nameKey: "common.label.payment",
    id: 120,
    appModule: "cxCrm",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewPayment,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 120
      },
      {
        value: PERMISSION_SELECT.permissionCreatePayment,
        label: "Tạo mới",
        labelKey: "common.action.create",
        type: 2,
        parent: 120
      },
      {
        value: PERMISSION_SELECT.permissionUpdatePayment,
        label: "Cập nhật",
        labelKey: "common.action.update",
        type: 3,
        parent: 120
      },
      {
        value: PERMISSION_SELECT.permissionExtPayment,
        label: "Mở rộng",
        labelKey: "define.permission.action.extend",
        type: 4,
        parent: 120
      },
      {
        value: PERMISSION_SELECT.permissionDeletePayment,
        label: "Xóa",
        labelKey: "common.action.delete",
        type: 5,
        parent: 120
      },
    ]
  },
  {
    name: "Đơn hàng - Dịch vụ",
    nameKey: "define.permission.module.order_plan",
    id: 12,
    appModule: "Kế toán",
    viewMode: 1,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewPlanOrder,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 12
      },
      {value: null, label: "Tạo mới", labelKey: "common.action.create", type: 0, parent: 12},
      {value: null, label: "Cập nhật", labelKey: "common.action.update", type: 0, parent: 12},
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 12},
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 12},
    ]
  },
  {
    name: "Dịch vụ đã dùng",
    nameKey: "define.permission.module.service_used",
    id: 13,
    appModule: "cxCrm",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewServiceUsed,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 13
      },
      {value: null, label: "Tạo mới", labelKey: "common.action.create", type: 0, parent: 13},
      {value: null, label: "Cập nhật", labelKey: "common.action.update", type: 0, parent: 13},
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 13},
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 13},
    ]
  },
  {
    name: "Thanh toán",
    nameKey: "common.label.payment",
    id: 5,
    appModule: "Kế toán",
    viewMode: 1,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewPayment,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 5
      },
      {value: null, label: "Tạo mới", labelKey: "common.action.create", type: 0, parent: 5},
      {value: null, label: "Cập nhật", labelKey: "common.action.update", type: 0, parent: 5},
      {
        value: PERMISSION_SELECT.permissionExtPayment,
        label: "Mở rộng",
        labelKey: "define.permission.action.extend",
        type: 4,
        parent: 5
      },
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 5},
    ]
  },
  {
    name: "Ghi chú",
    nameKey: "common.label.note",
    id: 16,
    appModule: "cxCrm",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewNote,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 16
      },
      {
        value: PERMISSION_SELECT.permissionCreateNote,
        label: "Tạo mới",
        labelKey: "common.action.create",
        type: 2,
        parent: 16
      },
      {
        value: PERMISSION_SELECT.permissionUpdateNote,
        label: "Cập nhật",
        labelKey: "common.action.update",
        type: 3,
        parent: 16
      },
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 16},
      {
        value: PERMISSION_SELECT.permissionDeleteNote,
        label: "Xóa",
        labelKey: "common.action.delete",
        type: 5,
        parent: 16
      },
    ]
  },
  {
    name: "Quản lý hình ảnh khách hàng",
    nameKey: "define.permission.module.customer_image",
    id: 37,
    appModule: "cxCrm",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {value: null, label: "Danh sách", labelKey: "define.permission.action.list", type: 0, parent: 37},
      {
        value: PERMISSION_SELECT.permissionCreateImage,
        label: "Tạo mới",
        labelKey: "common.action.create",
        type: 2,
        parent: 37
      },
      {value: null, label: "Cập nhật", labelKey: "common.action.update", type: 0, parent: 37},
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 37},
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 37},
    ]
  },
  {
    name: "Nhắc nhở",
    nameKey: "define.permission.module.reminder",
    id: 60,
    appModule: "cxCrm",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewReminder,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 60
      },
      {
        value: PERMISSION_SELECT.permissionCreateReminder,
        label: "Tạo mới",
        labelKey: "common.action.create",
        type: 2,
        parent: 60
      },
      {
        value: PERMISSION_SELECT.permissionUpdateReminder,
        label: "Cập nhật",
        labelKey: "common.action.update",
        type: 3,
        parent: 60
      },
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 60},
      {
        value: PERMISSION_SELECT.permissionDeleteReminder,
        label: "Xóa",
        labelKey: "common.action.delete",
        type: 5,
        parent: 60
      },
    ]
  },
  {
    name: "Nhân viên",
    nameKey: "common.label.staff",
    id: 24,
    appModule: "Nhân sự",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewUser,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 24
      },
      {
        value: PERMISSION_SELECT.permissionCreateUser,
        label: "Tạo mới",
        labelKey: "common.action.create",
        type: 2,
        parent: 24
      },
      {
        value: PERMISSION_SELECT.permissionUpdateUser,
        label: "Cập nhật",
        labelKey: "common.action.update",
        type: 3,
        parent: 24
      },
      {
        value: PERMISSION_SELECT.permissionExtUser,
        label: "Mở rộng",
        labelKey: "define.permission.action.extend",
        type: 4,
        parent: 24
      },
      {
        value: PERMISSION_SELECT.permissionDeleteUser,
        label: "Xóa",
        labelKey: "common.action.delete",
        type: 5,
        parent: 24
      },
    ]
  },
  {
    name: "Vai trò",
    nameKey: "common.label.role",
    id: 118,
    appModule: "Nhân sự",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewRole,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 118
      },
      {
        value: PERMISSION_SELECT.permissionCreateRole,
        label: "Tạo mới",
        labelKey: "common.action.create",
        type: 2,
        parent: 118
      },
      {
        value: PERMISSION_SELECT.permissionUpdateRole,
        label: "Cập nhật",
        labelKey: "common.action.update",
        type: 3,
        parent: 118
      },
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 118},
      {
        value: PERMISSION_SELECT.permissionDeleteRole,
        label: "Xóa",
        labelKey: "common.action.delete",
        type: 5,
        parent: 118
      },
    ]
  },
  {
    name: "Báo cáo CRM",
    nameKey: "define.permission.module.crm_report",
    id: 19,
    appModule: "cxCrm",
    viewMode: 1,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionManageCrmReport,
        label: "Quản lý",
        labelKey: "define.permission.action.manage",
        type: 1,
        parent: 19
      },
    ]
  },
  {
    name: "Lịch sử",
    nameKey: "common.label.history",
    id: 29,
    appModule: "cxCrm",
    viewMode: 1,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewHistory,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 29
      },
      {value: null, label: "Tạo mới", labelKey: "common.action.create", type: 0, parent: 29},
      {value: null, label: "Cập nhật", labelKey: "common.action.update", type: 0, parent: 29},
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 29},
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 29},
    ]
  },
  {
    name: "Khoá dịch vụ đã dùng",
    nameKey: "define.permission.module.lock_service_used",
    id: 38,
    appModule: "cxCrm",
    viewMode: 1,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionLockServiceUsed,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 38
      },
      {value: null, label: "Tạo mới", labelKey: "common.action.create", type: 0, parent: 38},
      {value: null, label: "Cập nhật", labelKey: "common.action.update", type: 0, parent: 38},
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 38},
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 38},
    ]
  },
  {
    name: "Tìm kiếm khách hàng Global",
    nameKey: "define.permission.module.search_customer_global",
    id: 40,
    appModule: "cxCrm",
    viewMode: 1,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionSearchCustomerGlobal,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 40
      },
      {value: null, label: "Tạo mới", labelKey: "common.action.create", type: 0, parent: 40},
      {value: null, label: "Cập nhật", labelKey: "common.action.update", type: 0, parent: 40},
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 40},
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 40},
    ]
  },
  {
    name: "Cấu hình",
    nameKey: "common.label.system_configuration",
    id: 28,
    appModule: "Cấu hình",
    viewMode: 0,
    typeModule: "Cấu hình hệ thống",
    typeModuleKey: "common.label.system_config",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionManageConfig,
        label: "Quản lý",
        labelKey: "define.permission.action.manage",
        type: 1,
        parent: 28
      },
    ]
  },
  {
    name: "Group Facebook",
    nameKey: "define.permission.module.fb_group",
    id: 121,
    appModule: "cxCrm",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionViewFbGroup,
        label: "Danh sách",
        labelKey: "define.permission.action.list",
        type: 1,
        parent: 121
      },
      {value: null, label: "Tạo mới", labelKey: "common.action.create", type: 0, parent: 121},
      {
        value: PERMISSION_SELECT.permissionUpdateFbGroup,
        label: "Cập nhật",
        labelKey: "common.action.update",
        type: 3,
        parent: 121
      },
      {value: null, label: "Mở rộng", labelKey: "define.permission.action.extend", type: 0, parent: 121},
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 121},
    ]
  },
  {
    name: "Chủ đề group Facebook",
    nameKey: "define.permission.module.fb_group_topic",
    id: 122,
    appModule: "cxCrm",
    viewMode: 0,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      {
        value: PERMISSION_SELECT.permissionManageFbGroupTopic,
        label: "Quản lý",
        labelKey: "define.permission.action.manage",
        type: 1,
        parent: 122
      },
    ]
  },
  {
    name: "Đặt cọc",
    nameKey: "common.label.deposit",
    id: 103,
    appModule: "Kế toán",
    viewMode: 1,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      // Xem/in phiếu dùng chung viewPayment (2026-10-19 gộp viewDeposit vào "Thanh toán — Danh sách").
      {value: null, label: "Danh sách", labelKey: "define.permission.action.list", type: 0, parent: 103},
      {value: null, label: "Tạo mới", labelKey: "common.action.create", type: 0, parent: 103},
      {value: null, label: "Cập nhật", labelKey: "common.action.update", type: 0, parent: 103},
      {
        value: PERMISSION_SELECT.permissionExtDeposit,
        label: "Mở rộng",
        labelKey: "define.permission.action.extend",
        type: 4,
        parent: 103
      },
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 103},
    ]
  },
  {
    name: "Hoàn cọc",
    nameKey: "common.label.refund_deposit",
    id: 105,
    appModule: "Kế toán",
    viewMode: 1,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      // Xem/in phiếu dùng chung viewPayment (2026-10-19 gộp viewRefundDeposit vào "Thanh toán — Danh sách").
      {value: null, label: "Danh sách", labelKey: "define.permission.action.list", type: 0, parent: 105},
      {value: null, label: "Tạo mới", labelKey: "common.action.create", type: 0, parent: 105},
      {value: null, label: "Cập nhật", labelKey: "common.action.update", type: 0, parent: 105},
      {
        value: PERMISSION_SELECT.permissionExtRefundDeposit,
        label: "Mở rộng",
        labelKey: "define.permission.action.extend",
        type: 4,
        parent: 105
      },
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 105},
    ]
  },
  {
    name: "Hoàn dịch vụ",
    nameKey: "define.permission.module.refund_service",
    id: 106,
    appModule: "Kế toán",
    viewMode: 1,
    typeModule: "Chức năng",
    typeModuleKey: "common.label.feature",
    permissions: [
      // Xem/in phiếu dùng chung viewPayment (2026-10-19 gộp viewRefundService vào "Thanh toán — Danh sách").
      {value: null, label: "Danh sách", labelKey: "define.permission.action.list", type: 0, parent: 106},
      {value: null, label: "Tạo mới", labelKey: "common.action.create", type: 0, parent: 106},
      {value: null, label: "Cập nhật", labelKey: "common.action.update", type: 0, parent: 106},
      {
        value: PERMISSION_SELECT.permissionExtRefundService,
        label: "Mở rộng",
        labelKey: "define.permission.action.extend",
        type: 4,
        parent: 106
      },
      {value: null, label: "Xóa", labelKey: "common.action.delete", type: 0, parent: 106},
    ]
  },
];
