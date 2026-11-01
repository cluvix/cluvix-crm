package service

import (
	"fmt"

	"gorm.io/gorm"

	customerService "erpCluvix/internal/modules/webapp/crm/customer/service"
	"erpCluvix/pkg/errs"
)

// DeleteCustomer xoá khách hàng (hard-delete cascade lịch hẹn, ghi chú, nhắc việc, log) trong 1 transaction.
// Đường xoá khách DUY NHẤT của handler (web, mobile delegate, /emr/patient).
//
// CHẶN khi khách đã có đơn hàng HOẶC bất kỳ phiếu thu/chi nào (thanh toán, cọc, hoàn — gắn thẳng khách, theo
// dòng dịch vụ hay theo đơn) — user chốt 2026-11-01: dữ liệu tiền không được mất theo lần xoá khách. Bản cũ
// xoá kèm toàn bộ đơn + phiếu (qua deletePayment từng phiếu). FK payment.customer_id / serviceUsed_id /
// parent_id (RESTRICT) vẫn là lưới cuối nếu phiếu xuất hiện giữa lúc kiểm và lúc xoá.
func DeleteCustomer(db *gorm.DB, customerID uint, userID int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Khoá customer là câu ĐẦU TIÊN (thứ tự khoá chung — payment_lock.go): đơn/phiếu mới của khách phải xếp
		// hàng sau lần xoá này ⇒ số đếm bên dưới không lọt phiếu vừa commit.
		if err := lockCustomers(tx, int(customerID)); err != nil {
			return err
		}
		orders, payments, err := customerMoneyRecords(tx, customerID)
		if err != nil {
			return err
		}
		if orders > 0 || payments > 0 {
			return errs.NewLocalizedError("customer_delete_has_money", orders, payments)
		}
		return customerService.NewCustomerService(tx).DeleteWithCleanupTx(tx, customerID, userID)
	})
}

// customerMoneyRecords — số đơn hàng và số phiếu thu/chi của khách (phiếu gắn thẳng customer_id, theo dòng dịch
// vụ hoặc theo đơn của khách — cùng phạm vi bản xoá kèm phiếu cũ).
func customerMoneyRecords(tx *gorm.DB, customerID uint) (orders, payments int64, err error) {
	if err = tx.Table("order_plan").Where("customer_id = ?", customerID).Count(&orders).Error; err != nil {
		return 0, 0, fmt.Errorf("count customer orders: %w", err)
	}
	if err = tx.Table("payment").Where(`customer_id = ?
		OR serviceUsed_id IN (SELECT id FROM order_service_used WHERE customer_id = ?)
		OR parent_id IN (SELECT id FROM order_plan WHERE customer_id = ?)`, customerID, customerID, customerID).
		Count(&payments).Error; err != nil {
		return 0, 0, fmt.Errorf("count customer payments: %w", err)
	}
	return orders, payments, nil
}
