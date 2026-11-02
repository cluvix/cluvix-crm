package export

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/export"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tz"
)

// customerStageExportJoin — tên bước tiến trình HIỆN TẠI của khách cho cột "Tiến trình" (epic
// customer-stage-pipeline story-07; thay nhãn cứng theo tiến trình cũ). LEFT JOIN: khách chưa có
// bước ⇒ ô trống, không mất dòng; bước ẩn vẫn ra tên (không lọc customer_stage.status).
const customerStageExportJoin = "LEFT JOIN customer_stage cs ON cs.id = customer.stage_id"

// customerExpCol — 1 cột export; key khớp 1-1 FE displayCols[].value (ExportOptionsDialog).
type customerExpCol struct {
	key     string
	header  string
	extract func(r map[string]interface{}, loc *time.Location) interface{}
}

// pickCustomerCols filter colDef theo ?columns=col1,col2 (ExportOptionsDialog).
// Không có param hoặc không khớp key nào → trả full (backward-compat: export all).
func pickCustomerCols(all []customerExpCol, colParam string) []customerExpCol {
	if colParam == "" {
		return all
	}
	selected := map[string]bool{}
	for _, k := range strings.Split(colParam, ",") {
		selected[strings.TrimSpace(k)] = true
	}
	var filtered []customerExpCol
	for _, col := range all {
		if selected[col.key] {
			filtered = append(filtered, col)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}
	return all
}

// runCustomerExport build header + mapper từ colDef đã filter rồi xuất file.
func runCustomerExport(c *gin.Context, eq *gorm.DB, loc *time.Location, fileName string, cols []customerExpCol) {
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
			data[i] = col.extract(row, loc)
		}
		return data
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, url)
}

func ExportCustomer(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))

	eq := q.Session(&gorm.Session{}).
		Select(`customer.id, customer.code, customer.old_code, COALESCE(customer.fullname, customer.nickname) as fullname,
			CASE customer.customerType WHEN 2 THEN 'Cũ' ELSE 'Mới' END as customer_type_label,
			customer.phone, customer.email, cs.name AS stage_name,
			customer.gender,
			DATE_FORMAT(customer.birthday, '%d/%m/%Y') as birthday,
			TIMESTAMPDIFF(YEAR, customer.birthday, CURDATE()) AS age,
			src.name as source_name, st.name as service_type_name,
			rev.fullname as revenue_fullname,
			customer.totalRevenue, customer.totalPayment, customer.totalDebt, customer.totalDeposit,
			customer.refundDeposit, customer.refundService,
			cntry.CommonName as country_name, prov.name as province_name, ward.name as ward_name,
			customer.address,
			br.name as branch_name,
			uc.fullname as user_created_fullname,
			customer.created_day, customer.created_at`).
		Joins("LEFT JOIN source src ON src.id = customer.source_id").
		Joins("LEFT JOIN service_type st ON st.id = customer.type_service_id").
		Joins("LEFT JOIN user_profile rev ON rev.user_id = customer.payment_user_id").
		Joins("LEFT JOIN location_country cntry ON cntry.id = customer.country_id").
		Joins("LEFT JOIN location_province prov ON prov.id = customer.province_id").
		Joins("LEFT JOIN location_ward ward ON ward.id = customer.ward_id").
		Joins("LEFT JOIN branch br ON br.id = customer.branch_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = customer.created_by").
		Joins(customerStageExportJoin)

	// colDef — key khớp displayColsContactList (crm/customer/contact-list).
	// Các cột BE-only (customerType/ward/created_at)
	// giữ để "export all" đầy đủ như cũ; FE không gửi key này ở chế độ "selected".
	cols := []customerExpCol{
		{"id", "ID", func(r map[string]interface{}, _ *time.Location) interface{} { return r["id"] }},
		{"code", "Mã khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["code"] }},
		{"old_code", "Mã khách cũ", func(r map[string]interface{}, _ *time.Location) interface{} { return r["old_code"] }},
		{"fullname", "Khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["fullname"] }},
		{"customerType", "Loại khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["customer_type_label"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}, _ *time.Location) interface{} {
			return maskPhoneExport(showPhone, r["phone"])
		}},
		{"email", "Email", func(r map[string]interface{}, _ *time.Location) interface{} { return r["email"] }},
		{"genderCustomer", "Giới tính", func(r map[string]interface{}, _ *time.Location) interface{} { return genderExportLabel(r["gender"]) }},
		{"birthday", "Ngày sinh", func(r map[string]interface{}, _ *time.Location) interface{} { return r["birthday"] }},
		{"age", "Độ tuổi", func(r map[string]interface{}, _ *time.Location) interface{} { return r["age"] }},
		{"statusName", "Tiến trình", func(r map[string]interface{}, _ *time.Location) interface{} { return r["stage_name"] }},
		{"source", "Nguồn khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["source_name"] }},
		{"serviceType", "Nhóm dịch vụ quan tâm", func(r map[string]interface{}, _ *time.Location) interface{} { return r["service_type_name"] }},
		{"payment_user_id", "Tư vấn viên", func(r map[string]interface{}, _ *time.Location) interface{} { return r["revenue_fullname"] }},
		{"totalRevenue", "Doanh số", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalRevenue"]) }},
		{"totalPayment", "Doanh thu", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalPayment"]) }},
		{"totalDebt", "Công nợ", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalDebt"]) }},
		{"totalDeposit", "Đặt cọc", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalDeposit"]) }},
		{"refundDeposit", "Hoàn cọc", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["refundDeposit"]) }},
		{"refundService", "Hoàn dịch vụ", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["refundService"]) }},
		{"country", "Quốc gia", func(r map[string]interface{}, _ *time.Location) interface{} { return r["country_name"] }},
		{"province", "Tỉnh thành", func(r map[string]interface{}, _ *time.Location) interface{} { return r["province_name"] }},
		{"ward", "Phường/Xã", func(r map[string]interface{}, _ *time.Location) interface{} { return r["ward_name"] }},
		{"address", "Địa chỉ", func(r map[string]interface{}, _ *time.Location) interface{} { return r["address"] }},
		{"branch", "Chi nhánh", func(r map[string]interface{}, _ *time.Location) interface{} { return r["branch_name"] }},
		{"created_by", "Người tạo", func(r map[string]interface{}, _ *time.Location) interface{} { return r["user_created_fullname"] }},
		{"createdDay", "Ngày ghi nhận", func(r map[string]interface{}, loc *time.Location) interface{} {
			return fmtUnixDate(r["created_day"], loc)
		}},
		{"created_at", "Ngày tạo", func(r map[string]interface{}, loc *time.Location) interface{} {
			return fmtUnixDate(r["created_at"], loc)
		}},
	}

	cols = pickCustomerCols(cols, c.Query("columns"))
	runCustomerExport(c, eq, loc, "Khach-Hang-export", cols)
}

// genderExportLabel — map customer.gender (1/2/3) → nhãn; 0/giá trị lạ → "".
func genderExportLabel(v interface{}) string {
	return define.GenderLabels[int64(toInt(v))]
}

func ExportCustomerBirthday(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))

	eq := q.Session(&gorm.Session{}).
		Select(`customer.id, customer.code, COALESCE(customer.fullname, customer.nickname) as fullname,
			customer.phone, customer.gender, DATE_FORMAT(customer.birthday, '%d/%m/%Y') as birthday, cs.name AS stage_name,
			customer.totalRevenue, customer.totalPayment, customer.totalDebt, customer.totalDeposit,
			br.name as branch_name`).
		Joins("LEFT JOIN branch br ON br.id = customer.branch_id").
		Joins(customerStageExportJoin)

	// colDef — key khớp displayCols CustomerCareBirthday (care/birthday).
	// birthday format dd/mm/yyyy theo convention export module (installment.go, fmtUnixDateSlash) —
	// khác dash-format "dd-MM-yyyy" của FE list (BaseApiService.formatTimestampFields), có chủ đích.
	cols := []customerExpCol{
		{"id", "id", func(r map[string]interface{}, _ *time.Location) interface{} { return r["id"] }},
		{"code", "Mã khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["code"] }},
		{"fullname", "Khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}, _ *time.Location) interface{} {
			return maskPhoneExport(showPhone, r["phone"])
		}},
		{"genderCustomer", "Giới tính", func(r map[string]interface{}, _ *time.Location) interface{} { return genderExportLabel(r["gender"]) }},
		{"birthday", "Ngày sinh", func(r map[string]interface{}, _ *time.Location) interface{} { return r["birthday"] }},
		{"statusName", "Tiến trình", func(r map[string]interface{}, _ *time.Location) interface{} { return r["stage_name"] }},
		{"branch", "Chi nhánh", func(r map[string]interface{}, _ *time.Location) interface{} { return r["branch_name"] }},
		{"totalRevenue", "Doanh số", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalRevenue"]) }},
		{"totalPayment", "Doanh thu", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalPayment"]) }},
		{"totalDebt", "Công nợ", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalDebt"]) }},
		{"totalDeposit", "Đặt cọc", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalDeposit"]) }},
	}

	cols = pickCustomerCols(cols, c.Query("columns"))
	runCustomerExport(c, eq, loc, "Birthday-export", cols)
}

func ExportCustomerConvert(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).
		Select(`customer.id, COALESCE(customer.fullname, customer.nickname) as fullname, customer.code, customer.old_code,
			customer.nickname, customer.data_id, customer.connect_id,
			customer.phone, customer.gender, DATE_FORMAT(customer.birthday, '%d/%m/%Y') as birthday,
			rev.fullname as revenue_fullname, cs.name AS stage_name,
			cc.name as category_name,
			src.name as source_name, st.name as service_type_name,
			prov.name as province_name, cntry.CommonName as country_name,
			br.name as branch_name,
			customer.created_day, customer.created_at, customer.updated_at,
			uc.fullname as user_created_fullname, uu.fullname as user_updated_fullname,
			(SELECT n.note FROM note n WHERE n.customer_id = customer.id ORDER BY n.created_at DESC, n.id DESC LIMIT 1) AS latestNote`).
		Joins("LEFT JOIN user_profile rev ON rev.user_id = customer.payment_user_id").
		Joins("LEFT JOIN customer_category cc ON cc.id = customer.category").
		Joins("LEFT JOIN source src ON src.id = customer.source_id").
		Joins("LEFT JOIN service_type st ON st.id = customer.type_service_id").
		Joins("LEFT JOIN location_province prov ON prov.id = customer.province_id").
		Joins("LEFT JOIN location_country cntry ON cntry.id = customer.country_id").
		Joins("LEFT JOIN branch br ON br.id = customer.branch_id").
		Joins("LEFT JOIN user_profile uc ON uc.user_id = customer.created_by").
		Joins("LEFT JOIN user_profile uu ON uu.user_id = customer.updated_by").
		Joins(customerStageExportJoin)

	// colDef — key khớp displayCols marketing/convert/list.
	cols := []customerExpCol{
		{"id", "ID", func(r map[string]interface{}, _ *time.Location) interface{} { return r["id"] }},
		{"code", "Mã khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["code"] }},
		{"old_code", "Mã khách cũ", func(r map[string]interface{}, _ *time.Location) interface{} { return r["old_code"] }},
		{"fullname", "Khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["fullname"] }},
		{"nickname", "Tên khác", func(r map[string]interface{}, _ *time.Location) interface{} { return r["nickname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}, _ *time.Location) interface{} {
			return maskPhoneExport(showPhone, r["phone"])
		}},
		{"gender", "Giới tính", func(r map[string]interface{}, _ *time.Location) interface{} { return genderExportLabel(r["gender"]) }},
		{"birthday", "Ngày sinh", func(r map[string]interface{}, _ *time.Location) interface{} { return r["birthday"] }},
		{"status", "Tiến trình", func(r map[string]interface{}, _ *time.Location) interface{} { return r["stage_name"] }},
		{"data_id", "Dữ liệu", func(r map[string]interface{}, _ *time.Location) interface{} {
			return customerDataExportLabel(r["data_id"])
		}},
		{"connect_id", "Kết nối", func(r map[string]interface{}, _ *time.Location) interface{} {
			return customerConnectExportLabel(r["connect_id"])
		}},
		{"payment_user_id", "Tư vấn viên", func(r map[string]interface{}, _ *time.Location) interface{} { return r["revenue_fullname"] }},
		{"source_id", "Nguồn khách", func(r map[string]interface{}, _ *time.Location) interface{} { return r["source_name"] }},
		{"category", "Nhóm khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["category_name"] }},
		{"type_service_id", "Quan tâm", func(r map[string]interface{}, _ *time.Location) interface{} { return r["service_type_name"] }},
		{"latestNote", "Ghi chú gần nhất", func(r map[string]interface{}, _ *time.Location) interface{} { return r["latestNote"] }},
		{"province", "Tỉnh thành", func(r map[string]interface{}, _ *time.Location) interface{} { return r["province_name"] }},
		{"country", "Quốc gia", func(r map[string]interface{}, _ *time.Location) interface{} { return r["country_name"] }},
		{"branch_id", "Chi nhánh", func(r map[string]interface{}, _ *time.Location) interface{} { return r["branch_name"] }},
		{"created_day", "Ngày thu thập", func(r map[string]interface{}, loc *time.Location) interface{} {
			return fmtUnixDateTime(r["created_day"], loc)
		}},
		{"userCreated", "Người tạo", func(r map[string]interface{}, _ *time.Location) interface{} { return r["user_created_fullname"] }},
		{"created_at", "Ngày tạo", func(r map[string]interface{}, loc *time.Location) interface{} {
			return fmtUnixDateTime(r["created_at"], loc)
		}},
		{"updated_by", "Người cập nhật", func(r map[string]interface{}, _ *time.Location) interface{} { return r["user_updated_fullname"] }},
		{"updated_at", "Ngày cập nhật", func(r map[string]interface{}, loc *time.Location) interface{} {
			return fmtUnixDateTime(r["updated_at"], loc)
		}},
	}

	cols = pickCustomerCols(cols, c.Query("columns"))
	runCustomerExport(c, eq, loc, "Convert-export", cols)
}

func ExportCustomerDebts(c *gin.Context, q *gorm.DB) {
	showPhone := phoneutil.CanSeePhoneExport(c, q.Session(&gorm.Session{NewDB: true}))

	eq := q.Session(&gorm.Session{}).
		Select(`customer.id, customer.code, customer.old_code,
			CONCAT(COALESCE(customer.fullname, customer.nickname, ''),
				IF(customer.fullname IS NOT NULL AND customer.fullname != '' AND customer.nickname IS NOT NULL AND customer.nickname != '',
					CONCAT(' - ', customer.nickname), '')) as fullname,
			customer.phone,
			src.name as source_name, customer.totalRevenue, customer.totalPayment, customer.totalDebt, customer.totalDeposit,
			br.name as branch_name`).
		Joins("LEFT JOIN source src ON src.id = customer.source_id").
		Joins("LEFT JOIN branch br ON br.id = customer.branch_id")

	// colDef — key khớp displayCols accountant/report/debt/customer-debts.
	// source là cột BE-only để "export all" như cũ; FE debts không có key này. Story-38: bỏ
	// company/company_id khỏi export (D23).
	cols := []customerExpCol{
		{"id", "ID", func(r map[string]interface{}, _ *time.Location) interface{} { return r["id"] }},
		{"customer_number", "Mã khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["code"] }},
		{"customer_old_code", "Mã khách cũ", func(r map[string]interface{}, _ *time.Location) interface{} { return r["old_code"] }},
		{"customer_id", "Khách hàng", func(r map[string]interface{}, _ *time.Location) interface{} { return r["fullname"] }},
		{"phone", "Số điện thoại", func(r map[string]interface{}, _ *time.Location) interface{} {
			return maskPhoneExport(showPhone, r["phone"])
		}},
		{"source", "Nguồn", func(r map[string]interface{}, _ *time.Location) interface{} { return r["source_name"] }},
		{"totalDoanhSo", "Doanh số", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalRevenue"]) }},
		{"totalPayment", "Thanh toán", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalPayment"]) }},
		{"dept", "Công nợ", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalDebt"]) }},
		{"totalDeposit", "Đặt cọc", func(r map[string]interface{}, _ *time.Location) interface{} { return toFloat(r["totalDeposit"]) }},
		{"branch_id", "Chi nhánh", func(r map[string]interface{}, _ *time.Location) interface{} { return r["branch_name"] }},
	}

	cols = pickCustomerCols(cols, c.Query("columns"))
	runCustomerExport(c, eq, nil, "Cong_no-export", cols)
}

// customerDataExportLabel — nhãn cột "Dữ liệu" (customer.data_id): 1=mới, 2=cũ.
func customerDataExportLabel(v interface{}) interface{} {
	switch toInt(v) {
	case 1:
		return "Dữ liệu mới"
	case 2:
		return "Dữ liệu cũ"
	}
	return ""
}

// customerConnectExportLabel — nhãn cột "Kết nối" (customer.connect_id): 1=đã, 2=chưa.
func customerConnectExportLabel(v interface{}) interface{} {
	switch toInt(v) {
	case 1:
		return "Đã kết nối"
	case 2:
		return "Chưa kết nối"
	}
	return ""
}

func maskPhoneExport(show bool, v interface{}) interface{} {
	if show {
		return v
	}
	s, _ := v.(string)
	return phoneutil.MaskPhone(s)
}
