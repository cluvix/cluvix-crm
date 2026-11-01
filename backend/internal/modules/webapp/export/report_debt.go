package export

// Export báo cáo công nợ CRM (crm/report/debt) — port từ ERP ExportTreatmentDebts (công nợ điều trị →
// "công nợ theo dịch vụ", bỏ cột buổi điều trị/% hoàn thành/thương hiệu).

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/export"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tz"
)

// serviceDebtExportCols — key khớp FE crm/report/debt/service-debts displayCols[].value.
func serviceDebtExportCols(loc *time.Location, showPhone bool) []exportCol {
	return []exportCol{
		{"id", "ID", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_id", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"customer_number", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"customer_old_code", "Mã khách cũ", func(r map[string]interface{}) interface{} { return r["customer_old_code"] }},
		{"phone", "Điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"serviceUsedCode", "Mã dịch vụ sử dụng", func(r map[string]interface{}) interface{} { return r["code"] }},
		{"serviceName", "Dịch vụ", func(r map[string]interface{}) interface{} { return r["service_name"] }},
		{"serviceUsedCreate", "Ngày lên đơn", func(r map[string]interface{}) interface{} { return fmtUnixDate(r["created_day"], loc) }},
		{"serviceIncurred", "Phát sinh", func(r map[string]interface{}) interface{} { return toFloat(r["totalEnd"]) }},
		{"servicePaid", "Thanh toán", func(r map[string]interface{}) interface{} { return toFloat(r["payment"]) }},
		{"serviceDebt", "Công nợ", func(r map[string]interface{}) interface{} {
			// Tính bằng decimal; .InexactFloat64() chỉ ở ô Excel (kiểu số của Excel là double).
			return helpers.ToDecimal(r["totalEnd"]).Sub(helpers.ToDecimal(r["payment"])).InexactFloat64()
		}},
		{"customerDeposit", "Cọc còn lại của khách", func(r map[string]interface{}) interface{} { return toFloat(r["totalDeposit"]) }},
		{"branch_id", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
	}
}

// ExportServiceDebts — q đã có FROM order_service_used + LEFT JOIN customer + JOIN service + WHERE + ORDER của handler.
func ExportServiceDebts(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Select(`order_service_used.id, order_service_used.code,
			order_service_used.totalEnd, order_service_used.payment, order_service_used.created_day,
			customer.code AS customer_code, customer.old_code AS customer_old_code,
			customer.fullname AS customer_fullname, customer.phone AS customer_phone, customer.totalDeposit,
			service.name AS service_name, br.name AS branch_name`).
		Joins("LEFT JOIN branch br ON br.id = order_service_used.branch_id")

	// Phát sinh/Thanh toán/Công nợ/Cọc render cứng trên màn — luôn giữ trong export.
	cols := filterExportCols(serviceDebtExportCols(loc, showPhone), c.Query("columns"),
		"serviceIncurred", "servicePaid", "serviceDebt", "customerDeposit")
	headers, rowFn := exportHeadersRow(cols)
	url, err := doExport(c, eq, export.Config{
		FileName:    "Cong_no_dich_vu",
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
