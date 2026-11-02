//go:build integration

package handler

// Story-03 AC4 — import file có 2 khách MỚI + 1 khách ĐÃ CÓ (khớp SĐT + họ tên):
//   - 2 khách mới nhận bước đang mặc định + đúng 1 dòng lịch sử source = Import (from NULL).
//   - khách đã có: stage_id giữ nguyên, không thêm dòng lịch sử nào.
// File dựng bằng excelize trong t.TempDir(), trỏ qua env IMPORT_CUSTOMER_FILE (đường handler thật đọc).
// KHÔNG có cột branch_id: importBranchAllowed tra chi nhánh trên h.DB khi tx đang giữ kết nối duy nhất
// của Suite (SetMaxOpenConns(1)) ⇒ treo; đường đó đã có TestImportBranchAllowed riêng.

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/i18n"
)

type importHist struct {
	FromStageID *int `gorm:"column:from_stage_id"`
	ToStageID   int  `gorm:"column:to_stage_id"`
	Source      int
	ChangedBy   *int `gorm:"column:changed_by"`
}

func importHistory(t *testing.T, customerID uint) []importHist {
	t.Helper()
	rows := make([]importHist, 0)
	require.NoError(t, testDB.Table("customer_stage_history").Select("from_stage_id, to_stage_id, source, changed_by").
		Where("customer_id = ?", customerID).Order("id ASC").Find(&rows).Error)
	return rows
}

func writeImportFile(t *testing.T, rows [][]string) string {
	t.Helper()
	f := excelize.NewFile()
	sheet := f.GetSheetName(0)
	header := [][]string{{"Họ tên", "Điện thoại"}, {"fullname", "phone"}}
	for i, r := range append(header, rows...) {
		for j, v := range r {
			cell, err := excelize.CoordinatesToCellName(j+1, i+1)
			require.NoError(t, err)
			require.NoError(t, f.SetCellStr(sheet, cell, v))
		}
	}
	path := filepath.Join(t.TempDir(), "import_s03.xlsx")
	require.NoError(t, f.SaveAs(path))
	require.NoError(t, f.Close())
	return path
}

func TestImportCustomer_NewCustomersGetDefaultStage_ExistingUntouched(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := testhelpers.CreateScopedUser(t, testDB, "s03imp", tn, tn.BranchID)

	var defaultStage, negotiating int
	require.NoError(t, testDB.Raw("SELECT id FROM customer_stage WHERE is_default = 1").Scan(&defaultStage).Error)
	require.NoError(t, testDB.Raw("SELECT id FROM customer_stage WHERE name = ?", "Đang đàm phán").Scan(&negotiating).Error)
	require.NotZero(t, defaultStage)
	require.NotZero(t, negotiating)

	n := time.Now().UnixNano()
	suffix := fmt.Sprintf("%d", n)[len(fmt.Sprintf("%d", n))-6:]
	phone := func(k int) string { return fmt.Sprintf("091%d%s", k, suffix) }
	existingName := "S03 Import Cu " + suffix
	newNames := []string{"S03 Import Moi Mot " + suffix, "S03 Import Moi Hai " + suffix}

	// Khách đã có — đang ở bước KHÁC mặc định để thấy rõ import không đụng bước.
	now := time.Now().Unix()
	existing := map[string]any{
		"fullname": existingName, "phone": phone(0),
		"stage_id": negotiating, "created_at": now, "updated_at": now,
	}
	require.NoError(t, testDB.Table("customer").Create(&existing).Error)
	existingID := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id IN (SELECT id FROM (SELECT id FROM customer WHERE fullname LIKE ?) x)", "S03 Import %"+suffix)
		testDB.Exec("DELETE FROM customer WHERE fullname LIKE ?", "S03 Import %"+suffix)
	})

	path := writeImportFile(t, [][]string{
		{newNames[0], phone(1)},
		{existingName, phone(0)},
		{newNames[1], phone(2)},
	})
	t.Setenv(define.EnvImportCustomerFile, path)

	resp := callImport(t, New(Deps{DB: testDB}), tn.BranchID, uid)
	require.Equal(t, http.StatusOK, resp.Code, "body: %s", resp.Body.String())
	require.Contains(t, resp.Body.String(), i18n.TL(i18n.Vi, "import_success"))

	for _, name := range newNames {
		var id uint
		require.NoError(t, testDB.Table("customer").Where("fullname = ?", name).Pluck("id", &id).Error)
		require.NotZero(t, id, "khách mới %q phải được tạo", name)
		var stage *int
		require.NoError(t, testDB.Raw("SELECT stage_id FROM customer WHERE id = ?", id).Scan(&stage).Error)
		require.NotNil(t, stage)
		require.Equal(t, defaultStage, *stage, "khách mới %q nhận bước mặc định", name)
		hist := importHistory(t, id)
		require.Len(t, hist, 1, "khách mới %q: đúng 1 dòng lịch sử", name)
		require.Nil(t, hist[0].FromStageID)
		require.Equal(t, defaultStage, hist[0].ToStageID)
		require.Equal(t, define.CustomerStageHistorySourceImport, hist[0].Source)
		require.NotNil(t, hist[0].ChangedBy)
		require.EqualValues(t, uid, *hist[0].ChangedBy)
	}

	var stage int
	require.NoError(t, testDB.Raw("SELECT stage_id FROM customer WHERE id = ?", existingID).Scan(&stage).Error)
	require.Equal(t, negotiating, stage, "khách đã có: stage_id giữ nguyên")
	require.Empty(t, importHistory(t, existingID), "khách đã có: không có lịch sử mới")
	var dup int64
	require.NoError(t, testDB.Table("customer").Where("fullname = ?", existingName).Count(&dup).Error)
	require.EqualValues(t, 1, dup, "đối chứng: dòng khách cũ được nhận diện là đã có, không tạo trùng")
}

func callImport(t *testing.T, h *Handler, branchID, uid uint) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("branchActiveId", branchID)
	c.Set("branchIds", []uint{branchID})
	c.Set("userID", uid)
	c.Request = httptest.NewRequest(http.MethodPost, "/import/customer/from-excel", nil)
	h.ImportCustomerFromExcel(c)
	return w
}

// --- Story-03 QA pass 1 (P1): lịch sử bước của import ghi CÙNG tx với khách ---------------------------

func importTotalHistory(t *testing.T) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testDB.Table("customer_stage_history").Count(&n).Error)
	return n
}

// seedImportExisting — khách đã có (khớp SĐT + họ tên) ở bước `stageID`; dọn mọi khách "<prefix>%<suffix>".
func seedImportExisting(t *testing.T, prefix, suffix, phone string, stageID int) uint {
	t.Helper()
	row := map[string]any{"fullname": prefix + "Cu " + suffix, "phone": phone,
		"stage_id": stageID, "created_at": 1000, "updated_at": 1000}
	require.NoError(t, testDB.Table("customer").Create(&row).Error)
	id := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id IN (SELECT id FROM (SELECT id FROM customer WHERE fullname LIKE ?) x)", prefix+"%"+suffix)
		testDB.Exec("DELETE FROM customer WHERE fullname LIKE ?", prefix+"%"+suffix)
	})
	return id
}

func importSuffix() string {
	n := fmt.Sprintf("%d", time.Now().UnixNano())
	return n[len(n)-6:]
}

// (c) Dòng 3 trùng SĐT dòng 1 (1062 giữa chừng) ⇒ 422, rollback toàn bộ: dòng 1 (đã INSERT khách + lịch
// sử trong tx) biến mất, tổng lịch sử không đổi, khách cũ giữ bước.
func TestImportCustomer_ErrorMidway_RollsBackHistory(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := testhelpers.CreateScopedUser(t, testDB, "s03imp", tn, tn.BranchID)
	var negotiating int
	require.NoError(t, testDB.Raw("SELECT id FROM customer_stage WHERE name = ?", "Đang đàm phán").Scan(&negotiating).Error)
	sfx := importSuffix()
	phone := func(k int) string { return fmt.Sprintf("092%d%s", k, sfx) }
	const prefix = "S03 ImpTx "
	existingID := seedImportExisting(t, prefix, sfx, phone(0), negotiating)
	before := importTotalHistory(t)

	t.Setenv(define.EnvImportCustomerFile, writeImportFile(t, [][]string{
		{prefix + "Moi " + sfx, phone(1)},
		{prefix + "Cu " + sfx, phone(0)},
		{prefix + "Trung " + sfx, phone(1)},
	}))
	resp := callImport(t, New(Deps{DB: testDB}), tn.BranchID, uid)
	require.Equal(t, http.StatusUnprocessableEntity, resp.Code, "body: %s", resp.Body.String())

	var cnt int64
	require.NoError(t, testDB.Table("customer").Where("fullname LIKE ?", prefix+"%"+sfx).Count(&cnt).Error)
	require.EqualValues(t, 1, cnt, "rollback ⇒ chỉ còn khách cũ")
	require.Equal(t, before, importTotalHistory(t), "rollback ⇒ không còn lịch sử của dòng đã INSERT")
	var st int
	require.NoError(t, testDB.Raw("SELECT stage_id FROM customer WHERE id = ?", existingID).Scan(&st).Error)
	require.Equal(t, negotiating, st)
}

// (d) INSERT lịch sử bị chặn ⇒ 422, rollback: không khách mới nào, không lịch sử.
func TestImportCustomer_HistoryInsertFails_RollsBack(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := testhelpers.CreateScopedUser(t, testDB, "s03imp", tn, tn.BranchID)
	sfx := importSuffix()
	const prefix = "S03 ImpHf "
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id IN (SELECT id FROM (SELECT id FROM customer WHERE fullname LIKE ?) x)", prefix+"%"+sfx)
		testDB.Exec("DELETE FROM customer WHERE fullname LIKE ?", prefix+"%"+sfx)
	})
	const cb = "s03_import_fail_history"
	hits := 0
	require.NoError(t, testDB.Callback().Create().Before("gorm:create").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "customer_stage_history" {
			hits++
			_ = tx.AddError(errors.New("s03 boom: import history"))
		}
	}))
	t.Cleanup(func() { _ = testDB.Callback().Create().Remove(cb) })
	before := importTotalHistory(t)

	t.Setenv(define.EnvImportCustomerFile, writeImportFile(t, [][]string{
		{prefix + "Mot " + sfx, fmt.Sprintf("0931%s", sfx)},
		{prefix + "Hai " + sfx, fmt.Sprintf("0932%s", sfx)},
	}))
	resp := callImport(t, New(Deps{DB: testDB}), tn.BranchID, uid)
	require.Equal(t, http.StatusUnprocessableEntity, resp.Code, "body: %s", resp.Body.String())
	require.Equal(t, 1, hits, "đối chứng: đường ghi lịch sử import CÓ được gọi (dừng ở dòng đầu)")
	var cnt int64
	require.NoError(t, testDB.Table("customer").Where("fullname LIKE ?", prefix+"%"+sfx).Count(&cnt).Error)
	require.Zero(t, cnt, "lịch sử lỗi ⇒ không khách mới nào")
	require.Equal(t, before, importTotalHistory(t))
}
