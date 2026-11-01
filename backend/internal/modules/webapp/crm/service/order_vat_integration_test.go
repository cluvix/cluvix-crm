//go:build integration

package service

// Lưới VAT khi LƯU ĐƠN (bmad/order-vat S1): vat_amount dòng server tính theo mức VAT đơn, đối chiếu số FE gửi,
// số phải thu = totalEnd + VAT, đổi mức VAT không gửi dòng vẫn tính lại, mức 8% có hạn ngày.
// (payment_status / công nợ theo VAT thuộc S2 — chưa kiểm ở đây.)

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"erpCluvix/pkg/brand"
	"erpCluvix/pkg/define"
)

func (f *orderEditFx) vat(t *testing.T) (rate *string, amount int64) {
	t.Helper()
	require.NoError(t, testDB.Raw("SELECT vat_rate FROM order_plan WHERE id = ?", f.planID).Scan(&rate).Error)
	require.NoError(t, testDB.Raw("SELECT CAST(vat_amount AS SIGNED) FROM order_service_used WHERE id = ?", f.osuID).Scan(&amount).Error)
	return
}

func vatRow(id int, unit int64, extra map[string]interface{}) map[string]interface{} {
	row := map[string]interface{}{"id": id, "quantity": 1, "unit_price": unit, "total": unit, "totalEnd": unit}
	for k, v := range extra {
		row[k] = v
	}
	return row
}

func withVat(rate string) func(r *OrderPlanRequest) {
	return func(r *OrderPlanRequest) { r.VatRate = &rate }
}

// Đổi mức VAT KHÔNG gửi dòng ⇒ VAT từng dòng vẫn được tính lại (và ghi log).
func TestOrderVat_RateChangeWithoutServicesRecalcsRows(t *testing.T) {
	f := newOrderEditFx(t)
	require.NoError(t, f.update(t, nil, withVat("10")))
	rate, amount := f.vat(t)
	require.NotNil(t, rate)
	require.Equal(t, "10", *rate)
	require.EqualValues(t, 100_000, amount, "VAT 10% của totalEnd 1tr")

	var logged int64
	require.NoError(t, testDB.Raw("SELECT COUNT(*) FROM log_order_service_used WHERE order_service_used_id = ? AND JSON_CONTAINS_PATH(changed_fields, 'one', '$.vat_amount')", f.osuID).Scan(&logged).Error)
	require.EqualValues(t, 1, logged, "đổi vat_amount phải có dòng log")

	require.NoError(t, f.update(t, nil, withVat("KCT")))
	_, amount = f.vat(t)
	require.EqualValues(t, 0, amount, "KCT ⇒ VAT 0")
}

// vat_amount FE gửi lệch số server tính > 1đ ⇒ chặn; khớp ⇒ lưu.
func TestOrderVat_SentAmountMustMatch(t *testing.T) {
	f := newOrderEditFx(t)
	err := f.update(t, []map[string]interface{}{vatRow(f.osuID, 1_000_000, map[string]interface{}{"vat_amount": 80_000})}, withVat("10"))
	requireLocalized(t, err, "service_vat_mismatch")

	require.NoError(t, f.update(t, []map[string]interface{}{vatRow(f.osuID, 1_000_000, map[string]interface{}{"vat_amount": 100_000})}, withVat("10")))
	_, amount := f.vat(t)
	require.EqualValues(t, 100_000, amount)
}

// Đã thu 1tr: có VAT 10%, hạ totalEnd còn 950k vẫn được (phải thu 1.045tr ≥ đã thu); 900k (phải thu 990k) ⇒ chặn.
func TestOrderVat_PayableIncludesVatInBelowPaymentGuard(t *testing.T) {
	f := newOrderEditFx(t)
	require.NoError(t, f.update(t, []map[string]interface{}{vatRow(f.osuID, 950_000, nil)}, withVat("10")))
	_, amount := f.vat(t)
	require.EqualValues(t, 95_000, amount)

	err := f.update(t, []map[string]interface{}{vatRow(f.osuID, 900_000, nil)}, withVat("10"))
	requireLocalized(t, err, "total_end_below_payment")

	// Hạ mức VAT làm phải thu < đã thu cũng bị chặn (không gửi totalEnd).
	err = f.update(t, nil, withVat("0"))
	requireLocalized(t, err, "total_end_below_payment")
}

// Mức 8%: ngày đơn sau 31/12/2026 (giờ thương hiệu) ⇒ chặn; mức lạ ⇒ lỗi validate.
func TestOrderVat_Rate8DeadlineAndInvalidRate(t *testing.T) {
	f := newOrderEditFx(t)
	after := time.Date(2027, 1, 5, 10, 0, 0, 0, brand.Location(testDB)).Unix()
	err := f.update(t, nil, func(r *OrderPlanRequest) {
		withVat("8")(r)
		r.CreatedTime = &after
	})
	requireLocalized(t, err, "vat_rate_8_expired")

	require.NoError(t, f.update(t, nil, withVat("8")), "ngày đơn hiện tại (trước mốc) chọn 8% được")
	_, amount := f.vat(t)
	require.EqualValues(t, 80_000, amount)

	require.Error(t, f.update(t, nil, withVat("7")), "mức 7% không hợp lệ")
}

// S2 (D4/D6): đã thu đủ 1tr, bật VAT 10% ⇒ còn nợ 100k (Partly); thu thêm 100k ⇒ Done; thu vượt ⇒ chặn.
// Doanh số khách giữ TRƯỚC thuế (1tr), công nợ khách theo phải thu có VAT.
func TestOrderVat_PaymentStatusAndDebtIncludeVat(t *testing.T) {
	f := newOrderEditFx(t)
	require.NoError(t, f.update(t, nil, withVat("10")))
	require.Equal(t, define.ServicePaymentStatusPartly, f.status(t), "phải thu 1.1tr, đã thu 1tr ⇒ Partly")
	paid, debts := f.planMoney(t)
	requireDec(t, 1_000_000, paid, "order.paymentPlan")
	requireDec(t, 100_000, debts, "order.debtsPlan = phải thu có VAT − đã thu")
	requireDec(t, 100_000, f.dec(t, "SELECT totalDebt FROM customer WHERE id = ?", f.cust), "customer.totalDebt")
	requireDec(t, 1_000_000, f.dec(t, "SELECT totalRevenue FROM customer WHERE id = ?", f.cust), "customer.totalRevenue (trước thuế)")

	_, err := f.svc.CreatePaymentCustomerFromPayment(f.tpl(100_001), []int{f.osuID}, int(f.userID))
	requireLocalized(t, err, "payment_amount_too_large")

	f.pay(t, f.osuID, 100_000)
	require.Equal(t, define.ServicePaymentStatusDone, f.status(t))
	_, debts = f.planMoney(t)
	requireDec(t, 0, debts, "order.debtsPlan sau khi thu đủ VAT")
	requireDec(t, 0, f.dec(t, "SELECT totalDebt FROM customer WHERE id = ?", f.cust), "customer.totalDebt sau khi thu đủ VAT")
}
