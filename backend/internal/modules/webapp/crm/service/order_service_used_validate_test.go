package service

import (
	"testing"

	crmServiceDao "erpCluvix/internal/modules/webapp/crm/service/dao"

	"github.com/shopspring/decimal"
)

// baseValidOSU trả về 1 svc đủ field required để validateOrderServiceUsed không có lỗi.
// (crm-standalone story-14: test nhánh numTreatment theo serviceCategory_id đã bỏ cùng rule đó.)
func baseValidOSU() *crmServiceDao.OrderServiceUsed {
	price := decimal.NewFromInt(100)
	qty := 1
	return &crmServiceDao.OrderServiceUsed{
		CustomerId:  1,
		ServiceId:   1,
		UnitPrice:   &price,
		OrderPlanId: 1,
		Quantity:    &qty,
	}
}

func TestValidateOrderServiceUsed_Basic(t *testing.T) {
	if err := validateOrderServiceUsed(baseValidOSU()); err != nil {
		t.Fatalf("svc hợp lệ không được lỗi, có %v", err)
	}

	svc := baseValidOSU()
	pay, total := decimal.NewFromInt(200), decimal.NewFromInt(100)
	svc.Payment, svc.TotalEnd = &pay, &total
	if err := validateOrderServiceUsed(svc); err == nil {
		t.Fatal("payment > totalEnd phải bị chặn")
	}

	svc = baseValidOSU()
	zero := 0
	svc.Quantity = &zero
	if err := validateOrderServiceUsed(svc); err == nil {
		t.Fatal("quantity < 1 phải bị chặn")
	}
}

// Công thức tiền dòng sau khi bỏ giảm giá dòng (`discount`):
// total = đơn giá × số lượng − chiết khấu dịch vụ; totalEnd = total − giảm khác − giảm trừ khoá dịch vụ.
func TestExpectedServiceUsedMoney_NoLineDiscount(t *testing.T) {
	d := decimal.NewFromInt
	pct, vnd := 2, 1
	cases := []struct {
		name                 string
		unit, sd, oth        int64
		qty, sdt             int
		wantTotal, wantTotEn int64
	}{
		{"chiết khấu dịch vụ 10%", 500_000, 10, 50_000, 2, pct, 900_000, 850_000},
		{"chiết khấu dịch vụ VND", 1_000_000, 100_000, 0, 1, vnd, 900_000, 900_000},
		{"không chiết khấu", 300_000, 0, 0, 1, vnd, 300_000, 300_000},
	}
	for _, tc := range cases {
		svc := baseValidOSU()
		unit, sd, oth := d(tc.unit), d(tc.sd), d(tc.oth)
		qty, sdt := tc.qty, tc.sdt
		svc.UnitPrice, svc.Quantity, svc.ServiceDiscount, svc.ServiceDiscountType, svc.DiscountOther = &unit, &qty, &sd, &sdt, &oth
		total, totalEnd := expectedServiceUsedMoney(svc)
		if !total.Equal(d(tc.wantTotal)) || !totalEnd.Equal(d(tc.wantTotEn)) {
			t.Fatalf("%s: muốn total=%d totalEnd=%d, có total=%s totalEnd=%s", tc.name, tc.wantTotal, tc.wantTotEn, total, totalEnd)
		}
	}
}

// VAT dòng (bmad/order-vat D2): round(totalEnd × % / 100) theo DÒNG; KCT / 0% / nil / mức lạ ⇒ 0.
func TestExpectedVatAmount(t *testing.T) {
	d := decimal.NewFromInt
	rate := func(s string) *string { return &s }
	cases := []struct {
		name     string
		totalEnd decimal.Decimal
		rate     *string
		want     decimal.Decimal
	}{
		{"10%", d(850_000), rate("10"), d(85_000)},
		{"8% làm tròn lên", d(123_456), rate("8"), d(9_876)},   // 9876.48
		{"5% làm tròn .5 lên", d(10), rate("5"), d(1)},         // 0.5
		{"8% làm tròn xuống", d(123_440), rate("8"), d(9_875)}, // 9875.2
		{"KCT", d(500_000), rate("KCT"), d(0)},
		{"0%", d(500_000), rate("0"), d(0)},
		{"đơn cũ (nil)", d(500_000), nil, d(0)},
		{"mức lạ", d(500_000), rate("7"), d(0)},
		{"totalEnd 0", d(0), rate("10"), d(0)},
	}
	for _, tc := range cases {
		if got := expectedVatAmount(tc.totalEnd, tc.rate); !got.Equal(tc.want) {
			t.Fatalf("%s: muốn %s, có %s", tc.name, tc.want, got)
		}
	}
}

// Dòng đã KHOÁ giữ VAT chốt lúc khoá; dòng thường tính lại theo mức VAT đơn.
func TestApplyServiceUsedVat_LockedFrozen(t *testing.T) {
	ten := "10"
	svc := baseValidOSU()
	te := decimal.NewFromInt(200_000)
	svc.TotalEnd = &te
	applyServiceUsedVat(svc, &ten)
	if !svc.VatAmount.Equal(decimal.NewFromInt(20_000)) {
		t.Fatalf("dòng thường: muốn VAT 20000, có %s", svc.VatAmount)
	}
	svc.IsLock = 2
	svc.VatAmount = decimal.NewFromInt(7_000)
	applyServiceUsedVat(svc, &ten)
	if !svc.VatAmount.Equal(decimal.NewFromInt(7_000)) {
		t.Fatalf("dòng khoá phải giữ VAT 7000, có %s", svc.VatAmount)
	}
}

// Đã thu được tới totalEnd + VAT (số phải thu), vượt mới chặn.
func TestValidateOrderServiceUsed_PayableIncludesVat(t *testing.T) {
	svc := baseValidOSU()
	te, pay := decimal.NewFromInt(100_000), decimal.NewFromInt(110_000)
	svc.TotalEnd, svc.Payment, svc.VatAmount = &te, &pay, decimal.NewFromInt(10_000)
	if err := validateOrderServiceUsed(svc); err != nil {
		t.Fatalf("đã thu = totalEnd + VAT phải hợp lệ, có %v", err)
	}
	pay = decimal.NewFromInt(110_001)
	if err := validateOrderServiceUsed(svc); err == nil {
		t.Fatal("đã thu > totalEnd + VAT phải bị chặn")
	}
	svc.Payment, svc.VatAmount = nil, decimal.NewFromInt(-1)
	if err := validateOrderServiceUsed(svc); err == nil {
		t.Fatal("VAT âm phải bị chặn")
	}
}

// vat_amount FE gửi: không gửi ⇒ bỏ qua; lệch ≤ 1đ ⇒ qua; lệch hơn ⇒ chặn.
func TestCheckServiceUsedVat(t *testing.T) {
	svc := baseValidOSU()
	svc.VatAmount = decimal.NewFromInt(9_876)
	for _, sent := range []any{nil, 9_876, "9877", 9_875.0} {
		if err := checkServiceUsedVat(svc, sent); err != nil {
			t.Fatalf("gửi %v phải qua, có %v", sent, err)
		}
	}
	if err := checkServiceUsedVat(svc, 9_878); err == nil {
		t.Fatal("lệch 2đ phải bị chặn")
	}
}
