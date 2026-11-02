package service

import (
	"encoding/json"
	"fmt"
	"strings"

	history "erpCluvix/internal/modules/webapp/history/log/service"
	"erpCluvix/pkg/codegen"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/optlock"
	"erpCluvix/pkg/pagination"

	"gorm.io/gorm"
)

// Id phòng ban lấy từ `pkg/define` (nguồn sự thật DUY NHẤT) — 2026-07-30 đã sửa
// `define.HrDepartmentConvert`/`Marketing` về đúng giá trị thật (11/4) nên khối hằng
// local ở đây không còn lý do tồn tại, xoá để không đẻ nguồn sự thật thứ hai (DW-38).
const (
	customizeFormContactID = 1
	customizeFormLeadID    = 2
)

// leadDepts — phòng ban dùng form khoá field "lead" (control_field_in_table id=2) trong
// applyFieldLocks; phòng ban khác dùng form "contact" (id=1). Mirror FE customer-form.ts
// loadValidationRules. KHÔNG còn quyết định status khi tạo khách: từ D14 (crm-standalone)
// khách mới luôn Contact(16) — xem CreateCustomer.
var leadDepts = []int{
	define.HrDepartmentSaleStaff,
	define.HrDepartmentTelesale,
	define.HrDepartmentConvert,
	define.HrDepartmentCustomerCare,
	define.HrDepartmentMarketing,
}

type CustomerService struct {
	DB *gorm.DB
}

func NewCustomerService(db *gorm.DB) *CustomerService {
	return &CustomerService{DB: db}
}

// Whitelist columns cho phép update
var customerAllowedFields = map[string]bool{
	"fullname": true, "nickname": true, "avatar": true, "phone": true, "other_phone": true,
	"gender": true, "birthday": true, "year_born": true, "address": true, "email": true,
	"status": true, "category": true, "customerType": true,
	"source_id": true, "branch_id": true, "province_id": true, "country_id": true, "ward_id": true,
	"payment_user_id": true, "connect_id": true,
	"type_service_id": true,
	"loai_message":    true, "link_message": true, "chat_message": true,
	"job_id": true, "ethnicity_id": true,
	"tags": true, "relatives": true,
	"last_contact_date": true, "last_visit_date": true,
	"facebook": true, "zalo": true, "viber": true, "instagram": true,

	"address_other":                     true,
	"appointment_not_booked_reasons_id": true,
	"salesStaff_id":                     true, "data_id": true, "customer_relation_id": true,
	"accepted": true, "updated_at": true, "updated_by": true,
	// create-only fields
	"created_at": true, "created_by": true, "created_day": true, "old_code": true,
}

// FilterAllowed trả về map chỉ chứa các field trong whitelist
func (s *CustomerService) FilterAllowed(body map[string]interface{}) map[string]interface{} {
	filtered := make(map[string]interface{}, len(body))
	for k, v := range body {
		if customerAllowedFields[k] {
			filtered[k] = v
		}
	}
	// Normalize birthday: FE thường gửi ISO "1986-08-12T00:00:00Z" — MySQL DATE column reject.
	// Chuỗi rỗng → NULL. Chuỗi có nội dung nhưng KHÔNG phải ngày thật (vd năm 5 chữ số) →
	// giữ nguyên chuỗi gốc (không tự null hoá, không tự sửa) để validateCustomerFromMap bắt
	// được và trả lỗi rõ ràng cho user, thay vì âm thầm xoá dữ liệu họ vừa nhập.
	if v, ok := filtered["birthday"]; ok {
		if s, isStr := v.(string); isStr {
			trimmed := strings.TrimSpace(s)
			if trimmed == "" {
				filtered["birthday"] = nil
			} else if normalized, valid := helpers.ParseDateOnly(trimmed); valid {
				filtered["birthday"] = normalized
			}
		}
	}
	// Trim whitespace từ phone fields — FE đôi khi gửi dư space ở cuối, làm
	// search/match không hoạt động đúng (bug đã gặp với SearchGlobal).
	for _, key := range []string{"phone", "other_phone"} {
		if v, ok := filtered[key]; ok {
			if s, isStr := v.(string); isStr {
				filtered[key] = strings.TrimSpace(s)
			}
		}
	}
	return filtered
}

// inDepartment kiểm tra userID có thuộc phòng ban deptID không (JSON_CONTAINS trên
// user_profile.departmentList — cùng pattern với validatePaymentUserIDChange).
func (s *CustomerService) inDepartment(userID, deptID int) bool {
	var n int64
	s.DB.Table("user_profile").
		Where("user_id = ? AND JSON_CONTAINS(departmentList, ?)", userID, fmt.Sprintf("%d", deptID)).
		Count(&n)
	return n > 0
}

// applyFieldLocks strip khỏi filtered các field bị đánh dấu "Không được cập nhật"
// (edit=true) trong control_field_in_table — enforce ở BE thay vì chỉ dựa vào FE disable
// control (customer-form.ts:handleEditMode), vì client có thể gọi thẳng API bỏ qua UI.
// BOD bypass toàn bộ khoá field (mirror customer-form.ts isDepartmentBOD()).
// Form nào áp dụng (contact id=1 / lead id=2) chọn theo phòng ban user đang thao tác,
// mirror customer-form.ts loadValidationRules().
func (s *CustomerService) applyFieldLocks(userID int, filtered map[string]interface{}) {
	if s.inDepartment(userID, define.HrDepartmentBoardOfDirectors) {
		return
	}

	formID := customizeFormContactID
	for _, d := range leadDepts {
		if s.inDepartment(userID, d) {
			formID = customizeFormLeadID
			break
		}
	}

	var raw []byte
	row := s.DB.Table("control_field_in_table").Select("control").Where("id = ?", formID).Row()
	if row.Scan(&raw) != nil || len(raw) == 0 {
		return
	}

	var fields []map[string]interface{}
	if json.Unmarshal(raw, &fields) != nil {
		// DB có thể lưu bọc {"control": [...]} thay vì mảng thuần
		var wrapper map[string]json.RawMessage
		if json.Unmarshal(raw, &wrapper) == nil {
			if inner, ok := wrapper["control"]; ok {
				_ = json.Unmarshal(inner, &fields)
			}
		}
	}

	for _, f := range fields {
		edit, _ := f["edit"].(bool)
		name, _ := f["field_name"].(string)
		if edit && name != "" {
			delete(filtered, name)
		}
	}
}

// genCode sinh mã khi Update gán chi nhánh cho khách chưa có mã (quy tắc: codegen.EnsureCustomerCode).
func genCode(db *gorm.DB, filtered map[string]interface{}, customerID int) {
	if code := codegen.EnsureCustomerCode(db, int64(customerID), helpers.ToInt64(filtered["branch_id"])); code != "" {
		filtered["code"] = code
	}
}

// Update cập nhật customer với optimistic locking + log changes.
// body đã được set sẵn updated_by từ handler; expectedUpdatedAt = updated_at client đã đọc (khoá lạc quan D6).
// id PHẢI là id đã parse ra số ở handler (`parseCustomerID`), KHÔNG phải `c.Param("id")` thô:
// MySQL cắt chuỗi thành số khi so với cột INT, nên `/crm/customer/79738abc` từng ghi trúng
// khách 79738 (warning 1292 bị nuốt). Kiểu `int` ở đây là để compiler chặn đường đó.
func (s *CustomerService) Update(id int, body map[string]interface{}, expectedUpdatedAt int64, userID int) (map[string]interface{}, error) {
	// Filter whitelist + normalize (birthday/phone) — dùng chung FilterAllowed
	// với path Create, tránh lệch normalize giữa 2 path (đã gây lỗi MySQL 1292 nhận thẳng
	// birthday chưa normalize xuống DB, xem incident 2026-08-19).
	filtered := s.FilterAllowed(body)

	// Strip field bị khoá "Không được cập nhật" — enforce ở BE, không chỉ dựa vào FE disable.
	s.applyFieldLocks(userID, filtered)

	if err := validateCustomerFromMap(filtered, false); err != nil {
		return nil, err
	}

	// Validate status transition before applying update
	if newStatus, ok := filtered["status"]; ok {
		var currentStatus int
		s.DB.Table("customer").Select("COALESCE(status, 0)").Where("id = ?", id).Scan(&currentStatus)
		if !define.IsValidStatusTransition(currentStatus, int(helpers.ToInt64(newStatus))) {
			return nil, errs.NewLocalizedError("customer_status_invalid_transition", currentStatus, int(helpers.ToInt64(newStatus)))
		}
	}

	// Serialize JSON columns trước khi update
	helpers.SerializeJSONFieldsForTable(filtered, "customer")

	// Snapshot trước khi update
	oldSnap := history.SnapshotRow(s.DB, "customer", id, history.CustomerIncludeFields)

	// Ghi có khoá: WHERE updated_at = expected + bump GREATEST(now, updated_at+1); 0 dòng ⇒ ErrVersionConflict
	// (race window: bản ghi đổi giữa bước kiểm ở handler và bước ghi).
	if _, err := optlock.Updates(s.DB.Table("customer").Where("id = ?", id), expectedUpdatedAt, filtered); err != nil {
		return nil, err
	}

	// Gen code nếu branch_id được set và customer chưa có code (beforeSave). Chạy SAU bước ghi có
	// khoá: EnsureCustomerCode tự UPDATE customer (bump updated_at) — chạy trước sẽ làm lệch
	// updated_at vừa kiểm ⇒ 409 giả. Item trả về đọc lại sau cùng nên có updated_at cuối.
	genCode(s.DB, filtered, id)

	// Log changes
	newSnap := history.SnapshotRow(s.DB, "customer", id, history.CustomerIncludeFields)
	_ = history.LogChanges(s.DB, "log_customer", "customer_id", id, oldSnap, newSnap, history.CustomerIncludeFields, history.CustomerSkipNullFields, userID)

	// Return updated row
	var item map[string]interface{}
	if tx := s.DB.Table("customer").Where("id = ?", id).Take(&item); pagination.IsQueryError(tx.Error) {
		return nil, tx.Error
	}
	// Cột DECIMAL scan vào map[string]interface{} là []uint8 → parse để JSON ra number.
	helpers.ParseDecimalFieldsForTables([]map[string]interface{}{item}, "customer")
	return item, nil
}
