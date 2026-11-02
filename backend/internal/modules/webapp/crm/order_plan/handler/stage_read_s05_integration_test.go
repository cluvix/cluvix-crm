//go:build integration

package handler

// Story-05 (customer-stage-pipeline) AC1 — danh sách đơn (ListOrderPlan) attach `customer.stage_id`.

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"erpCluvix/internal/testhelpers"
)

func TestListOrderPlan_CustomerCoStageID(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	var stages []int
	require.NoError(t, testDB.Table("customer_stage").Order("id").Limit(1).Pluck("id", &stages).Error)
	require.Len(t, stages, 1, "DB test phải có bước seed")
	st := stages[0]

	tn := testhelpers.FreshTenant(t, testDB)
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ? WHERE id = ?", st, tn.CustomerID).Error)
	orderID := seedOrderPlan(t, tn)

	code, body := callList(t, tn.BranchID, url.Values{"filter[customer_id][in][]": {fmt.Sprint(tn.CustomerID)}})
	require.Equal(t, 200, code, "body=%v", body)
	items, ok := body["items"].([]any)
	require.True(t, ok, "body=%v", body)
	var found map[string]any
	for _, it := range items {
		m := it.(map[string]any)
		if fmt.Sprint(m["id"]) == fmt.Sprint(orderID) {
			found = m
		}
	}
	require.NotNil(t, found, "đối chứng: đơn seed phải có trong list")
	cu, ok := found["customer"].(map[string]any)
	require.True(t, ok, "phải attach customer, item=%v", found)
	require.Equal(t, fmt.Sprint(st), fmt.Sprint(cu["stage_id"]))
}
