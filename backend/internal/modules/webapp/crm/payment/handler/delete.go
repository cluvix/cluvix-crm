package handler

import (
	"erpCluvix/internal/middleware"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 4 delete variants: customer payment, deposit, refund (2 sub-types: hoàn cọc, hoàn dịch vụ).
// Mọi endpoint verifyPaymentOwnership trước khi gọi service (chống IDOR), rồi truyền db ĐÃ scope tenant
// xuống service để service tra lại phiếu + kiểm đúng payment_for của endpoint (lens-money M-6).

type deleteFn func(scoped *gorm.DB, id uint, userID int) error

func (h *Handler) deletePaymentKind(c *gin.Context, del deleteFn) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	if !h.verifyPaymentOwnership(c, id) {
		return
	}
	if err := del(h.paymentScope(c), id, int(middleware.GetUserID(c))); err != nil {
		response.HandleError(c, err)
		return
	}
	response.Deleted(c)
}

func (h *Handler) DeletePaymentCustomer(c *gin.Context) {
	h.deletePaymentKind(c, h.PaymentSvc.DeletePayment)
}

func (h *Handler) DeleteDepositCustomer(c *gin.Context) {
	h.deletePaymentKind(c, h.PaymentSvc.DeleteDeposit)
}

func (h *Handler) DeleteRefundCustomer(c *gin.Context) {
	h.deletePaymentKind(c, h.PaymentSvc.DeleteRefundCustomer)
}

func (h *Handler) DeleteRefundOrderPlan(c *gin.Context) {
	h.deletePaymentKind(c, h.PaymentSvc.DeleteRefundOrderPlan)
}

// paymentScope — db của bảng payment đã gắn phạm vi tenant (cùng phạm vi verifyPaymentOwnership).
func (h *Handler) paymentScope(c *gin.Context) *gorm.DB {
	return tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "payment", tenant.IsCrossBranchServiceEnabled(c, h.DB))
}
