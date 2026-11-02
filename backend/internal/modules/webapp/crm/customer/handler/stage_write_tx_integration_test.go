//go:build integration

package handler

// Story-03 QA pass 1 (P1) — đường TẠO khách: lịch sử bước khởi tạo phải ghi CÙNG transaction với INSERT
// khách. Lỗi ở bất kỳ bước nào sau INSERT khách ⇒ rollback cả khách lẫn lịch sử; không còn lịch sử mồ côi,
// không còn khách thiếu lịch sử. Ép lỗi bằng callback GORM `Create` chặn đúng một bảng (code sản phẩm
// không đổi). Lưới này đỏ khi lịch sử bị ghi sau Commit / ngoài tx / nuốt lỗi.

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/logger"
)

// failCreateOn đăng ký callback chặn mọi INSERT vào `table` trên testDB; trả con trỏ đếm số lần chặn.
func failCreateOn(t *testing.T, table string) *int {
	t.Helper()
	name := "s03_fail_create_" + table
	hits := new(int)
	require.NoError(t, testDB.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == table {
			*hits++
			_ = tx.AddError(fmt.Errorf("s03 boom: insert %s", table))
		}
	}))
	t.Cleanup(func() { _ = testDB.Callback().Create().Remove(name) })
	return hits
}

// response.ServerError ghi log qua logger.L — nil trong test ⇒ panic.
func ensureLogger() {
	if logger.L == nil {
		logger.L = zap.NewNop()
	}
}

func totalStageHistory(t *testing.T) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testDB.Table("customer_stage_history").Count(&n).Error)
	return n
}

// (a) INSERT lịch sử lỗi ⇒ CreateCustomer 500 và KHÔNG có dòng khách.
func TestCreateCustomer_HistoryInsertFails_NoCustomer(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	ensureLogger()
	tn := testhelpers.FreshTenant(t, testDB)
	t.Cleanup(func() { cleanupS33Customers(tn) })
	uid := testhelpers.CreateScopedUser(t, testDB, "s03tx", tn, tn.BranchID)
	hits := failCreateOn(t, "customer_stage_history")

	name := fmt.Sprintf("S33 S03tx hist %d", time.Now().UnixNano())
	w := callCreateCustomer(t, uid, map[string]any{"fullname": name, "branch_id": tn.BranchID})
	require.Equal(t, 500, w.Code, "body: %s", w.Body.String())
	require.Equal(t, 1, *hits, "đối chứng: đường ghi lịch sử CÓ được gọi")
	var n int64
	require.NoError(t, testDB.Table("customer").Where("fullname = ?", name).Count(&n).Error)
	require.Zero(t, n, "lịch sử lỗi ⇒ khách phải rollback")
}

// (b) Tạo khách kèm hẹn, INSERT appointment lỗi (sau khi đã ghi lịch sử trong tx) ⇒ 500, không khách,
// tổng số dòng lịch sử KHÔNG đổi (lịch sử rollback cùng khách).
func TestCreateCustomer_AppointmentInsertFails_NoOrphanHistory(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	ensureLogger()
	tn := testhelpers.FreshTenant(t, testDB)
	t.Cleanup(func() { cleanupS33Customers(tn) })
	seedS33AppointmentConfigCode(t, tn)
	uid := testhelpers.CreateScopedUser(t, testDB, "s03tx", tn, tn.BranchID)
	hits := failCreateOn(t, "appointment")

	before := totalStageHistory(t)
	for _, d := range []int64{time.Now().Unix() + 86400, time.Now().Unix() - 3600} {
		name := fmt.Sprintf("S33 S03tx appt %d", time.Now().UnixNano())
		w := callCreateCustomer(t, uid, map[string]any{"fullname": name, "branch_id": tn.BranchID, "appointment_date": d})
		require.Equal(t, 500, w.Code, "body: %s", w.Body.String())
		var n int64
		require.NoError(t, testDB.Table("customer").Where("fullname = ?", name).Count(&n).Error)
		require.Zero(t, n, "appointment lỗi ⇒ khách rollback")
	}
	require.Equal(t, 2, *hits, "đối chứng: đường tạo lịch hẹn CÓ được gọi")
	require.Equal(t, before, totalStageHistory(t), "không được còn lịch sử mồ côi")
}
