//go:build integration

package service

// DeleteCustomer: khách đã có đơn hàng HOẶC phiếu thu/chi (gắn thẳng khách, theo dòng dịch vụ, theo đơn) ⇒ chặn,
// không xoá gì (user chốt 2026-11-01). Khách chưa có ⇒ xoá, chỉ còn 1 dòng log xoá khách.
// Fixture logFx: log_delete_integration_test.go.

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	history "erpCluvix/internal/modules/webapp/history/log/service"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

func TestDeleteCustomer_BlockedWhenHasDeposit(t *testing.T) {
	f := newLogFx(t)
	deposit, err := testhelpers.CreatePayment(testDB, f.tn, decimal.NewFromInt(500000), define.PaymentForDeposit)
	require.NoError(t, err)
	withFKChecks(t)

	requireLocalized(t, DeleteCustomer(testDB, f.tn.CustomerID, int(f.userID)), "customer_delete_has_money")
	require.EqualValues(t, 1, countWhere(t, "SELECT COUNT(*) FROM customer WHERE id = ?", f.tn.CustomerID), "khách phải còn")
	require.EqualValues(t, 1, countWhere(t, "SELECT COUNT(*) FROM payment WHERE id = ?", deposit), "phiếu cọc phải còn")
}

func TestDeleteCustomer_BlockedWhenHasOrder(t *testing.T) {
	f := newLogFx(t)
	planID, _ := f.seedPlan(t, 1) // đơn chưa thu đồng nào vẫn chặn
	withFKChecks(t)

	requireLocalized(t, DeleteCustomer(testDB, f.tn.CustomerID, int(f.userID)), "customer_delete_has_money")
	require.EqualValues(t, 1, countWhere(t, "SELECT COUNT(*) FROM order_plan WHERE id = ?", planID), "đơn phải còn")
}

func TestDeleteCustomer_NoMoneyDeletesAndLogsOnce(t *testing.T) {
	f := newLogFx(t)
	cid := f.tn.CustomerID
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM log_customer WHERE changed_by = ?", f.userID)
	})
	withFKChecks(t)

	require.NoError(t, DeleteCustomer(testDB, cid, int(f.userID)))

	require.Zero(t, countWhere(t, "SELECT COUNT(*) FROM customer WHERE id = ?", cid))
	require.EqualValues(t, 1, countWhere(t, "SELECT COUNT(*) FROM log_customer WHERE changed_by = ? AND action_type = ? AND customer_id IS NULL",
		f.userID, history.ActionDelete), "chỉ còn 1 dòng log xoá khách")
}
