//go:build integration

package handler

// Story-05 (customer-stage-pipeline) — nhắc nhở: customer lồng (list) và customer sau Get có `stage_id`.
// Danh sách đóng gói từ cột phẳng `customer_stage_id` rồi xoá cột phẳng — phải CÓ trong object lồng và
// KHÔNG còn ở gốc item.

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"erpCluvix/internal/testhelpers"
)

func TestReminder_CustomerCoStageID(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	var stages []int
	require.NoError(t, testDB.Table("customer_stage").Order("id").Limit(1).Pluck("id", &stages).Error)
	require.Len(t, stages, 1, "DB test phải có bước seed")
	st := stages[0]

	tn := testhelpers.FreshTenant(t, testDB)
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ? WHERE id = ?", st, tn.CustomerID).Error)
	now := time.Now().Unix()
	code := fmt.Sprintf("NV%d", time.Now().UnixNano())
	require.NoError(t, testDB.Table("reminder").Create(map[string]any{
		"code": code, "customer_id": tn.CustomerID, "start_time": now + 3600, "type": 1, "status": 1,
		"note": "s05", "created_at": now, "updated_at": now,
	}).Error)
	var rid uint
	require.NoError(t, testDB.Table("reminder").Where("code = ?", code).Pluck("id", &rid).Error)
	t.Cleanup(func() { testDB.Exec("DELETE FROM reminder WHERE id = ?", rid) })

	scoped := func(c *gin.Context) {
		c.Set("branchActiveId", tn.BranchID)
		c.Set("branchIds", []uint{tn.BranchID})
		c.Set("userID", uint(1))
	}
	h := &Handler{DB: testDB}

	t.Run("List", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", fmt.Sprintf("/crm/reminder?per-page=50&filter[customer_id]=%d", tn.CustomerID), nil)
		scoped(c)
		h.ListReminder(c)
		require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
		var body struct {
			Items []map[string]any `json:"items"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Len(t, body.Items, 1)
		it := body.Items[0]
		cu, ok := it["customer"].(map[string]any)
		require.True(t, ok, "item=%v", it)
		require.Equal(t, fmt.Sprint(st), fmt.Sprint(cu["stage_id"]), "customer lồng phải có stage_id")
		require.NotContains(t, it, "customer_stage_id", "cột phẳng phải được xoá sau khi đóng gói")
		require.NotContains(t, it, "customer_status")
	})

	t.Run("Get", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", fmt.Sprintf("/crm/reminder/%d", rid), nil)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(rid)}}
		scoped(c)
		h.GetReminder(c)
		require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
		var body struct {
			Data map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		cu, ok := body.Data["customer"].(map[string]any)
		require.True(t, ok, "data=%v", body.Data)
		require.Equal(t, fmt.Sprint(st), fmt.Sprint(cu["stage_id"]))
	})
}
