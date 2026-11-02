package handler

import (
	"strconv"

	"erpCluvix/internal/middleware"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
)

// DeleteOrderPlan DELETE /crm/order-plan/:id
func (h *Handler) DeleteOrderPlan(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if !h.verifyCanDeleteOrderPlan(c, uint(id)) {
		return
	}
	scopedDB := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_plan", true)
	if err := h.svc.Delete(uint(id), middleware.GetUserID(c), scopedDB); err != nil {
		response.HandleError(c, err)
		return
	}
	response.Deleted(c)
}
