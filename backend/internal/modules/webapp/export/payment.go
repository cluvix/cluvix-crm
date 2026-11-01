package export

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/export"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tz"
)

type paymentReceiptCol struct {
	key     string
	header  string
	extract func(row map[string]interface{}) interface{}
}

type paymentCol struct {
	key     string
	header  string
	extract func(row map[string]interface{}) interface{}
}

// paymentGenCol is the generic column definition shared by the deposit/refund exports
// (ExportPaymentDeposit, ExportPaymentRefundDeposit, ExportPaymentRefundOrderPlan) — key
// khớp 1-1 với FE displayCols[].value cho column-selection qua ?columns=.
type paymentGenCol struct {
	key     string
	header  string
	extract func(row map[string]interface{}) interface{}
}

// resolveUserNames parse cột JSON array id ([1,2,3]) từ row và map sang fullname
// qua nameByID đã preload, join bằng ", ".
func resolveUserNames(raw interface{}, nameByID map[int]string) string {
	var b []byte
	switch v := raw.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return ""
	}
	if len(b) == 0 {
		return ""
	}
	var ids []int
	if err := json.Unmarshal(b, &ids); err != nil {
		return ""
	}
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		if n, ok := nameByID[id]; ok && n != "" {
			names = append(names, n)
		}
	}
	return strings.Join(names, ", ")
}

// firstUserName parse cột JSON array id ([1,2,3]) từ row và trả về tên (qua nameByID) của
// phần tử đầu tiên — dùng cho cột chỉ hiển thị 1 người đại diện (vd tư vấn viên = listRevenues[0]).
func firstUserName(raw interface{}, nameByID map[int]string) string {
	var b []byte
	switch v := raw.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return ""
	}
	if len(b) == 0 {
		return ""
	}
	var ids []int
	if err := json.Unmarshal(b, &ids); err != nil || len(ids) == 0 {
		return ""
	}
	return nameByID[ids[0]]
}

// paymentColsFor defines all exportable columns for ExportPayment (ListPayment screen),
// keyed by the FE selectedColumns value so the FE can request a subset via ?columns=.
// loc dùng cho các cột format ngày/giờ theo timezone company đang active.
// nameByID dùng để resolve các cột tư vấn viên/bác sĩ/direct sale (JSON array id).
// showPhone (option 20 — CanSeePhoneExport) quyết định có mask SĐT khách hàng hay không.
func paymentColsFor(c *gin.Context, loc *time.Location, nameByID map[int]string, showPhone bool) []paymentCol {
	return []paymentCol{
		{"id", "ID", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_code", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"customer_old_code", "Mã khách cũ", func(r map[string]interface{}) interface{} { return r["customer_old_code"] }},
		{"customer_id", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"payment_number", "Mã phiếu", func(r map[string]interface{}) interface{} { return r["code"] }},
		{"orderCode", "Mã đơn hàng", func(r map[string]interface{}) interface{} { return r["plan_code"] }},
		{"serviceUsedCode", "Mã dịch vụ sử dụng", func(r map[string]interface{}) interface{} { return r["ose_code"] }},
		{"serviceCode", "Mã dịch vụ", func(r map[string]interface{}) interface{} { return r["ose_service_code"] }},
		{"service", "Dịch vụ", func(r map[string]interface{}) interface{} { return r["ose_service_name"] }},
		{"quantity", "Số lượng", func(r map[string]interface{}) interface{} { return r["ose_quantity"] }},
		{"totalEnd", "Thành tiền", func(r map[string]interface{}) interface{} { return toFloat(r["ose_totalEnd"]) }},
		{"vat_amount", "Tiền VAT", func(r map[string]interface{}) interface{} { return toFloat(r["ose_vat_amount"]) }},
		{"total_after_vat", "Tổng sau VAT", func(r map[string]interface{}) interface{} {
			return osuPayableOf(r["ose_totalEnd"], r["ose_vat_amount"]).InexactFloat64()
		}},
		{"serviceType", "Nhóm dịch vụ", func(r map[string]interface{}) interface{} { return r["service_type_name"] }},
		{"statusPayment", "Trạng thái thanh toán", func(r map[string]interface{}) interface{} {
			// Phải thu = totalEnd + VAT (bmad/order-vat): dòng có VAT mà đã thu = totalEnd là MỚI thu 1 phần.
			return osuPaymentStatusLabel(osuPayableOf(r["ose_totalEnd"], r["ose_vat_amount"]), helpers.ToDecimal(r["ose_payment"]))
		}},
		{"type", "Loại thanh toán", func(r map[string]interface{}) interface{} { return r["paymentFor"] }},
		{"method", "Hình thức", func(r map[string]interface{}) interface{} { return r["payment_method_name"] }},
		{"cardType", "Hình thức TT", func(r map[string]interface{}) interface{} { return r["card_type_name"] }},
		{"amount", "Số tiền", func(r map[string]interface{}) interface{} { return toFloat(r["amount"]) }},
		{"code_old", "Mã phiếu thu gốc", func(r map[string]interface{}) interface{} { return r["deposit_code"] }},
		{"content_old", "Nội dung", func(r map[string]interface{}) interface{} { return stripHTML(r["note"]) }},
		{"tuVan", "Tư vấn viên", func(r map[string]interface{}) interface{} { return resolveUserNames(r["listRevenues"], nameByID) }},
		{"source", "Nguồn khách hàng", func(r map[string]interface{}) interface{} { return r["customer_source_name"] }},
		{"sourceApp", "Nguồn thanh toán", func(r map[string]interface{}) interface{} { return r["source_name"] }},
		{"serviceInterested", "Nhóm dịch vụ quan tâm", func(r map[string]interface{}) interface{} { return r["customer_service_type_name"] }},
		{"province", "Tỉnh thành", func(r map[string]interface{}) interface{} { return r["customer_province_name"] }},
		{"country", "Quốc gia", func(r map[string]interface{}) interface{} { return r["customer_country_name"] }},
		{"createdOrder", "Ngày lên đơn", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["ose_created_day"], loc) }},
		{"payment_of_date", "Ngày thanh toán", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["payment_of_date"], loc) }},
		{"payment_time", "Ngày thu tiền", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["payment_time"], loc) }},
		{"branch_id", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
		{"address", "Địa chỉ khách hàng", func(r map[string]interface{}) interface{} { return r["customer_address"] }},
		{"created_at", "Ngày tạo", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_at"], loc) }},
		{"created_by", "Người tạo", func(r map[string]interface{}) interface{} { return r["user_created_fullname"] }},
		{"updated_by", "Người cập nhật", func(r map[string]interface{}) interface{} { return r["user_updated_fullname"] }},
		{"updated_at", "Ngày cập nhật", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["updated_at"], loc) }},
	}
}

// paymentReceiptColsFor defines all exportable columns for ExportPaymentReceipts,
// keyed by the FE selectedColumns value so the FE can request a subset via ?columns=.
// loc dùng cho các cột format ngày/giờ theo timezone company đang active.
// showPhone (option 20 — CanSeePhoneExport) quyết định có mask SĐT khách hàng hay không.
func paymentReceiptColsFor(loc *time.Location, showPhone bool) []paymentReceiptCol {
	return []paymentReceiptCol{
		{"id", "ID", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_code", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"customer_old_code", "Mã khách cũ", func(r map[string]interface{}) interface{} { return r["customer_old_code"] }},
		{"customer_id", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"payment_number", "Mã thu/chi", func(r map[string]interface{}) interface{} { return r["code"] }},
		{"serviceUsedCode", "Mã dịch vụ sử dụng", func(r map[string]interface{}) interface{} { return r["ose_code"] }},
		{"service", "Dịch vụ", func(r map[string]interface{}) interface{} { return r["service_name"] }},
		{"quantity", "Số lượng", func(r map[string]interface{}) interface{} { return r["ose_quantity"] }},
		{"totalEnd", "Thành tiền", func(r map[string]interface{}) interface{} { return toFloat(r["ose_totalEnd"]) }},
		{"vat_amount", "Tiền VAT", func(r map[string]interface{}) interface{} { return toFloat(r["ose_vat_amount"]) }},
		{"total_after_vat", "Tổng sau VAT", func(r map[string]interface{}) interface{} {
			return osuPayableOf(r["ose_totalEnd"], r["ose_vat_amount"]).InexactFloat64()
		}},
		{"serviceType", "Nhóm dịch vụ", func(r map[string]interface{}) interface{} { return r["service_type_name"] }},
		{"type", "Loại thanh toán", func(r map[string]interface{}) interface{} { return r["payment_for_name"] }},
		{"method", "Hình thức", func(r map[string]interface{}) interface{} { return r["payment_method_name"] }},
		{"cardType", "Hình thức TT", func(r map[string]interface{}) interface{} { return r["card_type_name"] }},
		{"typeDepositLabel", "Loại đặt cọc", func(r map[string]interface{}) interface{} { return r["deposit_type_name"] }},
		{"amount", "Tiền thu/chi", func(r map[string]interface{}) interface{} { return toFloat(r["amount"]) }},
		{"note", "Ghi chú", func(r map[string]interface{}) interface{} { return stripHTML(r["note"]) }},
		{"sourceApp", "Nguồn thanh toán", func(r map[string]interface{}) interface{} { return r["source_name"] }},
		{"source", "Nguồn khách hàng", func(r map[string]interface{}) interface{} { return r["customer_source_name"] }},
		{"serviceInterested", "Nhóm dịch vụ quan tâm", func(r map[string]interface{}) interface{} { return r["customer_service_type_name"] }},
		{"province", "Tỉnh thành", func(r map[string]interface{}) interface{} { return r["customer_province_name"] }},
		{"country", "Quốc gia", func(r map[string]interface{}) interface{} { return r["customer_country_name"] }},
		{"payment_time", "Ngày thu tiền", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["payment_time"], loc) }},
		{"branch_id", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
		{"address", "Địa chỉ khách hàng", func(r map[string]interface{}) interface{} { return r["customer_address"] }},
		{"created_by", "Người tạo", func(r map[string]interface{}) interface{} { return r["user_created_fullname"] }},
		{"created_at", "Ngày tạo", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_at"], loc) }},
		{"updated_by", "Người cập nhật", func(r map[string]interface{}) interface{} { return r["user_updated_fullname"] }},
		{"updated_at", "Ngày cập nhật", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["updated_at"], loc) }},
	}
}

// paymentJoins adds extra JOINs needed by payment export queries.
// Note: customer and order_plan are already joined by buildPaymentFilters.
func paymentJoins(q *gorm.DB) *gorm.DB {
	return q.
		Joins("LEFT JOIN order_plan otp ON otp.id = payment.parent_id").
		Joins("LEFT JOIN order_service_used ose ON ose.id = payment.serviceUsed_id").
		Joins("LEFT JOIN service svc ON svc.id = ose.service_id").
		Joins("LEFT JOIN service_type st ON st.id = svc.service_type_id").
		Joins("LEFT JOIN service_type cst ON cst.id = customer.type_service_id").
		Joins("LEFT JOIN source src ON src.id = ose.source_id").
		Joins("LEFT JOIN source csrc ON csrc.id = customer.source_id").
		Joins("LEFT JOIN location_province prov ON prov.id = customer.province_id").
		Joins("LEFT JOIN location_country ctry ON ctry.id = customer.country_id").
		Joins("LEFT JOIN branch br ON br.id = payment.branch_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = payment.created_by").
		Joins("LEFT JOIN payment dep ON dep.id = payment.deposit_id").
		Joins("LEFT JOIN payment_method pm ON pm.id = payment.method").
		Joins("LEFT JOIN card_type ct ON ct.id = payment.card_type_id")
}

func ExportPayment(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	eq := paymentJoins(q.Session(&gorm.Session{})).
		Joins("LEFT JOIN user_profile uu ON uu.user_id = payment.updated_by").
		Select(`payment.id, payment.code,
			payment.amount, payment.bank_code, payment.note,
			payment.payment_for, payment.payment_time, payment.payment_of_date,
			payment.created_at, payment.updated_at,
			customer.code as customer_code, customer.old_code as customer_old_code,
			customer.fullname as customer_fullname, customer.address as customer_address,
			customer.phone as customer_phone,
			otp.code as plan_code,
			ose.code as ose_code, svc.code as ose_service_code, svc.name as ose_service_name,
			ose.quantity as ose_quantity, ose.totalEnd as ose_totalEnd, ose.vat_amount as ose_vat_amount, ose.payment as ose_payment,
			ose.created_day as ose_created_day,
			st.name as service_type_name, cst.name as customer_service_type_name,
			src.name as source_name, csrc.name as customer_source_name,
			prov.name as customer_province_name, ctry.CommonName as customer_country_name,
			br.name as branch_name,
			uc.fullname as user_created_fullname,
			uu.fullname as user_updated_fullname,
			dep.code as deposit_code,
			pm.name as payment_method_name, ct.name as card_type_name,
			ose.listRevenues as listRevenues,
			CASE payment.payment_for
				WHEN 1 THEN 'Hoàn dịch vụ' WHEN 2 THEN 'Hoàn cọc' WHEN 3 THEN 'Đặt cọc'
				WHEN 4 THEN 'Thanh toán đơn hàng' WHEN 5 THEN 'Chuyển cọc'
				WHEN 6 THEN 'Hoàn ký gửi' WHEN 7 THEN 'Thu phòng khám' WHEN 8 THEN 'Chi phòng khám'
			END as paymentFor`)

	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	allCols := paymentColsFor(c, loc, loadNameMap(q, "user_profile", "user_id", "fullname"), showPhone)
	cols := allCols
	if colParam := c.Query("columns"); colParam != "" {
		selected := map[string]bool{}
		for _, k := range strings.Split(colParam, ",") {
			selected[strings.TrimSpace(k)] = true
		}
		var filtered []paymentCol
		for _, col := range allCols {
			if selected[col.key] {
				filtered = append(filtered, col)
			}
		}
		if len(filtered) > 0 {
			cols = filtered
		}
	}

	headers := make([]string, len(cols))
	for i, col := range cols {
		headers[i] = col.header
	}

	url, err := doExport(c, eq, export.Config{
		FileName:    "Phieu_thanh_toan",
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

func ExportPaymentClinic(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Select(`payment.code, payment.amount, payment.bank_code, payment.note,
			payment.payment_time, payment.created_at,
			br.name as branch_name,
			pm.name as payment_method_name, ct.name as card_type_name,
			uc.fullname as user_created_fullname,
			CASE payment.payment_for
				WHEN 7 THEN 'Thu phòng khám' WHEN 8 THEN 'Chi phòng khám'
				ELSE 'Khác'
			END as paymentFor`).
		Joins("LEFT JOIN branch br ON br.id = payment.branch_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = payment.created_by").
		Joins("LEFT JOIN payment_method pm ON pm.id = payment.method").
		Joins("LEFT JOIN card_type ct ON ct.id = payment.card_type_id")

	url, err := doExport(c, eq, export.Config{
		FileName: "Phieu_thu_chi",
		Headers: []string{
			"Mã thanh toán", "Số tiền thanh toán", "Loại thanh toán", "Hình thức",
			"Ngân hàng", "Hóa đơn", "Chi nhánh", "Ngày thu tiền",
			"Nội dung", "Ngày tạo", "Người tạo",
		},
		ExportDir:   "./storage/export/",
		DownloadURL: DownloadURL,
	}, func(row map[string]interface{}) []interface{} {
		return []interface{}{
			row["code"], toFloat(row["amount"]), row["paymentFor"],
			row["payment_method_name"], row["card_type_name"],
			row["bank_code"], row["branch_name"],
			fmtUnixDateTime(row["payment_time"], loc),
			stripHTML(row["note"]), fmtUnixDateTime(row["created_at"], loc),
			row["user_created_fullname"],
		}
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, url)
}

// depositColsFor defines the exportable columns for ExportPaymentDeposit (list-deposit, svc method
// exportCustomerDeposit). key = displayCols[].value của list; FE gửi subset qua ?columns=.
// loc format ngày; latestApptByCustomerID cho cột "Ngày hẹn lịch"; showPhone (option 20) mask SĐT.
func depositColsFor(loc *time.Location, latestApptByCustomerID map[int]int64, showPhone bool) []paymentGenCol {
	return []paymentGenCol{
		{"id", "ID", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_code", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"customer_old_code", "Mã khách cũ", func(r map[string]interface{}) interface{} { return r["customer_old_code"] }},
		{"fullname", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"payment_code", "Mã đặt cọc", func(r map[string]interface{}) interface{} { return r["code"] }},
		{"paymentMethod", "Hình thức", func(r map[string]interface{}) interface{} { return r["payment_method_name"] }},
		{"cardType", "Phương thức", func(r map[string]interface{}) interface{} { return r["card_type_name"] }},
		{"amount", "Số tiền cọc", func(r map[string]interface{}) interface{} { return toFloat(r["amount"]) }},
		{"deposit_after", "Còn lại", func(r map[string]interface{}) interface{} { return toFloat(r["deposit_after"]) }},
		{"typeDepositLabel", "Loại cọc", func(r map[string]interface{}) interface{} { return r["payment_for_name"] }},
		{"source", "Nguồn khách hàng", func(r map[string]interface{}) interface{} { return r["source_name"] }},
		{"appointment_date", "Ngày hẹn lịch", func(r map[string]interface{}) interface{} {
			var apptDate interface{}
			if d, ok := latestApptByCustomerID[toInt(r["customer_id"])]; ok && d != 0 {
				apptDate = d
			}
			return fmtUnixDate(apptDate, loc)
		}},
		{"province", "Tỉnh thành", func(r map[string]interface{}) interface{} { return r["customer_province_name"] }},
		{"bank_code", "Hóa đơn", func(r map[string]interface{}) interface{} { return r["bank_code"] }},
		{"content", "Nội dung", func(r map[string]interface{}) interface{} { return stripHTML(r["note"]) }},
		{"payment_time", "Ngày thu cọc", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["payment_time"], loc) }},
		{"branch", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
		{"created_at", "Ngày tạo", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_at"], loc) }},
		{"created_by", "Người tạo", func(r map[string]interface{}) interface{} { return r["user_created_fullname"] }},
		{"updated_by", "Người cập nhật", func(r map[string]interface{}) interface{} { return r["user_updated_fullname"] }},
		{"updated_at", "Ngày cập nhật", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["updated_at"], loc) }},
	}
}

func ExportPaymentDeposit(c *gin.Context, q *gorm.DB) {
	exportPaymentDeposit(c, q, "Phieu_dat_coc", nil)
}

// skipCols = nil → lấy trọn colDef. Khác nhau ở đây quyết định luôn việc có chạy query phụ
// lấy ngày hẹn gần nhất hay không.
func exportPaymentDeposit(c *gin.Context, q *gorm.DB, fileName string, skipCols map[string]bool) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	var latestApptByCustomerID map[int]int64
	if !skipCols["appointment_date"] {
		latestApptByCustomerID = loadLatestAppointmentDateByCustomerID(q, "payment.customer_id")
	}

	eq := q.Session(&gorm.Session{}).
		Select(`payment.id, payment.customer_id,
			payment.code, payment.amount, payment.deposit_after, payment.bank_code, payment.note,
			payment.payment_time, payment.created_at, payment.updated_at,
			customer.code as customer_code, customer.old_code as customer_old_code,
			customer.fullname as customer_fullname, customer.phone as customer_phone,
			csrc.name as source_name,
			prov.name as customer_province_name,
			br.name as branch_name,
			pm.name as payment_method_name, ct.name as card_type_name,
			uc.fullname as user_created_fullname, uu.fullname as user_updated_fullname,
			CASE payment.deposit_type
				WHEN 1 THEN 'Đặt cọc giữ chỗ' WHEN 2 THEN 'Đặt cọc tạm' WHEN 3 THEN 'Ký gửi'
			END as payment_for_name`).
		Joins("LEFT JOIN source csrc ON csrc.id = customer.source_id").
		Joins("LEFT JOIN location_province prov ON prov.id = customer.province_id").
		Joins("LEFT JOIN branch br ON br.id = payment.branch_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = payment.created_by").
		Joins("LEFT JOIN user_profile uu ON uu.user_id = payment.updated_by").
		Joins("LEFT JOIN payment_method pm ON pm.id = payment.method").
		Joins("LEFT JOIN card_type ct ON ct.id = payment.card_type_id")

	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	allCols := depositColsFor(loc, latestApptByCustomerID, showPhone)
	if len(skipCols) > 0 {
		kept := make([]paymentGenCol, 0, len(allCols))
		for _, col := range allCols {
			if !skipCols[col.key] {
				kept = append(kept, col)
			}
		}
		allCols = kept
	}
	cols := allCols
	if colParam := c.Query("columns"); colParam != "" {
		selected := map[string]bool{}
		for _, k := range strings.Split(colParam, ",") {
			selected[strings.TrimSpace(k)] = true
		}
		var filtered []paymentGenCol
		for _, col := range allCols {
			if selected[col.key] {
				filtered = append(filtered, col)
			}
		}
		if len(filtered) > 0 {
			cols = filtered
		}
	}

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

func ExportPaymentDoctor(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Select(`payment.id, payment.code, payment.amount, payment.note,
			payment.payment_time, payment.created_at,
			customer.code as customer_code, customer.old_code as customer_old_code,
			customer.fullname as customer_fullname,
			src.name as source_name,
			order_plan.code as plan_code,
			ose.code as ose_code, svc.code as ose_service_code, svc.name as ose_service_name,
			ose.quantity as ose_quantity, ose.totalEnd as ose_totalEnd,
			ose.created_day as ose_created_day,
			br.name as branch_name,
			uc.fullname as user_created_fullname`).
		Joins("LEFT JOIN order_service_used ose ON ose.id = payment.serviceUsed_id").
		Joins("LEFT JOIN source src ON src.id = ose.source_id").
		Joins("LEFT JOIN service svc ON svc.id = ose.service_id").
		Joins("LEFT JOIN branch br ON br.id = payment.branch_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = payment.created_by")

	url, err := doExport(c, eq, export.Config{
		FileName: "Bao_cao_bac_si",
		Headers: []string{
			"id", "Mã khách hàng", "Mã khách cũ", "Khách hàng",
			"Nguồn", "Mã đơn hàng", "Mã dịch vụ sử dụng", "Mã thanh toán",
			"Mã dịch vụ", "Tên dịch vụ", "Số lượng", "Ghi chú",
			"Doanh số", "Doanh thu", "Ngày tạo dịch vụ", "Ngày thanh toán",
			"Chi nhánh", "Người tạo", "Ngày tạo",
		},
		ExportDir:   "./storage/export/",
		DownloadURL: DownloadURL,
	}, func(row map[string]interface{}) []interface{} {
		return []interface{}{
			row["id"], row["customer_code"], row["customer_old_code"], row["customer_fullname"],
			row["source_name"], row["plan_code"], row["ose_code"], row["code"],
			row["ose_service_code"], row["ose_service_name"], row["ose_quantity"],
			stripHTML(row["note"]),
			toFloat(row["ose_totalEnd"]), toFloat(row["amount"]),
			fmtUnixDateTime(row["ose_created_day"], loc), fmtUnixDateTime(row["payment_time"], loc),
			row["branch_name"], row["user_created_fullname"],
			fmtUnixDateTime(row["created_at"], loc),
		}
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, url)
}

func ExportPaymentReceipts(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	eq := paymentJoins(q.Session(&gorm.Session{})).
		Joins("LEFT JOIN user_profile uu ON uu.user_id = payment.updated_by").
		Select(`payment.id, payment.code, payment.amount, payment.bank_code, payment.note,
			payment.payment_time, payment.created_at, payment.updated_at,
			customer.code as customer_code, customer.old_code as customer_old_code,
			customer.fullname as customer_fullname, customer.address as customer_address,
			customer.phone as customer_phone,
			otp.code as plan_code,
			ose.code as ose_code, svc.code as service_code, svc.name as service_name,
			ose.quantity as ose_quantity, ose.totalEnd as ose_totalEnd, ose.vat_amount as ose_vat_amount,
			st.name as service_type_name, cst.name as customer_service_type_name,
			src.name as source_name, csrc.name as customer_source_name,
			prov.name as customer_province_name, ctry.CommonName as customer_country_name,
			br.name as branch_name,
			uc.fullname as user_created_fullname, uu.fullname as user_updated_fullname,
			pm.name as payment_method_name, ct.name as card_type_name,
			CASE payment.payment_for
				WHEN 1 THEN 'Hoàn dịch vụ' WHEN 2 THEN 'Hoàn cọc' WHEN 3 THEN 'Đặt cọc'
				WHEN 4 THEN 'Thanh toán đơn hàng' WHEN 5 THEN 'Chuyển cọc'
				WHEN 6 THEN 'Hoàn ký gửi' WHEN 7 THEN 'Thu phòng khám' WHEN 8 THEN 'Chi phòng khám'
			END as payment_for_name,
			CASE payment.deposit_type
				WHEN 1 THEN 'Đặt cọc giữ chỗ' WHEN 2 THEN 'Đặt cọc tạm' WHEN 3 THEN 'Ký gửi'
			END as deposit_type_name`)

	// Filter columns by FE selectedColumns — ?columns=customer_id,amount,...
	// Preserve order from paymentReceiptColsFor; fall back to all if param absent.
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	allCols := paymentReceiptColsFor(loc, showPhone)
	cols := allCols
	if colParam := c.Query("columns"); colParam != "" {
		selected := map[string]bool{}
		for _, k := range strings.Split(colParam, ",") {
			selected[strings.TrimSpace(k)] = true
		}
		var filtered []paymentReceiptCol
		for _, col := range allCols {
			if selected[col.key] {
				filtered = append(filtered, col)
			}
		}
		if len(filtered) > 0 {
			cols = filtered
		}
	}

	headers := make([]string, len(cols))
	for i, col := range cols {
		headers[i] = col.header
	}

	url, err := doExport(c, eq, export.Config{
		FileName:    "Phieu_thu_tien",
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

// refundDepositColsFor defines the exportable columns for ExportPaymentRefundDeposit,
// keyed by FE list-refund-deposit displayCols[].value. showPhone (option 20) mask SĐT.
func refundDepositColsFor(loc *time.Location, showPhone bool) []paymentGenCol {
	return []paymentGenCol{
		{"id", "ID", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_number", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"customer_old_code", "Mã khách cũ", func(r map[string]interface{}) interface{} { return r["customer_old_code"] }},
		{"customer_id", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"code", "Mã phiếu hoàn cọc", func(r map[string]interface{}) interface{} { return r["code"] }},
		{"deposit", "Số tiền hoàn", func(r map[string]interface{}) interface{} { return toFloat(r["amount"]) }},
		{"content", "Nội dung hoàn cọc", func(r map[string]interface{}) interface{} { return stripHTML(r["note"]) }},
		{"codeDeposit", "Mã phiếu thu cọc", func(r map[string]interface{}) interface{} { return r["deposit_code"] }},
		{"contentDeposit", "Nội dung thu cọc", func(r map[string]interface{}) interface{} { return stripHTML(r["deposit_note"]) }},
		{"payment_time", "Ngày hoàn tiền", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["payment_time"], loc) }},
		{"branch_id", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
		{"created_by", "Người tạo", func(r map[string]interface{}) interface{} { return r["user_created_fullname"] }},
		{"created_at", "Ngày tạo", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_at"], loc) }},
		{"updated_by", "Người cập nhật", func(r map[string]interface{}) interface{} { return r["user_updated_fullname"] }},
		{"updated_at", "Ngày cập nhật", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["updated_at"], loc) }},
	}
}

func ExportPaymentRefundDeposit(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Select(`payment.id, payment.code, payment.amount, payment.note, payment.payment_time,
			payment.created_at, payment.updated_at,
			customer.code as customer_code, customer.old_code as customer_old_code,
			customer.fullname as customer_fullname, customer.phone as customer_phone,
			dep.code as deposit_code, dep.note as deposit_note,
			br.name as branch_name,
			uc.fullname as user_created_fullname, uu.fullname as user_updated_fullname`).
		Joins("LEFT JOIN payment dep ON dep.id = payment.deposit_id").
		Joins("LEFT JOIN branch br ON br.id = payment.branch_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = payment.created_by").
		Joins("LEFT JOIN user_profile uu ON uu.user_id = payment.updated_by")

	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	allCols := refundDepositColsFor(loc, showPhone)
	cols := allCols
	if colParam := c.Query("columns"); colParam != "" {
		selected := map[string]bool{}
		for _, k := range strings.Split(colParam, ",") {
			selected[strings.TrimSpace(k)] = true
		}
		var filtered []paymentGenCol
		for _, col := range allCols {
			if selected[col.key] {
				filtered = append(filtered, col)
			}
		}
		if len(filtered) > 0 {
			cols = filtered
		}
	}

	headers := make([]string, len(cols))
	for i, col := range cols {
		headers[i] = col.header
	}

	url, err := doExport(c, eq, export.Config{
		FileName:    "Hoan_coc",
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

// refundOrderPlanColsFor defines the exportable columns for ExportPaymentRefundOrderPlan,
// keyed by FE list-refund-service displayCols[].value. showPhone (option 20) mask SĐT.
func refundOrderPlanColsFor(loc *time.Location, showPhone bool) []paymentGenCol {
	return []paymentGenCol{
		{"id", "ID", func(r map[string]interface{}) interface{} { return r["id"] }},
		{"customer_number", "Mã khách hàng", func(r map[string]interface{}) interface{} { return r["customer_code"] }},
		{"customer_old_code", "Mã khách cũ", func(r map[string]interface{}) interface{} { return r["customer_old_code"] }},
		{"customer_id", "Khách hàng", func(r map[string]interface{}) interface{} { return r["customer_fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}) interface{} { return maskPhoneExport(showPhone, r["customer_phone"]) }},
		{"serviceCode", "Mã dịch vụ sử dụng", func(r map[string]interface{}) interface{} { return r["ose_code"] }},
		{"service", "Dịch vụ", func(r map[string]interface{}) interface{} { return r["service_name"] }},
		{"code", "Mã phiếu", func(r map[string]interface{}) interface{} { return r["code"] }},
		{"deposit", "Số tiền hoàn", func(r map[string]interface{}) interface{} { return toFloat(r["amount"]) }},
		{"note", "Nội dung", func(r map[string]interface{}) interface{} { return stripHTML(r["note"]) }},
		{"payment_time", "Ngày hoàn tiền", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["payment_time"], loc) }},
		{"branch_id", "Chi nhánh", func(r map[string]interface{}) interface{} { return r["branch_name"] }},
		{"created_by", "Người tạo", func(r map[string]interface{}) interface{} { return r["user_created_fullname"] }},
		{"created_at", "Ngày tạo", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["created_at"], loc) }},
		{"updated_by", "Người cập nhật", func(r map[string]interface{}) interface{} { return r["user_updated_fullname"] }},
		{"updated_at", "Ngày cập nhật", func(r map[string]interface{}) interface{} { return fmtUnixDateTime(r["updated_at"], loc) }},
	}
}

func ExportPaymentRefundOrderPlan(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Select(`payment.id, payment.code, payment.amount, payment.note, payment.payment_time,
			payment.created_at, payment.updated_at,
			customer.code as customer_code, customer.old_code as customer_old_code,
			customer.fullname as customer_fullname, customer.phone as customer_phone,
			ose.code as ose_code, svc.name as service_name,
			br.name as branch_name,
			uc.fullname as user_created_fullname, uu.fullname as user_updated_fullname`).
		Joins("LEFT JOIN order_service_used ose ON ose.id = payment.serviceUsed_id").
		Joins("LEFT JOIN service svc ON svc.id = ose.service_id").
		Joins("LEFT JOIN branch br ON br.id = payment.branch_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = payment.created_by").
		Joins("LEFT JOIN user_profile uu ON uu.user_id = payment.updated_by")

	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	allCols := refundOrderPlanColsFor(loc, showPhone)
	cols := allCols
	if colParam := c.Query("columns"); colParam != "" {
		selected := map[string]bool{}
		for _, k := range strings.Split(colParam, ",") {
			selected[strings.TrimSpace(k)] = true
		}
		var filtered []paymentGenCol
		for _, col := range allCols {
			if selected[col.key] {
				filtered = append(filtered, col)
			}
		}
		if len(filtered) > 0 {
			cols = filtered
		}
	}

	headers := make([]string, len(cols))
	for i, col := range cols {
		headers[i] = col.header
	}

	url, err := doExport(c, eq, export.Config{
		FileName:    "Hoan_dich_vu",
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

// ExportPaymentTelesale: q là commission-based query (order_commission_telesale + payment + customer + order_service_used + service đã join).
func ExportPaymentTelesale(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Joins("LEFT JOIN order_plan otp ON otp.id = order_commission_telesale.order_plan_id").
		Joins("LEFT JOIN service_type st ON st.id = service.service_type_id").
		Joins("LEFT JOIN service_type cst ON cst.id = customer.type_service_id").
		Joins("LEFT JOIN source src ON src.id = order_service_used.source_id").
		Joins("LEFT JOIN source csrc ON csrc.id = customer.source_id").
		Joins("LEFT JOIN location_province prov ON prov.id = customer.province_id").
		Joins("LEFT JOIN location_country ctry ON ctry.id = customer.country_id").
		Joins("LEFT JOIN branch br ON br.id = order_commission_telesale.branch_id").
		// story-17: cột `telesale_id` đã RENAME → `revenue_id` (migration 20260621161526).
		Joins("LEFT JOIN user_profile tv ON tv.user_id = order_commission_telesale.revenue_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = payment.created_by").
		Select(`payment.id, payment.code, payment.amount, payment.note,
			payment.payment_time, payment.created_at,
			customer.code as customer_code, customer.old_code as customer_old_code,
			customer.fullname as customer_fullname, customer.address as customer_address,
			otp.code as plan_code,
			order_service_used.code as ose_code, service.code as ose_service_code, service.name as ose_service_name,
			order_service_used.quantity as ose_quantity, order_service_used.totalEnd as ose_totalEnd,
			st.name as service_type_name, cst.name as service_category_name,
			src.name as source_name, csrc.name as customer_source_name,
			prov.name as customer_province_name, ctry.CommonName as customer_country_name,
			br.name as branch_name,
			tv.fullname as telesale_fullname,
			uc.fullname as user_created_fullname`)

	url, err := doExport(c, eq, export.Config{
		FileName: "Hoa_hong_tu_van",
		Headers: []string{
			"id", "Mã khách hàng", "Mã khách cũ", "Khách hàng",
			"Nguồn khách hàng", "Nguồn tư vấn", "Mã đơn hàng",
			"Mã dịch vụ sử dụng", "Mã thanh toán", "Mã dịch vụ", "Dịch vụ",
			"Nhóm dịch vụ", "Số lượng",
			"Ghi chú", "Tư vấn viên",
			"Doanh số", "Doanh thu", "Ngày thanh toán",
			"Địa chỉ", "Tỉnh thành", "Quốc gia", "Chi nhánh",
			"Người tạo", "Ngày tạo",
		},
		ExportDir:   "./storage/export/",
		DownloadURL: DownloadURL,
	}, func(row map[string]interface{}) []interface{} {
		return []interface{}{
			row["id"], row["customer_code"], row["customer_old_code"], row["customer_fullname"],
			row["customer_source_name"], row["source_name"], row["plan_code"],
			row["ose_code"], row["code"], row["ose_service_code"], row["ose_service_name"],
			row["service_type_name"],
			row["ose_quantity"],
			stripHTML(row["note"]), row["telesale_fullname"],
			toFloat(row["ose_totalEnd"]), toFloat(row["amount"]),
			fmtUnixDateTime(row["payment_time"], loc),
			row["customer_address"], row["customer_province_name"], row["customer_country_name"],
			row["branch_name"],
			row["user_created_fullname"], fmtUnixDateTime(row["created_at"], loc),
		}
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, url)
}
