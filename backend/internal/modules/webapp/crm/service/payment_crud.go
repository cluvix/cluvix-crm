// Payment CRUD methods — convert từ PHP PaymentController actions.
//
// Create: CreatePaymentCustomer, CreateDepositCustomer, CreateRefundDepositCustomer,
//
//	CreateRefundOrderPlan, MoveDepositToPayment
//
// Update: UpdatePaymentCustomer, UpdateDepositCustomer
// Delete: DeletePayment, DeleteDeposit, DeleteRefundCustomer, DeleteRefundOrderPlan
package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	crmPaymentDao "erpCluvix/internal/modules/webapp/crm/payment/dao"
	history "erpCluvix/internal/modules/webapp/history/log/service"
	"erpCluvix/pkg/codegen"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/optlock"
)

// --- Create ---

// CreatePaymentClinic: tạo phiếu thu/chi phòng khám (payment_for = FOR_CLINIC_INCOME hoặc FOR_CLINIC_EXPENSE)
//
// Deprecated: Phase 2 DTO migration — handler nên truyền *Payment trực tiếp qua
// CreatePaymentClinicFromPayment. Method này giữ cho legacy/internal callers
// (nếu có) — kết quả gọi xuống cùng path validation.
func (s *PaymentService) CreatePaymentClinic(params map[string]interface{}, userID int) (*crmPaymentDao.Payment, error) {
	return s.CreatePaymentClinicFromPayment(s.bindPayment(params), userID)
}

// CreatePaymentClinicFromPayment — typed entry point (Phase 2 DTO).
// Caller (handler) đã convert DTO → *Payment qua CreatePaymentClinicInput.ToPayment().
// Service chỉ chịu trách nhiệm validate nghiệp vụ + persist.
func (s *PaymentService) CreatePaymentClinicFromPayment(p *crmPaymentDao.Payment, userID int) (*crmPaymentDao.Payment, error) {
	if p.PaymentFor == nil {
		return nil, errs.ErrMissingPaymentFor
	}
	if *p.PaymentFor != define.PaymentForClinicIncome && *p.PaymentFor != define.PaymentForClinicExpense {
		return nil, errs.ErrInvalidPaymentFor
	}
	orig := *p
	err := s.txMoney(func(tx *gorm.DB) error {
		*p = orig
		return s.createPayment(tx, p, userID)
	})
	return p, err
}

// CreatePaymentOrder: thanh toán toàn bộ đơn hàng (giống PHP actionCreatePaymentOrder)
//
// Deprecated: Phase 2 DTO — handler nên dùng CreatePaymentOrderFromPayment.
func (s *PaymentService) CreatePaymentOrder(params map[string]interface{}, userID int) (string, error) {
	parentId := int(toInt64(params["parent_id"]))
	if parentId == 0 {
		return "", errs.ErrMissingParentID
	}
	p := s.bindPayment(params)
	p.ParentId = new(parentId)
	return s.CreatePaymentOrderFromPayment(p, userID)
}

// CreatePaymentOrderFromPayment — typed entry. ParentID = order_plan.id.
// Service iterate order_service_used, tạo payment cho mỗi service còn nợ.
//
// Race-safety: đọc số nợ còn lại của TỪNG dịch vụ + tạo payment PHẢI nằm cùng 1 transaction,
// khoá `FOR UPDATE` các dòng order_service_used liên quan — chặn double-submit (double-click,
// nhiều tab) tạo trùng payment cho cùng 1 đơn. Request thứ 2 bị block tới khi request thứ 1
// commit (đã cập nhật order_service_used.payment qua calculatorPaymentServiceUsed trong cùng
// tx), sau đó đọc lại thấy nợ = 0 → tự bỏ qua, không tạo trùng.
func (s *PaymentService) CreatePaymentOrderFromPayment(template *crmPaymentDao.Payment, userID int) (string, error) {
	if template.ParentId == nil || *template.ParentId == 0 {
		return "", errs.ErrMissingParentID
	}
	parentId := *template.ParentId

	// Đếm tổng dịch vụ của đơn (không filter isLock) để phân biệt "đơn không tồn tại/không có
	// dịch vụ nào" (order_not_found) với "đơn tồn tại nhưng mọi dịch vụ đã bị khoá, không còn
	// gì để thanh toán" (no_payable_service) — 2 message khác nghĩa, tránh gây hiểu lầm.
	var totalServiceCount int64
	if err := s.DB.Table("order_service_used").Where("order_plan_id = ?", parentId).
		Count(&totalServiceCount).Error; err != nil {
		return "", err
	}
	if totalServiceCount == 0 {
		return "", errs.ErrOrderNotFound
	}

	type svcInfo struct {
		ID         int             `gorm:"column:id"`
		CustomerID int             `gorm:"column:customer_id"`
		Payable    decimal.Decimal `gorm:"column:payable"` // totalEnd + VAT (bmad/order-vat D4)
		Payment    decimal.Decimal `gorm:"column:payment"`
	}

	// Khách của đơn — đọc NGOÀI tx chỉ để biết khoá ai; dưới khoá đối chiếu lại (lens-money M-2).
	custIDs, err := serviceUsedCustomers(s.DB, "order_plan_id = ?", parentId)
	if err != nil {
		return "", err
	}

	err = s.txMoney(func(tx *gorm.DB) error {
		if err := lockCustomers(tx, custIDs...); err != nil {
			return err
		}
		var svcs []svcInfo
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Table("order_service_used").
			Select("id, customer_id, "+osuPayableSQL+" AS payable, COALESCE(payment, 0) as payment").
			Where("order_plan_id = ? AND isLock = 1", parentId).Order("id").Find(&svcs).Error; err != nil {
			return err
		}
		if len(svcs) == 0 {
			return errs.ErrNoPayableService
		}
		for _, su := range svcs {
			if !slices.Contains(custIDs, su.CustomerID) {
				return errs.ErrPaymentBusyRetry // đơn đổi khách giữa lúc đọc và lúc khoá
			}
		}

		for _, su := range svcs {
			amount := su.Payable.Sub(su.Payment)
			if !amount.IsPositive() {
				continue
			}
			// Clone template per iteration (shared struct sẽ bị mutate qua beforeSave).
			p := *template
			p.ServiceUsedId = &su.ID
			p.Amount = amount
			p.CustomerId = &su.CustomerID
			p.PaymentFor = new(define.PaymentForOrder)

			if err := s.createPayment(tx, &p, userID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return "order_payment_success", nil
}

// CreatePaymentCustomer: thanh toán cho 1 hoặc nhiều dịch vụ (giống PHP actionCreatePaymentCustomer)
//
// Deprecated: Phase 2 DTO — handler nên dùng CreatePaymentCustomerFromPayment.
func (s *PaymentService) CreatePaymentCustomer(params map[string]interface{}, userID int) (string, error) {
	serviceUsedIdArr, ok := params["serviceUsed_idArr"].([]interface{})
	if !ok || len(serviceUsedIdArr) == 0 {
		return "", errs.ErrInvalidServiceList
	}

	ids := make([]int, len(serviceUsedIdArr))
	for i, v := range serviceUsedIdArr {
		ids[i] = int(toInt64(v))
	}
	return s.CreatePaymentCustomerFromPayment(s.bindPayment(params), ids, userID)
}

// CreatePaymentCustomerFromPayment — typed entry. ids = serviceUsed_idArr.
// amount phải <= tổng (phải thu − payment; phải thu = totalEnd + VAT) các dịch vụ đã chọn; nếu trả thiếu,
// phân bổ waterfall theo created_day ASC (dịch vụ cũ trả đủ trước, dư mới
// chảy qua dịch vụ tiếp theo — không chia tỷ lệ).
//
// Race-safety: đọc số nợ + tạo payment PHẢI cùng 1 transaction, khoá `FOR UPDATE`
// các dòng order_service_used liên quan — chặn double-submit tạo trùng/vượt payment
// (cùng lý do với CreatePaymentOrderFromPayment ở trên).
func (s *PaymentService) CreatePaymentCustomerFromPayment(template *crmPaymentDao.Payment, ids []int, userID int) (string, error) {
	if len(ids) == 0 {
		return "", errs.ErrInvalidServiceList
	}

	amount := template.Amount
	// Guard: Không cho phép amount âm hoặc bằng 0
	if !amount.IsPositive() {
		return "", errs.ErrAmountMustBePositive
	}
	count := len(ids)

	// Khách của các dịch vụ — đọc NGOÀI tx chỉ để biết khoá ai; dưới khoá đối chiếu lại. Khoá customer
	// TRƯỚC dòng dịch vụ: 6 phiếu song song cho 6 dịch vụ khác nhau của cùng một khách trước đây chỉ khoá
	// 6 dòng khác nhau ⇒ tổng của khách/đơn bị ghi đè (lens-money M-2).
	custIDs, err := serviceUsedCustomers(s.DB, "id IN ?", ids)
	if err != nil {
		return "", err
	}

	return "Tạo mới thanh toán thành công", s.txMoney(func(tx *gorm.DB) error {
		if err := lockCustomers(tx, custIDs...); err != nil {
			return err
		}
		var svcs []serviceUsedDebt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Table("order_service_used").
			Select("id, customer_id, "+osuPayableSQL+" AS payable, COALESCE(payment, 0) as payment, created_day").
			Where("id IN ?", ids).Order("id").Find(&svcs).Error; err != nil {
			return err
		}
		svcMap := map[int]serviceUsedDebt{}
		for _, su := range svcs {
			if !slices.Contains(custIDs, su.CustomerID) {
				return errs.ErrPaymentBusyRetry // dịch vụ đổi khách giữa lúc đọc và lúc khoá
			}
			svcMap[su.ID] = su
		}

		// Validate
		for _, id := range ids {
			if _, found := svcMap[id]; !found {
				return errs.NewLocalizedError("service_id_invalid", id)
			}
		}

		if count > 1 {
			// Validate customer_id nhất quán (giống PHP: các dịch vụ phải thuộc cùng 1 khách)
			customerIdSet := map[int]bool{}
			for _, su := range svcMap {
				customerIdSet[su.CustomerID] = true
			}
			if len(customerIdSet) > 1 {
				return errs.ErrServicesMultipleCustomers
			}
		}

		totalRequired := decimal.Zero
		for _, su := range svcMap {
			totalRequired = totalRequired.Add(su.Payable.Sub(su.Payment))
		}
		// amount không được vượt tổng nợ (đọc dưới khoá FOR UPDATE, đã trừ mọi
		// payment vừa commit từ request khác) — cho phép trả thiếu, phân bổ
		// waterfall theo thứ tự created_day ASC bên dưới.
		if amount.GreaterThan(totalRequired) {
			return errs.NewLocalizedError("payment_amount_too_large", amount.String(), totalRequired.String())
		}

		allocations := allocatePaymentWaterfall(ids, svcMap, amount)
		for _, alloc := range allocations {
			// Clone template per iteration — beforeSave mutates fields.
			p := *template
			p.ServiceUsedId = new(alloc.ID)
			p.PaymentFor = new(define.PaymentForOrder)
			p.Amount = alloc.Amount

			if err := s.createPayment(tx, &p, userID); err != nil {
				return err
			}
		}
		return nil
	})
}

// serviceUsedCustomers — customer_id (khác nhau, ≠ 0) của các dòng order_service_used khớp điều kiện.
// Đọc ngoài tx để biết cần khoá khách nào trước; caller PHẢI đối chiếu lại dưới khoá.
func serviceUsedCustomers(db *gorm.DB, where string, args ...any) ([]int, error) {
	var ids []int
	err := db.Table("order_service_used").Where(where, args...).
		Where("customer_id IS NOT NULL").Distinct().Pluck("customer_id", &ids).Error
	return ids, err
}

// serviceUsedDebt — dòng order_service_used cần biết để phân bổ payment: nợ
// còn lại (phải thu − payment; phải thu = totalEnd + VAT) + created_day làm khoá thứ tự waterfall.
type serviceUsedDebt struct {
	ID         int             `gorm:"column:id"`
	CustomerID int             `gorm:"column:customer_id"`
	Payable    decimal.Decimal `gorm:"column:payable"`
	Payment    decimal.Decimal `gorm:"column:payment"`
	CreatedDay *int64          `gorm:"column:created_day"`
}

type paymentAllocation struct {
	ID     int
	Amount decimal.Decimal
}

// allocatePaymentWaterfall — phân bổ 1 khoản `amount` cho nhiều dịch vụ đang
// nợ theo thứ tự created_day ASC (id làm tie-break, created_day nil coi như
// cũ nhất): trả ĐỦ dịch vụ đứng trước rồi mới chảy phần dư qua dịch vụ sau,
// KHÔNG chia tỷ lệ. Caller phải đảm bảo amount <= tổng nợ trước khi gọi.
//
// Hàm pure (không I/O) để unit-test độc lập logic tiền — xem payment_test.go.
func allocatePaymentWaterfall(ids []int, svcMap map[int]serviceUsedDebt, amount decimal.Decimal) []paymentAllocation {
	orderedIds := make([]int, len(ids))
	copy(orderedIds, ids)
	sort.Slice(orderedIds, func(i, j int) bool {
		a, b := svcMap[orderedIds[i]], svcMap[orderedIds[j]]
		aDay, bDay := int64(0), int64(0)
		if a.CreatedDay != nil {
			aDay = *a.CreatedDay
		}
		if b.CreatedDay != nil {
			bDay = *b.CreatedDay
		}
		if aDay != bDay {
			return aDay < bDay
		}
		return a.ID < b.ID
	})

	allocations := make([]paymentAllocation, 0, len(orderedIds))
	remaining := amount
	for _, id := range orderedIds {
		if !remaining.IsPositive() {
			break
		}
		su := svcMap[id]
		due := su.Payable.Sub(su.Payment)
		if !due.IsPositive() {
			continue
		}
		pay := remaining
		if pay.GreaterThan(due) {
			pay = due
		}

		allocations = append(allocations, paymentAllocation{ID: id, Amount: pay})
		remaining = remaining.Sub(pay)
	}
	return allocations
}

// CreateDepositCustomer: đặt cọc (giống PHP actionCreateDepositCustomer)
//
// Deprecated: Phase 2 DTO — handler nên dùng CreateDepositCustomerFromPayment.
func (s *PaymentService) CreateDepositCustomer(params map[string]interface{}, userID int) (*crmPaymentDao.Payment, error) {
	return s.CreateDepositCustomerFromPayment(s.bindPayment(params), userID)
}

// CreateDepositCustomerFromPayment — typed entry cho Phase 2 DTO.
func (s *PaymentService) CreateDepositCustomerFromPayment(p *crmPaymentDao.Payment, userID int) (*crmPaymentDao.Payment, error) {
	p.PaymentFor = new(define.PaymentForDeposit)
	p.DepositAfter = new(p.Amount)
	// Default deposit_type = Reserve nếu FE không gửi — bắt buộc để updateCustomer
	// nhánh totalDeposit chạy đúng (else rơi vào nhánh ký gửi sai).
	if p.DepositType == nil {
		p.DepositType = new(define.PaymentTypeDepositReserve)
	}

	orig := *p
	err := s.txMoney(func(tx *gorm.DB) error {
		*p = orig
		if err := lockCustomers(tx, derefInt(p.CustomerId)); err != nil {
			return err
		}
		return s.createPayment(tx, p, userID)
	})
	return p, err
}

// CreateRefundDepositCustomer: hoàn cọc (giống PHP actionCreateRefundDepositCustomer)
// PHP flow: check depositAfter >= amount → save refund (amount bị negate trong beforeSave)
//
//	→ update deposit.deposit_after -= amount_gốc → afterSave → calculatorDeposit
//
// BUG FIX: không update deposit_after trực tiếp nữa vì afterSaveAll → calculatorDeposit đã tính lại.
//
//	Nếu update trực tiếp + calculatorDeposit = double update → sai.
//
// validateDepositUse — phiếu DÙNG cọc (hoàn cọc, trả dịch vụ bằng cọc) chỉ được dùng cọc
// của CHÍNH khách đó; dòng dịch vụ / đơn được trả (nếu có) cũng phải của khách đó (story-37 bỏ vế
// "cùng công ty" — D15 1 thương hiệu/DB).
// deposit_id / serviceUsed_id / parent_id đến từ body ⇒ thiếu guard này là hoàn/dùng được cọc của khách
// khác (IDOR body-id), và xoá khách chủ cọc (DeleteCustomerWithPayments) làm phiếu của khách kia mất
// liên kết cọc. customerID nil ⇒ không so vế đó. Trả customer_id của phiếu cọc để caller điền
// khi phiếu mới chưa có.
func (s *PaymentService) validateDepositUse(depositID int, customerID *int, serviceUsedID, parentID *int) (*int, error) {
	var dep struct {
		CustomerID *int `gorm:"column:customer_id"`
		PaymentFor *int `gorm:"column:payment_for"`
	}
	if err := s.DB.Table("payment").Select("customer_id, payment_for").
		Where("id = ?", depositID).Take(&dep).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.ErrDepositNotFound
		}
		return nil, err
	}
	if dep.PaymentFor == nil || *dep.PaymentFor != define.PaymentForDeposit || dep.CustomerID == nil {
		return nil, errs.ErrDepositNotFound
	}
	if customerID != nil && *customerID != *dep.CustomerID {
		return nil, errs.ErrDepositCustomerMismatch
	}
	for _, ref := range []struct {
		table string
		id    *int
	}{{"order_service_used", serviceUsedID}, {"order_plan", parentID}} {
		if ref.id == nil || *ref.id <= 0 {
			continue
		}
		var owner struct {
			CustomerID *int `gorm:"column:customer_id"`
		}
		if err := s.DB.Table(ref.table).Select("customer_id").Where("id = ?", *ref.id).Take(&owner).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errs.ErrDepositCustomerMismatch
			}
			return nil, err
		}
		if owner.CustomerID == nil || *owner.CustomerID != *dep.CustomerID {
			return nil, errs.ErrDepositCustomerMismatch
		}
	}
	return dep.CustomerID, nil
}

// Deprecated: Phase 2 DTO — handler nên dùng CreateRefundDepositCustomerFromPayment.
func (s *PaymentService) CreateRefundDepositCustomer(params map[string]interface{}, userID int) (*crmPaymentDao.Payment, error) {
	return s.CreateRefundDepositCustomerFromPayment(s.bindPayment(params), userID)
}

func (s *PaymentService) CreateRefundDepositCustomerFromPayment(p *crmPaymentDao.Payment, userID int) (*crmPaymentDao.Payment, error) {
	p.PaymentFor = new(define.PaymentForRefundDeposit)

	if p.DepositId == nil {
		return nil, errs.ErrMissingDepositID
	}
	owner, vErr := s.validateDepositUse(*p.DepositId, p.CustomerId, nil, nil)
	if vErr != nil {
		return nil, vErr
	}
	if p.CustomerId == nil {
		p.CustomerId = owner
	}

	return p, s.refundFromDeposit(p, *owner, errs.ErrRefundExceedsDeposit, userID)
}

// refundFromDeposit — lõi hoàn cọc (lens-money M-1). Số dư `deposit_after` đọc và so
// TRONG transaction, dưới khoá customer → phiếu cọc gốc (FOR UPDATE), nên N request hoàn song song
// trên cùng một cọc chạy tuần tự: request sau thấy số dư đã trừ. afterSaveAll tính lại số dư và trả
// lỗi nếu âm (lưới thứ hai). amount lúc gọi vẫn DƯƠNG (beforeSave mới đổi dấu).
func (s *PaymentService) refundFromDeposit(p *crmPaymentDao.Payment, ownerID int, exceeds error, userID int) error {
	if !p.Amount.IsPositive() {
		return errs.ErrAmountMustBePositive
	}
	orig := *p
	return s.txMoney(func(tx *gorm.DB) error {
		*p = orig
		if err := lockCustomers(tx, ownerID); err != nil {
			return err
		}
		depositAfter, err := lockDepositAfter(tx, *p.DepositId)
		if err != nil {
			return err
		}
		if p.Amount.GreaterThan(depositAfter) {
			return exceeds
		}
		// createPayment → beforeSave (đổi dấu) → save → afterSaveAll → calculatorDeposit* (tính lại số dư)
		return s.createPayment(tx, p, userID)
	})
}

// Deprecated: Phase 2 DTO — handler nên dùng CreateRefundOrderPlanFromPayment.
func (s *PaymentService) CreateRefundOrderPlan(params map[string]interface{}, userID int) (*crmPaymentDao.Payment, error) {
	return s.CreateRefundOrderPlanFromPayment(s.bindPayment(params), userID)
}

func (s *PaymentService) CreateRefundOrderPlanFromPayment(p *crmPaymentDao.Payment, userID int) (*crmPaymentDao.Payment, error) {
	p.PaymentFor = new(define.PaymentForRefundService)

	if p.ServiceUsedId == nil {
		return nil, errs.ErrMissingServiceUsedID
	}

	if !p.Amount.IsPositive() {
		return nil, errs.ErrAmountMustBePositive
	}
	sid := *p.ServiceUsedId
	// Khách của dòng dịch vụ — đọc ngoài tx để biết khoá ai, đối chiếu lại dưới khoá.
	osuCust, err := serviceUsedCustomers(s.DB, "id = ?", sid)
	if err != nil {
		return nil, err
	}
	if len(osuCust) != 1 {
		return nil, errs.NewLocalizedError("service_id_invalid", sid)
	}

	// lens-money M-3: "đã thu" đọc và so TRONG tx dưới khoá customer → dòng dịch vụ ⇒ N lần hoàn song
	// song không vượt được số đã thu ròng (thanh toán + chuyển cọc − các lần hoàn trước).
	orig := *p
	err = s.txMoney(func(tx *gorm.DB) error {
		*p = orig
		if err := lockCustomers(tx, osuCust[0]); err != nil {
			return err
		}
		su, err := lockServiceUsed(tx, sid)
		if err != nil {
			return err
		}
		if derefInt(su.CustomerID) != osuCust[0] {
			return errs.ErrPaymentBusyRetry
		}
		if p.CustomerId == nil {
			p.CustomerId = su.CustomerID
		} else if *p.CustomerId != osuCust[0] {
			return errs.NewLocalizedError("service_id_invalid", sid)
		}
		netPaid, err := serviceUsedNetPaid(tx, sid)
		if err != nil {
			return err
		}
		if p.Amount.GreaterThan(netPaid) {
			return errs.ErrRefundExceedsPaid
		}
		return s.createPayment(tx, p, userID)
	})
	return p, err
}

// MoveDepositToPaymentFromPayment — typed entry. depositID từ URL param.
// in *Payment chỉ chứa các field từ DTO (Amount, BranchId, ServiceUsedId,
// PaymentOfDate, ParentId). Service tự set customer_id + payment_time từ
// deposit gốc, method = Transfer, payment_for = MoveDeposit.
//
// lens-money M-1/M-4: số dư cọc VÀ nợ còn lại của dịch vụ đều đọc + so TRONG tx, dưới khoá
// customer → order_service_used → phiếu cọc gốc. Chuyển cọc không được vượt nợ còn lại của dịch vụ
// (phải thu − payment, phải thu = totalEnd + VAT; cùng guard với CreatePaymentCustomerFromPayment) — trước đây chỉ so số dư cọc
// ⇒ chuyển 5tr vào dịch vụ 1tr làm công nợ âm 4tr.
func (s *PaymentService) MoveDepositToPaymentFromPayment(depositID int, in *crmPaymentDao.Payment, userID int) (string, error) {
	var deposit struct {
		CustomerID  *int   `gorm:"column:customer_id"`
		PaymentTime *int64 `gorm:"column:payment_time"`
	}
	if err := s.DB.Table("payment").Select("customer_id, payment_time").Where("id = ?", depositID).Take(&deposit).Error; err != nil {
		return "", errs.ErrDepositNotFound
	}

	if !in.Amount.IsPositive() {
		return "", errs.ErrAmountMustBePositive
	}
	// Chuyển cọc là trả cho MỘT dòng dịch vụ — không có dòng dịch vụ thì không có nợ nào để trừ.
	if in.ServiceUsedId == nil || *in.ServiceUsedId <= 0 {
		return "", errs.ErrMissingServiceUsedID
	}
	// customer_id của phiếu mới lấy từ phiếu cọc, nhưng serviceUsed_id / parent_id từ body ⇒ phải cùng khách.
	owner, err := s.validateDepositUse(depositID, nil, in.ServiceUsedId, in.ParentId)
	if err != nil {
		return "", err
	}
	sid := *in.ServiceUsedId

	return "Chuyển cọc thành công", s.txMoney(func(tx *gorm.DB) error {
		if err := lockCustomers(tx, *owner); err != nil {
			return err
		}
		su, err := lockServiceUsed(tx, sid)
		if err != nil {
			return err
		}
		if derefInt(su.CustomerID) != *owner {
			return errs.ErrDepositCustomerMismatch
		}
		debt := su.Payable.Sub(su.Payment)
		if in.Amount.GreaterThan(debt) {
			return errs.NewLocalizedError("payment_amount_too_large", in.Amount.String(), debt.String())
		}
		depositAfter, err := lockDepositAfter(tx, depositID)
		if err != nil {
			return err
		}
		if depositAfter.LessThan(in.Amount) {
			return errs.ErrTransferExceedsDeposit
		}

		// Build payment từ inputs + deposit-derived fields (mới mỗi lượt ⇒ chạy lại được khi retry).
		p := &crmPaymentDao.Payment{
			CustomerId:    deposit.CustomerID,
			Amount:        in.Amount,
			PaymentTime:   deposit.PaymentTime,
			DepositId:     &depositID,
			Method:        define.PaymentMethodTransferDeposit,
			BranchId:      in.BranchId,
			ServiceUsedId: in.ServiceUsedId,
			PaymentOfDate: in.PaymentOfDate,
			ParentId:      in.ParentId,
		}
		p.PaymentFor = new(define.PaymentForMoveDeposit)

		return s.createPayment(tx, p, userID)
	})
}

// MoveDepositToPayment: chuyển cọc thành thanh toán (giống PHP actionMoveDepositToPayment)
// BUG FIX: thêm parent_id từ params (PHP: $params['parent_id'])
//
// Deprecated: Phase 2 DTO — handler nên dùng MoveDepositToPaymentFromInput. Nay chỉ map params rồi đi
// chung đường typed (cùng khoá + guard nợ dịch vụ — lens-money M-4).
func (s *PaymentService) MoveDepositToPayment(depositID int, params map[string]interface{}, userID int) (string, error) {
	in := &crmPaymentDao.Payment{Amount: helpers.ToDecimal(params["amount"])}
	if v, ok := params["branch_id"]; ok {
		in.BranchId = int(toInt64(v))
	}
	if v, ok := params["serviceUsed_id"]; ok && v != nil {
		in.ServiceUsedId = new(int(toInt64(v)))
	}
	if v, ok := params["payment_of_date"]; ok && v != nil {
		in.PaymentOfDate = new(toInt64(v))
	}
	if v, ok := params["parent_id"]; ok && v != nil {
		in.ParentId = new(int(toInt64(v)))
	}
	return s.MoveDepositToPaymentFromPayment(depositID, in, userID)
}

// --- Update ---

// UpdatePaymentClinicFromPayment — typed entry cho UpdatePaymentClinic handler.
// Validate payment_for thuộc Clinic Income/Expense, sau đó delegate xuống
// UpdatePaymentCustomer (vì merge logic + updated_at check + log diff shared
// giữa clinic và customer payment update).
//
// Phase 2 DTO migration: bridge từ typed DTO sang map-based core. Khi nào
// migrate UpdatePaymentCustomer hoàn toàn sang typed, gộp 2 method này lại.
func (s *PaymentService) UpdatePaymentClinicFromPayment(id uint, in *crmPaymentDao.Payment, userID int) (*crmPaymentDao.Payment, error) {
	if in.PaymentFor == nil {
		return nil, errs.ErrMissingPaymentFor
	}
	if *in.PaymentFor != define.PaymentForClinicIncome && *in.PaymentFor != define.PaymentForClinicExpense {
		return nil, errs.ErrInvalidPaymentFor
	}
	// Bản ghi đang sửa cũng phải là phiếu thu/chi phòng khám (payment_for trong payload KHÔNG được merge
	// vào bản ghi) — thiếu vế này thì PUT /payment/:id sửa được phiếu thanh toán/cọc của khách.
	return s.updatePayment(id, paymentToParamsForUpdate(in), userID,
		[]int{define.PaymentForClinicIncome, define.PaymentForClinicExpense})
}

// paymentToParamsForUpdate convert typed Payment → map cho UpdatePaymentCustomer.
//
// Contract: Update DTO yêu cầu FE gửi full payload (binding:"required" trên các
// field bắt buộc). Vì vậy nil-pointer ở field nullable = "clear field", KHÔNG
// phải "skip". Map luôn chứa khóa cho mọi nullable field, value = nil để merge
// logic clear; non-nil thì deref. Required field (amount/method/branch_id)
// đã đảm bảo non-zero qua binding.
func paymentToParamsForUpdate(p *crmPaymentDao.Payment) map[string]interface{} {
	m := map[string]interface{}{
		"updated_at":      p.UpdatedAt,
		"amount":          p.Amount,
		"method":          p.Method,
		"branch_id":       p.BranchId,
		"bank_code":       p.BankCode,
		"note":            p.Note,
		"card_type_id":    derefIntOrNil(p.CardTypeId),
		"payment_time":    derefInt64OrNil(p.PaymentTime),
		"payment_of_date": derefInt64OrNil(p.PaymentOfDate),
		"customer_id":     derefIntOrNil(p.CustomerId),
		"serviceUsed_id":  derefIntOrNil(p.ServiceUsedId),
		"deposit_id":      derefIntOrNil(p.DepositId),
		"appointment_id":  derefIntOrNil(p.AppointmentId),
		"deposit_type":    derefIntOrNil(p.DepositType),
	}
	if p.PaymentFor != nil {
		m["payment_for"] = *p.PaymentFor
	}
	return m
}

func derefIntOrNil(p *int) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func derefInt64OrNil(p *int64) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

func derefDecimalOrNil(p *decimal.Decimal) interface{} {
	if p == nil {
		return nil
	}
	return *p
}

// UpdatePaymentCustomer: cập nhật thanh toán (giống PHP actionUpdatePaymentCustomer)
func (s *PaymentService) UpdatePaymentCustomer(id uint, params map[string]interface{}, userID int) (*crmPaymentDao.Payment, error) {
	return s.updatePayment(id, params, userID, nil)
}

// updatePayment — lõi sửa phiếu. onlyFor != nil ⇒ bản ghi phải thuộc một trong các payment_for đó.
//
// lens-money (2026-10-18): mọi guard tiền chạy TRONG tx dưới khoá customer → dòng dịch vụ → phiếu cọc gốc:
//   - M-7: phiếu cọc (payment_for=3) KHÔNG đổi SỐ TIỀN qua đường chung — phải qua UpdateDepositCustomer
//     (có guard "cọc đã dùng" + đặt lại deposit_after). Sửa ghi chú/hình thức… vẫn được.
//   - M-8: thanh toán / chuyển cọc tăng tiền không được vượt nợ còn lại của dịch vụ.
//   - chuyển cọc tăng tiền không vượt số dư cọc; hoàn dịch vụ tăng tiền không vượt số đã thu ròng.
func (s *PaymentService) updatePayment(id uint, params map[string]interface{}, userID int, onlyFor []int) (*crmPaymentDao.Payment, error) {
	var head crmPaymentDao.Payment
	if err := s.DB.First(&head, id).Error; err != nil {
		return nil, errs.ErrPaymentNotFound
	}
	if onlyFor != nil && (head.PaymentFor == nil || !slices.Contains(onlyFor, *head.PaymentFor)) {
		return nil, errs.ErrInvalidPaymentFor
	}
	// Snapshot old values cho log diff (dùng history package chung)
	paymentLogFields := []string{"branch_id", "payment_time", "payment_of_date", "amount", "note", "method", "card_type_id", "bank_code"}
	oldSnapshot := history.SnapshotRow(s.DB, "payment", id, paymentLogFields)

	var current crmPaymentDao.Payment
	err := s.txMoney(func(tx *gorm.DB) error {
		if err := lockCustomers(tx, derefInt(head.CustomerId)); err != nil {
			return err
		}
		current = crmPaymentDao.Payment{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, id).Error; err != nil {
			return errs.ErrPaymentNotFound
		}
		oldPaymentFor := current.PaymentFor
		oldAmount := current.Amount

		// Merge params
		s.mergePaymentParams(&current, params)

		if !current.Amount.IsPositive() {
			return errs.ErrAmountMustBePositive
		}
		if err := validatePaymentCommon(&current); err != nil {
			return err
		}

		// Không cho đổi từ chuyển cọc sang loại khác
		if oldPaymentFor != nil && *oldPaymentFor == define.PaymentForMoveDeposit {
			if current.PaymentFor == nil || *current.PaymentFor != define.PaymentForMoveDeposit {
				return errs.ErrCannotChangeTransferType
			}
		}

		if current.PaymentFor != nil && *current.PaymentFor == define.PaymentForDeposit && !current.Amount.Equal(oldAmount) {
			return errs.ErrInvalidPaymentFor
		}
		if err := s.guardPaymentUpdate(tx, &current, oldAmount); err != nil {
			return err
		}

		if err := s.beforeSave(tx, &current, false); err != nil {
			return err
		}
		current.UpdatedBy = new(uint(userID))
		if err := optlock.Save(tx, &current, current.UpdatedAt); err != nil {
			return err
		}

		// Log diff (dùng history.LogChanges chung)
		newSnapshot := history.SnapshotRow(tx, "payment", current.ID, paymentLogFields)
		if err := history.LogChanges(tx, "log_payment", "payment_id", int(current.ID), oldSnapshot, newSnapshot, paymentLogFields, nil, userID); err != nil {
			return err
		}

		return s.afterSaveAll(tx, &current)
	})
	return &current, err
}

// guardPaymentUpdate — guard tiền khi SỬA số tiền phiếu (gọi trong tx, sau lockCustomers).
// p.Amount = số mới (DƯƠNG, chưa qua beforeSave); oldAmount = số đang lưu (phiếu hoàn lưu ÂM).
func (s *PaymentService) guardPaymentUpdate(tx *gorm.DB, p *crmPaymentDao.Payment, oldAmount decimal.Decimal) error {
	if p.PaymentFor == nil {
		return nil
	}
	pf := *p.PaymentFor
	switch pf {
	case define.PaymentForOrder, define.PaymentForMoveDeposit:
		if p.ServiceUsedId != nil {
			su, err := lockServiceUsed(tx, *p.ServiceUsedId)
			if err != nil {
				return err
			}
			// Nợ còn lại nếu bỏ phiếu này ra = phải thu (totalEnd + VAT) − payment + oldAmount.
			room := su.Payable.Sub(su.Payment).Add(oldAmount)
			if p.Amount.GreaterThan(oldAmount) && p.Amount.GreaterThan(room) {
				return errs.NewLocalizedError("payment_amount_too_large", p.Amount.String(), room.String())
			}
			// R-3: sửa GIẢM không được làm số đã thu ròng âm (dịch vụ đã hoàn X thì phải còn ≥ X đã thu)
			// — cùng quy tắc với đường xoá (refundStillCovered), phần bớt đi = oldAmount − số mới.
			if p.Amount.LessThan(oldAmount) {
				if err := netPaidCovers(tx, *p.ServiceUsedId, oldAmount.Sub(p.Amount)); err != nil {
					return err
				}
			}
		}
		if pf == define.PaymentForMoveDeposit && p.DepositId != nil {
			depositAfter, err := lockDepositAfter(tx, *p.DepositId)
			if err != nil {
				return err
			}
			if depositAfter.Add(oldAmount).LessThan(p.Amount) {
				return errs.ErrDepositRemainingNegative
			}
		}
	case define.PaymentForRefundService:
		if p.ServiceUsedId != nil {
			if _, err := lockServiceUsed(tx, *p.ServiceUsedId); err != nil {
				return err
			}
			netPaid, err := serviceUsedNetPaid(tx, *p.ServiceUsedId)
			if err != nil {
				return err
			}
			// netPaid đã trừ chính phiếu hoàn này (oldAmount âm) ⇒ trần = netPaid + |oldAmount|.
			if p.Amount.Abs().GreaterThan(netPaid.Add(oldAmount.Abs())) {
				return errs.ErrRefundExceedsPaid
			}
		}
	}
	return nil
}

// UpdateDepositCustomer: cập nhật đặt cọc (giống PHP actionUpdateDepositCustomer)
// BUG FIX: thêm log diff giống UpdatePaymentCustomer
// lens-money M-1: guard "cọc đã dùng" chạy TRONG tx dưới khoá customer → phiếu cọc (chuyển/hoàn cọc
// song song không lọt qua giữa lúc kiểm và lúc ghi).
func (s *PaymentService) UpdateDepositCustomer(id uint, params map[string]interface{}, userID int) (*crmPaymentDao.Payment, error) {
	var head crmPaymentDao.Payment
	if err := s.DB.First(&head, id).Error; err != nil {
		return nil, errs.ErrDepositNotFound
	}
	if head.PaymentFor == nil || *head.PaymentFor != define.PaymentForDeposit {
		return nil, errs.ErrDepositNotFound
	}

	// Snapshot old values cho log diff
	paymentLogFields := []string{"branch_id", "payment_time", "payment_of_date", "amount", "note", "method", "card_type_id", "bank_code"}
	oldSnapshot := history.SnapshotRow(s.DB, "payment", id, paymentLogFields)

	var current crmPaymentDao.Payment
	err := s.txMoney(func(tx *gorm.DB) error {
		if err := lockCustomers(tx, derefInt(head.CustomerId)); err != nil {
			return err
		}
		current = crmPaymentDao.Payment{}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, id).Error; err != nil {
			return errs.ErrDepositNotFound
		}
		// Không cho cập nhật đặt cọc đã sử dụng
		if current.DepositAfter != nil && current.Amount.Cmp(*current.DepositAfter) != 0 {
			return errs.ErrDepositUsedCannotUpdate
		}

		s.mergePaymentParams(&current, params)

		if !current.Amount.IsPositive() {
			return errs.ErrAmountMustBePositive
		}
		if err := validatePaymentCommon(&current); err != nil {
			return err
		}

		current.DepositAfter = new(current.Amount)

		if err := s.beforeSave(tx, &current, false); err != nil {
			return err
		}
		current.UpdatedBy = new(uint(userID))
		if err := optlock.Save(tx, &current, current.UpdatedAt); err != nil {
			return err
		}

		// Log diff
		newSnapshot := history.SnapshotRow(tx, "payment", current.ID, paymentLogFields)
		if err := history.LogChanges(tx, "log_payment", "payment_id", int(current.ID), oldSnapshot, newSnapshot, paymentLogFields, nil, userID); err != nil {
			return err
		}

		return s.afterSaveAll(tx, &current)
	})
	return &current, err
}

// --- Delete ---
//
// lens-money M-6 (2026-10-18): mỗi endpoint xoá chỉ xoá đúng LOẠI phiếu của nó (quyền hẹp như
// extRefundDeposit trước đây xoá được cả phiếu thanh toán đơn hàng) và tra phiếu qua `scoped` — db đã
// gắn phạm vi tenant do handler dựng (tenant.ScopedDBNullBranch*), KHÔNG tra trần theo id. Sai loại ⇒
// lỗi not-found của endpoint đó (422), không xoá gì.

// DeletePayment: xóa thanh toán (giống PHP actionDeletePaymentCustomer). Phiếu cọc đã dùng không xoá
// được qua đường chung (M-7: xoá cọc làm các phiếu chuyển/hoàn cọc con mất liên kết, tiền "đã thu" của
// dịch vụ không còn đồng tiền vào nào tương ứng).
func (s *PaymentService) DeletePayment(scoped *gorm.DB, id uint, userID int) error {
	return s.deleteChecked(scoped, id, userID, errs.ErrPaymentNotFound, func(tx *gorm.DB, p *crmPaymentDao.Payment) error {
		if p.PaymentFor == nil {
			return nil
		}
		switch *p.PaymentFor {
		case define.PaymentForDeposit:
			return depositUnused(p)
		case define.PaymentForOrder, define.PaymentForMoveDeposit:
			return refundStillCovered(tx, p)
		}
		return nil
	})
}

// DeleteDeposit: xóa đặt cọc (giống PHP actionDeleteDepositCustomer)
func (s *PaymentService) DeleteDeposit(scoped *gorm.DB, id uint, userID int) error {
	return s.deleteChecked(scoped, id, userID, errs.ErrDepositNotFound, func(_ *gorm.DB, p *crmPaymentDao.Payment) error {
		if p.PaymentFor == nil || *p.PaymentFor != define.PaymentForDeposit {
			return errs.ErrDepositNotFound
		}
		return depositUnused(p)
	})
}

// DeleteRefundCustomer: xóa hoàn cọc
func (s *PaymentService) DeleteRefundCustomer(scoped *gorm.DB, id uint, userID int) error {
	return s.deleteChecked(scoped, id, userID, errs.ErrDepositReturnNotFound, paymentForIs(define.PaymentForRefundDeposit, errs.ErrDepositReturnNotFound))
}

// DeleteRefundOrderPlan: xóa hoàn dịch vụ (giống PHP actionDeleteRefundOrderPlan)
// PHP cộng dồn refundServiceUsed trực tiếp rồi afterDelete calculatorPaymentServiceUsed ghi đè lại → redundant.
// Go: chỉ dựa vào afterDeleteAll → calculatorPaymentServiceUsed để tính lại.
func (s *PaymentService) DeleteRefundOrderPlan(scoped *gorm.DB, id uint, userID int) error {
	return s.deleteChecked(scoped, id, userID, errs.ErrRefundNotFound, paymentForIs(define.PaymentForRefundService, errs.ErrRefundNotFound))
}

// paymentForIs — check cho deleteChecked: phiếu phải đúng loại want, sai ⇒ notFound.
func paymentForIs(want int, notFound error) func(*gorm.DB, *crmPaymentDao.Payment) error {
	return func(_ *gorm.DB, p *crmPaymentDao.Payment) error {
		if p.PaymentFor == nil || *p.PaymentFor != want {
			return notFound
		}
		return nil
	}
}

// depositUnused — cọc đã chuyển/hoàn một phần (deposit_after ≠ amount) thì không xoá.
func depositUnused(p *crmPaymentDao.Payment) error {
	if p.DepositAfter != nil && p.Amount.Cmp(*p.DepositAfter) != 0 {
		return errs.ErrDepositUsedCannotDelete
	}
	return nil
}

// refundStillCovered — xoá phiếu thanh toán/chuyển cọc không được làm số đã thu ròng của dịch vụ âm
// (dịch vụ đã hoàn X thì phải còn ≥ X đã thu). Gọi dưới khoá (deleteChecked).
func refundStillCovered(tx *gorm.DB, p *crmPaymentDao.Payment) error {
	if p.ServiceUsedId == nil || tx == nil {
		return nil
	}
	if _, err := lockServiceUsed(tx, *p.ServiceUsedId); err != nil {
		return err
	}
	return netPaidCovers(tx, *p.ServiceUsedId, p.Amount)
}

// netPaidCovers — bớt `removed` khỏi số đã thu ròng của dịch vụ mà không âm (đã hoàn X thì còn ≥ X đã
// thu). Dùng chung cho xoá phiếu (removed = số phiếu) và sửa giảm (removed = cũ − mới). Caller đã khoá
// customer + dòng dịch vụ.
func netPaidCovers(tx *gorm.DB, serviceUsedID int, removed decimal.Decimal) error {
	netPaid, err := serviceUsedNetPaid(tx, serviceUsedID)
	if err != nil {
		return err
	}
	if netPaid.Sub(removed).IsNegative() {
		return errs.ErrRefundExceedsPaid
	}
	return nil
}

// deleteChecked — tra phiếu qua scoped (tenant) → check loại (ngoài tx, báo sớm) → trong tx: khoá
// customer → khoá + đọc lại phiếu (FOR UPDATE) → check lại dưới khoá → xoá + tính lại.
func (s *PaymentService) deleteChecked(scoped *gorm.DB, id uint, userID int, notFound error,
	check func(tx *gorm.DB, p *crmPaymentDao.Payment) error) error {
	if scoped == nil || id == 0 {
		return notFound
	}
	var head crmPaymentDao.Payment
	if err := scoped.First(&head, id).Error; err != nil {
		return notFound
	}
	if err := check(nil, &head); err != nil {
		return err
	}
	return s.txMoney(func(tx *gorm.DB) error {
		if err := lockCustomers(tx, derefInt(head.CustomerId)); err != nil {
			return err
		}
		var p crmPaymentDao.Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error; err != nil {
			return notFound
		}
		if err := check(tx, &p); err != nil {
			return err
		}
		return s.deletePaymentTx(tx, &p, userID, nil)
	})
}

// --- Internal helpers ---

// createPayment: internal create with beforeSave + GORM Create + afterSave
func (s *PaymentService) createPayment(tx *gorm.DB, p *crmPaymentDao.Payment, userID int) error {
	if !p.Amount.IsPositive() {
		return errs.ErrAmountMustBePositive
	}
	if err := validatePaymentCommon(p); err != nil {
		return err
	}
	if err := s.beforeSave(tx, p, true); err != nil {
		return err
	}
	if p.Code == "" {
		// GenerateErr: lỗi bộ đếm (deadlock 1213, lock-wait) phải nổi lên để txMoney chạy lại / rollback.
		code, err := codegen.GenerateErr(tx, "payment", new(uint(p.BranchId)))
		if err != nil {
			return err
		}
		p.Code = code
	}
	if p.Code == "" {
		return fmt.Errorf("code generation failed for payment: check config_code table")
	}
	// userID<=0 = không resolve được ai tạo (vd sync không tìm thấy user hệ
	// thống) — để trống thay vì gán 0 (created_by có FK constraint thật tới
	// user, giá trị 0 không phải NULL sẽ vi phạm FK, lỗi 1452).
	if userID > 0 {
		uid := uint(userID)
		p.CreatedBy = &uid
		p.UpdatedBy = &uid
	}
	if err := tx.Create(p).Error; err != nil {
		return err
	}

	// afterSave insert: update customer last_visit_date
	if p.PaymentFor != nil && *p.PaymentFor == define.PaymentForOrder && p.PaymentTime != nil && p.CustomerId != nil {
		tx.Table("customer").Where("id = ?", *p.CustomerId).Updates(map[string]interface{}{
			"last_visit_date":   *p.PaymentTime,
			"last_contact_date": *p.PaymentTime,
		})
	}

	return s.afterSaveAll(tx, p)
}

// deletePayment: internal delete with beforeDelete + afterDelete
func (s *PaymentService) deletePayment(p *crmPaymentDao.Payment, userID int) error {
	return s.deletePaymentWith(p, userID, nil)
}

// deletePaymentWith — deferred != nil: phần TÍNH LẠI của afterDeleteAll được gom vào deferred để
// caller chạy 1 lần sau khi xoá xong cả loạt (xem deferredPaymentRecalc). Phần lõi — guard, dọn bảng
// con, log — luôn chạy ngay như cũ.
func (s *PaymentService) deletePaymentWith(p *crmPaymentDao.Payment, userID int, deferred *deferredPaymentRecalc) error {
	// crm-standalone story-09: bỏ guard hoá đơn điện tử đã phát hành + dọn bảng `invoice` — module
	// einvoice đã xoá (story-07, D2), không còn code nào ghi bảng `invoice`.
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockCustomers(tx, derefInt(p.CustomerId)); err != nil {
			return err
		}
		return s.deletePaymentTx(tx, p, userID, deferred)
	})
}

// deletePaymentTx — thân xoá phiếu chạy trong tx của caller (caller đã khoá customer).
func (s *PaymentService) deletePaymentTx(tx *gorm.DB, p *crmPaymentDao.Payment, userID int, deferred *deferredPaymentRecalc) error {
	// beforeDelete: log + unlink relations
	if err := s.logPaymentDelete(tx, p.ID, userID); err != nil {
		return err
	}
	// M-21: lỗi các lệnh dọn phải nổi lên để rollback (bản cũ bỏ qua .Error ⇒ commit nửa vời).
	for _, q := range []string{
		"DELETE FROM order_commission_telesale WHERE payment_id = ?",
		"DELETE FROM order_revenue_telesale WHERE payment_id = ?",
		"UPDATE payment SET deposit_id = NULL, " + optlock.RawSet + " WHERE deposit_id = ?",
	} {
		if err := tx.Exec(q, p.ID).Error; err != nil {
			return err
		}
	}
	// Phase 2 (audit toàn vẹn): KHÔNG xóa log_payment — giữ lịch sử + bản ghi delete (đã ghi ở logPaymentDelete phía trên).

	if err := tx.Delete(p).Error; err != nil {
		return err
	}

	if deferred != nil {
		deferred.collect(p)
		return nil
	}
	return s.afterDeleteAll(tx, p)
}

// bindPayment: chỉ map các field được phép từ client vào Payment mới.
// Blocked: code, created_by, updated_at, deposit_after, revenue_id, source_id, list_direct_sales.
func (s *PaymentService) bindPayment(params map[string]interface{}) *crmPaymentDao.Payment {
	p := &crmPaymentDao.Payment{}
	if v, ok := params["amount"]; ok {
		p.Amount = helpers.ToDecimal(v)
	}
	if v, ok := params["method"]; ok {
		p.Method = int(toInt64(v))
	}
	if v, ok := params["branch_id"]; ok {
		p.BranchId = int(toInt64(v))
	}
	if v, ok := params["payment_time"]; ok {
		p.PaymentTime = new(toInt64(v))
	}
	if v, ok := params["payment_of_date"]; ok {
		p.PaymentOfDate = new(toInt64(v))
	}
	if v, ok := params["card_type_id"]; ok {
		if v == nil {
			p.CardTypeId = nil
		} else if ct := int(toInt64(v)); ct > 0 {
			p.CardTypeId = &ct
		}
	}
	if v, ok := params["bank_code"]; ok {
		p.BankCode, _ = v.(string)
	}
	if v, ok := params["note"]; ok {
		if v == nil {
			p.Note = ""
		} else {
			p.Note, _ = v.(string)
		}
	}
	if v, ok := params["payment_for"]; ok && v != nil {
		p.PaymentFor = new(int(toInt64(v)))
	}
	if v, ok := params["customer_id"]; ok && v != nil {
		p.CustomerId = new(int(toInt64(v)))
	}
	if v, ok := params["serviceUsed_id"]; ok && v != nil {
		p.ServiceUsedId = new(int(toInt64(v)))
	}
	if v, ok := params["deposit_id"]; ok && v != nil {
		p.DepositId = new(int(toInt64(v)))
	}
	if v, ok := params["appointment_id"]; ok && v != nil {
		p.AppointmentId = new(int(toInt64(v)))
	}
	return p
}

// mergePaymentParams: selective merge params vào existing payment
func (s *PaymentService) mergePaymentParams(p *crmPaymentDao.Payment, params map[string]interface{}) {
	if v, ok := params["amount"]; ok {
		p.Amount = helpers.ToDecimal(v)
	}
	if v, ok := params["method"]; ok {
		p.Method = int(toInt64(v))
	}
	if v, ok := params["branch_id"]; ok {
		p.BranchId = int(toInt64(v))
	}
	if v, ok := params["payment_time"]; ok {
		p.PaymentTime = new(toInt64(v))
	}
	if v, ok := params["payment_of_date"]; ok {
		p.PaymentOfDate = new(toInt64(v))
	}
	if v, ok := params["card_type_id"]; ok {
		if v == nil {
			p.CardTypeId = nil
		} else {
			ct := int(toInt64(v))
			if ct > 0 {
				p.CardTypeId = &ct
			} else {
				p.CardTypeId = nil
			}
		}
	}
	if v, ok := params["bank_code"]; ok {
		p.BankCode, _ = v.(string)
	}
	if v, ok := params["note"]; ok {
		if v == nil {
			p.Note = ""
		} else {
			p.Note, _ = v.(string)
		}
	}
	if v, ok := params["updated_at"]; ok && toInt64(v) > 0 { // khoá lạc quan (D6): giá trị client đã đọc
		p.UpdatedAt = toInt64(v)
	}
	if v, ok := params["deposit_type"]; ok {
		if v == nil {
			p.DepositType = nil
		} else if dt := int(toInt64(v)); dt > 0 {
			p.DepositType = &dt
		} else {
			p.DepositType = nil
		}
	}
}

// toInt64: safe convert interface{} to int64 for integer fields (IDs, codes, timestamps, version).
func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case float32:
		return int64(val)
	case json.Number:
		i, _ := val.Int64()
		return i
	}
	return 0
}
