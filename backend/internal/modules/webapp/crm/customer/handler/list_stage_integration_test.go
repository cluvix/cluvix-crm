//go:build integration

package handler

// Story-04 (customer-stage-pipeline) — đường ĐỌC bước khách ở tầng handler, chạy thật trên MySQL:
//   - AC1: danh sách KHÔNG còn lọc ngầm theo tiến trình cũ — khách ở bước đang mở / thành công /
//     thất bại đều hiện (cột `status` cũ đã bỏ ở story-12).
//   - AC2: `filter[stage_id][i]` (đa giá trị ⇒ IN) chỉ trả khách ở các bước đó; `total` khớp số dòng.
//   - AC3: list, GET /:id, GET theo mã đều trả `stage_id`.
//   - AC4: export (`_export=1`, dùng chung buildCustomerFilters) áp đúng `filter[stage_id]`.
//   - AC8: ảnh chụp xoá khách + sự kiện seclog `customer_deleted` mang `stage_id`.

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"erpCluvix/db"
	customerService "erpCluvix/internal/modules/webapp/crm/customer/service"
	pkgExport "erpCluvix/pkg/export"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/logger"
	"erpCluvix/pkg/seclog"
)

// stageListBranch — chi nhánh riêng của file này; mọi lời gọi list lọc `filter[branch_id]` = nó để
// không lẫn khách NULL-branch / chi nhánh khác do test khác trong package seed.
const stageListBranch = uint(7301)

// stageSeedSeq — mỗi lần seed một tiền tố riêng (mã + SĐT không trùng giữa các test trong cùng lượt;
// `phone_unique_key` UNIQUE theo 9 số cuối).
var stageSeedSeq int

type stageSeed struct {
	id    uint
	code  string
	stage int
}

// seedStageCustomers — 3 khách × 3 loại bước (đang mở / thành công / thất bại). `status` cũ để DEFAULT
// của cột (=1, bộ trước story-04 bị ẩn); không seed xoay vòng vì cột bỏ ở story-12. Trả theo thứ tự seed.
func seedStageCustomers(t *testing.T) (open, won, lost []stageSeed) {
	t.Helper()
	stages := map[string][]int{
		"open": {stageIDByName(t, "Liên hệ"), stageIDByName(t, "Đang tư vấn"), stageIDByName(t, "Đang tư vấn")},
		"won":  {stageIDByName(t, "Khách hàng đã thuê"), stageIDByName(t, "Khách hàng đã thuê"), stageIDByName(t, "Khách hàng đã thuê")},
		"lost": {stageIDByName(t, "Từ chối"), stageIDByName(t, "Dừng chăm sóc"), stageIDByName(t, "Từ chối")},
	}
	out := map[string][]stageSeed{}
	n := 0
	stageSeedSeq++
	prefix := fmt.Sprintf("S04L%02d", stageSeedSeq)
	for _, kind := range []string{"open", "won", "lost"} {
		for _, st := range stages[kind] {
			code := fmt.Sprintf("%s%02d", prefix, n)
			phone := fmt.Sprintf("0966%04d%02d", stageSeedSeq, n)
			row := map[string]any{
				"code": code, "fullname": "S04 Stage " + code, "phone": phone,
				"branch_id": stageListBranch, "stage_id": st,
				"created_at": 1, "updated_at": 1,
			}
			require.NoError(t, testDB.Table("customer").Create(row).Error)
			var id uint
			require.NoError(t, testDB.Table("customer").Where("code = ?", code).Pluck("id", &id).Error)
			require.NotZero(t, id)
			out[kind] = append(out[kind], stageSeed{id: id, code: code, stage: st})
			n++
		}
	}
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer WHERE code LIKE ?", prefix+"%") })
	return out["open"], out["won"], out["lost"]
}

func callListStage(t *testing.T, rawQuery string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/crm/customer?filter[branch_id][0]="+
		strconv.FormatUint(uint64(stageListBranch), 10)+"&"+rawQuery, nil)
	c.Set("userID", uint(1))
	(&Handler{DB: testDB}).ListCustomer(c)
	return w
}

func idsOf(seeds ...[]stageSeed) []uint {
	var ids []uint
	for _, ss := range seeds {
		for _, s := range ss {
			ids = append(ids, s.id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func TestListCustomer_StageVisibilityAndFilter(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	open, won, lost := seedStageCustomers(t)
	wantStage := map[uint]int{}
	for _, s := range append(append(append([]stageSeed{}, open...), won...), lost...) {
		wantStage[s.id] = s.stage
	}

	check := func(t *testing.T, qs string, want []uint) {
		t.Helper()
		b := decodeList(t, callListStage(t, "page=1&per-page=50&"+qs))
		got := make([]uint, 0, len(b.Items))
		for _, it := range b.Items {
			id := uint(helpers.ToInt64(it["id"]))
			got = append(got, id)
			v, ok := it["stage_id"]
			require.True(t, ok, "(%s) dòng list thiếu key stage_id (AC3): %v", qs, it)
			require.EqualValues(t, wantStage[id], helpers.ToInt64(v), "(%s) stage_id dòng %d", qs, id)
		}
		sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
		require.Equal(t, want, got, "(%s) tập khách trả về", qs)
		require.EqualValues(t, len(want), b.Meta.TotalCount, "(%s) total phải khớp số dòng (AC2)", qs)
	}

	t.Run("AC1 không lọc ⇒ đủ 9 khách cả 3 loại bước, kể cả status cũ mặc định 1", func(t *testing.T) {
		check(t, "", idsOf(open, won, lost))
	})
	t.Run("AC2 lọc 2 bước (thành công + thất bại 'Từ chối')", func(t *testing.T) {
		qs := fmt.Sprintf("filter[stage_id][0]=%d&filter[stage_id][1]=%d", won[0].stage, lost[0].stage)
		check(t, qs, idsOf(won, []stageSeed{lost[0], lost[2]}))
	})
	t.Run("AC2 lọc 1 bước", func(t *testing.T) {
		check(t, fmt.Sprintf("filter[stage_id][0]=%d", open[1].stage), idsOf(open[1:]))
	})
	t.Run("filter cũ _status/status bị bỏ qua (không còn lọc ngầm)", func(t *testing.T) {
		check(t, "filter[_status][0]=1&filter[status]=2", idsOf(open, won, lost))
	})
	t.Run("stage_id rác ⇒ rỗng, KHÔNG ép kiểu thành bước khác", func(t *testing.T) {
		check(t, fmt.Sprintf("filter[stage_id][0]=%dabc", won[0].stage), []uint{})
	})
}

func TestGetCustomer_AndByCode_ReturnStageID(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	_, won, _ := seedStageCustomers(t)
	s := won[1]

	var body struct {
		Data map[string]any `json:"data"`
	}
	w := callGetCustomer(t, s.id)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.EqualValues(t, s.stage, helpers.ToInt64(body.Data["stage_id"]), "GET /:id thiếu/sai stage_id: %v", body.Data)

	gin.SetMode(gin.TestMode)
	w = httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/crm/customer/get-customer-by-code?code="+s.code, nil)
	c.Set("userID", uint(1))
	(&Handler{DB: testDB}).GetCustomerByCode(c)
	require.Equal(t, 200, w.Code, w.Body.String())
	body.Data = nil
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.EqualValues(t, s.id, helpers.ToInt64(body.Data["id"]))
	require.EqualValues(t, s.stage, helpers.ToInt64(body.Data["stage_id"]), "GET theo mã thiếu/sai stage_id: %v", body.Data)
}

// exportIDs gọi CHÍNH ListCustomer nhánh `_export=1` (cột id) rồi đọc file xlsx thật.
func exportIDs(t *testing.T, rawQuery string) []uint {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/crm/customer?_export=1&columns=id&filter[branch_id][0]="+
		strconv.FormatUint(uint64(stageListBranch), 10)+"&"+rawQuery, nil)
	c.Set("userID", uint(1))
	(&Handler{DB: testDB}).ListCustomer(c)
	require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
	var url string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &url), "body: %s", w.Body.String())
	path, ok := pkgExport.OwnerFilePath(pkgExport.DefaultDir, 1, filepath.Base(url))
	require.True(t, ok, "url=%s", url)
	f, err := excelize.OpenFile(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows("Sheet1")
	require.NoError(t, err)
	require.NotEmpty(t, rows, "file export không có header")
	ids := make([]uint, 0, len(rows)-1)
	for _, r := range rows[1:] {
		require.NotEmpty(t, r)
		ids = append(ids, uint(helpers.ToInt64(r[0])))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func TestExportCustomer_StageFilter(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	t.Chdir(t.TempDir()) // ExportDir "./storage/export/" — không ghi file vào cây source
	open, won, lost := seedStageCustomers(t)

	require.Equal(t, idsOf(won), exportIDs(t, fmt.Sprintf("filter[stage_id][0]=%d", won[0].stage)),
		"⛔ export không áp filter[stage_id] (AC4)")
	// Đối chứng: không lọc bước ⇒ đủ 9 (export cũng không còn lọc ngầm status).
	require.Equal(t, idsOf(open, won, lost), exportIDs(t, ""))
}

func TestDeleteCustomer_AuditSnapshotHasStageID(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	logger.Init(logger.Config{Env: "test", AppCode: "crm_customer_handler_test"})
	origDB := db.DB
	db.DB = testDB
	t.Cleanup(func() { db.DB = origDB })

	got := make(chan seclog.Entry, 64)
	seclog.InitSink(64, func(batch []seclog.Entry) {
		for _, e := range batch {
			got <- e
		}
	})

	_, _, lost := seedStageCustomers(t)
	s := lost[1]

	// Ảnh chụp đọc TRƯỚC khi xoá.
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("DELETE", "/crm/customer/1", nil)
	snap, found, err := (&Handler{DB: testDB}).deleteAuditSnapshot(c, s.id)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, s.stage, snap.StageID, "ảnh chụp xoá thiếu stage_id")

	// Xoá thật qua handler ⇒ sự kiện seclog mang stage_id.
	w := httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	id := strconv.FormatUint(uint64(s.id), 10)
	c.Request = httptest.NewRequest("DELETE", "/crm/customer/"+id, nil)
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Set("userID", uint(1))
	(&Handler{DB: testDB, CustomerSvc: customerService.NewCustomerService(testDB)}).DeleteCustomer(c)
	require.Equal(t, 200, w.Code, w.Body.String())

	deadline := time.After(6 * time.Second) // sink flush theo ticker 2s
	for {
		select {
		case e := <-got:
			if e.Event != evCustomerDeleted || helpers.ToInt64(e.Detail["customer_id"]) != int64(s.id) {
				continue
			}
			v, ok := e.Detail["stage_id"]
			require.True(t, ok, "seclog customer_deleted thiếu stage_id: %v", e.Detail)
			require.EqualValues(t, s.stage, helpers.ToInt64(v))
			return
		case <-deadline:
			t.Fatal("không nhận được sự kiện seclog customer_deleted")
		}
	}
}
