//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/storage"
)

// Lưới PARITY cho bản vá C-4 (list_branch_split.go): ở phạm vi 1 chi nhánh, câu trang tách
// UNION ALL + câu COUNT gắn hint PHẢI trả về y hệt bản cũ — từng byte của response (thứ tự dòng,
// cột, tổng, header phân trang), với mọi bộ lọc và cả 4 dạng phạm vi.
//
// "Bản cũ" = `oldListCustomerBody` dưới đây: chép NGUYÊN thân ListCustomer trước bản vá (đếm
// bằng `Count` trần, trang bằng `PaginateWithTotal` trên câu 1 khối) — CỐ Ý chép, vì đây là chuẩn
// đối chiếu đã đóng băng, không phải hàm sản phẩm.
//
// ⚠ Chống xanh giả: nếu cổng không bao giờ mở thì hai vế cùng đi đường cũ và luôn bằng nhau ⇒
// test đếm số ca THỰC SỰ đi đường tách và đòi > 0; thêm vào đó test gọi THẲNG
// customerListSplitPage ở mọi offset (bỏ qua cổng) để phủ cả trang sâu.

const (
	splitBranchA = uint(7101) // chi nhánh của nhân viên 1 chi nhánh
	splitBranchB = uint(7102) // chi nhánh khác — KHÔNG được lọt vào phạm vi A
)

// splitScope dựng phạm vi tenant lên context theo đúng các khoá middleware ghi.
type splitScope struct {
	name   string
	active uint
	ids    []uint
	single bool // customerListSingleBranch kỳ vọng
}

var splitScopes = []splitScope{
	{name: "active=A", active: splitBranchA, single: true},
	{name: "branchIds=[A]", ids: []uint{splitBranchA}, single: true},
	{name: "branchIds=[A,B]", ids: []uint{splitBranchA, splitBranchB}},
	{name: "không giới hạn CN", single: false},
}

func splitCtx(rawQuery string, s splitScope) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/crm/customer?"+rawQuery, nil)
	if s.active > 0 {
		c.Set("branchActiveId", s.active)
	}
	if len(s.ids) > 0 {
		c.Set("branchIds", s.ids)
	}
	c.Set("userID", uint(1))
	return c, w
}

// oldListCustomerBody — thân ListCustomer TRƯỚC bản vá C-4 (đường không-export), đóng băng.
func oldListCustomerBody(h *Handler, c *gin.Context) {
	base := h.buildCustomerFilters(c, false)
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		pagination.FailQuery(c, err)
		return
	}
	q := customerListPresentation(base.Session(&gorm.Session{}), customerListSelect)
	var items []map[string]interface{}
	result := pagination.PaginateWithTotal(c, q, &items, total)
	phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), items)
	storage.ResolveRows(items, "avatar")
	c.JSON(http.StatusOK, result)
}

// splitStages — 4 bước seed (khoá `name`, migration 20261102095255) đủ 3 loại: 2 đang mở, 1 thành
// công, 1 thất bại. Dùng cho seed + filter[stage_id] của các test parity bên dưới.
func splitStages(t *testing.T) []int {
	t.Helper()
	return []int{stageIDByName(t, "Liên hệ"), stageIDByName(t, "Đang tư vấn"),
		stageIDByName(t, "Khách hàng đã thuê"), stageIDByName(t, "Từ chối")}
}

// seedSplitCustomers — khách xen kẽ A / NULL / B theo id (để hợp 2 nhánh phải trộn thật),
// đủ 4 bước (3 loại), giới tính, ngày tạo, tên, SĐT. Không seed `customer.status` (cột bỏ ở
// story-12): danh sách không còn lọc theo status nên không cần phân phối status để chứng minh.
func seedSplitCustomers(t *testing.T) (createdFrom, createdTo int64) {
	t.Helper()
	stages := splitStages(t)
	names := []string{"Nguyen Van", "Tran Thi", "Le Minh"}
	const baseDay = int64(1788220800)
	for i := 0; i < 84; i++ {
		var branch any
		switch i % 4 {
		case 0, 1:
			branch = splitBranchA
		case 2:
			branch = nil // khách chưa gán chi nhánh — LUÔN thuộc phạm vi
		default:
			branch = splitBranchB
		}
		row := map[string]any{
			"fullname":    fmt.Sprintf("%s C4 %02d", names[i%len(names)], i),
			"phone":       fmt.Sprintf("0977%06d", 400000+i),
			"branch_id":   branch,
			"stage_id":    stages[(i/3)%len(stages)], // (i/3)%4 lệch pha với i%4 ⇒ chi nhánh A có đủ 4 bước
			"gender":      i % 2,
			"created_day": baseDay + int64(i)*86400,
			"created_at":  1,
			"updated_at":  1,
		}
		require.NoError(t, testDB.Table("customer").Create(row).Error)
	}
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer WHERE phone LIKE '0977%'") })
	return baseDay + 10*86400, baseDay + 60*86400
}

func TestListCustomer_BranchSplit_ParityWithOldQuery(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	from, to := seedSplitCustomers(t)

	// Ngram phải thật sự chạy, không thì ca từ khoá số chỉ là LIKE đội lốt.
	var ftCount int64
	require.NoError(t, testDB.Raw(`SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer' AND INDEX_NAME = 'ft_customer_ngram'`).
		Scan(&ftCount).Error)
	require.EqualValues(t, 1, ftCount, "DB test thiếu ft_customer_ngram ⇒ ca ngram vô nghĩa")

	stages := splitStages(t)
	filters := []string{
		"",
		fmt.Sprintf("filter[stage_id][0]=%d", stages[1]),
		fmt.Sprintf("filter[stage_id][0]=%d&filter[stage_id][1]=%d", stages[2], stages[3]),
		fmt.Sprintf("filter[branch_id][0]=%d", splitBranchA),
		fmt.Sprintf("filter[branch_id][0]=%d", splitBranchB),
		"filter[gender][0]=1",
		fmt.Sprintf("filter[created_day][gte]=%d&filter[created_day][lte]=%d", from, to),
		"_keyword=" + url.QueryEscape("Nguyen"),     // đường LIKE
		"_keyword=" + url.QueryEscape("0977400042"), // đường ngram (có số, ≥4 ký tự)
		"_keyword=" + url.QueryEscape("zzqq"),       // không khớp
		"_keyword=" + url.QueryEscape("   "),        // toàn khoảng trắng = không từ khoá
	}
	pages := []string{"page=1&per-page=15", "page=1&per-page=3", "page=2&per-page=3",
		"page=4&per-page=3", "page=7&per-page=3", "page=40&per-page=3", "page=0&per-page=500"}

	h := &Handler{DB: testDB}
	splitTaken := 0
	for _, s := range splitScopes {
		for _, f := range filters {
			for _, p := range pages {
				qs := p
				if f != "" {
					qs = f + "&" + p
				}
				cNew, wNew := splitCtx(qs, s)
				h.ListCustomer(cNew)
				cOld, wOld := splitCtx(qs, s)
				oldListCustomerBody(h, cOld)

				require.Equal(t, http.StatusOK, wNew.Code, "[%s] %s: %s", s.name, qs, wNew.Body.String())
				require.Equal(t, wOld.Body.String(), wNew.Body.String(),
					"⛔ [%s] %s: response khác bản cũ (dòng/thứ tự/tổng)", s.name, qs)
				for _, hk := range []string{"X-Pagination-Total-Count", "X-Pagination-Page-Count",
					"X-Pagination-Current-Page", "X-Pagination-Per-Page"} {
					require.Equal(t, wOld.Header().Get(hk), wNew.Header().Get(hk), "[%s] %s header %s", s.name, qs, hk)
				}

				_, single := customerListSingleBranch(cNew)
				require.Equal(t, s.single, single, "[%s] nhận diện phạm vi 1 chi nhánh sai", s.name)
				if single {
					var total int64
					require.NoError(t, customerCountQuery(h.buildCustomerFilters(cNew, false), &total).Error)
					_, perPage, offset := pagination.PageParams(cNew)
					if customerListSplitWorthIt(total, offset, perPage) {
						splitTaken++
					}
				}
			}
		}
	}
	require.Greater(t, splitTaken, 20, "⛔ đường UNION gần như không được chạy ⇒ parity ở trên không chứng minh gì")
	t.Logf("số ca đi đường UNION: %d", splitTaken)
}

// Gọi THẲNG câu UNION ở mọi offset (kể cả vượt tổng — chỗ cổng không cho tới) và so id + thứ tự
// với câu 1 khối cũ.
func TestCustomerListSplitPage_AllOffsetsMatchOldQuery(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	seedSplitCustomers(t)
	h := &Handler{DB: testDB}
	stageFilter := fmt.Sprintf("filter[stage_id][0]=%d", splitStages(t)[3])

	for _, s := range splitScopes[:2] {
		for _, f := range []string{"", stageFilter, "filter[gender][0]=0", "_keyword=Tran"} {
			c, _ := splitCtx(f, s)
			branchID, single := customerListSingleBranch(c)
			require.True(t, single)
			base := h.buildCustomerFilters(c, false)

			var oldAll []uint
			require.NoError(t, base.Session(&gorm.Session{}).Order("customer.id DESC").Pluck("customer.id", &oldAll).Error)
			require.NotEmpty(t, oldAll, "[%s] %s: dữ liệu seed không khớp — ca này vô nghĩa", s.name, f)
			var hasNull int64
			require.NoError(t, base.Session(&gorm.Session{}).Where("customer.branch_id IS NULL").Count(&hasNull).Error)
			require.Positive(t, hasNull, "[%s] %s: thiếu khách NULL-branch ⇒ nhánh IS NULL không được thử", s.name, f)

			for _, perPage := range []int{1, 3, 7} {
				for offset := 0; offset <= len(oldAll)+perPage; offset++ {
					var rows []map[string]any
					require.NoError(t, customerListSplitPage(testDB, base, branchID, offset, perPage).
						Find(&rows).Error)
					newIDs := make([]uint, 0, len(rows))
					for _, r := range rows {
						newIDs = append(newIDs, uint(helpers.ToInt64(r["id"])))
					}
					end := offset + perPage
					if end > len(oldAll) {
						end = len(oldAll)
					}
					want := []uint{}
					if offset < len(oldAll) {
						want = oldAll[offset:end]
					}
					require.Equal(t, want, newIDs, "[%s] %s offset=%d perPage=%d", s.name, f, offset, perPage)
				}
			}
		}
	}
}

// Câu COUNT (story-04: KHÔNG còn hint) chạy trên MySQL thật: không cảnh báo nào (trước đây 3128/3126
// khi chỉ mục trong hint bị đổi tên/xoá), đúng con số của Count trần, và đếm MỌI khách trong phạm vi
// bất kể `status` cũ (FR17). Giữ tên test cũ vì AC6 story-04 dẫn chiếu tên này.
func TestCustomerCountQuery_HintMatchesPlainCount(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	seedSplitCustomers(t)
	h := &Handler{DB: testDB}
	c, _ := splitCtx("", splitScopes[0])
	base := h.buildCustomerFilters(c, false)

	var counted, plain int64
	require.NoError(t, customerCountQuery(base, &counted).Error)
	// Câu thật đi prepared statement (EXECUTE xoá vùng chẩn đoán) nên chạy lại đúng câu đó dạng
	// TEXT (đã nội suy tham số, COM_QUERY); suite chạy 1 kết nối (MaxOpenConns=1) ⇒ SHOW WARNINGS
	// đọc đúng câu vừa chạy.
	orig := applyTelesaleScope
	applyTelesaleScope = func(_ *gin.Context, _ *gorm.DB, q *gorm.DB) (*gorm.DB, error) { return q, nil }
	dry := testDB.Session(&gorm.Session{DryRun: true})
	var dummy int64
	stmt := customerCountQuery((&Handler{DB: dry}).buildCustomerFilters(c, false), &dummy).Statement
	applyTelesaleScope = orig
	countText := testDB.Dialector.Explain(stmt.SQL.String(), stmt.Vars...)
	require.NotContains(t, countText, "/*+", "COUNT không được mang optimizer hint (gỡ ở story-04)")
	require.NotContains(t, countText, "status", "COUNT không được lọc ngầm theo status (FR17)")
	var textCount int64
	require.NoError(t, testDB.Raw(countText).Scan(&textCount).Error)
	var warns []map[string]any
	require.NoError(t, testDB.Raw("SHOW WARNINGS").Scan(&warns).Error)
	require.Empty(t, warns, "⛔ câu COUNT sinh cảnh báo")
	require.Equal(t, counted, textCount)
	require.NoError(t, base.Session(&gorm.Session{}).Count(&plain).Error)
	require.Equal(t, plain, counted)

	// Đối chiếu độc lập (SQL tay, không qua hàm sản phẩm): mọi khách seed thuộc phạm vi A (A + NULL).
	// Seed khác trong package có thể cùng phạm vi ⇒ so trên tập SĐT seed. (Vế "kể cả status 1/2/4"
	// bỏ ở story-12 cùng cột `customer.status`.)
	var seedInScope, seedCounted int64
	require.NoError(t, testDB.Raw(`SELECT COUNT(*) FROM customer WHERE phone LIKE '0977%'
		AND (branch_id = ? OR branch_id IS NULL)`, splitBranchA).Scan(&seedInScope).Error)
	require.Positive(t, seedInScope, "tiền đề: seed phải có khách trong phạm vi A")
	require.NoError(t, base.Session(&gorm.Session{}).Where("customer.phone LIKE '0977%'").Count(&seedCounted).Error)
	require.Equal(t, seedInScope, seedCounted, "⛔ COUNT danh sách bỏ sót khách trong phạm vi")
}

// ── search-global (bench C-4 vòng 2) ──────────────────────────────────────────────────────

// oldSearchGlobalBody — thân SearchGlobal TRƯỚC bản vá, đóng băng (câu SQL: oldSearchGlobalQuery
// ở list_branch_split_test.go).
func oldSearchGlobalBody(h *Handler, c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if utf8.RuneCountInString(keyword) < searchGlobalMinRunes {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}
	q, err := oldSearchGlobalQuery(h, c, keyword)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	var items []map[string]interface{}
	if tx := q.Find(&items); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), items)
	c.JSON(http.StatusOK, items)
}

func TestSearchGlobal_BranchSplit_ParityWithOldQuery(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	seedSplitCustomers(t)
	h := &Handler{DB: testDB}

	keywords := []string{
		"C4",            // khớp cả 84 khách seed ⇒ > 50 dòng trong phạm vi A+NULL: thử trần LIMIT 50 khi trộn 2 nhánh
		"Nguyen",        // khớp vừa
		"Tran Thi C4 1", // khớp ít
		"0977400042",    // SĐT 10 số ⇒ nhánh 9 số cuối; khách NULL-branch
		"0977400003",    // SĐT của khách chi nhánh B ⇒ phạm vi A phải ra rỗng
		"977400",        // số ngắn, khớp nhiều
		"zzqq",          // không khớp — ca xấu nhất (quét)
		"ab",            // 2 ký tự, không khớp
		"C",             // dưới ngưỡng ⇒ []
	}
	taken := 0
	for _, s := range splitScopes {
		for _, kw := range keywords {
			qs := "keyword=" + url.QueryEscape(kw)
			cNew, wNew := splitCtx(qs, s)
			h.SearchGlobal(cNew)
			cOld, wOld := splitCtx(qs, s)
			oldSearchGlobalBody(h, cOld)

			require.Equal(t, http.StatusOK, wNew.Code, "[%s] %q: %s", s.name, kw, wNew.Body.String())
			require.Equal(t, wOld.Body.String(), wNew.Body.String(), "⛔ [%s] %q: body khác bản cũ", s.name, kw)
			if s.single && kw != "C" {
				taken++
			}
		}
	}
	require.Greater(t, taken, 10)

	// Chống xanh giả: ca "C4" phạm vi A phải THẬT SỰ cắt ở 50 dòng và có cả khách NULL-branch.
	c, w := splitCtx("keyword=C4", splitScopes[0])
	h.SearchGlobal(c)
	var items []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	require.Len(t, items, searchGlobalLimit)
	var inScope int64
	require.NoError(t, testDB.Table("customer").Where("phone LIKE '0977%' AND (branch_id = ? OR branch_id IS NULL)", splitBranchA).
		Count(&inScope).Error)
	require.Greater(t, inScope, int64(searchGlobalLimit), "seed phải vượt trần 50 trong phạm vi A")
}
