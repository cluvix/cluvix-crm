package debt

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"erpCluvix/internal/modules/webapp/crm/customer/scope"
	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/handlerutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"
)

var orderPlanDebtKeySearch = []string{
	"customer.fullname", "customer.phone", "customer.code", "customer.old_code", "order_plan.code",
}

// orderPlanDebtHas — đơn còn phải thu: nợ đầu kỳ > 0 hoặc có doanh thu trong kỳ.
const orderPlanDebtHas = "(A.teBefore - A.payBefore > 0 OR A.teIn > 0)"

type orderPlanDebtSummary struct {
	Total          int64           `gorm:"column:total"`
	TotalEndBefore decimal.Decimal `gorm:"column:totalEndBefore"`
	TotalEndIn     decimal.Decimal `gorm:"column:totalEndIn"`
	DaThuTrongKy   decimal.Decimal `gorm:"column:daThuTrongKy"`
	TotalEndOrder  decimal.Decimal `gorm:"column:totalEndOrder"`
}

// orderPlanDebtSQL — mảnh SQL dùng chung cho câu trang và câu tổng của kỳ [from, to] (Unix, đã parse số).
//
// Ngữ nghĩa từng đơn (giữ nguyên ERP, "phải thu" thay cho totalEnd từ khi có VAT — bmad/order-vat):
//
//	totalEndBefore = Σ phải thu dịch vụ tạo < from − Σ thu (payment_of_date < from) của CHÍNH các dịch vụ đó
//	totalEndIn     = Σ phải thu dịch vụ tạo trong kỳ
//	daThuTrongKy   = Σ thu trong kỳ của MỌI dịch vụ của đơn
//
// Phải thu dòng = totalEnd (TRƯỚC thuế) + vat_amount: các số trên đều đem trừ với phiếu thu (tiền CÓ thuế) nên
// phải cùng gốc có thuế. Tên khoá JSON giữ nguyên (FE đang đọc). Dòng cũ vat_amount = 0 ⇒ số không đổi.
//
// "Thu" = payment_for IN (Order, MoveDeposit). KHÔNG cộng phiếu hoàn dịch vụ: hoàn = huỷ bớt dịch vụ (doanh thu
// và đã thu cùng giảm) ⇒ không đổi công nợ — khớp order_plan.debtsPlan / customer.totalDebt (xem route.go).
// Phiếu thu gom qua LATERAL theo dịch vụ ⇒ totalEnd không bị nhân bản khi dịch vụ có nhiều phiếu.
type orderPlanDebtSQL struct {
	paymentLateral string
	sumsFmt        string
	from, to       string
}

func newOrderPlanDebtSQL(from, to int64) orderPlanDebtSQL {
	f, t := strconv.FormatInt(from, 10), strconv.FormatInt(to, 10)
	return orderPlanDebtSQL{
		from: f, to: t,
		paymentLateral: fmt.Sprintf(
			`LEFT JOIN LATERAL (SELECT SUM(CASE WHEN payment_of_date < %[1]s THEN amount END) AS pb,
			SUM(CASE WHEN payment_of_date BETWEEN %[1]s AND %[2]s THEN amount END) AS pi
			FROM payment WHERE payment.serviceUsed_id = osu.id AND payment_for IN (%[3]d, %[4]d)) AS P ON TRUE`,
			f, t, define.PaymentForOrder, define.PaymentForMoveDeposit),
		// %[3]s = alias có created_day/totalEnd/vat_amount, %[4]s = alias có pb/pi.
		sumsFmt: `COALESCE(SUM(CASE WHEN %[3]s.created_day < %[1]s THEN %[3]s.totalEnd + %[3]s.vat_amount END), 0) AS teBefore,
		COALESCE(SUM(CASE WHEN %[3]s.created_day < %[1]s THEN %[4]s.pb END), 0) AS payBefore,
		COALESCE(SUM(CASE WHEN %[3]s.created_day BETWEEN %[1]s AND %[2]s THEN %[3]s.totalEnd + %[3]s.vat_amount END), 0) AS teIn,
		COALESCE(SUM(%[4]s.pi), 0) AS paidIn`,
	}
}

func (s orderPlanDebtSQL) sums(rowAlias, payAlias string) string {
	return fmt.Sprintf(s.sumsFmt, s.from, s.to, rowAlias, payAlias)
}

// orderPlanScoped — đơn trong phạm vi chi nhánh + bộ lọc + từ khoá. withCustomer = JOIN customer (từ khoá cần cột
// khách, export in tên khách).
func (h *Handler) orderPlanScoped(c *gin.Context, withCustomer bool) *gorm.DB {
	q := tenant.ScopedDBNullBranch(c, h.DB, "order_plan").Table("order_plan")
	q = query.ApplyFilter(c, q, "branch_id", "order_plan.branch_id")
	if withCustomer {
		q = q.Joins("LEFT JOIN customer ON customer.id = order_plan.customer_id")
		q = query.ApplyKeywordFilter(c, q, orderPlanDebtKeySearch...)
	}
	// S-10: phạm vi telesale — áp ở gốc nên câu trang, câu tổng (perService) và export cùng một tập.
	return scope.WhereCustomerID(c, h.DB, q, "order_plan.customer_id")
}

// orderPlanDebtPage — câu TRANG / export: công nợ từng đơn qua LATERAL, lọc đơn còn nợ, đã sắp xếp.
func (h *Handler) orderPlanDebtPage(c *gin.Context, s orderPlanDebtSQL, withCustomer bool) *gorm.DB {
	q := h.orderPlanScoped(c, withCustomer).
		Joins(fmt.Sprintf(`JOIN LATERAL (SELECT %s FROM order_service_used osu %s
			WHERE osu.order_plan_id = order_plan.id) AS A ON TRUE`, s.sums("osu", "P"), s.paymentLateral)).
		Where(orderPlanDebtHas)
	return query.ApplySort(c, q, "order_plan.id DESC", "order_plan.id")
}

// orderPlanDebtTotals — câu TỔNG: đơn trong phạm vi → dịch vụ → phiếu thu (LATERAL), gom theo đơn rồi cộng.
func (h *Handler) orderPlanDebtTotals(c *gin.Context, s orderPlanDebtSQL, withCustomer bool) (orderPlanDebtSummary, error) {
	perService := h.orderPlanScoped(c, withCustomer).
		Select("order_plan.id AS plan, osu.created_day, osu.totalEnd, osu.vat_amount, P.pb, P.pi").
		Joins("JOIN order_service_used osu ON osu.order_plan_id = order_plan.id").
		Joins(s.paymentLateral)
	perPlan := h.DB.Table("(?) AS u", perService).
		Select("u.plan, " + s.sums("u", "u")).
		Group("u.plan")
	var sum orderPlanDebtSummary
	err := h.DB.Table("(?) AS A", perPlan).
		Select(`COUNT(*) AS total,
			COALESCE(SUM(A.teBefore - A.payBefore), 0) AS totalEndBefore,
			COALESCE(SUM(A.teIn), 0) AS totalEndIn,
			COALESCE(SUM(A.paidIn), 0) AS daThuTrongKy,
			COALESCE(SUM((A.teBefore - A.payBefore) + A.teIn - A.paidIn), 0) AS totalEndOrder`).
		Where(orderPlanDebtHas).
		Scan(&sum).Error
	return sum, err
}

// parseUnixParam đọc 1 mốc Unix từ query; rỗng ⇒ (def, true); không phải số ⇒ ok=false.
func parseUnixParam(c *gin.Context, key string, def int64) (int64, bool) {
	raw := c.Query(key)
	if raw == "" {
		return def, true
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	return v, err == nil
}

// OrderPlanDebts GET /crm/report/debt/order-plan — công nợ theo đơn hàng trong kỳ filter[created_time][gte/lte].
func (h *Handler) OrderPlanDebts(c *gin.Context) {
	// from/to đi vào SQL dạng literal ⇒ BẮT BUỘC parse số trước.
	from, ok1 := parseUnixParam(c, "filter[created_time][gte]", 0)
	to, ok2 := parseUnixParam(c, "filter[created_time][lte]", time.Now().Unix())
	if !ok1 || !ok2 {
		response.Validate(c, i18n.T(c, "invalid_request"))
		return
	}
	s := newOrderPlanDebtSQL(from, to)
	withCustomer := c.Query("_keyword") != ""

	if c.Query("_export") == "1" {
		exportHandlers.ExportOrderPlanDebts(c, h.orderPlanDebtPage(c, s, true))
		return
	}

	summary, err := h.orderPlanDebtTotals(c, s, withCustomer)
	if err != nil {
		pagination.FailQuery(c, err)
		return
	}

	items := []map[string]interface{}{}
	var result pagination.Result
	if page, perPage, offset := pagination.PageParams(c); summary.Total <= int64(offset) {
		result = pagination.ResultForPage(c, &items, summary.Total, page, perPage, "order_plan")
	} else {
		q := h.orderPlanDebtPage(c, s, withCustomer).Select(`order_plan.id, order_plan.code,
			order_plan.customer_id, order_plan.branch_id,
			A.teBefore - A.payBefore AS totalEndBefore,
			A.teIn AS totalEndIn,
			A.paidIn AS daThuTrongKy,
			(A.teBefore - A.payBefore) + A.teIn - A.paidIn AS totalEndOrder,
			order_plan.created_time`)
		result = pagination.PaginateWithTotal(c, q, &items, summary.Total)
		if c.IsAborted() {
			return
		}
	}
	for _, item := range items {
		helpers.ParseDecimalFields(item, "totalEndBefore", "totalEndIn", "daThuTrongKy", "totalEndOrder")
	}
	attachDebtCustomers(c, h.DB, items)

	c.JSON(http.StatusOK, gin.H{
		"items": result.Items,
		"_meta": result.Meta,
		"total": gin.H{
			"totalEndBefore": summary.TotalEndBefore,
			"totalEndIn":     summary.TotalEndIn,
			"daThuTrongKy":   summary.DaThuTrongKy,
			"totalEndOrder":  summary.TotalEndOrder,
		},
	})
}

// attachDebtCustomers gắn item.customer (FK lookup, cố ý không lọc chi nhánh) + customerDisplayName, che SĐT theo quyền.
func attachDebtCustomers(c *gin.Context, db *gorm.DB, items []map[string]interface{}) {
	if len(items) == 0 {
		return
	}
	ids := handlerutil.CollectIDs(items, "customer_id")
	if len(ids) == 0 {
		return
	}
	handlerutil.AttachRelation(c, db, items, "customer_id", "customer",
		"customer", "id", "id, code, old_code, fullname, nickname, phone, totalDeposit", ids)
	phoneutil.MaskNestedCustomerPhone(c, db, items, "customer")
	for _, item := range items {
		cust, _ := item["customer"].(map[string]interface{})
		if cust == nil {
			continue
		}
		display := helpers.ToString(cust["fullname"])
		if nick := helpers.ToString(cust["nickname"]); nick != "" {
			display += " - " + nick
		}
		item["customerDisplayName"] = display
	}
}
