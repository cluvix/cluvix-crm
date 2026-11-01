package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"
)

type Deps struct {
	DB *gorm.DB
}

type Handler struct{ Deps }

func New(d Deps) *Handler { return &Handler{Deps: d} }

// PrintRefundConsignment GET /api/print/refund-deposit/:id
// Trả về dữ liệu in phiếu hoàn cọc. Tên hàm giữ từ thời còn route in phiếu hoàn ký gửi (đã gỡ cùng
// tính năng ký gửi) — nay chỉ phục vụ /refund-deposit/:id.
// FE can: code, amount, payment_time, note, customer{fullname,phone,address},
// paymentMethod{name}, branch_id
func (h *Handler) PrintRefundConsignment(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}

	// 1. Lay thong tin payment
	var item map[string]interface{}
	if err := tenant.ScopedDBNullBranch(c, h.DB, "payment").Table("payment").
		Select("id, code, customer_id, amount, payment_time, note, method, branch_id").
		Where("id = ?", id).Limit(1).Scan(&item).Error; err != nil || len(item) == 0 {
		response.NotFound(c)
		return
	}

	// 2. Attach customer
	if custID := helpers.ToInt64(item["customer_id"]); custID > 0 {
		var customer map[string]interface{}
		tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").Select("id, fullname, phone, address").
			Where("id = ?", custID).Limit(1).Scan(&customer)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		item["customer"] = customer
	}

	// 3. Attach paymentMethod (ten phuong thuc thanh toan)
	if methodID := helpers.ToInt64(item["method"]); methodID > 0 {
		var pm map[string]interface{}
		tx := h.DB.Table("payment_method").Select("id, name").
			Where("id = ?", methodID).Limit(1).Scan(&pm)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		item["paymentMethod"] = pm
	}

	// 4. Parse decimal fields va map key cho FE
	helpers.ParseDecimalFields(item, "amount")
	item["branch"] = item["branch_id"]

	c.JSON(http.StatusOK, gin.H{"data": item})
}

// PrintDeposit GET /api/print/deposit/:id
// Tra ve du lieu in phieu dat coc.
// FE can: code, amount, payment_time, note, customer{fullname,phone,address},
// paymentMethod{name}, branch_id
func (h *Handler) PrintDeposit(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}

	var item map[string]interface{}
	if err := tenant.ScopedDBNullBranch(c, h.DB, "payment").Table("payment").
		Select("id, code, customer_id, amount, payment_time, note, method, branch_id").
		Where("id = ?", id).Limit(1).Scan(&item).Error; err != nil || len(item) == 0 {
		response.NotFound(c)
		return
	}

	if custID := helpers.ToInt64(item["customer_id"]); custID > 0 {
		var customer map[string]interface{}
		tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").
			Select("id, fullname, phone, address").
			Where("id = ?", custID).Limit(1).Scan(&customer)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		item["customer"] = customer
	}

	helpers.ParseDecimalFields(item, "amount")
	c.JSON(http.StatusOK, item)
}

// PrintPayment GET /api/print/payment/:id
// Tra ve du lieu in phieu thu thanh toan.
func (h *Handler) PrintPayment(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}

	var payment map[string]interface{}
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "payment", tenant.IsCrossBranchServiceEnabled(c, h.DB)).Table("payment").
		Select("id, code, customer_id, serviceUsed_id, parent_id, amount, payment_time, payment_of_date, note, method, branch_id").
		Where("id = ?", id).Limit(1).Scan(&payment).Error; err != nil || len(payment) == 0 {
		response.NotFound(c)
		return
	}

	// Parse decimal cho payment chinh
	helpers.ParseDecimalFields(payment, "amount")

	// Attach customer
	if custID := helpers.ToInt64(payment["customer_id"]); custID > 0 {
		var customer map[string]interface{}
		tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").Select("id, fullname, phone, address").
			Where("id = ?", custID).Limit(1).Scan(&customer)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		payment["customer"] = customer
	}

	// Attach orderServiceUsed kem service.name
	if suID := helpers.ToInt64(payment["serviceUsed_id"]); suID > 0 {
		var osu map[string]interface{}
		tx := tenant.CrossBranchLookup(h.DB, "order_service_used").Table("order_service_used").
			Select("order_service_used.id, order_service_used.code, order_service_used.service_id, order_service_used.quantity, order_service_used.unit_price, order_service_used.service_discount_type, order_service_used.service_discount, order_service_used.discount_other, order_service_used.totalEnd, order_service_used.payment, order_service_used.refundServiceUsed, service.name AS service_name").
			Joins("LEFT JOIN service ON service.id = order_service_used.service_id").
			Where("order_service_used.id = ?", suID).Limit(1).Scan(&osu)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		if len(osu) > 0 {
			osu["service"] = map[string]interface{}{"name": osu["service_name"]}
			delete(osu, "service_name")
			helpers.ParseDecimalFields(osu, "unit_price", "service_discount", "discount_other", "totalEnd", "payment", "refundServiceUsed")
			helpers.ParseJSONFieldForTable(osu, "order_service_used")
			payment["orderServiceUsed"] = osu
		}
	}

	// Lich su thanh toan cho dich vu nay - chi tinh cac lan thanh toan tinh den
	// thoi diem (payment_of_date) cua phieu dang in, khong gom cac lan thanh
	// toan phat sinh SAU do (tranh phieu in cu the hien sai la da thanh toan het).
	parentID := helpers.ToInt64(payment["parent_id"])
	serviceUsedID := helpers.ToInt64(payment["serviceUsed_id"])
	paymentOfDate := helpers.ToInt64(payment["payment_of_date"])
	paymentID := helpers.ToInt64(payment["id"])
	var pmRows []map[string]interface{}
	if parentID > 0 && serviceUsedID > 0 {
		tx := tenant.ScopedDBNullBranch(c, h.DB, "payment").Table("payment").
			Select("payment.id, payment.amount, payment.payment_of_date, payment_method.name AS method_name").
			Joins("LEFT JOIN payment_method ON payment_method.id = payment.method").
			Where("payment.parent_id = ? AND payment.serviceUsed_id = ? AND payment.payment_for IN (4, 5)", parentID, serviceUsedID).
			Where("(payment.payment_of_date < ? OR (payment.payment_of_date = ? AND payment.id <= ?))", paymentOfDate, paymentOfDate, paymentID).
			Order("payment.payment_of_date ASC, payment.id ASC").
			Find(&pmRows)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
	}
	var historyPayment []map[string]interface{}
	// Tiền cộng bằng decimal (lens-money M-20) — float64 cộng dồn lệch phần lẻ.
	totalPayment := decimal.Zero
	for i, p := range pmRows {
		amt := helpers.ToDecimal(p["amount"])
		totalPayment = totalPayment.Add(amt)
		methodName := helpers.ToString(p["method_name"])
		historyPayment = append(historyPayment, map[string]interface{}{
			"text":   fmt.Sprintf("Lần %d - %s", i+1, methodName),
			"amount": amt,
		})
	}

	if historyPayment == nil {
		historyPayment = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{
		"payment":        payment,
		"historyPayment": historyPayment,
		"totalPayment":   totalPayment,
	})
}

// PrintMultiPayment GET /api/print/multi-payment?ids=1,2,3&customerId=X
// In phieu thanh toan tuy chon nhieu payment.
// Shape: { "customer": {...}, "payment": [{serviceName, totalEnd, amount, note}...] }
func (h *Handler) PrintMultiPayment(c *gin.Context) {
	idsParam := c.Query("ids")
	customerIdParam := c.Query("customerId")

	if idsParam == "" {
		response.Validate(c, i18n.T(c, "ids_invalid"))
		return
	}

	// Lay thong tin customer
	var customer map[string]interface{}
	if customerIdParam != "" {
		tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").
			Select("id, code, fullname, phone, address, branch_id").
			Where("id = ?", customerIdParam).Limit(1).Scan(&customer)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
	}

	idStrs := strings.Split(idsParam, ",")
	// Convert sang []int de GORM generate IN (613) thay vi IN ('613')
	var ids []int
	for _, s := range idStrs {
		s = strings.TrimSpace(s)
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			ids = append(ids, v)
		}
	}
	if len(ids) == 0 {
		response.Validate(c, i18n.T(c, "ids_invalid"))
		return
	}

	// Story-38 (D15): bỏ vế company — 1 thương hiệu/DB, `payment.id IN (?)` đã đủ giới hạn đúng
	// tenant (DB-per-customer); route-level auth vẫn quyết định user có được xem payment hay không.
	var payments []map[string]interface{}
	baseSelect := `SELECT ANY_VALUE(payment.customer_id) AS customer_id,
		ANY_VALUE(payment.note) AS note,
		ANY_VALUE(service.name) AS serviceName,
		ANY_VALUE(order_service_used.totalEnd) AS totalEnd,
		SUM(payment.amount) AS amount
		FROM payment
		LEFT JOIN order_service_used ON order_service_used.id = payment.serviceUsed_id
		LEFT JOIN service ON service.id = order_service_used.service_id
		WHERE payment.id IN (?)`

	var q *gorm.DB
	if customerIdParam == "" {
		q = h.DB.Raw(baseSelect+" GROUP BY payment.serviceUsed_id", ids)
	} else {
		q = h.DB.Raw(baseSelect+" AND payment.customer_id = ? GROUP BY payment.serviceUsed_id", ids, customerIdParam)
	}
	tx := q.Scan(&payments)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	// Parse decimal fields
	for _, p := range payments {
		helpers.ParseDecimalFields(p, "totalEnd", "amount")
	}

	// Neu chua co customer tu param, lay tu payment dau tien
	if len(customer) == 0 && len(payments) > 0 {
		if custID := helpers.ToInt64(payments[0]["customer_id"]); custID > 0 {
			tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").
				Select("id, code, fullname, phone, address, branch_id").
				Where("id = ?", custID).Limit(1).Scan(&customer)
			if pagination.IsQueryError(tx.Error) {
				pagination.FailQuery(c, tx.Error)
				return
			}
		}
	}

	// Dam bao payments luon la array (khong nil) de FE khong crash
	if payments == nil {
		payments = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{
		"customer": customer,
		"payment":  payments,
	})
}

// PrintRefundService GET /api/print/refund-service/:id
// Tra ve du lieu in phieu hoan dich vu.
// FE can: code, amount, payment_time, note, customer{fullname,phone,address},
// paymentMethod{name}, orderServiceUsed{service{name}, quantity, unit_price, service_discount, total}
func (h *Handler) PrintRefundService(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}

	// 1. Lay thong tin payment
	var item map[string]interface{}
	if err := tenant.ScopedDBNullBranch(c, h.DB, "payment").Table("payment").
		Select("id, code, customer_id, serviceUsed_id, amount, payment_time, note, method, branch_id").
		Where("id = ?", id).Limit(1).Scan(&item).Error; err != nil || len(item) == 0 {
		response.NotFound(c)
		return
	}

	// 2. Parse decimal fields
	helpers.ParseDecimalFields(item, "amount")

	// 3. Attach customer
	if custID := helpers.ToInt64(item["customer_id"]); custID > 0 {
		var customer map[string]interface{}
		tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").Select("id, fullname, phone, address").
			Where("id = ?", custID).Limit(1).Scan(&customer)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		item["customer"] = customer
	}

	// 4. Attach paymentMethod
	if methodID := helpers.ToInt64(item["method"]); methodID > 0 {
		var pm map[string]interface{}
		tx := h.DB.Table("payment_method").Select("id, name").
			Where("id = ?", methodID).Limit(1).Scan(&pm)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		item["paymentMethod"] = pm
	}

	// 5. Attach orderServiceUsed kem service.name
	if suID := helpers.ToInt64(item["serviceUsed_id"]); suID > 0 {
		var osu map[string]interface{}
		tx := tenant.CrossBranchLookup(h.DB, "order_service_used").Table("order_service_used").
			Select("order_service_used.id, order_service_used.code, order_service_used.quantity, order_service_used.unit_price, order_service_used.service_discount_type, order_service_used.service_discount, order_service_used.discount_other, order_service_used.totalEnd, service.name AS service_name").
			Joins("LEFT JOIN service ON service.id = order_service_used.service_id").
			Where("order_service_used.id = ?", suID).Limit(1).Scan(&osu)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		if len(osu) > 0 {
			osu["service"] = map[string]interface{}{"name": osu["service_name"]}
			delete(osu, "service_name")
			helpers.ParseDecimalFields(osu, "unit_price", "service_discount", "discount_other", "totalEnd")
			helpers.ParseJSONFieldForTable(osu, "order_service_used")
			// Tinh total = quantity * unit_price - service_discount
			qty := helpers.ToDecimal(osu["quantity"])
			unitPrice := helpers.ToDecimal(osu["unit_price"])
			svcDiscount := helpers.ToDecimal(osu["service_discount"])
			osu["total"] = qty.Mul(unitPrice).Sub(svcDiscount)
			item["orderServiceUsed"] = osu
		}
	}

	item["branch"] = item["branch_id"]

	c.JSON(http.StatusOK, gin.H{"data": item})
}
