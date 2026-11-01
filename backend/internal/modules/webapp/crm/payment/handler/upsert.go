package handler

import (
	"net/http"
	"strconv"

	"erpCluvix/internal/middleware"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/response"

	"github.com/gin-gonic/gin"
)

// CreatePaymentClinic POST /payment — tạo phiếu thu/chi phòng khám.
func (h *Handler) CreatePaymentClinic(c *gin.Context) {
	var input CreatePaymentClinicInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ValidateErr(c, err)
		return
	}
	p, err := h.PaymentSvc.CreatePaymentClinicFromPayment(input.ToPayment(), int(middleware.GetUserID(c)))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.Created(c, p)
}

// UpdatePaymentClinic PUT /payment/:id — updated_at (khoá lạc quan D6) required enforce qua binding tag.
func (h *Handler) UpdatePaymentClinic(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if !h.verifyPaymentOwnership(c, uint(id)) {
		return
	}
	var input UpdatePaymentClinicInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ValidateErr(c, err)
		return
	}
	p, err := h.PaymentSvc.UpdatePaymentClinicFromPayment(uint(id), input.ToPayment(), int(middleware.GetUserID(c)))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.Updated(c, p)
}

// CreatePaymentCustomer POST /payment/create-payment-customer — thanh toán 1+ dịch vụ.
// FK = serviceUsed_idArr (order_service_used) — BẮT BUỘC verify tenant trước khi tạo payment,
// KHÔNG trust branch_id từ payload FE (derive từ dịch vụ đầu tiên, đã verify tồn tại).
func (h *Handler) CreatePaymentCustomer(c *gin.Context) {
	var input CreatePaymentCustomerInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ValidateErr(c, err)
		return
	}
	if !h.serviceUsedIDsInTenant(c, input.ServiceUsedIDArr) {
		response.NotFound(c)
		return
	}
	branchID, ok := h.fkBranch(c, "order_service_used", input.ServiceUsedIDArr[0])
	if !ok {
		response.NotFound(c)
		return
	}
	p := input.ToPayment()
	p.BranchId = branchID
	msg, err := h.PaymentSvc.CreatePaymentCustomerFromPayment(p, input.ServiceUsedIDArr, int(middleware.GetUserID(c)))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

// CreatePaymentOrder POST /payment/create-payment-order — thanh toán toàn bộ đơn hàng.
// FK = parent_id (order_plan) — BẮT BUỘC verify tenant trước khi tạo payment,
// KHÔNG trust branch_id từ payload FE (derive từ order, đã verify tồn tại).
func (h *Handler) CreatePaymentOrder(c *gin.Context) {
	var input CreatePaymentOrderInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ValidateErr(c, err)
		return
	}
	branchID, ok := h.fkBranch(c, "order_plan", input.ParentID)
	if !ok {
		response.NotFound(c)
		return
	}
	p := input.ToPayment()
	p.BranchId = branchID
	msgKey, err := h.PaymentSvc.CreatePaymentOrderFromPayment(p, int(middleware.GetUserID(c)))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.DataWithMessage(c, nil, i18n.T(c, msgKey))
}

// CreateDepositCustomer POST /payment/create-deposit-customer — đặt cọc.
// FK = customer_id — verify khách tồn tại khi có customer_id (không lọc chi nhánh: khách đăng ký ở
// chi nhánh khác vẫn đặt cọc được); customer_id thiếu → business validation service tự chặn.
func (h *Handler) CreateDepositCustomer(c *gin.Context) {
	var input PaymentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ValidateErr(c, err)
		return
	}
	p := input.ToPayment()
	if input.CustomerID != nil {
		if _, ok := h.fkBranch(c, "customer", *input.CustomerID); !ok {
			response.NotFound(c)
			return
		}
	}
	if !branchAllowed(c, input.BranchID) {
		response.Forbidden(c)
		return
	}
	pp, err := h.PaymentSvc.CreateDepositCustomerFromPayment(p, int(middleware.GetUserID(c)))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.Created(c, pp)
}

// CreateRefundDepositCustomer POST /payment/create-refund-deposit-customer — hoàn cọc.
// FK = deposit_id (payment gốc) — BẮT BUỘC verify tenant trước khi tạo refund.
func (h *Handler) CreateRefundDepositCustomer(c *gin.Context) {
	var input PaymentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ValidateErr(c, err)
		return
	}
	if input.DepositID == nil {
		response.HandleError(c, errs.ErrMissingDepositID)
		return
	}
	branchID, ok := h.fkBranch(c, "payment", *input.DepositID)
	if !ok {
		response.NotFound(c)
		return
	}
	p := input.ToPayment()
	p.BranchId = branchID
	pp, err := h.PaymentSvc.CreateRefundDepositCustomerFromPayment(p, int(middleware.GetUserID(c)))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.Created(c, pp)
}

// CreateRefundOrderPlan POST /payment/create-refund-order-plan — hoàn dịch vụ đơn hàng.
// FK = serviceUsed_id (order_service_used) — BẮT BUỘC verify tenant trước khi tạo refund.
func (h *Handler) CreateRefundOrderPlan(c *gin.Context) {
	var input PaymentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ValidateErr(c, err)
		return
	}
	if input.ServiceUsedID == nil {
		response.HandleError(c, errs.ErrMissingServiceUsedID)
		return
	}
	branchID, ok := h.fkBranch(c, "order_service_used", *input.ServiceUsedID)
	if !ok {
		response.NotFound(c)
		return
	}
	p := input.ToPayment()
	p.BranchId = branchID
	pp, err := h.PaymentSvc.CreateRefundOrderPlanFromPayment(p, int(middleware.GetUserID(c)))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.Created(c, pp)
}

func (h *Handler) MoveDepositToPayment(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if !h.verifyPaymentOwnership(c, uint(id)) {
		return
	}
	var input MoveDepositToPaymentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ValidateErr(c, err)
		return
	}
	// branch_id là branch của payment MỚI (chuyển cọc) — verifyPaymentOwnership chỉ xác nhận
	// deposit gốc thuộc tenant, không xác nhận branch đích FE gửi lên. Thiếu check này = user
	// chuyển cọc sang branch họ không được gán (đồng pattern CreateDepositCustomer).
	if !branchAllowed(c, input.BranchID) {
		response.Forbidden(c)
		return
	}
	msg, err := h.PaymentSvc.MoveDepositToPaymentFromPayment(id, input.ToPayment(), int(middleware.GetUserID(c)))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": msg})
}

func (h *Handler) UpdatePaymentCustomer(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if !h.verifyCanEditPaymentCustomer(c, uint(id)) {
		return
	}
	var input UpdatePaymentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ValidateErr(c, err)
		return
	}
	// verifyCanEditPaymentCustomer chỉ check branch HIỆN TẠI của record — branch_id MỚI FE gửi
	// lên (mergePaymentParams sẽ ghi thẳng) phải verify riêng, tránh đổi payment sang branch
	// ngoài phạm vi được gán.
	if !branchAllowed(c, input.BranchID) {
		response.Forbidden(c)
		return
	}
	p, err := h.PaymentSvc.UpdatePaymentCustomer(uint(id), input.ToParams(), int(middleware.GetUserID(c)))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.Updated(c, p)
}

func (h *Handler) UpdateDepositCustomer(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if !h.verifyCanEditPaymentCustomer(c, uint(id)) {
		return
	}
	var input UpdatePaymentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.ValidateErr(c, err)
		return
	}
	if !branchAllowed(c, input.BranchID) {
		response.Forbidden(c)
		return
	}
	p, err := h.PaymentSvc.UpdateDepositCustomer(uint(id), input.ToParams(), int(middleware.GetUserID(c)))
	if err != nil {
		response.HandleError(c, err)
		return
	}
	response.Updated(c, p)
}
