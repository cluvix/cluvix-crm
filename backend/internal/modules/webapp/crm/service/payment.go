// Package crm — Payment service layer.
//
// Convert từ PHP: be/common/models/Payment.php (1353 dòng)
// Phase 1a: helpers + beforeSave logic
// Phase 1b: afterSave + afterDelete logic
// Phase 1c: Create/Update/Delete methods
//
// Dependencies: Customer, OrderServiceUsed, OrderPlan, LogPayment,
//
//	OrderCommissionTelesale, OrderRevenueTelesale, OrderInstallment
package service

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	crmPaymentDao "erpCluvix/internal/modules/webapp/crm/payment/dao"
	paymentRepo "erpCluvix/internal/modules/webapp/crm/payment/repo"
	history "erpCluvix/internal/modules/webapp/history/log/service"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/logger"

	"go.uber.org/zap"
)

// netPaidPaymentFors — các loại phiếu cấu thành "đã thu RÒNG" của dịch vụ/khách: thanh toán + chuyển cọc
// + hoàn dịch vụ (lưu âm). Cùng tập với customer.totalPayment (updateCustomer).
var netPaidPaymentFors = []int{define.PaymentForRefundService, define.PaymentForOrder, define.PaymentForMoveDeposit}

type PaymentService struct {
	DB   *gorm.DB
	repo paymentRepo.Reader
}

func NewPaymentService(db *gorm.DB) *PaymentService {
	return &PaymentService{DB: db, repo: paymentRepo.NewGormReader()}
}

// NewPaymentServiceWithReader cho phép test inject fake Reader (no DB).
// Production code dùng NewPaymentService.
func NewPaymentServiceWithReader(db *gorm.DB, r paymentRepo.Reader) *PaymentService {
	return &PaymentService{DB: db, repo: r}
}

// --- beforeSave: giống PHP Payment::beforeValidate + beforeSave ---

// beforeSave xử lý logic trước khi lưu payment.
// insert=true cho create, insert=false cho update.
//
// PHP flow:
//
//	beforeValidate: clinic → null customer fields; sync payment_of_date
//	beforeSave(insert): parent_id from serviceUsed
//	beforeSave: switch payment_for → set amount sign, revenue_id, source_id, etc.
//	beforeSave(!insert): khoá lạc quan updated_at (D6) check
func (s *PaymentService) beforeSave(tx *gorm.DB, p *crmPaymentDao.Payment, insert bool) error {
	// --- beforeValidate ---
	if p.PaymentFor != nil && (*p.PaymentFor == define.PaymentForClinicIncome || *p.PaymentFor == define.PaymentForClinicExpense) {
		p.CustomerId = nil
		p.ParentId = nil
		p.ServiceUsedId = nil
	}

	// Sync payment_of_date cho FOR_ORDER và FOR_REFUND_SERVICE
	if p.PaymentFor != nil && (*p.PaymentFor == define.PaymentForOrder || *p.PaymentFor == define.PaymentForRefundService) {
		p.PaymentOfDate = p.PaymentTime
	}

	// --- beforeSave ---
	if insert {
		// parent_id from orderServiceUsed.order_plan_id
		if p.ServiceUsedId != nil {
			planID, err := s.repo.ServiceUsedPlanID(tx, *p.ServiceUsedId)
			if err != nil {
				return err
			}
			p.ParentId = planID
		}
	}

	if p.PaymentFor == nil {
		return nil
	}

	switch *p.PaymentFor {
	case define.PaymentForRefundDeposit, define.PaymentForRefundDepositKG:
		// Negate cả INSERT lẫn UPDATE (mirror ClinicExpense): quy ước DB là refund amount ÂM.
		// Bug cũ chỉ negate khi insert → update lật dấu thành dương → tồn quỹ/P&L lệch 2×.
		if p.Amount.IsPositive() {
			p.Amount = p.Amount.Neg()
		}

	case define.PaymentForRefundService:
		if p.Amount.IsPositive() {
			p.Amount = p.Amount.Neg()
		}

	case define.PaymentForOrder:
		// M-5 (lens-money 2026-10-18): KHÔNG còn chặn thu tiếp dịch vụ đã hoàn một phần. Rule đã chốt:
		// hoàn = huỷ bớt dịch vụ ⇒ công nợ (totalEnd − payment) KHÔNG đổi, phần nợ còn lại phải thu được.
		// Chặn thu VƯỢT nợ nằm ở các luồng tạo/sửa (khoá dòng dịch vụ, so totalEnd − payment).
		// Check: ngày thanh toán không thể trước ngày lên đơn
		if p.ServiceUsedId != nil && p.PaymentTime != nil {
			var createdDay *int64
			tx.Table("order_service_used").Select("created_day").Where("id = ?", *p.ServiceUsedId).Scan(&createdDay)
			if createdDay != nil && *p.PaymentTime < *createdDay {
				return errs.ErrPaymentBeforeOrderDate
			}
		}

	case define.PaymentForDeposit:
		p.PaymentOfDate = nil

	case define.PaymentForMoveDeposit:
		// M-5: như FOR_ORDER — dịch vụ đã hoàn một phần vẫn nhận chuyển cọc cho phần nợ còn lại.
		// Check: ngày chuyển cọc không thể trước ngày lên đơn
		if p.ServiceUsedId != nil && p.PaymentOfDate != nil {
			var createdDay *int64
			tx.Table("order_service_used").Select("created_day").Where("id = ?", *p.ServiceUsedId).Scan(&createdDay)
			if createdDay != nil && *p.PaymentOfDate < *createdDay {
				return errs.ErrPaymentBeforeOrderDate
			}
		}

	case define.PaymentForClinicExpense:
		if p.Amount.IsPositive() {
			p.Amount = p.Amount.Neg()
		}
	}

	// --- beforeSave(!insert): khoá lạc quan updated_at check (D6) ---
	// Bước ghi (optlock.Save) kiểm lại `WHERE updated_at = ?` nên đây chỉ là tiền kiểm báo sớm.
	if !insert {
		current, err := s.repo.PaymentSnapshot(tx, p.ID)
		if err != nil {
			return errs.ErrPaymentNotFound
		}
		if p.UpdatedAt != current.UpdatedAt {
			return errs.ErrVersionConflict
		}
	}

	return nil
}

// --- afterSave helpers ---

// logPaymentDelete: ghi log xóa (action_type=3) KÈM snapshot giá trị cũ + id/code/customer_id +
// company_id (history.PaymentDeleteFields). Gọi TRƯỚC khi xóa payment để chụp giá trị. KHÔNG
// DELETE log_payment ở deletePayment → log cũ được giữ (FK payment_id ON DELETE SET NULL).
func (s *PaymentService) logPaymentDelete(tx *gorm.DB, paymentID uint, userID int) error {
	if err := history.LogDeleteSnapshot(tx, "log_payment", int(paymentID), history.PaymentDeleteFields, userID); err != nil {
		return fmt.Errorf("create payment delete log: %w", err)
	}
	return nil
}

// --- afterSave: calculators (giống PHP deleteCache → calculatorPaymentServiceUsed, calculatorOrderPlan, updateCustomer) ---

// calculatorPaymentServiceUsed: cập nhật payment + refundServiceUsed + payment_status cho order_service_used.
// (crm-standalone: bỏ tích/trừ điểm story-10, bỏ tạo chăm sóc sau mua sản phẩm story-11 — chỉ còn tính tiền.)
func (s *PaymentService) calculatorPaymentServiceUsed(tx *gorm.DB, serviceUsedId int) {
	var paymentTotal decimal.Decimal
	tx.Table("payment").
		Where("serviceUsed_id = ? AND payment_for IN ?", serviceUsedId, []int{define.PaymentForOrder, define.PaymentForMoveDeposit}).
		Select("COALESCE(SUM(amount), 0)").Scan(&paymentTotal)

	var refundTotal decimal.Decimal
	tx.Table("payment").
		Where("serviceUsed_id = ? AND payment_for = ?", serviceUsedId, define.PaymentForRefundService).
		Select("COALESCE(SUM(amount), 0)").Scan(&refundTotal)

	absRefund := refundTotal.Abs()

	// payment_status: Start=1 (chưa TT), Partly=3 (1 phần), Done=4 (đủ 100%)
	// So với số PHẢI THU = totalEnd + VAT (bmad/order-vat D4): phiếu thu là tiền có thuế.
	paymentStatus := define.ServicePaymentStatusStart
	var payable decimal.Decimal
	tx.Table("order_service_used").Select(osuPayableSQL).Where("id = ?", serviceUsedId).Scan(&payable)
	if paymentTotal.IsPositive() && paymentTotal.LessThan(payable) {
		paymentStatus = define.ServicePaymentStatusPartly
	} else if paymentTotal.GreaterThanOrEqual(payable) && payable.IsPositive() {
		paymentStatus = define.ServicePaymentStatusDone
	}

	tx.Table("order_service_used").Where("id = ?", serviceUsedId).Updates(map[string]interface{}{
		"payment":           paymentTotal,
		"refundServiceUsed": absRefund,
		"payment_status":    paymentStatus,
	})
}

// calculatorOrderPlan: cập nhật paymentPlan + debtsPlan cho order_plan.
// D20a (story-56): hoàn tiền giảm CẢ phải thu lẫn đã thu ⇒ debtsPlan = (Σ phải thu − ΣrefundServiceUsed) − paymentPlan,
// phải thu = totalEnd + VAT (bmad/order-vat D4); cùng công thức với customer.totalDebt (customerReceivable).
// Không có hoàn ⇒ refund = 0, số giữ nguyên.
func (s *PaymentService) calculatorOrderPlan(tx *gorm.DB, parentId int, paymentFor int) error {
	if parentId == 0 {
		return nil
	}
	validFors := []int{define.PaymentForOrder, define.PaymentForMoveDeposit, define.PaymentForRefundService}
	found := false
	for _, v := range validFors {
		if paymentFor == v {
			found = true
			break
		}
	}
	if !found {
		return nil
	}

	var totalPayment decimal.Decimal
	if err := tx.Table("payment").
		Where("parent_id = ? AND payment_for IN ?", parentId, validFors).
		Select("COALESCE(SUM(amount), 0)").Scan(&totalPayment).Error; err != nil {
		return fmt.Errorf("calculatorOrderPlan sum payment: %w", err)
	}

	var orderReceivable decimal.Decimal
	if err := tx.Table("order_service_used").
		Select(osuReceivableSumSQL).
		Where("order_plan_id = ?", parentId).Scan(&orderReceivable).Error; err != nil {
		return fmt.Errorf("calculatorOrderPlan sum receivable: %w", err)
	}

	if err := tx.Table("order_plan").Where("id = ?", parentId).Updates(map[string]interface{}{
		"paymentPlan": totalPayment,
		"debtsPlan":   orderReceivable.Sub(totalPayment),
	}).Error; err != nil {
		return fmt.Errorf("calculatorOrderPlan update: %w", err)
	}
	return nil
}

// calculatorDeposit: tính lại deposit_after cho đặt cọc gốc (khi chuyển cọc hoặc hoàn cọc)
func (s *PaymentService) calculatorDeposit(tx *gorm.DB, depositId int) error {
	var deposit struct {
		Amount decimal.Decimal `gorm:"column:amount"`
	}
	if err := tx.Table("payment").Select("amount").Where("id = ?", depositId).Take(&deposit).Error; err != nil {
		return errs.ErrDepositNotFound
	}

	var moveAmount decimal.Decimal
	tx.Table("payment").
		Where("deposit_id = ? AND payment_for = ?", depositId, define.PaymentForMoveDeposit).
		Select("COALESCE(SUM(amount), 0)").Scan(&moveAmount)

	var refundAmount decimal.Decimal
	tx.Table("payment").
		Where("deposit_id = ? AND payment_for = ?", depositId, define.PaymentForRefundDeposit).
		Select("COALESCE(SUM(amount), 0)").Scan(&refundAmount)

	// deposit còn lại = amount gốc - chuyển cọc + hoàn cọc (refund là số âm sau beforeSave).
	depositAfter := deposit.Amount.Sub(moveAmount).Add(refundAmount)
	if depositAfter.IsNegative() {
		return errs.ErrDepositRemainingNegative
	}

	return tx.Table("payment").Where("id = ?", depositId).Update("deposit_after", depositAfter).Error
}

// calculatorDepositConsignment: tính lại deposit_after cho ký gửi gốc
func (s *PaymentService) calculatorDepositConsignment(tx *gorm.DB, depositId int) error {
	var deposit struct {
		Amount decimal.Decimal `gorm:"column:amount"`
	}
	if err := tx.Table("payment").Select("amount").Where("id = ?", depositId).Take(&deposit).Error; err != nil {
		return errs.ErrEscrowNotFound
	}

	var refundAmount decimal.Decimal
	tx.Table("payment").
		Where("deposit_id = ? AND payment_for = ?", depositId, define.PaymentForRefundDepositKG).
		Select("COALESCE(SUM(amount), 0)").Scan(&refundAmount)

	depositAfter := deposit.Amount.Add(refundAmount)
	if depositAfter.IsNegative() {
		return errs.ErrRefundExceedsEscrow
	}
	return tx.Table("payment").Where("id = ?", depositId).Update("deposit_after", depositAfter).Error
}

// updateInstallmentPayment: phân bổ thanh toán vào các kỳ trả góp (giống PHP)
func (s *PaymentService) updateInstallmentPayment(tx *gorm.DB, serviceUsedId int) {
	var hasConfig int64
	tx.Table("order_installment").Where("order_service_used_id = ?", serviceUsedId).Count(&hasConfig)
	if hasConfig == 0 {
		return
	}

	var totalPaid decimal.Decimal
	tx.Table("payment").
		Where("serviceUsed_id = ? AND payment_for IN ?", serviceUsedId, []int{define.PaymentForOrder, define.PaymentForMoveDeposit}).
		Select("COALESCE(SUM(amount), 0)").Scan(&totalPaid)

	type installment struct {
		ID     uint            `gorm:"column:id"`
		Amount decimal.Decimal `gorm:"column:amount"`
	}
	var installments []installment
	tx.Table("order_installment").
		Select("id, amount").
		Where("order_service_used_id = ?", serviceUsedId).
		Order("period_number ASC").Find(&installments)

	type calcRow struct {
		ID     uint
		Paid   decimal.Decimal
		Status int
	}
	calcs := make([]calcRow, 0, len(installments))
	remaining := totalPaid
	for _, inst := range installments {
		var c calcRow
		c.ID = inst.ID
		if !remaining.IsPositive() {
			c.Paid = decimal.Zero
			c.Status = define.InstallmentStatusUnpaid
		} else {
			paid := remaining
			if paid.GreaterThan(inst.Amount) {
				paid = inst.Amount
			}
			c.Paid = paid
			if paid.GreaterThanOrEqual(inst.Amount) {
				c.Status = define.InstallmentStatusPaid
			} else {
				c.Status = define.InstallmentStatusPartial
			}
			remaining = remaining.Sub(paid)
		}
		calcs = append(calcs, c)
	}

	if len(calcs) == 0 {
		return
	}

	// 1 UPDATE for paid_amount via CASE WHEN (values từ internal computation, không phải user input)
	ids := make([]uint, 0, len(calcs))
	casePaid := strings.Builder{}
	casePaid.WriteString("CASE id ")
	for _, c := range calcs {
		fmt.Fprintf(&casePaid, "WHEN %d THEN %s ", c.ID, c.Paid.String())
		ids = append(ids, c.ID)
	}
	casePaid.WriteString("END")
	tx.Exec("UPDATE order_installment SET paid_amount = "+casePaid.String()+" WHERE id IN ?", ids)

	// 1 UPDATE per status group (tối đa 3 nhóm: Unpaid/Paid/Partial)
	statusGroup := make(map[int][]uint)
	for _, c := range calcs {
		statusGroup[c.Status] = append(statusGroup[c.Status], c.ID)
	}
	for st, gIDs := range statusGroup {
		tx.Table("order_installment").Where("id IN ?", gIDs).Update("status", st)
	}

	// Reset paid_date cho installments chưa thanh toán
	var unpaidIDs []uint
	for _, c := range calcs {
		if c.Status == define.InstallmentStatusUnpaid {
			unpaidIDs = append(unpaidIDs, c.ID)
		}
	}
	if len(unpaidIDs) > 0 {
		tx.Table("order_installment").Where("id IN ?", unpaidIDs).Update("paid_date", nil)
	}
}

// updateCustomer: cập nhật financials cho customer (giống PHP updateCustomer)
// Flow:
//  1. Switch payment_for → recalc financials
//  2. Set customerType = OLD nếu có thanh toán đơn hàng
//
// KHÔNG đổi tiến trình khách (status/stage_id) — chỉ đổi tay (customer-stage-pipeline story-06 gỡ
// bước PHP cũ "accepted + có dịch vụ thanh toán đủ ⇒ Khách hàng" và "đặt cọc giữ chỗ ⇒ Đặt cọc").
func (s *PaymentService) updateCustomer(tx *gorm.DB, p *crmPaymentDao.Payment) error {
	if p.CustomerId == nil {
		return nil
	}
	custID := *p.CustomerId
	if p.PaymentFor == nil {
		return nil
	}

	custUpdates := map[string]interface{}{}
	flagOldOrNew := false

	switch *p.PaymentFor {
	case define.PaymentForRefundDeposit:
		var refund decimal.Decimal
		tx.Table("payment").Where("customer_id = ? AND payment_for = ?", custID, define.PaymentForRefundDeposit).
			Select("COALESCE(SUM(amount), 0)").Scan(&refund)
		custUpdates["refundDeposit"] = refund

		// Hoàn cọc làm deposit_after của bản gốc giảm → tính lại totalDeposit
		var deposit decimal.Decimal
		tx.Table("payment").
			Where("customer_id = ? AND payment_for = ? AND (deposit_type IS NULL OR deposit_type NOT IN ?)", custID, define.PaymentForDeposit, []int{define.PaymentTypeDepositKG}).
			Select("COALESCE(SUM(deposit_after), 0)").Scan(&deposit)
		custUpdates["totalDeposit"] = deposit

	case define.PaymentForRefundDepositKG:
		var refundConsignment decimal.Decimal
		tx.Table("payment").Where("customer_id = ? AND payment_for = ?", custID, define.PaymentForRefundDepositKG).
			Select("COALESCE(SUM(amount), 0)").Scan(&refundConsignment)
		var consignment decimal.Decimal
		tx.Table("payment").Where("customer_id = ? AND deposit_type = ?", custID, define.PaymentTypeDepositKG).
			Select("COALESCE(SUM(deposit_after), 0)").Scan(&consignment)
		custUpdates["consignment"] = consignment
		custUpdates["refundConsignment"] = refundConsignment

	case define.PaymentForDeposit:
		if p.DepositType != nil && (*p.DepositType == define.PaymentTypeDepositReserve || *p.DepositType == define.PaymentTypeDepositTemp) {
			var deposit decimal.Decimal
			tx.Table("payment").
				Where("customer_id = ? AND payment_for = ? AND (deposit_type IS NULL OR deposit_type NOT IN ?)", custID, define.PaymentForDeposit, []int{define.PaymentTypeDepositKG}).
				Select("COALESCE(SUM(deposit_after), 0)").Scan(&deposit)
			custUpdates["totalDeposit"] = deposit
		} else {
			var consignment decimal.Decimal
			tx.Table("payment").
				Where("customer_id = ? AND payment_for = ? AND deposit_type = ?", custID, define.PaymentForDeposit, define.PaymentTypeDepositKG).
				Select("COALESCE(SUM(deposit_after), 0)").Scan(&consignment)
			custUpdates["consignment"] = consignment
		}

	case define.PaymentForOrder, define.PaymentForMoveDeposit:
		var payment decimal.Decimal
		tx.Table("payment").
			Where("customer_id = ? AND payment_for IN ?", custID, []int{define.PaymentForRefundService, define.PaymentForOrder, define.PaymentForMoveDeposit}).
			Select("COALESCE(SUM(amount), 0)").Scan(&payment)
		// Công nợ = (Σ phải thu có VAT − Σ hoàn) − đã thu ròng (bmad/order-vat D6) — không suy từ totalRevenue
		// (doanh số trước thuế).
		_, receivable, err := customerRevenueReceivable(tx, custID)
		if err != nil {
			return fmt.Errorf("updateCustomer receivable: %w", err)
		}
		custUpdates["totalPayment"] = payment
		custUpdates["totalDebt"] = receivable.Sub(payment)
		flagOldOrNew = true

		// Khi chuyển cọc → thanh toán, deposit_after của bản cọc gốc đã giảm
		// → cần tính lại totalDeposit cho customer
		if *p.PaymentFor == define.PaymentForMoveDeposit {
			var deposit decimal.Decimal
			tx.Table("payment").
				Where("customer_id = ? AND payment_for = ? AND (deposit_type IS NULL OR deposit_type NOT IN ?)", custID, define.PaymentForDeposit, []int{define.PaymentTypeDepositKG}).
				Select("COALESCE(SUM(deposit_after), 0)").Scan(&deposit)
			custUpdates["totalDeposit"] = deposit
		}

	case define.PaymentForRefundService:
		var refundService decimal.Decimal
		tx.Table("payment").Where("customer_id = ? AND payment_for = ?", custID, define.PaymentForRefundService).
			Select("COALESCE(SUM(amount), 0)").Scan(&refundService)
		var totalPayment decimal.Decimal
		tx.Table("payment").
			Where("customer_id = ? AND payment_for IN ?", custID, []int{define.PaymentForOrder, define.PaymentForMoveDeposit, define.PaymentForRefundService}).
			Select("COALESCE(SUM(amount), 0)").Scan(&totalPayment)
		custUpdates["refundService"] = refundService
		custUpdates["totalPayment"] = totalPayment
		// D20a (story-56): hoàn tiền giảm CẢ doanh thu lẫn đã thu ⇒ tính lại ngay totalRevenue + totalDebt theo
		// cùng định nghĩa với updateCustomerRevenue (customerRevenueReceivable; refundServiceUsed đã được
		// calculatorPaymentServiceUsed cập nhật trước đó trong afterSaveAll/afterDeleteAll).
		// (Hành vi cũ kiểu Yii2 không tính lại totalDebt ở nhánh này — đã bỏ theo D20.)
		totalRevenue, receivable, err := customerRevenueReceivable(tx, custID)
		if err != nil {
			return fmt.Errorf("updateCustomer refund revenue: %w", err)
		}
		custUpdates["totalRevenue"] = totalRevenue
		custUpdates["totalDebt"] = receivable.Sub(totalPayment)
	}

	if flagOldOrNew {
		if p.ParentId != nil {
			// Chỉ upgrade OLD khi đơn có ít nhất 1 item không phải phí khám.
			// LEFT JOIN để không bị drop khi service bị xóa khỏi catalog (treat NULL as non-fee).
			var nonFeeCount int64
			tx.Raw(`SELECT COUNT(*) FROM order_service_used osu
				LEFT JOIN service s ON s.id = osu.service_id
				WHERE osu.order_plan_id = ?
				AND (s.id IS NULL OR COALESCE(s.serviceCategory_id, 1) != ?)`,
				*p.ParentId, serviceCategoryFee).Scan(&nonFeeCount)
			if nonFeeCount > 0 {
				custUpdates["customerType"] = define.CustomerTypeOld
			}
		} else {
			// Không xác định được đơn hàng (MoveDeposit không gắn service) → upgrade unconditionally
			custUpdates["customerType"] = define.CustomerTypeOld
		}
	}

	if len(custUpdates) > 0 {
		if err := tx.Table("customer").Where("id = ?", custID).Updates(custUpdates).Error; err != nil {
			return fmt.Errorf("updateCustomer update: %w", err)
		}
	}
	return nil
}

// afterSaveAll: gọi tất cả calculator sau khi save (giống PHP deleteCache + afterSave)
func (s *PaymentService) afterSaveAll(tx *gorm.DB, p *crmPaymentDao.Payment) error {
	// Tính lại số dư cọc gốc. M-1 (lens-money 2026-10-18): lỗi (số dư âm / mất phiếu cọc) PHẢI trả về để
	// rollback — bản cũ chỉ log warn rồi commit, nên 3 lần hoàn cọc song song chi ra gấp 3 số dư.
	if p.PaymentFor != nil && p.DepositId != nil {
		pf := *p.PaymentFor
		if pf == define.PaymentForMoveDeposit || pf == define.PaymentForRefundDeposit {
			if err := s.calculatorDeposit(tx, *p.DepositId); err != nil {
				return err
			}
		}
		if pf == define.PaymentForRefundDepositKG {
			if err := s.calculatorDepositConsignment(tx, *p.DepositId); err != nil {
				return err
			}
		}
	}

	// Ghi nhận hoa hồng + doanh số Telesale (FOR_ORDER, FOR_MOVE_DEPOSIT) và THU HỒI theo phiếu hoàn dịch vụ.
	// lens-money M-13 (user chốt 2026-10-18 "trừ cả doanh số và hoa hồng"): phiếu hoàn (amount ÂM) đi cùng
	// công thức ⇒ ghi dòng âm theo tỷ lệ phần hoàn, chia đều người cuối nhận phần dư; xoá phiếu hoàn ⇒
	// deletePaymentTx dọn theo payment_id. Báo cáo staff SUM 2 bảng ⇒ tự ra số ròng.
	if p.PaymentFor != nil && (*p.PaymentFor == define.PaymentForOrder || *p.PaymentFor == define.PaymentForMoveDeposit ||
		*p.PaymentFor == define.PaymentForRefundService) {
		if p.ServiceUsedId != nil && p.ParentId != nil && p.CustomerId != nil {
			// Nạp 1 query duy nhất (osu+service+order_plan) dùng chung cho 2 hàm dưới.
			if ctx, ok := loadOSUCommissionContext(tx, *p.ServiceUsedId); ok {
				s.handleTelesaleCommission(tx, ctx, p.ID, *p.ServiceUsedId, *p.ParentId, *p.CustomerId,
					p.BranchId, p.Amount, p.PaymentOfDate)
				// Ghi nhận doanh số (order_revenue_telesale) — luôn ghi bất kể có cấu hình hoa hồng hay
				// không, khác hàm commission ở trên (chỉ ghi khi rate > 0).
				handleRevenueTelesale(tx, ctx, p.ID, *p.ServiceUsedId, *p.ParentId, *p.CustomerId,
					p.BranchId, p.Amount, p.PaymentOfDate)
			}
		}
	}

	// Cập nhật service used
	if p.ServiceUsedId != nil {
		s.calculatorPaymentServiceUsed(tx, *p.ServiceUsedId)
		// Trả góp chỉ cập nhật khi thanh toán đơn hàng hoặc chuyển cọc (giống PHP)
		if p.PaymentFor != nil && (*p.PaymentFor == define.PaymentForOrder || *p.PaymentFor == define.PaymentForMoveDeposit) {
			s.updateInstallmentPayment(tx, *p.ServiceUsedId)
		}
	}

	// Cập nhật order treatment plan
	if p.ParentId != nil && p.PaymentFor != nil {
		if err := s.calculatorOrderPlan(tx, *p.ParentId, *p.PaymentFor); err != nil {
			return err
		}
	}

	// Cập nhật customer financials
	if p.PaymentFor != nil && *p.PaymentFor != define.PaymentForClinicIncome && *p.PaymentFor != define.PaymentForClinicExpense {
		if err := s.updateCustomer(tx, p); err != nil {
			return err
		}
	}
	return nil
}

// afterDeleteAll: gọi tất cả calculator sau khi delete (giống PHP afterDelete)
func (s *PaymentService) afterDeleteAll(tx *gorm.DB, p *crmPaymentDao.Payment) error {
	if p.PaymentFor != nil && p.DepositId != nil {
		pf := *p.PaymentFor
		if pf == define.PaymentForMoveDeposit || pf == define.PaymentForRefundDeposit {
			if err := s.calculatorDeposit(tx, *p.DepositId); err != nil {
				logger.L.Warn("afterDelete calculatorDeposit failed",
					zap.Uint("payment_id", p.ID), zap.Int("deposit_id", *p.DepositId), zap.Error(err))
			}
		}
		if pf == define.PaymentForRefundDepositKG {
			if err := s.calculatorDepositConsignment(tx, *p.DepositId); err != nil {
				logger.L.Warn("afterDelete calculatorDepositConsignment failed",
					zap.Uint("payment_id", p.ID), zap.Int("deposit_id", *p.DepositId), zap.Error(err))
			}
		}
	}

	if p.ServiceUsedId != nil {
		s.calculatorPaymentServiceUsed(tx, *p.ServiceUsedId)
		// Trả góp chỉ cập nhật khi thanh toán đơn hàng hoặc chuyển cọc (giống PHP)
		if p.PaymentFor != nil && (*p.PaymentFor == define.PaymentForOrder || *p.PaymentFor == define.PaymentForMoveDeposit) {
			s.updateInstallmentPayment(tx, *p.ServiceUsedId)
		}
	}

	if p.ParentId != nil && p.PaymentFor != nil {
		if err := s.calculatorOrderPlan(tx, *p.ParentId, *p.PaymentFor); err != nil {
			return err
		}
	}

	if p.PaymentFor != nil && *p.PaymentFor != define.PaymentForClinicIncome && *p.PaymentFor != define.PaymentForClinicExpense {
		if err := s.updateCustomer(tx, p); err != nil {
			return err
		}
	}
	return nil
}
