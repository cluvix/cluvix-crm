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
