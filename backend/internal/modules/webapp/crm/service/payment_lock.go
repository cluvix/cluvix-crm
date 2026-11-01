package service

// Khoá dòng cho luồng tiền CRM (lens-money M-1..M-4, 2026-10-18).
//
// Thứ tự khoá NHẤT QUÁN mọi transaction ghi tiền của khách:
//
//	customer (id tăng dần) → order_service_used (id tăng dần) → payment cọc gốc (id tăng dần)
//
// `customer` là aggregate cha của mọi số tiền (totalPayment/totalDebt/totalDeposit…), khoá nó TRƯỚC thì
// mọi phiếu của cùng một khách chạy tuần tự ⇒ không lost-update tổng (M-2), không 2 lần hoàn/chuyển
// trên cùng một số dư (M-1/M-3). Khoá các dòng con sau đó là lưới thứ hai cho đường không đi qua
// customer (vd LockServiceUsed) và cho số đọc ra (FOR UPDATE luôn đọc bản commit mới nhất).
//
// ⚠ lockCustomers PHẢI là câu ĐẦU TIÊN trong tx: REPEATABLE READ chốt ảnh chụp ở lần đọc KHÔNG khoá đầu
// tiên — có một SELECT thường đứng trước thì tx sau xếp hàng đúng nhưng tới lượt vẫn đọc ảnh cũ
// (skill debugging-backend-queries §Trong 1 transaction — ảnh chụp REPEATABLE READ).

import (
	"errors"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"erpCluvix/pkg/errs"
)

// mysqlDeadlock — ER_LOCK_DEADLOCK: InnoDB đã rollback CẢ transaction nạn nhân.
const mysqlDeadlock = 1213

// mysqlLockWaitTimeout — ER_LOCK_WAIT_TIMEOUT: chờ khoá quá innodb_lock_wait_timeout. KHÔNG chạy lại (đã
// chờ cả chục giây) — trả errs.ErrPaymentBusyRetry để người dùng bấm lại thay vì lỗi MySQL thô (R-10).
const mysqlLockWaitTimeout = 1205

// paymentTxMaxAttempts — số lần chạy tối đa một tx tiền khi dính deadlock 1213.
const paymentTxMaxAttempts = 3

// lockCustomers khoá FOR UPDATE các dòng customer (bỏ id ≤ 0, bỏ trùng, theo id tăng dần).
func lockCustomers(tx *gorm.DB, ids ...int) error {
	uniq := make([]int, 0, len(ids))
	seen := map[int]bool{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	if len(uniq) == 0 {
		return nil
	}
	sort.Ints(uniq)
	var got []int
	return tx.Raw("SELECT id FROM customer WHERE id IN ? ORDER BY id FOR UPDATE", uniq).Scan(&got).Error
}

// derefInt — 0 khi nil (lockCustomers bỏ qua id ≤ 0).
func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// lockedServiceUsed — dòng dịch vụ đọc dưới FOR UPDATE.
type lockedServiceUsed struct {
	ID         int             `gorm:"column:id"`
	CustomerID *int            `gorm:"column:customer_id"`
	Payable    decimal.Decimal `gorm:"column:payable"` // totalEnd + VAT (bmad/order-vat D4)
	Payment    decimal.Decimal `gorm:"column:payment"`
}

// lockServiceUsed khoá + đọc 1 dòng order_service_used.
func lockServiceUsed(tx *gorm.DB, id int) (*lockedServiceUsed, error) {
	var su lockedServiceUsed
	if err := tx.Raw("SELECT id, customer_id, "+osuPayableSQL+" AS payable, COALESCE(payment, 0) AS payment "+
		"FROM order_service_used WHERE id = ? FOR UPDATE", id).Scan(&su).Error; err != nil {
		return nil, err
	}
	if su.ID == 0 {
		return nil, errs.NewLocalizedError("service_id_invalid", id)
	}
	return &su, nil
}

// lockDepositAfter khoá dòng phiếu cọc gốc và trả số dư `deposit_after` (bản commit mới nhất).
func lockDepositAfter(tx *gorm.DB, depositID int) (decimal.Decimal, error) {
	var row struct {
		ID           int             `gorm:"column:id"`
		DepositAfter decimal.Decimal `gorm:"column:deposit_after"`
	}
	if err := tx.Raw("SELECT id, COALESCE(deposit_after, 0) AS deposit_after FROM payment WHERE id = ? FOR UPDATE",
		depositID).Scan(&row).Error; err != nil {
		return decimal.Zero, err
	}
	if row.ID == 0 {
		return decimal.Zero, errs.ErrDepositNotFound
	}
	return row.DepositAfter, nil
}

// serviceUsedNetPaid — số đã thu RÒNG của 1 dòng dịch vụ (thanh toán + chuyển cọc + hoàn âm).
// Gọi SAU khi đã khoá customer/dòng dịch vụ để đọc bản mới nhất.
func serviceUsedNetPaid(tx *gorm.DB, serviceUsedID int) (decimal.Decimal, error) {
	var paid decimal.Decimal
	err := tx.Table("payment").
		Where("serviceUsed_id = ? AND payment_for IN ?", serviceUsedID, netPaidPaymentFors).
		Select("COALESCE(SUM(amount), 0)").Scan(&paid).Error
	return paid, err
}

// isDeadlock — lỗi MySQL 1213 (bọc %w vẫn nhận ra).
func isDeadlock(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == mysqlDeadlock
}

// isLockWaitTimeout — lỗi MySQL 1205 (bọc %w vẫn nhận ra).
func isLockWaitTimeout(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == mysqlLockWaitTimeout
}

// mapLockWaitTimeout — 1205 ⇒ errs.ErrPaymentBusyRetry; lỗi khác giữ nguyên.
func mapLockWaitTimeout(err error) error {
	if isLockWaitTimeout(err) {
		return errs.ErrPaymentBusyRetry
	}
	return err
}

// inTransaction — s.DB đang là một transaction (vd DeleteCustomerWithPayments dựng PaymentService{DB: tx}).
func inTransaction(db *gorm.DB) bool {
	_, ok := db.Statement.ConnPool.(gorm.TxCommitter)
	return ok
}

// txMoney chạy fn trong transaction; dính deadlock 1213 thì chạy lại (tối đa paymentTxMaxAttempts lần,
// có lùi ngẫu nhiên), hết lượt ⇒ errs.ErrPaymentBusyRetry (422, người dùng bấm lại). Lock-wait timeout
// 1205 ⇒ ErrPaymentBusyRetry luôn, không chạy lại.
// fn PHẢI chạy lại được: tự đặt lại mọi biến nó sửa ở đầu mỗi lượt.
// s.DB đã là transaction (gọi lồng) ⇒ KHÔNG retry: 1213 đã rollback cả tx ngoài, chạy lại savepoint là
// ghi vào kết nối đã mất transaction — trả lỗi để tx ngoài rollback.
func (s *PaymentService) txMoney(fn func(tx *gorm.DB) error) error {
	if inTransaction(s.DB) {
		return mapLockWaitTimeout(s.DB.Transaction(fn))
	}
	for attempt := 1; ; attempt++ {
		err := s.DB.Transaction(fn)
		if err == nil || !isDeadlock(err) {
			return mapLockWaitTimeout(err)
		}
		if attempt >= paymentTxMaxAttempts {
			return errs.ErrPaymentBusyRetry
		}
		time.Sleep(time.Duration(attempt*20+rand.IntN(30)) * time.Millisecond)
	}
}
