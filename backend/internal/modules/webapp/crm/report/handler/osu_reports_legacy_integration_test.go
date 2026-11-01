//go:build integration

package handler

// BẢN CŨ (HEAD trước bản vá hiệu năng 2026-10-19) của report-service-used + service-type-statistics,
// chép NGUYÊN VĂN chỉ đổi tên hàm (+ bỏ nhánh `_export`, export đối chiếu riêng bằng câu SQL) — làm
// chuẩn đối chiếu old == new trong osu_reports_parity_integration_test.go. KHÔNG sửa logic ở đây: file
// này là "đáp án cũ"; muốn đổi hành vi báo cáo thì đổi ở treatment.go và cập nhật kỳ vọng test.

import (
	"math"
	"net/http"
	"strconv"
	"strings"

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

func (h *Handler) legacyBuildReportServiceUsedQuery(c *gin.Context) *gorm.DB {
	q := tenant.ScopedDBNullBranch(c, h.DB, "order_service_used").
		Table("order_service_used").
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
		Joins("LEFT JOIN customer ON customer.id = order_service_used.customer_id").
		Where("order_service_used.isLock = 1")

	// Range filters
	q = query.ApplyFilter(c, q, "created_day", "order_service_used.created_day")
	q = query.ApplyFilter(c, q, "created_at", "order_service_used.created_at")

	// Exact filters
	q = query.ApplyFilter(c, q, "branch_id", "order_service_used.branch_id")
	q = query.ApplyFilter(c, q, "created_by", "order_service_used.created_by")
	q = query.ApplyFilter(c, q, "sourceService_id", "order_service_used.source_id")
	q = query.ApplyFilter(c, q, "source_id", "customer.source_id")
	q = query.ApplyFilter(c, q, "service_type_id", "service.service_type_id")

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

	// Keyword
	if kw := strings.TrimSpace(c.Query("_keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("service.name LIKE ? OR service.code LIKE ? OR service.code_old LIKE ?", like, like, like)
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

func (h *Handler) legacyReportServiceUsed(c *gin.Context) {
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

	q := h.legacyBuildReportServiceUsedQuery(c)

	// Totals — separate query without GROUP BY.
	// `customerCount` gộp CHUNG câu này thay vì chạy câu thứ 2: cùng bộ lọc, cùng bảng, nên
	// tách ra chỉ tổ quét thêm một lượt nữa. Đo ca không lọc ngày @898.720 dòng (2026-09-09):
	// 2 câu riêng 15,8–28,4s -> gộp 1 câu 2,3–2,8s (~7x).
	var totals struct {
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
	buildTotalsQ := func() *gorm.DB {
		tq := tenant.ScopedDBNullBranch(c, h.DB, "order_service_used").
			Table("order_service_used").
			Joins("LEFT JOIN service ON service.id = order_service_used.service_id").
			Joins("LEFT JOIN service_type ON service_type.id = service.service_type_id").
			Joins("LEFT JOIN customer ON customer.id = order_service_used.customer_id").
			Where("order_service_used.isLock = 1")

		tq = query.ApplyFilter(c, tq, "created_day", "order_service_used.created_day")
		tq = query.ApplyFilter(c, tq, "created_at", "order_service_used.created_at")
		tq = query.ApplyFilter(c, tq, "branch_id", "order_service_used.branch_id")
		tq = query.ApplyFilter(c, tq, "created_by", "order_service_used.created_by")
		tq = query.ApplyFilter(c, tq, "sourceService_id", "order_service_used.source_id")
		tq = query.ApplyFilter(c, tq, "source_id", "customer.source_id")
		tq = query.ApplyFilter(c, tq, "service_type_id", "service.service_type_id")

		if kw := strings.TrimSpace(c.Query("_keyword")); kw != "" {
			like := "%" + kw + "%"
			tq = tq.Where("service.name LIKE ? OR service.code LIKE ? OR service.code_old LIKE ?", like, like, like)
		}
		return tq
	}

	buildTotalsQ().Select(`COUNT(order_service_used.id) as numService,
		SUM(quantity) as quantity,
		SUM(totalEnd) as totalEnd,
		SUM(payment) as payment,
		SUM(CASE WHEN service_discount_type = 2 THEN (service_discount * unit_price * quantity / 100) ELSE service_discount END) as service_discount_amount,
		SUM(discount_other) as discount_other,
		SUM(rank_discount_amount) as rank_discount_amount,
		SUM(refundServiceUsed) as refundServiceUsed,
		COUNT(DISTINCT order_service_used.customer_id) as customerCount`).
		Scan(&totals)

	// Tổng số NHÓM = số dịch vụ phân biệt. `pagination.Paginate` bọc NGUYÊN câu (13 cột tổng +
	// GROUP BY + ORDER BY) vào `COUNT(*) FROM (…)`, tức phải dựng lại trọn bảng dẫn xuất chỉ để
	// đếm số dòng của nó. `COUNT(DISTINCT service_id)` trên cùng bộ lọc cho ĐÚNG con số đó mà
	// không aggregate gì. Đo ca không lọc ngày @898.720 dòng (2026-09-09):
	// COUNT dẫn xuất 33,0–71,8s (sort 782.568 dòng) -> 1,77–1,88s (sort 0) ⇒ ~19-40x.
	// Đối chiếu số nhóm trên 7 bộ lọc: 536/131/0/293/1/0/371 — khớp tuyệt đối hai cách.
	var totalGroups int64
	buildTotalsQ().Select("COUNT(DISTINCT order_service_used.service_id)").Scan(&totalGroups)

	// Paginate
	var items []map[string]interface{}
	result := pagination.PaginateWithTotal(c, q, &items, totalGroups)

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

func (h *Handler) legacyBuildServiceTypeStatsQuery(c *gin.Context) *gorm.DB {
	q := tenant.ScopedDBNullBranch(c, h.DB, "order_service_used", "osu").
		Table("order_service_used osu").
		Joins("LEFT JOIN service ON service.id = osu.service_id").
		Joins("LEFT JOIN service_type st ON st.id = service.service_type_id").
		Where("osu.isLock = 1").
		Where("st.id IS NOT NULL")
	return query.ApplyFilter(c, q, "created_day", "osu.created_day")
}

func (h *Handler) legacyServiceTypeStatistics(c *gin.Context) {
	q := h.legacyBuildServiceTypeStatsQuery(c).
		Select(`st.id, st.name,
			COUNT(DISTINCT osu.customer_id) AS customerCount,
			COUNT(osu.id) AS numService,
			SUM(osu.quantity) AS quantity,
			SUM(osu.totalEnd) AS totalEnd,
			SUM(osu.payment) AS payment,
			SUM(CASE WHEN osu.service_discount_type = 2 THEN (osu.service_discount * osu.unit_price * osu.quantity / 100) ELSE osu.service_discount END) AS service_discount_amount,
			SUM(osu.discount_other) AS discount_other,
			SUM(osu.rank_discount_amount) AS rank_discount_amount,
			SUM(osu.refundServiceUsed) AS refundServiceUsed`).
		Group("st.id, st.name").Order("SUM(osu.totalEnd) DESC")

	var rows []map[string]interface{}
	tx := q.Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	// Alias của SUM(decimal)/biểu thức → []uint8 → JSON ra string; alias không thuộc bảng nào
	// nên phải liệt kê tường minh. `quantity` = SUM(int) — MySQL cũng trả DECIMAL nên ra string qua
	// cùng đường (story-34 F-E, đã audit + duyệt vá — FE trước đó tự coerce `+t.quantity`).
	// numService/customerCount = COUNT thuần → BIGINT, không cần parse.
	for _, r := range rows {
		helpers.ParseDecimalFields(r, "totalEnd", "payment", "service_discount_amount",
			"discount_other", "rank_discount_amount", "refundServiceUsed", "quantity")
	}

	// customerCount total — riêng biệt để tránh double-count khách mua nhiều loại DV
	var tCustomerCount int64
	h.legacyBuildServiceTypeStatsQuery(c).
		Select("COUNT(DISTINCT osu.customer_id)").
		Scan(&tCustomerCount)

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
