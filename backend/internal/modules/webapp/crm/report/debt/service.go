package debt

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"erpCluvix/internal/modules/webapp/crm/customer/scope"
	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"
)

var serviceDebtKeySearch = []string{
	"customer.fullname", "customer.code", "customer.phone", "order_service_used.code",
}

// osuPayable — phải thu của dòng dịch vụ (bmad/order-vat, phương án B): totalEnd là tiền TRƯỚC thuế, phiếu thu
// (payment) là tiền CÓ thuế ⇒ công nợ dòng = (totalEnd + vat_amount) − payment. vat_amount NOT NULL DEFAULT 0 ⇒
// cố ý KHÔNG COALESCE: totalEnd NULL vẫn ra NULL, giữ ngữ nghĩa predicate như ERP. Dòng cũ vat_amount = 0 ⇒
// số liệu y như trước khi có VAT.
const osuPayable = "(order_service_used.totalEnd + order_service_used.vat_amount)"

// serviceDebtPredicate — "dịch vụ còn nợ" = phải thu (có VAT) ≠ đã thu. Viết dạng `(<biểu thức>) = 1` để khớp
// chỉ mục hàm nếu sau này thêm (biểu thức chỉ mục phải là CHÍNH biểu thức này, kể cả vế vat_amount); cột NULL ⇒
// NULL ⇒ loại (như ERP).
const serviceDebtPredicate = "(" + osuPayable + " <> order_service_used.payment) = 1"

type serviceDebtTotals struct {
	Total        int64           `gorm:"column:total"`
	TotalEnd     decimal.Decimal `gorm:"column:totalEnd"`
	VatAmount    decimal.Decimal `gorm:"column:vatAmount"`
	TotalPayment decimal.Decimal `gorm:"column:totalPayment"`
	TotalDebt    decimal.Decimal `gorm:"column:totalDebt"`
	TotalDeposit decimal.Decimal `gorm:"column:totalDeposit"`
}

// serviceDebtPlan — câu nào đang dựng (mỗi câu một plan tối ưu khác nhau, cùng TẬP DÒNG).
type serviceDebtPlan int

const (
	serviceDebtPlanRows serviceDebtPlan = iota // đọc trọn tập dòng có thứ tự (export): ép range ngày khi hẹp
	serviceDebtPlanPage                        // câu trang ORDER BY … LIMIT: KHÔNG ép (đọc ngược PRIMARY dừng sớm)
	serviceDebtPlanAgg                         // câu gom tổng + cọc: ép range khi hẹp; rộng ⇒ hash join service
)

// serviceDebtDateIndex — chỉ mục ép dùng khi có dải ngày hẹp. Có sẵn từ migration TẠO BẢNG
// (20260928093237_create_order_service_used) ⇒ mọi DB CRM đều có, không cần dò information_schema
// trước khi ép (ép chỉ mục vắng mặt = Error 1176 = 500).
const serviceDebtDateIndex = "idx_osu_created_day_branch"

// serviceDebtHintMaxRangeSec — trần độ rộng dải ngày để ép serviceDebtDateIndex (31 ngày: đủ "tháng
// này"/"tháng trước" của FE). Rộng hơn thì tra PK theo chỉ mục đắt hơn quét bảng (số đo ở dưới).
const serviceDebtHintMaxRangeSec int64 = 31 * 24 * 60 * 60

// serviceDebtTable — FROM của câu công nợ dịch vụ, kèm `FORCE INDEX` CÓ CỔNG.
//
// Đo /check-scale 2026-10-19 trên bench 982.900 dòng `order_service_used` (30% dòng còn nợ), lọc
// tháng (90.407 dòng trong dải), `EXPLAIN ANALYZE` 1 lượt, cache ấm:
//   - plan cũ LẬT theo trạng thái cache: dẫn từ `service` rồi tra `fk-order_service_used-service_id`
//     ~885k lượt ⇒ 5,2s (lạnh, cả request 13s); hoặc quét bảng ⇒ 0,41s. Đã chặn LẬT bằng
//     STRAIGHT_JOIN (xem serviceDebtQuery) — từ đó osu luôn là bảng dẫn.
//   - osu dẫn, tự chọn: optimizer vẫn QUÉT BẢNG ngay cả với dải 1 tháng (ước ~21% bảng) ⇒ 422ms.
//   - ép `idx_osu_created_day_branch` (range + MRR): tháng **241ms**; 1 năm (519.567 dòng) **1.384ms**
//     so với quét bảng 527ms ⇒ CHỈ ép khi dải ≤ 31 ngày. Không có dải ngày ⇒ quét bảng là đúng.
//
// Không ép khi (allowHint=false do caller quyết, hoặc tự xét ở đây):
//   - câu TRANG (xem serviceDebtQueryPlan) hoặc phạm vi telesale (restricted — tập khách nhỏ,
//     `idx_osu_customer_totalend` qua semi-join tốt hơn): caller truyền allowHint=false;
//   - lọc `service_id` — `fk-order_service_used-service_id` (vài nghìn dòng) rẻ hơn hẳn dải tháng.
func serviceDebtTable(c *gin.Context, from, to int64, allowHint bool) string {
	if allowHint && from > 0 && to > 0 && to-from <= serviceDebtHintMaxRangeSec &&
		len(query.GetFilterValues(c, "service_id")) == 0 {
		return "order_service_used FORCE INDEX (" + serviceDebtDateIndex + ")"
	}
	return "order_service_used"
}

// serviceDebtQuery — dòng dịch vụ còn nợ (chỉ loại Dịch vụ / Liệu trình) trong phạm vi chi nhánh + bộ lọc, chưa
// SELECT/ORDER. from/to > 0 ⇒ lọc ngày lên đơn (created_day). Dùng cho câu GOM (tổng, totalDeposit) và
// export — đọc TRỌN tập dòng khớp nên ép range ngày (có cổng) là đúng.
func (h *Handler) serviceDebtQuery(c *gin.Context, withCustomer bool, from, to int64) *gorm.DB {
	return h.serviceDebtQueryPlan(c, withCustomer, from, to, serviceDebtPlanRows)
}

// serviceDebtQueryPlan — như serviceDebtQuery, chọn plan theo câu đang dựng (serviceDebtPlan).
//
// Câu TRANG (serviceDebtPlanPage, ORDER BY … LIMIT 15): không ép thì optimizer đọc ngược PRIMARY và dừng sau
// vài trăm dòng — tháng này, sort=-id (mặc định FE): **0,4ms**, ép range ngày 231ms (phải đọc trọn
// 90.407 dòng của tháng rồi sort), bản cũ 3.225ms (dẫn từ service). ⚠ Bench sinh `created_day` KHÔNG
// tương quan với `id`; dữ liệu thật thì tương quan ⇒ xem 1 tháng CŨ có thể phải đọc ngược qua mọi dòng
// mới hơn — vẫn bị chặn trên bởi 1 lượt quét PK (≈ quét bảng), không tệ hơn bản cũ.
//
// Câu GOM dải RỘNG / không lọc ngày (serviceDebtPlanAgg, không ép range): HASH JOIN với `service` làm
// bảng build (`FROM service STRAIGHT_JOIN order_service_used IGNORE INDEX (fk service_id)` ⇒ không còn
// đường tra osu theo service_id ⇒ optimizer buộc hash join, build 300 dòng service, probe = quét osu).
// Thay ~258k lượt tra PK `service` từng dòng. Đo bench (round 2, 2026-10-19, 1 lượt, máy đang chia tải):
// câu tổng+cọc gộp, không lọc ngày: STRAIGHT_JOIN osu→service 1.038ms → hash **766ms**. Không dùng
// khi lọc `service_id` (ref fk service_id rẻ hơn) hoặc phạm vi telesale (semi-join theo khách).
func (h *Handler) serviceDebtQueryPlan(c *gin.Context, withCustomer bool, from, to int64, plan serviceDebtPlan) *gorm.DB {
	// S-10: phạm vi telesale. Đọc TRƯỚC để cổng hint biết có đang bị giới hạn tập khách không;
	// lỗi đọc phạm vi ⇒ query mang lỗi, không trả dòng (fail-closed như scope.WhereCustomerID).
	custCond, custArgs, custErr := scope.CustomerIDCond(c, h.DB, "order_service_used.customer_id")

	allowHint := plan != serviceDebtPlanPage && custCond == ""
	table := serviceDebtTable(c, from, to, allowHint)
	hashService := plan == serviceDebtPlanAgg && allowHint && table == "order_service_used" &&
		len(query.GetFilterValues(c, "service_id")) == 0

	q := tenant.ScopedDBNullBranch(c, h.DB, "order_service_used")
	if hashService {
		// Cùng tập dòng với nhánh dưới (INNER JOIN theo PK service), chỉ khác thuật toán join.
		q = q.Table(serviceDebtHashFrom)
		if withCustomer {
			q = q.Joins("LEFT JOIN customer ON customer.id = order_service_used.customer_id")
		}
	} else {
		q = q.Table(table)
		if withCustomer {
			q = q.Joins("LEFT JOIN customer ON customer.id = order_service_used.customer_id")
		}
		// STRAIGHT_JOIN (= INNER JOIN, ép osu đứng TRƯỚC service): chặn plan dẫn từ `service` (270 dịch vụ
		// loại 1/3) rồi tra osu theo `fk-order_service_used-service_id` — plan đó tốn 5,2s @982.900 dòng
		// và optimizer chọn nó tuỳ trạng thái cache (xem serviceDebtTable). Cách truy cập osu (quét bảng /
		// range ngày / ref service_id / ref branch) vẫn để optimizer tự chọn. Kết quả không đổi: chỉ đổi
		// THỨ TỰ join, không đổi tập dòng.
		q = q.Joins("STRAIGHT_JOIN service ON service.id = order_service_used.service_id")
	}
	q = q.Where("service.serviceCategory_id IN ?", []int{define.ServiceCategoryService, define.ServiceCategoryTreatment}).
		Where(serviceDebtPredicate).
		// Khoá dịch vụ (isLock=2) chốt totalEnd + vat_amount = payment ⇒ predicate đã tự loại; ghi tường minh cho
		// đúng ý định.
		Where("order_service_used.isLock != ?", define.ServiceUsedIsLock)

	q = query.ApplyFilter(c, q, "branch_id", "order_service_used.branch_id")
	q = query.ApplyFilter(c, q, "service_id", "order_service_used.service_id")
	if from > 0 && to > 0 {
		q = q.Where("order_service_used.created_day BETWEEN ? AND ?", from, to)
	}
	if withCustomer {
		q = query.ApplyKeywordFilter(c, q, serviceDebtKeySearch...)
	}
	if custErr != nil {
		_ = q.AddError(custErr)
		return q
	}
	if custCond != "" {
		q = q.Where(custCond, custArgs...)
	}
	return q
}

// serviceDebtHashFrom — FROM của câu gom dải rộng (xem serviceDebtQueryPlan).
const serviceDebtHashFrom = "service STRAIGHT_JOIN order_service_used IGNORE INDEX (`fk-order_service_used-service_id`) " +
	"ON service.id = order_service_used.service_id"

func serviceDebtSort(c *gin.Context, q *gorm.DB) *gorm.DB {
	return query.ApplySort(c, q, "order_service_used.created_day DESC", "order_service_used.created_day", "order_service_used.id")
}

// ServiceDebts GET /crm/report/debt/service — công nợ theo dịch vụ (ERP: công nợ điều trị, bỏ phần buổi điều trị).
func (h *Handler) ServiceDebts(c *gin.Context) {
	from, ok1 := parseUnixParam(c, "_orderCreatedTime[gte]", 0)
	to, ok2 := parseUnixParam(c, "_orderCreatedTime[lte]", 0)
	if !ok1 || !ok2 {
		response.Validate(c, i18n.T(c, "invalid_request"))
		return
	}
	withCustomer := c.Query("_keyword") != ""

	if c.Query("_export") == "1" {
		exportHandlers.ExportServiceDebts(c, serviceDebtSort(c, h.serviceDebtQuery(c, true, from, to)))
		return
	}

	totals, err := h.serviceDebtSummary(c, withCustomer, from, to)
	if err != nil {
		pagination.FailQuery(c, err)
		return
	}

	items := []map[string]interface{}{}
	var result pagination.Result
	if page, perPage, offset := pagination.PageParams(c); totals.Total <= int64(offset) {
		result = pagination.ResultForPage(c, &items, totals.Total, page, perPage, "order_service_used")
	} else {
		q := serviceDebtSort(c, h.serviceDebtQueryPlan(c, withCustomer, from, to, serviceDebtPlanPage)).
			Select(`order_service_used.id, order_service_used.code,
				order_service_used.service_id, order_service_used.customer_id,
				order_service_used.totalEnd, order_service_used.vat_amount, order_service_used.payment,
				` + osuPayable + ` - order_service_used.payment AS debt,
				order_service_used.quantity, order_service_used.branch_id,
				order_service_used.created_day`)
		result = pagination.PaginateWithTotal(c, q, &items, totals.Total)
		if c.IsAborted() {
			return
		}
	}
	for _, item := range items {
		helpers.ParseDecimalFields(item, "debt")
	}
	attachDebtCustomers(c, h.DB, items)

	c.JSON(http.StatusOK, gin.H{
		"items": result.Items,
		"_meta": result.Meta,
		"total": gin.H{
			"totalEnd":     totals.TotalEnd,
			"vatAmount":    totals.VatAmount,
			"totalPayment": totals.TotalPayment,
			"totalDebt":    totals.TotalDebt,
			"totalDeposit": totals.TotalDeposit,
		},
	})
}

// serviceDebtSummary — dòng tổng (đếm + 4 tổng tiền) VÀ totalDeposit trong MỘT lượt đọc osu.
//
// totalDeposit = tổng cọc của các KHÁCH (mỗi khách 1 lần) có dịch vụ còn nợ theo ĐÚNG bộ lọc danh sách. Bản
// trước chạy 2 câu riêng trên cùng bộ lọc (tổng: 1 lượt quét; cọc: `customer WHERE id IN (subquery)` = lượt
// quét thứ 2 + khử trùng khách). Gộp: gom osu theo customer_id (đếm/tổng của nhóm cộng lại = đếm/tổng toàn
// bộ, tiền vẫn là DECIMAL cộng chính xác) rồi LEFT JOIN customer 1 lần/khách ⇒ cùng tập khách với `IN`.
// Bench không lọc ngày (232.405 dòng nợ / 117.732 khách, 1 lượt, máy chia tải, round 2 2026-10-19):
// 2 câu 658 + 898ms → gộp + hash join service **766ms**. Đối chiếu bench: 5 con số khớp tuyệt đối.
func (h *Handler) serviceDebtSummary(c *gin.Context, withCustomer bool, from, to int64) (serviceDebtTotals, error) {
	var s serviceDebtTotals
	inner := h.serviceDebtQueryPlan(c, withCustomer, from, to, serviceDebtPlanAgg).
		Select(`order_service_used.customer_id,
			COUNT(*) AS cnt,
			SUM(order_service_used.totalEnd) AS te,
			SUM(order_service_used.vat_amount) AS vat,
			SUM(order_service_used.payment) AS pay,
			SUM(` + osuPayable + ` - order_service_used.payment) AS debt`).
		Group("order_service_used.customer_id")
	// Lỗi gắn vào câu con (vd đọc phạm vi telesale lỗi — fail-closed) KHÔNG tự lan ra câu ngoài khi câu con
	// được nhúng làm bảng dẫn xuất ⇒ kiểm tay, nếu không sẽ chạy câu THIẾU vế phạm vi.
	if inner.Error != nil {
		return s, inner.Error
	}
	err := h.DB.Table("(?) AS g", inner).
		Joins("LEFT JOIN customer dep ON dep.id = g.customer_id").
		Select(`COALESCE(SUM(g.cnt), 0) AS total,
			COALESCE(SUM(g.te), 0) AS totalEnd,
			COALESCE(SUM(g.vat), 0) AS vatAmount,
			COALESCE(SUM(g.pay), 0) AS totalPayment,
			COALESCE(SUM(g.debt), 0) AS totalDebt,
			COALESCE(SUM(dep.totalDeposit), 0) AS totalDeposit`).
		Scan(&s).Error
	return s, err
}
