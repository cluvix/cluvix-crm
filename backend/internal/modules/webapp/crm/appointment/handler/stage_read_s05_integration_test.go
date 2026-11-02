//go:build integration

package handler

// Story-05 (customer-stage-pipeline) — lịch hẹn đọc/lọc bước khách (`customer.stage_id`):
//   - AC1: list + detail attach `customer.stage_id`.
//   - AC2: `filter[_stage_id][i]` (đa giá trị ⇒ IN) chỉ trả lịch hẹn của khách ở các bước đó; kéo theo
//     LEFT JOIN customer (customerFilterColumns).
//   - AC4: khoá cũ `filter[_status]` KHÔNG còn lọc tiến trình cũ của khách; `filter[status]` của chính bảng
//     appointment giữ nguyên.

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"erpCluvix/internal/testhelpers"
)

// twoStageIDsS05 — 2 bước khác nhau trong bảng seed `customer_stage`.
func twoStageIDsS05(t *testing.T) (int, int) {
	t.Helper()
	var ids []int
	require.NoError(t, testDB.Table("customer_stage").Order("id").Limit(2).Pluck("id", &ids).Error)
	require.Len(t, ids, 2, "DB test phải có ≥ 2 bước seed")
	return ids[0], ids[1]
}

type stageApptList struct {
	ids   []string
	items map[string]map[string]any
}

// listApptRaw gọi ListAppointment với query thô, phạm vi chi nhánh = branch.
func listApptRaw(t *testing.T, branch uint, rawQuery string) stageApptList {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/crm/appointment?per-page=50&"+rawQuery, nil)
	c.Set("branchActiveId", branch)
	c.Set("branchIds", []uint{branch})
	c.Set("userID", uint(1))
	(&Handler{DB: testDB}).ListAppointment(c)
	require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
	var body struct {
		Items []map[string]any `json:"items"`
		Meta  struct {
			TotalCount int64 `json:"totalCount"`
		} `json:"_meta"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "body: %s", w.Body.String())
	require.EqualValues(t, len(body.Items), body.Meta.TotalCount, "total khớp số dòng")
	out := stageApptList{items: map[string]map[string]any{}}
	for _, it := range body.Items {
		id := fmt.Sprint(it["id"])
		out.ids = append(out.ids, id)
		out.items[id] = it
	}
	sort.Strings(out.ids)
	return out
}

func TestListAppointment_StageIDFilterVaAttach(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	stA, stB := twoStageIDsS05(t)
	tn := testhelpers.FreshTenant(t, testDB)
	tn2 := testhelpers.FreshTenant(t, testDB) // chỉ mượn khách thứ 2 (khách không gắn chi nhánh)
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ? WHERE id = ?", stA, tn.CustomerID).Error)
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ? WHERE id = ?", stB, tn2.CustomerID).Error)

	apptA := seedAppointment(t, tn, nil)
	apptB := seedAppointment(t, tn, map[string]any{"customer_id": tn2.CustomerID})
	idA, idB := fmt.Sprint(apptA), fmt.Sprint(apptB)
	both := []string{idA, idB}
	sort.Strings(both)

	// Không lọc ⇒ cả hai; AC1: mỗi dòng có customer.stage_id đúng của khách.
	all := listApptRaw(t, tn.BranchID, "")
	require.Equal(t, both, all.ids)
	custOf := func(id string) map[string]any {
		cu, ok := all.items[id]["customer"].(map[string]any)
		require.True(t, ok, "phải attach customer, item=%v", all.items[id])
		return cu
	}
	require.Contains(t, custOf(idA), "stage_id", "list phải trả customer.stage_id")
	require.Equal(t, fmt.Sprint(stA), fmt.Sprint(custOf(idA)["stage_id"]))
	require.Equal(t, fmt.Sprint(stB), fmt.Sprint(custOf(idB)["stage_id"]))

	// AC2: 1 giá trị.
	require.Equal(t, []string{idA}, listApptRaw(t, tn.BranchID, fmt.Sprintf("filter[_stage_id][0]=%d", stA)).ids)
	require.Equal(t, []string{idB}, listApptRaw(t, tn.BranchID, fmt.Sprintf("filter[_stage_id]=%d", stB)).ids)
	// AC2: đa giá trị (cú pháp FE `kind: 'in'`).
	multi := url.Values{"filter[_stage_id][0]": {fmt.Sprint(stA)}, "filter[_stage_id][1]": {fmt.Sprint(stB)}}
	require.Equal(t, both, listApptRaw(t, tn.BranchID, multi.Encode()).ids)
	// Giá trị rác ⇒ rỗng (không đọc trúng khách `5abc` → 5).
	require.Empty(t, listApptRaw(t, tn.BranchID, fmt.Sprintf("filter[_stage_id]=%dabc", stA)).ids)

	// AC4: `_status` cũ không còn lọc tiến trình khách (giá trị 999 không khớp khách nào mà vẫn đủ 2 dòng).
	require.Equal(t, both, listApptRaw(t, tn.BranchID, "filter[_status]=999").ids)
	// `filter[status]` của chính appointment (status=1) giữ nguyên.
	require.Equal(t, both, listApptRaw(t, tn.BranchID, "filter[status]=1").ids)
	require.Empty(t, listApptRaw(t, tn.BranchID, "filter[status]=999").ids)
}

func TestGetAppointment_CustomerCoStageID(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	stA, _ := twoStageIDsS05(t)
	tn := testhelpers.FreshTenant(t, testDB)
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ? WHERE id = ?", stA, tn.CustomerID).Error)
	apptID := seedAppointment(t, tn, nil)

	code, body := callApptDetail(t, tn.BranchID, fmt.Sprintf("/%d", apptID))
	require.Equal(t, 200, code)
	cu, ok := body["data"].(map[string]any)["customer"].(map[string]any)
	require.True(t, ok, "phải attach customer")
	require.Equal(t, fmt.Sprint(stA), fmt.Sprint(cu["stage_id"]), "AC1: chi tiết lịch hẹn trả customer.stage_id")
}
