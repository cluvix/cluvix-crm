//go:build integration

package service

// customer-stage-pipeline story-06: lịch hẹn KHÔNG còn tự đổi tiến trình khách.
//   - AC1: tạo / sửa / huỷ / check-in / check-out / is_accepted (đồng ý, không đồng ý) ⇒ stage_id của
//     khách giữ nguyên; không thêm dòng customer_stage_history. Mỗi thao tác 1 test.
//   - AC2: ngày liên hệ / ghé thăm, chi nhánh (khi khách chưa có), mã khách vẫn ghi như trước.
// Khách được ghim ở bước KHÁC bước mặc định ⇒ lịch hẹn mà đổi bước/ghi lịch sử là test đỏ.
// (story-12: cột `customer.status`/hằng tiến trình cũ đã bỏ ⇒ vế status/accepted của bản story-06 bỏ theo.)

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appointmentDao "erpCluvix/internal/modules/webapp/crm/appointment/dao"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/logger"
)

const s06UserID = 1

// s06Stage — trạng thái tiến trình của khách (những gì story-06 cam kết KHÔNG đổi).
type s06Stage struct {
	StageID int `gorm:"column:stage_id"`
	History int `gorm:"column:history"`
}

// s06CustDates — các field lịch hẹn VẪN được ghi (AC2).
type s06CustDates struct {
	LastContact int64  `gorm:"column:last_contact_date"`
	LastVisit   int64  `gorm:"column:last_visit_date"`
	BranchID    uint   `gorm:"column:branch_id"`
	Code        string `gorm:"column:code"`
}

type s06Fx struct {
	tn     *testhelpers.TestTenant
	svc    *AppointmentService
	before s06Stage
}

func newS06Fx(t *testing.T) *s06Fx {
	t.Helper()
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}
	t.Setenv(define.CustomerCodeOptionEnv, "") // mã khách theo config_code
	if logger.L == nil {
		logger.Init(logger.Config{Env: "test", AppCode: "crm_appointment_service_test"})
	}
	tn := testhelpers.FreshTenant(t, testDB)
	require.NoError(t, testDB.Exec(`INSERT INTO config_code
		(branch_id, name, parent_table, infix_code, include_branch_code, monthly_reset, serial_length, created_at, updated_at)
		VALUES (?, 'Khách hàng', 'customer', 'KH', 1, 1, 2, 0, 0),
		       (?, 'Lịch hẹn', 'appointment', 'LH', 1, 1, 3, 0, 0)`, tn.BranchID, tn.BranchID).Error)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM log_appointment WHERE appointment_id IN (SELECT id FROM appointment WHERE customer_id = ?)", tn.CustomerID)
		testDB.Exec("DELETE FROM appointment WHERE customer_id = ?", tn.CustomerID)
		testDB.Exec("DELETE FROM config_code_monthly WHERE config_code_id IN (SELECT id FROM config_code WHERE branch_id = ?)", tn.BranchID)
		testDB.Exec("DELETE FROM config_code WHERE branch_id = ?", tn.BranchID)
	})

	var stageID int
	require.NoError(t, testDB.Raw("SELECT id FROM customer_stage WHERE is_default = 0 ORDER BY sort_order, id LIMIT 1").Scan(&stageID).Error)
	require.NotZero(t, stageID, "tiền đề: migration phải seed bước không mặc định")
	// Khách chưa có chi nhánh + chưa có mã (để AC2 thấy lịch hẹn gán chi nhánh/sinh mã).
	require.NoError(t, testDB.Exec("UPDATE customer SET stage_id = ?, branch_id = NULL, code = NULL, last_contact_date = NULL, last_visit_date = NULL WHERE id = ?",
		stageID, tn.CustomerID).Error)

	f := &s06Fx{tn: tn, svc: NewAppointmentService(testDB)}
	f.before = f.stage(t)
	return f
}

func (f *s06Fx) stage(t *testing.T) s06Stage {
	t.Helper()
	var st s06Stage
	require.NoError(t, testDB.Raw(`SELECT c.stage_id AS stage_id,
		(SELECT COUNT(*) FROM customer_stage_history h WHERE h.customer_id = c.id) AS history
		FROM customer c WHERE c.id = ?`, f.tn.CustomerID).Scan(&st).Error)
	require.NotZero(t, st.StageID, "tiền đề: khách phải có stage_id")
	return st
}

// rebase chụp lại tiến trình SAU bước dựng lịch hẹn ⇒ mỗi test chỉ đo đúng thao tác của nó.
func (f *s06Fx) rebase(t *testing.T) {
	t.Helper()
	f.before = f.stage(t)
}

func (f *s06Fx) requireStageUntouched(t *testing.T) {
	t.Helper()
	require.Equal(t, f.before, f.stage(t), "stage_id/lịch sử bước của khách phải giữ nguyên")
}

func (f *s06Fx) dates(t *testing.T) s06CustDates {
	t.Helper()
	var d s06CustDates
	require.NoError(t, testDB.Raw(`SELECT COALESCE(last_contact_date, 0) AS last_contact_date,
		COALESCE(last_visit_date, 0) AS last_visit_date, COALESCE(branch_id, 0) AS branch_id, COALESCE(code, '') AS code
		FROM customer WHERE id = ?`, f.tn.CustomerID).Scan(&d).Error)
	return d
}

func (f *s06Fx) setDates(t *testing.T, lastContact, lastVisit int64) {
	t.Helper()
	require.NoError(t, testDB.Exec("UPDATE customer SET last_contact_date = ?, last_visit_date = ? WHERE id = ?",
		lastContact, lastVisit, f.tn.CustomerID).Error)
}

// create tạo 1 lịch hẹn qua service (Create).
func (f *s06Fx) create(t *testing.T) *appointmentDao.Appointment {
	t.Helper()
	cust := int(f.tn.CustomerID)
	branch := int(f.tn.BranchID)
	typeID := 1 // seed appointment_type "Tư vấn"
	date := time.Now().Add(48 * time.Hour).Unix()
	a := &appointmentDao.Appointment{CustomerId: &cust, BranchId: &branch, AppointmentTypeId: &typeID, AppointmentDate: &date}
	require.NoError(t, f.svc.Create(a, s06UserID))
	require.NotZero(t, a.ID)
	return a
}

// version đọc updated_at hiện tại của lịch hẹn (khoá lạc quan D6).
func (f *s06Fx) version(t *testing.T, id uint) int64 {
	t.Helper()
	var v int64
	require.NoError(t, testDB.Raw("SELECT COALESCE(updated_at, 0) FROM appointment WHERE id = ?", id).Scan(&v).Error)
	return v
}

func (f *s06Fx) apptStatus(t *testing.T, id uint) int {
	t.Helper()
	var st int
	require.NoError(t, testDB.Raw("SELECT COALESCE(status, 0) FROM appointment WHERE id = ?", id).Scan(&st).Error)
	return st
}

// checkin gửi form tiếp nhận (không gửi status — BE tự suy từ ngày).
func (f *s06Fx) checkin(t *testing.T, id uint, checkin, checkout *int64, isAccepted *int) {
	t.Helper()
	req := &appointmentDao.Appointment{CheckinDate: checkin, CheckoutDate: checkout, IsAccepted: isAccepted}
	req.ID = id
	req.UpdatedAt = f.version(t, id)
	require.NoError(t, f.svc.Checkin(req, map[string]bool{}, s06UserID, testDB))
}

// AC1+AC2 — Tạo lịch hẹn (luật cũ: status ⇒ Hẹn).
func TestS06_AppointmentCreate_CustomerStageUntouched(t *testing.T) {
	f := newS06Fx(t)
	a := f.create(t)

	f.requireStageUntouched(t)
	d := f.dates(t)
	require.Equal(t, a.CreatedAt, d.LastContact, "last_contact_date = created_at lịch hẹn (như trước)")
	require.Equal(t, f.tn.BranchID, d.BranchID, "khách chưa có chi nhánh ⇒ nhận chi nhánh lịch hẹn (như trước)")
	require.NotEmpty(t, d.Code, "khách chưa có mã ⇒ sinh mã theo chi nhánh (như trước)")
}

// AC1+AC2 — Sửa lịch hẹn (dời ngày, status Chưa đến; luật cũ: status ⇒ Hẹn).
func TestS06_AppointmentUpdate_CustomerStageUntouched(t *testing.T) {
	f := newS06Fx(t)
	a := f.create(t)
	f.rebase(t)
	f.setDates(t, 1000, 0)

	t0 := time.Now().Unix()
	newDate := time.Now().Add(72 * time.Hour).Unix()
	notCheckin := define.AppointmentStatusNotCheckin
	req := &appointmentDao.Appointment{AppointmentDate: &newDate, Status: &notCheckin}
	req.ID = a.ID
	req.UpdatedAt = f.version(t, a.ID)
	require.NoError(t, f.svc.Update(req, s06UserID, testDB))

	f.requireStageUntouched(t)
	d := f.dates(t)
	require.GreaterOrEqual(t, d.LastContact, t0, "sửa lịch ⇒ last_contact_date = now (như trước)")
	require.Zero(t, d.LastVisit, "sửa lịch không ghi last_visit_date")
}

// AC1+AC2 — Huỷ lịch hẹn (luật cũ: status ⇒ Huỷ hẹn). Đường huỷ trước story-06 KHÔNG ghi ngày liên hệ
// (chưa từng gọi hàm ghi ngày) ⇒ "như trước" = giữ nguyên giá trị cũ.
func TestS06_AppointmentCancel_CustomerStageUntouched(t *testing.T) {
	f := newS06Fx(t)
	a := f.create(t)
	f.rebase(t)
	f.setDates(t, 1234, 5678)

	req := &appointmentDao.Appointment{NoteAppointment: "khách bận"}
	req.ID = a.ID
	req.UpdatedAt = f.version(t, a.ID)
	require.NoError(t, f.svc.Cancel(req, s06UserID, testDB))

	require.Equal(t, define.AppointmentStatusCancel, f.apptStatus(t, a.ID), "tiền đề: lịch hẹn đã huỷ")
	f.requireStageUntouched(t)
	d := f.dates(t)
	require.EqualValues(t, 1234, d.LastContact, "huỷ không ghi last_contact_date (như trước)")
	require.EqualValues(t, 5678, d.LastVisit, "huỷ không ghi last_visit_date (như trước)")
}

// AC1+AC2 — Check-in (luật cũ: status ⇒ Đã đến).
func TestS06_AppointmentCheckin_CustomerStageUntouched(t *testing.T) {
	f := newS06Fx(t)
	a := f.create(t)
	f.rebase(t)
	f.setDates(t, 1000, 0)

	checkin := time.Now().Add(-time.Hour).Unix()
	f.checkin(t, a.ID, &checkin, nil, nil)

	require.Equal(t, define.AppointmentStatusCheckedIn, f.apptStatus(t, a.ID), "tiền đề: lịch hẹn đã check-in")
	f.requireStageUntouched(t)
	d := f.dates(t)
	require.Equal(t, checkin, d.LastVisit, "check-in ⇒ last_visit_date = checkin_date (như trước)")
	require.Equal(t, checkin, d.LastContact, "check-in ⇒ last_contact_date = checkin_date (như trước)")
}

// AC1+AC2 — Check-out (luật cũ: status ⇒ Đã đến).
func TestS06_AppointmentCheckout_CustomerStageUntouched(t *testing.T) {
	f := newS06Fx(t)
	a := f.create(t)
	f.rebase(t)
	f.setDates(t, 1000, 0)

	checkin := time.Now().Add(-2 * time.Hour).Unix()
	checkout := time.Now().Add(-time.Hour).Unix()
	f.checkin(t, a.ID, &checkin, &checkout, nil)

	require.Equal(t, define.AppointmentStatusCheckedOut, f.apptStatus(t, a.ID), "tiền đề: lịch hẹn đã check-out")
	f.requireStageUntouched(t)
	d := f.dates(t)
	require.Equal(t, checkout, d.LastVisit, "check-out ⇒ last_visit_date = checkout_date (như trước)")
	require.Equal(t, checkout, d.LastContact, "check-out ⇒ last_contact_date = checkout_date (như trước)")
}

// AC1 — is_accepted = Đồng ý (luật cũ: status ⇒ Tiền khách hàng + accepted ⇒ Ok).
func TestS06_AppointmentAccepted_CustomerStageUntouched(t *testing.T) {
	f := newS06Fx(t)
	a := f.create(t)
	f.rebase(t)

	// Suite test không chạy seed lookup ⇒ tự thêm dòng "Đồng ý" như seed production (seed_lookup.up.sql).
	res := testDB.Exec(`INSERT IGNORE INTO appointment_is_accepted (id, name, severity, SortOrder, status, isAccepted, created_at, updated_at)
		VALUES (?, 'Đồng ý', 'success', 1, 1, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP())`, define.IsAccepted)
	require.NoError(t, res.Error)
	if res.RowsAffected > 0 {
		t.Cleanup(func() { testDB.Exec("DELETE FROM appointment_is_accepted WHERE id = ?", define.IsAccepted) })
	}
	var agreed int
	require.NoError(t, testDB.Raw("SELECT COUNT(*) FROM appointment_is_accepted WHERE id = ? AND isAccepted = 1 AND status = 1", define.IsAccepted).Scan(&agreed).Error)
	require.Equal(t, 1, agreed, "tiền đề: id Đồng ý thuộc nhóm isAccepted = 1 (luật cũ nâng khách)")

	checkin := time.Now().Add(-time.Hour).Unix()
	accepted := define.IsAccepted
	f.checkin(t, a.ID, &checkin, nil, &accepted)

	var got int
	require.NoError(t, testDB.Raw("SELECT COALESCE(is_accepted, 0) FROM appointment WHERE id = ?", a.ID).Scan(&got).Error)
	require.Equal(t, define.IsAccepted, got, "tiền đề: lịch hẹn lưu is_accepted")
	f.requireStageUntouched(t)
}

// AC1 — is_accepted = Không đồng ý (luật cũ: status ⇒ Mất khách).
func TestS06_AppointmentNotAccepted_CustomerStageUntouched(t *testing.T) {
	f := newS06Fx(t)
	a := f.create(t)
	f.rebase(t)

	checkin := time.Now().Add(-time.Hour).Unix()
	notAccepted := define.IsNotAccepted
	f.checkin(t, a.ID, &checkin, nil, &notAccepted)

	var got int
	require.NoError(t, testDB.Raw("SELECT COALESCE(is_accepted, 0) FROM appointment WHERE id = ?", a.ID).Scan(&got).Error)
	require.Equal(t, define.IsNotAccepted, got, "tiền đề: lịch hẹn lưu is_accepted")
	f.requireStageUntouched(t)
}
