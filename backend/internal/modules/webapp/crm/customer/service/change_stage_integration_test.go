//go:build integration

package service

// Story-03 (customer-stage-pipeline) — CustomerService.ChangeStage:
//   AC5 nhảy cóc / đi lùi / từ bước thất bại về bước mở · AC6 cùng bước = no-op · AC7 bước ẩn/không
//   tồn tại/ngoài phạm vi không ghi gì · AC8 INSERT lịch sử lỗi ⇒ rollback stage_id · AC9 đồng thời ⇒
//   chuỗi lịch sử nhất quán (khoá dòng FOR UPDATE).

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	stagesvc "erpCluvix/internal/modules/webapp/config/customer_stage/service"
	"erpCluvix/internal/modules/webapp/crm/customer/scope"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/optlock"
	"erpCluvix/pkg/tenant"
)

func csStageID(t *testing.T, db *gorm.DB, name string) int {
	t.Helper()
	var id int
	require.NoError(t, db.Raw("SELECT id FROM customer_stage WHERE name = ?", name).Scan(&id).Error)
	require.NotZero(t, id, "thiếu bước seed %q", name)
	return id
}

// csScoped — query khách đã scope y như handler ChangeStage dựng: customerByIDScope (chi nhánh theo option
// cross-branch id=18) + scope.Apply (telesale). Lỗi đọc phạm vi ⇒ gắn vào query (service fail-closed).
func csScoped(c *gin.Context, db *gorm.DB) *gorm.DB {
	q := tenant.ScopedDBNullBranchSkipBranch(c, db, "customer", tenant.IsCrossBranchEnabled(c, db)).Table("customer")
	scoped, err := scope.Apply(c, db, q)
	if err != nil {
		_ = scoped.AddError(err)
	}
	return scoped
}

func csSeedCustomer(t *testing.T, branchID *uint, stageID int, paymentUserID *uint) int {
	t.Helper()
	row := map[string]any{
		"fullname":        fmt.Sprintf("S03 Svc %d", time.Now().UnixNano()),
		"stage_id":        stageID,
		"payment_user_id": paymentUserID,
		"created_at":      1000,
		"updated_at":      1000,
	}
	if branchID != nil {
		row["branch_id"] = *branchID
	}
	require.NoError(t, testDB.Table("customer").Create(&row).Error)
	id := int(testhelpers.LastInsertID(testDB))
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", id)
		testDB.Exec("DELETE FROM customer WHERE id = ?", id)
	})
	return id
}

type csHist struct {
	ID          int
	FromStageID *int `gorm:"column:from_stage_id"`
	ToStageID   int  `gorm:"column:to_stage_id"`
	Source      int
	ChangedBy   *int `gorm:"column:changed_by"`
}

func csHistory(t *testing.T, db *gorm.DB, customerID int) []csHist {
	t.Helper()
	rows := make([]csHist, 0)
	require.NoError(t, db.Table("customer_stage_history").Select("id, from_stage_id, to_stage_id, source, changed_by").
		Where("customer_id = ?", customerID).Order("id ASC").Find(&rows).Error)
	return rows
}

func csCustomer(t *testing.T, db *gorm.DB, id int) (stage *int, updatedAt int64) {
	t.Helper()
	var row struct {
		StageID   *int  `gorm:"column:stage_id"`
		UpdatedAt int64 `gorm:"column:updated_at"`
	}
	require.NoError(t, db.Table("customer").Select("stage_id, updated_at").Where("id = ?", id).Take(&row).Error)
	return row.StageID, row.UpdatedAt
}

// AC5 + AC6 — mọi hướng đi tới bước đang hiện đều được; cùng bước ⇒ no-op.
func TestChangeStage_AnyVisibleStage(t *testing.T) {
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := int(testhelpers.CreateScopedUser(t, testDB, "s03svc", tn, tn.BranchID))
	contact := csStageID(t, testDB, "Liên hệ")
	consulting := csStageID(t, testDB, "Đang tư vấn")
	negotiating := csStageID(t, testDB, "Đang đàm phán")
	rejected := csStageID(t, testDB, "Từ chối")
	id := csSeedCustomer(t, &tn.BranchID, contact, nil)
	svc := NewCustomerService(testDB)

	steps := []struct {
		name string
		to   int
	}{
		{"nhảy cóc Liên hệ → Đang đàm phán", negotiating},
		{"đi lùi → Đang tư vấn", consulting},
		{"sang bước thất bại", rejected},
		{"từ bước thất bại về bước đang mở", consulting},
	}
	prev := contact
	_, prevUpdated := csCustomer(t, testDB, id)
	for i, st := range steps {
		c := testhelpers.TenantCtx(tn.BranchID, uint(uid))
		item, err := svc.ChangeStage(csScoped(c, testDB), id, st.to, uid)
		require.NoError(t, err, st.name)
		require.EqualValues(t, id, item["id"])
		require.EqualValues(t, st.to, item["stage_id"])
		stage, updated := csCustomer(t, testDB, id)
		require.Equal(t, st.to, *stage, st.name)
		require.Greater(t, updated, prevUpdated, "%s: updated_at phải tăng", st.name)
		require.EqualValues(t, updated, item["updated_at"])
		hist := csHistory(t, testDB, id)
		require.Len(t, hist, i+1, st.name)
		last := hist[i]
		require.NotNil(t, last.FromStageID)
		require.Equal(t, prev, *last.FromStageID, "%s: from = bước cũ", st.name)
		require.Equal(t, st.to, last.ToStageID)
		require.Equal(t, define.CustomerStageHistorySourceManual, last.Source)
		require.NotNil(t, last.ChangedBy)
		require.Equal(t, uid, *last.ChangedBy)
		prev, prevUpdated = st.to, updated
	}

	// AC6 — cùng bước hiện tại.
	c := testhelpers.TenantCtx(tn.BranchID, uint(uid))
	item, err := svc.ChangeStage(csScoped(c, testDB), id, prev, uid)
	require.NoError(t, err)
	require.EqualValues(t, prev, item["stage_id"])
	_, updated := csCustomer(t, testDB, id)
	require.Equal(t, prevUpdated, updated, "cùng bước ⇒ updated_at không đổi")
	require.Len(t, csHistory(t, testDB, id), len(steps), "cùng bước ⇒ không có lịch sử mới")
}

// AC7 (tầng service) — bước ẩn / không tồn tại ⇒ LocalizedError; ngoài phạm vi ⇒ ErrNotFound; không ghi.
func TestChangeStage_RejectsWithoutWriting(t *testing.T) {
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := int(testhelpers.CreateScopedUser(t, testDB, "s03svc", tn, tn.BranchID))
	contact := csStageID(t, testDB, "Liên hệ")
	consulting := csStageID(t, testDB, "Đang tư vấn")
	now := time.Now().Unix()
	require.NoError(t, testDB.Exec(
		"INSERT INTO customer_stage (name, type, color, sort_order, is_default, status, created_at, updated_at) VALUES (?, 1, 'gray', 99, 0, 0, ?, ?)",
		fmt.Sprintf("S03 svc ẩn %d", time.Now().UnixNano()), now, now).Error)
	hidden := int(testhelpers.LastInsertID(testDB))
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer_stage WHERE id = ?", hidden) })
	other := testhelpers.CreateBranch(t, testDB)
	mine := csSeedCustomer(t, &tn.BranchID, contact, nil)
	foreign := csSeedCustomer(t, &other, contact, nil)
	svc := NewCustomerService(testDB)

	isNotSelectable := func(err error) bool {
		le, ok := errors.AsType[*errs.LocalizedError](err)
		return ok && le.Key == stagesvc.KeyNotSelectable
	}
	cases := []struct {
		name   string
		target int
		stage  int
		check  func(error) bool
	}{
		{"bước ẩn", mine, hidden, isNotSelectable},
		{"bước không tồn tại", mine, 99999999, isNotSelectable},
		{"stage_id <= 0", mine, 0, isNotSelectable},
		{"khách chi nhánh khác", foreign, consulting, func(err error) bool { return errors.Is(err, errs.ErrNotFound) }},
		{"khách không tồn tại", 99999999, consulting, func(err error) bool { return errors.Is(err, errs.ErrNotFound) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testhelpers.TenantCtx(tn.BranchID, uint(uid))
			_, err := svc.ChangeStage(csScoped(c, testDB), tc.target, tc.stage, uid)
			require.Error(t, err)
			require.True(t, tc.check(err), "lỗi sai loại: %v", err)
			if tc.target == 99999999 {
				return
			}
			stage, updated := csCustomer(t, testDB, tc.target)
			require.Equal(t, contact, *stage)
			require.EqualValues(t, 1000, updated)
			require.Empty(t, csHistory(t, testDB, tc.target))
		})
	}

	// Đọc phạm vi telesale lỗi (scope gắn lỗi vào query) ⇒ fail-closed, không ghi.
	c := testhelpers.TenantCtx(tn.BranchID, uint(uid))
	broken := csScoped(c, testDB)
	_ = broken.AddError(errors.New("scope boom"))
	_, err := svc.ChangeStage(broken, mine, consulting, uid)
	require.Error(t, err)
	stage, _ := csCustomer(t, testDB, mine)
	require.Equal(t, contact, *stage)
}

// AC8 — INSERT lịch sử lỗi ⇒ cả giao dịch rollback: customer.stage_id + updated_at không đổi.
// Ép lỗi bằng callback GORM chỉ chặn đúng INSERT vào customer_stage_history (code sản phẩm không đổi).
func TestChangeStage_HistoryInsertFails_RollsBack(t *testing.T) {
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := int(testhelpers.CreateScopedUser(t, testDB, "s03svc", tn, tn.BranchID))
	contact := csStageID(t, testDB, "Liên hệ")
	consulting := csStageID(t, testDB, "Đang tư vấn")
	id := csSeedCustomer(t, &tn.BranchID, contact, nil)

	const cbName = "s03_fail_stage_history_insert"
	boom := errors.New("boom: history insert")
	var attempts int
	require.NoError(t, testDB.Callback().Create().Before("gorm:create").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement.Table == "customer_stage_history" {
			attempts++
			_ = tx.AddError(boom)
		}
	}))
	t.Cleanup(func() { _ = testDB.Callback().Create().Remove(cbName) })

	c := testhelpers.TenantCtx(tn.BranchID, uint(uid))
	_, err := NewCustomerService(testDB).ChangeStage(csScoped(c, testDB), id, consulting, uid)
	require.ErrorIs(t, err, boom)
	require.Equal(t, 1, attempts, "đối chứng: đường ghi lịch sử CÓ được gọi")
	stage, updated := csCustomer(t, testDB, id)
	require.Equal(t, contact, *stage, "lịch sử lỗi ⇒ stage_id phải rollback")
	require.EqualValues(t, 1000, updated, "lịch sử lỗi ⇒ updated_at phải rollback")
	require.Empty(t, csHistory(t, testDB, id))
}

// AC9 — nhiều request đổi bước ĐỒNG THỜI cho cùng khách ⇒ chuỗi lịch sử liền mạch: from của dòng sau =
// to của dòng trước, dòng đầu from = bước ban đầu, dòng cuối to = stage_id hiện tại.
// testhelpers đặt SetMaxOpenConns(1) ⇒ PHẢI mở pool riêng nhiều kết nối, không thì các request chạy nối
// tiếp và test không chứng minh gì. Chạy: go test -race -p 1 -tags=integration -run ChangeStageConcurrent.
func TestChangeStageConcurrent_HistoryChainConsistent(t *testing.T) {
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	uid := int(testhelpers.CreateScopedUser(t, testDB, "s03svc", tn, tn.BranchID))
	contact := csStageID(t, testDB, "Liên hệ")
	targets := []int{
		csStageID(t, testDB, "Đang tư vấn"),
		csStageID(t, testDB, "Đang đàm phán"),
		csStageID(t, testDB, "Đã lên đơn"),
	}
	id := csSeedCustomer(t, &tn.BranchID, contact, nil)

	pool, err := gorm.Open(mysql.Open(os.Getenv("TEST_MYSQL_DSN")), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	require.NoError(t, optlock.Register(pool))
	sqlDB, err := pool.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(len(targets) + 2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	svc := NewCustomerService(pool)

	const rounds = 15
	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup
		errsOut := make([]error, len(targets))
		start := make(chan struct{})
		for i, to := range targets {
			wg.Add(1)
			go func() {
				defer wg.Done()
				c := testhelpers.TenantCtx(tn.BranchID, uint(uid))
				<-start
				_, errsOut[i] = svc.ChangeStage(csScoped(c, pool), id, to, uid)
			}()
		}
		close(start)
		wg.Wait()
		for i, e := range errsOut {
			require.NoError(t, e, "vòng %d request %d", round, i)
		}
	}

	hist := csHistory(t, pool, id)
	require.NotEmpty(t, hist)
	require.NotNil(t, hist[0].FromStageID)
	require.Equal(t, contact, *hist[0].FromStageID, "dòng đầu: from = bước ban đầu")
	for i := 1; i < len(hist); i++ {
		require.NotNil(t, hist[i].FromStageID)
		require.Equal(t, hist[i-1].ToStageID, *hist[i].FromStageID,
			"dòng %d: from phải = to của dòng trước (chuỗi lịch sử đứt ⇒ thiếu khoá FOR UPDATE)", i)
		require.NotEqual(t, *hist[i].FromStageID, hist[i].ToStageID, "dòng %d: không ghi lịch sử cho đổi cùng bước", i)
	}
	stage, _ := csCustomer(t, pool, id)
	require.Equal(t, hist[len(hist)-1].ToStageID, *stage, "stage_id hiện tại = to của dòng cuối")
	// Mỗi vòng có ≥ 2 request đổi sang bước khác bước hiện tại ⇒ ít nhất 2 dòng/vòng.
	require.GreaterOrEqual(t, len(hist), 2*rounds)
}
