package handler

import (
	"fmt"
	"strings"
	"testing"

	"erpCluvix/pkg/define"
)

// Danh mục 8 bước như seed (đã sort theo sort_order, id); id cố ý KHÁC thứ tự để bắt nhầm id ↔ vị trí.
func seedStages() []customerStageRow {
	o, w, l := define.CustomerStageTypeOpen, define.CustomerStageTypeWon, define.CustomerStageTypeLost
	return []customerStageRow{
		{ID: 11, Name: "Liên hệ", Type: o, Color: "gray", Status: 1},
		{ID: 12, Name: "Đang tư vấn", Type: o, Color: "blue", Status: 1},
		{ID: 30, Name: "Từ chối", Type: l, Color: "red", Status: 1}, // bước thất bại xen giữa: vẫn ra cuối
		{ID: 13, Name: "Đang đàm phán", Type: o, Color: "amber", Status: 1},
		{ID: 14, Name: "Đã lên đơn", Type: o, Color: "purple", Status: 1},
		{ID: 15, Name: "Khách hàng đã thuê", Type: w, Color: "green", Status: 1},
		{ID: 31, Name: "Dừng chăm sóc", Type: l, Color: "orange", Status: 1},
		{ID: 32, Name: "Ngừng thuê", Type: l, Color: "teal", Status: 1},
	}
}

func ip(v int) *int { return &v }

func funnelStr(items []customerStageFunnelItem) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, fmt.Sprintf("%s=%d", it.Name, it.Value))
	}
	return strings.Join(parts, ", ")
}

// Kịch bản PRD AC12 ở dạng kết quả SQL: A tới vị trí 2 (Đang tư vấn) và đang ở Từ chối; B tới vị trí 5.
func TestBuildCustomerStageFunnel_PRD(t *testing.T) {
	got := buildCustomerStageFunnel(seedStages(), []customerStageFunnelCount{
		{Mo: ip(2), Ls: 30, N: 1},
		{Mo: ip(5), Ls: 0, N: 1},
	})
	want := "Liên hệ=2, Đang tư vấn=2, Đang đàm phán=1, Đã lên đơn=1, Khách hàng đã thuê=1, Từ chối=1, Dừng chăm sóc=0, Ngừng thuê=0"
	if s := funnelStr(got); s != want {
		t.Fatalf("muốn %s\nnhận %s", want, s)
	}
	if got[0].StageID != 11 || got[0].Color != "gray" || got[0].Type != define.CustomerStageTypeOpen || got[5].StageID != 30 {
		t.Fatalf("stage_id/color/type sai: %+v", got)
	}
}

func TestBuildCustomerStageFunnel_HiddenAndEmpty(t *testing.T) {
	st := seedStages()
	st[3].Status = 0 // Đang đàm phán ẩn — có khách ⇒ giữ
	st[6].Status = 0 // Dừng chăm sóc ẩn — 0 khách ⇒ bỏ
	got := buildCustomerStageFunnel(st, []customerStageFunnelCount{{Mo: ip(3), N: 4}})
	want := "Liên hệ=4, Đang tư vấn=4, Đang đàm phán=4, Đã lên đơn=0, Khách hàng đã thuê=0, Từ chối=0, Ngừng thuê=0"
	if s := funnelStr(got); s != want {
		t.Fatalf("muốn %s\nnhận %s", want, s)
	}

	for name, counts := range map[string][]customerStageFunnelCount{
		"không nhóm nào":          nil,
		"chỉ khách không lịch sử": {{Mo: nil, Ls: 0, N: 7}, {Mo: ip(0), Ls: 0, N: 2}},
	} {
		got := buildCustomerStageFunnel(seedStages(), counts)
		if got == nil || len(got) != 0 {
			t.Fatalf("%s: muốn [] (không nil), nhận %#v", name, got)
		}
	}

	// Khách chỉ có lịch sử bước thất bại (mo = 0) vẫn được đếm ở bước thất bại đó.
	got = buildCustomerStageFunnel(seedStages(), []customerStageFunnelCount{{Mo: ip(0), Ls: 32, N: 3}})
	want = "Liên hệ=0, Đang tư vấn=0, Đang đàm phán=0, Đã lên đơn=0, Khách hàng đã thuê=0, Từ chối=0, Dừng chăm sóc=0, Ngừng thuê=3"
	if s := funnelStr(got); s != want {
		t.Fatalf("muốn %s\nnhận %s", want, s)
	}
}
