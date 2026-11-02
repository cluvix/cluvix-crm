//go:build integration

package testhelpers

import (
	"errors"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"erpCluvix/pkg/define"
)

// Story-11 (customer-stage-pipeline) AC1 — khách do fixture dựng phải có `stage_id` = bước mặc định
// (đọc từ DB) + đúng 1 dòng lịch sử khởi tạo; SetCustomerStage đổi được bước.

var stageFixtureDB *gorm.DB

func TestMain(m *testing.M) {
	s, err := New(MigrationsDir(), SchemaAll)
	if err != nil {
		if !errors.Is(err, ErrNoDSN) {
			log.Fatalf("testhelpers: %v", err)
		}
		log.Printf("testhelpers: %v — skipping integration tests", err)
		os.Exit(m.Run())
	}
	defer s.Close()
	stageFixtureDB = s.DB
	os.Exit(m.Run())
}

type stageFixtureRow struct {
	StageID *uint
}

func customerStage(t *testing.T, id uint) *uint {
	t.Helper()
	var rows []stageFixtureRow
	require.NoError(t, stageFixtureDB.Raw("SELECT stage_id FROM customer WHERE id = ?", id).Scan(&rows).Error)
	require.Len(t, rows, 1)
	return rows[0].StageID
}

func stageHistory(t *testing.T, customerID uint) (n int64, to uint, source int, fromNull bool) {
	t.Helper()
	type h struct {
		ToStageID   uint
		Source      int
		FromStageID *uint
	}
	var rows []h
	require.NoError(t, stageFixtureDB.Raw(
		"SELECT to_stage_id, source, from_stage_id FROM customer_stage_history WHERE customer_id = ?", customerID).Scan(&rows).Error)
	if len(rows) == 1 {
		return 1, rows[0].ToStageID, rows[0].Source, rows[0].FromStageID == nil
	}
	return int64(len(rows)), 0, 0, false
}

func TestFixtures_FreshTenantCustomerHasDefaultStageAndHistory(t *testing.T) {
	if stageFixtureDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}
	def, err := DefaultCustomerStageID(stageFixtureDB)
	require.NoError(t, err)

	tn := FreshTenant(t, stageFixtureDB)
	got := customerStage(t, tn.CustomerID)
	require.NotNil(t, got, "khách fixture phải có stage_id (không NULL)")
	require.Equal(t, def, *got, "stage_id = bước mặc định đọc từ DB")

	n, to, source, fromNull := stageHistory(t, tn.CustomerID)
	require.EqualValues(t, 1, n, "đúng 1 dòng lịch sử khởi tạo")
	require.Equal(t, def, to)
	require.Equal(t, define.CustomerStageHistorySourceCreate, source)
	require.True(t, fromNull, "from_stage_id = NULL (khởi tạo)")
}

func TestFixtures_CreateScopedCustomerHasDefaultStageAndHistory(t *testing.T) {
	if stageFixtureDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}
	def := MustDefaultCustomerStageID(t, stageFixtureDB)
	tn := FreshTenant(t, stageFixtureDB)

	id := uint(CreateScopedCustomer(t, stageFixtureDB, tn, nil))
	got := customerStage(t, id)
	require.NotNil(t, got)
	require.Equal(t, def, *got)
	n, to, _, _ := stageHistory(t, id)
	require.EqualValues(t, 1, n)
	require.Equal(t, def, to)
}

func TestFixtures_SetCustomerStageChangesStageOnly(t *testing.T) {
	if stageFixtureDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}
	tn := FreshTenant(t, stageFixtureDB)
	var other uint
	require.NoError(t, stageFixtureDB.Raw("SELECT id FROM customer_stage WHERE is_default = 0 ORDER BY id LIMIT 1").Scan(&other).Error)
	require.NotZero(t, other, "seed migration phải có bước không mặc định")

	require.NoError(t, SetCustomerStage(stageFixtureDB, tn.CustomerID, other))
	got := customerStage(t, tn.CustomerID)
	require.NotNil(t, got)
	require.Equal(t, other, *got)
}
