package handler

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/pkg/tenant"
)

// DryRun — neo HÌNH DẠNG SQL của bản vá C-4 (list_branch_split.go). Kết quả chạy thật trên MySQL
// do list_branch_split_integration_test.go gánh.

func ctxWithScope(t *testing.T, rawQuery string, active uint, ids []uint) *gin.Context {
	t.Helper()
	c := custCtx(t, rawQuery)
	if active > 0 {
		c.Set("branchActiveId", active)
	}
	if ids != nil {
		c.Set("branchIds", ids)
	}
	return c
}

func explainSQL(db *gorm.DB, q *gorm.DB) string {
	return strings.Join(strings.Fields(db.Dialector.Explain(q.Statement.SQL.String(), q.Statement.Vars...)), " ")
}

func TestCustomerListSingleBranch(t *testing.T) {
	cases := []struct {
		name   string
		active uint
		ids    []uint
		want   uint
		single bool
	}{
		{"branchActive", 7, nil, 7, true},
		{"branchActive thắng branchIds", 7, []uint{1, 2}, 7, true},
		{"branchIds 1 phần tử", 0, []uint{9}, 9, true},
		{"branchIds nhiều phần tử", 0, []uint{1, 2}, 0, false},
		{"không có phạm vi chi nhánh", 0, nil, 0, false},
		{"branchIds rỗng", 0, []uint{}, 0, false},
	}
	for _, tc := range cases {
		got, single := customerListSingleBranch(ctxWithScope(t, "", tc.active, tc.ids))
		if got != tc.want || single != tc.single {
			t.Errorf("%s: = (%d,%v), muốn (%d,%v)", tc.name, got, single, tc.want, tc.single)
		}
	}
}

// Hợp 2 nhánh UNION chỉ ĐỦ khi vế scope tenant đúng là `branch_id = B OR branch_id IS NULL`
// (hoặc `IN (B) OR IS NULL`). Luật scope ở pkg/tenant đổi mà customerListSingleBranch không đổi
// theo ⇒ danh sách THIẾU dòng im lặng. Test này đỏ ngay khi hai bên lệch.
func TestCustomerSingleBranchMatchesTenantScope(t *testing.T) {
	db := dryRunDB(t)
	h := New(Deps{DB: db})
	var total int64

	for _, tc := range []struct {
		active uint
		ids    []uint
		want   string
	}{
		{7, nil, "(customer.branch_id = 7 OR customer.branch_id IS NULL)"},
		{0, []uint{9}, "(customer.branch_id IN (9) OR customer.branch_id IS NULL)"},
	} {
		c := ctxWithScope(t, "", tc.active, tc.ids)
		if _, single := customerListSingleBranch(c); !single {
			t.Fatalf("(%d,%v) phải là phạm vi 1 chi nhánh", tc.active, tc.ids)
		}
		sqlText := explainSQL(db, h.buildCustomerFilters(c, false).Session(&gorm.Session{}).Count(&total))
		// Story-04 bỏ vế `status IN` ⇒ không lọc gì thêm thì vế scope là điều kiện DUY NHẤT và GORM bỏ
		// cặp ngoặc ngoài (`WHERE a OR b` — vẫn đúng ngữ nghĩa; ghép thêm điều kiện thì GORM tự bọc lại).
		if !strings.Contains(sqlText, tc.want) && "("+whereOf(sqlText)+")" != tc.want {
			t.Errorf("vế scope tenant đã đổi — xét lại customerListSingleBranch + UNION 2 nhánh:\nmuốn chứa %s\n%s",
				tc.want, sqlText)
		}
		if strings.Count(sqlText, "branch_id") != 2 {
			t.Errorf("scope tenant sinh thêm vế branch_id khác ⇒ hợp 2 nhánh có thể không còn đúng:\n%s", sqlText)
		}
	}
}

func TestCustomerListSplitWorthIt(t *testing.T) {
	cases := []struct {
		total           int64
		offset, perPage int
		want            bool
	}{
		{160498, 0, 15, true},
		{160498, 7485, 15, true},
		{160498, 80234, 15, true}, // window 80249 = total/2
		{160498, 80235, 15, false},
		{160498, 599985, 15, false}, // vượt tổng ⇒ đường cũ
		{29, 0, 15, false},
		{30, 0, 15, true},
		{0, 0, 15, false},
		{100, -1, 15, false},
		{100, 0, 0, false},
	}
	for _, tc := range cases {
		if got := customerListSplitWorthIt(tc.total, tc.offset, tc.perPage); got != tc.want {
			t.Errorf("total=%d offset=%d perPage=%d: = %v, muốn %v", tc.total, tc.offset, tc.perPage, got, tc.want)
		}
	}
}

// Câu COUNT KHÔNG còn optimizer hint ở MỌI phạm vi (story-04 customer-stage-pipeline gỡ hint
// `NO_INDEX(fk-customer_branch_id, …)`: bỏ `status IN` ⇒ fk-branch là chỉ mục phủ, giữ hint chậm 1,52× —
// bảng số đo ở list_branch_split.go). Gắn lại hint mà không đo lại ở quy mô ⇒ test này đỏ.
func TestCustomerCountHasNoHint(t *testing.T) {
	db := dryRunDB(t)
	h := New(Deps{DB: db})

	for _, tc := range []struct {
		name   string
		qs     string
		active uint
		ids    []uint
	}{
		{"1 CN, không lọc", "", 7, nil},
		{"1 CN qua branchIds", "", 0, []uint{7}},
		{"1 CN + lọc 1 bước", "filter[stage_id][0]=2", 7, nil},
		{"1 CN + lọc 2 bước", "filter[stage_id][0]=2&filter[stage_id][1]=5", 7, nil},
		{"1 CN + từ khoá LIKE", "_keyword=Nguyen", 7, nil},
		{"1 CN + từ khoá ngram", "_keyword=0913353112", 7, nil},
		{"nhiều CN", "", 0, []uint{1, 2}},
		{"không phạm vi CN + lọc bước", "filter[stage_id][0]=2", 0, nil},
	} {
		c := ctxWithScope(t, tc.qs, tc.active, tc.ids)
		var total int64
		sqlText := explainSQL(db, customerCountQuery(h.buildCustomerFilters(c, false), &total))
		if strings.Contains(sqlText, "/*+") || !strings.HasPrefix(sqlText, "SELECT count(*) FROM `customer` WHERE ") {
			t.Errorf("%s: COUNT phải là Count trần, không hint:\n%s", tc.name, sqlText)
		}
		// Đường COUNT không được đổi bộ lọc so với Count trần trên cùng base.
		plain := explainSQL(db, h.buildCustomerFilters(c, false).Session(&gorm.Session{}).Count(&total))
		if whereOf(sqlText) != whereOf(plain) {
			t.Errorf("%s: WHERE của câu COUNT bị đổi:\n%s\n%s", tc.name, sqlText, plain)
		}
		if strings.Contains(whereOf(sqlText), "customer.status") {
			t.Errorf("%s: COUNT còn lọc ngầm theo status (FR17):\n%s", tc.name, sqlText)
		}
	}
}

// Câu trang UNION: mỗi nhánh mang NGUYÊN bộ lọc base + đúng một vế equality, cùng LIMIT cửa sổ.
func TestCustomerListSplitPageShape(t *testing.T) {
	db := dryRunDB(t)
	h := New(Deps{DB: db})

	for _, qs := range []string{"", "_keyword=0913353112", "filter[gender]=1&filter[stage_id][0]=2", "_keyword=Nguyen"} {
		c := ctxWithScope(t, qs, 7, nil)
		base := h.buildCustomerFilters(c, false)
		var total int64
		baseWhere := whereOf(explainSQL(db, base.Session(&gorm.Session{}).Count(&total)))
		if baseWhere == "" {
			t.Fatalf("(%q) không tách được WHERE", qs)
		}
		// Vế scope đứng một mình (qs rỗng) ⇒ GORM in không ngoặc; ghép thêm vế nhánh thì GORM bọc lại.
		if !strings.HasPrefix(baseWhere, "(") && strings.Contains(baseWhere, " OR ") {
			baseWhere = "(" + baseWhere + ")"
		}
		var items []map[string]interface{}
		sqlText := explainSQL(db, customerListSplitPage(db, base, 7, 30, 15).Find(&items))

		for _, part := range []string{
			"(SELECT customer.id FROM `customer` WHERE " + baseWhere + " AND customer.branch_id = 7 ORDER BY customer.id DESC LIMIT 45)",
			"(SELECT customer.id FROM `customer` WHERE " + baseWhere + " AND customer.branch_id IS NULL ORDER BY customer.id DESC LIMIT 45)",
			" UNION ALL ",
			"ORDER BY id DESC LIMIT 15 OFFSET 30) AS customer_page)",
		} {
			if !strings.Contains(sqlText, part) {
				t.Errorf("(%q) câu trang thiếu %q:\n%s", qs, part, sqlText)
			}
		}
		// Câu ngoài: chỉ lọc theo id của trang (không gắn lại scope — xem doc customerListSplitPage).
		head := sqlText[:strings.Index(sqlText, "(SELECT customer.id")]
		if !strings.HasSuffix(head, " FROM `customer` WHERE customer.id IN (SELECT customer_page.id FROM (") ||
			!strings.HasSuffix(sqlText, ") AS customer_page) ORDER BY customer.id DESC") {
			t.Errorf("(%q) câu ngoài phải chỉ lọc theo id của trang + ORDER BY id DESC:\n%s", qs, sqlText)
		}
		if !strings.HasPrefix(sqlText, "SELECT "+strings.Join(strings.Fields(customerListSelect), " ")+" FROM `customer` WHERE customer.id IN (") {
			t.Errorf("(%q) câu ngoài phải giữ nguyên danh sách cột customerListSelect:\n%s", qs, sqlText)
		}
	}
}

// ── search-global (bench C-4 vòng 2) ──────────────────────────────────────────────────────

// oldSearchGlobalQuery — câu search-global TRƯỚC bản vá, ĐÓNG BĂNG làm chuẩn đối chiếu (CỐ Ý
// chép, không gọi hàm sản phẩm). Dùng cả ở test DryRun lẫn parity integration.
func oldSearchGlobalQuery(h *Handler, c *gin.Context, keyword string) (*gorm.DB, error) {
	like := "%" + keyword + "%"
	q := h.DB.Table("customer").Select("id, fullname, nickname, code, phone")
	if !tenant.IsCrossBranchEnabled(c, h.DB) {
		if v, _ := c.Get("branchActiveId"); v != nil && v.(uint) > 0 {
			q = q.Where("(customer.branch_id = ? OR customer.branch_id IS NULL)", v.(uint))
		} else if v, _ := c.Get("branchIds"); v != nil && len(v.([]uint)) > 0 {
			q = q.Where("(customer.branch_id IN ? OR customer.branch_id IS NULL)", v.([]uint))
		}
	}
	phoneCond := "phone LIKE ? OR other_phone LIKE ?"
	phoneArgs := []interface{}{like, like}
	if len(keyword) >= 10 && len(keyword) <= 11 && isNumeric(keyword) {
		phoneFragment := "%" + keyword[len(keyword)-9:] + "%"
		phoneArgs = []interface{}{phoneFragment, phoneFragment}
	}
	q = q.Where("fullname LIKE ? OR code LIKE ? OR old_code LIKE ? OR nickname LIKE ? OR "+phoneCond,
		append([]interface{}{like, like, like, like}, phoneArgs...)...)
	q, err := applyTelesaleScope(c, h.DB, q)
	if err != nil {
		return nil, err
	}
	return q.Order("id DESC").Limit(50), nil
}

// Gate search-global: CHỈ phạm vi đúng 1 chi nhánh mới đổi SQL; nhiều CN / cross-branch / không
// giới hạn ⇒ SQL giống TỪNG KÝ TỰ bản cũ.
func TestSearchGlobalQuery_GateAndShape(t *testing.T) {
	db := dryRunDB(t)
	h := New(Deps{DB: db})
	var items []map[string]interface{}

	for _, kw := range []string{"Nguyễn", "zzqq", "0959500000", "09", "KH0001000"} {
		for _, tc := range []struct {
			name   string
			active uint
			ids    []uint
			cross  bool
			split  bool
		}{
			{"active=7", 7, nil, false, true},
			{"branchIds=[9]", 0, []uint{9}, false, true},
			{"branchIds=[1,2]", 0, []uint{1, 2}, false, false},
			{"không giới hạn CN", 0, nil, false, false},
			{"active=7 + cross-branch bật", 7, nil, true, false},
		} {
			mk := func() *gin.Context {
				c := ctxWithScope(t, "", tc.active, tc.ids)
				c.Set("_opt_cross_branch", tc.cross) // khoá cache của tenant.IsCrossBranchEnabled
				return c
			}
			oldQ, err := oldSearchGlobalQuery(h, mk(), kw)
			if err != nil {
				t.Fatal(err)
			}
			newQ, err := h.searchGlobalQuery(mk(), kw)
			if err != nil {
				t.Fatal(err)
			}
			oldSQL := explainSQL(db, oldQ.Find(&items))
			newSQL := explainSQL(db, newQ.Find(&items))
			if !tc.split {
				if oldSQL != newSQL {
					t.Errorf("[%s %q] phạm vi KHÔNG phải 1 CN mà SQL đổi:\ncũ: %s\nmới: %s", tc.name, kw, oldSQL, newSQL)
				}
				continue
			}
			oldWhere := whereOf(oldSQL)
			b := "7"
			if tc.active == 0 {
				b = "9"
			}
			for _, part := range []string{
				"SELECT id, fullname, nickname, code, phone FROM `customer` WHERE customer.id IN (SELECT customer_page.id FROM (",
				"(SELECT customer.id FROM `customer` WHERE " + oldWhere + " AND customer.branch_id = " + b + " ORDER BY customer.id DESC LIMIT 50)",
				"(SELECT customer.id FROM `customer` WHERE " + oldWhere + " AND customer.branch_id IS NULL ORDER BY customer.id DESC LIMIT 50)",
				"ORDER BY id DESC LIMIT 50 OFFSET 0) AS customer_page) ORDER BY customer.id DESC",
			} {
				if !strings.Contains(newSQL, part) {
					t.Errorf("[%s %q] câu tách thiếu %q:\n%s", tc.name, kw, part, newSQL)
				}
			}
		}
	}
}
