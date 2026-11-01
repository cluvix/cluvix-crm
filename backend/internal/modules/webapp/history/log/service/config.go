package service

// --- Customer ---

var CustomerIncludeFields = []string{
	"fullname", "nickname", "phone", "category", "branch_id",
	"source_id", "payment_user_id", "type_service_id", "created_day",
}
var CustomerSkipNullFields = []string{"branch_id", "fullname", "nickname", "phone", "type_service_id"}

// CustomerDeleteFields — snapshot ghi vào dòng log xoá khách: field theo dõi + `code`. Sau khi
// xoá, log_customer.customer_id = NULL (ON DELETE SET NULL) nên snapshot này là thứ duy nhất
// còn nhận diện được khách; màn lịch sử đọc tên/mã từ đây (xem logCustomerQuery).
var CustomerDeleteFields = append([]string{"code"}, CustomerIncludeFields...)

// withDeleteIdentity — snapshot dòng log xoá = `id`, `code`, `customer_id` + field theo dõi.
// Sau khi xoá, fk trên dòng log = NULL (ON DELETE SET NULL) và JOIN bảng gốc rỗng: màn lịch sử
// đọc lại id (cột ID), customer_id (cột khách hàng — khách VẪN còn) và code từ snapshot này.
func withDeleteIdentity(fields []string) []string {
	return append([]string{"id", "code", "customer_id"}, fields...)
}

// --- Appointment ---

var AppointmentIncludeFields = []string{
	"appointment_date", "checkin_date", "checkout_date", "revenue_id",
	"type_service_id", "bookType",
	"content", "contentPrivate",
	"branch_id", "appointment_type_id", "service_id", "source_id",
}
var AppointmentSkipNullFields = []string{"checkin_date", "checkout_date", "type_service_id"}
var AppointmentDeleteFields = withDeleteIdentity(AppointmentIncludeFields)

// --- Payment ---

var PaymentIncludeFields = []string{
	"branch_id", "payment_time", "payment_of_date",
	"amount", "note", "method", "card_type_id", "bank_code",
}
var PaymentSkipNullFields []string // no skip-null fields
var PaymentDeleteFields = withDeleteIdentity(PaymentIncludeFields)

// --- OrderServiceUsed ---

var OrderServiceUsedIncludeFields = []string{
	"service_id", "quantity", "unit_price",
	"service_discount", "service_discount_type", "discount_other", "totalEnd",
	"note", "branch_id", "created_day",
	"appointment_id", "source_id", "listRevenues",
}
var OrderServiceUsedSkipNullFields = []string{
	"appointment_id", "source_id", "note", "listRevenues",
}
var OrderServiceUsedDeleteFields = withDeleteIdentity(OrderServiceUsedIncludeFields)

// --- User ---

var UserIncludeFields = []string{
	"username", "status", "branchId", "allowed_ips",
}

// Story-40 (E6/D15): bỏ theo dõi cột thương hiệu (schema mới story-62 không còn cột).
var UserSkipNullFields = []string{"branchId", "allowed_ips"}
