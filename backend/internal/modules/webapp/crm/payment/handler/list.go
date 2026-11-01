package handler

// Danh sách phiếu (5 màn: thanh toán, thu/chi, đặt cọc, hoàn cọc, hoàn dịch vụ) — port từ ERP
// `accountant/payment/handler/payment.go`. Khác ERP có chủ đích:
//   - 1 thương hiệu/DB (D15): không còn `company_id` ở select/filter/index hint.
//   - Không có `customer.search_text` (ngram) ⇒ `_keyword` là LIKE thô trên customer/đơn/mã phiếu
//     (cùng khuôn `crm/report/revenue/handler/branch_receipts.go`), không có đường "tra id nhanh".
//   - Bảng đơn là `order_plan` (ERP: `order_treatment_plan`); cột không còn trong schema CRM bị bỏ:
//     `payment.cash_flow_category_id`, `customer.page_facebook_id/campain_event_id`,
//     `order_service_used.listDoctors/listDirectSales/completion_percent`, `order_plan.listDirectSales`.
//   - Quan hệ của phiếu (khách/đơn/dịch vụ/phiếu cọc gốc) nạp qua `tenant.CrossBranchLookup` — cố ý
//     không lọc chi nhánh, như màn chi tiết (story-37): khách/đơn có thể ở chi nhánh khác phiếu.
//
// MỌI list dùng chung 1 khung lọc (`paymentFilters`) cho câu tổng, câu trang và nhánh export
// (`_export=1`) ⇒ 3 nơi không bao giờ lệch nhau.

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"erpCluvix/internal/middleware"
	"erpCluvix/internal/modules/webapp/crm/customer/scope"
	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/handlerutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/storage"
	"erpCluvix/pkg/tenant"
)

// paymentKeySearch — `_keyword` khớp LIKE (OR) trên các cột này.
var paymentKeySearch = []string{"customer.fullname", "customer.code", "customer.phone", "order_plan.code", "payment.code"}

// paymentListSelect — cột của phiếu cho mọi list; quan hệ gắn sau bằng expandPaymentItems.
const paymentListSelect = `payment.id, payment.code,
	payment.customer_id, payment.parent_id, payment.serviceUsed_id, payment.deposit_id,
	payment.branch_id,
	payment.method, payment.card_type_id, payment.bank_code,
	payment.amount, payment.deposit_after, payment.deposit_type,
	payment.payment_for, payment.payment_time, payment.payment_of_date,
	payment.note,
	payment.created_at, payment.updated_at,
	payment.created_by, payment.updated_by`

// LEFT JOIN tra cứu — đều theo KHOÁ CHÍNH của bảng được nối ⇒ không nhân dòng, nên câu tổng chỉ cần
// JOIN bảng nào bộ lọc thật sự dùng (withPaymentJoinsFor) mà COUNT/SUM không đổi.
// Bảng nối KHÔNG gắn vế chi nhánh vào ON: đây là quan hệ của chính phiếu đã scope (FK lookup, cố ý
// cross-branch — xem package doc), không phải nguồn sinh dòng.
const (
	paymentJoinCustomer    = "LEFT JOIN customer ON customer.id = payment.customer_id"
	paymentJoinOrderPlan   = "LEFT JOIN order_plan ON order_plan.id = payment.parent_id"
	paymentJoinServiceUsed = "LEFT JOIN order_service_used ON order_service_used.id = payment.serviceUsed_id"
	paymentJoinService     = "LEFT JOIN service ON service.id = order_service_used.service_id"
)

// paymentFilterColumns — bộ lọc chuẩn (query.ApplyFilter) dùng chung cho cả 6 list.
var paymentFilterColumns = []struct{ key, column string }{
	{"branch_id", "payment.branch_id"},
	{"source_id", "order_service_used.source_id"},
	{"customer_id", "payment.customer_id"},
	{"payment_for", "payment.payment_for"},
	{"payment_time", "payment.payment_time"},
	{"payment_of_date", "payment.payment_of_date"},
	{"method", "payment.method"},
	{"card_type_id", "payment.card_type_id"},
	{"deposit_type", "payment.deposit_type"},
	{"created_at", "payment.created_at"},
	{"created_by", "payment.created_by"},
	{"created_time", "order_plan.created_time"},
	{"_groupLocal", "customer.province_id"},
	{"_groupCountry", "customer.country_id"},
	{"service_id", "order_service_used.service_id"},
	{"service_type", "service.service_type_id"},
}

// paymentSummary — dòng tổng của các list; mỗi endpoint chỉ SELECT cột nó cần.
type paymentSummary struct {
	Total             int64           `gorm:"column:total"`
	TotalSum          decimal.Decimal `gorm:"column:total_sum"`
	TotalDepositAfter decimal.Decimal `gorm:"column:total_deposit_after"`
}

// paymentListSpec — phần RIÊNG của từng list: điều kiện cố định + cột tổng + tên field tổng trong
// response + hàm export. Mọi thứ còn lại (lọc, phân trang, gắn quan hệ) dùng chung.
type paymentListSpec struct {
	conds      func(q *gorm.DB) *gorm.DB
	sumSelect  string            // "" ⇒ chỉ đếm
	sumFields  map[string]string // field response → "sum" | "deposit_after"
	exportFunc func(c *gin.Context, q *gorm.DB)
}

var (
	paymentListPaymentSpec = paymentListSpec{
		conds: func(q *gorm.DB) *gorm.DB {
			return q.Where("payment.payment_for IN ?", []int{define.PaymentForOrder, define.PaymentForMoveDeposit})
		},
		sumSelect:  ", COALESCE(SUM(payment.amount), 0) AS total_sum",
		sumFields:  map[string]string{"totalSum": "sum"},
		exportFunc: exportHandlers.ExportPayment,
	}
	paymentListReceiptsSpec = paymentListSpec{
		conds: func(q *gorm.DB) *gorm.DB {
			return q.Where("payment.amount != 0").
				Where("payment.payment_for IN ?", []int{
					define.PaymentForOrder, define.PaymentForDeposit, define.PaymentForRefundDeposit,
					define.PaymentForRefundService, define.PaymentForClinicIncome, define.PaymentForClinicExpense,
				}).
				Where("(payment.deposit_type IS NULL OR payment.deposit_type NOT IN ?)", []int{define.PaymentTypeDepositKG})
		},
		sumSelect:  ", COALESCE(SUM(payment.amount), 0) AS total_sum",
		sumFields:  map[string]string{"totalSum": "sum"},
		exportFunc: exportHandlers.ExportPaymentReceipts,
	}
	paymentListDepositSpec = paymentListSpec{
		conds: func(q *gorm.DB) *gorm.DB {
			return q.Where("payment.payment_for = ?", define.PaymentForDeposit).
				Where("(payment.deposit_type IS NULL OR payment.deposit_type NOT IN ?)", []int{define.PaymentTypeDepositKG}).
				Where("payment.deposit_after > 0")
		},
		sumSelect:  ", COALESCE(SUM(payment.amount), 0) AS total_sum, COALESCE(SUM(payment.deposit_after), 0) AS total_deposit_after",
		sumFields:  map[string]string{"totalSumDeposit": "sum", "totalSumDepositAfter": "deposit_after"},
		exportFunc: exportHandlers.ExportPaymentDeposit,
	}
	paymentListRefundDepositSpec = paymentListSpec{
		conds: func(q *gorm.DB) *gorm.DB {
			return q.Where("payment.payment_for = ?", define.PaymentForRefundDeposit).Where("payment.amount != 0")
		},
		sumSelect:  ", COALESCE(SUM(payment.amount), 0) AS total_sum",
		sumFields:  map[string]string{"totalRefundDeposit": "sum"},
		exportFunc: exportHandlers.ExportPaymentRefundDeposit,
	}
	paymentListRefundOrderPlanSpec = paymentListSpec{
		conds: func(q *gorm.DB) *gorm.DB {
			return q.Where("payment.payment_for = ?", define.PaymentForRefundService).Where("payment.amount != 0")
		},
		sumSelect:  ", COALESCE(SUM(payment.amount), 0) AS total_sum",
		sumFields:  map[string]string{"totalRefund": "sum"},
		exportFunc: exportHandlers.ExportPaymentRefundOrderPlan,
	}
)

// ListPayment GET /crm/payment — phiếu thanh toán (thanh toán dịch vụ + chuyển cọc).
func (h *Handler) ListPayment(c *gin.Context) { h.listPayments(c, paymentListPaymentSpec) }

// ListReceipts GET /crm/payment/list-receipts — phiếu thu/chi (mọi loại có tiền, trừ ký gửi).
func (h *Handler) ListReceipts(c *gin.Context) { h.listPayments(c, paymentListReceiptsSpec) }

// ListDeposit GET /crm/payment/list-deposit — phiếu đặt cọc còn dư (không gồm ký gửi).
func (h *Handler) ListDeposit(c *gin.Context) { h.listPayments(c, paymentListDepositSpec) }

// ListRefundDeposit GET /crm/payment/list-refund-deposit — phiếu hoàn cọc.
func (h *Handler) ListRefundDeposit(c *gin.Context) { h.listPayments(c, paymentListRefundDepositSpec) }

// ListRefundOrderPlan GET /crm/payment/list-refund-order-plan — phiếu hoàn dịch vụ.
func (h *Handler) ListRefundOrderPlan(c *gin.Context) {
	h.listPayments(c, paymentListRefundOrderPlanSpec)
}

// listPayments — khung chung: export | (câu tổng → câu trang → gắn quan hệ).
// Response giữ hình dạng ERP: `{object: {items, _meta}, <các field tổng>}`.
func (h *Handler) listPayments(c *gin.Context, spec paymentListSpec) {
	// S-10: phạm vi telesale — chỉ phiếu của khách mình phụ trách (phiếu không có khách, vd thu/chi
	// phòng khám, bị loại khi bị giới hạn). Đọc phạm vi lỗi ⇒ 500 (fail-closed). Áp trên CẢ câu tổng,
	// câu trang lẫn export.
	teleCond, teleArgs, err := scope.CustomerIDCond(c, h.DB, "payment.customer_id")
	if err != nil {
		response.ServerError(c, err)
		return
	}
	tele := func(q *gorm.DB) *gorm.DB {
		if teleCond == "" {
			return q
		}
		return q.Where(teleCond, teleArgs...)
	}

	if c.Query("_export") == "1" {
		// Export chở trọn danh sách phiếu — cùng quyền với nút Export trên FE (permissionExtPayment);
		// route chỉ gác viewPayment và middleware đang audit-mode ⇒ chặn thật ở đây (super role tự có).
		if !h.hasPermission(middleware.GetUserID(c), define.PermExtPayment) {
			response.Forbidden(c)
			return
		}
		// Export mang ĐỦ JOIN (hàm export select cột customer.*) + CÙNG bộ lọc + sort với màn hình.
		spec.exportFunc(c, applyPaymentSort(c, spec.conds(tele(h.paymentFullBase(c)))))
		return
	}

	// COUNT + SUM gộp MỘT lượt, chỉ JOIN bảng bộ lọc dùng, không ORDER BY.
	q, refs := h.paymentFilters(c, tenant.ScopedDBNullBranch(c, h.DB, "payment").Table("payment"))
	var summary paymentSummary
	tx := withPaymentJoinsFor(spec.conds(tele(q)), refs).
		Select("COUNT(*) AS total" + spec.sumSelect).Scan(&summary)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	items := make([]map[string]interface{}, 0)
	pageQ := applyPaymentSort(c, spec.conds(tele(h.paymentFullBase(c)))).Select(paymentListSelect)
	result := pagination.PaginateWithTotal(c, pageQ, &items, summary.Total)
	if c.Writer.Written() { // PaginateWithTotal đã trả 500 khi lỗi SQL
		return
	}
	if !h.expandPaymentItems(c, items) {
		return
	}

	body := gin.H{"object": result}
	for field, kind := range spec.sumFields {
		if kind == "deposit_after" {
			body[field] = summary.TotalDepositAfter
		} else {
			body[field] = summary.TotalSum
		}
	}
	c.JSON(http.StatusOK, body)
}

// paymentFullBase — FROM payment (scope chi nhánh) + đủ JOIN tra cứu + bộ lọc của request.
func (h *Handler) paymentFullBase(c *gin.Context) *gorm.DB {
	q := tenant.ScopedDBNullBranch(c, h.DB, "payment").Table("payment").
		Joins(paymentJoinCustomer).
		Joins(paymentJoinOrderPlan).
		Joins(paymentJoinServiceUsed).
		Joins(paymentJoinService)
	q, _ = h.paymentFilters(c, q)
	return q
}

func applyPaymentSort(c *gin.Context, q *gorm.DB) *gorm.DB {
	return query.ApplySort(c, q, "payment.id DESC", "payment.id", "payment.payment_of_date", "payment.payment_time")
}

// paymentFilters gắn bộ lọc của request lên q và trả về tập bảng mà các bộ lọc CÓ HIỆU LỰC tham chiếu
// (theo tiền tố cột), để câu tổng chỉ JOIN đúng bảng cần.
func (h *Handler) paymentFilters(c *gin.Context, q *gorm.DB) (*gorm.DB, map[string]bool) {
	refs := map[string]bool{}
	q = trackFilter(q, refs, func(q *gorm.DB) *gorm.DB {
		return query.ApplyKeywordFilter(c, q, paymentKeySearch...)
	}, paymentKeySearch...)
	for _, f := range paymentFilterColumns {
		q = trackFilter(q, refs, func(q *gorm.DB) *gorm.DB { return query.ApplyFilter(c, q, f.key, f.column) }, f.column)
	}
	// Tư vấn viên của dịch vụ: listRevenues là JSON array id — khớp BẤT KỲ id nào được chọn.
	if vals := query.GetFilterValues(c, "_listRevenues"); len(vals) > 0 {
		conds := make([]string, len(vals))
		args := make([]interface{}, len(vals))
		for i, v := range vals {
			conds[i] = "JSON_CONTAINS(order_service_used.listRevenues, ?)"
			args[i] = v
		}
		q = q.Where("("+strings.Join(conds, " OR ")+")", args...)
		refs["order_service_used"] = true
	}
	return q, refs
}

// trackFilter chạy apply; nếu nó THẬT SỰ thêm điều kiện WHERE thì ghi các bảng của cols vào refs.
func trackFilter(q *gorm.DB, refs map[string]bool, apply func(*gorm.DB) *gorm.DB, cols ...string) *gorm.DB {
	before := whereExprCount(q)
	q = apply(q)
	if whereExprCount(q) > before {
		for _, col := range cols {
			if table, _, ok := strings.Cut(col, "."); ok {
				refs[table] = true
			}
		}
	}
	return q
}

func whereExprCount(q *gorm.DB) int {
	if w, ok := q.Statement.Clauses["WHERE"].Expression.(clause.Where); ok {
		return len(w.Exprs)
	}
	return 0
}

// withPaymentJoinsFor thêm đúng các LEFT JOIN mà refs cần (`service` nối qua `order_service_used`).
func withPaymentJoinsFor(q *gorm.DB, refs map[string]bool) *gorm.DB {
	if refs["customer"] {
		q = q.Joins(paymentJoinCustomer)
	}
	if refs["order_plan"] {
		q = q.Joins(paymentJoinOrderPlan)
	}
	if refs["order_service_used"] || refs["service"] {
		q = q.Joins(paymentJoinServiceUsed)
	}
	if refs["service"] {
		q = q.Joins(paymentJoinService)
	}
	return q
}

// parseJSONArrayFields — cột JSON đọc qua map-scan là []byte/string; đổi sang giá trị đã parse.
func parseJSONArrayFields(row map[string]interface{}, keys ...string) {
	for _, k := range keys {
		var raw []byte
		switch v := row[k].(type) {
		case []byte:
			raw = v
		case string:
			raw = []byte(v)
		default:
			continue
		}
		if len(raw) == 0 {
			continue
		}
		var parsed interface{}
		if json.Unmarshal(raw, &parsed) == nil {
			row[k] = parsed
		}
	}
}

// expandPaymentItems gắn quan hệ lồng cho từng phiếu: customer (+ lịch hẹn gần nhất), orderPlan,
// orderServiceUsed, deposit (phiếu cọc gốc). false ⇒ đã trả 500.
func (h *Handler) expandPaymentItems(c *gin.Context, items []map[string]interface{}) bool {
	if len(items) == 0 {
		return true
	}
	customerIds := handlerutil.CollectIDs(items, "customer_id")
	serviceUsedIds := handlerutil.CollectIDs(items, "serviceUsed_id")
	planIds := handlerutil.CollectIDs(items, "parent_id")
	depositIds := handlerutil.CollectIDs(items, "deposit_id")

	if len(customerIds) > 0 {
		custRows := make([]map[string]interface{}, 0)
		tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").
			Select("id, code, old_code, fullname, nickname, phone, avatar, status, address, province_id, country_id, type_service_id, source_id").
			Where("id IN ?", customerIds).Find(&custRows)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return false
		}
		storage.ResolveRows(custRows, "avatar")
		showPhone := phoneutil.CanSeePhone(c, h.DB)
		custMap := make(map[int64]map[string]interface{}, len(custRows))
		for _, r := range custRows {
			if !showPhone {
				if p, ok := r["phone"].(string); ok {
					r["phone"] = phoneutil.MaskPhone(p)
				}
			}
			custMap[helpers.ToInt64(r["id"])] = r
		}
		// Lịch hẹn gần nhất của khách (cột "Ngày hẹn lịch" màn đặt cọc) — trong phạm vi chi nhánh đang
		// xem. JOIN bảng dẫn xuất thay `id IN (SELECT MAX(id) …)` (dạng IN quét toàn bảng appointment).
		latestApptIds := tenant.ScopedDBNullBranch(c, h.DB, "appointment").Table("appointment").
			Select("MAX(id) AS id").
			Where("customer_id IN ?", customerIds).
			Group("customer_id")
		apptRows := make([]map[string]interface{}, 0)
		tx = tenant.ScopedDBNullBranch(c, h.DB, "appointment", "a").Table("appointment AS a").
			Joins("JOIN (?) AS latest ON latest.id = a.id", latestApptIds).
			Select("a.customer_id, a.appointment_date, a.branch_id").
			Find(&apptRows)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return false
		}
		apptMap := make(map[int64]map[string]interface{}, len(apptRows))
		for _, r := range apptRows {
			apptMap[helpers.ToInt64(r["customer_id"])] = r
		}
		for _, item := range items {
			custID := helpers.ToInt64(item["customer_id"])
			cust, ok := custMap[custID]
			if !ok {
				item["customer"] = nil
				continue
			}
			// Bản sao theo từng phiếu: nhiều phiếu chung 1 khách KHÔNG được dùng chung 1 map.
			nested := make(map[string]interface{}, len(cust)+1)
			for k, v := range cust {
				nested[k] = v
			}
			if appt := apptMap[custID]; appt != nil {
				nested["appointment"] = appt
			}
			item["customer"] = nested
		}
	}

	if len(planIds) > 0 {
		rows := make([]map[string]interface{}, 0)
		tx := tenant.CrossBranchLookup(h.DB, "order_plan").Table("order_plan").
			Select("id, code, created_time").
			Where("id IN ?", planIds).Find(&rows)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return false
		}
		m := make(map[int64]map[string]interface{}, len(rows))
		for _, r := range rows {
			m[helpers.ToInt64(r["id"])] = r
		}
		for _, item := range items {
			if p, ok := m[helpers.ToInt64(item["parent_id"])]; ok {
				item["orderPlan"] = p
			}
		}
	}

	if len(serviceUsedIds) > 0 {
		oseRows := make([]map[string]interface{}, 0)
		tx := tenant.CrossBranchLookup(h.DB, "order_service_used").Table("order_service_used").
			Select("id, code, service_id, quantity, totalEnd, payment, isLock, created_day, listRevenues, source_id").
			Where("id IN ?", serviceUsedIds).Find(&oseRows)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return false
		}
		helpers.ParseDecimalFieldsForTables(oseRows, "order_service_used")
		oseMap := make(map[int64]map[string]interface{}, len(oseRows))
		for _, r := range oseRows {
			parseJSONArrayFields(r, "listRevenues")
			oseMap[helpers.ToInt64(r["id"])] = r
		}
		for _, item := range items {
			if ose, ok := oseMap[helpers.ToInt64(item["serviceUsed_id"])]; ok {
				item["orderServiceUsed"] = ose
			}
		}
	}

	if len(depositIds) > 0 {
		handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "deposit_id", "deposit",
			"payment", "id", "id, code, note", depositIds)
	}
	return true
}
