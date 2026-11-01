package service

import (
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Tiền có VAT của dòng order_service_used (bmad/order-vat — phương án B): totalEnd GIỮ là tiền TRƯỚC thuế,
// vat_amount = VAT dòng. Phiếu thu/hoàn (payment, refundServiceUsed) là tiền CÓ thuế.
//   - phải thu dòng   = totalEnd + vat_amount (D4) — so với đã thu ra công nợ / trạng thái thanh toán.
//   - doanh số dòng   = totalEnd − phần TRƯỚC thuế của hoàn (D5–D6); phần trước thuế của một khoản có thuế
//     = khoản × totalEnd / (totalEnd + vat_amount). Dòng không VAT ⇒ tỷ lệ 1 ⇒ số liệu như trước khi có VAT.
//
// Các đoạn SQL dưới dùng tên cột trần (không alias bảng) — chỉ dùng trong truy vấn 1 bảng order_service_used.
const (
	osuPayableSQL = "(COALESCE(totalEnd, 0) + COALESCE(vat_amount, 0))"

	// osuReceivableSumSQL — Σ phải thu − Σ hoàn của tập dòng: số đối chiếu với đã-thu-ròng ra công nợ.
	osuReceivableSumSQL = "COALESCE(SUM(COALESCE(totalEnd, 0) + COALESCE(vat_amount, 0)), 0) - COALESCE(SUM(refundServiceUsed), 0)"

	// osuRefundPreTaxSQL — phần TRƯỚC thuế của tiền hoàn 1 dòng (làm tròn đồng).
	osuRefundPreTaxSQL = "CASE WHEN COALESCE(vat_amount, 0) = 0 OR COALESCE(totalEnd, 0) + COALESCE(vat_amount, 0) <= 0 " +
		"THEN COALESCE(refundServiceUsed, 0) " +
		"ELSE ROUND(COALESCE(refundServiceUsed, 0) * COALESCE(totalEnd, 0) / (COALESCE(totalEnd, 0) + vat_amount), 0) END"

	// osuRevenueSumSQL — doanh số TRƯỚC thuế của tập dòng = ΣtotalEnd − Σ phần trước thuế của hoàn.
	osuRevenueSumSQL = "COALESCE(SUM(totalEnd), 0) - COALESCE(SUM(" + osuRefundPreTaxSQL + "), 0)"
)

// preTaxShare — phần TRƯỚC thuế của một khoản CÓ thuế `amount` trên dòng (totalEnd, vat): amount × totalEnd /
// (totalEnd + vat), làm tròn đồng. Không VAT / phải thu ≤ 0 ⇒ trả nguyên amount.
func preTaxShare(amount, totalEnd, vat decimal.Decimal) decimal.Decimal {
	payable := totalEnd.Add(vat)
	if vat.IsZero() || !payable.IsPositive() {
		return amount
	}
	return amount.Mul(totalEnd).Div(payable).Round(0)
}

// customerRevenueReceivable — doanh số TRƯỚC thuế và (Σ phải thu − Σ hoàn) của khách, tính từ nguồn
// order_service_used (D6). Công nợ khách = receivable − đã thu ròng.
func customerRevenueReceivable(tx *gorm.DB, customerID int) (revenue, receivable decimal.Decimal, err error) {
	var row struct {
		Revenue    decimal.Decimal `gorm:"column:revenue"`
		Receivable decimal.Decimal `gorm:"column:receivable"`
	}
	err = tx.Table("order_service_used").
		Select(osuRevenueSumSQL+" AS revenue, "+osuReceivableSumSQL+" AS receivable").
		Where("customer_id = ?", customerID).Scan(&row).Error
	return row.Revenue, row.Receivable, err
}

// LockSplit — khoá dòng dịch vụ (bmad/order-vat D7): phải thu chốt = đã thu `payment` (CÓ thuế), chia theo tỷ lệ
// dòng thành phần trước thuế (totalEnd mới) + phần thuế (vat_amount mới), tổng đúng bằng payment.
func LockSplit(totalEnd, vat, payment decimal.Decimal) (lockedTotalEnd, lockedVat decimal.Decimal) {
	lockedTotalEnd = preTaxShare(payment, totalEnd, vat)
	return lockedTotalEnd, payment.Sub(lockedTotalEnd)
}

// OrderServiceUsedVat — VAT của 1 dòng có totalEnd thuộc đơn orderPlanID theo mức VAT hiện tại của đơn (mở khoá
// dịch vụ tính lại VAT trên totalEnd khôi phục — D7).
func OrderServiceUsedVat(db *gorm.DB, orderPlanID int, totalEnd decimal.Decimal) decimal.Decimal {
	var rate *string
	db.Table("order_plan").Select("vat_rate").Where("id = ?", orderPlanID).Scan(&rate)
	return expectedVatAmount(totalEnd, rate)
}
