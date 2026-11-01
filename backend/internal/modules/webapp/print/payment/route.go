package payment

import (
	"erpCluvix/internal/modules/webapp/print/payment/handler"

	"github.com/gin-gonic/gin"
)

func RegisterRoutes(p gin.IRouter, d handler.Deps) {
	h := handler.New(d)
	p.GET("/deposit/:id", h.PrintDeposit)
	p.GET("/refund-deposit/:id", h.PrintRefundConsignment)
	p.GET("/refund-service/:id", h.PrintRefundService)
	p.GET("/payment/:id", h.PrintPayment)
	p.GET("/multi-payment", h.PrintMultiPayment)
}
