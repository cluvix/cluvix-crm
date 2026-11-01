package handler

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"erpCluvix/internal/middleware"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tz"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// ── Helpers ──

// buildScopeFilter trả SQL fragment scope branch cho raw SQL report.
// Story-38: bỏ vế company (1 thương hiệu/DB — không cần lọc); GIỮ NGUYÊN hành vi branch.
func buildScopeFilter(c *gin.Context, table string) (string, []interface{}) {
	filter := ""
	args := []interface{}{}

	branchActiveID := middleware.GetBranchActiveID(c)
	branchIDs := middleware.GetBranchIDs(c)
	if branchActiveID > 0 {
		filter += fmt.Sprintf(" AND (%s.branch_id = ? OR %s.branch_id IS NULL)", table, table)
		args = append(args, branchActiveID)
	} else if len(branchIDs) > 0 {
		filter += fmt.Sprintf(" AND (%s.branch_id IN (%s) OR %s.branch_id IS NULL)", table, placeholders(len(branchIDs)), table)
		for _, id := range branchIDs {
			args = append(args, id)
		}
	}

	return filter, args
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// dimStatDecimalAliases — alias của biểu thức SUM(...)/phép chia trong dimensionStatistics,
// BranchStatistics, GroupSourceStatistics. MySQL trả DECIMAL → driver trả []uint8 → JSON ra string.
// Alias KHÔNG thuộc bảng nào nên ParseDecimalFieldsForTables không đụng tới → phải liệt kê tường minh.
//
// story-34 (F-E): 5 alias cuối (totalLead..totalAppointmentDongY) CHỈ thật sự cần ở
// GroupSourceStatistics — nơi chúng là SUM(COUNT-alias) ở lớp ngoài (MySQL nâng kiểu SUM lên DECIMAL
// dù nguồn là COUNT). Ở dimensionStatistics/BranchStatistics cùng tên field là COUNT thuần (BIGINT,
// đã ra number đúng) — thêm vào list dùng chung vẫn AN TOÀN cho 2 hàm đó vì ToDecimal(int64) giữ
// nguyên giá trị, JSON output không đổi (đã verify qua accountant/report cùng pattern).
var dimStatDecimalAliases = []string{
	"totalOrderPlan", "totalVat", "totalRevenue", "totalPayment", "totalDeposit",
	"totalRefundDeposit", "totalRefundServiceUsed", "duKienThu", "tbKhach",
	"totalLead", "totalAppointmentNew", "totalAppointment", "totalAppointmentDen", "totalAppointmentDongY",
}

func paginateAndRespond(c *gin.Context, rows []map[string]interface{}) {
	for _, r := range rows {
		helpers.ParseDecimalFields(r, dimStatDecimalAliases...)
	}

	// Sort
	sortField := c.Query("sort")
	if sortField == "-lead" || sortField == "-totalLead" {
		for i := 0; i < len(rows); i++ {
			for j := i + 1; j < len(rows); j++ {
				if helpers.ToInt64(rows[j]["totalLead"]) > helpers.ToInt64(rows[i]["totalLead"]) {
					rows[i], rows[j] = rows[j], rows[i]
				}
			}
		}
	}

	// Totals on ALL rows
	totals := gin.H{
		"totalLead": 0, "totalAppointmentNew": 0, "totalAppointment": 0,
		"totalAppointmentDen": 0, "totalAppointmentDongY": 0,
	}
	var tOrderPlan, tVat, tRevenue, tPayment, tDeposit, tRefundDeposit, tRefundService decimal.Decimal
	for _, r := range rows {
		totals["totalLead"] = totals["totalLead"].(int) + int(helpers.ToInt64(r["totalLead"]))
		totals["totalAppointmentNew"] = totals["totalAppointmentNew"].(int) + int(helpers.ToInt64(r["totalAppointmentNew"]))
		totals["totalAppointment"] = totals["totalAppointment"].(int) + int(helpers.ToInt64(r["totalAppointment"]))
		totals["totalAppointmentDen"] = totals["totalAppointmentDen"].(int) + int(helpers.ToInt64(r["totalAppointmentDen"]))
		totals["totalAppointmentDongY"] = totals["totalAppointmentDongY"].(int) + int(helpers.ToInt64(r["totalAppointmentDongY"]))
		tOrderPlan = tOrderPlan.Add(helpers.ToDecimal(r["totalOrderPlan"]))
		tVat = tVat.Add(helpers.ToDecimal(r["totalVat"]))
		tRevenue = tRevenue.Add(helpers.ToDecimal(r["totalRevenue"]))
		tPayment = tPayment.Add(helpers.ToDecimal(r["totalPayment"]))
		tDeposit = tDeposit.Add(helpers.ToDecimal(r["totalDeposit"]))
		tRefundDeposit = tRefundDeposit.Add(helpers.ToDecimal(r["totalRefundDeposit"]))
		tRefundService = tRefundService.Add(helpers.ToDecimal(r["totalRefundServiceUsed"]))
	}
	totals["totalOrderPlan"] = tOrderPlan
	totals["totalVat"] = tVat
	totals["totalRevenue"] = tRevenue
	totals["totalPayment"] = tPayment
	totals["totalDeposit"] = tDeposit
	totals["totalRefundDeposit"] = tRefundDeposit
	totals["totalRefundServiceUsed"] = tRefundService
	// Dự kiến thu = phải thu (doanh số TRƯỚC thuế + VAT, bmad/order-vat) − đã thu (CÓ thuế) của cùng tập dịch vụ.
	totals["duKienThu"] = tOrderPlan.Add(tVat).Sub(tRevenue)
	tLead := totals["totalLead"].(int)
	tApp := totals["totalAppointment"].(int)
	tDen := totals["totalAppointmentDen"].(int)
	tAppNew := totals["totalAppointmentNew"].(int)
	tDongY := totals["totalAppointmentDongY"].(int)
	if tLead > 0 {
		totals["appointmentNewLead"] = decimal.NewFromInt(int64(tAppNew)).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(int64(tLead)))
	} else {
		totals["appointmentNewLead"] = decimal.Zero
	}
	if tApp > 0 {
		totals["denAppointment"] = decimal.NewFromInt(int64(tDen)).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(int64(tApp)))
	} else {
		totals["denAppointment"] = decimal.Zero
	}
	if tDen > 0 {
		totals["dongYDen"] = decimal.NewFromInt(int64(tDongY)).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(int64(tDen)))
	} else {
		totals["dongYDen"] = decimal.Zero
	}
	if tDongY > 0 {
		totals["tbKhach"] = tOrderPlan.Div(decimal.NewFromInt(int64(tDongY)))
	} else {
		totals["tbKhach"] = decimal.Zero
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per-page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 20
	}

	total := len(rows)
	pageCount := int(math.Ceil(float64(total) / float64(perPage)))
	offset := (page - 1) * perPage
	end := offset + perPage
	if end > total {
		end = total
	}

	var items []map[string]interface{}
	if offset < total {
		items = rows[offset:end]
	} else {
		items = make([]map[string]interface{}, 0)
	}

	c.Header("X-Pagination-Total-Count", strconv.Itoa(total))
	c.Header("X-Pagination-Page-Count", strconv.Itoa(pageCount))
	c.Header("X-Pagination-Current-Page", strconv.Itoa(page))
	c.Header("X-Pagination-Per-Page", strconv.Itoa(perPage))

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"_meta": gin.H{
			"totalCount":  total,
			"pageCount":   pageCount,
			"currentPage": page,
			"perPage":     perPage,
		},
		"totals": totals,
	})
}

func getDateRange(c *gin.Context) (string, string) {
	from := c.Query("filter[created_at][gte]")
	to := c.Query("filter[created_at][lte]")
	if from == "" {
		from = "0"
	}
	if to == "" {
		to = fmt.Sprintf("%d", 9999999999)
	}
	return from, to
}

// ── Dimension Statistics (generic) ──

func (h *Handler) dimensionStatistics(c *gin.Context, masterTable, selectCols, dimColumn, customerDimCol, appointmentDimCol string, useOsuFinancial bool) {
	// Bắt buộc có dải ngày và trần 1 năm — hàm này phục vụ 8 endpoint `*-statistics`
	// (source · pathology · province · country · category · page-facebook · campain-event ·
	// interested) nên guard đặt ở đây phủ cả 8. Lưu ý `service-type-statistics` KHÔNG nằm
	// trong nhóm này — nó là handler riêng, đừng gộp vào khi rà consumer. `getDateRange` (dùng chung) tự đặt
	// `from=0 / to=9999999999` khi thiếu ngày, và nút xoá ngày trên bộ lọc FE đi thẳng vào đó.
	// Đo @872.284 lịch hẹn / 615.992 khách / 449.360 osu / 372.504 payment, toàn bộ chi nhánh
	// (2026-09-09), KỂ CẢ SAU khi đã gộp: ca không-lọc-ngày `useOsuFinancial=false`
	// **759.208 dòng InnoDB / 0,79–1,16s**, `=true` **2.963.026 dòng / 6,1–14,4s**.
	// Vượt trần ⇒ 422 tường minh, KHÔNG tự cắt dải ngày (cắt = trả thiếu số liệu im lặng).
	fromSec, errFrom := strconv.ParseInt(c.Query("filter[created_at][gte]"), 10, 64)
	toSec, errTo := strconv.ParseInt(c.Query("filter[created_at][lte]"), 10, 64)
	if errFrom != nil || errTo != nil || toSec < fromSec {
		response.Validate(c, i18n.T(c, "report_date_range_required"))
		return
	}
	if toSec-fromSec > define.ReportMaxRangeSec {
		response.Validate(c, i18n.T(c, "report_date_range_max_one_year"))
		return
	}

	from, to := getDateRange(c)
	custScope, custScopeArgs := buildScopeFilter(c, "customer")
	appScope, appScopeArgs := buildScopeFilter(c, "appointment")
	osuScope, osuScopeArgs := buildScopeFilter(c, "osu")
	payScope, payScopeArgs := buildScopeFilter(c, "p")

	dimCol := strings.TrimPrefix(customerDimCol, "customer.")

	appJoinCustomer := ""
	if strings.HasPrefix(appointmentDimCol, "customer.") {
		appJoinCustomer = " LEFT JOIN customer ON customer.id = appointment.customer_id"
	}

	// 4 cột tài chính phải gọi đúng tên bảng dẫn xuất của TỪNG biến thể: nhánh
	// `useOsuFinancial=true` gộp thành RU (qua osu) + ST (qua customer), nhánh còn lại gộp
	// trọn thành RSTU. SELECT là phần DÙNG CHUNG nên danh sách cột phải tham số hoá theo cờ.
	finCols := `COALESCE(RSTU.totalPayment, 0) AS totalPayment,
		COALESCE(RSTU.totalDeposit, 0) AS totalDeposit,
		COALESCE(RSTU.totalRefundDeposit, 0) AS totalRefundDeposit,
		COALESCE(RSTU.totalRefundServiceUsed, 0) AS totalRefundServiceUsed,`
	if useOsuFinancial {
		finCols = `COALESCE(RU.totalPayment, 0) AS totalPayment,
		COALESCE(ST.totalDeposit, 0) AS totalDeposit,
		COALESCE(ST.totalRefundDeposit, 0) AS totalRefundDeposit,
		COALESCE(RU.totalRefundServiceUsed, 0) AS totalRefundServiceUsed,`
	}

	crmSQL := fmt.Sprintf(`SELECT %s,
		COALESCE(A.totalLead, 0) as totalLead,
		COALESCE(E.totalAppointmentNew, 0) as totalAppointmentNew,
		COALESCE(BCD.totalAppointment, 0) as totalAppointment,
		COALESCE(BCD.totalAppointmentDen, 0) as totalAppointmentDen,
		COALESCE(BCD.totalAppointmentDongY, 0) as totalAppointmentDongY,
		COALESCE(P.totalOrderPlan, 0) AS totalOrderPlan,
		COALESCE(P.totalVat, 0) AS totalVat,
		COALESCE(Q.totalRevenue, 0) AS totalRevenue,
		%s
		(COALESCE(P.totalOrderPlan, 0) + COALESCE(P.totalVat, 0) - COALESCE(Q.totalRevenue, 0)) AS duKienThu,
		(CASE WHEN COALESCE(BCD.totalAppointmentDongY, 0) = 0 THEN 0
			ELSE COALESCE(P.totalOrderPlan, 0) / COALESCE(BCD.totalAppointmentDongY, 0) END) AS tbKhach
	FROM %s m
	LEFT JOIN (
		SELECT %s as dim_id, COUNT(*) as totalLead
		FROM customer WHERE %s IS NOT NULL AND created_day BETWEEN ? AND ?%s
		GROUP BY dim_id
	) A ON m.id = A.dim_id
	LEFT JOIN (
		SELECT App.dim_id, COUNT(App.id) as totalAppointmentNew FROM customer
		LEFT JOIN (
			SELECT appointment.id, customer_id, %s as dim_id
			FROM appointment%s
			INNER JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id
			WHERE %s IS NOT NULL AND appointment_type.isTypeRevenue = 1
				AND appointment.created_at BETWEEN ? AND ?%s
			GROUP BY customer_id, appointment.id, dim_id
		) App ON customer.id = App.customer_id
		GROUP BY App.dim_id
	) E ON m.id = E.dim_id
	LEFT JOIN (
		-- Gop B/C/D: ba chi so doc CUNG dai appointment_date, khac dung dieu kien status /
		-- is_accepted. Do @872.284 lich hen, loc thang mac dinh, toan chi nhanh (2026-09-09):
		--   useOsuFinancial=false: 2.683.539 dong InnoDB / 0,94-1,22s -> 717.130 dong / 0,33-0,61s
		--   useOsuFinancial=true : 2.669.676 dong / 1,21-1,28s        -> 705.889 dong / 0,41-0,43s
		-- Tang trong gom theo appointment.id (khoa chinh) nen moi nhom 1 dong; MAX() chi de thoa
		-- ONLY_FULL_GROUP_BY, KHONG phai lua chon ngu nghia (da do: MIN cho ket qua y het).
		-- Ban cu dung LEFT JOIN customer khong kem WHERE dim_id IS NOT NULL nen sinh them nhom
		-- dim_id = NULL -- nhom do khong khop m.id nen khong vao ket qua; ban gop dung INNER JOIN.
		-- Doi chieu 4 bo loc x 2 bien the: khop tuyet doi.
		SELECT App.dim_id,
			COUNT(App.id) as totalAppointment,
			COUNT(CASE WHEN App.den THEN App.id END) as totalAppointmentDen,
			COUNT(CASE WHEN App.dongY THEN App.id END) as totalAppointmentDongY
		FROM customer
		INNER JOIN (
			SELECT appointment.id, customer_id, %s as dim_id,
				MAX(appointment.status IN (2, 3)) as den,
				MAX(appointment.status IN (2, 3) AND appointment.is_accepted = 1) as dongY
			FROM appointment%s
			INNER JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id
			WHERE %s IS NOT NULL AND appointment_type.isTypeRevenue = 1
				AND appointment_date BETWEEN ? AND ?%s
			GROUP BY customer_id, appointment.id, dim_id
		) App ON customer.id = App.customer_id
		GROUP BY App.dim_id
	) BCD ON m.id = BCD.dim_id`,
		selectCols, finCols, masterTable,
		customerDimCol, customerDimCol, custScope,
		appointmentDimCol, appJoinCustomer, appointmentDimCol, appScope, // E: lịch mới (created_at)
		appointmentDimCol, appJoinCustomer, appointmentDimCol, appScope) // BCD: gộp 3 chỉ số

	var financialSQL string
	// ⚠ Danh sách tham số phải dựng RIÊNG cho từng biến thể sau khi gộp nhánh tài chính:
	// số verb `%d`/`%s` của hai nhánh nay KHÁC nhau (true: 13, false: 12). `financialSQLArgs`
	// là slice động nên `go vet` KHÔNG kiểm được — lệch thì câu SQL mang `%!d(MISSING)` và
	// MySQL từ chối, chỉ lộ ra lúc chạy thật. Đã neo bằng test integration cho CẢ HAI biến thể.
	var financialSQLArgs []interface{}
	if useOsuFinancial {
		financialSQLArgs = []interface{}{
			osuScope,                                                       // P
			define.PaymentForOrder, define.PaymentForMoveDeposit, payScope, // Q
			// RU: payment_for IN (Order, Deposit) NOT KG · refund service · scope
			define.PaymentForOrder, define.PaymentForDeposit, define.PaymentTypeDepositKG,
			define.PaymentForRefundService, payScope,
			// ST: deposit NOT KG · refund deposit · scope
			define.PaymentForDeposit, define.PaymentTypeDepositKG,
			define.PaymentForRefundDeposit, payScope,
		}
	} else {
		financialSQLArgs = []interface{}{
			osuScope,                                                       // P
			define.PaymentForOrder, define.PaymentForMoveDeposit, payScope, // Q
			// RSTU: 4 chỉ số gộp một lượt quét
			define.PaymentForOrder, define.PaymentForDeposit, define.PaymentTypeDepositKG,
			define.PaymentForDeposit, define.PaymentTypeDepositKG,
			define.PaymentForRefundDeposit,
			define.PaymentForRefundService, payScope,
		}
	}
	if useOsuFinancial {
		// P, Q, R, U dùng osu.source_id (nguồn trên chính dịch vụ/thanh toán)
		// S, T (deposit/refund deposit) không có osu link → vẫn dùng customer.source_id
		financialSQL = fmt.Sprintf(`
LEFT JOIN (
	SELECT osu.`+dimCol+`, SUM(osu.totalEnd) AS totalOrderPlan, SUM(osu.vat_amount) AS totalVat
	FROM order_service_used osu
	WHERE osu.`+dimCol+` IS NOT NULL AND osu.created_day BETWEEN ? AND ?%s
	GROUP BY osu.`+dimCol+`
) P ON m.id = P.`+dimCol+`
LEFT JOIN (
	SELECT osu.`+dimCol+`, SUM(p.amount) AS totalRevenue
	FROM payment p
	INNER JOIN order_service_used osu ON osu.id = p.serviceUsed_id
	WHERE osu.`+dimCol+` IS NOT NULL
		AND p.payment_for IN (%d, %d)
		AND osu.created_day BETWEEN ? AND ?
		AND p.payment_of_date BETWEEN ? AND ?%s
	GROUP BY osu.`+dimCol+`
) Q ON m.id = Q.`+dimCol+`
LEFT JOIN (
	-- Gop R+U: hai chi so doc CUNG cap bang (payment x osu), CUNG dai payment_time,
	-- khac dung payment_for va cot duoc SUM. Mot luot quet thay cho hai.
	SELECT osu.`+dimCol+`,
		SUM(CASE WHEN p.payment_for IN (%d, %d) AND (p.deposit_type IS NULL OR p.deposit_type NOT IN (%d)) THEN p.amount ELSE 0 END) AS totalPayment,
		SUM(CASE WHEN p.payment_for = %d THEN p.amount ELSE 0 END) AS totalRefundServiceUsed
	FROM payment p
	INNER JOIN order_service_used osu ON osu.id = p.serviceUsed_id
	WHERE osu.`+dimCol+` IS NOT NULL AND p.payment_time BETWEEN ? AND ?%s
	GROUP BY osu.`+dimCol+`
) RU ON m.id = RU.`+dimCol+`
LEFT JOIN (
	-- Gop S+T: hai chi so con lai di qua customer.`+dimCol+` (khong co lien ket osu),
	-- cung dai payment_time. Mot luot quet thay cho hai.
	SELECT customer.`+dimCol+`,
		SUM(CASE WHEN p.payment_for = %d AND (p.deposit_type IS NULL OR p.deposit_type NOT IN (%d)) THEN p.deposit_after ELSE 0 END) AS totalDeposit,
		SUM(CASE WHEN p.payment_for = %d THEN p.amount ELSE 0 END) AS totalRefundDeposit
	FROM payment p
	LEFT JOIN customer ON customer.id = p.customer_id
	WHERE customer.`+dimCol+` IS NOT NULL AND p.payment_time BETWEEN ? AND ?%s
	GROUP BY customer.`+dimCol+`
) ST ON m.id = ST.`+dimCol+`
WHERE COALESCE(A.totalLead,0) != 0 OR COALESCE(E.totalAppointmentNew,0) != 0
	OR COALESCE(BCD.totalAppointment,0) != 0 OR COALESCE(BCD.totalAppointmentDen,0) != 0
	OR COALESCE(BCD.totalAppointmentDongY,0) != 0
	OR COALESCE(P.totalOrderPlan,0) != 0 OR COALESCE(Q.totalRevenue,0) != 0
	OR COALESCE(RU.totalPayment,0) != 0`, financialSQLArgs...)
	} else {
		financialSQL = fmt.Sprintf(`
LEFT JOIN (
	SELECT customer.`+dimCol+`, SUM(osu.totalEnd) AS totalOrderPlan, SUM(osu.vat_amount) AS totalVat
	FROM order_service_used osu
	LEFT JOIN customer ON customer.id = osu.customer_id
	WHERE customer.`+dimCol+` IS NOT NULL AND osu.created_day BETWEEN ? AND ?%s
	GROUP BY customer.`+dimCol+`
) P ON m.id = P.`+dimCol+`
LEFT JOIN (
	SELECT customer.`+dimCol+`, SUM(p.amount) AS totalRevenue
	FROM payment p
	INNER JOIN order_service_used osu ON osu.id = p.serviceUsed_id
	LEFT JOIN customer ON customer.id = p.customer_id
	WHERE customer.`+dimCol+` IS NOT NULL
		AND p.payment_for IN (%d, %d)
		AND osu.created_day BETWEEN ? AND ?
		AND p.payment_of_date BETWEEN ? AND ?%s
	GROUP BY customer.`+dimCol+`
) Q ON m.id = Q.`+dimCol+`
LEFT JOIN (
	-- Gop R/S/T/U: bon chi so doc CUNG bang payment, CUNG dai payment_time, khac dung
	-- payment_for / deposit_type / cot duoc SUM. Mot luot quet thay cho bon.
	-- Do @372.504 payment, loc thang mac dinh, toan chi nhanh (2026-09-09), TOAN cau:
	-- 2.683.539 dong InnoDB / 0,94-1,22s -> 717.130 dong / 0,33-0,61s.
	SELECT customer.`+dimCol+`,
		SUM(CASE WHEN p.payment_for IN (%d, %d) AND (p.deposit_type IS NULL OR p.deposit_type NOT IN (%d)) THEN p.amount ELSE 0 END) AS totalPayment,
		SUM(CASE WHEN p.payment_for = %d AND (p.deposit_type IS NULL OR p.deposit_type NOT IN (%d)) THEN p.deposit_after ELSE 0 END) AS totalDeposit,
		SUM(CASE WHEN p.payment_for = %d THEN p.amount ELSE 0 END) AS totalRefundDeposit,
		SUM(CASE WHEN p.payment_for = %d THEN p.amount ELSE 0 END) AS totalRefundServiceUsed
	FROM payment p
	LEFT JOIN customer ON customer.id = p.customer_id
	WHERE customer.`+dimCol+` IS NOT NULL AND p.payment_time BETWEEN ? AND ?%s
	GROUP BY customer.`+dimCol+`
) RSTU ON m.id = RSTU.`+dimCol+`
WHERE COALESCE(A.totalLead,0) != 0 OR COALESCE(E.totalAppointmentNew,0) != 0
	OR COALESCE(BCD.totalAppointment,0) != 0 OR COALESCE(BCD.totalAppointmentDen,0) != 0
	OR COALESCE(BCD.totalAppointmentDongY,0) != 0
	OR COALESCE(P.totalOrderPlan,0) != 0 OR COALESCE(Q.totalRevenue,0) != 0
	OR COALESCE(RSTU.totalPayment,0) != 0`, financialSQLArgs...)
	}

	sql := crmSQL + financialSQL

	args := []interface{}{}
	args = append(args, from, to)
	args = append(args, custScopeArgs...)
	// 2 khối appointment: E (created_at) và BCD (appointment_date, gộp 3 chỉ số)
	for i := 0; i < 2; i++ {
		args = append(args, from, to)
		args = append(args, appScopeArgs...)
	}
	args = append(args, from, to)
	args = append(args, osuScopeArgs...)
	args = append(args, from, to, from, to)
	args = append(args, payScopeArgs...)
	args = append(args, from, to)
	args = append(args, payScopeArgs...)
	args = append(args, from, to)
	args = append(args, payScopeArgs...)
	args = append(args, from, to)
	args = append(args, payScopeArgs...)
	args = append(args, from, to)
	args = append(args, payScopeArgs...)

	rows := make([]map[string]interface{}, 0)
	tx := h.DB.Raw(sql, args...).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	paginateAndRespond(c, rows)
}

// ── Report Handlers ──

// AppointmentStatistics GET /crm/report/appointment-statistics
func (h *Handler) AppointmentStatistics(c *gin.Context) {
	// Bắt buộc có dải ngày và trần 1 năm. Trước đây thiếu ngày thì handler tự đặt
	// from=0 / to=9999999999 ⇒ quét CẠN bảng `appointment` — mà nút xoá ngày trên bộ lọc FE
	// đi thẳng vào ca đó. Đo @1.090.355 lịch hẹn, toàn bộ chi nhánh: không-lọc 2.729.935 dòng
	// InnoDB, cửa sổ 1 năm 2.718.239 dòng — hai ca này quyết định trần dưới đây.
	// Vượt trần ⇒ 422 tường minh, KHÔNG tự cắt dải ngày (cắt = trả thiếu số liệu im lặng).
	from := c.Query("_created_at[gte]")
	to := c.Query("_created_at[lte]")
	fromSec, errFrom := strconv.ParseInt(from, 10, 64)
	toSec, errTo := strconv.ParseInt(to, 10, 64)
	if errFrom != nil || errTo != nil || toSec < fromSec {
		response.Validate(c, i18n.T(c, "report_date_range_required"))
		return
	}
	if toSec-fromSec > define.ReportMaxRangeSec {
		response.Validate(c, i18n.T(c, "report_date_range_max_one_year"))
		return
	}

	extraFilter := ""
	extraArgs := []interface{}{}

	// Tenant scope
	scopeFilter, scopeArgs := buildScopeFilter(c, "appointment")
	extraFilter += scopeFilter
	extraArgs = append(extraArgs, scopeArgs...)

	if vals := query.GetFilterValues(c, "branch_id"); len(vals) > 0 {
		extraFilter += fmt.Sprintf(" AND appointment.branch_id IN (%s)", placeholders(len(vals)))
		for _, v := range vals {
			extraArgs = append(extraArgs, v)
		}
	}
	if vals := query.GetFilterValues(c, "appointment_type_id"); len(vals) > 0 {
		extraFilter += fmt.Sprintf(" AND appointment.appointment_type_id IN (%s)", placeholders(len(vals)))
		for _, v := range vals {
			extraArgs = append(extraArgs, v)
		}
	}

	// dayCreate dùng FLOOR-arithmetic theo company TZ — tránh phụ thuộc MySQL session TZ.
	offsetSec := tz.OffsetSec(tz.LocFromContext(c, h.DB))
	dayBucket := func(col string) string {
		return fmt.Sprintf("FLOOR((%s + %d) / 86400) * 86400 - %d", col, offsetSec, offsetSec)
	}

	// 6 chỉ số = 6 lát của ĐÚNG HAI tập dữ liệu: (n) lịch hẹn TẠO trong kỳ, (d) lịch hẹn CÓ NGÀY HẸN
	// trong kỳ. Bản cũ viết 6 subquery độc lập ⇒ 6 lượt quét dải ngày trên `appointment` + 6 bảng tạm
	// gom nhóm, cộng thêm `LEFT JOIN customer` vô điều kiện ở cả 6 (MySQL KHÔNG loại-bỏ-left-join nên
	// mỗi câu tra PK `customer` cho TỪNG dòng lịch hẹn, dù 5/6 câu không đọc cột nào của nó).
	// Nay gom về 2 CTE quét dải ngày MỘT LẦN mỗi cột; 4 chỉ số theo `appointment_date` tách nhau bằng
	// MIN(CASE WHEN …) nên giữ nguyên ngữ nghĩa "ngày sớm nhất của KHÁCH trong tập con đó".
	// `numAppNewPlus` chỉ là `numAppNew` lọc thêm khách cũ (`customer.created_day` là thuộc tính của
	// KHÁCH, hằng số trong nhóm) ⇒ join `customer` một lần trên ~195k khách thay vì 1,09M lịch hẹn.
	// Đo @1.090.355 lịch hẹn / 615.992 khách, cửa sổ 1 năm, toàn bộ chi nhánh (2026-09-09):
	//   12.731.465 -> 7.070.190 dòng InnoDB (-44%) · 27,5s -> ~4,7s. Kết quả khớp tuyệt đối 362/362 dòng.
	// ⚠ Thêm chỉ số mới đụng cột của `customer` thì join vào NHÁNH của chỉ số đó (trên CTE, theo
	// khách), ĐỪNG gắn join vào CTE — làm thế là quay lại đúng bệnh cũ.
	cteScan := fmt.Sprintf(`WITH d AS (
		SELECT appointment.customer_id AS cid,
			MIN(appointment.appointment_date) AS dAll,
			MIN(CASE WHEN appointment.status = 4 THEN appointment.appointment_date END) AS dCancel,
			MIN(CASE WHEN appointment.status IN (2,3) THEN appointment.appointment_date END) AS dDen,
			MIN(CASE WHEN appointment.status IN (2,3) AND appointment.is_accepted = 1 THEN appointment.appointment_date END) AS dDenOk
		FROM appointment
		WHERE appointment.appointment_date BETWEEN ? AND ?%s
		GROUP BY appointment.customer_id
	), n AS (
		SELECT appointment.customer_id AS cid, MIN(appointment.created_at) AS createdAt
		FROM appointment
		WHERE appointment.created_at BETWEEN ? AND ?%s
		GROUP BY appointment.customer_id
	)`, extraFilter, extraFilter)

	sql := cteScan + `
	SELECT dayCreate,
		SUM(numAppNew) as numAppNew, SUM(numAppNewPlus) as numAppNewPlus,
		SUM(numAppDate) as numAppDate, SUM(numAppCancel) as numAppCancel,
		SUM(numAppDen) as numAppDen, SUM(numAppDenOk) as numAppDenOk
	FROM (
		(SELECT ` + dayBucket("n.createdAt") + ` as dayCreate,
			COUNT(*) as numAppNew, 0 as numAppNewPlus, 0 as numAppDate, 0 as numAppCancel, 0 as numAppDen, 0 as numAppDenOk
		FROM n GROUP BY dayCreate)
		UNION ALL
		(SELECT ` + dayBucket("n.createdAt") + ` as dayCreate,
			0 as numAppNew, COUNT(*) as numAppNewPlus, 0 as numAppDate, 0 as numAppCancel, 0 as numAppDen, 0 as numAppDenOk
		FROM n JOIN customer ON customer.id = n.cid WHERE customer.created_day < ? GROUP BY dayCreate)
		UNION ALL
		(SELECT ` + dayBucket("d.dAll") + ` as dayCreate,
			0 as numAppNew, 0 as numAppNewPlus, COUNT(*) as numAppDate, 0 as numAppCancel, 0 as numAppDen, 0 as numAppDenOk
		FROM d WHERE d.dAll IS NOT NULL GROUP BY dayCreate)
		UNION ALL
		(SELECT ` + dayBucket("d.dCancel") + ` as dayCreate,
			0 as numAppNew, 0 as numAppNewPlus, 0 as numAppDate, COUNT(*) as numAppCancel, 0 as numAppDen, 0 as numAppDenOk
		FROM d WHERE d.dCancel IS NOT NULL GROUP BY dayCreate)
		UNION ALL
		(SELECT ` + dayBucket("d.dDen") + ` as dayCreate,
			0 as numAppNew, 0 as numAppNewPlus, 0 as numAppDate, 0 as numAppCancel, COUNT(*) as numAppDen, 0 as numAppDenOk
		FROM d WHERE d.dDen IS NOT NULL GROUP BY dayCreate)
		UNION ALL
		(SELECT ` + dayBucket("d.dDenOk") + ` as dayCreate,
			0 as numAppNew, 0 as numAppNewPlus, 0 as numAppDate, 0 as numAppCancel, 0 as numAppDen, COUNT(*) as numAppDenOk
		FROM d WHERE d.dDenOk IS NOT NULL GROUP BY dayCreate)
	) X
	GROUP BY dayCreate
	ORDER BY dayCreate DESC`

	// Thứ tự args PHẢI theo thứ tự placeholder trong văn bản SQL: CTE d, CTE n, rồi nhánh NewPlus.
	args := []interface{}{}
	args = append(args, from, to)
	args = append(args, extraArgs...)
	args = append(args, from, to)
	args = append(args, extraArgs...)
	args = append(args, from)

	rows := make([]map[string]interface{}, 0)
	tx := h.DB.Raw(sql, args...).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	// story-34 (F-D/F-E): `dayCreate` = FLOOR(.../86400)*86400-offset — phép chia MySQL trả DECIMAL
	// nên ra JSON string dù không phải cột tiền; numAppNew.../numAppDenOk là SUM(COUNT(*)) — MySQL
	// nâng kiểu SUM lên DECIMAL dù nguồn là COUNT. Cả 2 lớp không được ParseDecimalFieldsForTables
	// phủ (không phải cột thật của bảng nào) — parse tường minh theo alias.
	for _, r := range rows {
		helpers.ParseDecimalFields(r, "dayCreate", "numAppNew", "numAppNewPlus", "numAppDate",
			"numAppCancel", "numAppDen", "numAppDenOk")
	}

	// Totals
	totals := gin.H{
		"numAppNew": 0, "numAppNewPlus": 0, "numAppDate": 0,
		"numAppCancel": 0, "numAppDen": 0, "numAppDenOk": 0,
	}
	for _, r := range rows {
		totals["numAppNew"] = totals["numAppNew"].(int) + int(helpers.ToInt64(r["numAppNew"]))
		totals["numAppNewPlus"] = totals["numAppNewPlus"].(int) + int(helpers.ToInt64(r["numAppNewPlus"]))
		totals["numAppDate"] = totals["numAppDate"].(int) + int(helpers.ToInt64(r["numAppDate"]))
		totals["numAppCancel"] = totals["numAppCancel"].(int) + int(helpers.ToInt64(r["numAppCancel"]))
		totals["numAppDen"] = totals["numAppDen"].(int) + int(helpers.ToInt64(r["numAppDen"]))
		totals["numAppDenOk"] = totals["numAppDenOk"].(int) + int(helpers.ToInt64(r["numAppDenOk"]))
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per-page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 20
	}

	total := len(rows)
	pageCount := int(math.Ceil(float64(total) / float64(perPage)))
	offset := (page - 1) * perPage
	end := offset + perPage
	if end > total {
		end = total
	}

	var items []map[string]interface{}
	if offset < total {
		items = rows[offset:end]
	} else {
		items = make([]map[string]interface{}, 0)
	}

	c.Header("X-Pagination-Total-Count", strconv.Itoa(total))
	c.Header("X-Pagination-Page-Count", strconv.Itoa(pageCount))
	c.Header("X-Pagination-Current-Page", strconv.Itoa(page))
	c.Header("X-Pagination-Per-Page", strconv.Itoa(perPage))

	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"_meta":  gin.H{"totalCount": total, "pageCount": pageCount, "currentPage": page, "perPage": perPage},
		"totals": totals,
	})
}

// BranchStatistics GET /crm/report/branch-statistics
// Trả về cả CRM funnel (lead → accepted) lẫn financial (order, revenue, payment, deposit).
func (h *Handler) BranchStatistics(c *gin.Context) {
	// Bắt buộc có dải ngày và trần 1 năm. `getDateRange` (dùng chung) tự đặt
	// `from=0 / to=9999999999` khi thiếu ngày, và nút xoá ngày trên bộ lọc FE đi thẳng vào đó.
	// Báo cáo này có 11 bảng dẫn xuất; đo @872.284 lịch hẹn / 615.992 khách / 449.360 osu /
	// 372.504 payment (2026-09-09), KỂ CẢ SAU khi đã gộp 7 lượt quét thành 2, ca không-lọc-ngày
	// vẫn **2.598.623 dòng InnoDB / 3,80–5,21s** (trước gộp: 4.659.669 dòng / 4,72–6,40s).
	// Vượt trần ⇒ 422 tường minh, KHÔNG tự cắt dải ngày (cắt = trả thiếu số liệu im lặng).
	fromSec, errFrom := strconv.ParseInt(c.Query("filter[created_at][gte]"), 10, 64)
	toSec, errTo := strconv.ParseInt(c.Query("filter[created_at][lte]"), 10, 64)
	if errFrom != nil || errTo != nil || toSec < fromSec {
		response.Validate(c, i18n.T(c, "report_date_range_required"))
		return
	}
	if toSec-fromSec > define.ReportMaxRangeSec {
		response.Validate(c, i18n.T(c, "report_date_range_max_one_year"))
		return
	}

	from, to := getDateRange(c)
	custScope, custScopeArgs := buildScopeFilter(c, "customer")
	appScope, appScopeArgs := buildScopeFilter(c, "appointment")
	osuScope, osuScopeArgs := buildScopeFilter(c, "osu")
	payScope, payScopeArgs := buildScopeFilter(c, "p")

	sql := fmt.Sprintf(`
SELECT b.id, b.name,
	COALESCE(A.totalLead, 0) AS totalLead,
	COALESCE(E.totalAppointmentNew, 0) AS totalAppointmentNew,
	COALESCE(BCD.totalAppointment, 0) AS totalAppointment,
	COALESCE(BCD.totalAppointmentDen, 0) AS totalAppointmentDen,
	COALESCE(BCD.totalAppointmentDongY, 0) AS totalAppointmentDongY,
	COALESCE(P.totalOrderPlan, 0) AS totalOrderPlan,
	COALESCE(P.totalVat, 0) AS totalVat,
	COALESCE(Q.totalRevenue, 0) AS totalRevenue,
	COALESCE(RSTU.totalPayment, 0) AS totalPayment,
	COALESCE(RSTU.totalDeposit, 0) AS totalDeposit,
	COALESCE(RSTU.totalRefundDeposit, 0) AS totalRefundDeposit,
	COALESCE(RSTU.totalRefundServiceUsed, 0) AS totalRefundServiceUsed,
	(COALESCE(P.totalOrderPlan, 0) + COALESCE(P.totalVat, 0) - COALESCE(Q.totalRevenue, 0)) AS duKienThu,
	(CASE WHEN COALESCE(BCD.totalAppointmentDongY, 0) = 0 THEN 0
		ELSE COALESCE(P.totalOrderPlan, 0) / COALESCE(BCD.totalAppointmentDongY, 0) END) AS tbKhach
FROM branch b
LEFT JOIN (
	SELECT branch_id, COUNT(*) AS totalLead
	FROM customer WHERE branch_id IS NOT NULL AND created_day BETWEEN ? AND ?%s
	GROUP BY branch_id
) A ON b.id = A.branch_id
LEFT JOIN (
	SELECT App.branch_id, COUNT(App.id) AS totalAppointmentNew FROM customer
	LEFT JOIN (
		SELECT appointment.id, customer_id, branch_id
		FROM appointment
		INNER JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id
		WHERE branch_id IS NOT NULL AND appointment_type.isTypeRevenue = 1
			AND appointment.created_at BETWEEN ? AND ?%s
		GROUP BY customer_id, appointment.id, branch_id
	) App ON customer.id = App.customer_id
	GROUP BY App.branch_id
) E ON b.id = E.branch_id
LEFT JOIN (
	-- Gop B/C/D: ba chi so doc CUNG dai appointment_date, khac dung dieu kien status /
	-- is_accepted. Do @872.284 lich hen, loc thang mac dinh (2026-09-09), TOAN cau:
	-- 2.670.044 dong InnoDB / 1,26-1,31s -> 701.639 dong / 0,36-0,45s (3,8x dong, 3,2x).
	-- Tang trong gom theo appointment.id nen moi dong la 1 lich hen; MAX() chi de thoa
	-- ONLY_FULL_GROUP_BY. Ban cu dung LEFT JOIN customer khong kem WHERE branch_id IS NOT NULL
	-- nen sinh them nhom branch_id = NULL -- nhom do khong khop b.id nen khong vao ket qua;
	-- ban gop dung INNER JOIN. Doi chieu o muc TOAN CAU tren 5 bo loc: khop tuyet doi.
	SELECT App.branch_id,
		COUNT(App.id) AS totalAppointment,
		COUNT(CASE WHEN App.den THEN App.id END) AS totalAppointmentDen,
		COUNT(CASE WHEN App.dongY THEN App.id END) AS totalAppointmentDongY
	FROM customer
	INNER JOIN (
		SELECT appointment.id, customer_id, branch_id,
			MAX(appointment.status IN (2, 3)) AS den,
			MAX(appointment.status IN (2, 3) AND appointment.is_accepted = 1) AS dongY
		FROM appointment
		INNER JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id
		WHERE branch_id IS NOT NULL AND appointment_type.isTypeRevenue = 1
			AND appointment_date BETWEEN ? AND ?%s
		GROUP BY customer_id, appointment.id, branch_id
	) App ON customer.id = App.customer_id
	GROUP BY App.branch_id
) BCD ON b.id = BCD.branch_id
LEFT JOIN (
	SELECT osu.branch_id, SUM(osu.totalEnd) AS totalOrderPlan, SUM(osu.vat_amount) AS totalVat
	FROM order_service_used osu
	WHERE osu.branch_id IS NOT NULL AND osu.created_day BETWEEN ? AND ?%s
	GROUP BY osu.branch_id
) P ON b.id = P.branch_id
LEFT JOIN (
	SELECT p.branch_id, SUM(p.amount) AS totalRevenue
	FROM payment p
	INNER JOIN order_service_used osu ON osu.id = p.serviceUsed_id
	WHERE p.branch_id IS NOT NULL
		AND p.payment_for IN (%d, %d)
		AND osu.created_day BETWEEN ? AND ?
		AND p.payment_of_date BETWEEN ? AND ?%s
	GROUP BY p.branch_id
) Q ON b.id = Q.branch_id
LEFT JOIN (
	-- Gop R/S/T/U: bon chi so doc CUNG bang payment, CUNG dai payment_time, khac dung
	-- payment_for / deposit_type / cot duoc SUM. Mot luot quet thay cho bon.
	SELECT p.branch_id,
		SUM(CASE WHEN p.payment_for IN (%d, %d) AND (p.deposit_type IS NULL OR p.deposit_type NOT IN (%d)) THEN p.amount ELSE 0 END) AS totalPayment,
		SUM(CASE WHEN p.payment_for = %d AND (p.deposit_type IS NULL OR p.deposit_type NOT IN (%d)) THEN p.deposit_after ELSE 0 END) AS totalDeposit,
		SUM(CASE WHEN p.payment_for = %d THEN p.amount ELSE 0 END) AS totalRefundDeposit,
		SUM(CASE WHEN p.payment_for = %d THEN p.amount ELSE 0 END) AS totalRefundServiceUsed
	FROM payment p
	WHERE p.branch_id IS NOT NULL AND p.payment_time BETWEEN ? AND ?%s
	GROUP BY p.branch_id
) RSTU ON b.id = RSTU.branch_id
WHERE COALESCE(A.totalLead,0) != 0 OR COALESCE(E.totalAppointmentNew,0) != 0
	OR COALESCE(BCD.totalAppointment,0) != 0 OR COALESCE(BCD.totalAppointmentDen,0) != 0
	OR COALESCE(BCD.totalAppointmentDongY,0) != 0
	OR COALESCE(P.totalOrderPlan,0) != 0 OR COALESCE(Q.totalRevenue,0) != 0
	OR COALESCE(RSTU.totalPayment,0) != 0`,
		custScope,
		appScope, // E: lịch mới (created_at)
		appScope, // BCD: gộp 3 chỉ số theo appointment_date
		osuScope,
		define.PaymentForOrder, define.PaymentForMoveDeposit, payScope,
		// RSTU: gộp 4 chỉ số payment cùng dải payment_time
		define.PaymentForOrder, define.PaymentForDeposit, define.PaymentTypeDepositKG,
		define.PaymentForDeposit, define.PaymentTypeDepositKG,
		define.PaymentForRefundDeposit,
		define.PaymentForRefundService, payScope,
	)

	args := []interface{}{}
	// A: lead
	args = append(args, from, to)
	args = append(args, custScopeArgs...)
	// E: lịch mới (created_at)
	args = append(args, from, to)
	args = append(args, appScopeArgs...)
	// BCD: gộp 3 chỉ số theo appointment_date — MỘT lượt quét thay cho ba
	args = append(args, from, to)
	args = append(args, appScopeArgs...)
	// P: doanh số osu
	args = append(args, from, to)
	args = append(args, osuScopeArgs...)
	// Q: doanh thu (payment × osu)
	args = append(args, from, to, from, to)
	args = append(args, payScopeArgs...)
	// RSTU: gộp 4 chỉ số payment — MỘT lượt quét thay cho bốn
	args = append(args, from, to)
	args = append(args, payScopeArgs...)

	rows := make([]map[string]interface{}, 0)
	tx := h.DB.Raw(sql, args...).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	for _, r := range rows {
		helpers.ParseDecimalFields(r, dimStatDecimalAliases...)
	}

	// Sort
	if sf := c.Query("sort"); sf == "-lead" || sf == "-totalLead" {
		for i := 0; i < len(rows); i++ {
			for j := i + 1; j < len(rows); j++ {
				if helpers.ToInt64(rows[j]["totalLead"]) > helpers.ToInt64(rows[i]["totalLead"]) {
					rows[i], rows[j] = rows[j], rows[i]
				}
			}
		}
	}

	// Totals
	totals := gin.H{
		"totalLead": 0, "totalAppointmentNew": 0, "totalAppointment": 0,
		"totalAppointmentDen": 0, "totalAppointmentDongY": 0,
	}
	var tOrderPlan, tVat, tRevenue, tPayment, tDeposit, tRefundDeposit, tRefundService decimal.Decimal
	for _, r := range rows {
		totals["totalLead"] = totals["totalLead"].(int) + int(helpers.ToInt64(r["totalLead"]))
		totals["totalAppointmentNew"] = totals["totalAppointmentNew"].(int) + int(helpers.ToInt64(r["totalAppointmentNew"]))
		totals["totalAppointment"] = totals["totalAppointment"].(int) + int(helpers.ToInt64(r["totalAppointment"]))
		totals["totalAppointmentDen"] = totals["totalAppointmentDen"].(int) + int(helpers.ToInt64(r["totalAppointmentDen"]))
		totals["totalAppointmentDongY"] = totals["totalAppointmentDongY"].(int) + int(helpers.ToInt64(r["totalAppointmentDongY"]))
		tOrderPlan = tOrderPlan.Add(helpers.ToDecimal(r["totalOrderPlan"]))
		tVat = tVat.Add(helpers.ToDecimal(r["totalVat"]))
		tRevenue = tRevenue.Add(helpers.ToDecimal(r["totalRevenue"]))
		tPayment = tPayment.Add(helpers.ToDecimal(r["totalPayment"]))
		tDeposit = tDeposit.Add(helpers.ToDecimal(r["totalDeposit"]))
		tRefundDeposit = tRefundDeposit.Add(helpers.ToDecimal(r["totalRefundDeposit"]))
		tRefundService = tRefundService.Add(helpers.ToDecimal(r["totalRefundServiceUsed"]))
	}
	totals["totalOrderPlan"] = tOrderPlan
	totals["totalVat"] = tVat
	totals["totalRevenue"] = tRevenue
	totals["totalPayment"] = tPayment
	totals["totalDeposit"] = tDeposit
	totals["totalRefundDeposit"] = tRefundDeposit
	totals["totalRefundServiceUsed"] = tRefundService

	tLead := totals["totalLead"].(int)
	tApp := totals["totalAppointment"].(int)
	tDen := totals["totalAppointmentDen"].(int)
	tDongY := totals["totalAppointmentDongY"].(int)
	tAppNew := totals["totalAppointmentNew"].(int)
	if tLead > 0 {
		totals["appointmentNewLead"] = decimal.NewFromInt(int64(tAppNew)).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(int64(tLead)))
	} else {
		totals["appointmentNewLead"] = decimal.Zero
	}
	if tApp > 0 {
		totals["denAppointment"] = decimal.NewFromInt(int64(tDen)).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(int64(tApp)))
	} else {
		totals["denAppointment"] = decimal.Zero
	}
	if tDen > 0 {
		totals["dongYDen"] = decimal.NewFromInt(int64(tDongY)).Mul(decimal.NewFromInt(100)).Div(decimal.NewFromInt(int64(tDen)))
	} else {
		totals["dongYDen"] = decimal.Zero
	}
	if tDongY > 0 {
		totals["tbKhach"] = tOrderPlan.Div(decimal.NewFromInt(int64(tDongY)))
	} else {
		totals["tbKhach"] = decimal.Zero
	}
	// Dự kiến thu = phải thu (doanh số TRƯỚC thuế + VAT, bmad/order-vat) − đã thu (CÓ thuế) của cùng tập dịch vụ.
	totals["duKienThu"] = tOrderPlan.Add(tVat).Sub(tRevenue)

	// Pagination (in-memory)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per-page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 20
	}
	total := len(rows)
	pageCount := int(math.Ceil(float64(total) / float64(perPage)))
	offset := (page - 1) * perPage
	end := offset + perPage
	if end > total {
		end = total
	}
	var items []map[string]interface{}
	if offset < total {
		items = rows[offset:end]
	} else {
		items = make([]map[string]interface{}, 0)
	}

	c.Header("X-Pagination-Total-Count", strconv.Itoa(total))
	c.Header("X-Pagination-Page-Count", strconv.Itoa(pageCount))
	c.Header("X-Pagination-Current-Page", strconv.Itoa(page))
	c.Header("X-Pagination-Per-Page", strconv.Itoa(perPage))

	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"_meta":  gin.H{"totalCount": total, "pageCount": pageCount, "currentPage": page, "perPage": perPage},
		"totals": totals,
	})
}

// SourceStatistics GET /crm/report/source-statistics
func (h *Handler) SourceStatistics(c *gin.Context) {
	h.dimensionStatistics(c, "source", "m.id, m.name", "source_id",
		"customer.source_id", "appointment.source_id", true)
}

// GroupSourceStatistics GET /crm/report/group-source-statistics
// Giống source-statistics nhưng gom nguồn theo group_source.sourceId (JSON array [1,3,5]).
// Dùng JSON_TABLE để flatten sourceId thành rows, join bằng equality thay vì JSON_CONTAINS.
func (h *Handler) GroupSourceStatistics(c *gin.Context) {
	// Bắt buộc có dải ngày và trần 1 năm — cùng luật với 8 endpoint `*-statistics` anh em
	// (xem `dimensionStatistics`). Endpoint này KHÔNG dùng hàm chung đó nên guard phải đặt
	// riêng ở đây. `getDateRange` tự đặt `from=0 / to=9999999999` khi thiếu ngày, mà nút xoá
	// ngày trên FE đi thẳng vào đó. Đo @872.284 lịch hẹn / 615.992 khách / 372.504 payment,
	// toàn chi nhánh, trung vị 5 lượt (2026-09-09), KỂ CẢ SAU bản vá gộp: ca không-lọc-ngày
	// **4,18s**, ca 1 năm **3,57s** — gộp không cứu nổi vì phần lớn thời gian là quét trọn
	// `customer` (615.992 dòng: dải rộng thì optimizer bỏ `idx_customer_created_day_branch`).
	// Vượt trần ⇒ 422 tường minh, KHÔNG tự cắt dải ngày.
	fromSec, errFrom := strconv.ParseInt(c.Query("filter[created_at][gte]"), 10, 64)
	toSec, errTo := strconv.ParseInt(c.Query("filter[created_at][lte]"), 10, 64)
	if errFrom != nil || errTo != nil || toSec < fromSec {
		response.Validate(c, i18n.T(c, "report_date_range_required"))
		return
	}
	if toSec-fromSec > define.ReportMaxRangeSec {
		response.Validate(c, i18n.T(c, "report_date_range_max_one_year"))
		return
	}

	from, to := getDateRange(c)
	custScope, custScopeArgs := buildScopeFilter(c, "customer")
	appScope, appScopeArgs := buildScopeFilter(c, "appointment")
	osuScope, osuScopeArgs := buildScopeFilter(c, "osu")
	payScope, payScopeArgs := buildScopeFilter(c, "p")
	// Story-38: group_source.companyId (JSON) không còn được lọc — 1 thương hiệu/DB.
	gsScope, gsScopeArgs := "", []interface{}(nil)

	sql := fmt.Sprintf(`
SELECT gs.id, gs.name,
    COALESCE(SUM(A.totalLead), 0) AS totalLead,
    COALESCE(SUM(E.totalAppointmentNew), 0) AS totalAppointmentNew,
    COALESCE(SUM(BCD.totalAppointment), 0) AS totalAppointment,
    COALESCE(SUM(BCD.totalAppointmentDen), 0) AS totalAppointmentDen,
    COALESCE(SUM(BCD.totalAppointmentDongY), 0) AS totalAppointmentDongY,
    COALESCE(SUM(P.totalOrderPlan), 0) AS totalOrderPlan,
    COALESCE(SUM(P.totalVat), 0) AS totalVat,
    COALESCE(SUM(Q.totalRevenue), 0) AS totalRevenue,
    COALESCE(SUM(R.totalPayment), 0) AS totalPayment,
    COALESCE(SUM(S.totalDeposit), 0) AS totalDeposit,
    COALESCE(SUM(T.totalRefundDeposit), 0) AS totalRefundDeposit,
    COALESCE(SUM(U.totalRefundServiceUsed), 0) AS totalRefundServiceUsed,
    (COALESCE(SUM(P.totalOrderPlan), 0) + COALESCE(SUM(P.totalVat), 0) - COALESCE(SUM(Q.totalRevenue), 0)) AS duKienThu,
    (CASE WHEN COALESCE(SUM(BCD.totalAppointmentDongY), 0) = 0 THEN 0
        ELSE COALESCE(SUM(P.totalOrderPlan), 0) / COALESCE(SUM(BCD.totalAppointmentDongY), 0) END) AS tbKhach
FROM (
    SELECT gs.id, gs.name, jt.source_id
    FROM group_source gs
    CROSS JOIN JSON_TABLE(gs.sourceId, '$[*]' COLUMNS(source_id INT PATH '$')) AS jt
    WHERE gs.sourceId IS NOT NULL%s
) gs
LEFT JOIN (
    SELECT source_id, COUNT(*) AS totalLead
    FROM customer
    WHERE source_id IS NOT NULL AND created_day BETWEEN ? AND ?%s
    GROUP BY source_id
) A ON gs.source_id = A.source_id
LEFT JOIN (
    -- Truoc 2026-09-09 khoi nay boc them mot lop FROM customer LEFT JOIN (...) App, trong do
    -- customer KHONG dong gop cot nao va menh de WHERE App.source_id IS NOT NULL bien
    -- LEFT JOIN thanh INNER ⇒ chi de keo 615.992 dong khach vao cho vui. Bo duoc VI
    -- appointment.customer_id co FOREIGN KEY that toi customer (RESTRICT): id khac NULL thi
    -- CHAC CHAN co khach tuong ung. Cot lai nullable nen phai giu customer_id IS NOT NULL —
    -- thieu ve nay la lich hen khong gan khach se duoc dem them (hom nay 0 dong, nhung do la
    -- du lieu chu khong phai rang buoc). GROUP BY ... appointment.id cu cung thua: id la khoa chinh.
    SELECT appointment.source_id, COUNT(*) AS totalAppointmentNew
    FROM appointment
    INNER JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id
    WHERE appointment.source_id IS NOT NULL AND appointment_type.isTypeRevenue = 1
        AND appointment.customer_id IS NOT NULL
        AND appointment.created_at BETWEEN ? AND ?%s
    GROUP BY appointment.source_id
) E ON gs.source_id = E.source_id
LEFT JOIN (
    -- Gop B+C+D: ba chi so doc CUNG bang appointment, CUNG dai appointment_date, khac dung
    -- dieu kien status/is_accepted. Mot luot quet thay cho ba. Do @872.284 lich hen, toan
    -- chi nhanh, trung vi 5 luot (2026-09-09): 1 nam 0,811s -> 0,196s (4,1x); thang mac dinh
    -- 0,177s -> 0,014s (13x).
    --
    -- ⚠ Cung ky thuat nay da bi LOAI cho cum P/Q/R/S/T/U ben duoi — xem comment o do.
    SELECT appointment.source_id,
        COUNT(*) AS totalAppointment,
        COUNT(CASE WHEN appointment.status IN (2, 3) THEN 1 END) AS totalAppointmentDen,
        COUNT(CASE WHEN appointment.status IN (2, 3) AND appointment.is_accepted = 1 THEN 1 END) AS totalAppointmentDongY
    FROM appointment
    INNER JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id
    WHERE appointment.source_id IS NOT NULL AND appointment_type.isTypeRevenue = 1
        AND appointment.customer_id IS NOT NULL
        AND appointment_date BETWEEN ? AND ?%s
    GROUP BY appointment.source_id
) BCD ON gs.source_id = BCD.source_id
LEFT JOIN (
    -- ⚠ CUM P/Q/R/S/T/U CO Y KHONG GOP — da thu va LOAI, co so do.
    -- Gop R+U (payment x order_service_used) va S+T (payment x customer) thanh 2 khoi
    -- SUM(CASE WHEN ...) do @372.504 payment, toan chi nhanh, trung vi 5 luot (2026-09-09):
    -- 1 nam **0,794s -> 1,908s, CHAM 2,4x**; so dong doc 375.681 -> 745.008.
    -- Ly do: moi khoi co bo loc payment_for rat chon loc; gop lai la BO bo loc do di, nen
    -- lan quet duy nhat phai xu ly HOP cua ca bon tap roi con JOIN order_service_used cho
    -- tung dong. Gop N luot quet chi thang khi cong viec MOI DONG re (tra khoa chinh);
    -- o day moi dong keo theo mot JOIN dat nen gop thanh lo.
    -- Doi chieu: cung ky thuat nay THANG that trong dimensionStatistics (ca 4 khoi o do chi
    -- tra PK customer): 1 nam 0,055s -> 0,020s. Khac nhau la o CHI PHI MOI DONG, khong phai o
    -- so luot quet — dung suy tu cai nay sang cai kia.
    SELECT osu.source_id, SUM(osu.totalEnd) AS totalOrderPlan, SUM(osu.vat_amount) AS totalVat
    FROM order_service_used osu
    WHERE osu.source_id IS NOT NULL AND osu.created_day BETWEEN ? AND ?%s
    GROUP BY osu.source_id
) P ON gs.source_id = P.source_id
LEFT JOIN (
    SELECT osu.source_id, SUM(p.amount) AS totalRevenue
    FROM payment p
    INNER JOIN order_service_used osu ON osu.id = p.serviceUsed_id
    WHERE osu.source_id IS NOT NULL
        AND p.payment_for IN (%d, %d)
        AND osu.created_day BETWEEN ? AND ?
        AND p.payment_of_date BETWEEN ? AND ?%s
    GROUP BY osu.source_id
) Q ON gs.source_id = Q.source_id
LEFT JOIN (
    SELECT osu.source_id, SUM(p.amount) AS totalPayment
    FROM payment p
    INNER JOIN order_service_used osu ON osu.id = p.serviceUsed_id
    WHERE osu.source_id IS NOT NULL
        AND p.payment_for IN (%d, %d)
        AND (p.deposit_type IS NULL OR p.deposit_type NOT IN (%d))
        AND p.payment_time BETWEEN ? AND ?%s
    GROUP BY osu.source_id
) R ON gs.source_id = R.source_id
LEFT JOIN (
    SELECT customer.source_id, SUM(p.deposit_after) AS totalDeposit
    FROM payment p
    LEFT JOIN customer ON customer.id = p.customer_id
    WHERE customer.source_id IS NOT NULL
        AND p.payment_for = %d
        AND (p.deposit_type IS NULL OR p.deposit_type NOT IN (%d))
        AND p.payment_time BETWEEN ? AND ?%s
    GROUP BY customer.source_id
) S ON gs.source_id = S.source_id
LEFT JOIN (
    SELECT customer.source_id, SUM(p.amount) AS totalRefundDeposit
    FROM payment p
    LEFT JOIN customer ON customer.id = p.customer_id
    WHERE customer.source_id IS NOT NULL
        AND p.payment_for = %d
        AND p.payment_time BETWEEN ? AND ?%s
    GROUP BY customer.source_id
) T ON gs.source_id = T.source_id
LEFT JOIN (
    SELECT osu.source_id, SUM(p.amount) AS totalRefundServiceUsed
    FROM payment p
    INNER JOIN order_service_used osu ON osu.id = p.serviceUsed_id
    WHERE osu.source_id IS NOT NULL
        AND p.payment_for = %d
        AND p.payment_time BETWEEN ? AND ?%s
    GROUP BY osu.source_id
) U ON gs.source_id = U.source_id
GROUP BY gs.id
HAVING (COALESCE(SUM(A.totalLead),0) != 0 OR COALESCE(SUM(E.totalAppointmentNew),0) != 0
    OR COALESCE(SUM(BCD.totalAppointment),0) != 0 OR COALESCE(SUM(BCD.totalAppointmentDen),0) != 0
    OR COALESCE(SUM(BCD.totalAppointmentDongY),0) != 0
    OR COALESCE(SUM(P.totalOrderPlan),0) != 0 OR COALESCE(SUM(Q.totalRevenue),0) != 0
    OR COALESCE(SUM(R.totalPayment),0) != 0)`,
		gsScope,
		custScope,
		appScope, // E: lịch mới (created_at)
		appScope, // BCD: gộp 3 chỉ số (appointment_date)
		osuScope,
		define.PaymentForOrder, define.PaymentForMoveDeposit, payScope,
		define.PaymentForOrder, define.PaymentForDeposit, define.PaymentTypeDepositKG, payScope,
		define.PaymentForDeposit, define.PaymentTypeDepositKG, payScope,
		define.PaymentForRefundDeposit, payScope,
		define.PaymentForRefundService, payScope,
	)

	args := []interface{}{}
	args = append(args, gsScopeArgs...)
	args = append(args, from, to)
	args = append(args, custScopeArgs...)
	// 2 khối appointment: E (created_at) và BCD (appointment_date, gộp 3 chỉ số)
	for i := 0; i < 2; i++ {
		args = append(args, from, to)
		args = append(args, appScopeArgs...)
	}
	args = append(args, from, to)
	args = append(args, osuScopeArgs...)
	args = append(args, from, to, from, to)
	args = append(args, payScopeArgs...)
	args = append(args, from, to)
	args = append(args, payScopeArgs...)
	args = append(args, from, to)
	args = append(args, payScopeArgs...)
	args = append(args, from, to)
	args = append(args, payScopeArgs...)
	args = append(args, from, to)
	args = append(args, payScopeArgs...)

	rows := make([]map[string]interface{}, 0)
	tx := h.DB.Raw(sql, args...).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	paginateAndRespond(c, rows)
}

// ProvinceStatistics GET /crm/report/province-statistics
func (h *Handler) ProvinceStatistics(c *gin.Context) {
	h.dimensionStatistics(c, "location_province", "m.id, m.name", "province_id",
		"customer.province_id", "customer.province_id", false)
}

// CountryStatistics GET /crm/report/country-statistics
func (h *Handler) CountryStatistics(c *gin.Context) {
	h.dimensionStatistics(c, "location_country", "m.id, m.CommonName as name, m.CountryCode as code", "country_id",
		"customer.country_id", "customer.country_id", false)
}

// CategoryStatistics GET /crm/report/category-statistics
func (h *Handler) CategoryStatistics(c *gin.Context) {
	h.dimensionStatistics(c, "customer_category", "m.id, m.name", "category",
		"customer.category", "customer.category", false)
}

// InterestedStatistics GET /crm/report/interested-statistics
func (h *Handler) InterestedStatistics(c *gin.Context) {
	h.dimensionStatistics(c, "service_type", "m.id, m.name", "type_service_id",
		"customer.type_service_id", "customer.type_service_id", false)
}

// Revenue GET /crm/report/revenue
// Per-user revenue report: appointments, orders, payments, deposits
func (h *Handler) Revenue(c *gin.Context) {
	// Bắt buộc có dải ngày và trần 1 năm — cùng luật với `AppointmentStatistics` và
	// `ReportServiceUsed`. `getDateRange` (dùng chung nhiều handler) tự đặt
	// `from=0 / to=9999999999` khi thiếu ngày, mà nút xoá ngày trên bộ lọc FE
	// (`clearStartTime`) đi thẳng vào ca đó bằng ĐÚNG một cú bấm. Khi ấy cả 4 nhánh còn lại
	// đều quét cạn bảng của mình — đo @872.284 lịch hẹn / 596.680 `order_plan` /
	// 372.504 payment, toàn bộ chi nhánh (2026-09-09): **1.218.228 dòng InnoDB + 743.786 dòng
	// bảng tạm**, 2,5–4,0s (cache lạnh 32,2s). Riêng nhánh J (otp × JSON_TABLE × osu) đã là
	// 701.068 dòng, nhánh K quét TRỌN bảng payment 372.504 dòng.
	// Vượt trần ⇒ 422 tường minh, KHÔNG tự cắt dải ngày (cắt = trả thiếu số liệu im lặng).
	fromSec, errFrom := strconv.ParseInt(c.Query("filter[created_at][gte]"), 10, 64)
	toSec, errTo := strconv.ParseInt(c.Query("filter[created_at][lte]"), 10, 64)
	if errFrom != nil || errTo != nil || toSec < fromSec {
		response.Validate(c, i18n.T(c, "report_date_range_required"))
		return
	}
	if toSec-fromSec > define.ReportMaxRangeSec {
		response.Validate(c, i18n.T(c, "report_date_range_max_one_year"))
		return
	}

	from, to := getDateRange(c)

	// Scope filters for each table
	appScope, appScopeArgs := buildScopeFilter(c, "appointment")
	otpScope, otpScopeArgs := buildScopeFilter(c, "otp")
	pmScope, pmScopeArgs := buildScopeFilter(c, "payment")

	// 3 chỉ số lịch hẹn (tổng / đã đến / đã đồng ý) gom vào MỘT lượt quét `appointment`.
	//
	// ⚠ Bản vá hiệu năng có số đo, không phải dọn dẹp phong cách. Trước đây là 3 bảng dẫn xuất
	// F/G/H đọc CÙNG một dải ngày, chỉ khác điều kiện `status` / `is_accepted` ⇒ 3 lượt quét +
	// 3 bảng tạm. Gộp bằng `MAX(<điều kiện>)` ở tầng trong (mỗi dòng đã là 1 khách của 1
	// revenue_id sau `GROUP BY revenue_id, customer_id`) rồi `COUNT(CASE WHEN …)` ở tầng ngoài.
	// Đo @872.284 lịch hẹn, toàn bộ chi nhánh (2026-09-09):
	//   · riêng cụm 3 chỉ số: 140.920 → 57.384 dòng (0,93–9,96s → 0,34–0,58s)
	//   · TOÀN câu, lọc tháng mặc định: 199.450 → **38.775 dòng**, 0,41–0,44s → **0,15–0,22s**
	//   · TOÀN câu, không lọc ngày (nay đã bị guard chặn): 1.218.228 → 1.134.692 dòng
	// Đối chiếu 6 bộ lọc: chuỗi ghép toàn bộ dòng × 4 cột KHỚP tuyệt đối với bản 3 nhánh.
	//
	// ⚠ `LEFT JOIN customer … WHERE App.revenue_id IS NOT NULL` của bản cũ CHÍNH LÀ inner join
	// (vế WHERE trên bảng phải huỷ tính LEFT) — nên bản gộp viết thẳng `INNER JOIN customer`,
	// giữ nguyên ngữ nghĩa "chỉ đếm lịch hẹn mà khách còn tồn tại". Đừng bỏ join này: bỏ đi là
	// đếm cả `customer_id` mồ côi.
	sql := fmt.Sprintf(`SELECT u.id, up.fullname,
		COALESCE(A.totalAppointment, 0) as totalAppointment,
		COALESCE(A.totalAppointmentDen, 0) as totalAppointmentDen,
		COALESCE(A.totalAppointmentDongY, 0) as totalAppointmentDongY,
		COALESCE(J.totalOrderPlan, 0) as totalOrderPlan,
		COALESCE(K.totalThanhToan, 0) as totalThanhToan,
		COALESCE(L.totalDepositMoney, 0) as totalDepositMoney
	FROM user u
	LEFT JOIN user_profile up ON up.user_id = u.id
	LEFT JOIN (
		SELECT App.revenue_id,
			COUNT(App.customer_id) as totalAppointment,
			COUNT(CASE WHEN App.den THEN App.customer_id END) as totalAppointmentDen,
			COUNT(CASE WHEN App.dongY THEN App.customer_id END) as totalAppointmentDongY
		FROM customer
		INNER JOIN (
			SELECT customer_id, revenue_id,
				MAX(appointment.status IN (2, 3)) as den,
				MAX(appointment.status IN (2, 3) AND appointment.is_accepted = 1) as dongY
			FROM appointment
			INNER JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id
			WHERE appointment_type.isTypeRevenue = 1 AND appointment_date BETWEEN ? AND ?
				AND appointment.revenue_id IS NOT NULL%s
			GROUP BY revenue_id, customer_id
		) App ON App.customer_id = customer.id
		GROUP BY App.revenue_id
	) A ON u.id = A.revenue_id
	LEFT JOIN (
		SELECT jt.revenue_id, COALESCE(SUM(osu.totalEnd), 0) as totalOrderPlan
		FROM order_plan otp
		JOIN JSON_TABLE(otp.listRevenues, '$[*]' COLUMNS(revenue_id INT PATH '$')) jt
		LEFT JOIN order_service_used osu ON osu.order_plan_id = otp.id
		WHERE otp.listRevenues IS NOT NULL AND otp.created_time BETWEEN ? AND ?%s
		GROUP BY jt.revenue_id
	) J ON u.id = J.revenue_id
	LEFT JOIN (
		SELECT pm.revenue_id, SUM(totalThanhToan) as totalThanhToan FROM (
			SELECT jt.revenue_id, payment.customer_id, SUM(payment.amount) as totalThanhToan
			FROM payment
			INNER JOIN order_service_used osu ON osu.id = payment.serviceUsed_id
			JOIN JSON_TABLE(osu.listRevenues, '$[*]' COLUMNS(revenue_id INT PATH '$')) jt
			WHERE payment.payment_of_date BETWEEN ? AND ? AND payment.payment_for IN (%d, %d)%s
				AND osu.listRevenues IS NOT NULL
			GROUP BY jt.revenue_id, payment.customer_id
		) pm GROUP BY pm.revenue_id
	) K ON u.id = K.revenue_id
	LEFT JOIN (
		SELECT payment_user_id, SUM(totalDepositMoney) as totalDepositMoney FROM (
			SELECT customer_id, SUM(deposit_after) as totalDepositMoney
			FROM payment
			WHERE payment_time BETWEEN ? AND ? AND payment_for = %d AND deposit_type != %d%s
			GROUP BY customer_id
		) dep
		LEFT JOIN customer ON customer.id = dep.customer_id
		GROUP BY customer.payment_user_id
	) L ON u.id = L.payment_user_id
	WHERE COALESCE(A.totalAppointment,0) != 0 OR COALESCE(A.totalAppointmentDen,0) != 0
		OR COALESCE(A.totalAppointmentDongY,0) != 0 OR COALESCE(J.totalOrderPlan,0) != 0
		OR COALESCE(K.totalThanhToan,0) != 0 OR COALESCE(L.totalDepositMoney,0) != 0`,
		appScope,
		otpScope,
		define.PaymentForOrder, define.PaymentForMoveDeposit, pmScope,
		define.PaymentForDeposit, define.PaymentTypeDepositKG, pmScope)

	args := []interface{}{}
	// A: 3 chỉ số lịch hẹn — MỘT lượt quét thay cho F/G/H trước đây
	args = append(args, from, to)
	args = append(args, appScopeArgs...)
	// J: orderPlan
	args = append(args, from, to)
	args = append(args, otpScopeArgs...)
	// K: payment
	args = append(args, from, to)
	args = append(args, pmScopeArgs...)
	// L: deposit
	args = append(args, from, to)
	args = append(args, pmScopeArgs...)
	rows := make([]map[string]interface{}, 0)
	tx := h.DB.Raw(sql, args...).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	// Alias của SUM(decimal) → []uint8 → JSON ra string; alias không thuộc bảng nào
	// nên phải liệt kê tường minh.
	for _, r := range rows {
		helpers.ParseDecimalFields(r, "totalOrderPlan", "totalThanhToan", "totalDepositMoney")
	}

	// Sort
	sortField := c.Query("sort")
	if sortField != "" {
		desc := false
		if strings.HasPrefix(sortField, "-") {
			desc = true
			sortField = sortField[1:]
		}
		for i := 0; i < len(rows); i++ {
			for j := i + 1; j < len(rows); j++ {
				// So bằng decimal: cột tiền có phần lẻ — ToInt64 cắt phần lẻ làm thứ tự sai.
				vi := helpers.ToDecimal(rows[i][sortField])
				vj := helpers.ToDecimal(rows[j][sortField])
				if (desc && vj.GreaterThan(vi)) || (!desc && vj.LessThan(vi)) {
					rows[i], rows[j] = rows[j], rows[i]
				}
			}
		}
	}

	// Totals
	var tTotalOrderPlan, tTotalThanhToan, tTotalDepositMoney decimal.Decimal
	totals := gin.H{
		"totalAppointment": 0, "totalAppointmentDen": 0, "totalAppointmentDongY": 0,
		"totalOrderPlan": 0, "totalThanhToan": 0, "totalDepositMoney": 0,
	}
	for _, r := range rows {
		totals["totalAppointment"] = totals["totalAppointment"].(int) + int(helpers.ToInt64(r["totalAppointment"]))
		totals["totalAppointmentDen"] = totals["totalAppointmentDen"].(int) + int(helpers.ToInt64(r["totalAppointmentDen"]))
		totals["totalAppointmentDongY"] = totals["totalAppointmentDongY"].(int) + int(helpers.ToInt64(r["totalAppointmentDongY"]))
		tTotalOrderPlan = tTotalOrderPlan.Add(helpers.ToDecimal(r["totalOrderPlan"]))
		tTotalThanhToan = tTotalThanhToan.Add(helpers.ToDecimal(r["totalThanhToan"]))
		tTotalDepositMoney = tTotalDepositMoney.Add(helpers.ToDecimal(r["totalDepositMoney"]))
	}
	totals["totalOrderPlan"] = tTotalOrderPlan
	totals["totalThanhToan"] = tTotalThanhToan
	totals["totalDepositMoney"] = tTotalDepositMoney
	// Percentages
	pct := func(a, b int) float64 {
		if b == 0 {
			return 0
		}
		return float64(a) * 100 / float64(b)
	}
	tApp := totals["totalAppointment"].(int)
	tDen := totals["totalAppointmentDen"].(int)
	tDongY := totals["totalAppointmentDongY"].(int)
	totals["denLichHen"] = pct(tDen, tApp)
	totals["dongYDen"] = pct(tDongY, tDen)
	if tDongY > 0 {
		totals["tbKhach"] = tTotalOrderPlan.Div(decimal.NewFromInt(int64(tDongY)))
	} else {
		totals["tbKhach"] = decimal.Zero
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per-page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 20
	}

	total := len(rows)
	pageCount := int(math.Ceil(float64(total) / float64(perPage)))
	offset := (page - 1) * perPage
	end := offset + perPage
	if end > total {
		end = total
	}

	var items []map[string]interface{}
	if offset < total {
		items = rows[offset:end]
	} else {
		items = make([]map[string]interface{}, 0)
	}

	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"_meta":  gin.H{"totalCount": total, "pageCount": pageCount, "currentPage": page, "perPage": perPage},
		"totals": totals,
	})
}

// TelesalesOnline GET /crm/report/telesales-online
// Per-user telesales report: interactions, leads, appointments, orders, payments, deposits
func (h *Handler) TelesalesOnline(c *gin.Context) {
	// Bắt buộc có dải ngày và trần 1 năm — cùng luật với `AppointmentStatistics`, `Revenue`,
	// `ReportServiceUsed`. `getDateRange` (dùng chung nhiều handler) tự đặt
	// `from=0 / to=9999999999` khi thiếu ngày, và nút xoá ngày trên bộ lọc FE (`clearStartTime`)
	// đi thẳng vào đó. Báo cáo này có 12 bảng dẫn xuất nên thiếu ngày là 12 lượt quét cạn.
	// Vượt trần ⇒ 422 tường minh, KHÔNG tự cắt dải ngày (cắt = trả thiếu số liệu im lặng).
	fromSec, errFrom := strconv.ParseInt(c.Query("filter[created_at][gte]"), 10, 64)
	toSec, errTo := strconv.ParseInt(c.Query("filter[created_at][lte]"), 10, 64)
	if errFrom != nil || errTo != nil || toSec < fromSec {
		response.Validate(c, i18n.T(c, "report_date_range_required"))
		return
	}
	if toSec-fromSec > define.ReportMaxRangeSec {
		response.Validate(c, i18n.T(c, "report_date_range_max_one_year"))
		return
	}

	from, to := getDateRange(c)

	// Note: Yii2 does NOT scope this report by branch/company
	// It's a cross-branch telesales performance report
	appScope := ""
	var appScopeArgs []interface{}
	otpScope := ""
	var otpScopeArgs []interface{}
	pmScope := ""
	var pmScopeArgs []interface{}
	custScope := ""
	var custScopeArgs []interface{}

	// sourceOnline: get source IDs where group_source.online = 1
	// Extract source_id from group_source.sourceId JSON column
	sourceSubquery := "(SELECT DISTINCT jt.source_id FROM group_source gs, JSON_TABLE(gs.sourceId, '$[*]' COLUMNS(source_id INT PATH '$')) jt WHERE gs.online = 1)"

	sql := fmt.Sprintf(`SELECT u.id, up.fullname,
		COALESCE(B.totalLead, 0) as totalLead,
		COALESCE(C.totalChamLai, 0) as totalChamLai,
		COALESCE(DE.totalAppointmentNew, 0) as totalAppointmentNew,
		COALESCE(DE.totalAppointmentNewPlus, 0) as totalAppointmentNewPlus,
		COALESCE(FGH.totalAppointment, 0) as totalAppointment,
		COALESCE(FGH.totalAppointmentDen, 0) as totalAppointmentDen,
		COALESCE(FGH.totalAppointmentDongY, 0) as totalAppointmentDongY,
		COALESCE(I.totalDeposit, 0) as totalDeposit,
		COALESCE(J.totalOrderPlan, 0) as totalOrderPlan,
		COALESCE(K.totalThanhToan, 0) as totalThanhToan,
		COALESCE(L.totalDepositMoney, 0) as totalDepositMoney
	FROM user u
	LEFT JOIN user_profile up ON up.user_id = u.id
	-- B: Lead
	LEFT JOIN (
		SELECT payment_user_id, COUNT(*) as totalLead
		FROM customer WHERE payment_user_id IS NOT NULL AND source_id IN %s AND created_day BETWEEN ? AND ?%s
		GROUP BY payment_user_id
	) B ON u.id = B.payment_user_id
	-- C: Cham lai
	LEFT JOIN (
		SELECT noTe.created_by as paymentUser, COUNT(*) as totalChamLai
		FROM customer
		-- Dieu kien ngay phai nam TRONG bang dan xuat, khong chi o WHERE ngoai: ban cu dung bang
		-- dan xuat tren TOAN BO note roi moi loc, tuc khu trung lap tren 3,37 trieu dong va goi
		-- ROUND(UNIX_TIMESTAMP(FROM_UNIXTIME(...))) cho tung dong. Ve created_at duoi day la bo
		-- loc NOI RONG (+-1 ngay quanh dai) nen KHONG doi ket qua -- ve dayCreated BETWEEN o ngoai
		-- van la dieu kien quyet dinh; no chi de cau sargable va de phep so sanh so nguyen loai
		-- ~99,9%% dong TRUOC khi phai goi chuoi 4 ham long nhau.
		-- Do @3.370.004 note, loc thang mac dinh (2026-09-09): 3.374.702 dong InnoDB / 6,4-6,8s
		-- -> 2.528 dong / 0,015s. Can chi muc idx_note_created_at (migration 20260909132102).
		LEFT JOIN (
			SELECT DISTINCT customer_id, ROUND(UNIX_TIMESTAMP(FROM_UNIXTIME(note.created_at, '%%Y-%%m-%%d')),0) as dayCreated, created_by
			FROM note
			WHERE note.created_at >= ? - 86400 AND note.created_at < ? + 172800
		) noTe ON customer.id = noTe.customer_id
		WHERE noTe.dayCreated != customer.created_day AND noTe.dayCreated BETWEEN ? AND ?
		GROUP BY paymentUser
	) C ON u.id = C.paymentUser
	-- D+E: Lich moi / Lich moi+ -- MOT luot quet thay cho hai.
	-- Hai chi so doc CUNG dai ngay, cung bo loc; E chi them dieu kien "khach cu"
	-- (customer.created_day < from) o tang NGOAI. Gop bang COUNT(CASE WHEN ...).
	LEFT JOIN (
		SELECT App.revenue_id,
			COUNT(App.customer_id) as totalAppointmentNew,
			COUNT(CASE WHEN customer.created_day < ? THEN App.customer_id END) as totalAppointmentNewPlus
		FROM customer
		INNER JOIN (
			SELECT customer_id, revenue_id
			FROM appointment
			INNER JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id
			WHERE revenue_id IS NOT NULL AND appointment_type.isTypeRevenue = 1
				AND appointment.created_at BETWEEN ? AND ? AND appointment.source_id IN %s%s
			GROUP BY revenue_id, customer_id
		) App ON App.customer_id = customer.id
		GROUP BY App.revenue_id
	) DE ON u.id = DE.revenue_id
	-- F+G+H: Lich hen / Khach den / Khach dong y -- MOT luot quet thay cho ba.
	-- Ba chi so doc CUNG dai appointment_date, khac nhau dung dieu kien status / is_accepted.
	-- Gop bang MAX(<dieu kien>) o tang trong (moi dong da la 1 khach cua 1 revenue_id sau
	-- GROUP BY revenue_id, customer_id) roi COUNT(CASE WHEN ...) o tang ngoai.
	-- Do @872.284 lich hen, loc thang mac dinh (2026-09-09):
	-- 2.104.718 dong InnoDB / 1,58-3,92s -> 114.768 dong / 0,36-0,55s (18x so dong).
	-- Ban cu dung LEFT JOIN customer KHONG kem WHERE App.revenue_id IS NOT NULL nen sinh them
	-- mot nhom revenue_id = NULL (khach khong co lich nao). Nhom do KHONG bao gio khop
	-- u.id = F.revenue_id nen khong vao ket qua -- ban gop dung INNER JOIN va bo no di.
	-- Da doi chieu o muc TOAN CAU (khong phai muc nhanh): 9 dong khop byte-for-byte.
	LEFT JOIN (
		SELECT App.revenue_id,
			COUNT(App.customer_id) as totalAppointment,
			COUNT(CASE WHEN App.den THEN App.customer_id END) as totalAppointmentDen,
			COUNT(CASE WHEN App.dongY THEN App.customer_id END) as totalAppointmentDongY
		FROM customer
		INNER JOIN (
			SELECT customer_id, revenue_id,
				MAX(appointment.status IN (2, 3)) as den,
				MAX(appointment.status IN (2, 3) AND appointment.is_accepted = 1) as dongY
			FROM appointment
			INNER JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id
			WHERE revenue_id IS NOT NULL AND appointment_type.isTypeRevenue = 1
				AND appointment_date BETWEEN ? AND ? AND appointment.source_id IN %s%s
			GROUP BY revenue_id, customer_id
		) App ON App.customer_id = customer.id
		GROUP BY App.revenue_id
	) FGH ON u.id = FGH.revenue_id
	-- I: Khach coc
	LEFT JOIN (
		SELECT Pay.revenue_id, COUNT(DISTINCT Pay.customer_id) as totalDeposit FROM (
			SELECT payment.customer_id, c.payment_user_id AS revenue_id FROM payment
			JOIN customer c ON c.id = payment.customer_id
			WHERE payment.payment_for = %d AND payment.payment_time BETWEEN ? AND ? AND payment.deposit_type = %d
				AND c.payment_user_id IS NOT NULL%s
			GROUP BY payment.customer_id, c.payment_user_id
		) Pay
		GROUP BY Pay.revenue_id
	) I ON u.id = I.revenue_id
	-- J: Doanh so
	LEFT JOIN (
		SELECT jt.revenue_id, COALESCE(SUM(osu.totalEnd), 0) as totalOrderPlan
		FROM order_plan otp
		JOIN JSON_TABLE(otp.listRevenues, '$[*]' COLUMNS(revenue_id INT PATH '$')) jt
		LEFT JOIN order_service_used osu ON osu.order_plan_id = otp.id
		WHERE otp.listRevenues IS NOT NULL AND otp.created_time BETWEEN ? AND ? AND otp.source_id IN %s%s
		GROUP BY jt.revenue_id
	) J ON u.id = J.revenue_id
	-- K: Thanh toan
	LEFT JOIN (
		SELECT revenue_id, SUM(totalThanhToan) as totalThanhToan FROM (
			SELECT jt.revenue_id AS revenue_id, payment.customer_id AS customer_id, SUM(payment.amount) as totalThanhToan
			FROM payment
			LEFT JOIN order_service_used ose ON ose.id = payment.serviceUsed_id
			JOIN JSON_TABLE(ose.listRevenues, '$[*]' COLUMNS(revenue_id INT PATH '$')) jt
			WHERE ose.listRevenues IS NOT NULL AND payment.payment_of_date BETWEEN ? AND ?
				AND payment.payment_for IN (%d, %d) AND ose.source_id IN %s%s
			GROUP BY jt.revenue_id, payment.customer_id
		) pm GROUP BY revenue_id
	) K ON u.id = K.revenue_id
	-- L: Tien coc
	LEFT JOIN (
		SELECT dep.revenue_id, SUM(dep.totalDepositMoney) as totalDepositMoney FROM (
			SELECT c.payment_user_id AS revenue_id, payment.customer_id AS customer_id, SUM(payment.deposit_after) as totalDepositMoney
			FROM payment
			JOIN customer c ON c.id = payment.customer_id
			WHERE c.payment_user_id IS NOT NULL AND payment.payment_time BETWEEN ? AND ?
				AND payment.payment_for = %d AND payment.deposit_type != %d AND c.source_id IN %s%s
			GROUP BY c.payment_user_id, payment.customer_id
		) dep GROUP BY dep.revenue_id
	) L ON u.id = L.revenue_id
	WHERE JSON_CONTAINS(up.departmentList, '%d')
		AND (COALESCE(B.totalLead,0) != 0 OR COALESCE(C.totalChamLai,0) != 0
		OR COALESCE(DE.totalAppointmentNew,0) != 0 OR COALESCE(FGH.totalAppointment,0) != 0
		OR COALESCE(J.totalOrderPlan,0) != 0 OR COALESCE(K.totalThanhToan,0) != 0)`,
		// B: sourceOnline, custScope
		sourceSubquery, custScope,
		// DE: sourceOnline, appScope  (gộp D+E — MỘT lượt quét)
		sourceSubquery, appScope,
		// FGH: sourceOnline, appScope (gộp F+G+H — MỘT lượt quét)
		sourceSubquery, appScope,
		// I: FOR_DEPOSIT, TYPE_DEPOSIT_RESERVE, pmScope
		define.PaymentForDeposit, define.PaymentTypeDepositReserve, pmScope,
		// J: sourceOnline, otpScope
		sourceSubquery, otpScope,
		// K: FOR_ORDER, FOR_MOVE_DEPOSIT, sourceOnline, pmScope
		define.PaymentForOrder, define.PaymentForMoveDeposit, sourceSubquery, pmScope,
		// L: FOR_DEPOSIT, TYPE_DEPOSIT_KG, sourceOnline, pmScope
		define.PaymentForDeposit, define.PaymentTypeDepositKG, sourceSubquery, pmScope,
		// WHERE: DEPARTMENT_TELESALE
		define.HrDepartmentTelesale)

	args := []interface{}{}
	// B: customer lead
	args = append(args, from, to)
	args = append(args, custScopeArgs...)
	// C: cham lai — 2 tham số cho vế `created_at` nới rộng TRONG bảng dẫn xuất, rồi 2 cho vế ngoài
	args = append(args, from, to, from, to)
	// DE: appointment new (+ new plus) — `customer.created_day < ?` đứng TRƯỚC dải ngày
	args = append(args, from)
	args = append(args, from, to)
	args = append(args, appScopeArgs...)
	// FGH: appointment / den / dong y
	args = append(args, from, to)
	args = append(args, appScopeArgs...)
	// I: deposit
	args = append(args, from, to)
	args = append(args, pmScopeArgs...)
	// J: order plan
	args = append(args, from, to)
	args = append(args, otpScopeArgs...)
	// K: payment
	args = append(args, from, to)
	args = append(args, pmScopeArgs...)
	// L: deposit money
	args = append(args, from, to)
	args = append(args, pmScopeArgs...)

	rows := make([]map[string]interface{}, 0)
	tx := h.DB.Raw(sql, args...).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	// Alias của SUM(decimal) → []uint8 → JSON ra string; alias không thuộc bảng nào
	// nên phải liệt kê tường minh.
	for _, r := range rows {
		helpers.ParseDecimalFields(r, "totalOrderPlan", "totalThanhToan", "totalDepositMoney")
	}

	// Sort + totals + pagination with percentages
	fields := []string{
		"totalLead", "totalChamLai",
		"totalAppointmentNew", "totalAppointmentNewPlus", "totalAppointment",
		"totalAppointmentDen", "totalAppointmentDongY", "totalDeposit",
		"totalOrderPlan", "totalThanhToan", "totalDepositMoney",
	}
	paginateAndRespondWithPercentages(c, rows, fields, telesalesTotals)
}

// telesalesTotals bổ sung tỉ lệ % và doanh số TB/khách vào dòng tổng của TelesalesOnline.
func telesalesTotals(totals gin.H) {
	// Tổng là decimal.Decimal (xem paginateAndRespondWithPercentages); các chỉ số đếm là số
	// nguyên nên IntPart() không mất gì. Tiền (totalOrderPlan) giữ decimal.
	count := func(k string) int64 { return totals[k].(decimal.Decimal).IntPart() }
	tLead := count("totalLead")
	tChamLai := count("totalChamLai")
	tAppNew := count("totalAppointmentNew")
	tAppNewPlus := count("totalAppointmentNewPlus")
	tApp := count("totalAppointment")
	tDen := count("totalAppointmentDen")
	tDongY := count("totalAppointmentDongY")
	tOrderPlan := totals["totalOrderPlan"].(decimal.Decimal)

	// Tỉ lệ % (không phải tiền) ⇒ float64.
	pct := func(a, b int64) float64 {
		if b == 0 {
			return 0
		}
		return float64(a) * 100 / float64(b)
	}

	totals["lichMoiLead"] = pct(tAppNew, tLead)
	totals["lichMoiMinusLead"] = pct(tAppNew-tAppNewPlus, tLead)
	totals["lichMoiPlusChamLai"] = pct(tAppNewPlus, tChamLai)
	totals["denLead"] = pct(tDen, tLead)
	totals["denLichHen"] = pct(tDen, tApp)
	totals["dongYDen"] = pct(tDongY, tDen)
	// Doanh số trung bình / khách đồng ý — tiền ⇒ decimal (cùng cách tính với Revenue).
	if tDongY > 0 {
		totals["tbKhach"] = tOrderPlan.Div(decimal.NewFromInt(tDongY))
	} else {
		totals["tbKhach"] = decimal.Zero
	}
}

// paginateAndRespondWithPercentages — like paginateAndRespondGeneric but with custom percentage callback
func paginateAndRespondWithPercentages(c *gin.Context, rows []map[string]interface{}, totalFields []string, addPercentages func(gin.H)) {
	// Sort
	sortField := c.Query("sort")
	if sortField != "" {
		desc := false
		if strings.HasPrefix(sortField, "-") {
			desc = true
			sortField = sortField[1:]
		}
		for i := 0; i < len(rows); i++ {
			for j := i + 1; j < len(rows); j++ {
				// So bằng decimal: cột tiền có phần lẻ (.50) — ToInt64 cắt phần lẻ làm thứ tự sai.
				vi := helpers.ToDecimal(rows[i][sortField])
				vj := helpers.ToDecimal(rows[j][sortField])
				if (desc && vj.GreaterThan(vi)) || (!desc && vj.LessThan(vi)) {
					rows[i], rows[j] = rows[j], rows[i]
				}
			}
		}
	}

	// Totals — decimal.Decimal cho MỌI field (M-20): trước đây cộng `int(float64)` cắt phần lẻ
	// của cột tiền ở dòng tổng. Cột đếm vẫn ra số nguyên (decimal JSON không nháy, main.go).
	totals := gin.H{}
	sums := make(map[string]decimal.Decimal, len(totalFields))
	for _, r := range rows {
		for _, f := range totalFields {
			sums[f] = sums[f].Add(helpers.ToDecimal(r[f]))
		}
	}
	for _, f := range totalFields {
		totals[f] = sums[f]
	}

	// Add percentages
	if addPercentages != nil {
		addPercentages(totals)
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per-page", "20"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 20
	}

	total := len(rows)
	pageCount := int(math.Ceil(float64(total) / float64(perPage)))
	offset := (page - 1) * perPage
	end := offset + perPage
	if end > total {
		end = total
	}

	var items []map[string]interface{}
	if offset < total {
		items = rows[offset:end]
	} else {
		items = make([]map[string]interface{}, 0)
	}

	c.JSON(http.StatusOK, gin.H{
		"items":  items,
		"_meta":  gin.H{"totalCount": total, "pageCount": pageCount, "currentPage": page, "perPage": perPage},
		"totals": totals,
	})
}

// BranchAppointment GET /crm/report/branch-appointment
// Lịch hẹn pivot theo chi nhánh: tổng hẹn, khách đến, khách hủy, đồng ý.
func (h *Handler) BranchAppointment(c *gin.Context) {
	from := c.Query("_created_at[gte]")
	to := c.Query("_created_at[lte]")
	if from == "" {
		from = "0"
	}
	if to == "" {
		to = fmt.Sprintf("%d", 9999999999)
	}

	appScope, appScopeArgs := buildScopeFilter(c, "appointment")

	typeFilter := ""
	typeArgs := []interface{}{}
	if vals := query.GetFilterValues(c, "appointment_type_id"); len(vals) > 0 {
		typeFilter = fmt.Sprintf(" AND appointment.appointment_type_id IN (%s)", placeholders(len(vals)))
		for _, v := range vals {
			typeArgs = append(typeArgs, v)
		}
	}

	combined := appScope + typeFilter

	sql := fmt.Sprintf(`
SELECT b.id AS branchId, b.name AS branchName,
    COALESCE(ABCD.totalAppointment, 0) AS totalAppointment,
    COALESCE(ABCD.arrived, 0) AS arrived,
    COALESCE(ABCD.cancelled, 0) AS cancelled,
    COALESCE(ABCD.agreed, 0) AS agreed
FROM branch b
LEFT JOIN (
    -- Gop A+B+C+D: bon chi so doc CUNG bang appointment, CUNG dai appointment_date, CUNG bo loc
    -- phu (scope + appointment_type_id), khac dung dieu kien status/is_accepted. Mot luot quet
    -- thay cho bon. COUNT(DISTINCT appointment.id) cu la thua: id la khoa chinh.
    -- Do @872.284 lich hen, toan chi nhanh, trung vi 5 luot (2026-09-09):
    --   thang mac dinh  0,0291s -> 0,0089s (3,3x) · 73.690 -> 18.423 dong InnoDB
    --   1 nam           4,4604s -> 1,4036s (3,2x)
    --   KHONG loc ngay  4,1286s -> 1,2874s (3,2x) · 2.642.888 -> 872.297 dong
    --   1 ngay          0,0024s -> 0,0020s (da re san)
    -- Sort_rows sap ve 0: 1.614.689 -> 17 (bon lan GROUP BY branch_id gio con mot).
    -- Gop THANG o day vi moi dong chi doc chinh bang appointment, khong keo theo JOIN nao —
    -- nguoc voi cum payment cua GroupSourceStatistics (da thu, LOAI, xem comment ben do).
    SELECT appointment.branch_id,
        COUNT(*) AS totalAppointment,
        COUNT(CASE WHEN appointment.status IN (2, 3) THEN 1 END) AS arrived,
        COUNT(CASE WHEN appointment.status = 4 THEN 1 END) AS cancelled,
        COUNT(CASE WHEN appointment.status IN (2, 3) AND appointment.is_accepted = 1 THEN 1 END) AS agreed
    FROM appointment
    WHERE appointment.branch_id IS NOT NULL
        AND appointment.appointment_date BETWEEN ? AND ?%s
    GROUP BY appointment.branch_id
) ABCD ON b.id = ABCD.branch_id
WHERE COALESCE(ABCD.totalAppointment,0) > 0
    OR COALESCE(ABCD.arrived,0) > 0
    OR COALESCE(ABCD.cancelled,0) > 0
    OR COALESCE(ABCD.agreed,0) > 0
ORDER BY COALESCE(ABCD.totalAppointment,0) DESC`,
		combined)

	subArgs := make([]interface{}, 0, len(appScopeArgs)+len(typeArgs))
	subArgs = append(subArgs, appScopeArgs...)
	subArgs = append(subArgs, typeArgs...)
	// MỘT khối dẫn xuất duy nhất sau khi gộp ⇒ đúng một bộ tham số.
	args := []interface{}{}
	args = append(args, from, to)
	args = append(args, subArgs...)

	rows := make([]map[string]interface{}, 0)
	tx := h.DB.Raw(sql, args...).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	totals := gin.H{
		"totalAppointment": 0,
		"arrived":          0,
		"cancelled":        0,
		"agreed":           0,
	}
	for _, r := range rows {
		totals["totalAppointment"] = totals["totalAppointment"].(int) + int(helpers.ToInt64(r["totalAppointment"]))
		totals["arrived"] = totals["arrived"].(int) + int(helpers.ToInt64(r["arrived"]))
		totals["cancelled"] = totals["cancelled"].(int) + int(helpers.ToInt64(r["cancelled"]))
		totals["agreed"] = totals["agreed"].(int) + int(helpers.ToInt64(r["agreed"]))
	}

	c.JSON(http.StatusOK, gin.H{
		"items":  rows,
		"totals": totals,
	})
}

// EffectiveAdvise GET /crm/report/effective-advise
func (h *Handler) EffectiveAdvise(c *gin.Context) {
	// Bắt buộc có dải ngày và trần 1 năm. Khác 2 report anh em `doctor-advise` /
	// `direct-sale-advise` (đo ra ca xấu nhất 0,3s nên CỐ Ý không chặn), ở đây ca không-lọc-ngày
	// thật sự đau — và đau ở chỗ khó thấy nhất: **cache lạnh**.
	//
	// Đo @872.284 lịch hẹn, toàn bộ chi nhánh (2026-09-09):
	//   · lọc 1 ngày:        755 dòng InnoDB — 0,004–0,009s
	//   · lọc tháng mặc định: 38.096 dòng    — 0,014–0,017s
	//   · KHÔNG lọc ngày:     57.384 dòng    — 0,180–0,187s WARM, nhưng **6,8–14,4s CACHE LẠNH**
	// Vì sao lệch 40x giữa warm và lạnh: bỏ dải ngày thì optimizer rời `idx_appointment_date_scope`
	// và quay về `Index lookup on fk-appointment_appointment_type_id` — chỉ mục đó chỉ có
	// `appointment_type_id` nên KHÔNG covering ⇒ 57.384 lượt fetch row NGẪU NHIÊN. Báo cáo là thứ
	// mở thưa, tức cache lạnh mới là trạng thái BÌNH THƯỜNG của nó.
	// Vượt trần ⇒ 422 tường minh, KHÔNG tự cắt dải ngày (cắt = trả thiếu số liệu im lặng).
	//
	// ⚠ Biến thể ĐÃ ĐO VÀ LOẠI — chỉ mục covering
	// `(appointment_type_id, company_id, status, is_accepted, appointment_date, branch_id)`:
	// đường ĐỌC thắng đậm (không-lọc 0,180s → 0,019–0,020s; tháng 38.096 → 18.460 dòng,
	// 0,014s → 0,006s; gate FORCE INDEX ĐẠT). Nhưng đo đường GHI: `UPDATE` 2.000 dòng
	// 0,068–0,130s → 0,159–0,173s, tức **+30–60% chi phí UPDATE mỗi dòng `appointment`**.
	// `appointment` là bảng ghi liên tục (đặt lịch, check-in, check-out) còn báo cáo này mở thưa
	// ⇒ đánh thuế đường ghi nóng để cứu một màn hiếm là lỗ. Trần ngày đạt cùng mục tiêu với
	// chi phí ghi bằng 0.
	fromSec, errFrom := strconv.ParseInt(c.Query("filter[created_at][gte]"), 10, 64)
	toSec, errTo := strconv.ParseInt(c.Query("filter[created_at][lte]"), 10, 64)
	if errFrom != nil || errTo != nil || toSec < fromSec {
		response.Validate(c, i18n.T(c, "report_date_range_required"))
		return
	}
	if toSec-fromSec > define.ReportMaxRangeSec {
		response.Validate(c, i18n.T(c, "report_date_range_max_one_year"))
		return
	}

	from, to := getDateRange(c)
	appScope, appScopeArgs := buildScopeFilter(c, "appointment")

	sql := fmt.Sprintf(`SELECT aia.id, aia.name, COALESCE(A.isAccepted, 0) as isAccepted
	FROM appointment_is_accepted aia
	LEFT JOIN (
		SELECT is_accepted, COUNT(is_accepted) as isAccepted
		FROM appointment
		INNER JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id
		WHERE is_accepted IS NOT NULL AND appointment_type.isTypeRevenue = 1
			AND appointment_date BETWEEN ? AND ?
			AND appointment.status IN (2, 3)%s
		GROUP BY is_accepted
	) A ON aia.id = A.is_accepted`, appScope)

	args := []interface{}{from, to}
	args = append(args, appScopeArgs...)

	rows := make([]map[string]interface{}, 0)
	tx := h.DB.Raw(sql, args...).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	paginateAndRespondWithPercentages(c, rows,
		[]string{"isAccepted"}, func(totals gin.H) {
			totals["totalIsAccepted"] = totals["isAccepted"]
		})
}
