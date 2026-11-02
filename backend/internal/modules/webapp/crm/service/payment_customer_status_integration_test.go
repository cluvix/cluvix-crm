//go:build integration

package service

// customer-stage-pipeline story-06: đơn hàng + thanh toán KHÔNG còn tự đổi tiến trình khách.
// Thay test cũ (story-14 crm-standalone) khẳng định "trả đủ ⇒ nâng status Khách hàng".
//   - AC4: thu đủ dịch vụ / đặt cọc giữ chỗ ⇒ stage_id giữ nguyên; không thêm dòng
//     customer_stage_history; tổng tiền (totalPayment/totalDebt/totalDeposit) vẫn tính đúng.
//   - AC3: tạo / sửa đơn ⇒ stage_id giữ nguyên; connect_id = 1 cho MỌI khách (trước đây chỉ khi
//     tiến trình cũ thuộc allow-list "tiền-khách").
// Khách được ghim ở bước KHÁC bước mặc định, dòng dịch vụ trỏ tới service có thật ⇒ đơn/thanh toán mà
// đổi bước hoặc ghi lịch sử là test đỏ. (story-12: cột `customer.status` + hằng tiến trình cũ đã bỏ ⇒
// vế status/accepted của bản story-06 bỏ theo; 2 ca "allow-list cũ / ngoài allow-list" nay là 2 LOẠI
// bước: đang mở / thành công.)

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

// s06Stage — trạng thái tiến trình của khách (những gì story-06 cam kết KHÔNG đổi).
type s06Stage struct {
	StageID int `gorm:"column:stage_id"`
	History int `gorm:"column:history"`
}

// s06NonDefaultStage — một bước KHÁC bước mặc định, thuộc loại stageType (seed migration) để thấy
// rõ "không đổi".
func s06NonDefaultStage(t *testing.T, stageType int) int {
	t.Helper()
	var id int
	require.NoError(t, testDB.Raw("SELECT id FROM customer_stage WHERE is_default = 0 AND type = ? ORDER BY sort_order, id LIMIT 1",
		stageType).Scan(&id).Error)
	require.NotZero(t, id, "tiền đề: migration phải seed bước không mặc định loại %d", stageType)
	return id
}

// s06PinCustomer ghim khách: bước không mặc định (loại stageType), connect_id.
func s06PinCustomer(t *testing.T, custID uint, stageType, connectID int) s06Stage {
	t.Helper()
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ?, connect_id = ? WHERE id = ?",
		s06NonDefaultStage(t, stageType), connectID, custID).Error)
	return s06ReadStage(t, custID)
}

func s06ReadStage(t *testing.T, custID uint) s06Stage {
	t.Helper()
	var st s06Stage
	require.NoError(t, testDB.Raw(`SELECT c.stage_id AS stage_id,
		(SELECT COUNT(*) FROM customer_stage_history h WHERE h.customer_id = c.id) AS history
		FROM customer c WHERE c.id = ?`, custID).Scan(&st).Error)
	require.NotZero(t, st.StageID, "tiền đề: khách phải có stage_id")
	return st
}

func s06ConnectID(t *testing.T, custID uint) int {
	t.Helper()
	var v int
	require.NoError(t, testDB.Raw("SELECT COALESCE(connect_id, 0) FROM customer WHERE id = ?", custID).Scan(&v).Error)
	return v
}

// newS06PayFx — 1 dòng dịch vụ totalEnd 1tr trỏ tới service có thật (luật cũ INNER JOIN service), khách
// ghim ở bước đang mở không mặc định.
func newS06PayFx(t *testing.T) (*moneyFx, int, s06Stage) {
	t.Helper()
	x := newMoneyFx(t)
	_, ids := x.services(t, 1, 1_000_000)
	osuID := ids[0]
	svcRow := map[string]any{"code": "S06S" + itoa(osuID), "name": "Stage untouched (s06)"}
	require.NoError(t, testDB.Table("service").Create(&svcRow).Error)
	svcID := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() { testDB.Exec("DELETE FROM service WHERE id = ?", svcID) })
	require.NoError(t, testDB.Exec("UPDATE order_service_used SET service_id = ? WHERE id = ?", svcID, osuID).Error)
	before := s06PinCustomer(t, x.tn.CustomerID, define.CustomerStageTypeOpen, 2)
	return x, osuID, before
}

// AC4 — thu đủ dịch vụ (1 lần và 2 lần) ⇒ tiến trình giữ nguyên, tổng tiền đúng.
func TestS06_PaymentFullyPaid_CustomerStageUntouched(t *testing.T) {
	cases := []struct {
		name string
		pays []int64
	}{
		{"thu đủ 1 lần", []int64{1_000_000}},
		{"thu 2 lần tới đủ", []int64{400_000, 600_000}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x, osuID, before := newS06PayFx(t)
			for _, amt := range tc.pays {
				x.pay(t, osuID, amt)
			}

			var paid int
			require.NoError(t, testDB.Raw("SELECT COUNT(*) FROM order_service_used WHERE id = ? AND totalEnd + vat_amount = payment", osuID).Scan(&paid).Error)
			require.Equal(t, 1, paid, "tiền đề: dòng dịch vụ đã thanh toán đủ (điều kiện luật cũ)")

			require.Equal(t, before, s06ReadStage(t, x.tn.CustomerID), "stage_id/lịch sử bước phải giữ nguyên")
			requireDec(t, 1_000_000, x.dec(t, "SELECT totalPayment FROM customer WHERE id = ?", x.cust), "customer.totalPayment")
			requireDec(t, 0, x.dec(t, "SELECT totalDebt FROM customer WHERE id = ?", x.cust), "customer.totalDebt")
		})
	}
}

// AC4 — đặt cọc giữ chỗ (luật cũ ⇒ tiến trình Đặt cọc) ⇒ bước giữ nguyên, totalDeposit đúng.
func TestS06_DepositReserve_CustomerStageUntouched(t *testing.T) {
	x, _, before := newS06PayFx(t)
	x.deposit(t, 300_000)
	x.deposit(t, 200_000)

	var reserve int
	require.NoError(t, testDB.Raw("SELECT COUNT(*) FROM payment WHERE customer_id = ? AND payment_for = ? AND deposit_type = ?",
		x.cust, define.PaymentForDeposit, define.PaymentTypeDepositReserve).Scan(&reserve).Error)
	require.Equal(t, 2, reserve, "tiền đề: 2 phiếu cọc giữ chỗ")

	require.Equal(t, before, s06ReadStage(t, x.tn.CustomerID), "stage_id/lịch sử bước phải giữ nguyên")
	requireDec(t, 500_000, x.dec(t, "SELECT totalDeposit FROM customer WHERE id = ?", x.cust), "customer.totalDeposit")
}

// newS06OrderFx — tenant + config_code đơn/khách cho chi nhánh (order_plan bắt buộc có mã).
func newS06OrderFx(t *testing.T) *logFx {
	t.Helper()
	t.Setenv(define.CustomerCodeOptionEnv, "")
	f := newLogFx(t)
	require.NoError(t, testDB.Exec(`INSERT INTO config_code
		(branch_id, name, parent_table, infix_code, include_branch_code, monthly_reset, serial_length, created_at, updated_at)
		VALUES (?, 'Khách hàng', 'customer', 'KH', 1, 1, 2, 0, 0),
		       (?, 'Đơn hàng', 'order_plan', 'DH', 1, 1, 2, 0, 0)`, f.tn.BranchID, f.tn.BranchID).Error)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM config_code_monthly WHERE config_code_id IN (SELECT id FROM config_code WHERE branch_id = ?)", f.tn.BranchID)
		testDB.Exec("DELETE FROM config_code WHERE branch_id = ?", f.tn.BranchID)
	})
	return f
}

func s06CreateOrder(t *testing.T, f *logFx, createdTime int64) uint {
	t.Helper()
	req := &OrderPlanRequest{}
	req.CustomerId = int(f.tn.CustomerID)
	req.BranchId = new(int(f.tn.BranchID))
	req.CreatedTime = new(createdTime)
	require.NoError(t, NewOrderPlanService(testDB).Create(req, int(f.userID)))
	require.NotZero(t, req.ID)
	return req.ID
}

// AC3 — tạo đơn: stage_id giữ nguyên; connect_id = 1 cho khách ở bước đang mở lẫn bước thành công;
// ngày ghé/liên hệ vẫn ghi như trước.
func TestS06_OrderCreate_CustomerStageUntouched_ConnectAlways(t *testing.T) {
	cases := []struct {
		name      string
		stageType int
	}{
		{"khách ở bước đang mở", define.CustomerStageTypeOpen},
		{"khách ở bước thành công", define.CustomerStageTypeWon},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newS06OrderFx(t)
			before := s06PinCustomer(t, f.tn.CustomerID, tc.stageType, 2)
			created := time.Now().Unix() - 3600
			s06CreateOrder(t, f, created)

			require.Equal(t, before, s06ReadStage(t, f.tn.CustomerID), "stage_id/lịch sử bước phải giữ nguyên")
			require.Equal(t, 1, s06ConnectID(t, f.tn.CustomerID), "connect_id luôn = 1 khi tạo đơn")
			var dates struct {
				LastVisit   int64 `gorm:"column:last_visit_date"`
				LastContact int64 `gorm:"column:last_contact_date"`
			}
			require.NoError(t, testDB.Raw("SELECT COALESCE(last_visit_date,0) AS last_visit_date, COALESCE(last_contact_date,0) AS last_contact_date FROM customer WHERE id = ?",
				f.tn.CustomerID).Scan(&dates).Error)
			require.Equal(t, created, dates.LastVisit, "last_visit_date = created_time đơn (như trước)")
			require.Equal(t, created, dates.LastContact, "last_contact_date = created_time đơn (như trước)")
		})
	}
}

// AC3 — sửa đơn: stage_id giữ nguyên; connect_id = 1 (kể cả khi đã bị đặt lại 2 sau lúc tạo).
func TestS06_OrderUpdate_CustomerStageUntouched_ConnectAlways(t *testing.T) {
	cases := []struct {
		name      string
		stageType int
	}{
		{"khách ở bước đang mở", define.CustomerStageTypeOpen},
		{"khách ở bước thành công", define.CustomerStageTypeWon},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newS06OrderFx(t)
			planID := s06CreateOrder(t, f, time.Now().Unix())
			before := s06PinCustomer(t, f.tn.CustomerID, tc.stageType, 2)

			var ver int64
			require.NoError(t, testDB.Raw("SELECT COALESCE(updated_at,0) FROM order_plan WHERE id = ?", planID).Scan(&ver).Error)
			req := &OrderPlanRequest{}
			req.ID = planID
			req.UpdatedAt = ver
			req.CustomerId = int(f.tn.CustomerID)
			req.BranchId = new(int(f.tn.BranchID))
			req.Note = "s06 sửa đơn"
			require.NoError(t, NewOrderPlanService(testDB).Update(req, int(f.userID), testDB))

			require.Equal(t, before, s06ReadStage(t, f.tn.CustomerID), "stage_id/lịch sử bước phải giữ nguyên")
			require.Equal(t, 1, s06ConnectID(t, f.tn.CustomerID), "connect_id luôn = 1 khi sửa đơn")
		})
	}
}
