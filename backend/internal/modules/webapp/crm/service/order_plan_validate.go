package service

import (
	orderPlanDao "erpCluvix/internal/modules/webapp/crm/order_plan/dao"
	"erpCluvix/pkg/validation"

	"github.com/shopspring/decimal"
)

// validateOrderPlan — port từ be/common/models/OrderPlan.php::rules()
// Required: customer_id, created_time, branch_id (company_id auto-derive từ customer).
// Number min 0: total, paymentPlan, debtsPlan.
// String: note max 300, code max 255.
func validateOrderPlan(o *orderPlanDao.OrderPlan) error {
	v := validation.New()
	v.Required(o.CustomerId, "Khách hàng")
	v.Required(o.BranchId, "Chi nhánh")
	v.Required(o.CreatedTime, "Ngày nhập đơn hàng")

	checkOrderPlanDecimalNonNeg(v, o.PaymentPlan, "Tổng thanh toán")
	checkOrderPlanDecimalNonNeg(v, o.DebtsPlan, "Công nợ đơn hàng")

	v.MaxLen(o.Note, 300, "Ghi chú")
	v.MaxLen(o.Code, 255, "Mã đơn hàng")

	if v.HasErrors() {
		return v
	}
	return nil
}

func checkOrderPlanDecimalNonNeg(v *validation.Errors, d *decimal.Decimal, label string) {
	if d == nil {
		return
	}
	if d.IsNegative() {
		v.Add(label + " không thể nhỏ hơn 0")
	}
}
