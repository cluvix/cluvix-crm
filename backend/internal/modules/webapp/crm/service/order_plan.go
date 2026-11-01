// Package crm — OrderPlan service layer.
//
// Convert từ PHP Yii2 model: be/common/models/OrderPlan.php (875 dòng)
// Gồm toàn bộ business logic: beforeSave, afterSave, beforeDelete hooks.
//
// Flow chính:
//   - Create: beforeSave(insert) → GORM Create → afterSave (update customer, services+installments)
//   - Update: version check → merge fields → GORM Save → afterSave (update customer, upsert services, delete orphans)
//   - Delete: check constraints (treatment, payment, warranty) → delete children → delete order
//
// Dependencies: Customer, OrderServiceUsed, OrderInstallment, Payment
//
// story-10 (crm-standalone): gỡ mã giảm giá / CTV / điểm tích luỹ — đơn không còn nhận mã giảm
// giá, không gán CTV, không ghi sổ hoa hồng CTV, không tích/trừ điểm. Các bước tính tiền còn lại
// (rank_discount_amount, debtsPlan, totalRevenue/totalDebt) giữ nguyên thứ tự.
package service

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	orderPlanDao "erpCluvix/internal/modules/webapp/crm/order_plan/dao"
	otpRepo "erpCluvix/internal/modules/webapp/crm/order_plan/repo"
	crmPaymentDao "erpCluvix/internal/modules/webapp/crm/payment/dao"
	crmServiceDao "erpCluvix/internal/modules/webapp/crm/service/dao"
	history "erpCluvix/internal/modules/webapp/history/log/service"
	"erpCluvix/pkg/brand"
	"erpCluvix/pkg/codegen"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/optlock"
)

var orderAllowedCustomerStatuses = map[int]bool{
	define.CustomerStatusTrash:             true,
	define.CustomerStatusTicket:            true,
	define.CustomerStatusOldSystem:         true,
	define.CustomerStatusLead:              true,
	define.CustomerStatusZalo:              true,
	define.CustomerStatusDeposit:           true,
	define.CustomerStatusAppointment:       true,
	define.CustomerStatusAppointmentCancel: true,
	define.CustomerStatusContact:           true,
}

type OrderPlanService struct {
	DB   *gorm.DB
	repo otpRepo.Reader
}

// validateAssignedUsersExist kiểm tra mọi ID trong listRaw (JSON array user_id — listRevenues) là
// user THẬT. Reject nếu có ID không hợp lệ — trước đây không validate gì, client gửi ID bất kỳ
// (không tồn tại) vẫn được lưu, tới lúc thanh toán hoa hồng mới "biến mất" vào ID sai, không ai
// nhận được và không có lỗi nào báo cho user biết đã gán sai người. /check-component 2026-07-30.
//
// Story-37 (E6/D15 — 1 thương hiệu/DB): bỏ vế "user thuộc company đang thao tác" (JSON
// user.companyId) — mọi user trong DB đều thuộc thương hiệu duy nhất. Giữ nguyên key lỗi.
func validateAssignedUsersExist(tx *gorm.DB, listRaw datatypes.JSON) error {
	if len(listRaw) == 0 {
		return nil
	}
	var ids []int
	if err := json.Unmarshal(listRaw, &ids); err != nil {
		// lens-money M-17: bản cũ `return nil` ⇒ `["12","13"]` lọt validate, tới lúc thanh toán
		// loadOSUCommissionContext cũng unmarshal lỗi ⇒ doanh số tư vấn viên ghi revenue_id = 0 (không thuộc ai).
		return errs.NewLocalizedError("list_revenues_invalid")
	}
	if len(ids) == 0 {
		return nil
	}

	var found []int
	tx.Table("user").Where("id IN ?", ids).Pluck("id", &found)

	valid := make(map[int]bool, len(found))
	for _, id := range found {
		valid[id] = true
	}

	for _, id := range ids {
		if !valid[id] {
			return errs.NewLocalizedError("assigned_user_not_in_company", id)
		}
	}
	return nil
}

func NewOrderPlanService(db *gorm.DB) *OrderPlanService {
	return &OrderPlanService{DB: db, repo: otpRepo.NewGormReader()}
}

// NewOrderPlanServiceWithReader cho phép test inject fake Reader (no DB).
func NewOrderPlanServiceWithReader(db *gorm.DB, r otpRepo.Reader) *OrderPlanService {
	return &OrderPlanService{DB: db, repo: r}
}

// --- Request DTO ---

type OrderPlanRequest struct {
	orderPlanDao.OrderPlan
	ServicesData []map[string]interface{} `json:"services_used"`
}

// --- Create ---

// Create tạo đơn hàng mới.
// Flow: load customer → beforeSave (version=1, orderPlanType, debtsPlan) → GORM Create
// → afterSave (update customer status/branch/last_visit, handle services+installments)
func (s *OrderPlanService) Create(req *OrderPlanRequest, userID int) error {
	order := &req.OrderPlan

	if err := validateOrderPlan(order); err != nil {
		return err
	}

	// Lấy customer info
	cust, err := s.repo.CustomerInfo(s.DB, order.CustomerId)
	if err != nil {
		return errs.ErrCustomerNotFound
	}

	err = s.DB.Transaction(func(tx *gorm.DB) error {
		// Khoá khách TRƯỚC mọi đọc/ghi: cùng thứ tự với luồng phiếu thu (customer → dịch vụ), và tổng tiền
		// của khách tính lại cuối tx đọc bản mới nhất (lens-money M-2).
		if err := lockCustomers(tx, order.CustomerId); err != nil {
			return err
		}
		// beforeSave (insert)
		order.OrderPlanType = cust.CustomerType
		// lens-money M-10: paymentPlan/debtsPlan là số server tính (recalcDebtsPlan cuối tx), KHÔNG nhận từ body.
		order.PaymentPlan = nil
		order.DebtsPlan = nil

		// Parse listRevenues nếu là string
		s.parseListRevenues(order)

		// Validate mỗi ID trong listRevenues là user thật — reject cứng nếu không.
		if err := validateAssignedUsersExist(tx, order.ListRevenues); err != nil {
			return err
		}

		uid := uint(userID)

		if err := tx.Set("userID", uid).Create(order).Error; err != nil {
			return err
		}

		// afterSave: update customer status
		custUpdates := map[string]interface{}{}
		if orderAllowedCustomerStatuses[cust.Status] {
			custUpdates["status"] = define.CustomerStatusVisited
			custUpdates["connect_id"] = 1 // CONNECTED
		}
		if cust.BranchID == nil && order.BranchId != nil {
			custUpdates["branch_id"] = *order.BranchId
		}
		if order.CreatedTime != nil {
			custUpdates["last_visit_date"] = *order.CreatedTime
			custUpdates["last_contact_date"] = *order.CreatedTime
		}
		// Gen mã nếu khách chưa có mã — theo chi nhánh CỦA KHÁCH (chi nhánh đơn chỉ dùng khi khách
		// chưa có, tức vừa được gán ở trên). Không chi nhánh nào ⇒ chưa có mã.
		custBranchID := cust.BranchID
		if custBranchID == nil {
			custBranchID = order.BranchId
		}
		if custBranchID != nil {
			if code := codegen.EnsureCustomerCode(tx, int64(order.CustomerId), int64(*custBranchID)); code != "" {
				custUpdates["code"] = code
			}
		}
		if len(custUpdates) > 0 {
			tx.Table("customer").Where("id = ?", order.CustomerId).Updates(custUpdates)
		}

		// Xử lý services
		if len(req.ServicesData) > 0 {
			if err := s.handleServices(tx, order, req.ServicesData, false, uid); err != nil {
				return err
			}
		}

		// Recalc debtsPlan từ SUM(osu.totalEnd) sau khi services đã được lưu
		if err := s.recalcDebtsPlan(tx, order.ID); err != nil {
			return err
		}

		// Cập nhật totalRevenue + totalDebt sau khi services đã được lưu
		s.updateCustomerRevenue(tx, order.CustomerId)

		return nil
	})
	return err
}

// --- Update ---

// Update cập nhật đơn hàng với optimistic locking.
// Flow: load current → updated_at check (khoá lạc quan D6) → merge fields → optlock.Save
// → afterSave (update customer, upsert services + delete orphans)
// Nếu FE không gửi customer_id → fallback về current.CustomerId.
// Update cập nhật đơn hàng. scopedDB được tạo bởi handler (ScopedDBNullBranch) và
// dùng duy nhất cho ownership check.
func (s *OrderPlanService) Update(req *OrderPlanRequest, userID int, scopedDB *gorm.DB) error {
	order := &req.OrderPlan

	var current orderPlanDao.OrderPlan
	if err := scopedDB.First(&current, order.ID).Error; err != nil {
		return errs.ErrNotFound
	}

	if order.UpdatedAt != current.UpdatedAt {
		return errs.ErrVersionConflict
	}

	// Lấy customer info - fallback về current nếu FE không gửi customer_id.
	// Story-37 (D15): bỏ vế "khách cùng company với đơn" — 1 thương hiệu/DB; khách phải tồn tại.
	// Không lọc chi nhánh (như trước): khách đăng ký ở chi nhánh khác vẫn lên đơn được.
	customerID := order.CustomerId
	if customerID == 0 {
		customerID = current.CustomerId
	}
	var cust struct {
		Status int `gorm:"column:status"`
	}
	if err := s.DB.Table("customer").Select("status").Where("id = ?", customerID).Take(&cust).Error; err != nil {
		return errs.ErrCustomerWrongTenant
	}

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// Khoá khách (cũ + mới nếu đổi) TRƯỚC mọi đọc/ghi — lens-money M-2, xem Create.
		if err := lockCustomers(tx, customerID, current.CustomerId); err != nil {
			return err
		}
		// lens-money M-11: đổi khách của đơn đã có dịch vụ/phiếu ⇒ dòng dịch vụ + phiếu vẫn mang khách cũ,
		// tổng tiền của khách cũ không tính lại (tổng nợ hệ thống nhân đôi). Chỉ cho đổi khi đơn còn trống.
		if customerID != current.CustomerId {
			var linked int64
			if err := tx.Raw("SELECT (SELECT COUNT(*) FROM order_service_used WHERE order_plan_id = ?) + "+
				"(SELECT COUNT(*) FROM payment WHERE parent_id = ?)", current.ID, current.ID).Scan(&linked).Error; err != nil {
				return err
			}
			if linked > 0 {
				return errs.NewLocalizedError("order_customer_change_blocked")
			}
		}
		// Merge fields
		merged := current
		s.mergeOrder(&merged, order)

		if err := validateOrderPlan(&merged); err != nil {
			return err
		}

		s.parseListRevenues(&merged)

		// Validate mỗi ID trong listRevenues là user thật — CHỈ khi client thực sự đổi list đó (Hard
		// Rule: validate FK/scope field chỉ khi giá trị body khác giá trị đang lưu — record cũ có thể
		// đã gán user không còn hợp lệ từ trước khi có rule này; validate lại giá trị KHÔNG đổi sẽ
		// chặn nhầm save hợp lệ không liên quan tới field này).
		if string(merged.ListRevenues) != string(current.ListRevenues) {
			if err := validateAssignedUsersExist(tx, merged.ListRevenues); err != nil {
				return err
			}
		}

		uid := uint(userID)
		if err := optlock.Save(tx.Set("userID", uid), &merged, current.UpdatedAt); err != nil {
			return err
		}

		// afterSave: update customer
		custUpdates := map[string]interface{}{}
		if orderAllowedCustomerStatuses[cust.Status] {
			custUpdates["status"] = define.CustomerStatusVisited
			custUpdates["connect_id"] = 1
		}
		// Load branch_id + code trong 1 query khi order có chi nhánh.
		// Khi merged.BranchId == nil: không có gì để set → skip hoàn toàn.
		if merged.BranchId != nil {
			var custBranchCode struct {
				BranchId *int   `gorm:"column:branch_id"`
				Code     string `gorm:"column:code"`
			}
			tx.Table("customer").
				Select("branch_id, COALESCE(code,'') as code").
				Where("id = ?", merged.CustomerId).Scan(&custBranchCode)
			if custBranchCode.BranchId == nil {
				custUpdates["branch_id"] = *merged.BranchId
			}
			if custBranchCode.Code == "" {
				branchID := *merged.BranchId
				if custBranchCode.BranchId != nil {
					branchID = *custBranchCode.BranchId
				}
				if code := codegen.NewCustomerCode(tx, int64(branchID)); code != "" {
					custUpdates["code"] = code
				}
			}
		}
		if len(custUpdates) > 0 {
			tx.Table("customer").Where("id = ?", merged.CustomerId).Updates(custUpdates)
		}

		// Xử lý services (update mode)
		if len(req.ServicesData) > 0 {
			if err := s.handleServices(tx, &merged, req.ServicesData, true, uid); err != nil {
				return err
			}
		}

		// Recalc debtsPlan từ SUM(osu.totalEnd) sau khi services đã được upsert/delete
		if err := s.recalcDebtsPlan(tx, merged.ID); err != nil {
			return err
		}

		// Cập nhật totalRevenue + totalDebt sau khi services đã được upsert/delete
		s.updateCustomerRevenue(tx, merged.CustomerId)

		*order = merged
		return nil
	})
	return err
}

// --- Delete ---

// Delete xoá đơn hàng sau khi check constraints.
// Không cho xoá nếu bất kỳ service nào đã có payment.
// Xoá theo thứ tự: con của order_service_used → order_service_used → con của đơn → order.
// checkDeleteConstraints — pre-flight check for Delete.
// Returns service_used IDs (cho caller dùng để xoá children) hoặc sentinel error
// nếu có payment đang reference các service.
//
// Tách ra method riêng để unit-test qua fake Reader (Phase 4 refactor pattern).
func (s *OrderPlanService) checkDeleteConstraints(tx *gorm.DB, orderID uint) ([]uint, error) {
	svcIDs, err := s.repo.OrderServiceUsedIDs(tx, orderID)
	if err != nil {
		return nil, err
	}
	if len(svcIDs) == 0 {
		return svcIDs, nil
	}

	paymentCount, err := s.repo.CountPaymentsForServices(tx, svcIDs)
	if err != nil {
		return nil, err
	}
	if paymentCount > 0 {
		return nil, errs.NewLocalizedErrorWrapping("service_delete_blocked", errs.ErrServiceHasPayment, "payment")
	}

	return svcIDs, nil
}

func (s *OrderPlanService) Delete(id, userID uint, scopedDB *gorm.DB) error {
	var order orderPlanDao.OrderPlan
	if err := scopedDB.First(&order, id).Error; err != nil {
		return errs.ErrNotFound
	}

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// Khoá khách TRƯỚC mọi đọc/ghi — lens-money M-2, xem Create.
		if err := lockCustomers(tx, order.CustomerId); err != nil {
			return err
		}
		// Pre-flight: check per-service constraints (giống PHP: loop từng orderServiceUsed)
		svcIDs, err := s.checkDeleteConstraints(tx, id)
		if err != nil {
			return err
		}

		// Xóa children của order_service_used trước (theo FK dependency)
		if len(svcIDs) > 0 {
			// appointment.order_service_used_id FK RESTRICT — phải NULL trước khi xóa OSU
			if err := tx.Exec("UPDATE appointment SET order_service_used_id = NULL, "+optlock.RawSet+" WHERE order_service_used_id IN ?", svcIDs).Error; err != nil {
				return fmt.Errorf("null appointment.order_service_used_id for order %d: %w", id, err)
			}
			if err := tx.Exec("DELETE FROM log_order_service_used WHERE order_service_used_id IN ?", svcIDs).Error; err != nil {
				return fmt.Errorf("delete log_order_service_used for order %d: %w", id, err)
			}
			// 1 dòng log xoá / dòng dịch vụ — bảng đơn không có log riêng, không ghi thì xoá đơn mất dấu.
			if err := logDeleteOSUs(tx, svcIDs, int(userID)); err != nil {
				return err
			}
			if err := tx.Exec("DELETE FROM order_installment WHERE order_service_used_id IN ?", svcIDs).Error; err != nil {
				return fmt.Errorf("delete order_installment for order %d: %w", id, err)
			}
			if err := tx.Exec("DELETE FROM order_commission_telesale WHERE order_service_used_id IN ?", svcIDs).Error; err != nil {
				return fmt.Errorf("delete order_commission_telesale for order %d: %w", id, err)
			}
			if err := tx.Exec("DELETE FROM order_revenue_telesale WHERE order_service_used_id IN ?", svcIDs).Error; err != nil {
				return fmt.Errorf("delete order_revenue_telesale for order %d: %w", id, err)
			}
		}
		// Xóa order_service_used
		if err := tx.Exec("DELETE FROM order_service_used WHERE order_plan_id = ?", id).Error; err != nil {
			return err
		}
		// Xóa children trực tiếp của order_plan
		if err := tx.Exec("DELETE FROM order_commission_telesale WHERE order_plan_id = ?", id).Error; err != nil {
			return fmt.Errorf("delete order_commission_telesale (plan) for order %d: %w", id, err)
		}
		if err := tx.Exec("DELETE FROM order_revenue_telesale WHERE order_plan_id = ?", id).Error; err != nil {
			return fmt.Errorf("delete order_revenue_telesale (plan) for order %d: %w", id, err)
		}
		// Payment theo parent_id bị xoá kèm đơn — ghi log xoá từng phiếu (log_payment FK SET NULL).
		var parentPaymentIDs []int
		if err := tx.Table("payment").Where("parent_id = ?", id).Pluck("id", &parentPaymentIDs).Error; err != nil {
			return fmt.Errorf("list payment for order %d: %w", id, err)
		}
		for _, pid := range parentPaymentIDs {
			if err := history.LogDeleteSnapshot(tx, "log_payment", pid, history.PaymentDeleteFields, int(userID)); err != nil {
				return err
			}
		}
		if err := tx.Exec("DELETE FROM payment WHERE parent_id = ?", id).Error; err != nil {
			return fmt.Errorf("delete payment for order %d: %w", id, err)
		}

		if err := tx.Delete(&order).Error; err != nil {
			return err
		}

		// Cập nhật totalRevenue + totalDebt sau khi xóa services
		s.updateCustomerRevenue(tx, order.CustomerId)

		return nil
	})
	return err
}

// --- Private helpers ---

// logDeleteOSUs ghi 1 dòng log_order_service_used action_type=delete cho mỗi dòng dịch vụ sắp xoá.
// PHẢI gọi SAU lệnh dọn log cũ và TRƯỚC `DELETE FROM order_service_used` (xem LogDeleteSnapshot).
func logDeleteOSUs(tx *gorm.DB, ids []uint, userID int) error {
	for _, osuID := range ids {
		if err := history.LogDeleteSnapshot(tx, "log_order_service_used", int(osuID), history.OrderServiceUsedDeleteFields, userID); err != nil {
			return err
		}
	}
	return nil
}

// recalcDebtsPlan: tính lại debtsPlan = (SUM(osu.totalEnd) − SUM(osu.refundServiceUsed)) − paymentPlan sau khi
// services đã được lưu (D20a story-56: hoàn tiền giảm cả doanh thu — cùng công thức calculatorOrderPlan).
// Gọi AFTER handleServices để đảm bảo debtsPlan chính xác theo service thực tế.
// lens-money M-10: paymentPlan tính lại từ NGUỒN (SUM phiếu thanh toán/chuyển cọc/hoàn theo parent_id — chính
// calculatorOrderPlan của luồng phiếu), KHÔNG đọc lại cột paymentPlan (bản cũ đọc cột ⇒ body gửi
// `paymentPlan` là đổi được công nợ đơn).
func (s *OrderPlanService) recalcDebtsPlan(tx *gorm.DB, orderID uint) error {
	return (&PaymentService{DB: tx}).calculatorOrderPlan(tx, int(orderID), define.PaymentForOrder)
}

// parseListRevenues xử lý trường hợp FE gửi listRevenues dạng JSON string lồng.
func (s *OrderPlanService) parseListRevenues(order *orderPlanDao.OrderPlan) {
	if order.ListRevenues != nil {
		var str string
		if err := json.Unmarshal(order.ListRevenues, &str); err == nil {
			var arr []int
			if json.Unmarshal([]byte(str), &arr) == nil {
				data, _ := json.Marshal(arr)
				order.ListRevenues = data
			}
		}
	}
}

// handleServices xử lý tạo/cập nhật/xoá OrderServiceUsed children.
// Giống PHP handleServicesCreation() — logic phức tạp nhất trong file:
//
//  1. Batch load existing services (tránh N+1)
//  2. Với mỗi service từ FE:
//     a. Nếu existing → selective merge (giữ ID, base fields). Nếu mới → json.Unmarshal
//     b. Set fields kế thừa từ order (branch, appointment, revenue, source...)
//     c. Tính rank_discount_amount server-side = totalEnd * rank_discount_percent / 100
//     d. GORM Create hoặc Save
//     e. Xử lý installment: có config → tạo/update kỳ trả góp; không có → xoá hết
//  3. Delete orphan services (chỉ update): check payment + isLock trước khi xoá
func (s *OrderPlanService) handleServices(tx *gorm.DB, order *orderPlanDao.OrderPlan, servicesData []map[string]interface{}, isUpdate bool, userID uint) error {
	var oldIDs []uint
	if isUpdate {
		oldIDs, _ = s.repo.OrderServiceUsedIDs(tx, order.ID)
	}

	// Batch load existing services (tránh N+1 query)
	existingSvcMap := map[uint]crmServiceDao.OrderServiceUsed{}
	if isUpdate {
		var reqIDs []uint
		for _, sd := range servicesData {
			if idVal, ok := sd["id"]; ok && idVal != nil {
				if svcID := uint(helpers.ToInt64(idVal)); svcID > 0 {
					reqIDs = append(reqIDs, svcID)
				}
			}
		}
		if len(reqIDs) > 0 {
			var existingSvcs []crmServiceDao.OrderServiceUsed
			tx.Where("id IN ? AND order_plan_id = ?", reqIDs, order.ID).Find(&existingSvcs)
			for i := range existingSvcs {
				existingSvcMap[existingSvcs[i].ID] = existingSvcs[i]
			}
		}
	}

	usedIDs := map[uint]bool{}

	for _, svcData := range servicesData {
		// Validate quantity > 0
		qv, hasQty := svcData["quantity"]
		if !hasQty {
			return errs.ErrQuantityRequired
		}
		q := int(helpers.ToInt64(qv))
		if q <= 0 {
			return errs.ErrQuantityMustBePositive
		}

		var svc crmServiceDao.OrderServiceUsed
		isExisting := false

		// Nếu update và có id → lookup từ batch-loaded map
		if isUpdate {
			if idVal, ok := svcData["id"]; ok && idVal != nil {
				svcID := uint(helpers.ToInt64(idVal))
				if svcID > 0 {
					if existing, found := existingSvcMap[svcID]; found {
						svc = existing
						usedIDs[svcID] = true
						isExisting = true
					}
				}
			}
		}

		var oldSnapshot map[string]interface{}
		if isExisting {
			oldSnapshot = history.SnapshotRow(tx, "order_service_used", svc.ID, history.OrderServiceUsedIncludeFields)
			// Guard: OSU đã có payment > 0 thì KHÔNG cho đổi service_id — đổi ngầm sẽ làm lệch
			// snapshot commission, EMR sync theo
			// service_id, report/export group-by service_id. FE chỉ disable control
			// (order-plan-form.ts), phải chặn cứng ở BE.
			if v, ok := svcData["service_id"]; ok {
				newServiceID := int(helpers.ToInt64(v))
				if newServiceID != svc.ServiceId {
					existingPaymentBeforeMerge := helpers.DerefDecimal(svc.Payment)
					if existingPaymentBeforeMerge.IsPositive() {
						return errs.NewLocalizedError("service_id_locked_by_payment")
					}
				}
			}
			// Update: selective merge, giữ nguyên ID + base fields
			mergeServiceData(&svc, svcData)
			// Guard: chỉ kiểm tra khi FE tường minh gửi totalEnd (tránh false-reject với legacy
			// row có totalEnd = NULL khi FE chỉ update các field khác như note).
			// payment trên order_service_used là SUM thực tế từ bảng payment — không thể hoàn tác.
			if _, totalEndSent := svcData["totalEnd"]; totalEndSent {
				existingPayment := helpers.DerefDecimal(svc.Payment)
				newTotalEnd := helpers.DerefDecimal(svc.TotalEnd)
				if existingPayment.IsPositive() && newTotalEnd.LessThan(existingPayment) {
					return errs.NewLocalizedError("total_end_below_payment",
						newTotalEnd.StringFixed(0), existingPayment.StringFixed(0))
				}
			}
		} else {
			// Create: bind toàn bộ từ FE data
			data, err := json.Marshal(svcData)
			if err != nil {
				return fmt.Errorf("marshal service data: %w", err)
			}
			if err := json.Unmarshal(data, &svc); err != nil {
				return fmt.Errorf("unmarshal service data: %w", err)
			}
			// FE có thể gửi id/code từ record cũ (copy order) → phải reset để
			// luôn đi qua Create + BeforeCreate sinh code mới thay vì Save (UPDATE).
			svc.ID = 0
			svc.Code = ""
			// lens-money M-16: số đã thu / hoàn / trạng thái TT / khoá là số server tính từ phiếu — dòng mới
			// luôn bắt đầu từ 0, không nhận từ body (copy đơn cũ hoặc request tự soạn).
			svc.Payment = nil
			svc.RefundServiceUsed = nil
			svc.PaymentStatus = nil
			svc.DiscountServiceLock = decimal.Zero
			svc.IsLock = 0 // 0 ⇒ GORM bỏ cột ⇒ DB default (bình thường)
			svc.CommissionTelesaleSnap = nil
			svc.CommissionTelesaleTypeSnap = nil
		}

		// Load service master info (serviceCategory_id, time_reminder) — dùng cho dayReExploitation
		// cho SẢN PHẨM (khối `if svc.ID == 0` bên dưới).
		var srv struct {
			ServiceCategoryId *int `gorm:"column:serviceCategory_id"`
			TimeReminder      *int `gorm:"column:time_reminder"`
		}
		if svc.ServiceId > 0 {
			tx.Table("service").Select("serviceCategory_id, time_reminder").
				Where("id = ?", svc.ServiceId).Take(&srv)
		}

		// Set fields from order
		svc.OrderPlanId = int(order.ID)
		if order.BranchId != nil {
			svc.BranchId = order.BranchId
		}
		if order.AppointmentId != nil {
			svc.AppointmentId = order.AppointmentId
		}
		if order.SourceId != nil {
			svc.SourceId = order.SourceId
		}
		if order.CreatedTime != nil {
			svc.CreatedDay = order.CreatedTime
		}
		svc.ListRevenues = order.ListRevenues
		svc.CustomerId = order.CustomerId

		// rank_discount_amount: tính server-side (giống PHP populateOrderServiceUsed).
		// amt = totalEnd * rank_discount_percent / 100
		rankPct := helpers.DerefDecimal(svc.RankDiscountPercent)
		if rankPct.IsPositive() {
			svc.RankDiscountAmount = new(helpers.DerefDecimal(svc.TotalEnd).Mul(rankPct).Div(decimal.NewFromInt(100)))
		} else {
			svc.RankDiscountAmount = new(decimal.Zero)
		}

		if err := validateOrderServiceUsed(&svc); err != nil {
			return err
		}
		// lens-money M-16: total/totalEnd client gửi phải khớp công thức từ đơn giá/số lượng/chiết khấu.
		// Dòng cũ chỉ kiểm khi một trường tiền thực sự đổi (dữ liệu nhập cũ có thể lệch công thức).
		var prevSvc *crmServiceDao.OrderServiceUsed
		if isExisting {
			if e, ok := existingSvcMap[svc.ID]; ok {
				prevSvc = &e
			}
		}
		if err := checkServiceUsedMoney(&svc, prevSvc); err != nil {
			return err
		}
		if svc.ID == 0 {
			// Tái khai thác cho SẢN PHẨM (serviceCategory_id=2): set 1 lần lúc lên đơn.
			if svc.DayReExploitation == nil && svc.CreatedDay != nil &&
				srv.ServiceCategoryId != nil && *srv.ServiceCategoryId == define.ServiceCategoryProduct &&
				srv.TimeReminder != nil && *srv.TimeReminder > 0 {
				svc.DayReExploitation = new(*svc.CreatedDay + int64(*srv.TimeReminder)*86400)
			}
			// Snapshot commission rates từ service — lock tại thời điểm lên đơn.
			// Nếu service đổi rate sau, đơn này vẫn dùng rate cũ khi tính hoa hồng thanh toán.
			snapshotCommissionRates(tx, order, &svc)
			if err := tx.Set("userID", userID).Create(&svc).Error; err != nil {
				return fmt.Errorf("Lỗi lưu dịch vụ: %w", err)
			}
		} else {
			existing, foundExisting := existingSvcMap[svc.ID]
			if err := tx.Set("userID", userID).Save(&svc).Error; err != nil {
				return fmt.Errorf("Lỗi lưu dịch vụ: %w", err)
			}
			// Tính lại order_revenue_telesale cho các payment đã có của dịch vụ này — CHỈ khi staff
			// thực sự đổi listRevenues (tránh DELETE+INSERT thừa mỗi lần sửa đơn không liên quan,
			// vd. chỉ đổi note). totalEnd đổi KHÔNG cần resync vì revenue tính theo payment.amount
			// thực nhận, không theo totalEnd.
			if !foundExisting ||
				string(existing.ListRevenues) != string(svc.ListRevenues) {
				resyncRevenueForOSU(tx, int(svc.ID), int(order.ID), svc.CustomerId)
			}
			newSnapshot := history.SnapshotRow(tx, "order_service_used", svc.ID, history.OrderServiceUsedIncludeFields)
			_ = history.LogChanges(tx, "log_order_service_used", "order_service_used_id", int(svc.ID),
				oldSnapshot, newSnapshot,
				history.OrderServiceUsedIncludeFields, history.OrderServiceUsedSkipNullFields, int(userID))
		}

		// Installment handling (giống PHP createOrderInstallments)
		if cfgRaw, ok := svcData["installment_config"]; ok && cfgRaw != nil {
			if err := s.handleInstallments(tx, &svc, cfgRaw); err != nil {
				return err
			}
		} else {
			if err := tx.Exec("DELETE FROM order_installment WHERE order_service_used_id = ?", svc.ID).Error; err != nil {
				return fmt.Errorf("delete installments for service_used %d: %w", svc.ID, err)
			}
		}

		// lens-money M-9: sửa totalEnd (hoặc lịch trả góp) của dòng ĐÃ có ⇒ tính lại payment_status + phân bổ
		// kỳ trả góp theo số đã thu (bản cũ giữ Done khi nâng totalEnd, kỳ góp giữ paid_amount cũ).
		if isExisting {
			prev := existingSvcMap[svc.ID]
			_, hasInstallment := svcData["installment_config"]
			if hasInstallment || !helpers.DerefDecimal(prev.TotalEnd).Equal(helpers.DerefDecimal(svc.TotalEnd)) {
				ps := &PaymentService{DB: tx}
				ps.calculatorPaymentServiceUsed(tx, int(svc.ID))
				ps.updateInstallmentPayment(tx, int(svc.ID))
			}
		}
	}

	// Delete orphan services (only for update)
	if isUpdate {
		// Step 1: collect candidates (skip usedIDs already filtered above, but loop may include them)
		candidates := make([]uint, 0, len(oldIDs))
		for _, id := range oldIDs {
			if !usedIDs[id] {
				candidates = append(candidates, id)
			}
		}
		if len(candidates) > 0 {
			// Step 2: single batch load of all candidate orphans
			var orphans []crmServiceDao.OrderServiceUsed
			tx.Select("id, customer_id, service_id, order_plan_id, isLock").
				Where("id IN ?", candidates).Find(&orphans)
			orphanByID := make(map[uint]*crmServiceDao.OrderServiceUsed, len(orphans))
			for i := range orphans {
				orphanByID[orphans[i].ID] = &orphans[i]
			}

			// Step 3: in-memory validate + collect deletable IDs; preserve per-orphan side-effects
			deletableIDs := make([]uint, 0, len(candidates))
			for _, id := range candidates {
				o, ok := orphanByID[id]
				if !ok {
					continue
				}
				if o.IsLock == define.ServiceUsedIsLock {
					return errs.ErrServiceHasTreatment
				}
				deletableIDs = append(deletableIDs, id)
			}

			// Step 4: bulk DELETE
			if len(deletableIDs) > 0 {
				if err := tx.Exec("DELETE FROM log_order_service_used WHERE order_service_used_id IN ?", deletableIDs).Error; err != nil {
					return fmt.Errorf("delete log_order_service_used for orphan services: %w", err)
				}
				if err := logDeleteOSUs(tx, deletableIDs, int(userID)); err != nil {
					return err
				}
				if err := tx.Exec("DELETE FROM order_installment WHERE order_service_used_id IN ?", deletableIDs).Error; err != nil {
					return fmt.Errorf("delete installments for orphan services: %w", err)
				}
				if err := tx.Exec("DELETE FROM order_commission_telesale WHERE order_service_used_id IN ?", deletableIDs).Error; err != nil {
					return fmt.Errorf("delete order_commission_telesale for orphan services: %w", err)
				}
				if err := tx.Exec("DELETE FROM order_revenue_telesale WHERE order_service_used_id IN ?", deletableIDs).Error; err != nil {
					return fmt.Errorf("delete order_revenue_telesale for orphan services: %w", err)
				}
				if err := tx.Exec("DELETE FROM order_service_used WHERE id IN ?", deletableIDs).Error; err != nil {
					return fmt.Errorf("delete orphan order_service_used: %w", err)
				}
			}
		}
	}

	return nil
}

// updateCustomerRevenue tính lại totalRevenue + totalDebt cho customer sau khi services thay đổi.
// totalRevenue = ΣtotalEnd − ΣrefundServiceUsed; totalDebt = totalRevenue − đã thu ròng.
// lens-money M-2: đã thu ròng tính từ NGUỒN (SUM phiếu thanh toán + chuyển cọc + hoàn) — bản cũ đọc
// cột customer.totalPayment, có thể là số cũ nếu phiếu thu commit xen giữa. Caller đã khoá customer.
func (s *OrderPlanService) updateCustomerRevenue(tx *gorm.DB, customerID int) {
	if customerID == 0 {
		return
	}
	var totalRevenue, totalPayment decimal.Decimal
	tx.Table("order_service_used").
		Select("COALESCE(SUM(totalEnd), 0) - COALESCE(SUM(refundServiceUsed), 0)").
		Where("customer_id = ?", customerID).Scan(&totalRevenue)
	tx.Table("payment").
		Select("COALESCE(SUM(amount), 0)").
		Where("customer_id = ? AND payment_for IN ?", customerID, netPaidPaymentFors).Scan(&totalPayment)

	tx.Table("customer").Where("id = ?", customerID).Updates(map[string]interface{}{
		"totalRevenue": totalRevenue,
		"totalPayment": totalPayment,
		"totalDebt":    totalRevenue.Sub(totalPayment),
	})
}

// RecalcAfterServiceUsedLock — tính lại tiền sau khi khoá/mở khoá 1 dòng dịch vụ (totalEnd đổi):
// paymentPlan/debtsPlan của đơn + totalRevenue/totalPayment/totalDebt của khách, đều tính từ nguồn.
// lens-money M-14: bản cũ chỉ tính lại khách ⇒ debtsPlan đơn vẫn giữ phần nợ của dịch vụ đã khoá.
// Caller PHẢI đã khoá dòng customer (thứ tự khoá: payment_lock.go).
func RecalcAfterServiceUsedLock(tx *gorm.DB, orderPlanID, customerID int) error {
	if err := (&PaymentService{DB: tx}).calculatorOrderPlan(tx, orderPlanID, define.PaymentForOrder); err != nil {
		return err
	}
	(&OrderPlanService{DB: tx}).updateCustomerRevenue(tx, customerID)
	return nil
}

// handleInstallments tạo/cập nhật kỳ trả góp cho 1 service.
// Flow:
//   - Parse installment_config (period, deposit_amount, amount_per_period, period_interval_days)
//   - Nếu số kỳ thay đổi → xoá hết tạo mới
//   - Kỳ 0: deposit (trả trước). Kỳ 1-N: trả góp, mỗi kỳ cách period_interval_days ngày
//   - Phân bổ số tiền đã thanh toán (svc.Payment) vào từng kỳ: PAID/PARTIAL/UNPAID
func (s *OrderPlanService) handleInstallments(tx *gorm.DB, svc *crmServiceDao.OrderServiceUsed, cfgRaw interface{}) error {
	cfgBytes, _ := json.Marshal(cfgRaw)
	var cfgRawData struct {
		Period             int             `json:"period"`
		DepositAmount      decimal.Decimal `json:"deposit_amount"`
		AmountPerPeriod    decimal.Decimal `json:"amount_per_period"`
		PeriodIntervalDays int             `json:"period_interval_days"`
	}
	if err := json.Unmarshal(cfgBytes, &cfgRawData); err != nil {
		return nil
	}
	if cfgRawData.PeriodIntervalDays == 0 {
		cfgRawData.PeriodIntervalDays = 30
	}
	if cfgRawData.Period <= 0 || !cfgRawData.AmountPerPeriod.IsPositive() {
		return nil
	}
	depositAmount := cfgRawData.DepositAmount
	amountPerPeriod := cfgRawData.AmountPerPeriod

	// Lấy các kỳ hiện tại
	var existing []crmPaymentDao.OrderInstallment
	tx.Where("order_service_used_id = ?", svc.ID).Order("period_number ASC").Find(&existing)
	existMap := map[int]*crmPaymentDao.OrderInstallment{}
	for i := range existing {
		existMap[existing[i].PeriodNumber] = &existing[i]
	}

	// Nếu số kỳ thay đổi, xóa hết và tạo mới
	expectedCount := cfgRawData.Period + 1 // +1 cho kỳ 0
	if len(existing) > 0 && len(existing) != expectedCount {
		if err := tx.Exec("DELETE FROM order_installment WHERE order_service_used_id = ?", svc.ID).Error; err != nil {
			return fmt.Errorf("delete installments for service_used %d: %w", svc.ID, err)
		}
		existMap = map[int]*crmPaymentDao.OrderInstallment{}
	}

	// Tính totalPaid và lastPaymentDate
	totalPaid := helpers.DerefDecimal(svc.Payment)
	var lastPaymentDate *string
	if totalPaid.IsPositive() {
		var payDate *int64
		tx.Table("payment").
			Select("payment_of_date").
			Where("serviceUsed_id = ? AND payment_for IN (?, ?)", svc.ID, define.PaymentForOrder, define.PaymentForMoveDeposit).
			Order("payment_of_date DESC").Limit(1).Scan(&payDate)
		if payDate != nil {
			lastPaymentDate = new(time.Unix(*payDate, 0).Format("2006-01-02"))
		}
	}

	// Base date — dùng timezone của thương hiệu (pkg/brand, story-37) để due dates tính đúng ngày địa phương
	loc := brand.Location(tx)
	var baseDate time.Time
	if svc.CreatedDay != nil {
		baseDate = time.Unix(*svc.CreatedDay, 0).In(loc)
	} else {
		baseDate = time.Now().In(loc)
	}

	remainingPaid := totalPaid

	// Kỳ 0: Trả trước (Deposit)
	if inst, ok := existMap[0]; ok {
		inst.Amount = depositAmount
		remainingPaid = remainingPaid.Sub(helpers.DerefDecimal(inst.PaidAmount))
		if err := tx.Save(inst).Error; err != nil {
			return fmt.Errorf("save installment period 0 for service_used %d: %w", svc.ID, err)
		}
	} else {
		inst := crmPaymentDao.OrderInstallment{
			OrderServiceUsedId: int(svc.ID),
			PeriodNumber:       0,
			Amount:             depositAmount,
			DueDate:            baseDate,
		}
		paid, status := allocatePayment(remainingPaid, depositAmount)
		inst.PaidAmount = &paid
		inst.Status = &status
		if status == 2 && lastPaymentDate != nil { // STATUS_PAID
			t, _ := time.Parse("2006-01-02", *lastPaymentDate)
			inst.PaidDate = &t
		}
		remainingPaid = remainingPaid.Sub(paid)
		if err := tx.Create(&inst).Error; err != nil {
			return fmt.Errorf("create installment period 0 for service_used %d: %w", svc.ID, err)
		}
	}

	// Kỳ cuối nhận phần dư để tổng kỳ = totalEnd - deposit (tránh lệch do Math.ceil FE)
	totalEnd := helpers.DerefDecimal(svc.TotalEnd)
	remainder := totalEnd.Sub(depositAmount).Sub(amountPerPeriod.Mul(decimal.NewFromInt(int64(cfgRawData.Period - 1))))
	if remainder.LessThan(decimal.Zero) {
		remainder = decimal.Zero
	}

	// Kỳ 1-N
	for i := 1; i <= cfgRawData.Period; i++ {
		periodAmount := amountPerPeriod
		if i == cfgRawData.Period {
			periodAmount = remainder
		}
		if inst, ok := existMap[i]; ok {
			inst.Amount = periodAmount
			remainingPaid = remainingPaid.Sub(helpers.DerefDecimal(inst.PaidAmount))
			if err := tx.Save(inst).Error; err != nil {
				return fmt.Errorf("save installment period %d for service_used %d: %w", i, svc.ID, err)
			}
		} else {
			dueDate := baseDate.AddDate(0, 0, i*cfgRawData.PeriodIntervalDays)
			inst := crmPaymentDao.OrderInstallment{
				OrderServiceUsedId: int(svc.ID),
				PeriodNumber:       i,
				Amount:             periodAmount,
				DueDate:            dueDate,
			}
			paid, status := allocatePayment(remainingPaid, periodAmount)
			inst.PaidAmount = &paid
			inst.Status = &status
			if status == 2 && lastPaymentDate != nil {
				t, _ := time.Parse("2006-01-02", *lastPaymentDate)
				inst.PaidDate = &t
			}
			remainingPaid = remainingPaid.Sub(paid)
			if err := tx.Create(&inst).Error; err != nil {
				return fmt.Errorf("create installment period %d for service_used %d: %w", i, svc.ID, err)
			}
		}
	}

	return nil
}

// allocatePayment: phân bổ số tiền đã thanh toán vào kỳ, trả về (paid, status)
// status: 1=UNPAID, 2=PAID, 3=PARTIAL
func allocatePayment(remaining, amount decimal.Decimal) (decimal.Decimal, int) {
	if !remaining.IsPositive() {
		return decimal.Zero, 1 // UNPAID
	}
	paid := remaining
	if paid.GreaterThan(amount) {
		paid = amount
	}
	if paid.GreaterThanOrEqual(amount) {
		return paid, 2 // PAID
	}
	return paid, 3 // PARTIAL
}

// mergeServiceData ghi đè selective các field từ FE map vào existing OrderServiceUsed.
// Giữ nguyên ID, UpdatedAt, CreatedAt, CreatedBy và các field không có trong map.
func mergeServiceData(svc *crmServiceDao.OrderServiceUsed, data map[string]interface{}) {
	if v, ok := data["service_id"]; ok {
		svc.ServiceId = int(helpers.ToInt64(v))
	}
	if v, ok := data["quantity"]; ok {
		svc.Quantity = new(int(helpers.ToInt64(v)))
	}
	if v, ok := data["unit_price"]; ok {
		svc.UnitPrice = new(helpers.ToDecimal(v))
	}
	if v, ok := data["service_discount"]; ok {
		svc.ServiceDiscount = new(helpers.ToDecimal(v))
	}
	if v, ok := data["service_discount_type"]; ok {
		svc.ServiceDiscountType = new(int(helpers.ToInt64(v)))
	}
	if v, ok := data["discount_other"]; ok {
		svc.DiscountOther = new(helpers.ToDecimal(v))
	}
	if v, ok := data["note_discount"]; ok {
		svc.NoteDiscount, _ = v.(string)
	}
	if v, ok := data["total"]; ok {
		svc.Total = new(helpers.ToDecimal(v))
	}
	if v, ok := data["totalEnd"]; ok {
		svc.TotalEnd = new(helpers.ToDecimal(v))
	}
	if v, ok := data["rank_discount_percent"]; ok {
		svc.RankDiscountPercent = new(helpers.ToDecimal(v))
	}
	if v, ok := data["rank_discount_amount"]; ok {
		svc.RankDiscountAmount = new(helpers.ToDecimal(v))
	}
	if v, ok := data["note"]; ok {
		svc.Note, _ = v.(string)
	}
	if v, ok := data["installment_config"]; ok {
		if v == nil {
			svc.InstallmentConfig = nil
		} else if b, err := json.Marshal(v); err == nil {
			svc.InstallmentConfig = b
		}
	}
}

// mergeOrder ghi đè selective các field từ FE request vào current order.
// Chỉ overwrite field nào FE gửi (non-zero/non-nil), giữ nguyên phần còn lại.
func (s *OrderPlanService) mergeOrder(dst *orderPlanDao.OrderPlan, src *orderPlanDao.OrderPlan) {
	if src.CustomerId != 0 {
		dst.CustomerId = src.CustomerId
	}
	if src.AppointmentId != nil {
		dst.AppointmentId = src.AppointmentId
	}
	if src.BranchId != nil {
		dst.BranchId = src.BranchId
	}

	if src.DiscountOther != nil {
		dst.DiscountOther = src.DiscountOther
	}
	// paymentPlan/debtsPlan KHÔNG merge từ body — server tính (lens-money M-10).
	if src.ListRevenues != nil {
		dst.ListRevenues = src.ListRevenues
	}
	if src.SourceId != nil {
		dst.SourceId = src.SourceId
	}
	if src.CreatedTime != nil {
		dst.CreatedTime = src.CreatedTime
	}
	dst.Note = src.Note
	if src.Status != nil {
		dst.Status = src.Status
	}
}

// snapshotCommissionRates đọc commission config từ service và ghi snapshot vào svc.
// Chỉ gọi khi tạo mới (svc.ID == 0). OrderPlanType quyết định dùng rate khách cũ/mới.
func snapshotCommissionRates(tx *gorm.DB, order *orderPlanDao.OrderPlan, svc *crmServiceDao.OrderServiceUsed) {
	if svc.ServiceId == 0 {
		return
	}
	var snap struct {
		CommissionTelesale     decimal.Decimal `gorm:"column:commission_telesale"`
		CommissionTelesaleNew  decimal.Decimal `gorm:"column:commission_telesale_new"`
		CommissionTelesaleType int             `gorm:"column:commission_telesale_type"`
	}
	if err := tx.Table("service").
		Select(`COALESCE(commission_telesale,0) as commission_telesale,
			COALESCE(commission_telesale_new,0) as commission_telesale_new,
			COALESCE(commission_telesale_type,1) as commission_telesale_type`).
		Where("id = ?", svc.ServiceId).Take(&snap).Error; err != nil {
		return
	}

	isOld := order.OrderPlanType != nil && *order.OrderPlanType == define.OrderPlanTypeOld

	var telesaleVal decimal.Decimal
	if isOld {
		telesaleVal = snap.CommissionTelesale
	} else {
		telesaleVal = snap.CommissionTelesaleNew
	}

	svc.CommissionTelesaleSnap = &telesaleVal
	svc.CommissionTelesaleTypeSnap = &snap.CommissionTelesaleType
}
