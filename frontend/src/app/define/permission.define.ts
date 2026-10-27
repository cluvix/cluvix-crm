export const PERMISSION_SELECT = {
  // ─── cxCrm — Vận hành KH & Lịch hẹn ────────────────────────────────
  permissionViewCustomer: 'viewCustomer', // Khách hàng — Danh sách
  permissionCreateCustomer: 'createCustomer', // Khách hàng — Tạo mới
  permissionUpdateCustomer: 'updateCustomer', // Khách hàng — Cập nhật
  permissionExtCustomer: 'extCustomer', // Khách hàng — Mở rộng
  permissionDeleteCustomer: 'deleteCustomer', // Khách hàng — Xóa
  permissionViewAppointment: 'viewAppointment', // Lịch hẹn — Danh sách
  permissionCreateAppointment: 'createAppointment', // Lịch hẹn — Tạo mới
  permissionUpdateAppointment: 'updateAppointment', // Lịch hẹn — Cập nhật
  permissionExtAppointment: 'extAppointment', // Lịch hẹn — Mở rộng
  permissionDeleteAppointment: 'deleteAppointment', // Lịch hẹn — Xóa
  permissionViewNote: 'viewNote', // Ghi chú — Danh sách
  permissionCreateNote: 'createNote', // Ghi chú — Tạo mới
  permissionUpdateNote: 'updateNote', // Ghi chú — Cập nhật
  permissionDeleteNote: 'deleteNote', // Ghi chú — Xóa
  permissionCreateImage: 'createImage', // Quản lý hình ảnh khách hàng — Tạo mới
  permissionViewHistory: 'viewHistory', // Lịch sử — Danh sách
  permissionSearchCustomerGlobal: 'searchCustomerGlobal', // Tìm kiếm khách hàng Global — Danh sách
  permissionViewReminder: 'viewReminder', // Nhắc nhở — Danh sách
  permissionCreateReminder: 'createReminder', // Nhắc nhở — Tạo mới
  permissionUpdateReminder: 'updateReminder', // Nhắc nhở — Cập nhật
  permissionDeleteReminder: 'deleteReminder', // Nhắc nhở — Xóa
  permissionViewServiceUsed: 'viewServiceUsed', // Dịch vụ đã dùng — Danh sách
  permissionLockServiceUsed: 'lockServiceUsed', // Khoá dịch vụ đã dùng — Danh sách
  permissionExtDeposit: 'extDeposit', // Đặt cọc — Mở rộng
  permissionExtRefundDeposit: 'extRefundDeposit', // Hoàn cọc — Mở rộng
  permissionExtRefundService: 'extRefundService', // Hoàn dịch vụ — Mở rộng
  permissionViewPlanOrder: 'viewPlanOrder', // Đơn hàng - Dịch vụ — Danh sách
  permissionCreatePlanOrder: 'createPlanOrder', // Đơn hàng - Dịch vụ — Tạo mới
  permissionUpdatePlanOrder: 'updatePlanOrder', // Đơn hàng - Dịch vụ — Cập nhật
  permissionDeletePlanOrder: 'deletePlanOrder', // Đơn hàng - Dịch vụ — Xóa
  permissionViewPayment: 'viewPayment', // Thanh toán — Danh sách
  permissionCreatePayment: 'createPayment', // Thanh toán — Tạo mới
  permissionUpdatePayment: 'updatePayment', // Thanh toán — Cập nhật
  permissionExtPayment: 'extPayment', // Thanh toán — Mở rộng
  permissionDeletePayment: 'deletePayment', // Thanh toán — Xóa
  permissionManageCrmReport: 'manageCrmReport', // Báo cáo CRM — Quản lý

  // ─── Nhân sự — Nhân viên & Vai trò ────────────────────────────────
  permissionViewUser: 'viewUser', // Nhân viên — Danh sách
  permissionCreateUser: 'createUser', // Nhân viên — Tạo mới
  permissionUpdateUser: 'updateUser', // Nhân viên — Cập nhật
  permissionExtUser: 'extUser', // Nhân viên — Mở rộng
  permissionDeleteUser: 'deleteUser', // Nhân viên — Xóa
  permissionViewRole: 'viewRole', // Vai trò — Danh sách
  permissionCreateRole: 'createRole', // Vai trò — Tạo mới
  permissionUpdateRole: 'updateRole', // Vai trò — Cập nhật
  permissionDeleteRole: 'deleteRole', // Vai trò — Xóa

  // ─── Cấu hình ────────────────────────────────
  permissionManageConfig: 'manageConfig', // Cấu hình — Quản lý

  // ─── Group Facebook (bmad/fb-group/thiet-ke.md) ────────────────────────────────
  permissionViewFbGroup: 'viewFbGroup', // Group Facebook — Danh sách
  permissionUpdateFbGroup: 'updateFbGroup', // Group Facebook — Cập nhật
  permissionManageFbGroupTopic: 'manageFbGroupTopic', // Chủ đề group Facebook — Quản lý
};

// CRM_MODULE_PERMISSIONS — tập 3 quyền OR-gate "quyền module CRM". FE từng dùng cho ô "/crm" trong
// lưới module (system-module-menu.ts) và tile CRM trên home — cả hai đã xoá 2026-10-17 (AppLayout là
// layout duy nhất, '/' vào thẳng dashboard). Giữ hằng làm bản đối chiếu: đổi tập này PHẢI sửa đồng
// thời cả 3 nơi còn lại (đúng thứ tự, đúng 3 quyền):
//  1. backend/internal/modules/webapp/auth/rbac/seed/crm_module_perm_set_test.go (crmModulePermCSV)
//  2. mobile/packages/core_domain/lib/src/permissions.dart (kPermsCrmModule)
//  3. backend/internal/modules/webapp/auth/rbac/seed/route_perm_map.json (entry GET
//     /api/crm/crm-dashboard/* + /api/mobile/v1/crm/*, kind=perm)
export const CRM_MODULE_PERMISSIONS = [
  PERMISSION_SELECT.permissionViewAppointment,
  PERMISSION_SELECT.permissionViewCustomer,
  PERMISSION_SELECT.permissionViewPlanOrder,
];

export interface PermissionMatrixItem {
  value: string | null;
  label: string;
  labelKey?: string;
  type: number;
  parent: number;
}

export interface PermissionMatrixModule {
  id: number;
  name: string;
  nameKey?: string;
  appModule: string;
  typeModule: string;
  typeModuleKey?: string;
  viewMode?: 0 | 1;
  permissions: PermissionMatrixItem[];
}
