//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Lưới RUNTIME cho bản tách câu đếm khỏi câu danh sách (story hiệu năng 2026-09-07).
//
// `list_query_test.go` chạy DryRun: nó chứng minh HÌNH DẠNG SQL đúng nhưng KHÔNG thực thi câu
// nào. Phần còn thiếu chính là phần dễ vỡ nhất và đã có tiền lệ trong repo: tái dùng một
// `*gorm.DB` cho hai truy vấn (đếm rồi liệt kê). File này chạy thật trên MySQL dùng-một-lần.
//
// Điều được khoá: `meta.totalCount` phải bằng SỐ DÒNG THẬT khớp bộ lọc — không phải số dòng của
// trang. Trước bản vá, tổng do `pagination.Paginate` tự đếm; nay handler tự đếm bằng câu riêng,
// nên một sai lệch phạm vi giữa hai câu sẽ hiện ra ở ĐÚNG con số này và không ở đâu khác.

func callListCustomer(t *testing.T, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/crm/customer?"+rawQuery, nil)
	c.Set("branchActiveId", listBranch)
	c.Set("userID", uint(1))
	h := &Handler{DB: testDB}
	h.ListCustomer(c)
	return w
}

type listBody struct {
	Items []map[string]any `json:"items"`
	Meta  struct {
		TotalCount  int64 `json:"totalCount"`
		PageCount   int   `json:"pageCount"`
		CurrentPage int   `json:"currentPage"`
		PerPage     int   `json:"perPage"`
	} `json:"_meta"`
}

func decodeList(t *testing.T, w *httptest.ResponseRecorder) listBody {
	t.Helper()
	require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
	var b listBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b), "body: %s", w.Body.String())
	return b
}

// listBranch / listBranchKhac — chi nhánh active của các lời gọi list và chi nhánh "nhiễu".
//
// Story-35 (D15 — 1 thương hiệu/DB): nhiễu dùng CHI NHÁNH (vế scope duy nhất còn lại) — bất biến
// cần khoá: câu đếm và câu danh sách cùng một phạm vi.
const (
	listBranch     = uint(7001)
	listBranchKhac = uint(7002)
)

// seedListCustomer chèn 1 khách ở bước mặc định. `status` để DEFAULT của cột (=1, trước story-04 bị
// ẩn khỏi list) — danh sách nay hiện mọi khách (FR17), tổng phải đếm cả khách này.
func seedListCustomer(t *testing.T, branchID uint, name, phone string) {
	t.Helper()
	row := map[string]any{
		"fullname":   name,
		"phone":      phone,
		"branch_id":  branchID,
		"stage_id":   defaultStageID(t),
		"created_at": 1,
		"updated_at": 1,
	}
	require.NoError(t, testDB.Table("customer").Create(row).Error)
}

func TestListCustomer_TotalMatchesFilterScope(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	// 25 khách của chi nhánh active → nhiều hơn 1 trang (per-page=15) để phân biệt "tổng" với "số dòng trang".
	for i := 0; i < 25; i++ {
		seedListCustomer(t, listBranch, fmt.Sprintf("Khach List %02d", i), fmt.Sprintf("090000%04d", i))
	}
	// Nhiễu: khách của CHI NHÁNH khác KHÔNG được lọt vào tổng.
	for i := 0; i < 9; i++ {
		seedListCustomer(t, listBranchKhac, fmt.Sprintf("Khach CN Khac %02d", i), fmt.Sprintf("091111%04d", i))
	}

	// Story-04: list không còn lọc ngầm `status` ⇒ khách CHƯA gán chi nhánh (luôn thuộc phạm vi) mà
	// test khác trong package để lại (trước story-04 bị ẩn theo `status` cũ — cột đã bỏ ở story-12)
	// nay được đếm. Kỳ vọng = 25 khách seed + số khách NULL-branch hiện có, đếm bằng SQL ĐỘC LẬP.
	var nullBranch int64
	require.NoError(t, testDB.Raw("SELECT COUNT(*) FROM customer WHERE branch_id IS NULL").Scan(&nullBranch).Error)
	want := 25 + nullBranch
	t.Logf("khách NULL-branch có sẵn trong DB test: %d", nullBranch)

	t.Run("tổng = số dòng khớp bộ lọc, KHÔNG phải số dòng của trang", func(t *testing.T) {
		b := decodeList(t, callListCustomer(t, "page=1&per-page=15"))
		require.Len(t, b.Items, 15, "trang 1 phải đủ 15 dòng")
		require.EqualValues(t, want, b.Meta.TotalCount,
			"⛔ tổng phải là 25 + khách NULL-branch (chỉ chi nhánh active) — lệch nghĩa là câu đếm và câu danh sách khác phạm vi")
		require.Equal(t, int((want+14)/15), b.Meta.PageCount)
	})

	t.Run("trang 2 giữ nguyên tổng và trả phần dư", func(t *testing.T) {
		b := decodeList(t, callListCustomer(t, "page=2&per-page=15"))
		require.Len(t, b.Items, int(min(15, want-15)))
		require.EqualValues(t, want, b.Meta.TotalCount)
	})

	// Bộ lọc phải áp cho CẢ hai câu. Nếu chỉ áp cho câu danh sách, tổng sẽ vẫn là 25.
	t.Run("lọc theo từ khoá: tổng phải giảm theo", func(t *testing.T) {
		b := decodeList(t, callListCustomer(t, "page=1&per-page=15&_keyword="+url.QueryEscape("List 07")))
		require.Len(t, b.Items, 1)
		require.EqualValues(t, 1, b.Meta.TotalCount,
			"⛔ từ khoá không áp vào câu đếm ⇒ tổng sai (danh sách 1 dòng nhưng báo 25 trang)")
	})

	// Ca đi ĐƯỜNG NGRAM (từ khoá có chữ số, ≥4 ký tự) — đường LIKE và đường ngram phải cho
	// cùng một kết quả, cả số dòng lẫn tổng.
	t.Run("đường ngram và đường LIKE cho cùng kết quả", func(t *testing.T) {
		const kw = "0900000007" // có chữ số, 10 ký tự ⇒ qua cổng ngram
		require.True(t, customerKeywordUseNgram(kw), "ca này phải đi ngram, nếu không test vô nghĩa")

		// ⚠ Chống xanh giả: thiếu chỉ mục thì handler tự rơi về LIKE và HAI VẾ VẪN BẰNG NHAU,
		// nên phải xác nhận chỉ mục có thật — nếu không, subtest này không chứng minh được gì.
		var ftCount int64
		require.NoError(t, testDB.Raw(`SELECT COUNT(*) FROM information_schema.STATISTICS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'customer'
			  AND INDEX_NAME = 'ft_customer_ngram'`).Scan(&ftCount).Error)
		require.EqualValues(t, 1, ftCount,
			"DB test thiếu ft_customer_ngram ⇒ handler rơi về LIKE ⇒ so sánh dưới vô nghĩa")

		ngram := decodeList(t, callListCustomer(t, "page=1&per-page=15&_keyword="+url.QueryEscape(kw)))

		// Đường lui (forceLike) qua chính hàm sản phẩm, không dựng lại query.
		var likeTotal int64
		require.NoError(t, h4Test().buildCustomerFilters(listCtx(t, "_keyword="+url.QueryEscape(kw)), true).
			Count(&likeTotal).Error)

		require.EqualValues(t, likeTotal, ngram.Meta.TotalCount,
			"⛔ đường ngram trả tổng khác đường LIKE — MATCH đang sót hoặc thừa dòng")
		require.Len(t, ngram.Items, int(likeTotal))
	})

	// Không tìm thấy gì ⇒ 0 dòng, KHÔNG phải trả nguyên danh sách.
	t.Run("từ khoá không khớp → 0 dòng, tổng 0", func(t *testing.T) {
		b := decodeList(t, callListCustomer(t, "page=1&per-page=15&_keyword="+url.QueryEscape("zzqqxx9999")))
		require.Len(t, b.Items, 0)
		require.EqualValues(t, 0, b.Meta.TotalCount,
			"⛔ không khớp mà tổng khác 0 = điều kiện tìm kiếm bị bỏ qua")
	})
}

// h4Test / listCtx — helper nhỏ để gọi hàm sản phẩm với đúng context tenant.
func h4Test() *Handler { return &Handler{DB: testDB} }

func listCtx(t *testing.T, rawQuery string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/crm/customer?"+rawQuery, nil)
	c.Set("branchActiveId", listBranch)
	c.Set("userID", uint(1))
	return c
}
