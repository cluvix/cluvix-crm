package service

import (
	"testing"

	"github.com/shopspring/decimal"
)

// D5/D7: phần trước thuế của khoản có thuế theo tỷ lệ dòng; khoá chia đã thu thành trước thuế + thuế, tổng = đã thu.
func TestPreTaxShareAndLockSplit(t *testing.T) {
	d := decimal.NewFromInt
	cases := []struct {
		name                      string
		amount, totalEnd, vat     int64
		wantPreTax, wantLockedVat int64
	}{
		{"VAT 10%, thu nửa", 550_000, 1_000_000, 100_000, 500_000, 50_000},
		{"không VAT ⇒ nguyên khoản", 300_000, 1_000_000, 0, 300_000, 0},
		{"VAT 8% làm tròn", 100_000, 123_456, 9_876, 92_593, 7_407}, // 100000×123456/133332 = 92592.9
		{"phải thu 0", 0, 0, 0, 0, 0},
	}
	for _, tc := range cases {
		got := preTaxShare(d(tc.amount), d(tc.totalEnd), d(tc.vat))
		if !got.Equal(d(tc.wantPreTax)) {
			t.Fatalf("%s: preTaxShare muốn %d, có %s", tc.name, tc.wantPreTax, got)
		}
		te, vat := LockSplit(d(tc.totalEnd), d(tc.vat), d(tc.amount))
		if !te.Equal(d(tc.wantPreTax)) || !vat.Equal(d(tc.wantLockedVat)) || !te.Add(vat).Equal(d(tc.amount)) {
			t.Fatalf("%s: LockSplit muốn %d + %d, có %s + %s", tc.name, tc.wantPreTax, tc.wantLockedVat, te, vat)
		}
	}
}
