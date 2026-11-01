//go:build integration

package service

// Lưới TIỀN cho calculatorPaymentServiceUsed / calculatorOrderPlan / updateCustomer qua API
// service thật (crm-standalone story-09, /bmad-review P2): gỡ kho/sổ cái khỏi payment không được làm
// lệch payment / refundServiceUsed / payment_status của dòng dịch vụ, paymentPlan/debtsPlan của đơn,
// totalPayment/totalDebt của khách — kể cả khi SỬA số tiền một phiếu (UpdatePaymentCustomer).

import (
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	crmPaymentDao "erpCluvix/internal/modules/webapp/crm/payment/dao"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

func TestPaymentMoney_StatusAndTotalsRecalc(t *testing.T) {
	f := newLogFx(t)
	const total = 1_000_000
	planID, osuIDs := f.seedPlan(t, 1)
	osuID := int(osuIDs[0])
	require.NoError(t, testDB.Exec("UPDATE order_service_used SET totalEnd = ?, payment = 0, payment_status = ? WHERE id = ?",
		total, define.ServicePaymentStatusStart, osuID).Error)
	require.NoError(t, testDB.Exec("UPDATE customer SET totalRevenue = ? WHERE id = ?", total, f.tn.CustomerID).Error)

	// codegen payment cần config_code khớp chi nhánh (FreshTenant không seed sẵn).
	ts := time.Now().Unix()
	cc09 := testhelpers.WithTenantCols(f.tn, map[string]any{
		"name":         "Payment (s09 tmp)",
		"parent_table": "payment", "infix_code": "TT", "serial_length": 4, "include_branch_code": 0,
		"monthly_reset": 1, "created_at": ts, "updated_at": ts,
	})
	require.NoError(t, testDB.Table("config_code").Create(&cc09).Error)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM config_code_monthly WHERE config_code_id IN (SELECT id FROM config_code WHERE branch_id = ?)", f.tn.BranchID)
		testDB.Exec("DELETE FROM config_code WHERE branch_id = ?", f.tn.BranchID)
	})

	svc := NewPaymentService(testDB)
	cust := int(f.tn.CustomerID)
	now := time.Now().Unix()
	pay := func(amount int64) {
		t.Helper()
		tpl := &crmPaymentDao.Payment{CustomerId: &cust, BranchId: int(f.tn.BranchID), Method: 1,
			Amount: decimal.NewFromInt(amount), PaymentTime: &now, PaymentOfDate: &now}
		_, err := svc.CreatePaymentCustomerFromPayment(tpl, []int{osuID}, int(f.userID))
		require.NoError(t, err)
	}
	type snap struct {
		Payment, Refund, PaymentPlan, DebtsPlan, TotalPayment, TotalDebt decimal.Decimal
		Status                                                           int
	}
	read := func() snap {
		t.Helper()
		var s snap
		require.NoError(t, testDB.Raw("SELECT COALESCE(payment,0), COALESCE(refundServiceUsed,0), COALESCE(payment_status,0) FROM order_service_used WHERE id = ?", osuID).
			Row().Scan(&s.Payment, &s.Refund, &s.Status))
		require.NoError(t, testDB.Raw("SELECT COALESCE(paymentPlan,0), COALESCE(debtsPlan,0) FROM order_plan WHERE id = ?", planID).
			Row().Scan(&s.PaymentPlan, &s.DebtsPlan))
		require.NoError(t, testDB.Raw("SELECT COALESCE(totalPayment,0), COALESCE(totalDebt,0) FROM customer WHERE id = ?", f.tn.CustomerID).
			Row().Scan(&s.TotalPayment, &s.TotalDebt))
		return s
	}
	d := decimal.NewFromInt
	eq := func(want int64, got decimal.Decimal, what string) {
		t.Helper()
		require.True(t, got.Equal(d(want)), "%s: muốn %d, có %s", what, want, got)
	}
	check := func(step string, payment, refund, plan, debts, custPay, custDebt int64, status int) {
		t.Helper()
		s := read()
		eq(payment, s.Payment, step+" osu.payment")
		eq(refund, s.Refund, step+" osu.refundServiceUsed")
		require.Equal(t, status, s.Status, step+" osu.payment_status")
		eq(plan, s.PaymentPlan, step+" order.paymentPlan")
		eq(debts, s.DebtsPlan, step+" order.debtsPlan")
		eq(custPay, s.TotalPayment, step+" customer.totalPayment")
		eq(custDebt, s.TotalDebt, step+" customer.totalDebt")
	}

	// 1. Trả thiếu ⇒ Partly.
	pay(400_000)
	check("trả thiếu", 400_000, 0, 400_000, 600_000, 400_000, 600_000, define.ServicePaymentStatusPartly)

	// 2. Trả nốt đúng bằng totalEnd ⇒ Done (biên = : GreaterThanOrEqual).
	pay(600_000)
	check("trả đủ", total, 0, total, 0, total, 0, define.ServicePaymentStatusDone)

	// 3. Sửa phiếu 600k → 500k ⇒ tổng dòng/đơn/khách tính lại, tụt về Partly.
	var secondID uint
	require.NoError(t, testDB.Raw("SELECT id FROM payment WHERE serviceUsed_id = ? AND amount = ? ORDER BY id DESC LIMIT 1", osuID, 600_000).Scan(&secondID).Error)
	require.NotZero(t, secondID)
	_, err := svc.UpdatePaymentCustomer(secondID, map[string]interface{}{"amount": 500_000}, int(f.userID))
	require.NoError(t, err)
	check("sửa 600k→500k", 900_000, 0, 900_000, 100_000, 900_000, 100_000, define.ServicePaymentStatusPartly)

	// 4. Sửa lại 500k → 600k ⇒ Done.
	_, err = svc.UpdatePaymentCustomer(secondID, map[string]interface{}{"amount": 600_000}, int(f.userID))
	require.NoError(t, err)
	check("sửa 500k→600k", total, 0, total, 0, total, 0, define.ServicePaymentStatusDone)

	// 5. Hoàn 200k ⇒ refundServiceUsed DƯƠNG 200k; payment dòng + payment_status KHÔNG đổi (refund không
	// trừ vào paymentTotal — working-on-crm §payment_status); khách: totalPayment trừ phần hoàn.
	osu := osuID
	_, err = svc.CreateRefundOrderPlanFromPayment(&crmPaymentDao.Payment{CustomerId: &cust, ServiceUsedId: &osu,
		BranchId: int(f.tn.BranchID), Method: 1, Amount: d(200_000), PaymentTime: &now, PaymentOfDate: &now}, int(f.userID))
	require.NoError(t, err)
	var refundAmt decimal.Decimal
	require.NoError(t, testDB.Raw("SELECT amount FROM payment WHERE serviceUsed_id = ? AND payment_for = ?", osuID, define.PaymentForRefundService).Scan(&refundAmt).Error)
	eq(-200_000, refundAmt, "phiếu hoàn lưu số âm")
	s := read()
	eq(total, s.Payment, "hoàn: osu.payment")
	eq(200_000, s.Refund, "hoàn: osu.refundServiceUsed")
	require.Equal(t, define.ServicePaymentStatusDone, s.Status, "hoàn: osu.payment_status")
	eq(800_000, s.TotalPayment, "hoàn: customer.totalPayment")
	// D20a (story-56): hoàn giảm CẢ doanh thu lẫn đã thu ⇒ nhánh RefundService tính lại totalRevenue
	// (1tr − 200k = 800k) + totalDebt (800k − 800k = 0) + debtsPlan đơn (800k − 800k = 0).
	// Trước story-56: totalDebt giữ 0 vì KHÔNG tính lại (Yii2), totalRevenue giữ 1tr, debtsPlan = 200k.
	eq(0, s.TotalDebt, "hoàn: customer.totalDebt")
	eq(800_000, s.PaymentPlan, "hoàn: order.paymentPlan")
	eq(0, s.DebtsPlan, "hoàn: order.debtsPlan")
	var custRevenue decimal.Decimal
	require.NoError(t, testDB.Raw("SELECT COALESCE(totalRevenue,0) FROM customer WHERE id = ?", f.tn.CustomerID).Scan(&custRevenue).Error)
	eq(800_000, custRevenue, "hoàn: customer.totalRevenue")
	var custRefund decimal.Decimal
	require.NoError(t, testDB.Raw("SELECT COALESCE(refundService,0) FROM customer WHERE id = ?", f.tn.CustomerID).Scan(&custRefund).Error)
	eq(-200_000, custRefund, "hoàn: customer.refundService")
}

// D1 (user chốt 2026-10-17): hoàn tiền = huỷ bớt dịch vụ ⇒ doanh thu giảm đúng số hoàn, công nợ KHÔNG đổi
// (nợ = doanh thu − đã thu ròng), cả trên dòng đã Done lẫn dòng chưa Done.
func TestPaymentMoney_RefundKeepsDebtLowersRevenue(t *testing.T) {
	revDebt := func(t *testing.T, x *refundDebtFx) (decimal.Decimal, decimal.Decimal) {
		t.Helper()
		var rev, debt decimal.Decimal
		require.NoError(t, testDB.Raw("SELECT COALESCE(totalRevenue,0), COALESCE(totalDebt,0) FROM customer WHERE id = ?", x.cust).Row().Scan(&rev, &debt))
		return rev, debt
	}
	for _, tc := range []struct {
		name    string
		paid    int64
		status  int
		wantDbt int64
	}{
		{"dòng Done", 1_000_000, define.ServicePaymentStatusDone, 0},
		{"dòng chưa Done", 400_000, define.ServicePaymentStatusPartly, 600_000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := newRefundDebtFx(t, 1_000_000)
			x.pay(t, tc.paid)
			var status int
			require.NoError(t, testDB.Raw("SELECT COALESCE(payment_status,0) FROM order_service_used WHERE id = ?", x.osuID).Row().Scan(&status))
			require.Equal(t, tc.status, status, "payment_status trước khi hoàn")
			rev0, debt0 := revDebt(t, x)
			require.True(t, debt0.Equal(decimal.NewFromInt(tc.wantDbt)), "nợ trước hoàn: muốn %d, có %s", tc.wantDbt, debt0)

			x.refund(t, 200_000)
			rev1, debt1 := revDebt(t, x)
			require.True(t, debt1.Equal(debt0), "hoàn không đổi totalDebt: trước %s, sau %s", debt0, debt1)
			require.True(t, rev0.Sub(rev1).Equal(decimal.NewFromInt(200_000)), "totalRevenue giảm đúng 200k: trước %s, sau %s", rev0, rev1)
		})
	}
}

// story-10 (crm-standalone) — xoá 1 phiếu thu đơn lẻ qua DeletePayment → deletePaymentWith →
// afterDeleteAll: payment / payment_status của dòng dịch vụ, paymentPlan/debtsPlan của đơn,
// totalPayment/totalDebt của khách phải tính lại đúng sau mỗi lần xoá (defer từ QA story-09).
// Gỡ calculatorBrokerCommissionOnDelete khỏi afterDeleteAll không được làm lệch bất kỳ số nào.
func TestPaymentMoney_DeleteSinglePaymentRecalc(t *testing.T) {
	f := newLogFx(t)
	const total = 1_000_000
	planID, osuIDs := f.seedPlan(t, 1)
	osuID := int(osuIDs[0])
	require.NoError(t, testDB.Exec("UPDATE order_service_used SET totalEnd = ?, payment = 0, payment_status = ? WHERE id = ?",
		total, define.ServicePaymentStatusStart, osuID).Error)
	require.NoError(t, testDB.Exec("UPDATE customer SET totalRevenue = ? WHERE id = ?", total, f.tn.CustomerID).Error)

	ts := time.Now().Unix()
	cc10 := testhelpers.WithTenantCols(f.tn, map[string]any{
		"name":         "Payment (s10 tmp)",
		"parent_table": "payment", "infix_code": "TT", "serial_length": 4, "include_branch_code": 0,
		"monthly_reset": 1, "created_at": ts, "updated_at": ts,
	})
	require.NoError(t, testDB.Table("config_code").Create(&cc10).Error)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM config_code_monthly WHERE config_code_id IN (SELECT id FROM config_code WHERE branch_id = ?)", f.tn.BranchID)
		testDB.Exec("DELETE FROM config_code WHERE branch_id = ?", f.tn.BranchID)
	})

	svc := NewPaymentService(testDB)
	cust := int(f.tn.CustomerID)
	now := time.Now().Unix()
	pay := func(amount int64) uint {
		t.Helper()
		tpl := &crmPaymentDao.Payment{CustomerId: &cust, BranchId: int(f.tn.BranchID), Method: 1,
			Amount: decimal.NewFromInt(amount), PaymentTime: &now, PaymentOfDate: &now}
		_, err := svc.CreatePaymentCustomerFromPayment(tpl, []int{osuID}, int(f.userID))
		require.NoError(t, err)
		var id uint
		require.NoError(t, testDB.Raw("SELECT id FROM payment WHERE serviceUsed_id = ? AND amount = ? ORDER BY id DESC LIMIT 1", osuID, amount).Scan(&id).Error)
		require.NotZero(t, id)
		return id
	}
	d := decimal.NewFromInt
	check := func(step string, payment, plan, debts, custPay, custDebt int64, status int) {
		t.Helper()
		var osuPay, pp, dp, cp, cd decimal.Decimal
		var st int
		require.NoError(t, testDB.Raw("SELECT COALESCE(payment,0), COALESCE(payment_status,0) FROM order_service_used WHERE id = ?", osuID).Row().Scan(&osuPay, &st))
		require.NoError(t, testDB.Raw("SELECT COALESCE(paymentPlan,0), COALESCE(debtsPlan,0) FROM order_plan WHERE id = ?", planID).Row().Scan(&pp, &dp))
		require.NoError(t, testDB.Raw("SELECT COALESCE(totalPayment,0), COALESCE(totalDebt,0) FROM customer WHERE id = ?", f.tn.CustomerID).Row().Scan(&cp, &cd))
		require.True(t, osuPay.Equal(d(payment)), "%s osu.payment: muốn %d, có %s", step, payment, osuPay)
		require.Equal(t, status, st, step+" osu.payment_status")
		require.True(t, pp.Equal(d(plan)), "%s order.paymentPlan: muốn %d, có %s", step, plan, pp)
		require.True(t, dp.Equal(d(debts)), "%s order.debtsPlan: muốn %d, có %s", step, debts, dp)
		require.True(t, cp.Equal(d(custPay)), "%s customer.totalPayment: muốn %d, có %s", step, custPay, cp)
		require.True(t, cd.Equal(d(custDebt)), "%s customer.totalDebt: muốn %d, có %s", step, custDebt, cd)
	}

	first := pay(400_000)
	second := pay(600_000)
	check("trả đủ 2 phiếu", total, total, 0, total, 0, define.ServicePaymentStatusDone)

	// Xoá phiếu 600k ⇒ tụt về Partly, còn nợ 600k ở đơn và khách.
	require.NoError(t, svc.DeletePayment(testDB, second, int(f.userID)))
	var n int64
	require.NoError(t, testDB.Raw("SELECT COUNT(*) FROM payment WHERE id = ?", second).Scan(&n).Error)
	require.Zero(t, n, "phiếu đã xoá phải biến mất")
	check("xoá phiếu 600k", 400_000, 400_000, 600_000, 400_000, 600_000, define.ServicePaymentStatusPartly)

	// Xoá nốt phiếu 400k ⇒ về Start, nợ toàn bộ.
	require.NoError(t, svc.DeletePayment(testDB, first, int(f.userID)))
	check("xoá nốt phiếu 400k", 0, 0, total, 0, total, define.ServicePaymentStatusStart)
}

// story-10 (crm-standalone) — đơn KHÔNG mã giảm giá có chiết khấu thường (chiết khấu dịch vụ % và
// VND, giảm giá khác, giảm theo hạng) tạo qua OrderPlanService.Create thật: bỏ bước mã giảm
// giá không được đổi totalEnd đã lưu, rank_discount_amount tính server-side, debtsPlan của đơn,
// totalRevenue/totalDebt của khách.
func TestOrderMoney_RegularDiscountNoCoupon(t *testing.T) {
	f := newLogFx(t)
	ts := time.Now().Unix()
	for _, cc := range []struct{ name, table, infix string }{
		{"Khách hàng", "customer", "KH"}, {"Đơn hàng", "order_plan", "DH"}, {"Dịch vụ sử dụng", "order_service_used", "SV"},
	} {
		ccRow := testhelpers.WithTenantCols(f.tn, map[string]any{
			"name":         cc.name,
			"parent_table": cc.table, "infix_code": cc.infix, "serial_length": 4, "include_branch_code": 0,
			"monthly_reset": 1, "created_at": ts, "updated_at": ts,
		})
		require.NoError(t, testDB.Table("config_code").Create(&ccRow).Error)
	}
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM order_installment WHERE order_service_used_id IN (SELECT id FROM order_service_used WHERE customer_id = ?)", f.tn.CustomerID)
		testDB.Exec("DELETE FROM config_code_monthly WHERE config_code_id IN (SELECT id FROM config_code WHERE branch_id = ?)", f.tn.BranchID)
		testDB.Exec("DELETE FROM config_code WHERE branch_id = ?", f.tn.BranchID)
	})
	seedService := func(name string) uint {
		t.Helper()
		row := testhelpers.WithTenantCols(f.tn, map[string]any{
			"name": name, "code": fmt.Sprintf("S10%d", time.Now().UnixNano()),
			"serviceCategory_id": define.ServiceCategoryService, "status": 1, "created_at": ts, "updated_at": ts,
		})
		delete(row, "branch_id") // service KHÔNG có cột branch_id
		require.NoError(t, testDB.Table("service").Create(&row).Error)
		return testhelpers.LastInsertID(testDB)
	}
	serviceA := seedService("DV A")
	serviceB := seedService("DV B")
	t.Cleanup(func() { testDB.Exec("DELETE FROM service WHERE id IN ?", []uint{serviceA, serviceB}) })

	d := decimal.NewFromInt
	svc := NewOrderPlanService(testDB)
	create := func(rows []map[string]interface{}) uint {
		t.Helper()
		req := &OrderPlanRequest{ServicesData: rows}
		req.CustomerId = int(f.tn.CustomerID)
		req.BranchId = new(int(f.tn.BranchID))
		req.CreatedTime = new(time.Now().Unix())
		require.NoError(t, svc.Create(req, int(f.userID)))
		require.NotZero(t, req.ID)
		return req.ID
	}
	type osuRow struct {
		ServiceID                              uint
		Total, TotalEnd, RankAmount, DiscOther decimal.Decimal
	}
	readOSU := func(planID uint) []osuRow {
		t.Helper()
		var out []osuRow
		rows, err := testDB.Raw(`SELECT service_id, COALESCE(total,0), COALESCE(totalEnd,0),
			COALESCE(rank_discount_amount,0), COALESCE(discount_other,0) FROM order_service_used WHERE order_plan_id = ? ORDER BY id`, planID).Rows()
		require.NoError(t, err)
		defer rows.Close()
		for rows.Next() {
			var r osuRow
			require.NoError(t, rows.Scan(&r.ServiceID, &r.Total, &r.TotalEnd, &r.RankAmount, &r.DiscOther))
			out = append(out, r)
		}
		return out
	}
	eq := func(want int64, got decimal.Decimal, what string) {
		t.Helper()
		require.True(t, got.Equal(d(want)), "%s: muốn %d, có %s", what, want, got)
	}
	planMoney := func(planID uint) (paymentPlan, debtsPlan decimal.Decimal) {
		t.Helper()
		require.NoError(t, testDB.Raw("SELECT COALESCE(paymentPlan,0), COALESCE(debtsPlan,0) FROM order_plan WHERE id = ?", planID).Row().Scan(&paymentPlan, &debtsPlan))
		return
	}
	custMoney := func() (rev, debt decimal.Decimal) {
		t.Helper()
		require.NoError(t, testDB.Raw("SELECT COALESCE(totalRevenue,0), COALESCE(totalDebt,0) FROM customer WHERE id = ?", f.tn.CustomerID).Row().Scan(&rev, &debt))
		return
	}

	// Đơn 2 dòng — dòng A: 2 × 500.000, chiết khấu dịch vụ 10% (100.000), giảm khác 50.000 ⇒ total 900.000,
	// totalEnd 850.000; hạng 10% ⇒ rank_discount_amount = 850.000 × 10% = 85.000 (server tính).
	// Dòng B: 1 × 300.000, chiết khấu dịch vụ 20.000 VND ⇒ total = totalEnd = 280.000.
	plan1 := create([]map[string]interface{}{
		{"service_id": serviceA, "quantity": 2, "unit_price": 500_000, "service_discount": 10,
			"service_discount_type": define.DiscountTypePercent, "discount_other": 50_000, "total": 900_000,
			"totalEnd": 850_000, "rank_discount_percent": 10},
		{"service_id": serviceB, "quantity": 1, "unit_price": 300_000, "service_discount": 20_000,
			"service_discount_type": define.DiscountTypeNormal, "total": 280_000, "totalEnd": 280_000},
	})
	rows := readOSU(plan1)
	require.Len(t, rows, 2)
	eq(900_000, rows[0].Total, "dòng A total")
	eq(850_000, rows[0].TotalEnd, "dòng A totalEnd")
	eq(50_000, rows[0].DiscOther, "dòng A discount_other")
	eq(85_000, rows[0].RankAmount, "dòng A rank_discount_amount")
	eq(280_000, rows[1].TotalEnd, "dòng B totalEnd")
	eq(0, rows[1].RankAmount, "dòng B rank_discount_amount")
	pp, dp := planMoney(plan1)
	eq(0, pp, "đơn 1 paymentPlan")
	eq(1_130_000, dp, "đơn 1 debtsPlan = Σ totalEnd")
	rev, debt := custMoney()
	eq(1_130_000, rev, "khách totalRevenue sau đơn 1")
	eq(1_130_000, debt, "khách totalDebt sau đơn 1")

	// Đơn 1 dòng: 1 × 1.000.000, chiết khấu dịch vụ 100.000 VND ⇒ total = totalEnd = 900.000.
	// (Giảm giá dòng `discount` đã bỏ hẳn — total = đơn giá × số lượng − chiết khấu dịch vụ.)
	plan2 := create([]map[string]interface{}{
		{"service_id": serviceA, "quantity": 1, "unit_price": 1_000_000, "service_discount": 100_000,
			"service_discount_type": define.DiscountTypeNormal, "total": 900_000, "totalEnd": 900_000},
	})
	rows = readOSU(plan2)
	require.Len(t, rows, 1)
	eq(900_000, rows[0].TotalEnd, "đơn 1 dòng totalEnd")
	_, dp = planMoney(plan2)
	eq(900_000, dp, "đơn 2 debtsPlan")
	rev, debt = custMoney()
	eq(2_030_000, rev, "khách totalRevenue sau đơn 2")
	eq(2_030_000, debt, "khách totalDebt sau đơn 2")

	// QA Pass 1 story-10 — SỬA đơn 1 qua OrderPlanService.Update thật (Update vừa bị sửa: bỏ
	// handleCoupon + đổi chữ ký handleServices): dòng A giảm khác 50.000 → 100.000 ⇒ totalEnd 800.000,
	// hạng 10% → 20% ⇒ rank_discount_amount tính LẠI = 800.000 × 20% = 160.000 (FE không gửi amount);
	// bỏ dòng B khỏi payload ⇒ dòng B bị xoá (mồ côi). debtsPlan đơn 1 = 800.000; khách = 800.000 +
	// 900.000 (đơn 2) = 1.700.000 doanh thu = công nợ (chưa thu đồng nào).
	var osuA uint
	require.NoError(t, testDB.Raw("SELECT id FROM order_service_used WHERE order_plan_id = ? AND service_id = ?", plan1, serviceA).Scan(&osuA).Error)
	require.NotZero(t, osuA)
	var ver int64 // khoá lạc quan D6: updated_at đang lưu
	require.NoError(t, testDB.Raw("SELECT updated_at FROM order_plan WHERE id = ?", plan1).Scan(&ver).Error)
	upd := &OrderPlanRequest{ServicesData: []map[string]interface{}{
		{"id": osuA, "service_id": serviceA, "quantity": 2, "unit_price": 500_000, "service_discount": 10,
			"service_discount_type": define.DiscountTypePercent, "discount_other": 100_000, "total": 900_000,
			"totalEnd": 800_000, "rank_discount_percent": 20},
	}}
	upd.ID = plan1
	upd.UpdatedAt = ver
	upd.CustomerId = int(f.tn.CustomerID)
	upd.BranchId = new(int(f.tn.BranchID))
	require.NoError(t, svc.Update(upd, int(f.userID), testDB))
	rows = readOSU(plan1)
	require.Len(t, rows, 1, "dòng B bỏ khỏi payload phải bị xoá")
	eq(800_000, rows[0].TotalEnd, "sửa: dòng A totalEnd")
	eq(100_000, rows[0].DiscOther, "sửa: dòng A discount_other")
	eq(160_000, rows[0].RankAmount, "sửa: dòng A rank_discount_amount tính lại")
	pp, dp = planMoney(plan1)
	eq(0, pp, "sửa: đơn 1 paymentPlan")
	eq(800_000, dp, "sửa: đơn 1 debtsPlan")
	rev, debt = custMoney()
	eq(1_700_000, rev, "sửa: khách totalRevenue")
	eq(1_700_000, debt, "sửa: khách totalDebt")
}
