//go:build integration

// Integration test handler `/api/config/customer-stage*` (story-02 epic customer-stage-pipeline).
// Dựng route bằng chính `customer_stage.RegisterRoutes` (không chép tay) để AC9 (`/5abc` ⇒ 404) và
// thứ tự `reorder` / `:id` được kiểm trên đường thật.
//
// ⛔ Chỉ chạy trên DB DÙNG-MỘT-LẦN (`TEST_MYSQL_DSN` tên kết thúc `_test`/`_tmp`); chạy qua
// `backend/scripts/verify_integration_isolated.sh`.
package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"erpCluvix/internal/modules/webapp/config/customer_stage"
	"erpCluvix/internal/modules/webapp/config/customer_stage/handler"
	indexeddb "erpCluvix/internal/modules/webapp/config/indexeddb/handler"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

const testUserID uint = 42

func requireDB(t *testing.T) {
	t.Helper()
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
}

func newRouter(branchID uint) *gin.Engine { return newRouterOn(testDB, branchID) }

func newRouterOn(db *gorm.DB, branchID uint) *gin.Engine {
	r := testhelpers.NewGin()
	r.Use(testhelpers.AuthMiddleware(branchID, testUserID))
	customer_stage.RegisterRoutes(r.Group("/api/config"), handler.Deps{DB: db})
	ix := indexeddb.New(indexeddb.Deps{DB: testDB})
	r.GET("/api/crm/indexeddb/sync", ix.Sync)
	return r
}

type resp struct {
	*httptest.ResponseRecorder
}

func (w resp) json(t *testing.T) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out), w.Body.String())
	return out
}

func (w resp) message(t *testing.T) string {
	t.Helper()
	m, _ := w.json(t)["message"].(string)
	return m
}

func (w resp) dataID(t *testing.T) uint {
	t.Helper()
	d, ok := w.json(t)["data"].(map[string]any)
	require.True(t, ok, w.Body.String())
	return uint(d["id"].(float64))
}

func call(r *gin.Engine, method, path string, body any) resp {
	var buf *bytes.Buffer
	if body == nil {
		buf = bytes.NewBuffer(nil)
	} else {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return resp{w}
}

type stageRow struct {
	ID        uint
	Name      string
	Type      int
	Color     string
	SortOrder int
	IsDefault int
	Status    int
	CreatedBy *int
	UpdatedBy *int
}

func rows(t *testing.T) []stageRow {
	t.Helper()
	var out []stageRow
	require.NoError(t, testDB.Raw("SELECT id, name, type, color, sort_order, is_default, status, created_by, updated_by FROM customer_stage ORDER BY id").Scan(&out).Error)
	return out
}

func rowByName(t *testing.T, name string) stageRow {
	t.Helper()
	for _, r := range rows(t) {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("không có bước %q", name)
	return stageRow{}
}

func count(t *testing.T, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testDB.Raw(sql, args...).Scan(&n).Error)
	return n
}

// resetCatalog dọn danh mục về rỗng (FK tắt ở mức session theo Suite) rồi dựng đúng 4 bước biết trước:
//
//	A "Liên hệ"      open  default sort 1
//	B "Đang tư vấn"  open  -       sort 2
//	C "Thành công"   won   -       sort 3
//	D "Từ chối"      lost  -       sort 4
//
// và đặt config_systems về `{}` để kiểm bump Dexie.
func resetCatalog(t *testing.T) (a, b, c, d uint) {
	t.Helper()
	requireDB(t)
	require.NoError(t, testDB.Exec("SET SESSION foreign_key_checks = 0").Error)
	require.NoError(t, testDB.Exec("DELETE FROM customer_stage_history").Error)
	// KHÔNG `UPDATE/DELETE customer` ở đây: mysqld 9.7.1 SIGSEGV (cleanup_value_generator_items) khi UPDATE/DELETE
	// một dòng `customer` đang có stage_id khác NULL (FK tới customer_stage — bảng cha có cột sinh VIRTUAL
	// default_key). 3/3 lần crash đều đúng ca đó; dòng khách giữ bước chỉ INSERT, không bao giờ sửa/xoá.
	// FK đang tắt nên xoá customer_stage để lại stage_id mồ côi trên khách thử — vô hại (DB dùng-một-lần).
	require.NoError(t, testDB.Exec("DELETE FROM customer_stage").Error)
	now := time.Now().Unix()
	ins := func(name string, typ int, color string, sort, def, status int) uint {
		require.NoError(t, testDB.Exec(
			"INSERT INTO customer_stage (name, type, color, sort_order, is_default, status, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)",
			name, typ, color, sort, def, status, now, now).Error)
		return testhelpers.LastInsertID(testDB)
	}
	a = ins("Liên hệ", define.CustomerStageTypeOpen, "gray", 1, 1, 1)
	b = ins("Đang tư vấn", define.CustomerStageTypeOpen, "blue", 2, 0, 1)
	c = ins("Thành công", define.CustomerStageTypeWon, "green", 3, 0, 1)
	d = ins("Từ chối", define.CustomerStageTypeLost, "red", 4, 0, 1)
	resetDexie(t)
	return
}

func resetDexie(t *testing.T) {
	t.Helper()
	require.NoError(t, testDB.Exec("DELETE FROM config_systems").Error)
	require.NoError(t, testDB.Exec("INSERT INTO config_systems (content) VALUES ('{}')").Error)
}

func dexieVersion(t *testing.T) int64 {
	t.Helper()
	return count(t, "SELECT COALESCE(JSON_EXTRACT(content, '$.dexieVersions.customerStages'), 0) FROM config_systems LIMIT 1")
}

func sortOrders(t *testing.T) map[uint]int {
	t.Helper()
	m := map[uint]int{}
	for _, r := range rows(t) {
		m[r.ID] = r.SortOrder
	}
	return m
}

func path(id uint, suffix string) string {
	return fmt.Sprintf("/api/config/customer-stage/%d%s", id, suffix)
}

// --- AC1 -----------------------------------------------------------------------------------------

func TestList_AllStagesIncludingHidden_SortedBySortOrderThenID(t *testing.T) {
	a, b, c, d := resetCatalog(t)
	// Đảo thứ tự lưu + ẩn 1 bước + 2 bước trùng sort_order (tie-break theo id).
	require.NoError(t, testDB.Exec("UPDATE customer_stage SET sort_order = 9 WHERE id = ?", a).Error)
	require.NoError(t, testDB.Exec("UPDATE customer_stage SET sort_order = 2 WHERE id IN (?, ?)", c, d).Error)
	require.NoError(t, testDB.Exec("UPDATE customer_stage SET sort_order = 1, status = 0 WHERE id = ?", b).Error)
	r := newRouter(1)

	w := call(r, "GET", "/api/config/customer-stage", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	items := w.json(t)["items"].([]any)
	var gotIDs []uint
	for _, it := range items {
		gotIDs = append(gotIDs, uint(it.(map[string]any)["id"].(float64)))
	}
	require.Equal(t, []uint{b, c, d, a}, gotIDs, "sort_order ASC, id ASC; bước ẩn vẫn có mặt")
	_, hasDefaultKey := items[0].(map[string]any)["default_key"]
	require.False(t, hasDefaultKey, "cột sinh default_key không được phơi ra")

	// filter[status]
	w = call(r, "GET", "/api/config/customer-stage?filter[status]=0", nil)
	items = w.json(t)["items"].([]any)
	require.Len(t, items, 1)
	require.EqualValues(t, b, items[0].(map[string]any)["id"])

	// _keyword (collation DB: LIKE không phân biệt hoa/thường)
	w = call(r, "GET", "/api/config/customer-stage?_keyword=T%E1%BB%AB%20ch", nil) // "Từ ch" — "từ" trần cũng khớp "tư" của "Đang tư vấn" vì collation bỏ dấu
	items = w.json(t)["items"].([]any)
	require.Len(t, items, 1)
	require.EqualValues(t, d, items[0].(map[string]any)["id"])

	// GET một bước
	w = call(r, "GET", path(b, ""), nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.EqualValues(t, b, w.json(t)["data"].(map[string]any)["id"])
}

// --- AC2 -----------------------------------------------------------------------------------------

func TestCreate_ValidationAndServerOwnedFields(t *testing.T) {
	a, _, _, _ := resetCatalog(t)
	r := newRouter(1)
	before := count(t, "SELECT COUNT(*) FROM customer_stage")

	t.Run("thành công: sort_order = max+1, is_default = 0 bất kể body, id/created_at client gửi bị bỏ", func(t *testing.T) {
		w := call(r, "POST", "/api/config/customer-stage", map[string]any{
			"name": "  Đã ký hợp đồng  ", "type": define.CustomerStageTypeOpen, "color": "teal",
			"is_default": 1, "sort_order": 99, "id": a, "created_at": 1,
		})
		require.Equal(t, 201, w.Code, w.Body.String())
		id := w.dataID(t)
		require.NotEqual(t, a, id, "id do DB cấp")
		got := rowByName(t, "Đã ký hợp đồng") // tên đã trim
		require.Equal(t, id, got.ID)
		require.Equal(t, 5, got.SortOrder, "max(4)+1")
		require.Equal(t, 0, got.IsDefault)
		require.Equal(t, 1, got.Status, "status mặc định 1 khi body không gửi")
		require.NotNil(t, got.CreatedBy)
		require.EqualValues(t, testUserID, *got.CreatedBy)
		require.EqualValues(t, 1, count(t, "SELECT COUNT(*) FROM customer_stage WHERE is_default = 1"), "vẫn đúng 1 bước mặc định")
		require.NotZero(t, dexieVersion(t), "ghi thành công ⇒ bump customerStages")
		require.Greater(t, count(t, "SELECT created_at FROM customer_stage WHERE id = ?", id), int64(1), "created_at do server")
	})
	before++

	bad := []struct {
		name string
		body map[string]any
		want string
	}{
		{"name rỗng", map[string]any{"name": "", "type": 1, "color": "gray"}, "Tên bước không được để trống"},
		{"name toàn khoảng trắng", map[string]any{"name": "   ", "type": 1, "color": "gray"}, "Tên bước không được để trống"},
		{"trùng tên y hệt", map[string]any{"name": "Đang tư vấn", "type": 1, "color": "gray"}, "Tên bước đã tồn tại"},
		{"trùng khác hoa/thường + khoảng trắng", map[string]any{"name": "  ĐANG TƯ VẤN ", "type": 1, "color": "gray"}, "Tên bước đã tồn tại"},
		{"trùng theo collation (không dấu)", map[string]any{"name": "lien HE", "type": 1, "color": "gray"}, "Tên bước đã tồn tại"},
		{"type 0", map[string]any{"name": "X1", "type": 0, "color": "gray"}, "Loại bước không hợp lệ"},
		{"type 4", map[string]any{"name": "X2", "type": 4, "color": "gray"}, "Loại bước không hợp lệ"},
		{"thiếu type", map[string]any{"name": "X3", "color": "gray"}, "Loại bước không hợp lệ"},
		{"color ngoài bảng màu", map[string]any{"name": "X4", "type": 1, "color": "pink"}, "Màu bước không hợp lệ"},
		{"color hex", map[string]any{"name": "X5", "type": 1, "color": "#ff0000"}, "Màu bước không hợp lệ"},
		{"color rỗng", map[string]any{"name": "X6", "type": 1, "color": ""}, "Màu bước không hợp lệ"},
		{"status ngoài {0,1}", map[string]any{"name": "X7", "type": 1, "color": "gray", "status": 2}, "Trạng thái không hợp lệ"},
	}
	for _, tc := range bad {
		t.Run("422: "+tc.name, func(t *testing.T) {
			w := call(r, "POST", "/api/config/customer-stage", tc.body)
			require.Equal(t, 422, w.Code, w.Body.String())
			require.Equal(t, tc.want, w.message(t))
		})
	}
	require.EqualValues(t, before, count(t, "SELECT COUNT(*) FROM customer_stage"), "mọi ca lỗi: không ghi dòng nào")

	t.Run("lỗi không bump Dexie", func(t *testing.T) {
		resetDexie(t)
		w := call(r, "POST", "/api/config/customer-stage", map[string]any{"name": "", "type": 1, "color": "gray"})
		require.Equal(t, 422, w.Code)
		require.Zero(t, dexieVersion(t))
	})
}

// --- AC3 + AC9 (PUT) -----------------------------------------------------------------------------

func TestUpdate_DefaultInvariantsDuplicateAndIgnoredFields(t *testing.T) {
	a, b, c, _ := resetCatalog(t)
	r := newRouter(1)

	put := func(id uint, body map[string]any) resp { return call(r, "PUT", path(id, ""), body) }

	t.Run("bước mặc định → ẩn ⇒ 422", func(t *testing.T) {
		w := put(a, map[string]any{"name": "Liên hệ", "type": 1, "color": "gray", "status": 0})
		require.Equal(t, 422, w.Code, w.Body.String())
		require.Contains(t, w.message(t), "Không thể ẩn bước mặc định")
		require.Equal(t, 1, rowByName(t, "Liên hệ").Status)
	})
	t.Run("bước mặc định → type thất bại ⇒ 422", func(t *testing.T) {
		w := put(a, map[string]any{"name": "Liên hệ", "type": 3, "color": "gray", "status": 1})
		require.Equal(t, 422, w.Code, w.Body.String())
		require.Contains(t, w.message(t), "phải thuộc loại đang mở")
		require.Equal(t, 1, rowByName(t, "Liên hệ").Type)
	})
	t.Run("bước mặc định → type thành công ⇒ 422 (bất biến: mặc định phải đang mở)", func(t *testing.T) {
		w := put(a, map[string]any{"name": "Liên hệ", "type": 2, "color": "gray", "status": 1})
		require.Equal(t, 422, w.Code, w.Body.String())
		require.Equal(t, 1, rowByName(t, "Liên hệ").Type)
	})
	t.Run("bước KHÔNG mặc định được ẩn / đổi loại", func(t *testing.T) {
		w := put(b, map[string]any{"name": "Đang tư vấn", "type": 3, "color": "blue", "status": 0})
		require.Equal(t, 200, w.Code, w.Body.String())
		got := rowByName(t, "Đang tư vấn")
		require.Equal(t, 3, got.Type)
		require.Equal(t, 0, got.Status)
		require.NotZero(t, dexieVersion(t))
	})
	t.Run("đổi tên trùng bước khác (kể cả khác hoa/thường) ⇒ 422", func(t *testing.T) {
		for _, name := range []string{"Thành công", "THÀNH CÔNG ", "Thanh cong"} { // "Thanh cong" ≡ "Thành công" theo utf8mb4_unicode_ci
			w := put(b, map[string]any{"name": name, "type": 1, "color": "blue", "status": 1})
			require.Equal(t, 422, w.Code, "%q: %s", name, w.Body.String())
			require.Equal(t, "Tên bước đã tồn tại", w.message(t))
		}
		require.Equal(t, "Đang tư vấn", rowByName(t, "Đang tư vấn").Name)
	})
	t.Run("giữ nguyên tên của chính nó (không tự trùng với mình) + trim", func(t *testing.T) {
		w := put(c, map[string]any{"name": "  Thành công  ", "type": 2, "color": "teal", "status": 1})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, "teal", rowByName(t, "Thành công").Color)
	})
	t.Run("body gửi is_default/sort_order/id/created_by ⇒ bị bỏ qua", func(t *testing.T) {
		before := rowByName(t, "Thành công")
		w := put(c, map[string]any{"name": "Thành công", "type": 2, "color": "green", "status": 1,
			"is_default": 1, "sort_order": 77, "id": a, "created_by": 999})
		require.Equal(t, 200, w.Code, w.Body.String())
		after := rowByName(t, "Thành công")
		require.Equal(t, c, after.ID)
		require.Equal(t, before.SortOrder, after.SortOrder)
		require.Equal(t, 0, after.IsDefault)
		require.Equal(t, before.CreatedBy, after.CreatedBy)
		require.EqualValues(t, 1, count(t, "SELECT COUNT(*) FROM customer_stage WHERE is_default = 1"))
		require.Equal(t, "Liên hệ", rowByName(t, "Liên hệ").Name, "bước a không bị ghi đè qua id trong body")
		require.NotNil(t, after.UpdatedBy)
		require.EqualValues(t, testUserID, *after.UpdatedBy)
	})
	t.Run("type/color/name sai ⇒ 422", func(t *testing.T) {
		require.Equal(t, 422, put(c, map[string]any{"name": "", "type": 2, "color": "green", "status": 1}).Code)
		require.Equal(t, 422, put(c, map[string]any{"name": "ok", "type": 9, "color": "green", "status": 1}).Code)
		require.Equal(t, 422, put(c, map[string]any{"name": "ok", "type": 2, "color": "nope", "status": 1}).Code)
		require.Equal(t, 422, put(c, map[string]any{"name": "ok", "type": 2, "color": "green", "status": 5}).Code)
		require.Equal(t, 422, put(c, map[string]any{"name": "ok", "type": 2, "color": "green", "status": nil}).Code)
	})
	t.Run("id không tồn tại ⇒ 404", func(t *testing.T) {
		require.Equal(t, 404, put(999999, map[string]any{"name": "ok", "type": 1, "color": "gray", "status": 1}).Code)
	})
}

// --- AC4 + AC9 (DELETE) --------------------------------------------------------------------------

// countStageDeletes đếm số câu DELETE gửi tới bảng customer_stage (callback GORM trên testDB) — bằng chứng
// "không có DELETE nào được gửi khi bước đang dùng" (pre-check ở app, không dựa vào 1451).
func countStageDeletes(t *testing.T) *int {
	t.Helper()
	n := new(int)
	const name = "customer_stage_test:count_delete"
	require.NoError(t, testDB.Callback().Delete().Before("gorm:delete").Register(name, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "customer_stage" {
			*n++
		}
	}))
	t.Cleanup(func() { _ = testDB.Callback().Delete().Remove(name) })
	return n
}

// ⚠ mysqld 9.7.1 (máy dev) SIGSEGV khi một DELETE cha bị 1451 bởi dòng `customer` từ lần thứ hai (cột sinh VIRTUAL
// regexp `customer.phone_unique_key`). Handler vì thế KIỂM TRƯỚC (service.stageInUse) và không gửi DELETE khi bước
// đang dùng; test chạy với FK BẬT thật nhưng fixture (INSERT history/customer chạm FK) dựng khi FK tắt, và test
// không UPDATE/DELETE customer, không dùng FreshTenant (cleanup của nó là DELETE customer).
func TestDelete_DefaultBlocked_InUseBlocked_UnusedDeleted(t *testing.T) {
	a, b, c, d := resetCatalog(t)
	r := newRouter(1)
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer_stage_history") })

	exists := func(id uint) bool {
		return count(t, "SELECT COUNT(*) FROM customer_stage WHERE id = ?", id) == 1
	}

	// Fixture (FK tắt — mặc định của Suite): khách ở bước b; lịch sử đổi c → d; bước e chưa dùng.
	now := time.Now().Unix()
	cust := map[string]any{"fullname": "Khách giữ bước", "stage_id": b, "created_day": now, "created_at": now, "updated_at": now}
	require.NoError(t, testDB.Table("customer").Create(&cust).Error)
	custID := testhelpers.LastInsertID(testDB)
	require.NoError(t, testDB.Exec(
		"INSERT INTO customer_stage_history (customer_id, from_stage_id, to_stage_id, source, changed_at) VALUES (?, ?, ?, ?, ?)",
		custID, c, d, define.CustomerStageHistorySourceManual, now).Error)
	require.NoError(t, testDB.Exec(
		"INSERT INTO customer_stage (name, type, color, sort_order, is_default, status, created_at, updated_at) VALUES ('Chưa dùng', 1, 'gray', 5, 0, 1, ?, ?)", now, now).Error)
	e := testhelpers.LastInsertID(testDB)

	// FK BẬT thật cho toàn bộ phần còn lại; trả về tắt TRƯỚC khi các cleanup khác chạy (LIFO).
	require.NoError(t, testDB.Exec("SET SESSION foreign_key_checks = 1").Error)
	t.Cleanup(func() { testDB.Exec("SET SESSION foreign_key_checks = 0") })
	deletes := countStageDeletes(t)

	t.Run("bước mặc định ⇒ 422, không gửi DELETE, còn nguyên", func(t *testing.T) {
		w := call(r, "DELETE", path(a, ""), nil)
		require.Equal(t, 422, w.Code, w.Body.String())
		require.Contains(t, w.message(t), "Không thể xoá bước mặc định")
		require.True(t, exists(a))
		require.Zero(t, *deletes)
	})
	t.Run("lỗi xoá không bump Dexie", func(t *testing.T) {
		resetDexie(t)
		require.Equal(t, 422, call(r, "DELETE", path(a, ""), nil).Code)
		require.Zero(t, dexieVersion(t))
	})
	for _, tc := range []struct {
		name string
		id   uint
	}{
		{"bước đang có khách (customer.stage_id)", b},
		{"bước có trong lịch sử — from_stage_id", c},
		{"bước có trong lịch sử — to_stage_id", d},
	} {
		t.Run(tc.name+" ⇒ 422 gợi ý ẩn, KHÔNG gửi DELETE, còn nguyên", func(t *testing.T) {
			resetDexie(t)
			before := *deletes
			w := call(r, "DELETE", path(tc.id, ""), nil)
			require.Equal(t, 422, w.Code, w.Body.String())
			require.Equal(t, "Bước đang được dùng, hãy ẩn thay vì xoá", w.message(t))
			require.True(t, exists(tc.id))
			require.Equal(t, before, *deletes, "pre-check phải chặn TRƯỚC khi gửi DELETE (không dựa vào 1451)")
			require.Zero(t, dexieVersion(t), "xoá thất bại không bump")
		})
	}
	t.Run("bước chưa dùng ⇒ xoá được (đúng 1 DELETE) + bump Dexie", func(t *testing.T) {
		resetDexie(t)
		before := *deletes
		w := call(r, "DELETE", path(e, ""), nil)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.False(t, exists(e))
		require.Equal(t, before+1, *deletes)
		require.NotZero(t, dexieVersion(t))
	})
	t.Run("id không tồn tại ⇒ 404", func(t *testing.T) {
		require.Equal(t, 404, call(r, "DELETE", path(999999, ""), nil).Code)
	})
}

// --- AC5 -----------------------------------------------------------------------------------------

func TestReorder(t *testing.T) {
	a, b, c, d := resetCatalog(t)
	r := newRouter(1)
	orig := sortOrders(t)
	reorder := func(body any) resp { return call(r, "PUT", "/api/config/customer-stage/reorder", body) }

	invalid := map[string][]any{
		"thiếu 1 id":               {a, b, c},
		"thừa id lạ":               {a, b, c, d, 9999},
		"trùng id":                 {a, a, c, d},
		"đủ độ dài nhưng có id lạ": {a, b, c, 9999},
		"rỗng":                     {},
		"id âm":                    {a, b, c, -1},
	}
	for name, ids := range invalid {
		t.Run("422, không ghi gì: "+name, func(t *testing.T) {
			resetDexie(t)
			w := reorder(map[string]any{"ids": ids})
			require.Equal(t, 422, w.Code, w.Body.String())
			require.Equal(t, "Danh sách thứ tự phải gồm đủ mọi bước, mỗi bước đúng một lần", w.message(t))
			require.Equal(t, orig, sortOrders(t))
			require.Zero(t, dexieVersion(t), "lỗi không bump Dexie")
		})
	}
	t.Run("thiếu field ids / sai kiểu ⇒ 422, không ghi gì", func(t *testing.T) {
		require.Equal(t, 422, reorder(map[string]any{}).Code)
		require.Equal(t, 422, reorder(map[string]any{"ids": []any{"x", "y"}}).Code)
		require.Equal(t, orig, sortOrders(t))
	})
	t.Run("hợp lệ ⇒ sort_order = 1..n theo thứ tự ids + bump", func(t *testing.T) {
		resetDexie(t)
		w := reorder(map[string]any{"ids": []any{d, b, a, c}})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, map[uint]int{d: 1, b: 2, a: 3, c: 4}, sortOrders(t))
		require.NotZero(t, dexieVersion(t))
		// list phản ánh thứ tự mới
		items := call(r, "GET", "/api/config/customer-stage", nil).json(t)["items"].([]any)
		require.EqualValues(t, d, items[0].(map[string]any)["id"])
		// mặc định không đổi khi reorder
		require.Equal(t, 1, rowByName(t, "Liên hệ").IsDefault)
	})
	t.Run("route reorder không bị :id nuốt (PUT .../reorder không phải sửa bước id='reorder')", func(t *testing.T) {
		w := reorder(map[string]any{"ids": []any{a, b, c, d}})
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, map[uint]int{a: 1, b: 2, c: 3, d: 4}, sortOrders(t))
	})
}

// --- AC6 + AC9 (set-default) ---------------------------------------------------------------------

func TestSetDefault(t *testing.T) {
	a, b, c, d := resetCatalog(t)
	r := newRouter(1)
	set := func(id uint) resp { return call(r, "PUT", path(id, "/set-default"), nil) }
	defaults := func() []uint {
		var ids []uint
		require.NoError(t, testDB.Raw("SELECT id FROM customer_stage WHERE is_default = 1").Scan(&ids).Error)
		return ids
	}

	t.Run("bước ẩn ⇒ 422, mặc định cũ giữ nguyên", func(t *testing.T) {
		require.NoError(t, testDB.Exec("UPDATE customer_stage SET status = 0 WHERE id = ?", b).Error)
		w := set(b)
		require.Equal(t, 422, w.Code, w.Body.String())
		require.Contains(t, w.message(t), "đang ẩn")
		require.Equal(t, []uint{a}, defaults())
		require.NoError(t, testDB.Exec("UPDATE customer_stage SET status = 1 WHERE id = ?", b).Error)
	})
	t.Run("type thành công / thất bại ⇒ 422", func(t *testing.T) {
		for _, id := range []uint{c, d} {
			w := set(id)
			require.Equal(t, 422, w.Code, w.Body.String())
			require.Contains(t, w.message(t), "phải thuộc loại đang mở")
			require.Equal(t, []uint{a}, defaults())
		}
	})
	t.Run("hợp lệ ⇒ đúng 1 dòng is_default=1 và là bước đó + bump", func(t *testing.T) {
		resetDexie(t)
		w := set(b)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, []uint{b}, defaults())
		require.EqualValues(t, b, w.json(t)["data"].(map[string]any)["id"])
		require.EqualValues(t, 1, w.json(t)["data"].(map[string]any)["is_default"])
		require.NotZero(t, dexieVersion(t))
	})
	t.Run("đặt lại chính bước đang mặc định ⇒ 200, vẫn đúng 1 mặc định", func(t *testing.T) {
		require.Equal(t, 200, set(b).Code)
		require.Equal(t, []uint{b}, defaults())
	})
	t.Run("chuyển lại sang bước khác", func(t *testing.T) {
		require.Equal(t, 200, set(a).Code)
		require.Equal(t, []uint{a}, defaults())
	})
	t.Run("sau khi đổi, bước mặc định cũ được ẩn/xoá bình thường", func(t *testing.T) {
		require.Equal(t, 200, set(b).Code)
		w := call(r, "PUT", path(a, ""), map[string]any{"name": "Liên hệ", "type": 1, "color": "gray", "status": 0})
		require.Equal(t, 200, w.Code, w.Body.String())
	})
	t.Run("lỗi không bump Dexie", func(t *testing.T) {
		resetDexie(t)
		require.Equal(t, 422, set(c).Code)
		require.Zero(t, dexieVersion(t))
	})
	t.Run("id không tồn tại ⇒ 404", func(t *testing.T) {
		require.Equal(t, 404, set(999999).Code)
	})
}

// --- set-default song song (QA pass 1 F1) --------------------------------------------------------

// testhelpers đặt SetMaxOpenConns(1) ⇒ mọi request của suite chạy NỐI TIẾP và không bao giờ thấy deadlock.
// Test này mở pool RIÊNG (nhiều kết nối) và bắn set-default song song cho nhiều bước KHÁC nhau qua handler thật.
func TestSetDefault_ConcurrentDifferentStages_NoDeadlock(t *testing.T) {
	a, b, _, _ := resetCatalog(t)
	now := time.Now().Unix()
	extra := []uint{}
	for _, name := range []string{"Mở E", "Mở F", "Mở G"} {
		require.NoError(t, testDB.Exec(
			"INSERT INTO customer_stage (name, type, color, sort_order, is_default, status, created_at, updated_at) VALUES (?, 1, 'gray', 9, 0, 1, ?, ?)",
			name, now, now).Error)
		extra = append(extra, testhelpers.LastInsertID(testDB))
	}
	candidates := append([]uint{a, b}, extra...)

	pool, err := gorm.Open(mysql.Open(os.Getenv("TEST_MYSQL_DSN")), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	sqlDB, err := pool.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(len(candidates) + 2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := newRouterOn(pool, 1)

	const rounds = 30
	var non200 int
	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup
		codes := make([]int, len(candidates))
		start := make(chan struct{})
		for i, id := range candidates {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				codes[i] = call(r, "PUT", path(id, "/set-default"), nil).Code
			}()
		}
		close(start)
		wg.Wait()
		for _, code := range codes {
			if code != 200 {
				non200++
				t.Errorf("vòng %d: set-default song song trả %d (kỳ vọng 200, không deadlock 500): %v", round, code, codes)
				break
			}
		}
		require.EqualValues(t, 1, count(t, "SELECT COUNT(*) FROM customer_stage WHERE is_default = 1"),
			"vòng %d: luôn đúng 1 bước mặc định", round)
	}
	require.Zero(t, non200)
}

// TestSetDefault_ConcurrentWithHideOrDelete_KeepsInvariant canh bất biến "bước mặc định phải đang hiện + loại mở"
// khi set-default chạy SONG SONG với ẩn / xoá chính bước đó (khoá cả danh mục trong service.SetDefault).
// Pool riêng + FK tắt ở mức session (xoá bước không đụng customer/history; tránh bug mysqld ở FK bật).
func TestSetDefault_ConcurrentWithHideOrDelete_KeepsInvariant(t *testing.T) {
	a, b, _, _ := resetCatalog(t)
	pool, err := gorm.Open(mysql.Open(os.Getenv("TEST_MYSQL_DSN")+"&foreign_key_checks=0"), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	sqlDB, err := pool.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(6)
	t.Cleanup(func() { _ = sqlDB.Close() })
	r := newRouterOn(pool, 1)
	const rounds = 30
	now := time.Now().Unix()

	assertInvariant := func(t *testing.T, round int, what string, codes ...int) {
		for _, code := range codes {
			require.Less(t, code, 500, "vòng %d (%s): có 5xx %v", round, what, codes)
		}
		require.EqualValues(t, 1, count(t, "SELECT COUNT(*) FROM customer_stage WHERE is_default = 1"), "vòng %d (%s): đúng 1 mặc định", round, what)
		require.EqualValues(t, 1, count(t, "SELECT COUNT(*) FROM customer_stage WHERE is_default = 1 AND status = 1 AND type = 1"),
			"vòng %d (%s): bước mặc định phải đang hiện + loại mở (codes=%v)", round, what, codes)
	}
	race := func(fns ...func() int) []int {
		codes := make([]int, len(fns))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i, fn := range fns {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; codes[i] = fn() }()
		}
		close(start)
		wg.Wait()
		return codes
	}

	t.Run("set-default B ‖ ẩn B", func(t *testing.T) {
		for round := 0; round < rounds; round++ {
			// dựng lại: a mặc định, b hiện + mở (2 lệnh vì UNIQUE default_key kiểm từng dòng)
			require.NoError(t, testDB.Exec("UPDATE customer_stage SET is_default = 0").Error)
			require.NoError(t, testDB.Exec("UPDATE customer_stage SET is_default = 1, status = 1 WHERE id = ?", a).Error)
			require.NoError(t, testDB.Exec("UPDATE customer_stage SET status = 1 WHERE id = ?", b).Error)
			codes := race(
				func() int { return call(r, "PUT", path(b, "/set-default"), nil).Code },
				func() int {
					return call(r, "PUT", path(b, ""), map[string]any{"name": "Đang tư vấn", "type": 1, "color": "blue", "status": 0}).Code
				},
			)
			assertInvariant(t, round, "set-default ‖ ẩn", codes...)
			require.False(t, codes[0] == 200 && codes[1] == 200 && count(t, "SELECT COUNT(*) FROM customer_stage WHERE id = ? AND status = 0 AND is_default = 1", b) == 1,
				"vòng %d: vừa đặt mặc định vừa ẩn cùng một bước", round)
		}
	})

	t.Run("set-default X ‖ xoá X", func(t *testing.T) {
		for round := 0; round < rounds; round++ {
			require.NoError(t, testDB.Exec("UPDATE customer_stage SET is_default = 0").Error)
			require.NoError(t, testDB.Exec("UPDATE customer_stage SET is_default = 1, status = 1 WHERE id = ?", a).Error)
			require.NoError(t, testDB.Exec(
				"INSERT INTO customer_stage (name, type, color, sort_order, is_default, status, created_at, updated_at) VALUES (?, 1, 'gray', 50, 0, 1, ?, ?)",
				fmt.Sprintf("Tạm %d", round), now, now).Error)
			x := testhelpers.LastInsertID(testDB)
			codes := race(
				func() int { return call(r, "PUT", path(x, "/set-default"), nil).Code },
				func() int { return call(r, "DELETE", path(x, ""), nil).Code },
			)
			// Cả hai cùng 200 ⇒ mặc định mất (bị xoá) ⇒ assertInvariant (đúng 1 mặc định) bắt được.
			assertInvariant(t, round, "set-default ‖ xoá", codes...)
		}
	})
}

// --- AC7 -----------------------------------------------------------------------------------------

func TestDexieSync_ReturnsHiddenStages(t *testing.T) {
	a, b, _, _ := resetCatalog(t)
	require.NoError(t, testDB.Exec("UPDATE customer_stage SET status = 0 WHERE id = ?", b).Error)
	r := newRouter(1)

	w := call(r, "GET", "/api/crm/indexeddb/sync?tables=customerStages", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var out map[string][]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	got := out["customerStages"]
	require.Len(t, got, 4, "cả bước ẩn")
	require.EqualValues(t, a, got[0]["id"], "sort_order ASC, id ASC")
	var hiddenSeen bool
	for _, row := range got {
		if uint(row["id"].(float64)) == b {
			hiddenSeen = true
			require.EqualValues(t, 0, row["status"])
		}
		for _, k := range []string{"id", "name", "type", "color", "sort_order", "is_default", "status"} {
			require.Contains(t, row, k)
		}
	}
	require.True(t, hiddenSeen)
}

// --- AC9 -----------------------------------------------------------------------------------------

func TestPathID_NonNumericIs404_AndNeverTouchesRealRecord(t *testing.T) {
	requireDB(t)
	a, _, _, _ := resetCatalog(t)
	// Bước id=5 THẬT: nếu '5abc' bị MySQL ép về 5 thì mọi lệnh dưới sẽ trúng nó.
	require.NoError(t, testDB.Exec("INSERT INTO customer_stage (id, name, type, color, sort_order, is_default, status) VALUES (5, 'Bước Năm', 1, 'gray', 5, 0, 1)").Error)
	r := newRouter(1)
	_ = a

	for _, tc := range []struct{ method, p string }{
		{"GET", "/api/config/customer-stage/5abc"},
		{"PUT", "/api/config/customer-stage/5abc"},
		{"DELETE", "/api/config/customer-stage/5abc"},
		{"PUT", "/api/config/customer-stage/5abc/set-default"},
		{"GET", "/api/config/customer-stage/0"},
		{"GET", "/api/config/customer-stage/-1"},
	} {
		w := call(r, tc.method, tc.p, map[string]any{"name": "HACK", "type": 1, "color": "gray", "status": 0})
		require.Equal(t, 404, w.Code, "%s %s → %s", tc.method, tc.p, w.Body.String())
	}
	got := rowByName(t, "Bước Năm")
	require.Equal(t, 1, got.Status)
	require.Equal(t, 0, got.IsDefault)
	require.EqualValues(t, 0, count(t, "SELECT COUNT(*) FROM customer_stage WHERE name = 'HACK'"))
}
