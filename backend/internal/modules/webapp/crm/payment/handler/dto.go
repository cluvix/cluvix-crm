// Payment DTOs — Phase 2 prototype of refactor-roadmap.
//
// Mục đích: thay `map[string]interface{}` input bằng struct typed có
// JSON tag + validation tag. Gin binding auto-validate, lỗi return 422
// qua response.ValidateErr.
//
// Convention (per roadmap §2.1):
//   - DTO sống trong handler package (`xxx_dto.go`).
//   - Suffix Input cho request body, Output cho response (nếu cần shape khác model).
//   - ToXxx() method convert DTO → domain model để pass vào service.
//
// Service layer KHÔNG biết về DTO — chỉ nhận model hoặc primitive params,
// giữ service không phụ thuộc tầng HTTP.

package handler

import (
	paymentDao "erpCluvix/internal/modules/webapp/crm/payment/dao"

	"github.com/shopspring/decimal"
)

// PaymentInput — shared field set cho các payment create endpoint mà
// `payment_for` được service set server-side (deposit, refund, refundKg,
// refundOrderPlan). Clinic dùng `CreatePaymentClinicInput` riêng vì FE
// bắt buộc gửi payment_for (income vs expense).
//
// Story-37 (E6/D15): KHÔNG còn field company_id ở mọi DTO phiếu thu — FE cũ vẫn gửi thì JSON thừa bị
// bỏ qua (nhận rồi bỏ qua, không 400); schema mới (story-62) không còn cột này.
type PaymentInput struct {
	BranchID int `json:"branch_id" binding:"required"`

	Amount decimal.Decimal `json:"amount" binding:"required"`
	Method int             `json:"method"`

	PaymentTime   *int64 `json:"payment_time"`
	PaymentOfDate *int64 `json:"payment_of_date"`

	CardTypeID *int   `json:"card_type_id"`
	BankCode   string `json:"bank_code"`
	Note       string `json:"note"`

	CustomerID    *int `json:"customer_id"`
	ServiceUsedID *int `json:"serviceUsed_id"`
	DepositID     *int `json:"deposit_id"`
	DepositType   *int `json:"deposit_type"`
	AppointmentID *int `json:"appointment_id"`
}

// ToPayment convert DTO → domain Payment. payment_for KHÔNG set ở đây —
// service tự gán theo endpoint (Deposit, Refund, RefundKg, ...).
func (in *PaymentInput) ToPayment() *paymentDao.Payment {
	return &paymentDao.Payment{
		BranchId:      in.BranchID,
		Amount:        in.Amount,
		Method:        in.Method,
		PaymentTime:   in.PaymentTime,
		PaymentOfDate: in.PaymentOfDate,
		CardTypeId:    in.CardTypeID,
		BankCode:      in.BankCode,
		Note:          in.Note,
		CustomerId:    in.CustomerID,
		ServiceUsedId: in.ServiceUsedID,
		DepositId:     in.DepositID,
		DepositType:   in.DepositType,
		AppointmentId: in.AppointmentID,
	}
}

// CreatePaymentOrderInput — POST /payment/order, thanh toán toàn bộ đơn hàng.
// parent_id = order_plan.id; service iterate order_service_used,
// tạo payment cho mỗi service chưa thanh toán đủ.
type CreatePaymentOrderInput struct {
	PaymentInput
	ParentID int `json:"parent_id" binding:"required"`
}

// ToPayment cho PaymentOrder — bao gồm parent_id mapping vào Payment.ParentId.
func (in *CreatePaymentOrderInput) ToPayment() *paymentDao.Payment {
	p := in.PaymentInput.ToPayment()
	p.ParentId = new(in.ParentID)
	return p
}

// CreatePaymentCustomerInput — POST /payment/customer, thanh toán 1+ dịch vụ
// (serviceUsed_idArr). amount phải <= tổng (totalEnd + vat_amount - payment) các dịch vụ
// đã chọn; nếu trả thiếu, service phân bổ waterfall theo created_day ASC
// (dịch vụ cũ được trả đủ trước) — xem CreatePaymentCustomerFromPayment.
type CreatePaymentCustomerInput struct {
	PaymentInput
	ServiceUsedIDArr []int `json:"serviceUsed_idArr" binding:"required,min=1"`
}

// ToPayment cho PaymentCustomer — không set parent_id, service iterate svc ids
// và override Amount + ServiceUsedId per loop. Pass PaymentInput-shaped Payment
// để service inject thêm.
func (in *CreatePaymentCustomerInput) ToPayment() *paymentDao.Payment {
	return in.PaymentInput.ToPayment()
}

// MoveDepositToPaymentInput — POST /payment/move-deposit/:id, chuyển tiền cọc
// thành thanh toán. Subset của PaymentInput vì service tự set nhiều field
// (customer_id, payment_time copy từ deposit gốc; method = Transfer).
type MoveDepositToPaymentInput struct {
	Amount        decimal.Decimal `json:"amount" binding:"required"`
	BranchID      int             `json:"branch_id" binding:"required"`
	ServiceUsedID *int            `json:"serviceUsed_id"`
	PaymentOfDate *int64          `json:"payment_of_date"`
	ParentID      *int            `json:"parent_id"`
}

// ToPayment build Payment struct chứa các field từ DTO. Service tự set
// các field từ deposit gốc (customer_id, payment_time, method, payment_for).
func (in *MoveDepositToPaymentInput) ToPayment() *paymentDao.Payment {
	return &paymentDao.Payment{
		Amount:        in.Amount,
		BranchId:      in.BranchID,
		ServiceUsedId: in.ServiceUsedID,
		PaymentOfDate: in.PaymentOfDate,
		ParentId:      in.ParentID,
	}
}

// UpdatePaymentInput — shared cho UpdatePaymentCustomer + UpdateDepositCustomer.
// Cùng shape với PaymentInput + UpdatedAt cho optimistic lock (D6).
//
// String fields nullable (note, bank_code) dùng *string để
// phân biệt 3 trạng thái FE: (a) không gửi → giữ nguyên, (b) gửi null → clear,
// (c) gửi value → set. Nếu dùng `string`, FE omit field sẽ silent overwrite "".
type UpdatePaymentInput struct {
	UpdatedAt int64 `json:"updated_at" binding:"required"`

	BranchID int `json:"branch_id" binding:"required"`

	Amount     decimal.Decimal `json:"amount" binding:"required"`
	Method     int             `json:"method"`
	PaymentFor *int            `json:"payment_for"`

	PaymentTime   *int64 `json:"payment_time"`
	PaymentOfDate *int64 `json:"payment_of_date"`

	CardTypeID *int    `json:"card_type_id"`
	BankCode   *string `json:"bank_code"`
	Note       *string `json:"note"`

	CustomerID    *int `json:"customer_id"`
	ServiceUsedID *int `json:"serviceUsed_id"`
	DepositID     *int `json:"deposit_id"`
	DepositType   *int `json:"deposit_type"`
	AppointmentID *int `json:"appointment_id"`
}

// ToParams convert Update DTO → params map cho legacy UpdatePaymentCustomer.
// Key chỉ xuất hiện nếu FE thực sự gửi field đó — preserve "absent vs set"
// để mergePaymentParams chỉ touch field nào FE chỉ định.
//
// Pointer-nil → key vắng mặt (giữ nguyên DB value).
// Non-nil pointer → key có, value = deref (kể cả "" hay 0 = clear field).
func (in *UpdatePaymentInput) ToParams() map[string]interface{} {
	m := map[string]interface{}{
		"updated_at": in.UpdatedAt,
		"amount":     in.Amount,
		"method":     in.Method,
		"branch_id":  in.BranchID,
	}
	if in.PaymentFor != nil {
		m["payment_for"] = *in.PaymentFor
	}
	if in.PaymentTime != nil {
		m["payment_time"] = *in.PaymentTime
	}
	if in.PaymentOfDate != nil {
		m["payment_of_date"] = *in.PaymentOfDate
	}
	if in.CardTypeID != nil {
		m["card_type_id"] = *in.CardTypeID
	}
	if in.BankCode != nil {
		m["bank_code"] = *in.BankCode
	}
	if in.Note != nil {
		m["note"] = *in.Note
	}
	if in.DepositType != nil {
		m["deposit_type"] = *in.DepositType
	}
	return m
}

// CreatePaymentClinicInput — payload cho POST /payment (thu/chi phòng khám).
//
// payment_for: 7 (clinic income) hoặc 8 (clinic expense) — service validate
// nghiệp vụ; binding ở đây chỉ enforce required + type.
type CreatePaymentClinicInput struct {
	BranchID int `json:"branch_id" binding:"required"`

	Amount     decimal.Decimal `json:"amount" binding:"required"`
	Method     int             `json:"method"`
	PaymentFor *int            `json:"payment_for" binding:"required"`

	PaymentTime   *int64 `json:"payment_time"`
	PaymentOfDate *int64 `json:"payment_of_date"`

	CardTypeID *int   `json:"card_type_id"`
	BankCode   string `json:"bank_code"`
	Note       string `json:"note"`

	// Các field FK — chấp nhận từ FE để giữ shape backwards-compatible với
	// map-based API cũ. Cho clinic income/expense, service sẽ nil hoá
	// customer_id / serviceUsed_id trong beforeSave.
	CustomerID    *int `json:"customer_id"`
	ServiceUsedID *int `json:"serviceUsed_id"`
	DepositID     *int `json:"deposit_id"`
	AppointmentID *int `json:"appointment_id"`

	// EMR / BHYT.
}

// UpdatePaymentClinicInput — payload cho PUT /payment/:id (thu/chi phòng khám).
// Giống CreatePaymentClinicInput + UpdatedAt cho optimistic locking (D6).
type UpdatePaymentClinicInput struct {
	UpdatedAt int64 `json:"updated_at" binding:"required"`

	// Cùng các field như Create — nhưng tất cả optional ở update (partial)?
	// Hiện service merge field-by-field, FE thường gửi full payload.
	// Giữ pattern required giống Create để CHẶN payload thiếu (ngăn accidental clear).
	BranchID int `json:"branch_id" binding:"required"`

	Amount     decimal.Decimal `json:"amount" binding:"required"`
	Method     int             `json:"method"`
	PaymentFor *int            `json:"payment_for" binding:"required"`

	PaymentTime   *int64 `json:"payment_time"`
	PaymentOfDate *int64 `json:"payment_of_date"`

	CardTypeID *int   `json:"card_type_id"`
	BankCode   string `json:"bank_code"`
	Note       string `json:"note"`

	CustomerID    *int `json:"customer_id"`
	ServiceUsedID *int `json:"serviceUsed_id"`
	DepositID     *int `json:"deposit_id"`
	AppointmentID *int `json:"appointment_id"`
}

// ToPayment convert UpdatePaymentClinicInput → domain Payment.
// Set ID via caller (URL param). UpdatedAt từ DTO body (FE gửi up).
func (in *UpdatePaymentClinicInput) ToPayment() *paymentDao.Payment {
	return &paymentDao.Payment{
		UpdatedAt:     in.UpdatedAt,
		BranchId:      in.BranchID,
		Amount:        in.Amount,
		Method:        in.Method,
		PaymentFor:    in.PaymentFor,
		PaymentTime:   in.PaymentTime,
		PaymentOfDate: in.PaymentOfDate,
		CardTypeId:    in.CardTypeID,
		BankCode:      in.BankCode,
		Note:          in.Note,
		CustomerId:    in.CustomerID,
		ServiceUsedId: in.ServiceUsedID,
		DepositId:     in.DepositID,
		AppointmentId: in.AppointmentID,
	}
}

// ToPayment converts the input DTO to the domain Payment model for service
// consumption. Tách rõ tầng HTTP (DTO) khỏi tầng domain (model) — service
// không thấy field từ FE, chỉ thấy entity đã được validate ở tầng binding.
func (in *CreatePaymentClinicInput) ToPayment() *paymentDao.Payment {
	return &paymentDao.Payment{
		BranchId:      in.BranchID,
		Amount:        in.Amount,
		Method:        in.Method,
		PaymentFor:    in.PaymentFor,
		PaymentTime:   in.PaymentTime,
		PaymentOfDate: in.PaymentOfDate,
		CardTypeId:    in.CardTypeID,
		BankCode:      in.BankCode,
		Note:          in.Note,
		CustomerId:    in.CustomerID,
		ServiceUsedId: in.ServiceUsedID,
		DepositId:     in.DepositID,
		AppointmentId: in.AppointmentID,
	}
}
