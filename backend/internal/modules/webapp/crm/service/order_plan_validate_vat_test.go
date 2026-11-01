package service

import (
	"testing"
	"time"

	orderPlanDao "erpCluvix/internal/modules/webapp/crm/order_plan/dao"
)

func vatOrder(rate string, day string, loc *time.Location) *orderPlanDao.OrderPlan {
	t, _ := time.ParseInLocation("2006-01-02 15:04", day, loc)
	ts := t.Unix()
	return &orderPlanDao.OrderPlan{VatRate: &rate, CreatedTime: &ts, CustomerId: 1, BranchId: new(1)}
}

// Mức 8%: ngày đơn (giờ thương hiệu) tới hết 31/12/2026 mới chọn được; đơn đã lưu 8% không đổi mức/ngày thì sửa
// được sau mốc.
func TestCheckOrderVatRate8(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Skip("thiếu tzdata")
	}
	if err := checkOrderVatRate8(loc, vatOrder("8", "2026-12-31 23:59", loc), nil); err != nil {
		t.Fatalf("31/12/2026 23:59 phải được chọn 8%%, có %v", err)
	}
	if err := checkOrderVatRate8(loc, vatOrder("8", "2027-01-01 00:00", loc), nil); err == nil {
		t.Fatal("01/01/2027 phải bị chặn 8%")
	}
	if err := checkOrderVatRate8(loc, vatOrder("10", "2027-01-01 00:00", loc), nil); err != nil {
		t.Fatalf("10%% sau mốc phải qua, có %v", err)
	}
	// Đơn cũ đã lưu 8% (ngày trong hạn), sửa phần khác ⇒ qua; đổi ngày sang sau mốc ⇒ chặn.
	prev := vatOrder("8", "2026-12-30 10:00", loc)
	same := vatOrder("8", "2026-12-30 10:00", loc)
	if err := checkOrderVatRate8(loc, same, prev); err != nil {
		t.Fatalf("giữ nguyên 8%% + ngày phải qua, có %v", err)
	}
	if err := checkOrderVatRate8(loc, vatOrder("8", "2027-01-02 10:00", loc), prev); err == nil {
		t.Fatal("dời ngày đơn 8% qua mốc phải bị chặn")
	}
}

func TestValidateOrderPlan_VatRate(t *testing.T) {
	for _, r := range []string{"KCT", "0", "5", "8", "10"} {
		o := vatOrder(r, "2026-11-01 10:00", time.UTC)
		if err := validateOrderPlan(o); err != nil {
			t.Fatalf("mức %s phải hợp lệ, có %v", r, err)
		}
	}
	if err := validateOrderPlan(vatOrder("7", "2026-11-01 10:00", time.UTC)); err == nil {
		t.Fatal("mức 7 phải bị chặn")
	}
}
