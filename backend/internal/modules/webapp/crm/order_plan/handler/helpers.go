// Helpers — copy từ handler dịch vụ điều trị cũ (duplication intentional cho boundary thật). Long-term có thể extract sang pkg/relation
// nếu nhiều module cần.
package handler

import (
	"time"

	"erpCluvix/internal/middleware"
	otpDao "erpCluvix/internal/modules/webapp/crm/order_plan/dao"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"
	"erpCluvix/pkg/tz"

	"github.com/gin-gonic/gin"
)

// verifyCanEditOrderPlan kiểm tra user có quyền sửa đơn hàng điều trị.
//
// Logic (mirror FE checkOrderPlanUpdate):
//   - Có quyền updatePlanOrder → luôn được phép.
//   - Không có quyền updatePlanOrder → phải đủ CẢ 4 điều kiện:
//     1. Có quyền createPlanOrder (prerequisite).
//     2. user_option id=3 (OPTION_UPDATE_ORDER_PLAN) = 1 (bật).
//     3. Là người tạo ra đơn hàng đó (created_by = userID).
//     4. Vẫn trong cùng ngày tạo (theo timezone của thương hiệu).
//
// Trả về branch_id HIỆN TẠI của đơn để caller chỉ kiểm branch payload khi đổi (orderBranchAllowed).
func (h *Handler) verifyCanEditOrderPlan(c *gin.Context, id uint) (*int, bool) {
	var o otpDao.OrderPlan
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_plan", true).First(&o, id).Error; err != nil {
		response.NotFound(c)
		return nil, false
	}

	userID := middleware.GetUserID(c)

	if h.hasPermission(userID, "updatePlanOrder") {
		return o.BranchId, true
	}

	if !h.hasPermission(userID, "createPlanOrder") {
		response.Forbidden(c)
		return nil, false
	}

	var optionVal int
	h.DB.Table("user_option").Where("id = ?", define.UserOptionUpdateOrderPlan).Limit(1).Pluck("options", &optionVal)
	if optionVal != 1 {
		response.Forbidden(c)
		return nil, false
	}

	if o.CreatedBy == nil || *o.CreatedBy != userID {
		response.Forbidden(c)
		return nil, false
	}

	now := time.Now().In(tz.LocFromContext(c, h.DB))
	createdAt := time.Unix(o.CreatedAt, 0).In(now.Location())
	if now.Year() != createdAt.Year() || now.Month() != createdAt.Month() || now.Day() != createdAt.Day() {
		response.Forbidden(c)
		return nil, false
	}

	return o.BranchId, true
}

// orderBranchAllowed — branch_id FE gửi lên (create/update ghi thẳng vào đơn + dịch vụ con) phải nằm trong
// phạm vi chi nhánh được gán (Hard Rule, chống IDOR ghi). Chỉ kiểm khi KHÁC `cur` (branch đang lưu; nil khi
// tạo mới): đơn ở chi nhánh khác giữ nguyên branch vẫn sửa được. nil ⇒ để validateOrderPlan báo thiếu.
func orderBranchAllowed(c *gin.Context, next, cur *int) bool {
	if next == nil || (cur != nil && *cur == *next) {
		return true
	}
	return *next > 0 && middleware.IsBranchAllowed(c, uint(*next))
}

// verifyCanDeleteOrderPlan kiểm tra order tồn tại trong tenant + user có quyền
// deletePlanOrder (mirror FE deleteRecordTreatmentPlan — flat permission check,
// không có nhánh creator/same-day như update).
func (h *Handler) verifyCanDeleteOrderPlan(c *gin.Context, id uint) bool {
	var o otpDao.OrderPlan
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_plan", true).First(&o, id).Error; err != nil {
		response.NotFound(c)
		return false
	}

	userID := middleware.GetUserID(c)
	if !h.hasPermission(userID, "deletePlanOrder") {
		response.Forbidden(c)
		return false
	}
	return true
}

func (h *Handler) hasPermission(userID uint, permCode string) bool {
	if h.RbacSvc == nil {
		return false
	}
	perms, _ := h.RbacSvc.GetUserPermissions(userID)
	for _, p := range perms {
		if p == permCode {
			return true
		}
	}
	return false
}

// attachRelationReverse — story-33: gộp về handlerutil.AttachRelationReverse.
