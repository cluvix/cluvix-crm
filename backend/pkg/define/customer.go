package define

// Default country ID for Vietnam (location_country.id = 237)
const CountryIDVN = 237

// Mã điện thoại quốc gia mặc định (location_country.TelephoneCode của VN) —
// dùng khi dò trùng số điện thoại mà không biết country của record.
const TelephoneCodeVN = "84"

// CustomerPhoneUniqueIndex — tên UNIQUE index chặn trùng SĐT khách trong 1 DB (D-3, migration
// 20261018230737_customer_phone_unique). Cột khoá `customer.phone_unique_key` = 9 chữ số cuối của SĐT
// (bỏ ký tự không phải số) — cùng định danh thuê bao với helpers.PhoneLast9/PhoneMatchCond. Lỗi 1062
// mang tên index này ⇒ trả "customer_duplicate" (helpers.IsCustomerPhoneDuplicate).
const CustomerPhoneUniqueIndex = "uniq_customer_phone_key"

// customer.data_id — dữ liệu mới hay dữ liệu cũ (migrate từ hệ thống khác).
const (
	CustomerDataNew = 1
	CustomerDataOld = 2
)

// GenderLabels maps gender id → display label.
var GenderLabels = map[int64]string{1: "Nam", 2: "Nữ", 3: "Khác"}

// Customer statuses
const (
	CustomerStatusTrash             = 1
	CustomerStatusTicket            = 2
	CustomerStatusOldSystem         = 3
	CustomerStatusLead              = 4
	CustomerStatusDeposit           = 5
	CustomerStatusAppointment       = 6
	CustomerStatusAppointmentCancel = 7
	CustomerStatusVisited           = 8
	CustomerStatusLost              = 9
	CustomerStatusPreCustomer       = 10
	CustomerStatusTreatment         = 11
	CustomerStatusCustomer          = 12
	CustomerStatusComplaint         = 13
	CustomerStatusDoneComplaint     = 14
	CustomerStatusBroker            = 15
	// CustomerStatusContact — status mặc định của MỌI khách mới tạo (D14, crm-standalone:
	// không còn nhánh Lead(4) theo phòng ban), hiển thị ngay trong CRM list. Xem CreateCustomer
	// (crm/customer/handler).
	CustomerStatusContact = 16
	// CustomerStatusZalo — khách tự đăng ký qua Zalo Mini App (findOrCreateCustomer),
	// ưu tiên xử lý sớm. Hiển thị CẢ CRM list lẫn Sale/lead — mirror Contact(16), khác
	// Lead(4) thường (ẩn khỏi CRM). Xem CustomerStatuses + LeadStatuses.
	CustomerStatusZalo = 17
)

// Customer accepted
const CustomerAcceptedOk = 2

// Customer customerType
const (
	CustomerTypeNew = 1
	CustomerTypeOld = 2
)

// OrderPlan orderPlanType
const (
	OrderPlanTypeNew = 1
	OrderPlanTypeOld = 2
)

// Customer statuses for list (exclude lead, ticket, trash)
// LeadStatuses — statuses dùng cho module sale/lead (3–9), + Contact(16)/Zalo(17): khách
// tạo từ phòng ban "contact" hoặc tự đăng ký qua Zalo vẫn cần Sale chăm sóc để lên
// Agreed/Customer, nên vẫn phải thấy ở đây (đồng thời hiện cả CRM list — xem CustomerStatuses).
var LeadStatuses = []int{
	CustomerStatusOldSystem, CustomerStatusLead, CustomerStatusZalo, CustomerStatusDeposit,
	CustomerStatusAppointment, CustomerStatusAppointmentCancel,
	CustomerStatusVisited, CustomerStatusLost, CustomerStatusContact,
}

var CustomerStatuses = []int{
	CustomerStatusOldSystem, CustomerStatusDeposit, CustomerStatusAppointment,
	CustomerStatusAppointmentCancel, CustomerStatusVisited, CustomerStatusLost,
	CustomerStatusPreCustomer, CustomerStatusTreatment, CustomerStatusCustomer,
	CustomerStatusComplaint, CustomerStatusDoneComplaint, CustomerStatusBroker,
	CustomerStatusContact, CustomerStatusZalo,
}

// Dashboard funnel buckets (crm/dashboard CrmStatusCustomerOverview) — status IN (...) tường
// minh, KHÔNG dùng status >= N: giá trị số của status không đảm bảo giữ đúng thứ tự tiến
// trình nghiệp vụ (CustomerStatusContact=16 là giai đoạn sớm nhưng có giá trị số lớn nhất).
var (
	CustomerFunnelTicketUp = []int{
		CustomerStatusTicket, CustomerStatusOldSystem, CustomerStatusLead, CustomerStatusZalo, CustomerStatusDeposit,
		CustomerStatusAppointment, CustomerStatusAppointmentCancel, CustomerStatusVisited, CustomerStatusLost,
		CustomerStatusPreCustomer, CustomerStatusTreatment, CustomerStatusCustomer, CustomerStatusComplaint,
		CustomerStatusDoneComplaint, CustomerStatusBroker,
	}
	CustomerFunnelLeadUp = []int{
		CustomerStatusLead, CustomerStatusZalo, CustomerStatusDeposit, CustomerStatusAppointment, CustomerStatusAppointmentCancel,
		CustomerStatusVisited, CustomerStatusLost, CustomerStatusPreCustomer, CustomerStatusTreatment,
		CustomerStatusCustomer, CustomerStatusComplaint, CustomerStatusDoneComplaint, CustomerStatusBroker,
	}
	CustomerFunnelAppointmentUp = []int{
		CustomerStatusAppointment, CustomerStatusAppointmentCancel, CustomerStatusVisited, CustomerStatusLost,
		CustomerStatusPreCustomer, CustomerStatusTreatment, CustomerStatusCustomer, CustomerStatusComplaint,
		CustomerStatusDoneComplaint, CustomerStatusBroker,
	}
	CustomerFunnelVisitedUp = []int{
		CustomerStatusVisited, CustomerStatusLost, CustomerStatusPreCustomer, CustomerStatusTreatment,
		CustomerStatusCustomer, CustomerStatusComplaint, CustomerStatusDoneComplaint, CustomerStatusBroker,
	}
	CustomerFunnelPreCustomerUp = []int{
		CustomerStatusPreCustomer, CustomerStatusTreatment, CustomerStatusCustomer, CustomerStatusComplaint,
		CustomerStatusDoneComplaint, CustomerStatusBroker,
	}
)

// Allowed customer statuses for appointment-driven transitions
var AllowedAppointmentCustomerStatuses = map[int]bool{
	CustomerStatusTrash:             true,
	CustomerStatusTicket:            true,
	CustomerStatusOldSystem:         true,
	CustomerStatusLead:              true,
	CustomerStatusZalo:              true,
	CustomerStatusDeposit:           true,
	CustomerStatusAppointment:       true,
	CustomerStatusAppointmentCancel: true,
	CustomerStatusVisited:           true,
	CustomerStatusLost:              true,
	CustomerStatusContact:           true,
}

// CustomerStatusTransitions defines valid next statuses from each status.
// nil means no restriction (legacy / open statuses).
// D14 (2026-09-23, QA story-33): không còn đường nào đưa khách vào Lead(4)/Ticket(2) — mọi
// đích trước đây trỏ tới 2 status này đổi thành Contact(16) (status mặc định khách mới, xem
// CustomerStatusContact). Lead(4)/Ticket(2) vẫn còn KEY (FROM) vì record cũ/migrate có thể
// đang đứng ở đó, chỉ không còn ai TIẾN vào nữa.
var CustomerStatusTransitions = map[int][]int{
	CustomerStatusTrash:             {CustomerStatusContact},
	CustomerStatusTicket:            {CustomerStatusContact, CustomerStatusTrash},
	CustomerStatusOldSystem:         nil,
	CustomerStatusLead:              {CustomerStatusDeposit, CustomerStatusAppointment, CustomerStatusLost, CustomerStatusTrash},
	CustomerStatusZalo:              {CustomerStatusDeposit, CustomerStatusAppointment, CustomerStatusLost, CustomerStatusTrash},
	CustomerStatusDeposit:           {CustomerStatusAppointment, CustomerStatusCustomer, CustomerStatusLost, CustomerStatusTrash},
	CustomerStatusAppointment:       {CustomerStatusVisited, CustomerStatusAppointmentCancel, CustomerStatusLost, CustomerStatusTrash},
	CustomerStatusAppointmentCancel: {CustomerStatusAppointment, CustomerStatusContact, CustomerStatusLost, CustomerStatusTrash},
	CustomerStatusVisited:           {CustomerStatusCustomer, CustomerStatusPreCustomer, CustomerStatusLost, CustomerStatusTrash},
	CustomerStatusLost:              {CustomerStatusContact, CustomerStatusTrash},
	CustomerStatusPreCustomer:       {CustomerStatusCustomer, CustomerStatusLost, CustomerStatusTrash},
	CustomerStatusTreatment:         {CustomerStatusCustomer, CustomerStatusComplaint, CustomerStatusDoneComplaint},
	CustomerStatusCustomer:          {CustomerStatusTreatment, CustomerStatusComplaint, CustomerStatusBroker},
	CustomerStatusComplaint:         {CustomerStatusDoneComplaint, CustomerStatusCustomer},
	CustomerStatusDoneComplaint:     {CustomerStatusCustomer, CustomerStatusComplaint},
	CustomerStatusBroker:            {CustomerStatusCustomer},
	CustomerStatusContact:           {CustomerStatusDeposit, CustomerStatusAppointment, CustomerStatusLost, CustomerStatusTrash},
}

// IsValidStatusTransition returns true if transitioning from → to is allowed.
func IsValidStatusTransition(from, to int) bool {
	if from == to {
		return true
	}
	allowed, ok := CustomerStatusTransitions[from]
	if !ok || allowed == nil {
		return true
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// Kiểu mã khách hàng — chọn bằng biến môi trường CUSTOMER_CODE_OPTION trong `.env` (mỗi khách SaaS
// 1 DB + 1 `.env` riêng). Không đặt / giá trị lạ ⇒ CustomerCodeOptionDefault. Ví dụ theo mã
// thương hiệu CLX, chi nhánh Q1 (id 3), tháng 09/2026. Logic sinh: pkg/codegen/customer_code.go.
const (
	CustomerCodeOptionEnv = "CUSTOMER_CODE_OPTION"

	CustomerCodeOptionDefault         = 1 // theo config_code: CLX.Q1.KH.2609.01 — đếm theo chi nhánh
	CustomerCodeOptionBranchMonth     = 2 // Q1.2609.01    — đếm theo chi nhánh, về 1 mỗi tháng
	CustomerCodeOptionBranch          = 3 // Q1.01         — đếm theo chi nhánh
	CustomerCodeOptionBranchID        = 4 // 3.00001       — đếm theo chi nhánh
	CustomerCodeOptionInfix           = 5 // KH.00001      — đếm chung toàn hệ thống
	CustomerCodeOptionBrandInfixMonth = 6 // CLX.KH.2609.001 — đếm chung toàn hệ thống, về 1 mỗi tháng (CLX = mã thương hiệu, brand)
)
