package handler

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func (h *Handler) buildReportServiceUsedQuery(c *gin.Context) *gorm.DB {
	q := tenant.ScopedDBNullBranch(c, h.DB, "order_service_used").
		Table(osuLockedReportTable(c, "")).
		Select(`order_service_used.service_id as id,
			service.name as service_name, service.code as service_code,
			COUNT(DISTINCT order_service_used.customer_id) AS customerCount,
			COUNT(order_service_used.id) as numService,
			SUM(quantity) as quantity,
			SUM(totalEnd) as totalEnd,
			SUM(payment) as payment,
			SUM(CASE WHEN service_discount_type = 2 THEN (service_discount * unit_price * quantity / 100) ELSE service_discount END) as service_discount_amount,
			SUM(discount_other) as discount_other,
			SUM(rank_discount_amount) as rank_discount_amount,
			SUM(refundServiceUsed) as refundServiceUsed`).
		Joins("LEFT JOIN service ON service.id = order_service_used.service_id").
		Joins("LEFT JOIN service_type ON service_type.id = service.service_type_id").
		Where("order_service_used.isLock = 1")
	if reportServiceUsedNeedsCustomer(c) {
		q = q.Joins(reportServiceUsedJoinCustomer)
	}

	q = applyReportServiceUsedFilters(c, q)

	// _groupLocal
	if vals := query.GetFilterValues(c, "_groupLocal"); len(vals) > 0 {
		provinceIds := query.GetProvinceIdsByGroupLocal(h.DB, vals)
		if len(provinceIds) > 0 {
			q = q.Where("customer.province_id IN ?", provinceIds)
		}
	}

	// _groupCountry
	if vals := query.GetFilterValues(c, "_groupCountry"); len(vals) > 0 {
		countryIds := query.GetCountryIdsByGroupCountry(h.DB, vals)
		if len(countryIds) > 0 {
			q = q.Where("customer.country_id IN ?", countryIds)
		}
	}

	// GROUP BY chỉ `service_id` — KHÔNG kèm `service.name, service.code`.
	//
	// ⚠ Bản vá hiệu năng có số đo, không phải dọn dẹp phong cách. Hai cột kia phụ thuộc HÀM vào
	// `service_id` (join theo khoá chính `service.id`) nên nhóm ra y hệt, nhưng đưa chúng vào
	// GROUP BY biến khoá sắp xếp thành 2 VARCHAR của BẢNG ĐƯỢC JOIN ⇒ filesort phải so chuỗi
	// trên toàn bộ dòng khớp. Đo @898.720 dòng `order_service_used` (782.032 dòng `isLock=1`),
	// ca người dùng bấm dấu × xoá bộ lọc ngày (2026-09-09):
	//   · GROUP BY 3 cột: 40,6–49,3s   (cùng 782.052 dòng phải sort)
	//   · GROUP BY 1 cột: 10,0–13,4s   ⇒ ~4x, KHÔNG cần thêm chỉ mục nào
	// `ONLY_FULL_GROUP_BY` đang BẬT và câu vẫn hợp lệ: MySQL suy ra phụ thuộc hàm qua đẳng thức
	// join `service.id = order_service_used.service_id`. `export.ExportTreatmentServices` vốn đã
	// gom nhóm đúng 1 cột này — nay hai đường thống nhất.
	//
	// Đối chiếu: 7 bộ lọc (không lọc · tháng · 1 ngày · 1 chi nhánh · từ khoá có/không khớp ·
	// nguồn khách), chuỗi ghép TOÀN BỘ nhóm × 13 cột KHỚP tuyệt đối giữa hai cách viết.
	q = q.Group("order_service_used.service_id").Order("COUNT(order_service_used.id) DESC")

	return q
}

const reportServiceUsedJoinCustomer = "LEFT JOIN customer ON customer.id = order_service_used.customer_id"

// reportServiceUsedNeedsCustomer — chỉ JOIN `customer` khi có bộ lọc đọc cột của nó (`source_id`,
// `_groupLocal`, `_groupCountry`). Không ai SELECT cột customer (kể cả export) và join theo PK không nhân
// dòng ⇒ bỏ join không đổi kết quả. Đo 1 năm @454.011 dòng khớp (2026-10-19): riêng bước tra PK
// `customer` cho từng dòng tốn ~1,8s/câu (2.976ms → ~1.2s), mà mỗi lần mở màn chạy 2 câu.
//
// ⚠ Thêm bộ lọc mới đọc cột `customer.*` thì PHẢI thêm điều kiện vào đây — thiếu ⇒ `Unknown column`
// (500), không âm thầm sai số.
func reportServiceUsedNeedsCustomer(c *gin.Context) bool {
	return hasFilterParam(c, "source_id") ||
		len(query.GetFilterValues(c, "_groupLocal")) > 0 ||
		len(query.GetFilterValues(c, "_groupCountry")) > 0
}

// applyReportServiceUsedFilters — bộ lọc DÙNG CHUNG của report-service-used (danh sách, totals, export, đường
// 1 lượt), trừ `_groupLocal`/`_groupCountry` (chỉ câu danh sách/export áp). Cột `customer.*` cần JOIN customer
// (reportServiceUsedNeedsCustomer), cột `service.*` cần JOIN service (reportServiceUsedNeedsService).
func applyReportServiceUsedFilters(c *gin.Context, q *gorm.DB) *gorm.DB {
	q = query.ApplyFilter(c, q, "created_day", "order_service_used.created_day")
	q = query.ApplyFilter(c, q, "created_at", "order_service_used.created_at")
	q = query.ApplyFilter(c, q, "branch_id", "order_service_used.branch_id")
	q = query.ApplyFilter(c, q, "created_by", "order_service_used.created_by")
	q = query.ApplyFilter(c, q, "sourceService_id", "order_service_used.source_id")
	q = query.ApplyFilter(c, q, "source_id", "customer.source_id")
	q = query.ApplyFilter(c, q, "service_type_id", "service.service_type_id")
	if kw := strings.TrimSpace(c.Query("_keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("service.name LIKE ? OR service.code LIKE ? OR service.code_old LIKE ?", like, like, like)
	}
	return q
}

// reportServiceUsedNeedsService — bộ lọc đọc cột `service.*` (từ khoá, loại dịch vụ).
func reportServiceUsedNeedsService(c *gin.Context) bool {
	return strings.TrimSpace(c.Query("_keyword")) != "" || hasFilterParam(c, "service_type_id")
}

// reportServiceUsedAgg — 9 cột tổng của 1 nhóm dịch vụ (cột osu ghi rõ bảng: câu 1 lượt có thể JOIN service/customer).
const reportServiceUsedAgg = `COUNT(DISTINCT order_service_used.customer_id) AS customerCount,
			COUNT(order_service_used.id) AS numService,
			SUM(order_service_used.quantity) AS quantity,
			SUM(order_service_used.totalEnd) AS totalEnd,
			SUM(order_service_used.payment) AS payment,
			SUM(CASE WHEN order_service_used.service_discount_type = 2 THEN (order_service_used.service_discount * order_service_used.unit_price * order_service_used.quantity / 100) ELSE order_service_used.service_discount END) AS service_discount_amount,
			SUM(order_service_used.discount_other) AS discount_other,
			SUM(order_service_used.rank_discount_amount) AS rank_discount_amount,
			SUM(order_service_used.refundServiceUsed) AS refundServiceUsed`

// reportServiceUsedMoneyCols — alias tiền/SUM ra chuỗi khi scan vào map, phải parse tay (bảng dẫn xuất ⇒
// Paginate/extractTables không suy ra được bảng gốc). `quantity` = SUM(int) ⇒ DECIMAL.
var reportServiceUsedMoneyCols = []string{"totalEnd", "payment", "service_discount_amount",
	"discount_other", "rank_discount_amount", "refundServiceUsed", "quantity"}

// reportServiceUsedOnePass — danh sách nhóm + dòng tổng trong MỘT lượt đọc osu.
//
// Bản trước: câu danh sách (GROUP BY service_id, JOIN service + service_type + [customer] cho TỪNG dòng)
// + câu totals (cùng JOIN, không GROUP BY) = 2 lượt quét + 2×454k lượt tra PK danh mục ở dải 1 năm.
// Bản này: bảng dẫn xuất gom osu theo `service_id` `WITH ROLLUP` (dòng ROLLUP = totals: số khách PHÂN BIỆT
// toàn bộ, tổng dòng, tổng tiền; số nhóm = số dòng không phải ROLLUP = COUNT(DISTINCT service_id) cũ vì
// service_id NOT NULL), JOIN `service` chỉ khi bộ lọc cần cột của nó, rồi mới JOIN `service` lấy tên trên
// ~300 dòng đã gom. Phân trang cắt trong Go (cùng `_meta`/header như PaginateWithTotal).
//
// Bench 1 năm (454.011 dòng khớp), 1 lượt EXPLAIN ANALYZE, máy chia tải (round 2, 2026-10-19):
//   - cũ: danh sách 1.252ms + totals 1.452ms (JOIN service+service_type mỗi dòng);
//   - totals không JOIN 909ms; danh sách gom trước rồi JOIN 1.025ms;
//   - gộp 1 câu ROLLUP **1.325ms** cho cả hai.
//
// Thứ tự: `numService DESC, id` — bản cũ chỉ `COUNT DESC` (nhóm hoà số dòng ra thứ tự tuỳ plan), nay hoà
// thì theo id tăng: tập nhóm và mọi con số không đổi.
func (h *Handler) reportServiceUsedOnePass(c *gin.Context) {
	var all []map[string]interface{}
	tx := h.reportServiceUsedRollupQuery(c).Find(&all)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	// Dòng tổng = dòng `id` NULL (service_id NOT NULL ⇒ chỉ ROLLUP sinh NULL). KHÔNG dựa vào GROUPING():
	// MySQL 9.7 với đầu vào RỖNG vẫn trả 1 dòng (NULL, GROUPING = 0, mọi COUNT = 0) — đã kiểm trên DB thật.
	var totalRow map[string]interface{}
	rows := make([]map[string]interface{}, 0, len(all))
	for _, r := range all {
		helpers.ParseDecimalFields(r, reportServiceUsedMoneyCols...)
		if r["id"] == nil {
			totalRow = r
			continue
		}
		rows = append(rows, r)
	}

	page, perPage, offset := pagination.PageParams(c)
	items := make([]map[string]interface{}, 0, perPage)
	if offset < len(rows) {
		items = rows[offset:min(offset+perPage, len(rows))]
	}
	result := pagination.ResultForPage(c, &items, int64(len(rows)), page, perPage)

	// Nest service object to match Yii2 joinWith response
	for _, item := range items {
		item["service_id"] = item["id"]
		item["service"] = map[string]any{
			"id":   item["id"],
			"name": item["service_name"],
			"code": item["service_code"],
		}
		delete(item, "service_name")
		delete(item, "service_code")
	}

	// Không dòng nào khớp ⇒ dòng tổng toàn 0 (hoặc vắng) ⇒ mọi tổng = 0 (như bản cũ).
	c.JSON(http.StatusOK, gin.H{
		"object": result,
		"totals": gin.H{
			"customerCount":           helpers.ToInt64(totalRow["customerCount"]),
			"numService":              helpers.ToInt64(totalRow["numService"]),
			"numQuantity":             helpers.ToInt64(totalRow["quantity"]),
			"totalTotalEnd":           helpers.ToDecimal(totalRow["totalEnd"]),
			"totalPayment":            helpers.ToDecimal(totalRow["payment"]),
			"totalDiscountService":    helpers.ToDecimal(totalRow["service_discount_amount"]),
			"totalDiscountOther":      helpers.ToDecimal(totalRow["discount_other"]),
			"totalRankDiscountAmount": helpers.ToDecimal(totalRow["rank_discount_amount"]),
			"totalRefundServiceUsed":  helpers.ToDecimal(totalRow["refundServiceUsed"]),
		},
	})
}

// reportServiceUsedRollupQuery — câu của reportServiceUsedOnePass (tách ra để test khuôn câu bằng DryRun).
func (h *Handler) reportServiceUsedRollupQuery(c *gin.Context) *gorm.DB {
	inner := tenant.ScopedDBNullBranch(c, h.DB, "order_service_used").
		Table(osuLockedReportTable(c, "")).
		Where("order_service_used.isLock = 1")
	if reportServiceUsedNeedsService(c) {
		inner = inner.Joins("LEFT JOIN service ON service.id = order_service_used.service_id")
	}
	if reportServiceUsedNeedsCustomer(c) {
		inner = inner.Joins(reportServiceUsedJoinCustomer)
	}
	inner = applyReportServiceUsedFilters(c, inner).
		Select("order_service_used.service_id AS id, " + reportServiceUsedAgg).
		Group("order_service_used.service_id WITH ROLLUP")
	return h.DB.Table("(?) AS a", inner).
		Select(`a.id, service.name AS service_name, service.code AS service_code,
			a.customerCount, a.numService, a.quantity, a.totalEnd, a.payment, a.service_discount_amount,
			a.discount_other, a.rank_discount_amount, a.refundServiceUsed`).
		Joins("LEFT JOIN service ON service.id = a.id").
		Order("a.numService DESC, a.id")
}

// rangeFilterSec đọc cận dưới/cận trên (unix giây) của một filter khoảng ngày, chấp nhận CẢ HAI
// khuôn FE sinh ra: `filter[<key>][gte]` (field thường) và `<key>[gte]` (field tiền tố `_`).
// Trả ok=false khi thiếu một trong hai cận, không parse được số, hoặc cận trên nhỏ hơn cận dưới.
func rangeFilterSec(c *gin.Context, key string) (from, to int64, ok bool) {
	pick := func(op string) string {
		if v := c.Query("filter[" + key + "][" + op + "]"); v != "" {
			return v
		}
		return c.Query(key + "[" + op + "]")
	}
	f, errF := strconv.ParseInt(pick("gte"), 10, 64)
	t, errT := strconv.ParseInt(pick("lte"), 10, 64)
	if errF != nil || errT != nil || t < f {
		return 0, 0, false
	}
	return f, t, true
}

// ReportServiceUsed GET /crm/order-service-used/report-service-used
func (h *Handler) ReportServiceUsed(c *gin.Context) {
	// Bắt buộc có dải ngày `created_day` và trần 1 năm (define.ReportMaxRangeSec) — cùng luật
	// với `AppointmentStatistics`.
	//
	// Không có dải ngày thì đây là aggregate TRỌN bảng `order_service_used`, chạy 3 lượt quét
	// mỗi lần mở màn. Đo @898.720 dòng (782.032 dòng `isLock = 1`), sau khi đã vá GROUP BY +
	// COUNT + gộp totals (2026-09-09): vẫn ~14–18s. Và đây KHÔNG phải ca giả định — nút ×
	// trên ô chọn ngày của FE (`clearCreateDay`) đi thẳng vào nó bằng đúng một cú bấm.
	//
	// Vượt trần ⇒ 422 tường minh, KHÔNG tự cắt bớt dải ngày: cắt im lặng là trả thiếu số liệu
	// mà người xem báo cáo không có cách nào biết.
	//
	// ⚠ Guard đặt TRƯỚC nhánh `_export=1`: export chạy đúng câu này qua con trỏ, không có lý do
	// để nó được miễn trần.
	from, to, ok := rangeFilterSec(c, "created_day")
	if !ok {
		response.Validate(c, i18n.T(c, "report_date_range_required"))
		return
	}
	if to-from > define.ReportMaxRangeSec {
		response.Validate(c, i18n.T(c, "report_date_range_max_one_year"))
		return
	}

	if c.Query("_export") == "1" {
		exportHandlers.ExportTreatmentServices(c, h.buildReportServiceUsedQuery(c))
		return
	}

	// Đường 1 lượt (ROLLUP) — mọi bộ lọc trừ `_groupLocal`/`_groupCountry`: câu totals cũ KHÔNG áp 2 bộ lọc
	// này còn câu danh sách có, nên gộp 2 câu sẽ đổi số tổng ⇒ có 2 bộ lọc đó thì đi đường 2 câu cũ bên dưới.
	if len(query.GetFilterValues(c, "_groupLocal")) == 0 && len(query.GetFilterValues(c, "_groupCountry")) == 0 {
		h.reportServiceUsedOnePass(c)
		return
	}

	q := h.buildReportServiceUsedQuery(c)

	// Totals — separate query without GROUP BY.
	// `customerCount` gộp CHUNG câu này thay vì chạy câu thứ 2: cùng bộ lọc, cùng bảng, nên
	// tách ra chỉ tổ quét thêm một lượt nữa. Đo ca không lọc ngày @898.720 dòng (2026-09-09):
	// 2 câu riêng 15,8–28,4s -> gộp 1 câu 2,3–2,8s (~7x).
	var totals struct {
		TotalGroups        int64           `gorm:"column:totalGroups"`
		CustomerCount      int64           `gorm:"column:customerCount"`
		NumService         int64           `gorm:"column:numService"`
		Quantity           int64           `gorm:"column:quantity"`
		TotalEnd           decimal.Decimal `gorm:"column:totalEnd"`
		Payment            decimal.Decimal `gorm:"column:payment"`
		ServiceDiscount    decimal.Decimal `gorm:"column:service_discount_amount"`
		DiscountOther      decimal.Decimal `gorm:"column:discount_other"`
		RankDiscountAmount decimal.Decimal `gorm:"column:rank_discount_amount"`
		RefundServiceUsed  decimal.Decimal `gorm:"column:refundServiceUsed"`
	}
	// Builder closure → mỗi Scan dùng *gorm.DB tươi, tránh GORM append Joins duplicate.
	// ⚠ Câu totals KHÔNG áp `_groupLocal`/`_groupCountry` (câu danh sách có) — lệch sẵn từ trước, giữ
	// nguyên ở bản vá hiệu năng 2026-10-19 (đối chiếu old == new); sửa là đổi số hiển thị, cần quyết riêng.
	buildTotalsQ := func() *gorm.DB {
		tq := tenant.ScopedDBNullBranch(c, h.DB, "order_service_used").
			Table(osuLockedReportTable(c, "")).
			Joins("LEFT JOIN service ON service.id = order_service_used.service_id").
			Joins("LEFT JOIN service_type ON service_type.id = service.service_type_id").
			Where("order_service_used.isLock = 1")
		if reportServiceUsedNeedsCustomer(c) {
			tq = tq.Joins(reportServiceUsedJoinCustomer)
		}

		return applyReportServiceUsedFilters(c, tq)
	}

	// COALESCE: dải rỗng ⇒ SUM = NULL, mà NULL KHÔNG scan được vào decimal.Decimal (lỗi ⇒ 500). Bản HEAD nuốt
	// lỗi đó nên trả 0 — COALESCE giữ đúng con số 0 ấy mà không cần nuốt lỗi.
	tx := buildTotalsQ().Select(`COUNT(order_service_used.id) as numService,
		COALESCE(SUM(quantity), 0) as quantity,
		COALESCE(SUM(totalEnd), 0) as totalEnd,
		COALESCE(SUM(payment), 0) as payment,
		COALESCE(SUM(CASE WHEN service_discount_type = 2 THEN (service_discount * unit_price * quantity / 100) ELSE service_discount END), 0) as service_discount_amount,
		COALESCE(SUM(discount_other), 0) as discount_other,
		COALESCE(SUM(rank_discount_amount), 0) as rank_discount_amount,
		COALESCE(SUM(refundServiceUsed), 0) as refundServiceUsed,
		COUNT(DISTINCT order_service_used.customer_id) as customerCount,
		COUNT(DISTINCT order_service_used.service_id) as totalGroups`).
		Scan(&totals)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	// Tổng số NHÓM = số dịch vụ phân biệt. `pagination.Paginate` bọc NGUYÊN câu (13 cột tổng +
	// GROUP BY + ORDER BY) vào `COUNT(*) FROM (…)`, tức phải dựng lại trọn bảng dẫn xuất chỉ để
	// đếm số dòng của nó. `COUNT(DISTINCT service_id)` trên cùng bộ lọc cho ĐÚNG con số đó mà
	// không aggregate gì. Đo ca không lọc ngày @898.720 dòng (2026-09-09):
	// COUNT dẫn xuất 33,0–71,8s (sort 782.568 dòng) -> 1,77–1,88s (sort 0) ⇒ ~19-40x.
	// Đối chiếu số nhóm trên 7 bộ lọc: 536/131/0/293/1/0/371 — khớp tuyệt đối hai cách.
	// 2026-10-19: gộp luôn vào câu totals ở trên (cùng bảng, cùng bộ lọc, không GROUP BY) ⇒ bớt 1
	// lượt quét/lần mở màn; trước đó lỗi SQL của cả 2 câu bị nuốt (totals = 0, vẫn 200).
	totalGroups := totals.TotalGroups

	// Paginate
	var items []map[string]interface{}
	result := pagination.PaginateWithTotal(c, q, &items, totalGroups)
	if c.IsAborted() {
		return
	}

	// `pagination.Paginate` chỉ parse decimal theo TÊN CỘT của bảng (extractTables →
	// ParseDecimalFieldsForTables). `service_discount_amount` là alias của biểu thức CASE,
	// KHÔNG phải cột của bảng nào ⇒ vẫn ra JSON string nếu không gọi tay.
	// Các alias tiền còn lại (totalEnd, payment, discount_other,
	// rank_discount_amount, refundServiceUsed) TRÙNG tên cột `order_service_used` nên đã được
	// Paginate phủ; `quantity` = SUM(int) — MySQL vẫn trả DECIMAL cho SUM() dù cột gốc INT, và vì
	// `ParseDecimalFieldsForTables` tra theo SCHEMA thật (quantity là INT, không phải decimal) nên
	// không phủ được alias này dù trùng tên cột — parse tường minh (story-34 F-E, đã duyệt vá).
	for _, item := range items {
		helpers.ParseDecimalFields(item, "service_discount_amount", "quantity")
	}

	// Nest service object to match Yii2 joinWith response
	for _, item := range items {
		item["service_id"] = item["id"]
		item["service"] = map[string]any{
			"id":   item["id"],
			"name": item["service_name"],
			"code": item["service_code"],
		}
		delete(item, "service_name")
		delete(item, "service_code")
	}

	c.JSON(http.StatusOK, gin.H{
		"object": result,
		"totals": gin.H{
			"customerCount":           totals.CustomerCount,
			"numService":              totals.NumService,
			"numQuantity":             totals.Quantity,
			"totalTotalEnd":           totals.TotalEnd,
			"totalPayment":            totals.Payment,
			"totalDiscountService":    totals.ServiceDiscount,
			"totalDiscountOther":      totals.DiscountOther,
			"totalRankDiscountAmount": totals.RankDiscountAmount,
			"totalRefundServiceUsed":  totals.RefundServiceUsed,
		},
	})
}

// buildServiceTypeStatsQuery dựng phần lọc của báo cáo loại dịch vụ: phạm vi tenant, 2 join danh
// mục, điều kiện `isLock` và bộ lọc ngày — tập dòng osu được tính vào báo cáo.
//
// ⚠ Tham số thứ 3 của ScopedDBNullBranch là TÊN BẢNG THẬT, tham số thứ 4 mới là alias. Trước
// 2026-09-09 chỗ này truyền `"osu"` (alias) làm tên bảng ⇒ registry scope không có "osu" ⇒ scope
// company VÀ branch đều bị BỎ QUA IM LẶNG: báo cáo trả dữ liệu của MỌI chi nhánh bất kể quyền
// người xem (đo @674.040 dòng, lọc tháng, user chỉ có chi nhánh 1: 2.688 khách / 7,80 tỷ ->
// đúng ra là 258 khách / 1,54 tỷ). Và vì query không còn vế `company_id` nên
// `idx_osu_appt_day_cover` (dẫn đầu `company_id`) KHÔNG dùng được ⇒ mọi lần mở màn `Table scan`
// toàn bảng: lọc 1 NGÀY (972 dòng khớp) vẫn đọc 674.040 dòng / 1.148ms; sau khi vá là
// `Index range scan` -> 614 dòng / 2ms (1 chi nhánh), 2.650 dòng / 9ms (toàn chi nhánh).
//
// 2026-10-19: `STRAIGHT_JOIN` (= INNER JOIN, osu luôn là bảng dẫn) thay cho `LEFT JOIN … WHERE st.id
// IS NOT NULL` — cùng tập dòng (LEFT JOIN rồi đòi st.id NOT NULL ⇔ có service VÀ có service_type), chỉ
// khoá thứ tự join để không lật sang "dẫn từ service rồi tra osu theo service_id". Index hint theo dải
// ngày: osuLockedReportTable.
//
// Round 2 (2026-10-19) — dải rộng / không lọc ngày (osuLockedReportTable trả IGNORE INDEX, gồm cả
// `fk-order_service_used-service_id`): đảo thành `service_type st STRAIGHT_JOIN service STRAIGHT_JOIN osu`.
// osu không còn chỉ mục nào theo service_id ⇒ optimizer buộc HASH JOIN, build = 300 dòng danh mục, probe =
// quét osu — thay 858k lượt tra PK `service` + 858k lượt tra PK `service_type`. Cùng tập dòng (INNER JOIN
// theo PK cả hai phía). Bench 1 lượt EXPLAIN ANALYZE, máy chia tải: 1 năm 2.005 → **1.382ms**; không lọc
// ngày 3.674 → **2.711ms** (riêng bước join 1,83 → 0,66s; phần còn lại là sort + COUNT(DISTINCT)+ROLLUP
// trên 858k dòng — xem serviceTypeStatsQuery). Dải hẹp (FORCE range ~79k dòng) giữ tra PK: rẻ, khỏi dựng hash.
// Không hint (dạng lọc ngày lạ) cũng giữ osu dẫn: đảo thứ tự mà osu còn fk service_id là mở lại plan lật 5s.
func (h *Handler) buildServiceTypeStatsQuery(c *gin.Context) *gorm.DB {
	table := osuLockedReportTable(c, "osu")
	q := tenant.ScopedDBNullBranch(c, h.DB, "order_service_used", "osu")
	if strings.Contains(table, "IGNORE INDEX") {
		q = q.Table("service_type st STRAIGHT_JOIN service ON service.service_type_id = st.id STRAIGHT_JOIN " +
			table + " ON osu.service_id = service.id")
	} else {
		q = q.Table(table).
			Joins("STRAIGHT_JOIN service ON service.id = osu.service_id").
			Joins("STRAIGHT_JOIN service_type st ON st.id = service.service_type_id")
	}
	q = q.Where("osu.isLock = 1")
	return query.ApplyFilter(c, q, "created_day", "osu.created_day")
}

// serviceTypeStatsAggSelect — 10 cột tổng của 1 nhóm loại dịch vụ, tính trên dòng osu.
const serviceTypeStatsAggSelect = `COUNT(DISTINCT osu.customer_id) AS customerCount,
			COUNT(osu.id) AS numService,
			SUM(osu.quantity) AS quantity,
			SUM(osu.totalEnd) AS totalEnd,
			SUM(osu.payment) AS payment,
			SUM(CASE WHEN osu.service_discount_type = 2 THEN (osu.service_discount * osu.unit_price * osu.quantity / 100) ELSE osu.service_discount END) AS service_discount_amount,
			SUM(osu.discount_other) AS discount_other,
			SUM(osu.rank_discount_amount) AS rank_discount_amount,
			SUM(osu.refundServiceUsed) AS refundServiceUsed`

// serviceTypeStatsQuery — câu báo cáo: gom osu theo KHOÁ SỐ `service.service_type_id` trong bảng dẫn
// xuất (kèm `WITH ROLLUP` ⇒ thêm 1 dòng tổng, `tid` NULL ⇒ `st.id` NULL), rồi mới JOIN `service_type` lấy tên
// trên ~20 dòng đã gom.
//
// Bản cũ `GROUP BY st.id, st.name` ⇒ filesort khoá có VARCHAR trên MỌI dòng khớp. Bench 982.900 dòng osu
// (858.011 isLock=1), không lọc ngày, `EXPLAIN ANALYZE` 1 lượt (2026-10-19):
//   - cũ: `Sort: st.id, st.name` 858.011 dòng 11,7s, cả câu **16,5s**; + câu `COUNT(DISTINCT customer_id)`
//     riêng (bench http: cả request 10,5s ấm / 17,4s lạnh);
//   - gom theo `service.service_type_id` (INT) trong bảng dẫn xuất: sort 0,4s, cả câu 4,0s;
//   - + IGNORE các chỉ mục dẫn đầu isLock (quét bảng thay vì `ref isLock=1` + tra PK): **2,2s**;
//   - + `WITH ROLLUP` lấy luôn số khách phân biệt toàn bộ ⇒ bỏ hẳn câu đếm thứ 2.
//
// Đã LOẠI: chỉ đổi `GROUP BY st.id` (vẫn JOIN st cho từng dòng trước khi gom — khoá INT nhưng tra PK
// service_type 858k lần, không cần thiết khi đã gom theo service_type_id).
//
// ROLLUP: dòng tổng có `customerCount` = COUNT(DISTINCT customer_id) trên TOÀN BỘ dòng khớp — đúng con số
// câu đếm riêng cũ trả (khách mua nhiều loại chỉ tính 1 lần), KHÔNG phải cộng customerCount từng nhóm.
// Đầu vào rỗng: MySQL 9.7 vẫn trả 1 dòng ROLLUP (NULL, GROUPING() = 0, COUNT = 0) ⇒ nhận dòng tổng bằng
// `st.id` NULL (tid luôn khác NULL ở dòng nhóm vì đã INNER JOIN service_type), KHÔNG bằng GROUPING();
// tổng khách = 0 (cũ cũng 0).
func (h *Handler) serviceTypeStatsQuery(c *gin.Context) *gorm.DB {
	inner := h.buildServiceTypeStatsQuery(c).
		Select("service.service_type_id AS tid, " + serviceTypeStatsAggSelect).
		Group("service.service_type_id WITH ROLLUP")
	return h.DB.Table("(?) AS a", inner).
		Select(`st.id, st.name, a.customerCount, a.numService, a.quantity, a.totalEnd, a.payment,
			a.service_discount_amount, a.discount_other, a.rank_discount_amount, a.refundServiceUsed`).
		Joins("LEFT JOIN service_type st ON st.id = a.tid").
		Order("a.totalEnd DESC")
}

// ServiceTypeStatistics GET /crm/report/service-type-statistics
// Báo cáo dịch vụ đồng ý nhóm theo loại dịch vụ, nguồn từ order_service_used (isLock=1).
func (h *Handler) ServiceTypeStatistics(c *gin.Context) {
	var all []map[string]interface{}
	tx := h.serviceTypeStatsQuery(c).Find(&all)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	// Tách dòng tổng ROLLUP: customerCount toàn bộ — riêng biệt để tránh double-count khách mua nhiều loại DV.
	var tCustomerCount int64
	rows := make([]map[string]interface{}, 0, len(all))
	for _, r := range all {
		if r["id"] == nil {
			tCustomerCount = helpers.ToInt64(r["customerCount"])
			continue
		}
		rows = append(rows, r)
	}

	// Alias của SUM(decimal)/biểu thức → []uint8 → JSON ra string; alias không thuộc bảng nào
	// nên phải liệt kê tường minh. `quantity` = SUM(int) — MySQL cũng trả DECIMAL nên ra string qua
	// cùng đường (story-34 F-E, đã audit + duyệt vá — FE trước đó tự coerce `+t.quantity`).
	// numService/customerCount = COUNT thuần → BIGINT, không cần parse.
	for _, r := range rows {
		helpers.ParseDecimalFields(r, "totalEnd", "payment", "service_discount_amount",
			"discount_other", "rank_discount_amount", "refundServiceUsed", "quantity")
	}

	var tNumService, tQuantity int64
	var tTotalEnd, tPayment, tDiscountService, tDiscountOther, tRankDiscount, tRefund decimal.Decimal
	for _, r := range rows {
		tNumService += helpers.ToInt64(r["numService"])
		tQuantity += helpers.ToInt64(r["quantity"])
		tTotalEnd = tTotalEnd.Add(helpers.ToDecimal(r["totalEnd"]))
		tPayment = tPayment.Add(helpers.ToDecimal(r["payment"]))
		tDiscountService = tDiscountService.Add(helpers.ToDecimal(r["service_discount_amount"]))
		tDiscountOther = tDiscountOther.Add(helpers.ToDecimal(r["discount_other"]))
		tRankDiscount = tRankDiscount.Add(helpers.ToDecimal(r["rank_discount_amount"]))
		tRefund = tRefund.Add(helpers.ToDecimal(r["refundServiceUsed"]))
	}

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
		"items": items,
		"_meta": gin.H{
			"totalCount":  total,
			"pageCount":   pageCount,
			"currentPage": page,
			"perPage":     perPage,
		},
		"totals": gin.H{
			"customerCount":           tCustomerCount,
			"numService":              tNumService,
			"quantity":                tQuantity,
			"totalEnd":                tTotalEnd,
			"payment":                 tPayment,
			"service_discount_amount": tDiscountService,
			"discount_other":          tDiscountOther,
			"rank_discount_amount":    tRankDiscount,
			"refundServiceUsed":       tRefund,
		},
	})
}
