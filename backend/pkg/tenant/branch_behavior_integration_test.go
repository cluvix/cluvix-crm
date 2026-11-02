//go:build integration

package tenant

import (
	"sort"
	"testing"
)

// Neo hành vi BRANCH của ScopedDBNullBranch trên DB thật (bảng synthetic `canary_scope`).
//
//	A: branch=10 · B: branch=11 · C: branch=20 · D, E: branch=NULL (chưa gán chi nhánh — LUÔN lấy)
func seedCanaryBranch(t *testing.T) {
	t.Helper()
	testDB.Exec("DELETE FROM canary_scope")
	for _, s := range []struct {
		label  string
		branch uint
	}{{"A", 10}, {"B", 11}, {"C", 20}, {"D", 0}, {"E", 0}} {
		if err := testDB.Exec("INSERT INTO canary_scope (label, branch_id) VALUES (?, ?)",
			s.label, up(s.branch)).Error; err != nil {
			t.Fatalf("seed %s: %v", s.label, err)
		}
	}
}

func TestScopedDBNullBranch_ChiBranch(t *testing.T) {
	if testDB == nil {
		t.Skip("no test DB")
	}
	seedCanaryBranch(t)

	cases := []struct {
		name         string
		branchActive uint
		branchIds    []uint
		skip         bool
		want         []string
	}{
		{name: "branchActive=10 ⇒ A + NULL(D,E)", branchActive: 10, want: []string{"A", "D", "E"}},
		{name: "branchIds=[10,20] ⇒ A,C + NULL(D,E)", branchIds: []uint{10, 20}, want: []string{"A", "C", "D", "E"}},
		{name: "branchActive thắng branchIds", branchActive: 20, branchIds: []uint{10, 11}, want: []string{"C", "D", "E"}},
		{name: "branch không khớp ⇒ chỉ NULL (D,E)", branchActive: 999, want: []string{"D", "E"}},
		{name: "không có branch trong ctx ⇒ tất cả", want: []string{"A", "B", "C", "D", "E"}},
		{name: "skipBranch ⇒ tất cả", branchActive: 10, skip: true, want: []string{"A", "B", "C", "D", "E"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := ctxWith(tc.branchActive, tc.branchIds)
			var got []string
			if err := ScopedDBNullBranchSkipBranch(c, testDB, "canary_scope", tc.skip).
				Table("canary_scope").Order("label").Pluck("label", &got).Error; err != nil {
				t.Fatalf("query: %v", err)
			}
			sort.Strings(tc.want)
			if !stringSliceEq(got, tc.want) {
				t.Errorf("got=%v want=%v", got, tc.want)
			}
		})
	}
}

// Cross-branch on/off: IsCrossBranchEnabled (id=18) đọc user_option, cache per-request; caller dùng
// kết quả làm skipBranch. Hành vi này KHÔNG đổi ở D15.
func TestCrossBranchOption_OnOff(t *testing.T) {
	if testDB == nil {
		t.Skip("no test DB")
	}
	seedCanaryBranch(t)
	restore := map[int]int{}
	for _, id := range []int{optCrossBranchID} {
		var v int
		if err := testDB.Table("user_option").Where("id = ?", id).Limit(1).Pluck("options", &v).Error; err != nil {
			t.Fatalf("đọc user_option %d: %v", id, err)
		}
		restore[id] = v
	}
	t.Cleanup(func() {
		for id, v := range restore {
			testDB.Exec("UPDATE user_option SET options = ? WHERE id = ?", v, id)
		}
	})

	for _, on := range []bool{false, true} {
		val := 0
		if on {
			val = 1
		}
		for _, id := range []int{optCrossBranchID} {
			if err := testDB.Exec("UPDATE user_option SET options = ? WHERE id = ?", val, id).Error; err != nil {
				t.Fatalf("set user_option %d=%d: %v", id, val, err)
			}
			var back int
			testDB.Table("user_option").Where("id = ?", id).Limit(1).Pluck("options", &back)
			if back != val {
				t.Fatalf("user_option %d không có dòng hoặc không ghi được: muốn %d, đọc lại %d", id, val, back)
			}
		}

		c := ctxWith(10, nil)
		if got := IsCrossBranchEnabled(c, testDB); got != on {
			t.Errorf("IsCrossBranchEnabled on=%v: got %v", on, got)
		}

		var got []string
		if err := ScopedDBNullBranchSkipBranch(c, testDB, "canary_scope", IsCrossBranchEnabled(c, testDB)).
			Table("canary_scope").Order("label").Pluck("label", &got).Error; err != nil {
			t.Fatalf("query: %v", err)
		}
		want := []string{"A", "D", "E"}
		if on {
			want = []string{"A", "B", "C", "D", "E"}
		}
		if !stringSliceEq(got, want) {
			t.Errorf("cross-branch on=%v: got=%v want=%v", on, got, want)
		}

		// Cache per-request: đổi DB giữa chừng KHÔNG đổi kết quả trong cùng context.
		testDB.Exec("UPDATE user_option SET options = ? WHERE id = ?", 1-val, optCrossBranchID)
		if got := IsCrossBranchEnabled(c, testDB); got != on {
			t.Errorf("cache per-request vỡ: on=%v got %v sau khi đổi DB", on, got)
		}
	}
}
