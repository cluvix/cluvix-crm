package handler

import (
	"sort"
	"strings"
	"testing"
)

// Lưới chống tái phát smoke S1 (epic customer-stage-pipeline): `customerStages` có trong configTables
// nhưng thiếu trong danh sách full sync mặc định ⇒ sau đăng nhập store IndexedDB rỗng.
// Tập loại trừ hiện tại = RỖNG (đã tính từ code 02/11/2026: mọi khoá của configTables + customTables
// đều nằm trong danh sách mặc định). Muốn loại cố ý một khoá thì thêm vào đây KÈM lý do.
var defaultSyncExcluded = map[string]string{}

func defaultSyncSet() map[string]bool {
	set := map[string]bool{}
	for _, t := range strings.Split(defaultSyncTables, ",") {
		set[strings.TrimSpace(t)] = true
	}
	return set
}

func TestDefaultSyncTables_ChuaMoiKhoaDaDangKy(t *testing.T) {
	set := defaultSyncSet()
	var missing []string
	check := func(k string) {
		if _, skip := defaultSyncExcluded[k]; !skip && !set[k] {
			missing = append(missing, k)
		}
	}
	for k := range configTables {
		check(k)
	}
	for k := range customTables {
		check(k)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("full sync mặc định thiếu khoá đã đăng ký: %v", missing)
	}
}

func TestDefaultSyncTables_CoCustomerStages(t *testing.T) {
	if !defaultSyncSet()["customerStages"] {
		t.Fatal("full sync mặc định phải trả customerStages (FE nạp bước lúc đăng nhập)")
	}
}
