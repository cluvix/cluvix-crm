package export

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/export"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tz"
)

// orderCol định nghĩa 1 cột export cho các report trong order.go. key khớp 1-1 với
// FE displayCols[].value để FE gửi subset qua ?columns=col1,col2.
type orderCol struct {
	key     string
	header  string
	extract func(row map[string]interface{}) interface{}
}

// filterOrderCols lọc allCols theo query ?columns= (giữ nguyên thứ tự allCols).
// Không có param hoặc không cột nào khớp → trả full allCols (backward-compat: export all).
func filterOrderCols(c *gin.Context, allCols []orderCol) []orderCol {
	colParam := c.Query("columns")
	if colParam == "" {
		return allCols
	}
	selected := map[string]bool{}
	for _, k := range strings.Split(colParam, ",") {
		selected[strings.TrimSpace(k)] = true
	}
	var filtered []orderCol
	for _, col := range allCols {
		if selected[col.key] {
			filtered = append(filtered, col)
		}
	}
	if len(filtered) == 0 {
		return allCols
	}
	return filtered
}

// osuPayableOf — phải thu của 1 dòng dịch vụ (bmad/order-vat): totalEnd (TRƯỚC thuế) + vat_amount. Phiếu thu là
// tiền CÓ thuế ⇒ mọi so sánh với đã thu (trạng thái thanh toán, còn nợ) dùng số này, không dùng totalEnd.
func osuPayableOf(totalEnd, vat interface{}) decimal.Decimal {
	return helpers.ToDecimal(totalEnd).Add(helpers.ToDecimal(vat))
}

// osuPaymentStatusLabel — nhãn "Trạng thái thanh toán" của 1 dòng dịch vụ theo phải thu (có VAT) và đã thu. So
// bằng decimal (bản cũ so float64 — số lẻ có thể lệch). Dòng không VAT ⇒ nhãn y như trước.
func osuPaymentStatusLabel(payable, paid decimal.Decimal) string {
	switch {
	case payable.IsZero():
		return "Dịch vụ miễn phí"
	case paid.IsZero():
		return "Chưa thanh toán"
	case payable.Equal(paid):
		return "Hoàn thành thanh toán"
	default:
		return "Thanh toán 1 phần"
	}
}

// orderExport build headers + row từ cols đã chọn rồi chạy doExport.
func orderExport(c *gin.Context, eq *gorm.DB, fileName string, cols []orderCol) {
	headers := make([]string, len(cols))
	for i, col := range cols {
		headers[i] = col.header
	}
	url, err := doExport(c, eq, export.Config{
		FileName:    fileName,
		Headers:     headers,
		ExportDir:   "./storage/export/",
		DownloadURL: DownloadURL,
	}, func(row map[string]interface{}) []interface{} {
		data := make([]interface{}, len(cols))
		for i, col := range cols {
			data[i] = col.extract(row)
		}
		return data
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, url)
}

func ExportOrderPlan(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	totalByPlanID := loadSumByFilteredIDs(q, "order_plan.id", "order_service_used", "order_plan_id", "totalEnd")
	// VAT đơn (bmad/order-vat): giá trị đơn giữ TRƯỚC thuế; phải thu = giá trị + VAT — "Còn nợ" so phải thu
	// với đã thu (paymentPlan là tiền CÓ thuế).
	vatByPlanID := loadSumByFilteredIDs(q, "order_plan.id", "order_service_used", "order_plan_id", "vat_amount")
	nameByID := loadNameMap(q, "user_profile", "user_id", "fullname")
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))

	eq := q.Session(&gorm.Session{}).
		Select(`order_plan.id as plan_id,
			customer.fullname as customer_fullname, customer.code as customer_code, customer.old_code as customer_old_code,
			customer.phone as customer_phone,
			order_plan.code,
			CASE order_plan.orderPlanType WHEN 1 THEN 'Đơn hàng mới' ELSE 'Đơn hàng cũ' END as order_plan_type,
			br.name as branch_name,
			order_plan.paymentPlan as payment,
			order_plan.listRevenues,
			order_plan.created_time, order_plan.created_at, order_plan.updated_at,
			uc.fullname as user_created_fullname, uu.fullname as user_updated_fullname`).
		Joins("LEFT JOIN customer ON customer.id = order_plan.customer_id").
		Joins("LEFT JOIN branch br ON br.id = order_plan.branch_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = order_plan.created_by").
		Joins("LEFT JOIN user_profile uu ON uu.user_id = order_plan.updated_by")

	allCols := []orderCol{
		{"id", "ID", func(r map[string]interface{}) interface{} { return r["plan_id"] }},
		{"customer_code", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"fullname", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"code", "Mã đơn hàng", func(r map[string]interface{}) interface{} { return r["code"] }},
		// Tiền tính bằng decimal; .InexactFloat64() chỉ ở ô Excel (kiểu số của Excel là double) — decimal.Decimal
		// ghi thẳng vào ô sẽ thành CHUỖI (excelize fmt.Sprint), mất khả năng tính toán trong Excel.
		{"total", "Giá trị đơn hàng", func(r map[string]interface{}) interface{} {
			return totalByPlanID[toInt(r["plan_id"])].InexactFloat64()
		}},
		{"vat_total", "Tiền VAT", func(r map[string]interface{}) interface{} {
			return vatByPlanID[toInt(r["plan_id"])].InexactFloat64()
		}},
		{"total_after_vat", "Tổng sau VAT", func(r map[string]interface{}) interface{} {
			id := toInt(r["plan_id"])
			return totalByPlanID[id].Add(vatByPlanID[id]).InexactFloat64()
		}},
		{"payment", "Đã thanh toán", func(r map[string]interface{}) interface{} { return toFloat(r["payment"]) }},
		{"report-debt", "Còn nợ", func(r map[string]interface{}) interface{} {
			id := toInt(r["plan_id"])
			return totalByPlanID[id].Add(vatByPlanID[id]).Sub(helpers.ToDecimal(r["payment"])).InexactFloat64()
		}},
		{"tuVan", "Tư vấn viên", func(r map[string]interface{}) interface{} { return firstUserName(r["listRevenues"], nameByID) }},
		{"created_time", "Ngày lên đơn", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_time"], loc) }},
		{"branch", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
		{"created_by", "Người tạo", func(r map[string]interface{}) interface{} { return r["user_created_fullname"] }},
		{"created_at", "Ngày tạo", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_at"], loc) }},
		{"updated_by", "Người cập nhật", func(r map[string]interface{}) interface{} { return r["user_updated_fullname"] }},
		{"updated_at", "Ngày cập nhật", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["updated_at"], loc) }},
	}
	orderExport(c, eq, "Don_Hang", filterOrderCols(c, allCols))
}

func ExportOrderPlanDebts(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	// `A` là LATERAL công nợ từng đơn trong base query q (teBefore/payBefore/teIn/paidIn) — xem handler
	// order_plan_debts; q đã JOIN customer cho export.
	eq := q.Session(&gorm.Session{}).
		Select(`order_plan.id, customer.code as customer_code, order_plan.code, customer.fullname as customer_fullname,
			A.teBefore - A.payBefore AS totalEndBefore,
			A.teIn AS totalEndIn,
			A.paidIn AS daThuTrongKy,
			(A.teBefore - A.payBefore) + A.teIn - A.paidIn AS totalEndOrder,
			order_plan.created_time,
			cntry.CommonName as country_name, prov.name as province_name, ward.name as ward_name, customer.address`).
		Joins("LEFT JOIN location_country cntry ON cntry.id = customer.country_id").
		Joins("LEFT JOIN location_province prov ON prov.id = customer.province_id").
		Joins("LEFT JOIN location_ward ward ON ward.id = customer.ward_id")

	allCols := []orderCol{
		{"id", "ID", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_code", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"order_plan_code", "Mã đơn hàng", func(r map[string]interface{}) interface{} { return r["code"] }},
		{"customer_name", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"P_T_D_K", "Phải thu đầu kỳ", func(r map[string]interface{}) interface{} { return toFloat(r["totalEndBefore"]) }},
		{"D_T_T_K", "Doanh thu trong kỳ", func(r map[string]interface{}) interface{} { return toFloat(r["totalEndIn"]) }},
		{"DA_T_T_K", "Đã thu trong kỳ", func(r map[string]interface{}) interface{} { return toFloat(r["daThuTrongKy"]) }},
		{"P_T_C_K", "Phải thu cuối kỳ", func(r map[string]interface{}) interface{} { return toFloat(r["totalEndOrder"]) }},
		{"created_time", "Ngày tạo đơn", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_time"], loc) }},
		{"country", "Quốc gia", func(r map[string]interface{}) interface{} { return r["country_name"] }},
		{"province", "Tỉnh thành", func(r map[string]interface{}) interface{} { return r["province_name"] }},
		{"ward", "Phường/Xã", func(r map[string]interface{}) interface{} { return r["ward_name"] }},
		{"address", "Địa chỉ", func(r map[string]interface{}) interface{} { return r["address"] }},
	}
	orderExport(c, eq, "Cong_no_don_Hang", filterOrderCols(c, allCols))
}

func ExportTelesaleList(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Select(`order_commission_telesale.id, cust.code as customer_code, cust.old_code as customer_old_code, cust.fullname as customer_fullname, cust.phone as customer_phone,
			osu.code as ose_code, svc.code as service_code, svc.name as service_name,
			ts.fullname as telesale_fullname, order_commission_telesale.amount, order_commission_telesale.commissionTelesale, order_commission_telesale.confirm_time,
			br.name as branch_name`).
		Joins("LEFT JOIN customer cust ON cust.id = order_commission_telesale.customer_id").
		Joins("LEFT JOIN order_service_used osu ON osu.id = order_commission_telesale.order_service_used_id").
		Joins("LEFT JOIN service svc ON svc.id = osu.service_id").
		Joins("LEFT JOIN user_profile ts ON ts.user_id = order_commission_telesale.revenue_id").
		Joins("LEFT JOIN branch br ON br.id = order_commission_telesale.branch_id")

	allCols := []orderCol{
		{"id", "id", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_code", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"customer_old_code", "Mã khách cũ", func(r map[string]interface{}) interface{} { return r["customer_old_code"] }},
		{"fullname", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"service_used_code", "Mã dịch vụ sử dụng", func(r map[string]interface{}) interface{} { return r["ose_code"] }},
		{"service_code", "Mã dịch vụ", func(r map[string]interface{}) interface{} { return r["service_code"] }},
		{"service", "Dịch vụ", func(r map[string]interface{}) interface{} { return r["service_name"] }},
		{"telesale", "Tư vấn viên", func(r map[string]interface{}) interface{} { return r["telesale_fullname"] }},
		{"amount", "Tiền thu", func(r map[string]interface{}) interface{} { return toFloat(r["amount"]) }},
		{"commissionTelesale", "Hoa hồng", func(r map[string]interface{}) interface{} { return toFloat(r["commissionTelesale"]) }},
		{"confirm_time", "Ngày tính hoa hồng", func(r map[string]interface{}) interface{} { return fmtUnixDate(r["confirm_time"], loc) }},
		{"branch", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
	}
	orderExport(c, eq, "Commission-Telesale-export", filterOrderCols(c, allCols))
}

func ExportTelesaleCommission(c *gin.Context, baseQ *gorm.DB) {
	eq := baseQ.Session(&gorm.Session{}).
		Select(`ts.fullname as telesale_fullname, COUNT(*) as count,
			SUM(order_commission_telesale.amount) as totalAmount,
			SUM(order_commission_telesale.commissionTelesale) as totalCommission`).
		Joins("LEFT JOIN user_profile ts ON ts.user_id = order_commission_telesale.revenue_id").
		Group("order_commission_telesale.revenue_id").
		Order("totalCommission DESC")

	url, err := doExport(c, eq, export.Config{
		FileName:  "Commission-Telesale-Summary-export",
		Headers:   []string{"Tư vấn viên", "Số lượt", "Tiền thu", "Hoa hồng"},
		ExportDir: "./storage/export/", DownloadURL: DownloadURL,
	}, func(row map[string]interface{}) []interface{} {
		return []interface{}{row["telesale_fullname"], row["count"],
			toFloat(row["totalAmount"]), toFloat(row["totalCommission"])}
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, url)
}

func ExportOrderServiceUsed(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	nameByID := loadNameMap(q, "user_profile", "user_id", "fullname")
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Select(`order_service_used.id,
			cust.code as customer_code, cust.old_code as customer_old_code, cust.fullname as customer_fullname, cust.phone as customer_phone,
			br.name as branch_name,
			otp.code as plan_code, otp.note as plan_note, order_service_used.code, svc.code as service_code, svc.name as service_name,
			CASE WHEN otp.orderPlanType = 1 THEN 'Đơn hàng mới' ELSE 'Đơn hàng cũ' END as order_plan_type,
			svt.name as service_type_name,
			order_service_used.unit_price, order_service_used.quantity,
			order_service_used.service_discount, order_service_used.service_discount_type,
			order_service_used.discount_other, order_service_used.rank_discount_amount,
			order_service_used.totalEnd, order_service_used.vat_amount, order_service_used.payment,
			order_service_used.dayComplete, order_service_used.created_day,
			order_service_used.created_at, order_service_used.updated_at,
			order_service_used.listRevenues,
			uc.fullname as user_created_fullname, uu.fullname as user_updated_fullname,
			csrc.name as customer_source_name, osrc.name as source_name,
			cust.address as customer_address, cust.year_born,
			cntry.CommonName as country_name, prov.name as province_name, ward.name as ward_name`).
		Joins("LEFT JOIN customer cust ON cust.id = order_service_used.customer_id").
		Joins("LEFT JOIN branch br ON br.id = order_service_used.branch_id").
		Joins("LEFT JOIN order_plan otp ON otp.id = order_service_used.order_plan_id").
		Joins("LEFT JOIN service svc ON svc.id = order_service_used.service_id").
		Joins("LEFT JOIN service_type svt ON svt.id = svc.service_type_id").
		Joins("LEFT JOIN source csrc ON csrc.id = cust.source_id").
		Joins("LEFT JOIN source osrc ON osrc.id = order_service_used.source_id").
		Joins("LEFT JOIN location_country cntry ON cntry.id = cust.country_id").
		Joins("LEFT JOIN location_province prov ON prov.id = cust.province_id").
		Joins("LEFT JOIN location_ward ward ON ward.id = cust.ward_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = order_service_used.created_by").
		Joins("LEFT JOIN user_profile uu ON uu.user_id = order_service_used.updated_by")

	allCols := []orderCol{
		{"id", "ID", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_code", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"customer_code_old", "Mã khách cũ", func(r map[string]interface{}) interface{} { return r["customer_old_code"] }},
		{"fullname", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"plan_code", "Mã đơn hàng", func(r map[string]interface{}) interface{} { return r["plan_code"] }},
		{"service_code_used", "Mã dịch vụ sử dụng", func(r map[string]interface{}) interface{} { return r["code"] }},
		{"service_code", "Mã dịch vụ", func(r map[string]interface{}) interface{} { return r["service_code"] }},
		{"service", "Dịch vụ", func(r map[string]interface{}) interface{} { return r["service_name"] }},
		{"orderPlanType", "Dịch vụ cũ/mới", func(r map[string]interface{}) interface{} { return r["order_plan_type"] }},
		{"serviceType", "Nhóm dịch vụ", func(r map[string]interface{}) interface{} { return r["service_type_name"] }},
		{"revenue", "Tư vấn viên", func(r map[string]interface{}) interface{} { return resolveUserNames(r["listRevenues"], nameByID) }},
		{"quantity", "Số lượng", func(r map[string]interface{}) interface{} { return toFloat(r["quantity"]) }},
		{"unit_price", "Đơn giá", func(r map[string]interface{}) interface{} { return toFloat(r["unit_price"]) }},
		{"priceQuantity", "Thành tiền", func(r map[string]interface{}) interface{} {
			return toFloat(r["unit_price"]) * toFloat(r["quantity"])
		}},
		{"service_discount", "Chiết khấu dịch vụ", func(r map[string]interface{}) interface{} { return toFloat(r["service_discount"]) }},
		{"discount_other", "Giảm giá khác", func(r map[string]interface{}) interface{} { return toFloat(r["discount_other"]) }},
		{"rank_discount_amount", "Giảm hạng thành viên", func(r map[string]interface{}) interface{} { return toFloat(r["rank_discount_amount"]) }},
		{"totalEnd", "Doanh số", func(r map[string]interface{}) interface{} { return toFloat(r["totalEnd"]) }},
		{"vat_amount", "Tiền VAT", func(r map[string]interface{}) interface{} { return toFloat(r["vat_amount"]) }},
		{"total_after_vat", "Tổng sau VAT", func(r map[string]interface{}) interface{} {
			return osuPayableOf(r["totalEnd"], r["vat_amount"]).InexactFloat64()
		}},
		{"thanhToan", "Thanh toán", func(r map[string]interface{}) interface{} { return toFloat(r["payment"]) }},
		{"statusPayment", "Trạng thái thanh toán", func(r map[string]interface{}) interface{} {
			return osuPaymentStatusLabel(osuPayableOf(r["totalEnd"], r["vat_amount"]), helpers.ToDecimal(r["payment"]))
		}},
		{"note", "Ghi chú", func(r map[string]interface{}) interface{} { return stripHTML(r["plan_note"]) }},
		{"dayComplete", "Ngày hoàn thành sử dụng", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["dayComplete"], loc) }},
		{"source", "Nguồn khách hàng", func(r map[string]interface{}) interface{} { return r["customer_source_name"] }},
		{"sourceApp", "Nguồn doanh số", func(r map[string]interface{}) interface{} { return r["source_name"] }},
		{"address", "Địa chỉ", func(r map[string]interface{}) interface{} { return r["customer_address"] }},
		{"year", "Năm sinh", func(r map[string]interface{}) interface{} { return r["year_born"] }},
		{"province", "Tỉnh/Thành", func(r map[string]interface{}) interface{} { return r["province_name"] }},
		{"ward", "Phường/xã", func(r map[string]interface{}) interface{} { return r["ward_name"] }},
		{"country", "Quốc gia", func(r map[string]interface{}) interface{} { return r["country_name"] }},
		{"created_time", "Ngày tạo dịch vụ", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_day"], loc) }},
		{"branch_id", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
		{"created_by", "Người tạo", func(r map[string]interface{}) interface{} { return r["user_created_fullname"] }},
		{"created_at", "Ngày tạo", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_at"], loc) }},
		{"updated_by", "Người cập nhật", func(r map[string]interface{}) interface{} { return r["user_updated_fullname"] }},
		{"updated_at", "Ngày cập nhật", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["updated_at"], loc) }},
	}
	orderExport(c, eq, "Dich_vu_su_dung-export", filterOrderCols(c, allCols))
}

func ExportOrderServiceUsedWithAppt(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	nameByID := loadNameMap(q, "user_profile", "user_id", "fullname")
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Select(`order_service_used.id,
			cust.code as customer_code, cust.fullname as customer_fullname, cust.phone as customer_phone,
			apt.name as appointment_type_name,
			br.name as branch_name,
			order_service_used.code, svc.name as service_name,
			order_service_used.unit_price, order_service_used.quantity,
			order_service_used.service_discount, order_service_used.service_discount_type,
			order_service_used.discount_other, order_service_used.rank_discount_amount,
			order_service_used.totalEnd, order_service_used.vat_amount, order_service_used.payment,
			order_service_used.listRevenues,
			osrc.name as source_name, cust.address as customer_address,
			order_service_used.created_day, order_service_used.created_at, uc.fullname as user_created_fullname`).
		Joins("LEFT JOIN customer cust ON cust.id = order_service_used.customer_id").
		Joins("LEFT JOIN appointment appt ON appt.id = order_service_used.appointment_id").
		Joins("LEFT JOIN appointment_type apt ON apt.id = appt.appointment_type_id").
		Joins("LEFT JOIN branch br ON br.id = order_service_used.branch_id").
		Joins("LEFT JOIN service svc ON svc.id = order_service_used.service_id").
		Joins("LEFT JOIN source osrc ON osrc.id = order_service_used.source_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = order_service_used.created_by")

	allCols := []orderCol{
		{"id", "ID", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_code", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"fullname", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"appointmentType", "Loại lịch hẹn", func(r map[string]interface{}) interface{} { return r["appointment_type_name"] }},
		{"service_code_used", "Mã dịch vụ sử dụng", func(r map[string]interface{}) interface{} { return r["code"] }},
		{"service", "Dịch vụ", func(r map[string]interface{}) interface{} { return r["service_name"] }},
		{"revenue", "Tư vấn viên", func(r map[string]interface{}) interface{} { return resolveUserNames(r["listRevenues"], nameByID) }},
		{"quantity", "Số lượng", func(r map[string]interface{}) interface{} { return toFloat(r["quantity"]) }},
		{"unit_price", "Đơn giá", func(r map[string]interface{}) interface{} { return toFloat(r["unit_price"]) }},
		{"priceQuantity", "Thành tiền", func(r map[string]interface{}) interface{} {
			return toFloat(r["unit_price"]) * toFloat(r["quantity"])
		}},
		{"service_discount", "Chiết khấu dịch vụ", func(r map[string]interface{}) interface{} { return toFloat(r["service_discount"]) }},
		{"discount_other", "Giảm giá khác", func(r map[string]interface{}) interface{} { return toFloat(r["discount_other"]) }},
		{"rank_discount_amount", "Giảm hạng thành viên", func(r map[string]interface{}) interface{} { return toFloat(r["rank_discount_amount"]) }},
		{"totalEnd", "Doanh số", func(r map[string]interface{}) interface{} { return toFloat(r["totalEnd"]) }},
		{"vat_amount", "Tiền VAT", func(r map[string]interface{}) interface{} { return toFloat(r["vat_amount"]) }},
		{"total_after_vat", "Tổng sau VAT", func(r map[string]interface{}) interface{} {
			return osuPayableOf(r["totalEnd"], r["vat_amount"]).InexactFloat64()
		}},
		{"thanhToan", "Thanh toán", func(r map[string]interface{}) interface{} { return toFloat(r["payment"]) }},
		{"source_id", "Nguồn dịch vụ", func(r map[string]interface{}) interface{} { return r["source_name"] }},
		{"address", "Địa chỉ", func(r map[string]interface{}) interface{} { return r["customer_address"] }},
		{"created_day", "Ngày tạo dịch vụ", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_day"], loc) }},
		{"branch_id", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
		{"created_by", "Người tạo", func(r map[string]interface{}) interface{} { return r["user_created_fullname"] }},
		{"created_at", "Ngày tạo", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_at"], loc) }},
	}
	orderExport(c, eq, "Dich_vu_su_dung-export", filterOrderCols(c, allCols))
}

// --- order_revenue_telesale / doctor_consultant / direct_sale (doanh số, ghi lúc lên đơn) ---

func ExportRevenueTelesale(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Select(`order_revenue_telesale.id, cust.code as customer_code, cust.old_code as customer_old_code, cust.fullname as customer_fullname, cust.phone as customer_phone,
			osu.code as ose_code, svc.code as service_code, svc.name as service_name,
			ts.fullname as telesale_fullname, order_revenue_telesale.amount, order_revenue_telesale.revenue, order_revenue_telesale.confirm_time,
			br.name as branch_name`).
		Joins("LEFT JOIN customer cust ON cust.id = order_revenue_telesale.customer_id").
		Joins("LEFT JOIN order_service_used osu ON osu.id = order_revenue_telesale.order_service_used_id").
		Joins("LEFT JOIN service svc ON svc.id = osu.service_id").
		Joins("LEFT JOIN user_profile ts ON ts.user_id = order_revenue_telesale.revenue_id").
		Joins("LEFT JOIN branch br ON br.id = order_revenue_telesale.branch_id")

	allCols := []orderCol{
		{"id", "id", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_code", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"customer_old_code", "Mã khách cũ", func(r map[string]interface{}) interface{} { return r["customer_old_code"] }},
		{"fullname", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"service_used_code", "Mã dịch vụ sử dụng", func(r map[string]interface{}) interface{} { return r["ose_code"] }},
		{"service_code", "Mã dịch vụ", func(r map[string]interface{}) interface{} { return r["service_code"] }},
		{"service", "Dịch vụ", func(r map[string]interface{}) interface{} { return r["service_name"] }},
		{"telesale", "Tư vấn viên", func(r map[string]interface{}) interface{} { return r["telesale_fullname"] }},
		{"amount", "Tiền thu", func(r map[string]interface{}) interface{} { return toFloat(r["amount"]) }},
		{"revenue", "Doanh số", func(r map[string]interface{}) interface{} { return toFloat(r["revenue"]) }},
		{"confirm_time", "Ngày ghi nhận", func(r map[string]interface{}) interface{} { return fmtUnixDate(r["confirm_time"], loc) }},
		{"branch", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
	}
	orderExport(c, eq, "Doanh-So-Telesale-export", filterOrderCols(c, allCols))
}

// --- Tổng hợp doanh số theo user (GROUP BY) ---

func ExportRevenueTelesaleSummary(c *gin.Context, baseQ *gorm.DB) {
	eq := baseQ.Session(&gorm.Session{}).
		Select(`ts.fullname as telesale_fullname, COUNT(*) as count,
			SUM(order_revenue_telesale.revenue) as totalRevenue`).
		Joins("LEFT JOIN user_profile ts ON ts.user_id = order_revenue_telesale.revenue_id").
		Group("order_revenue_telesale.revenue_id").
		Order("totalRevenue DESC")

	url, err := doExport(c, eq, export.Config{
		FileName:  "Doanh-So-Telesale-Tong-Hop-export",
		Headers:   []string{"Tư vấn viên", "Số lượt", "Doanh số"},
		ExportDir: "./storage/export/", DownloadURL: DownloadURL,
	}, func(row map[string]interface{}) []interface{} {
		return []interface{}{row["telesale_fullname"], row["count"], toFloat(row["totalRevenue"])}
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, url)
}
