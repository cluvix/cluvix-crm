package service

import (
	"time"

	orderPlanDao "erpCluvix/internal/modules/webapp/crm/order_plan/dao"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
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

	if o.VatRate != nil {
		if _, ok := define.VatRatePercent[*o.VatRate]; !ok {
			v.Add("Mức VAT không hợp lệ")
		}
	}

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

// checkOrderVatRate8 — mức 8% (giảm thuế tạm thời) chỉ cho đơn có ngày (giờ thương hiệu) tới hết
// define.VatRate8LastDay. prev != nil (sửa đơn) ⇒ chỉ kiểm khi mức VAT hoặc ngày đơn thực sự đổi: đơn đã lưu 8%
// trong hạn giữ nguyên được khi sửa phần khác sau mốc.
func checkOrderVatRate8(loc *time.Location, o, prev *orderPlanDao.OrderPlan) error {
	if o.VatRate == nil || *o.VatRate != define.VatRate8 || o.CreatedTime == nil {
		return nil
	}
	if prev != nil && prev.VatRate != nil && *prev.VatRate == define.VatRate8 &&
		prev.CreatedTime != nil && *prev.CreatedTime == *o.CreatedTime {
		return nil
	}
	day := time.Unix(*o.CreatedTime, 0).In(loc).Format(time.DateOnly)
	if day > define.VatRate8LastDay {
		return errs.NewLocalizedError("vat_rate_8_expired", vatDayLabel(define.VatRate8LastDay))
	}
	return nil
}

// vatDayLabel đổi yyyy-mm-dd ⇒ dd/mm/yyyy cho thông báo lỗi.
func vatDayLabel(day string) string {
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return day
	}
	return t.Format("02/01/2006")
}
