package handler

import (
	"strconv"

	"erpCluvix/internal/middleware"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
)

// permChangeService — user chốt 2026-07-20: dùng LẠI quyền "updatePlanOrder" hiện có,
// KHÔNG tạo permission riêng (tránh vỡ ma trận phân quyền 5-cột cứng ở user-role.ts).
const permChangeService = "updatePlanOrder"

// ChangeServiceForOSU PUT /crm/order-plan/service-used/:id/change-service
//
// order-service-change story-03: đổi service_id của 1 order_service_used đã có payment,
// CÓ KIỂM SOÁT — endpoint TÁCH BIỆT khỏi UpdateOrderPlan (handleServices()
// selective-merge không phù hợp cho hành động có side-effect riêng: snapshot lịch sử,
// resync kho, audit log — xem service.ChangeServiceForOSU).
//
// Permission guard TƯỜNG MINH ngay tại đây, KHÔNG dựa vào PermissionEnforcer chung —
// enforcer đang audit-mode toàn cục (PERM_ENFORCE=false mặc định → chỉ log would-deny,
// không chặn thật, xem internal/middleware/permission.go:decide()). Endpoint này sửa
// dữ liệu tài chính đã phát sinh giao dịch nên PHẢI luôn bị chặn thật khi thiếu quyền,
// bất kể cấu hình rollout permission chung của hệ thống.
func (h *Handler) ChangeServiceForOSU(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		response.Validate(c, i18n.T(c, "id_required"))
		return
	}

	var req struct {
		NewServiceId int    `json:"new_service_id" binding:"required"`
		Reason       string `json:"reason" binding:"max=500"`
		UpdatedAt    *int64 `json:"updated_at"` // khoá lạc quan (D6); nil = client không gửi
	}
	if err := helpers.BindJSON(c, &req); err != nil {
		response.ValidateErr(c, err)
		return
	}

	uid := middleware.GetUserID(c)
	if h.RbacSvc != nil && !h.RbacSvc.IsSuperUser(uid) {
		perms, _ := h.RbacSvc.GetUserPermissions(uid)
		if !helpers.StringInSlice(perms, permChangeService) {
			c.JSON(403, gin.H{"message": i18n.T(c, "service_change_permission_denied")})
			return
		}
	}

	scopedDB := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_service_used", true)

	updated, err := h.svc.ChangeServiceForOSU(scopedDB, uint(id), req.NewServiceId, req.Reason, req.UpdatedAt, uid)
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.Updated(c, updated)
}
