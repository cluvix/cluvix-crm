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

// customer_stage.type — loại bước trong pipeline tiến trình khách hàng (epic customer-stage-pipeline).
// Phễu, widget khu vực, ràng buộc bước mặc định dựa vào LOẠI, không dựa vào id/tên (id AUTO_INCREMENT
// khác nhau giữa các DB khách).
const (
	CustomerStageTypeOpen = 1 // đang mở
	CustomerStageTypeWon  = 2 // thành công
	CustomerStageTypeLost = 3 // thất bại
)

// customer_stage_history.source — nguồn của một lần đổi bước.
const (
	CustomerStageHistorySourceCreate    = 1 // tạo khách
	CustomerStageHistorySourceImport    = 2 // import khách
	CustomerStageHistorySourceManual    = 3 // người dùng đổi tay
	CustomerStageHistorySourceMigration = 4 // lịch sử khởi tạo do migration ghi
)

// CustomerTimelineEventStageChange — `type` của sự kiện chuyển bước trên timeline khách
// (GET /crm/customer/:id/timeline-customer); FE render theo khoá này, tra tên/màu bước qua Dexie.
const CustomerTimelineEventStageChange = "stage_change"

// CustomerTimelineStageChangeLimit — số lần chuyển bước gần nhất đưa lên timeline (khuôn Limit của
// các builder timeline khác: lịch hẹn 30, nhắc hẹn/đơn 15, thanh toán 20).
const CustomerTimelineStageChangeLimit = 50

// CustomerStageColors — khoá màu hợp lệ của customer_stage.color (bảng màu cố định, không nhận mã hex;
// FE map khoá sang token màu có sẵn).
var CustomerStageColors = []string{"gray", "blue", "teal", "green", "amber", "orange", "red", "purple"}

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
