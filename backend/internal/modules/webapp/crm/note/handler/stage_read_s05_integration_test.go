//go:build integration

package handler

// Story-05 (customer-stage-pipeline) — báo cáo ghi chú (GetAllNote) đọc/lọc bước khách:
//   - AC1: customer attach có `stage_id`.
//   - AC3: `filter[_stage_id]` lọc đúng theo `customer.stage_id`; ghi chú KHÔNG gắn khách không làm lỗi query.
//   - AC4: khoá cũ `filter[status]` không còn lọc tiến trình cũ của khách.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"erpCluvix/internal/testhelpers"
)

func TestGetAllNote_StageIDFilterVaAttach(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	var stages []int
	require.NoError(t, testDB.Table("customer_stage").Order("id").Limit(2).Pluck("id", &stages).Error)
	require.Len(t, stages, 2, "DB test phải có ≥ 2 bước seed")
	stA, stB := stages[0], stages[1]

	tn := testhelpers.FreshTenant(t, testDB)
	tn2 := testhelpers.FreshTenant(t, testDB)
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ? WHERE id = ?", stA, tn.CustomerID).Error)
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ? WHERE id = ?", stB, tn2.CustomerID).Error)

	stamp := time.Now().UnixNano()
	noteA := seedNote(t, tn, tn.CustomerID, fmt.Sprintf("S05 A %d", stamp))
	noteB := seedNote(t, tn2, tn2.CustomerID, fmt.Sprintf("S05 B %d", stamp))
	// Ghi chú không gắn khách (customer_id NULL): LEFT JOIN customer không được làm vỡ query.
	require.NoError(t, testDB.Table("note").Create(map[string]any{
		"note": fmt.Sprintf("S05 NULL %d", stamp), "created_at": time.Now().Unix(),
	}).Error)
	var noteNull uint
	require.NoError(t, testDB.Table("note").Where("note = ?", fmt.Sprintf("S05 NULL %d", stamp)).Pluck("id", &noteNull).Error)
	t.Cleanup(func() { testDB.Exec("DELETE FROM note WHERE id IN (?, ?, ?)", noteA, noteB, noteNull) })

	mine := map[string]uint{"A": noteA, "B": noteB, "NULL": noteNull}
	// call trả id (CHỈ trong 3 ghi chú seed — DB test có thể còn ghi chú khác) + từng item.
	call := func(raw string) (map[string]bool, map[uint]map[string]any) {
		c, w := newNoteCtx(t, "GET", "/crm/note/get-all-note?per-page=200&"+raw)
		c.Set("branchActiveId", tn.BranchID)
		c.Set("branchIds", []uint{tn.BranchID})
		(&Handler{DB: testDB}).GetAllNote(c)
		require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
		var body struct {
			Items []map[string]any `json:"items"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		got := map[string]bool{}
		items := map[uint]map[string]any{}
		for _, it := range body.Items {
			id := uint(it["id"].(float64))
			for k, v := range mine {
				if v == id {
					got[k] = true
					items[id] = it
				}
			}
		}
		return got, items
	}
	keys := func(m map[string]bool) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}

	// Không lọc ⇒ đủ 3 (kể cả ghi chú NULL); AC1: customer.stage_id.
	got, items := call("")
	require.Equal(t, []string{"A", "B", "NULL"}, keys(got))
	cu, ok := items[noteA]["customer"].(map[string]any)
	require.True(t, ok, "phải attach customer, item=%v", items[noteA])
	require.Equal(t, fmt.Sprint(stA), fmt.Sprint(cu["stage_id"]))

	// AC3: lọc 1 bước / đa giá trị. Ghi chú NULL bị loại khi lọc theo bước (không có khách ⇒ không khớp).
	got, _ = call(fmt.Sprintf("filter[_stage_id][0]=%d", stA))
	require.Equal(t, []string{"A"}, keys(got))
	got, _ = call(fmt.Sprintf("filter[_stage_id]=%d", stB))
	require.Equal(t, []string{"B"}, keys(got))
	multi := url.Values{"filter[_stage_id][0]": {fmt.Sprint(stA)}, "filter[_stage_id][1]": {fmt.Sprint(stB)}}
	got, _ = call(multi.Encode())
	require.Equal(t, []string{"A", "B"}, keys(got))
	got, _ = call(fmt.Sprintf("filter[_stage_id]=%dabc", stA))
	require.Empty(t, got, "giá trị rác ⇒ rỗng, không đọc trúng bước %d", stA)

	// AC4: `filter[status]` cũ không còn lọc tiến trình khách (999 không khớp khách nào mà vẫn đủ 3).
	got, _ = call("filter[status]=999")
	require.Equal(t, []string{"A", "B", "NULL"}, keys(got))
}
