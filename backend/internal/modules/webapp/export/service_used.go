package export

// Export thống kê dịch vụ đã dùng (order_service_used) — crm-standalone story-14 tách từ treatment.go
// (các export liệu trình / bác sĩ / phụ tá / KTV / hoa hồng đã bỏ cùng liệu trình, D1 + D7).

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/pkg/export"
	"erpCluvix/pkg/response"
)

// exportCol {key, header, extract} — colDef dùng chung cho export trong file này.
type exportCol struct {
	key     string
	header  string
	extract func(row map[string]interface{}) interface{}
}

// filterExportCols giữ backward-compat: không có ?columns= → trả full colDef; có param →
// lọc theo key (khớp 1-1 với FE displayCols[].value), rỗng sau lọc thì fallback full.
// // coreKeys: các cột UI hiển thị luôn (không có trong FE displayCols toggle, không bọc
// isColVisible trong template) — LUÔN giữ trong export bất kể user chọn gì ở
// ExportOptionsDialog, để tránh export thiếu cột mà bảng đang hiển thị. Xem gotcha
// "core vs optional colDef" trong .skill/backend/modules/ops/export.md.
func filterExportCols(all []exportCol, colParam string, coreKeys ...string) []exportCol {
	if colParam == "" {
		return all
	}
	selected := map[string]bool{}
	for _, k := range strings.Split(colParam, ",") {
		selected[strings.TrimSpace(k)] = true
	}
	for _, k := range coreKeys {
		selected[k] = true
	}
	var filtered []exportCol
	for _, col := range all {
		if selected[col.key] {
			filtered = append(filtered, col)
		}
	}
	if len(filtered) == 0 {
		return all
	}
	return filtered
}

// exportHeadersRow build headers + rowFn cho doExport từ colDef đã lọc.
func exportHeadersRow(cols []exportCol) ([]string, func(map[string]interface{}) []interface{}) {
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.header
	}
	rowFn := func(row map[string]interface{}) []interface{} {
		data := make([]interface{}, len(cols))
		for i, c := range cols {
			data[i] = c.extract(row)
		}
		return data
	}
	return headers, rowFn
}

// treatmentServicesCols định nghĩa colDef cho ExportTreatmentServices (thống kê dịch vụ),
// key khớp FE crm-report-service-used.ts displayCols[].value.
func treatmentServicesCols() []exportCol {
	return []exportCol{
		{"id", "id", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"code", "Mã dịch vụ", func(r map[string]interface{}) interface{} { return r["service_code"] }},
		{"service", "Dịch vụ", func(r map[string]interface{}) interface{} { return r["service_name"] }},
		{"groupService", "Nhóm dịch vụ", func(r map[string]interface{}) interface{} { return r["service_type_name"] }},
		{"customerCount", "Số khách hàng", func(r map[string]interface{}) interface{} { return r["customerCount"] }},
		{"numService", "Số lượng dịch vụ", func(r map[string]interface{}) interface{} { return r["numService"] }},
		{"numQuantity", "Số đơn vị sử dụng", func(r map[string]interface{}) interface{} { return r["quantity"] }},
		{"totalTotalEnd", "Doanh số", func(r map[string]interface{}) interface{} { return toFloat(r["totalEnd"]) }},
		// VAT (bmad/order-vat): Doanh số giữ TRƯỚC thuế; Thanh toán là tiền CÓ thuế ⇒ thêm cột VAT để đối chiếu.
		{"totalVat", "Tiền VAT", func(r map[string]interface{}) interface{} { return toFloat(r["vat_amount"]) }},
		{"totalPayment", "Thanh toán", func(r map[string]interface{}) interface{} { return toFloat(r["payment"]) }},
		{"totalDiscountService", "Giảm dịch vụ", func(r map[string]interface{}) interface{} { return toFloat(r["service_discount_amount"]) }},
		{"totalDiscountOther", "Giảm khác", func(r map[string]interface{}) interface{} { return toFloat(r["discount_other"]) }},
		{"rank_discount_amount", "Giảm hạng thành viên", func(r map[string]interface{}) interface{} { return toFloat(r["rank_discount_amount"]) }},
		{"totalRefundServiceUsed", "Hoàn dịch vụ", func(r map[string]interface{}) interface{} { return toFloat(r["refundServiceUsed"]) }},
		{"mediumService", "Trung bình", func(r map[string]interface{}) interface{} {
			numService := toFloat(r["numService"])
			if numService == 0 {
				return float64(0)
			}
			return toFloat(r["totalEnd"]) / numService
		}},
	}
}

func ExportTreatmentServices(c *gin.Context, q *gorm.DB) {
	// q from report handler already has SELECT + JOINs + GROUP BY
	// Add service_type_name via JOIN
	eq := q.Session(&gorm.Session{}).
		Select(`order_service_used.service_id as id,
			service.name as service_name, service.code as service_code,
			service_type.name as service_type_name,
			COUNT(DISTINCT order_service_used.customer_id) AS customerCount,
			COUNT(order_service_used.id) as numService,
			SUM(quantity) as quantity,
			SUM(totalEnd) as totalEnd,
			SUM(order_service_used.vat_amount) as vat_amount,
			SUM(payment) as payment,
			SUM(CASE WHEN service_discount_type = 2 THEN (service_discount * unit_price * quantity / 100) ELSE service_discount END) as service_discount_amount,
			SUM(discount_other) as discount_other,
			SUM(rank_discount_amount) as rank_discount_amount,
			SUM(refundServiceUsed) as refundServiceUsed`).
		Group("order_service_used.service_id")

	cols := filterExportCols(treatmentServicesCols(), c.Query("columns"))
	headers, rowFn := exportHeadersRow(cols)

	url, err := doExport(c, eq, export.Config{
		FileName:    "Thong_ke",
		Headers:     headers,
		ExportDir:   "./storage/export/",
		DownloadURL: DownloadURL,
	}, rowFn)
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, url)
}
