//go:build integration

package handler

import (
	"encoding/json"
	"log"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"

	"erpCluvix/internal/testhelpers"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaAll)
	if err != nil {
		log.Printf("testhelpers: %v — skipping integration tests", err)
		os.Exit(m.Run())
	}
	defer s.Close()
	testDB = s.DB
	os.Exit(m.Run())
}

// seedCustomer chèn 1 khách hàng (không gán chi nhánh) và trả id.
func seedCustomer(t *testing.T, name string) uint {
	t.Helper()
	row := map[string]any{
		"fullname":   name,
		"stage_id":   testhelpers.MustDefaultCustomerStageID(t, testDB),
		"created_at": 1,
		"updated_at": 1,
	}
	require.NoError(t, testDB.Table("customer").Create(row).Error)
	var id uint
	// GORM Create từ map KHÔNG backfill id → đọc lại theo fullname.
	require.NoError(t, testDB.Table("customer").Where("fullname = ?", name).
		Order("id DESC").Limit(1).Pluck("id", &id).Error)
	require.NotZero(t, id)
	return id
}

func callGetCustomer(t *testing.T, id uint) *httptest.ResponseRecorder {
	t.Helper()
	return callGetCustomerRaw(t, strconv.FormatUint(uint64(id), 10))
}

// callGetCustomerRaw nhận param id ở dạng CHUỖI THÔ để test được cả id không phải số.
func callGetCustomerRaw(t *testing.T, rawID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// URL phải escape (httptest.NewRequest panic với khoảng trắng thô), nhưng handler đọc
	// c.Params — giữ nguyên giá trị THÔ ở đó, đúng như gin trả về sau khi decode path.
	c.Request = httptest.NewRequest("GET", "/crm/customer/"+url.PathEscape(rawID), nil)
	c.Params = gin.Params{{Key: "id", Value: rawID}}
	c.Set("userID", uint(1))
	h := &Handler{DB: testDB}
	h.GetCustomer(c)
	return w
}

// B2 — GET theo id đi cùng phạm vi với UPDATE (ScopedDBNullBranch). Story-62: DB chỉ phục vụ 1
// thương hiệu, không còn khoá thương hiệu trong session/bảng — session không có phạm vi chi nhánh
// vẫn mở được khách chưa gán chi nhánh.
func TestGetCustomer_MoDuocKhachChuaGanChiNhanh(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	id := seedCustomer(t, "Khach Get Scope A")
	w := callGetCustomer(t, id)
	require.Equal(t, 200, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.NotNil(t, body["data"])
}

// Param id KHÔNG phải số phải 404 chứ không được rơi vào ép kiểu của MySQL.
// Trước bản vá, `Where("customer.id = ?", c.Param("id"))` bind chuỗi thô ⇒ MySQL cắt
// "…abc" thành số (warning 1292 bị nuốt) ⇒ `/crm/customer/<id>abc` trả về đúng khách <id>.
func TestGetCustomer_NonNumericIDIsRejected(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	id := seedCustomer(t, "Khach Kiem Tra Id Rac")
	numeric := strconv.FormatUint(uint64(id), 10)

	// Đối chứng: đúng id thì PHẢI 200 — nếu ca này hỏng thì mọi assert 404 dưới đây vô nghĩa.
	require.Equal(t, 200, callGetCustomerRaw(t, numeric).Code,
		"đối chứng: id hợp lệ phải mở được, nếu không thì test 404 bên dưới không chứng minh gì")

	for _, raw := range []string{numeric + "abc", numeric + " ", "abc", "", "0", "-1", " " + numeric} {
		t.Run("id="+strconv.Quote(raw), func(t *testing.T) {
			require.Equal(t, 404, callGetCustomerRaw(t, raw).Code,
				"id không parse được ra số dương phải 404, KHÔNG được để MySQL tự ép kiểu")
		})
	}
}
