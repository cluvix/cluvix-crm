//go:build integration

package service

// Lưới ĐỒNG THỜI cho luồng tiền (lens-money M-1..M-6, 2026-10-18).
//
// Bộ test chuẩn chạy trên testDB = 1 kết nối + foreign_key_checks=0 ⇒ mọi request tự xếp hàng trên
// một kết nối, KHÔNG BAO GIỜ tái hiện được lost-update / hoàn 2 lần (che M-1/M-2/M-3). File này dựng
// pool RIÊNG giống production: 10 kết nối, foreign_key_checks=1, rồi bắn N request song song cùng lúc
// (barrier). Fixture vẫn dựng qua testDB (FK tắt) — chỉ phần nghiệp vụ chạy trên pool prod-like.

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	crmPaymentDao "erpCluvix/internal/modules/webapp/crm/payment/dao"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/optlock"
	"erpCluvix/pkg/tenant"
)

const concurrentPoolSize = 10

// prodLikePool — pool 10 kết nối, FK bật trên MỌI kết nối (tham số DSN = biến hệ thống mỗi phiên).
func prodLikePool(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	pool, err := gorm.Open(mysql.Open(dsn+sep+"foreign_key_checks=1"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	require.NoError(t, optlock.Register(pool))
	sqlDB, err := pool.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(concurrentPoolSize)
	sqlDB.SetMaxIdleConns(concurrentPoolSize)
	t.Cleanup(func() { _ = sqlDB.Close() })

	// Đối chứng: pool thật sự bật FK (nếu không, test "giống production" chỉ là tên gọi).
	var fk int
	require.NoError(t, pool.Raw("SELECT @@SESSION.foreign_key_checks").Scan(&fk).Error)
	require.Equal(t, 1, fk, "pool prod-like phải bật foreign_key_checks")
	return pool
}

// moneyFx — tenant + user + config_code payment + payment_method (FK của payment.method) + pool.
type moneyFx struct {
	*logFx
	pool *gorm.DB
	svc  *PaymentService
	cust int
	now  int64
}

func newMoneyFx(t *testing.T) *moneyFx {
	t.Helper()
	f := newLogFx(t)
	pool := prodLikePool(t)
	for id, name := range map[int]string{1: "Chuyển khoản", 2: "Máy POS", 3: "Chuyển cọc", 4: "Tiền mặt"} {
		require.NoError(t, testDB.Exec("INSERT IGNORE INTO payment_method (id, name) VALUES (?, ?)", id, name).Error)
	}
	ts := time.Now().Unix()
	cc := testhelpers.WithTenantCols(f.tn, map[string]any{
		"name": "Payment (concurrency tmp)", "parent_table": "payment", "infix_code": "TT",
		"serial_length": 4, "include_branch_code": 0, "monthly_reset": 1, "created_at": ts, "updated_at": ts,
	})
	require.NoError(t, testDB.Table("config_code").Create(&cc).Error)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM order_installment WHERE order_service_used_id IN (SELECT id FROM order_service_used WHERE customer_id = ?)", f.tn.CustomerID)
		testDB.Exec("DELETE FROM order_revenue_telesale WHERE customer_id = ?", f.tn.CustomerID)
		testDB.Exec("UPDATE payment SET deposit_id = NULL WHERE customer_id = ?", f.tn.CustomerID)
		testDB.Exec("DELETE FROM config_code_monthly WHERE config_code_id IN (SELECT id FROM config_code WHERE branch_id = ?)", f.tn.BranchID)
		testDB.Exec("DELETE FROM config_code WHERE branch_id = ?", f.tn.BranchID)
	})
	return &moneyFx{logFx: f, pool: pool, svc: NewPaymentService(pool), cust: int(f.tn.CustomerID), now: ts}
}

// services — n dòng dịch vụ cùng 1 đơn, mỗi dòng totalEnd = each; customer.totalRevenue = n×each.
func (x *moneyFx) services(t *testing.T, n int, each int64) (uint, []int) {
	t.Helper()
	planID, osuIDs := x.seedPlan(t, n)
	ids := make([]int, 0, n)
	for _, id := range osuIDs {
		require.NoError(t, testDB.Exec("UPDATE order_service_used SET totalEnd = ?, payment = 0, payment_status = ?, created_day = ? WHERE id = ?",
			each, define.ServicePaymentStatusStart, x.now-86400, id).Error)
		ids = append(ids, int(id))
	}
	require.NoError(t, testDB.Exec("UPDATE customer SET totalRevenue = ?, totalDebt = ? WHERE id = ?",
		int64(n)*each, int64(n)*each, x.cust).Error)
	return planID, ids
}

func (x *moneyFx) tpl(amount int64) *crmPaymentDao.Payment {
	now := x.now
	return &crmPaymentDao.Payment{CustomerId: &x.cust, BranchId: int(x.tn.BranchID), Method: 4,
		Amount: decimal.NewFromInt(amount), PaymentTime: &now, PaymentOfDate: &now}
}

func (x *moneyFx) pay(t *testing.T, osuID int, amount int64) {
	t.Helper()
	_, err := x.svc.CreatePaymentCustomerFromPayment(x.tpl(amount), []int{osuID}, int(x.userID))
	require.NoError(t, err)
}

func (x *moneyFx) deposit(t *testing.T, amount int64) int {
	t.Helper()
	p, err := x.svc.CreateDepositCustomerFromPayment(x.tpl(amount), int(x.userID))
	require.NoError(t, err)
	return int(p.ID)
}

func (x *moneyFx) dec(t *testing.T, sql string, args ...any) decimal.Decimal {
	t.Helper()
	var d decimal.Decimal
	require.NoError(t, testDB.Raw(sql, args...).Scan(&d).Error)
	return d
}

func requireDec(t *testing.T, want int64, got decimal.Decimal, what string) {
	t.Helper()
	require.True(t, got.Equal(decimal.NewFromInt(want)), "%s: muốn %d, có %s", what, want, got)
}

// race chạy n hàm cùng lúc (barrier) và trả lỗi theo thứ tự.
func race(n int, fn func(i int) error) []error {
	out := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			out[i] = fn(i)
		}(i)
	}
	close(start)
	wg.Wait()
	return out
}

func countOK(errsList []error) int {
	n := 0
	for _, e := range errsList {
		if e == nil {
			n++
		}
	}
	return n
}

// requireOnlyExpected — mọi lỗi phải là lỗi nghiệp vụ dự kiến (không lọt deadlock/lỗi SQL thô).
func requireOnlyExpected(t *testing.T, got []error, allowed ...error) {
	t.Helper()
	for i, e := range got {
		if e == nil {
			continue
		}
		ok := errors.Is(e, errs.ErrPaymentBusyRetry)
		for _, a := range allowed {
			if errors.Is(e, a) {
				ok = true
			}
		}
		var le *errs.LocalizedError
		if errors.As(e, &le) && le.Key == "payment_amount_too_large" {
			ok = true
		}
		require.True(t, ok, "request %d: lỗi ngoài dự kiến: %v", i, e)
	}
}

// M-1: cọc 3tr, 3 thu ngân bấm "hoàn cọc 3tr" cùng lúc ⇒ đúng 1 thành công, tổng hoàn ≤ cọc.
func TestPaymentConcurrency_RefundDepositOnce(t *testing.T) {
	x := newMoneyFx(t)
	depID := x.deposit(t, 3_000_000)

	res := race(3, func(int) error {
		p := x.tpl(3_000_000)
		p.DepositId = &depID
		_, err := x.svc.CreateRefundDepositCustomerFromPayment(p, int(x.userID))
		return err
	})
	t.Logf("hoàn cọc 3tr ×3 trên cọc 3tr: %v", res)
	require.Equal(t, 1, countOK(res), "chỉ 1 lần hoàn được thành công: %v", res)
	requireOnlyExpected(t, res, errs.ErrRefundExceedsDeposit)

	requireDec(t, -3_000_000, x.dec(t, "SELECT COALESCE(SUM(amount),0) FROM payment WHERE deposit_id = ? AND payment_for = ?", depID, define.PaymentForRefundDeposit), "tổng phiếu hoàn")
	requireDec(t, 0, x.dec(t, "SELECT deposit_after FROM payment WHERE id = ?", depID), "deposit_after cọc gốc")
	requireDec(t, 0, x.dec(t, "SELECT COALESCE(totalDeposit,0) FROM customer WHERE id = ?", x.cust), "customer.totalDeposit")
	requireDec(t, -3_000_000, x.dec(t, "SELECT COALESCE(refundDeposit,0) FROM customer WHERE id = ?", x.cust), "customer.refundDeposit")
}

// M-2: 6 phiếu thu hợp lệ cho 6 dịch vụ khác nhau của CÙNG một khách, song song ⇒ tổng của khách và
// đơn phải bằng tổng phiếu (bản cũ: totalPayment=1tr, totalDebt=5tr dù đã thu đủ 6tr).
func TestPaymentConcurrency_SixPaymentsCustomerTotals(t *testing.T) {
	x := newMoneyFx(t)
	planID, ids := x.services(t, 6, 1_000_000)

	res := race(6, func(i int) error {
		_, err := x.svc.CreatePaymentCustomerFromPayment(x.tpl(1_000_000), []int{ids[i]}, int(x.userID))
		return err
	})
	t.Logf("6 phiếu 1tr song song: %v", res)
	require.Equal(t, 6, countOK(res), "%v", res)

	sum := x.dec(t, "SELECT COALESCE(SUM(amount),0) FROM payment WHERE customer_id = ? AND payment_for = ?", x.cust, define.PaymentForOrder)
	requireDec(t, 6_000_000, sum, "SUM(payment)")
	requireDec(t, 6_000_000, x.dec(t, "SELECT COALESCE(totalPayment,0) FROM customer WHERE id = ?", x.cust), "customer.totalPayment")
	requireDec(t, 0, x.dec(t, "SELECT COALESCE(totalDebt,0) FROM customer WHERE id = ?", x.cust), "customer.totalDebt")
	requireDec(t, 6_000_000, x.dec(t, "SELECT COALESCE(paymentPlan,0) FROM order_plan WHERE id = ?", planID), "order.paymentPlan")
	requireDec(t, 0, x.dec(t, "SELECT COALESCE(debtsPlan,0) FROM order_plan WHERE id = ?", planID), "order.debtsPlan")
}

// M-3: dịch vụ đã thu 5tr, 3 lần hoàn 5tr song song ⇒ chỉ 1 thành công, không hoàn vượt số đã thu.
func TestPaymentConcurrency_RefundServiceNotExceedPaid(t *testing.T) {
	x := newMoneyFx(t)
	_, ids := x.services(t, 1, 5_000_000)
	x.pay(t, ids[0], 5_000_000)

	res := race(3, func(int) error {
		p := x.tpl(5_000_000)
		p.ServiceUsedId = &ids[0]
		_, err := x.svc.CreateRefundOrderPlanFromPayment(p, int(x.userID))
		return err
	})
	t.Logf("hoàn DV 5tr ×3 (đã thu 5tr): %v", res)
	require.Equal(t, 1, countOK(res), "%v", res)
	requireOnlyExpected(t, res, errs.ErrRefundExceedsPaid)
	requireDec(t, -5_000_000, x.dec(t, "SELECT COALESCE(SUM(amount),0) FROM payment WHERE serviceUsed_id = ? AND payment_for = ?", ids[0], define.PaymentForRefundService), "tổng phiếu hoàn")
	requireDec(t, 5_000_000, x.dec(t, "SELECT COALESCE(refundServiceUsed,0) FROM order_service_used WHERE id = ?", ids[0]), "osu.refundServiceUsed")
}

// M-4: chuyển cọc không vượt nợ còn lại của dịch vụ; và 2 lần chuyển song song trên cùng cọc.
func TestPaymentConcurrency_MoveDepositLimits(t *testing.T) {
	x := newMoneyFx(t)
	planID, ids := x.services(t, 3, 1_000_000)
	_ = planID
	depID := x.deposit(t, 5_000_000)
	move := func(osu int, amount int64) error {
		in := &crmPaymentDao.Payment{Amount: decimal.NewFromInt(amount), BranchId: int(x.tn.BranchID),
			ServiceUsedId: &osu, PaymentOfDate: &x.now}
		_, err := x.svc.MoveDepositToPaymentFromPayment(depID, in, int(x.userID))
		return err
	}

	// Dịch vụ 1tr, chuyển 5tr ⇒ lỗi, không ghi gì.
	err := move(ids[0], 5_000_000)
	var le *errs.LocalizedError
	require.True(t, errors.As(err, &le) && le.Key == "payment_amount_too_large", "chuyển cọc vượt nợ phải lỗi, có: %v", err)
	requireDec(t, 5_000_000, x.dec(t, "SELECT deposit_after FROM payment WHERE id = ?", depID), "cọc còn nguyên")
	requireDec(t, 0, x.dec(t, "SELECT COALESCE(payment,0) FROM order_service_used WHERE id = ?", ids[0]), "osu chưa nhận đồng nào")

	// Không có dòng dịch vụ ⇒ lỗi.
	_, err = x.svc.MoveDepositToPaymentFromPayment(depID, &crmPaymentDao.Payment{Amount: decimal.NewFromInt(1), BranchId: int(x.tn.BranchID)}, int(x.userID))
	require.ErrorIs(t, err, errs.ErrMissingServiceUsedID)

	// Còn 5tr cọc: 6 lần chuyển 1tr song song vào CÙNG 1 dịch vụ 1tr ⇒ đúng 1 thành công (nợ hết).
	res := race(6, func(int) error { return move(ids[1], 1_000_000) })
	t.Logf("chuyển cọc 1tr ×6 vào DV 1tr: %v", res)
	require.Equal(t, 1, countOK(res), "%v", res)
	requireOnlyExpected(t, res)
	requireDec(t, 1_000_000, x.dec(t, "SELECT COALESCE(payment,0) FROM order_service_used WHERE id = ?", ids[1]), "osu.payment không vượt totalEnd")
	requireDec(t, 4_000_000, x.dec(t, "SELECT deposit_after FROM payment WHERE id = ?", depID), "cọc còn 4tr")
	requireDec(t, 4_000_000, x.dec(t, "SELECT COALESCE(totalDeposit,0) FROM customer WHERE id = ?", x.cust), "customer.totalDeposit")
	requireDec(t, 2_000_000, x.dec(t, "SELECT COALESCE(totalDebt,0) FROM customer WHERE id = ?", x.cust), "customer.totalDebt")
}

// M-1 (chuyển cọc): cọc 3tr, 3 dịch vụ 3tr, chuyển 3tr vào cả 3 cùng lúc ⇒ chỉ 1 lần lọt.
func TestPaymentConcurrency_MoveDepositOnce(t *testing.T) {
	x := newMoneyFx(t)
	_, ids := x.services(t, 3, 3_000_000)
	depID := x.deposit(t, 3_000_000)
	res := race(3, func(i int) error {
		osu := ids[i]
		in := &crmPaymentDao.Payment{Amount: decimal.NewFromInt(3_000_000), BranchId: int(x.tn.BranchID),
			ServiceUsedId: &osu, PaymentOfDate: &x.now}
		_, err := x.svc.MoveDepositToPaymentFromPayment(depID, in, int(x.userID))
		return err
	})
	t.Logf("chuyển cọc 3tr ×3 (3 DV) từ cọc 3tr: %v", res)
	require.Equal(t, 1, countOK(res), "%v", res)
	requireOnlyExpected(t, res, errs.ErrTransferExceedsDeposit)
	requireDec(t, 3_000_000, x.dec(t, "SELECT COALESCE(SUM(amount),0) FROM payment WHERE deposit_id = ? AND payment_for = ?", depID, define.PaymentForMoveDeposit), "tổng chuyển cọc")
	requireDec(t, 0, x.dec(t, "SELECT deposit_after FROM payment WHERE id = ?", depID), "deposit_after")
}

// M-6: mỗi endpoint xoá chỉ xoá đúng loại phiếu của nó và chỉ trong phạm vi tenant.
func TestPaymentDelete_WrongKindOrScopeRejected(t *testing.T) {
	x := newMoneyFx(t)
	_, ids := x.services(t, 1, 1_000_000)
	x.pay(t, ids[0], 1_000_000)
	var payID uint
	require.NoError(t, testDB.Raw("SELECT id FROM payment WHERE serviceUsed_id = ? AND payment_for = ?", ids[0], define.PaymentForOrder).Scan(&payID).Error)
	require.NotZero(t, payID)

	for name, tc := range map[string]struct {
		del  func(*gorm.DB, uint, int) error
		want error
	}{
		"hoàn cọc": {x.svc.DeleteRefundCustomer, errs.ErrDepositReturnNotFound},
		"hoàn DV":  {x.svc.DeleteRefundOrderPlan, errs.ErrRefundNotFound},
		"cọc":      {x.svc.DeleteDeposit, errs.ErrDepositNotFound},
	} {
		require.ErrorIs(t, tc.del(x.pool, payID, int(x.userID)), tc.want, name)
	}

	// Đúng endpoint nhưng NGOÀI phạm vi chi nhánh của phiên ⇒ không thấy phiếu.
	otherBranch := testhelpers.CreateBranch(t, testDB)
	outScope := tenant.ScopedDBNullBranch(testhelpers.TenantCtx(otherBranch, x.userID), x.pool, "payment")
	require.ErrorIs(t, x.svc.DeletePayment(outScope, payID, int(x.userID)), errs.ErrPaymentNotFound)

	requireDec(t, 1, x.dec(t, "SELECT COUNT(*) FROM payment WHERE id = ?", payID), "phiếu thanh toán còn nguyên")
	requireDec(t, 0, x.dec(t, "SELECT COALESCE(totalDebt,0) FROM customer WHERE id = ?", x.cust), "nợ khách không đổi")

	// Đúng loại + trong phạm vi ⇒ xoá được.
	inScope := tenant.ScopedDBNullBranch(testhelpers.TenantCtx(x.tn.BranchID, x.userID), x.pool, "payment")
	require.NoError(t, x.svc.DeletePayment(inScope, payID, int(x.userID)))
	requireDec(t, 1_000_000, x.dec(t, "SELECT COALESCE(totalDebt,0) FROM customer WHERE id = ?", x.cust), "xoá đúng ⇒ nợ quay lại")
}

// M-5: dịch vụ 5.4tr, thu 1tr, hoàn 500k (huỷ bớt) ⇒ nợ 4.4tr KHÔNG đổi và THU TIẾP được; thu vượt nợ vẫn bị chặn.
func TestPaymentMoney_PayAfterPartialRefund(t *testing.T) {
	x := newMoneyFx(t)
	_, ids := x.services(t, 1, 5_400_000)
	x.pay(t, ids[0], 1_000_000)
	r := x.tpl(500_000)
	r.ServiceUsedId = &ids[0]
	_, err := x.svc.CreateRefundOrderPlanFromPayment(r, int(x.userID))
	require.NoError(t, err)
	requireDec(t, 4_400_000, x.dec(t, "SELECT COALESCE(totalDebt,0) FROM customer WHERE id = ?", x.cust), "hoàn không đổi nợ")

	// Thu vượt nợ 4.4tr ⇒ chặn.
	_, err = x.svc.CreatePaymentCustomerFromPayment(x.tpl(4_400_001), []int{ids[0]}, int(x.userID))
	var le *errs.LocalizedError
	require.True(t, errors.As(err, &le) && le.Key == "payment_amount_too_large", "thu vượt nợ phải lỗi, có: %v", err)

	// Thu đúng phần nợ còn lại ⇒ OK, hết nợ.
	x.pay(t, ids[0], 4_400_000)
	requireDec(t, 0, x.dec(t, "SELECT COALESCE(totalDebt,0) FROM customer WHERE id = ?", x.cust), "thu nốt ⇒ hết nợ")
	requireDec(t, 4_900_000, x.dec(t, "SELECT COALESCE(totalPayment,0) FROM customer WHERE id = ?", x.cust), "đã thu ròng = 5.4tr − 0.5tr")
	requireDec(t, 4_900_000, x.dec(t, "SELECT COALESCE(totalRevenue,0) FROM customer WHERE id = ?", x.cust), "doanh thu = 5.4tr − 0.5tr")
	var st int
	require.NoError(t, testDB.Raw("SELECT payment_status FROM order_service_used WHERE id = ?", ids[0]).Scan(&st).Error)
	require.Equal(t, define.ServicePaymentStatusDone, st)
}

// M-8: sửa số tiền phiếu thanh toán không được vượt nợ còn lại của dịch vụ.
func TestPaymentMoney_UpdateCannotExceedDebt(t *testing.T) {
	x := newMoneyFx(t)
	_, ids := x.services(t, 1, 1_000_000)
	x.pay(t, ids[0], 400_000)
	var payID uint
	require.NoError(t, testDB.Raw("SELECT id FROM payment WHERE serviceUsed_id = ?", ids[0]).Scan(&payID).Error)

	_, err := x.svc.UpdatePaymentCustomer(payID, map[string]interface{}{"amount": 3_000_000}, int(x.userID))
	var le *errs.LocalizedError
	require.True(t, errors.As(err, &le) && le.Key == "payment_amount_too_large", "sửa vượt nợ phải lỗi, có: %v", err)
	_, err = x.svc.UpdatePaymentCustomer(payID, map[string]interface{}{"amount": 1_000_000}, int(x.userID))
	require.NoError(t, err, "sửa đúng bằng totalEnd phải được")
	requireDec(t, 0, x.dec(t, "SELECT COALESCE(totalDebt,0) FROM customer WHERE id = ?", x.cust), "nợ sau khi sửa")
}
