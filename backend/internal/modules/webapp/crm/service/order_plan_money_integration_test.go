//go:build integration

package service

// Lưới TIỀN cho SỬA ĐƠN (OrderPlanService.Update) — lens-money 2026-10-18:
//   - M-9  sửa totalEnd sau khi đã thu ⇒ tính lại payment_status + phân bổ kỳ trả góp
//   - M-10 paymentPlan/debtsPlan KHÔNG nhận từ body
//   - M-11 không đổi khách của đơn đã có dịch vụ/phiếu
//   - M-16 total/totalEnd phải khớp công thức đơn giá × số lượng − chiết khấu
//   - M-17 listRevenues không phải mảng số ⇒ 422

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
)

// orderEditFx — đơn 1 dòng dịch vụ totalEnd 1tr, đã thu đủ 1tr; trả góp 3 kỳ 500k/250k/250k (đều PAID).
type orderEditFx struct {
	*moneyFx
	planID uint
	osuID  int
	osvc   *OrderPlanService
}

func newOrderEditFx(t *testing.T) *orderEditFx {
	t.Helper()
	x := newMoneyFx(t)
	planID, ids := x.services(t, 1, 1_000_000)
	osuID := ids[0]
	require.NoError(t, testDB.Exec("UPDATE order_plan SET created_time = ?, customer_id = ? WHERE id = ?", x.now-86400, x.cust, planID).Error)
	for i, amt := range []int64{500_000, 250_000, 250_000} {
		require.NoError(t, testDB.Exec("INSERT INTO order_installment (order_service_used_id, period_number, amount, paid_amount, status, due_date) VALUES (?, ?, ?, 0, ?, CURDATE())",
			osuID, i, amt, define.InstallmentStatusUnpaid).Error)
	}
	x.pay(t, osuID, 1_000_000)
	f := &orderEditFx{moneyFx: x, planID: planID, osuID: osuID, osvc: NewOrderPlanService(testDB)}
	require.Equal(t, define.ServicePaymentStatusDone, f.status(t), "tiền đề: dòng đã Done")
	return f
}

func (f *orderEditFx) status(t *testing.T) int {
	t.Helper()
	var st int
	require.NoError(t, testDB.Raw("SELECT COALESCE(payment_status,0) FROM order_service_used WHERE id = ?", f.osuID).Scan(&st).Error)
	return st
}

// update gửi sửa đơn với khoá lạc quan đúng; mutate chỉnh request trước khi gửi.
func (f *orderEditFx) update(t *testing.T, services []map[string]interface{}, mutate func(r *OrderPlanRequest)) error {
	t.Helper()
	var ver int64
	require.NoError(t, testDB.Raw("SELECT COALESCE(updated_at,0) FROM order_plan WHERE id = ?", f.planID).Scan(&ver).Error)
	req := &OrderPlanRequest{ServicesData: services}
	req.ID = f.planID
	req.UpdatedAt = ver
	req.CustomerId = f.cust
	req.BranchId = new(int(f.tn.BranchID))
	if mutate != nil {
		mutate(req)
	}
	return f.osvc.Update(req, int(f.userID), testDB)
}

func (f *orderEditFx) planMoney(t *testing.T) (paid, debts decimal.Decimal) {
	t.Helper()
	require.NoError(t, testDB.Raw("SELECT COALESCE(paymentPlan,0), COALESCE(debtsPlan,0) FROM order_plan WHERE id = ?", f.planID).Row().Scan(&paid, &debts))
	return
}

func requireLocalized(t *testing.T, err error, key string) {
	t.Helper()
	var le *errs.LocalizedError
	require.True(t, errors.As(err, &le), "muốn lỗi %s, có %v", key, err)
	require.Equal(t, key, le.Key)
}

// M-9: nâng totalEnd 1tr → 1.5tr sau khi đã thu đủ 1tr ⇒ Partly; kỳ trả góp cũ còn sót bị dọn (trả góp đã gỡ — D12).
func TestOrderMoney_RaiseTotalEndRecalcStatusAndInstallments(t *testing.T) {
	f := newOrderEditFx(t)
	require.NoError(t, f.update(t, []map[string]interface{}{{
		"id": f.osuID, "quantity": 1, "unit_price": 1_500_000, "total": 1_500_000, "totalEnd": 1_500_000,
		"installment_config": nil,
	}}, nil))

	require.Equal(t, define.ServicePaymentStatusPartly, f.status(t), "còn nợ 500k ⇒ Partly, không còn Done")
	var left int64
	require.NoError(t, testDB.Raw("SELECT COUNT(*) FROM order_installment WHERE order_service_used_id = ?", f.osuID).Scan(&left).Error)
	require.EqualValues(t, 0, left, "kỳ trả góp cũ phải được dọn")
	paid, debts := f.planMoney(t)
	requireDec(t, 1_000_000, paid, "order.paymentPlan")
	requireDec(t, 500_000, debts, "order.debtsPlan")
	requireDec(t, 500_000, f.dec(t, "SELECT totalDebt FROM customer WHERE id = ?", f.cust), "customer.totalDebt")
}

// M-10: body gửi paymentPlan = 0 (mobile / request tự soạn) không đổi được công nợ đơn.
func TestOrderMoney_PaymentPlanFromBodyIgnored(t *testing.T) {
	f := newOrderEditFx(t)
	zero := decimal.Zero
	fake := decimal.NewFromInt(9_000_000)
	require.NoError(t, f.update(t, nil, func(r *OrderPlanRequest) {
		r.PaymentPlan = &zero
		r.DebtsPlan = &fake
	}))
	paid, debts := f.planMoney(t)
	requireDec(t, 1_000_000, paid, "order.paymentPlan giữ số đã thu thật")
	requireDec(t, 0, debts, "order.debtsPlan = doanh thu − đã thu")
}

// M-11: đổi khách của đơn đã có dịch vụ/phiếu bị chặn; đơn trống thì đổi được.
func TestOrderMoney_ChangeCustomerBlockedWhenLinked(t *testing.T) {
	f := newOrderEditFx(t)
	other := int(testhelpers.CreateScopedCustomer(t, testDB, f.tn, &f.tn.BranchID))
	err := f.update(t, nil, func(r *OrderPlanRequest) { r.CustomerId = other })
	requireLocalized(t, err, "order_customer_change_blocked")
	var cust int
	require.NoError(t, testDB.Raw("SELECT customer_id FROM order_plan WHERE id = ?", f.planID).Scan(&cust).Error)
	require.Equal(t, f.cust, cust, "đơn vẫn thuộc khách cũ")

	// Đối chứng: đơn KHÔNG có dịch vụ/phiếu ⇒ đổi khách được (guard không chặn bừa).
	empty := f.seed(t, "order_plan", map[string]any{"branch_id": f.tn.BranchID, "created_time": f.now})
	t.Cleanup(func() { testDB.Exec("DELETE FROM order_plan WHERE id = ?", empty) })
	var ver int64
	require.NoError(t, testDB.Raw("SELECT COALESCE(updated_at,0) FROM order_plan WHERE id = ?", empty).Scan(&ver).Error)
	req := &OrderPlanRequest{}
	req.ID = empty
	req.UpdatedAt = ver
	req.CustomerId = other
	require.NoError(t, f.osvc.Update(req, int(f.userID), testDB))
	require.NoError(t, testDB.Raw("SELECT customer_id FROM order_plan WHERE id = ?", empty).Scan(&cust).Error)
	require.Equal(t, other, cust)
}

// M-16: totalEnd lệch công thức bị chặn; dòng cũ lệch công thức nhưng KHÔNG đổi trường tiền vẫn lưu được.
func TestOrderMoney_ServiceTotalMustMatchFormula(t *testing.T) {
	f := newOrderEditFx(t)
	err := f.update(t, []map[string]interface{}{{
		"id": f.osuID, "quantity": 1, "unit_price": 10_000_000, "total": 1_000_000, "totalEnd": 1_000_000,
	}}, nil)
	requireLocalized(t, err, "service_total_mismatch")
	requireDec(t, 1_000_000, f.dec(t, "SELECT totalEnd FROM order_service_used WHERE id = ?", f.osuID), "totalEnd không đổi")

	// Công thức đủ vế: 2 × 600k − 10% (120k) = 1.08tr; − giảm khác 80k ⇒ totalEnd 1tr.
	require.NoError(t, f.update(t, []map[string]interface{}{{
		"id": f.osuID, "quantity": 2, "unit_price": 600_000, "service_discount": 10,
		"service_discount_type": define.DiscountTypePercent, "discount_other": 80_000,
		"total": 1_080_000, "totalEnd": 1_000_000,
	}}, nil))

	// Dòng nhập từ hệ thống cũ: unit_price/total NULL, totalEnd 1tr — FE gửi lại y nguyên ⇒ không chặn.
	legacy := f.seed(t, "order_service_used", map[string]any{"order_plan_id": f.planID, "service_id": 1,
		"branch_id": f.tn.BranchID, "quantity": 1, "totalEnd": 700_000})
	require.NoError(t, f.update(t, []map[string]interface{}{
		{"id": f.osuID, "quantity": 2, "unit_price": 600_000, "service_discount": 10,
			"service_discount_type": define.DiscountTypePercent, "discount_other": 80_000, "total": 1_080_000, "totalEnd": 1_000_000},
		{"id": legacy, "quantity": 1, "unit_price": nil, "total": nil, "totalEnd": 700_000, "note": "chỉ sửa ghi chú"},
	}, nil))
}

// M-17: listRevenues dạng chuỗi bị từ chối (trước đây lọt ⇒ doanh số TVV revenue_id = 0).
func TestOrderMoney_ListRevenuesMustBeIntArray(t *testing.T) {
	f := newOrderEditFx(t)
	err := f.update(t, nil, func(r *OrderPlanRequest) { r.ListRevenues = datatypes.JSON(`["12","13"]`) })
	requireLocalized(t, err, "list_revenues_invalid")
	// Đối chứng: mảng số hợp lệ (user thật) lưu được.
	err = f.update(t, nil, func(r *OrderPlanRequest) {
		r.ListRevenues = datatypes.JSON(`[` + itoa(int(f.userID)) + `]`)
	})
	require.NoError(t, err)
}
