//go:build integration

package export

// Epic customer-stage-pipeline story-07 (AC5): cột "Tiến trình" của 3 export khách (danh sách khách,
// sinh nhật, convert) = TÊN bước hiện tại của khách (LEFT JOIN customer_stage), thay nhãn cứng theo
// tiến trình cũ. Khách ở bước ẨN vẫn ra tên; khách trỏ bước không tồn tại ⇒ ô trống, không mất dòng.

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"erpCluvix/internal/testhelpers"
	pkgExport "erpCluvix/pkg/export"
)

// exportSheet chạy fn (1 hàm Export*) và trả toàn bộ dòng của Sheet1 (dòng 0 = header).
func exportSheet(t *testing.T, fn func(c *gin.Context)) [][]string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x?_export=1", nil)
	c.Set("userID", uint(1))
	fn(c)
	require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
	var url string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &url), "body: %s", w.Body.String())
	path, ok := pkgExport.OwnerFilePath(pkgExport.DefaultDir, 1, filepath.Base(url))
	require.True(t, ok, "url=%s", url)
	f, err := excelize.OpenFile(path)
	require.NoError(t, err, "url=%s", url)
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows("Sheet1")
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	return rows
}

func TestExportCustomer_StageNameColumn(t *testing.T) {
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaAll)
	if err != nil {
		t.Fatalf("TEST_MYSQL_DSN chưa set/không dựng được schema — integration test này KHÔNG được skip: %v", err)
	}
	defer s.Close()
	db := s.DB
	SetDB(db)
	t.Cleanup(func() { SetDB(nil) })
	t.Chdir(t.TempDir())

	tn := testhelpers.FreshTenant(t, db)
	var stageID int
	require.NoError(t, db.Raw("SELECT id FROM customer_stage WHERE name = ?", "Đang đàm phán").Scan(&stageID).Error)
	require.NotZero(t, stageID, "seed bước 'Đang đàm phán' của migration create_customer_stage")
	// Bước ẨN vẫn phải ra tên.
	require.NoError(t, db.Exec("UPDATE customer_stage SET status = 0 WHERE id = ?", stageID).Error)
	t.Cleanup(func() { db.Exec("UPDATE customer_stage SET status = 1 WHERE id = ?", stageID) })
	require.NoError(t, db.Exec("UPDATE customer SET stage_id = ?, birthday = '1990-05-06' WHERE id = ?", stageID, tn.CustomerID).Error)
	b := tn.BranchID
	// Khách "chưa có bước": stage_id trỏ tới bước KHÔNG tồn tại (FK tắt trong Suite) ⇒ LEFT JOIN ra tên rỗng,
	// dòng không được mất. Không dùng NULL: cột sẽ NOT NULL ở story-12.
	noStage := testhelpers.CreateScopedCustomer(t, db, nil, &b)
	require.NoError(t, db.Exec("UPDATE customer SET stage_id = 999999 WHERE id = ?", noStage).Error)

	cases := []struct {
		name   string
		header string
		run    func(c *gin.Context, ids ...any)
	}{
		{"danh sách khách", "Tiến trình", func(c *gin.Context, ids ...any) {
			ExportCustomer(c, db.Table("customer").Where("customer.id IN ?", ids).Order("customer.id"))
		}},
		{"sinh nhật", "Tiến trình", func(c *gin.Context, ids ...any) {
			ExportCustomerBirthday(c, db.Table("customer").Where("customer.id IN ?", ids).Order("customer.id"))
		}},
		{"convert", "Tiến trình", func(c *gin.Context, ids ...any) {
			ExportCustomerConvert(c, db.Table("customer").Where("customer.id IN ?", ids).Order("customer.id"))
		}},
	}
	for _, tc := range cases {
		rows := exportSheet(t, func(c *gin.Context) { tc.run(c, tn.CustomerID, noStage) })
		col := -1
		for i, h := range rows[0] {
			if h == tc.header {
				col = i
			}
		}
		require.GreaterOrEqual(t, col, 0, "%s: không có cột %q: %v", tc.name, tc.header, rows[0])
		require.Len(t, rows, 3, "%s: phải đủ 2 khách (khách chưa có bước không được mất dòng): %v", tc.name, rows)
		require.Greater(t, len(rows[1]), col, "%s: dòng khách có bước thiếu ô: %v", tc.name, rows[1])
		require.Equal(t, "Đang đàm phán", rows[1][col], "%s: khách bước ẩn phải ra tên bước", tc.name)
		if len(rows[2]) > col {
			require.Empty(t, rows[2][col], "%s: khách chưa có bước ⇒ ô trống", tc.name)
		}
	}
}
