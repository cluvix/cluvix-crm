package query

import "strconv"

// ── Chặn ép kiểu im lặng trên filter cột ID ────────────────────────────────────────────────
//
// VẤN ĐỀ (đo 2026-09-08 trên bench 545.547 dòng của 1 bảng con khách hàng thật, bảng gốc đã xoá
// ở story-13 crm-standalone):
// ApplyFilter đẩy CHUỖI THÔ vào `Where(col + " = ?", v)`. Với cột INT, MySQL ép kiểu rồi nuốt
// warning ⇒ URL rác đọc TRÚNG dữ liệu của bản ghi khác:
//
//	filter[customer_id]=888002      → 5.000 thẻ
//	filter[customer_id]=888002abc   → 5.000 thẻ   ← ĐỌC TRÚNG khách 888002
//	filter[customer_id]=" 888002"   → 5.000 thẻ
//
// `SHOW WARNINGS` xác nhận `Warning 1292 Truncated incorrect DOUBLE value: '888002abc'` — MySQL CÓ
// cảnh báo, GORM không đọc nên không ai thấy. Cùng lớp bug với Hard Rule "Path param id PHẢI parse
// ra số", nhưng ở cửa vào mà gate `verify-id-params.sh` KHÔNG soi (nó chỉ bắt c.Param/c.Query).
//
// VÌ SAO LÀ ALLOWLIST, KHÔNG PHẢI QUY TẮC "tên kết thúc _id":
// Trong schema có **36 cột tên `*_id` nhưng kiểu `varchar`** — `template_id`, `page_id`, `client_id`,
// `icd_id`, `snomed_id`, `msg_id`, `channel_id`, `app_id`… Với chúng, giá trị KHÔNG phải số là hợp lệ.
// Một quy tắc theo hậu tố tên sẽ biến `filter[template_id]=abc123` thành "không khớp gì" — đúng loại
// hỏng-im-lặng mà file này sinh ra để chặn. Allowlist thì fail an toàn: cột lạ rơi về hành vi cũ
// (không được bảo vệ, nhưng KHÔNG bị phá).
//
// Danh sách dưới đây = giao của (cột `*_id` mà ApplyFilter thật sự nhắm tới) × (kiểu thuần số ở MỌI
// bảng chứa nó), tra bằng information_schema. Tại thời điểm chốt: 63 cột id được dùng, 62 thuần số.
//
// ⚠ `service_type_id` CỐ Ý VẮNG MẶT: nó là `int` ở `service` nhưng `json` ở `treatment_course_type`.
// Thêm vào đây sẽ phá filter JSON của bảng kia.
//
// Thêm cột mới vào danh sách: chỉ khi đã kiểm cột đó thuần số ở MỌI bảng —
//
//	SELECT COLUMN_NAME, GROUP_CONCAT(DISTINCT DATA_TYPE), COUNT(DISTINCT TABLE_NAME)
//	FROM information_schema.COLUMNS
//	WHERE TABLE_SCHEMA=DATABASE() AND COLUMN_NAME='<tên>' GROUP BY COLUMN_NAME;
var numericIDColumns = map[string]bool{
	"id":                                true,
	"appointment_id":                    true,
	"appointment_not_booked_reasons_id": true,
	"appointment_type_id":               true,
	"branch_id":                         true,
	"broker_catagory_id":                true,
	"broker_id":                         true,
	"broker_type_id":                    true,
	"card_type_id":                      true,
	"category_id":                       true,
	"connect_id":                        true,
	"convert_id":                        true,
	"country_id":                        true,
	"coupon_catagory_id":                true,
	"coupon_id":                         true,
	"customer_care_classification_id":   true,
	"customer_care_complaints_type_id":  true,
	"customer_care_result_id":           true,
	"customer_care_type_id":             true,
	"customer_id":                       true,
	"department_id":                     true,
	"doctor_consultant_id":              true,
	"from_warehouse_id":                 true,
	"groupCountry_id":                   true,
	"groupLocal_id":                     true,
	"groupSource_id":                    true,
	"material_id":                       true,
	"order_service_used_id":             true,
	"owner_id":                          true,
	"payment_user_id":                   true,
	"plan_id":                           true,
	"priority_care_id":                  true,
	"province_id":                       true,
	"reason_id":                         true,
	"revenue_id":                        true,
	"serviceCategory_id":                true,
	"serviceType_id":                    true,
	"service_id":                        true,
	"service_used_id":                   true,
	"source_id":                         true,
	"staff_id":                          true,
	"stage_id":                          true, // customer.stage_id (int, bảng duy nhất — kiểm 02/11/2026)
	"staff_support_id":                  true,
	"stock_entry_id":                    true,
	"supplier_id":                       true,
	"to_warehouse_id":                   true,
	"type_id":                           true,
	"type_service_accepted_id":          true,
	"type_service_id":                   true,
	"user_id":                           true,
	"warehouse_id":                      true,
}

// isNumericIDColumn nhận cả dạng "col" lẫn "alias.col" / "table.col" — chỉ xét đoạn sau dấu chấm
// CUỐI, vì call site viết đủ kiểu ("customer_id", "appointment.customer_id", "osu.customer_id").
func isNumericIDColumn(column string) bool {
	name := column
	for i := len(column) - 1; i >= 0; i-- {
		if column[i] == '.' {
			name = column[i+1:]
			break
		}
	}
	return numericIDColumns[name]
}

// isValidIDValue — giá trị có phải số nguyên không dấu hợp lệ không.
//
// KHÔNG dùng strconv.Atoi rồi bỏ qua lỗi: chính việc "parse được phần đầu" là cơ chế gây bug.
// ParseUint từ chối "888002abc", " 888002" (có space), "888002.9", "-1", "" — đúng tập giá trị mà
// MySQL sẽ ép kiểu im lặng.
func isValidIDValue(v string) bool {
	_, err := strconv.ParseUint(v, 10, 64)
	return err == nil
}

// filterNumericIDValues giữ lại các giá trị hợp lệ trong danh sách (dùng cho IN).
func filterNumericIDValues(vals []string) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if isValidIDValue(v) {
			out = append(out, v)
		}
	}
	return out
}
