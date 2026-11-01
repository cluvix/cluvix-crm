//go:build integration

package service

// Phiếu DÙNG cọc (hoàn cọc, trả dịch vụ bằng cọc) chỉ được dùng cọc của chính khách đó,
// dòng dịch vụ được trả cũng phải của khách đó (validateDepositUse). (Xoá khách kèm phiếu đã bỏ — khách có
// phiếu thì không xoá được, xem customer_delete_integration_test.go.)

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	crmPaymentDao "erpCluvix/internal/modules/webapp/crm/payment/dao"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
)

func seedDeposit(t *testing.T, tn *testhelpers.TestTenant, amount int64) uint {
	t.Helper()
	id, err := testhelpers.CreatePayment(testDB, tn, decimal.NewFromInt(amount), define.PaymentForDeposit)
	require.NoError(t, err)
	require.NoError(t, testDB.Exec("UPDATE payment SET deposit_after = amount WHERE id = ?", id).Error)
	return id
}

func TestValidateDepositUse_RejectsForeignDeposit(t *testing.T) {
	f := newLogFx(t)
	a := f.tn
	b := testhelpers.FreshTenant(t, testDB)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM payment WHERE customer_id IN (?, ?)", a.CustomerID, b.CustomerID)
		testDB.Exec("DELETE FROM order_service_used WHERE customer_id = ?", b.CustomerID)
		testDB.Exec("DELETE FROM order_plan WHERE customer_id = ?", b.CustomerID)
	})
	depA := seedDeposit(t, a, 1000000)
	orderPay, err := testhelpers.CreatePayment(testDB, a, decimal.NewFromInt(50000), define.PaymentForOrder)
	require.NoError(t, err)
	svc := &PaymentService{DB: testDB}
	cust := func(tn *testhelpers.TestTenant) *int { v := int(tn.CustomerID); return &v }
	dep := func(id uint) *int { v := int(id); return &v }
	refund := func(c *int, depositID *int) *crmPaymentDao.Payment {
		return &crmPaymentDao.Payment{CustomerId: c, DepositId: depositID, Amount: decimal.NewFromInt(100000),
			BranchId: int(a.BranchID), Method: 1}
	}
	countPays := func(c uint) int64 { return countWhere(t, "SELECT COUNT(*) FROM payment WHERE customer_id = ?", c) }
	beforeA, beforeB := countPays(a.CustomerID), countPays(b.CustomerID)

	_, err = svc.CreateRefundDepositCustomerFromPayment(refund(cust(b), dep(depA)), int(f.userID))
	require.ErrorIs(t, err, errs.ErrDepositCustomerMismatch, "hoàn cọc của khách A cho khách B")

	// Khách KHÁC tạo trong cùng tenant fixture (chưa gán chi nhánh) — phép so customer_id phải chặn.
	otherCust := int(testhelpers.CreateScopedCustomer(t, testDB, a, nil))
	_, err = svc.CreateRefundDepositCustomerFromPayment(refund(&otherCust, dep(depA)), int(f.userID))
	require.ErrorIs(t, err, errs.ErrDepositCustomerMismatch, "hoàn cọc của khách A cho khách khác")

	_, err = svc.CreateRefundDepositCustomerFromPayment(refund(cust(a), dep(orderPay)), int(f.userID))
	require.ErrorIs(t, err, errs.ErrDepositNotFound, "deposit_id trỏ tới phiếu KHÔNG phải cọc")

	// Trả dịch vụ của khách B bằng cọc của khách A.
	require.NoError(t, testDB.Exec("INSERT INTO order_plan (code, customer_id) VALUES (?, ?)",
		fmt.Sprintf("DH%d", time.Now().UnixNano()), b.CustomerID).Error)
	planB := int(testhelpers.LastInsertID(testDB))
	require.NoError(t, testDB.Exec("INSERT INTO order_service_used (code, customer_id, order_plan_id, service_id) VALUES (?, ?, ?, 1)",
		fmt.Sprintf("SV%d", time.Now().UnixNano()), b.CustomerID, planB).Error)
	osuB := int(testhelpers.LastInsertID(testDB))
	_, err = svc.MoveDepositToPaymentFromPayment(int(depA), &crmPaymentDao.Payment{
		Amount: decimal.NewFromInt(100000), BranchId: int(a.BranchID), ServiceUsedId: &osuB, ParentId: &planB,
	}, int(f.userID))
	require.ErrorIs(t, err, errs.ErrDepositCustomerMismatch, "trả dịch vụ khách B bằng cọc khách A")

	require.Equal(t, beforeA, countPays(a.CustomerID), "không được tạo phiếu nào khi bị chặn")
	require.Equal(t, beforeB, countPays(b.CustomerID))

	// Đúng khách ⇒ qua guard (không trả lỗi mismatch/not found). codegen payment cần config_code
	// của chi nhánh.
	ts := time.Now().Unix()
	require.NoError(t, testDB.Table("config_code").Create(&map[string]any{
		"branch_id": a.BranchID, "name": "Payment (s37 tmp)",
		"parent_table": "payment", "infix_code": "TT", "serial_length": 4, "include_branch_code": 0,
		"monthly_reset": 1, "created_at": ts, "updated_at": ts,
	}).Error)
	ccID := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM config_code_monthly WHERE config_code_id = ?", ccID)
		testDB.Exec("DELETE FROM config_code WHERE id = ?", ccID)
	})
	created, err := svc.CreateRefundDepositCustomerFromPayment(refund(cust(a), dep(depA)), int(f.userID))
	require.NotErrorIs(t, err, errs.ErrDepositCustomerMismatch)
	require.NotErrorIs(t, err, errs.ErrDepositNotFound)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
}
