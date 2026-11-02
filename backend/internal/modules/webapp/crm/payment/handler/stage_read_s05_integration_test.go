//go:build integration

package handler

// Story-05 (customer-stage-pipeline) AC1 — danh sách phiếu thu/chi (ListReceipts) attach `customer.stage_id`.

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

func TestListReceipts_CustomerCoStageID(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	var stages []int
	require.NoError(t, testDB.Table("customer_stage").Order("id").Limit(1).Pluck("id", &stages).Error)
	require.Len(t, stages, 1, "DB test phải có bước seed")
	st := stages[0]

	tn := testhelpers.FreshTenant(t, testDB)
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ? WHERE id = ?", st, tn.CustomerID).Error)
	id, _ := seedListPayment(t, tn, listPayRow{branchID: tn.BranchID, amount: 100_000, paymentFor: define.PaymentForOrder})

	h := New(Deps{DB: testDB})
	code, body := callPaymentList(t, tn.BranchID, h.ListReceipts, "per-page=50")
	require.Equal(t, 200, code, "body=%v", body)
	items := body["object"].(map[string]any)["items"].([]any)
	var found map[string]any
	for _, it := range items {
		m := it.(map[string]any)
		if fmt.Sprint(m["id"]) == fmt.Sprint(id) {
			found = m
		}
	}
	require.NotNil(t, found, "đối chứng: phiếu seed phải có trong list")
	cu, ok := found["customer"].(map[string]any)
	require.True(t, ok, "phải attach customer, item=%v", found)
	require.Equal(t, fmt.Sprint(st), fmt.Sprint(cu["stage_id"]))
}
