// Package payment — module root.
//
// Plan B.2 (2026-05-16): Payment handler ra layered module. Service giữ ở
// internal/services/crm/ (cross-deps loyalty/CLV/validate giống OTP).
//
// Note: GET /crm/payment/get-payment-detail-for-customer KHÔNG nằm trong module
// (handler ở customer_detail.go, package crm). Caller (router/crm.go) wire
// endpoint đó riêng sau khi gọi RegisterRoutes.
package payment

import (
	"erpCluvix/internal/modules/webapp/crm/payment/handler"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers all /crm/payment/* endpoints EXCEPT
// /get-payment-detail-for-customer (xem package doc).
func RegisterRoutes(crm gin.IRouter, d handler.Deps) {
	h := handler.New(d)

	payment := crm.Group("/payment")
	{
		// Danh sách phiếu (5 màn, handler/list.go) — route tĩnh khai TRƯỚC `/:id`. `_export=1` ⇒ Excel.
		payment.GET("", h.ListPayment)
		payment.GET("/list-receipts", h.ListReceipts)
		payment.GET("/list-deposit", h.ListDeposit)
		payment.GET("/list-refund-deposit", h.ListRefundDeposit)
		payment.GET("/list-refund-order-plan", h.ListRefundOrderPlan)
		payment.GET("/get-order-plan/:id", h.GetOrderPlan)
		payment.GET("/:id", h.GetPayment)
		// Endpoint riêng cho màn CHI TIẾT phiếu thu (SlideOver) — xem docblock GetPaymentDetail.
		payment.GET("/:id/detail", h.GetPaymentDetail)
		payment.POST("/create-payment-order", h.CreatePaymentOrder)
		payment.POST("", h.CreatePaymentClinic)
		payment.PUT("/:id", h.UpdatePaymentClinic)
		payment.POST("/create-payment-customer", h.CreatePaymentCustomer)
		payment.POST("/create-deposit-customer", h.CreateDepositCustomer)
		payment.POST("/create-refund-deposit-customer", h.CreateRefundDepositCustomer)
		payment.POST("/create-refund-order-plan", h.CreateRefundOrderPlan)
		payment.POST("/:id/move-deposit-to-payment", h.MoveDepositToPayment)
		payment.PUT("/:id/move-deposit-to-payment", h.MoveDepositToPayment)
		payment.PUT("/:id/update-payment-customer", h.UpdatePaymentCustomer)
		payment.PUT("/:id/update-deposit-customer", h.UpdateDepositCustomer)
		payment.DELETE("/:id/delete-payment-customer", h.DeletePaymentCustomer)
		payment.DELETE("/:id/delete-deposit-customer", h.DeleteDepositCustomer)
		payment.DELETE("/:id/delete-refund-customer", h.DeleteRefundCustomer)
		payment.DELETE("/:id/delete-refund-order-plan", h.DeleteRefundOrderPlan)
	}
}
