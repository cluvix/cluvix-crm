package service

import (
	crmServiceDao "erpCluvix/internal/modules/webapp/crm/service/dao"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/validation"

	"github.com/shopspring/decimal"
)

// validateOrderServiceUsed — port từ be/common/models/OrderServiceUsed.php::rules()
// Required: customer_id, service_id, unit_price, quantity (company_id auto từ order).
// SCENARIO_HAS_TREATMENT_PLAN → order_plan_id required (luôn áp dụng vì OrderServiceUsed
//
//	luôn thuộc 1 OrderPlan trong flow này).
//
// Cross-field: payment <= totalEnd.
// Number min 0: unit_price, service_discount, discount_other, total,
//
//	discountServiceLock, rank_discount_percent, rank_discount_amount, totalEnd, payment.
//
// quantity min 1.
// Enum: service_discount_type ∈ {1, 2}.
// Lưu ý: quantity > 0 đã được check ở handleServices (errs.ErrQuantityMustBePositive).
// (crm-standalone story-14: rule số lần điều trị theo loại dịch vụ đã bỏ cùng liệu trình — D1.)
func validateOrderServiceUsed(svc *crmServiceDao.OrderServiceUsed) error {
	v := validation.New()
	v.Required(svc.CustomerId, "Khách hàng")
	v.Required(svc.ServiceId, "Dịch vụ")
	v.Required(svc.UnitPrice, "Đơn giá")
	v.Required(svc.OrderPlanId, "Đơn hàng")
	v.Required(svc.Quantity, "Số lượng")

	if svc.Quantity != nil {
		v.Min(*svc.Quantity, 1, "Số lượng")
	}

	checkOSUDecimalNonNeg(v, svc.UnitPrice, "Đơn giá")
	checkOSUDecimalNonNeg(v, svc.ServiceDiscount, "Chiết khấu dịch vụ")
	checkOSUDecimalNonNeg(v, svc.DiscountOther, "Giảm giá khác")
	checkOSUDecimalNonNeg(v, svc.Total, "Tổng tiền")
	checkOSUDecimalNonNeg(v, &svc.DiscountServiceLock, "Giảm trừ khoá dịch vụ")
	checkOSUDecimalNonNeg(v, svc.RankDiscountPercent, "% giảm theo hạng")
	checkOSUDecimalNonNeg(v, svc.RankDiscountAmount, "Số tiền giảm theo hạng")
	checkOSUDecimalNonNeg(v, svc.TotalEnd, "Tổng tiền cuối")
	checkOSUDecimalNonNeg(v, svc.Payment, "Đã thanh toán")

	if svc.Payment != nil && svc.TotalEnd != nil && svc.Payment.Cmp(*svc.TotalEnd) > 0 {
		v.Add("Thanh toán vượt quá giá trị dịch vụ")
	}

	if svc.ServiceDiscountType != nil && *svc.ServiceDiscountType != 0 {
		// 1 = VND, 2 = %  (Yii2 OrderServiceUsed::TYPE_DISCOUNT_SERVICE_NOMARL/PERCEN)
		v.In(*svc.ServiceDiscountType, []int{1, 2}, "Loại chiết khấu dịch vụ")
	}

	if v.HasErrors() {
		return v
	}
	return nil
}

func checkOSUDecimalNonNeg(v *validation.Errors, d *decimal.Decimal, label string) {
	if d == nil {
		return
	}
	if d.IsNegative() {
		v.Add(label + " không thể nhỏ hơn 0")
	}
}

// serviceMoneyTolerance — lệch tối đa (VND) giữa total/totalEnd client gửi và số server tính lại: FE làm
// tròn bằng Math.round trên số thực, 1 đồng là đủ hấp thụ sai số đó.
var serviceMoneyTolerance = decimal.NewFromInt(1)

// expectedServiceUsedMoney — công thức tiền dòng dịch vụ, khớp FE order-plan-form `_recalcMoney` +
// `totalTotalService`:
//
//	gross    = đơn giá × số lượng
//	total    = round(gross − chiết khấu dịch vụ (VND, hoặc round(% × gross)))
//	totalEnd = round(total − giảm khác − giảm trừ khoá dịch vụ)
//
// (rank_discount_amount KHÔNG trừ vào totalEnd — server tính riêng.)
func expectedServiceUsedMoney(svc *crmServiceDao.OrderServiceUsed) (total, totalEnd decimal.Decimal) {
	unit := derefDec(svc.UnitPrice)
	qty := decimal.Zero
	if svc.Quantity != nil {
		qty = decimal.NewFromInt(int64(*svc.Quantity))
	}
	gross := unit.Mul(qty)
	total = gross
	if !gross.IsZero() {
		svcDisc := derefDec(svc.ServiceDiscount)
		if svc.ServiceDiscountType != nil && *svc.ServiceDiscountType == define.DiscountTypePercent {
			svcDisc = svcDisc.Mul(gross).Div(decimal.NewFromInt(100)).Round(0)
		}
		if svcDisc.IsPositive() {
			total = total.Sub(svcDisc)
		}
		total = total.Round(0)
	}
	totalEnd = total.Sub(derefDec(svc.DiscountOther)).Sub(svc.DiscountServiceLock).Round(0)
	return total, totalEnd
}

func derefDec(d *decimal.Decimal) decimal.Decimal {
	if d == nil {
		return decimal.Zero
	}
	return *d
}

// serviceUsedMoneyChanged — dòng cũ có trường tiền nào đổi so với bản đang lưu không.
func serviceUsedMoneyChanged(svc, prev *crmServiceDao.OrderServiceUsed) bool {
	decs := [][2]*decimal.Decimal{
		{svc.UnitPrice, prev.UnitPrice}, {svc.ServiceDiscount, prev.ServiceDiscount},
		{svc.DiscountOther, prev.DiscountOther}, {svc.Total, prev.Total}, {svc.TotalEnd, prev.TotalEnd},
	}
	for _, p := range decs {
		if !derefDec(p[0]).Equal(derefDec(p[1])) {
			return true
		}
	}
	intOr0 := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	return intOr0(svc.Quantity) != intOr0(prev.Quantity) || intOr0(svc.ServiceDiscountType) != intOr0(prev.ServiceDiscountType)
}

// checkServiceUsedMoney — lens-money M-16: chặn total/totalEnd client gửi lệch công thức (trước đây BE tin
// thẳng ⇒ đơn giá 10tr, totalEnd 1tr, không giảm giá nào vẫn lưu ⇒ doanh thu 1tr). prev != nil (dòng đã có)
// ⇒ chỉ kiểm khi một trường tiền thực sự đổi: dòng nhập từ hệ thống cũ có thể lệch công thức, sửa ghi chú
// đơn không được bị chặn vì nó.
func checkServiceUsedMoney(svc, prev *crmServiceDao.OrderServiceUsed) error {
	if svc.TotalEnd == nil && svc.Total == nil {
		return nil
	}
	if prev != nil && !serviceUsedMoneyChanged(svc, prev) {
		return nil
	}
	total, totalEnd := expectedServiceUsedMoney(svc)
	if svc.Total != nil && svc.Total.Sub(total).Abs().GreaterThan(serviceMoneyTolerance) {
		return errs.NewLocalizedError("service_total_mismatch", total.StringFixed(0), svc.Total.StringFixed(0))
	}
	if svc.TotalEnd != nil && svc.TotalEnd.Sub(totalEnd).Abs().GreaterThan(serviceMoneyTolerance) {
		return errs.NewLocalizedError("service_total_mismatch", totalEnd.StringFixed(0), svc.TotalEnd.StringFixed(0))
	}
	return nil
}
