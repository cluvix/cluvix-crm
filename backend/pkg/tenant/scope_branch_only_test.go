package tenant

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

// Story-35 (E6/D15 — 1 thương hiệu): `applyNullBranchScope` bỏ vế company, GIỮ NGUYÊN vế branch.
//
// TestApplyNullBranchScope_VeBranchGiuNguyen — neo HÀNH VI branch bằng chuỗi SQL literal: branch
// active ⇒ `= ? OR IS NULL`, danh sách branch ⇒ `IN ? OR IS NULL`, `branch_id IS NULL` LUÔN được
// lấy, alias, skipBranch, bảng không thuộc branchTables ⇒ không có vế branch; và KHÔNG ca nào có
// `company_id`.
//
// Story-41: xoá TestApplyNullBranchScope_BangSQLCuTruVeCompany + bản sao đóng băng thân cũ — test đó
// là bằng chứng chuyển đổi của story-35 (so thân cũ có company với thân mới), cần 2 field company
// của Bounds + registry company mà story-41 xoá hẳn. Vế branch vẫn neo đủ bằng test literal ở đây.

type scopeFn func(db *gorm.DB, s Bounds, table string, skipBranch bool, alias ...string) *gorm.DB

// renderScopeSQL dựng SQL đã nội suy tham số (DryRun, không mở kết nối).
func renderScopeSQL(t *testing.T, fn scopeFn, s Bounds, table string, skipBranch bool, alias string) string {
	t.Helper()
	db := dryRunDBForScope(t)
	from := table
	var al []string
	if alias != "" {
		from = table + " " + alias
		al = []string{alias}
	}
	var rows []map[string]any
	stmt := fn(db, s, table, skipBranch, al...).Table(from).Select("id").Find(&rows).Statement
	return db.Dialector.Explain(stmt.SQL.String(), stmt.Vars...)
}

type branchCase struct {
	name       string
	bounds     Bounds
	table      string
	alias      string
	skipBranch bool
	// muonCo: các mảnh SQL PHẢI có (không kèm ngoặc: GORM chỉ bọc ngoặc vế có OR khi
	// WHERE có ≥2 vế — lúc ghép thêm Where/Or ở caller, xem clause.Where.Build); khongCo: các mảnh KHÔNG được có.
	muonCo  []string
	khongCo []string
}

func branchCases() []branchCase {
	return []branchCase{
		{
			name:   "branch active ⇒ = ? OR IS NULL (NULL luôn lấy)",
			bounds: Bounds{BranchActiveID: 10},
			table:  "customer",
			muonCo: []string{"customer.branch_id = 10 OR customer.branch_id IS NULL"},
		},
		{
			name:   "branch active + danh sách ⇒ Active thắng",
			bounds: Bounds{BranchActiveID: 10, BranchIDs: []uint{10, 11}},
			table:  "customer",
			muonCo: []string{"customer.branch_id = 10 OR customer.branch_id IS NULL"},
			// ActiveID thắng — KHÔNG gộp danh sách vào.
			khongCo: []string{"customer.branch_id IN"},
		},
		{
			name:   "danh sách branch ⇒ IN ? OR IS NULL",
			bounds: Bounds{BranchIDs: []uint{10, 11}},
			table:  "appointment",
			muonCo: []string{"appointment.branch_id IN (10,11) OR appointment.branch_id IS NULL"},
		},
		{
			name:    "không có branch trong phạm vi ⇒ không có vế branch",
			bounds:  Bounds{},
			table:   "customer",
			khongCo: []string{"branch_id"},
		},
		{
			name:    "alias ⇒ vế branch dùng alias",
			bounds:  Bounds{BranchActiveID: 10},
			table:   "appointment",
			alias:   "a",
			muonCo:  []string{"a.branch_id = 10 OR a.branch_id IS NULL"},
			khongCo: []string{"appointment.branch_id"},
		},
		{
			name:       "skipBranch ⇒ không có vế branch",
			bounds:     Bounds{BranchActiveID: 10},
			table:      "customer",
			skipBranch: true,
			khongCo:    []string{"branch_id"},
		},
		{
			name:   "bảng chỉ thuộc branchTables ⇒ vế branch",
			bounds: Bounds{BranchActiveID: 10},
			table:  "config_code",
			muonCo: []string{"config_code.branch_id = 10 OR config_code.branch_id IS NULL"},
		},
		{
			name:    "bảng không thuộc branchTables (bank_accounts — không có branch_id) ⇒ không có vế nào",
			bounds:  Bounds{BranchActiveID: 10},
			table:   "bank_accounts",
			khongCo: []string{"branch_id"},
		},
		{
			name:    "bảng không thuộc registry nào ⇒ không có vế branch",
			bounds:  Bounds{BranchActiveID: 10},
			table:   "branch",
			khongCo: []string{"branch_id"},
		},
	}
}

func TestApplyNullBranchScope_VeBranchGiuNguyen(t *testing.T) {
	for _, tc := range branchCases() {
		t.Run(tc.name, func(t *testing.T) {
			sqlText := renderScopeSQL(t, applyNullBranchScope, tc.bounds, tc.table, tc.skipBranch, tc.alias)
			for _, m := range tc.muonCo {
				if !strings.Contains(sqlText, m) {
					t.Errorf("thiếu mảnh %q\nSQL: %s", m, sqlText)
				}
			}
			for _, m := range tc.khongCo {
				if strings.Contains(sqlText, m) {
					t.Errorf("không được có %q\nSQL: %s", m, sqlText)
				}
			}
		})
	}
}

// Ghép thêm Where ở caller ⇒ GORM PHẢI bọc ngoặc vế branch (có OR) — không thì `a OR b IS NULL AND x`
// đổi nghĩa. Trước story-35 luôn có ≥2 vế (company + branch) nên ngoặc luôn có; sau khi bỏ company,
// vế branch có thể đứng một mình trong scope ⇒ neo lại rằng ngoặc vẫn xuất hiện khi caller ghép.
func TestApplyNullBranchScope_VeBranchBocNgoacKhiGhepWhere(t *testing.T) {
	db := dryRunDBForScope(t)
	var rows []map[string]any
	for _, tc := range []struct {
		name string
		q    *gorm.DB
		want string
	}{
		{"Where", applyNullBranchScope(db, Bounds{BranchActiveID: 10}, "customer", false).Where("customer.stage_id = ?", 1),
			"(customer.branch_id = 10 OR customer.branch_id IS NULL) AND customer.stage_id = 1"},
		{"Or", applyNullBranchScope(db, Bounds{BranchIDs: []uint{10, 11}}, "customer", false).Or("customer.id = ?", 5),
			"(customer.branch_id IN (10,11) OR customer.branch_id IS NULL) OR customer.id = 5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stmt := tc.q.Table("customer").Select("id").Find(&rows).Statement
			got := db.Dialector.Explain(stmt.SQL.String(), stmt.Vars...)
			if !strings.Contains(got, tc.want) {
				t.Errorf("thiếu %q\nSQL: %s", tc.want, got)
			}
		})
	}
}

// AC3: CrossBranchLookup là identity — không thêm vế nào (kể cả branch) trên bảng có cả 2 cột
// scope, có alias hay không.
func TestCrossBranchLookup_KhongThemVeNao(t *testing.T) {
	db := dryRunDBForScope(t)
	var rows []map[string]any
	for _, tc := range []struct{ table, alias string }{{"customer", ""}, {"appointment", "a"}, {"bank_accounts", ""}} {
		from := tc.table
		var al []string
		if tc.alias != "" {
			from += " " + tc.alias
			al = []string{tc.alias}
		}
		stmt := CrossBranchLookup(db, tc.table, al...).Table(from).Select("id").Find(&rows).Statement
		got := db.Dialector.Explain(stmt.SQL.String(), stmt.Vars...)
		if strings.Contains(got, "WHERE") {
			t.Errorf("%s: CrossBranchLookup không được thêm điều kiện\nSQL: %s", tc.table, got)
		}
		// Đối chứng: cùng bảng qua ScopedDBNullBranch-body CÓ vế branch (trừ bank_accounts).
		if tc.table != "bank_accounts" {
			st2 := applyNullBranchScope(db, Bounds{BranchActiveID: 10}, tc.table, false, al...).Table(from).Select("id").Find(&rows).Statement
			if !strings.Contains(st2.SQL.String(), "branch_id") {
				t.Errorf("%s: đối chứng hỏng — scope thường không có vế branch", tc.table)
			}
		}
	}
}
