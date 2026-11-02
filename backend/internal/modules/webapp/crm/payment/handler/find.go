package handler

import (
	paymentDao "erpCluvix/internal/modules/webapp/crm/payment/dao"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
)

// GetPayment GET /payment/:id
func (h *Handler) GetPayment(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	var item paymentDao.Payment
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "payment", true).
		Where("id = ?", id).Take(&item).Error; err != nil {
		response.NotFound(c)
		return
	}
	response.Data(c, item)
}
