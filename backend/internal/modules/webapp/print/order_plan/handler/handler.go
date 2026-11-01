package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"gorm.io/gorm"
)

type Deps struct {
	DB *gorm.DB
}

type Handler struct{ Deps }

func New(d Deps) *Handler { return &Handler{Deps: d} }

func (h *Handler) PrintOrderPlan(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}

	var orderPlan map[string]interface{}
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "order_plan", tenant.IsCrossBranchServiceEnabled(c, h.DB)).Table("order_plan").
		Select("id, code, branch_id, customer_id, note, discount_other, paymentPlan, debtsPlan, created_time, COALESCE((SELECT SUM(osu.totalEnd) FROM order_service_used osu WHERE osu.order_plan_id = order_plan.id), 0) AS total").
		Where("id = ?", id).Limit(1).Scan(&orderPlan).Error; err != nil || len(orderPlan) == 0 {
		response.NotFound(c)
		return
	}

	helpers.ParseDecimalFields(orderPlan, "discount_other", "total", "paymentPlan", "debtsPlan")

	if custID := helpers.ToInt64(orderPlan["customer_id"]); custID > 0 {
		var customer map[string]interface{}
		tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").Select("id, fullname, phone, address").
			Where("id = ?", custID).Limit(1).Scan(&customer)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		orderPlan["customer"] = customer
	}

	var services []map[string]interface{}
	tx := tenant.CrossBranchLookup(h.DB, "order_service_used").Table("order_service_used").
		Select("order_service_used.id, order_service_used.service_id, order_service_used.quantity, order_service_used.unit_price, order_service_used.service_discount_type, order_service_used.service_discount, order_service_used.discount_other, order_service_used.totalEnd, service.name AS service_name").
		Joins("LEFT JOIN service ON service.id = order_service_used.service_id").
		Where("order_service_used.order_plan_id = ?", id).
		Find(&services)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	for _, svc := range services {
		svc["service"] = map[string]interface{}{"name": svc["service_name"]}
		delete(svc, "service_name")
		helpers.ParseDecimalFields(svc, "unit_price", "service_discount", "discount_other", "totalEnd")
	}
	helpers.ParseJSONFieldsForTables(services, "order_service_used")
	orderPlan["orderServiceUseds"] = services

	var payments []map[string]interface{}
	tx = tenant.CrossBranchLookup(h.DB, "payment").Table("payment").
		Select("payment.id, payment.amount, payment.payment_of_date, branch.name AS chiNhanh, payment_method.name AS method").
		Joins("LEFT JOIN branch ON branch.id = payment.branch_id").
		Joins("LEFT JOIN payment_method ON payment_method.id = payment.method").
		Where("payment.parent_id = ? AND payment.payment_for IN (4, 5)", id).
		Order("payment.id ASC").
		Find(&payments)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	for i, p := range payments {
		p["lanThanhToan"] = i + 1
		helpers.ParseDecimalFields(p, "amount")
	}

	c.JSON(http.StatusOK, gin.H{
		"orderPlan":      orderPlan,
		"historyPayment": payments,
	})
}
