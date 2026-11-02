//go:build integration

package handler

// Story-05 (customer-stage-pipeline) AC1 — dịch vụ đã dùng kèm lịch hẹn (ListServiceUsedWithAppointment)
// attach `customer.stage_id`.

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"erpCluvix/internal/testhelpers"
)

func TestListServiceUsedWithAppointment_CustomerCoStageID(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	var stages []int
	require.NoError(t, testDB.Table("customer_stage").Order("id").Limit(1).Pluck("id", &stages).Error)
	require.Len(t, stages, 1, "DB test phải có bước seed")
	st := stages[0]

	tn := testhelpers.FreshTenant(t, testDB)
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ? WHERE id = ?", st, tn.CustomerID).Error)
	day := time.Date(2033, 3, 10, 5, 0, 0, 0, time.UTC).Unix()
	apptID := seedApptS37(t, tn, tn.BranchID, day)
	suID := seedOrderServiceUsed(t, tn, map[string]any{"appointment_id": apptID})

	items := listSUWithAppt(t, tn, tn.BranchID, []uint{tn.BranchID}, day-86400, day+86400)
	var found map[string]any
	for _, it := range items {
		if uint(it["id"].(float64)) == suID {
			found = it
		}
	}
	require.NotNil(t, found, "đối chứng: dòng dịch vụ seed phải có trong list")
	cu, ok := found["customer"].(map[string]any)
	require.True(t, ok, "phải attach customer, item=%v", found)
	require.Equal(t, fmt.Sprint(st), fmt.Sprint(cu["stage_id"]))
}
