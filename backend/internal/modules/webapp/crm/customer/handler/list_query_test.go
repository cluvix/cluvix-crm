package handler

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// nopConnector — connector không bao giờ mở kết nối thật. `sql.OpenDB` LƯỜI (không dial lúc gọi),
// GORM ở DryRun dựng SQL rồi dừng ⇒ test chạy mọi máy/CI mà KHÔNG đụng database nào.
// (Lặp khuôn `crm/reminder/handler/list_query_test.go`.)
type nopConnector struct{}

func (nopConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("dry run: test này không được phép mở kết nối")
}
func (nopConnector) Driver() driver.Driver { return nil }

func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	// DryRun không đọc được user_profile ⇒ scope telesale (fail-closed) sẽ chặn query; test này chỉ
	// soi mệnh đề WHERE của bộ lọc nên cho scope đi qua. Scope có test riêng ở crm/customer/scope.
	orig := applyTelesaleScope
	applyTelesaleScope = func(_ *gin.Context, _ *gorm.DB, q *gorm.DB) (*gorm.DB, error) { return q, nil }
	t.Cleanup(func() { applyTelesaleScope = orig })
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sql.OpenDB(nopConnector{}),
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("mở DryRun DB: %v", err)
	}
	return db
}

func custCtx(t *testing.T, rawQuery string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/crm/customer?"+rawQuery, nil)
	return c
}

// Hai helper dưới chạy CHÍNH các hàm mà ListCustomer dùng — KHÔNG dựng lại query trong test.
// Dựng lại là tự chép, và bản chép vẫn xanh khi code sản phẩm bị cấy lỗi.
func custCountSQL(t *testing.T, h *Handler, c *gin.Context) string {
	t.Helper()
	var total int64
	base := h.buildCustomerFilters(c, false)
	return customerCountQuery(base, &total).Statement.SQL.String()
}

func custListSQL(t *testing.T, h *Handler, c *gin.Context) string {
	t.Helper()
	var items []map[string]interface{}
	base := h.buildCustomerFilters(c, false)
	return customerListPresentation(base.Session(&gorm.Session{}), customerListSelect).
		Limit(15).Find(&items).Statement.SQL.String()
}

// whereOf cắt lấy phần WHERE… để so hai câu. Trả "" khi không có WHERE (⇒ test tự đỏ).
func whereOf(sqlText string) string {
	i := strings.Index(sqlText, " WHERE ")
	if i < 0 {
		return ""
	}
	w := sqlText[i+len(" WHERE "):]
	if j := strings.Index(w, " ORDER BY "); j >= 0 {
		w = w[:j]
	}
	if j := strings.Index(w, " LIMIT "); j >= 0 {
		w = w[:j]
	}
	return strings.TrimSpace(w)
}

// ⚠ ĐÃ THỬ VÀ BỎ — đừng thêm lại (lý lẽ đã ghi ở `crm/reminder/handler/list_query_test.go`):
// "COUNT không có ORDER BY" và "COUNT không mang danh sách cột" đều do GORM `.Count()` tự bảo
// đảm, nên chúng xanh kể cả khi câu đếm được cho nhận thẳng query danh sách — an toàn giả.
// (story-10 crm-standalone: test "COUNT không join broker" đã xoá cùng JOIN broker của danh sách.)

// Câu đếm và câu danh sách BẮT BUỘC cùng phạm vi lọc. Lệch phạm vi = tổng số trang sai mà không
// có lỗi nào — đúng lớp lỗi đã ghi ở `perf-list-endpoints.md` (§câu baseline phải cùng phạm vi lọc).
func TestCustomerCountAndListShareSameWhere(t *testing.T) {
	h := New(Deps{DB: dryRunDB(t)})

	for _, qs := range []string{
		"", "filter[gender]=1", "_keyword=0913353112", "_keyword=Nguyen",
		"_ageFrom=20&_ageTo=40", "filter[branch_id]=3",
	} {
		countW := whereOf(custCountSQL(t, h, custCtx(t, qs)))
		listW := whereOf(custListSQL(t, h, custCtx(t, qs)))
		// qs rỗng + không phạm vi chi nhánh ⇒ story-04 bỏ vế `status IN` nên CẢ HAI câu không còn WHERE
		// (so sánh "" == "" vẫn bắt được câu danh sách lén mang thêm điều kiện).
		if (countW == "" || listW == "") && qs != "" {
			t.Fatalf("(%q) không tách được WHERE — helper hỏng, mọi so sánh sau đều vô nghĩa", qs)
		}
		if countW != listW {
			t.Errorf("(%q) COUNT và DANH SÁCH lệch phạm vi lọc:\nCOUNT: %s\nLIST : %s", qs, countW, listW)
		}
	}
}

// ⚠ Test trên dựng `base` MỚI cho mỗi câu, nên nó KHÔNG phủ thứ tự thật của handler: đếm trước,
// rồi dựng câu danh sách từ CHÍNH `base` đó. Tái dùng một `*gorm.DB` cho hai truy vấn là bẫy đã
// có tiền lệ trong repo (điều kiện của lượt trước dính sang lượt sau, hoặc ngược lại là mất
// điều kiện). Test này chạy đúng trình tự đó và đối chiếu với câu dựng độc lập.
func TestCustomerBaseReuseCountThenListKeepsFilters(t *testing.T) {
	h := New(Deps{DB: dryRunDB(t)})

	for _, qs := range []string{"", "_keyword=0913353112", "filter[gender]=1&filter[branch_id]=3"} {
		c := custCtx(t, qs)
		base := h.buildCustomerFilters(c, false)

		var total int64
		countSQL := base.Session(&gorm.Session{}).Count(&total).Statement.SQL.String()

		// Câu danh sách dựng SAU câu đếm, từ CÙNG base — đúng như ListCustomer làm.
		var items []map[string]interface{}
		listSQL := customerListPresentation(base.Session(&gorm.Session{}), customerListSelect).
			Limit(15).Find(&items).Statement.SQL.String()

		if whereOf(countSQL) != whereOf(listSQL) {
			t.Errorf("(%q) tái dùng base làm lệch bộ lọc:\nCOUNT: %s\nLIST : %s",
				qs, whereOf(countSQL), whereOf(listSQL))
		}
		// Câu danh sách sau khi tái dùng phải GIỐNG HỆT câu dựng từ base độc lập.
		if whereOf(listSQL) != whereOf(custListSQL(t, h, custCtx(t, qs))) {
			t.Errorf("(%q) câu danh sách dựng sau COUNT khác câu dựng độc lập:\n%s", qs, listSQL)
		}
	}
}

// Story-04 (customer-stage-pipeline): danh sách KHÔNG còn lọc ngầm theo `status` (FR17), filter tiến
// trình là `filter[stage_id]` (đa giá trị ⇒ IN), filter cũ `_status`/`status` bị bỏ qua, id rác ⇒ rỗng.
// Gọi CHÍNH custCountSQL/custListSQL (hàm sản phẩm), không dựng lại query.
func TestCustomerStageFilterShape(t *testing.T) {
	db := dryRunDB(t)
	h := New(Deps{DB: db})

	for _, tc := range []struct {
		qs, want string // want: mệnh đề WHERE đầy đủ (scope rỗng vì context không có chi nhánh)
	}{
		{"", ""},
		{"filter[stage_id][0]=2", "customer.stage_id = '2'"},
		{"filter[stage_id][0]=2&filter[stage_id][1]=5", "customer.stage_id IN ('2','5')"},
		{"filter[stage_id][in][0]=2&filter[stage_id][in][1]=5", "customer.stage_id IN ('2','5')"},
		{"filter[stage_id]=7", "customer.stage_id = '7'"},
		{"filter[stage_id][0]=2abc", "1 = 0"}, // id rác ⇒ không dòng nào (allowlist numeric id)
		{"filter[stage_id]=1,2", "1 = 0"},     // dấu phẩy KHÔNG phải cú pháp đa giá trị
		{"filter[_status][0]=12", ""},         // filter cũ: bỏ qua, KHÔNG còn lọc status
		{"filter[status]=12&filter[_status][1]=3", ""},
		{"filter[gender]=1", "customer.gender = '1'"}, // đối chứng: filter khác vẫn chạy
	} {
		var total int64
		var items []map[string]interface{}
		for name, sqlText := range map[string]string{
			"COUNT": explainSQL(db, customerCountQuery(h.buildCustomerFilters(custCtx(t, tc.qs), false), &total)),
			"LIST": explainSQL(db, customerListPresentation(h.buildCustomerFilters(custCtx(t, tc.qs), false),
				customerListSelect).Limit(15).Find(&items)),
		} {
			if got := whereOf(sqlText); got != tc.want {
				t.Errorf("(%q) %s WHERE = %q, muốn %q\n%s", tc.qs, name, got, tc.want, sqlText)
			}
			if strings.Contains(whereOf(sqlText), "status") {
				t.Errorf("(%q) %s còn lọc theo status:\n%s", tc.qs, name, sqlText)
			}
		}
	}
	if !strings.Contains(customerListSelect, "customer.stage_id") {
		t.Errorf("customerListSelect thiếu customer.stage_id (AC3)")
	}
}

// Cổng ngram: CHỈ từ khoá có chữ số VÀ ≥4 ký tự mới đi MATCH. Bảng số đo ở `list_search.go`.
func TestCustomerKeywordNgramGate(t *testing.T) {
	h := New(Deps{DB: dryRunDB(t)})

	useNgram := []string{"0913", "KH40", "09135", "KH4028", "0913353112", "120220330854830316"}
	likeOnly := []string{"09", "091", "KH4", "Nguy", "Nguyen", "Nguyễn", "Thị", "Trần"}

	for _, kw := range useNgram {
		sqlText := custCountSQL(t, h, custCtx(t, "_keyword="+kw))
		if !strings.Contains(sqlText, "MATCH(customer.search_ngram)") {
			t.Errorf("từ khoá %q PHẢI đi đường ngram (đo được nhanh hơn 2,4-5,4×):\n%s", kw, sqlText)
		}
	}
	for _, kw := range likeOnly {
		sqlText := custCountSQL(t, h, custCtx(t, "_keyword="+kw))
		if strings.Contains(sqlText, "MATCH(") {
			t.Errorf("từ khoá %q KHÔNG được đi ngram — đo được ngram thua LIKE 1,1-4,4× ở nhóm này:\n%s", kw, sqlText)
		}
	}
}

// ⚠ Ca nguy hiểm nhất của bản vá: MATCH khớp theo chuỗi TOKEN kề nhau bất kể ký tự phân cách, nên
// một mình nó trả THỪA dòng. `LIKE` là lưới lọc giữ kết quả đúng bằng đường cũ — bỏ nó đi thì
// danh sách trả thêm dòng mà không có lỗi nào. Phải luôn có mặt, kể cả khi đã đi ngram.
func TestCustomerNgramPathKeepsLikeFilter(t *testing.T) {
	h := New(Deps{DB: dryRunDB(t)})

	sqlText := custCountSQL(t, h, custCtx(t, "_keyword=0913353112"))
	if !strings.Contains(sqlText, "MATCH(customer.search_ngram)") {
		t.Fatalf("ca này lẽ ra phải đi ngram — test dưới trở nên vô nghĩa:\n%s", sqlText)
	}
	for _, col := range []string{"customer.phone LIKE", "customer.loai_message LIKE", "customer.link_message LIKE"} {
		if !strings.Contains(sqlText, col) {
			t.Errorf("đường ngram thiếu lưới lọc %q ⇒ trả THỪA dòng:\n%s", col, sqlText)
		}
	}
}

// forceLike (đường export + đường lui khi FULLTEXT hỏng) tuyệt đối không được sinh MATCH.
func TestCustomerForceLikeNeverUsesMatch(t *testing.T) {
	h := New(Deps{DB: dryRunDB(t)})
	var total int64

	for _, kw := range []string{"0913353112", "KH4028", "09135"} {
		c := custCtx(t, "_keyword="+kw)
		sqlText := h.buildCustomerFilters(c, true).Session(&gorm.Session{}).
			Count(&total).Statement.SQL.String()
		if strings.Contains(sqlText, "MATCH(") {
			t.Errorf("forceLike=true vẫn sinh MATCH cho %q:\n%s", kw, sqlText)
		}
		if !strings.Contains(sqlText, "customer.phone LIKE") {
			t.Errorf("forceLike=true mất luôn cả LIKE cho %q ⇒ không lọc gì:\n%s", kw, sqlText)
		}
	}
}

// customerKeySearch và cột sinh `search_ngram` PHẢI phủ cùng một tập cột. Thêm cột vào
// customerKeySearch mà quên thêm vào cột sinh (migration 20260907081100) ⇒ MATCH loại mất dòng
// TRƯỚC khi LIKE kịp nhìn thấy ⇒ SÓT IM LẶNG. Test khoá đúng danh sách đó.
func TestCustomerKeySearchMatchesNgramColumns(t *testing.T) {
	want := []string{
		"customer.phone", "customer.code", "customer.fullname", "customer.nickname",
		"customer.old_code", "customer.other_phone", "customer.link_message", "customer.loai_message",
	}
	if len(customerKeySearch) != len(want) {
		t.Fatalf("customerKeySearch đổi (%d cột) — cột sinh search_ngram phải đổi theo bằng "+
			"migration MỚI, rồi cập nhật danh sách này: %v", len(customerKeySearch), customerKeySearch)
	}
	got := make(map[string]bool, len(customerKeySearch))
	for _, c := range customerKeySearch {
		got[c] = true
	}
	for _, c := range want {
		if !got[c] {
			t.Errorf("thiếu %q trong customerKeySearch — hoặc cột sinh search_ngram đang phủ thừa", c)
		}
	}
}

// D2 (retro lần 2): scope telesale lỗi ⇒ list KHÔNG được chạy với bộ lọc hở — mọi câu dựng từ
// buildCustomerFilters phải mang lỗi (customerListTotal ⇒ 500) thay vì trả toàn bộ khách.
func TestCustomerFilters_ScopeError_FailClosed(t *testing.T) {
	h := New(Deps{DB: dryRunDB(t)})
	boom := errors.New("user_profile down")
	applyTelesaleScope = func(_ *gin.Context, _ *gorm.DB, q *gorm.DB) (*gorm.DB, error) { return q, boom }

	var total int64
	err := h.buildCustomerFilters(custCtx(t, ""), false).Session(&gorm.Session{}).Count(&total).Error
	if !errors.Is(err, boom) {
		t.Fatalf("scope lỗi mà Count không mang lỗi (fail-open): err=%v", err)
	}
}
