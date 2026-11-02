//go:build integration

package scope

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaAll)
	if err != nil {
		log.Printf("testhelpers: %v — skipping integration tests", err)
		os.Exit(m.Run())
	}
	defer s.Close()
	testDB = s.DB
	os.Exit(m.Run())
}

// seedUserWithDepts tạo user + user_profile có departmentList = depts (JSON array số).
func seedUserWithDepts(t *testing.T, depts ...int) uint {
	t.Helper()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	now := time.Now().Unix()
	require.NoError(t, testDB.Table("user").Create(map[string]any{
		"username": "s08u" + suffix, "password_hash": "x", "status": define.UserStatusActive,
		"created_at": now, "updated_at": now,
	}).Error)
	uid := testhelpers.LastInsertID(testDB)

	list := "["
	for i, d := range depts {
		if i > 0 {
			list += ","
		}
		list += strconv.Itoa(d)
	}
	list += "]"
	require.NoError(t, testDB.Table("user_profile").Create(map[string]any{
		"user_id": uid, "fullname": "S08 " + suffix, "departmentList": list,
	}).Error)
	return uid
}

func seedCustomer(t *testing.T, tenant *testhelpers.TestTenant, paymentUserID *uint) uint {
	t.Helper()
	now := time.Now().Unix()
	row := testhelpers.WithTenantCols(tenant, map[string]any{
		"fullname": fmt.Sprintf("S08 KH %d", time.Now().UnixNano()),
		"stage_id": testhelpers.MustDefaultCustomerStageID(t, testDB), "payment_user_id": paymentUserID,
		"created_day": now, "created_at": now, "updated_at": now,
	})
	delete(row, "branch_id") // giữ NULL như bản gốc (test không gán chi nhánh)
	require.NoError(t, testDB.Table("customer").Create(&row).Error)
	return testhelpers.LastInsertID(testDB)
}

func setOption(t *testing.T, on bool) {
	t.Helper()
	val := 0
	if on {
		val = 1
	}
	require.NoError(t, testDB.Exec(
		"INSERT INTO user_option (id, name, options, status) VALUES (?, 'view_lead_of_tele_s08', ?, 1) "+
			"ON DUPLICATE KEY UPDATE options = VALUES(options)",
		define.UserOptionViewLeadOfTele, val).Error)
}

// countVisible đếm số khách trong ids mà userID nhìn thấy sau khi áp scope.Apply.
func countVisible(t *testing.T, userID uint, ids []uint) int64 {
	t.Helper()
	c := testhelpers.TenantCtx(0, userID)
	q := testDB.Table("customer").Where("customer.id IN ?", ids)
	var n int64
	scoped, err := Apply(c, testDB, q)
	require.NoError(t, err)
	require.NoError(t, scoped.Count(&n).Error)
	return n
}

// AC2 story-08 (D8): 4 tính chất của scope telesale.
func TestApply_TelesaleScope(t *testing.T) {
	if testDB == nil {
		t.Skip("testDB nil — TEST_MYSQL_DSN chưa set")
	}
	tenant := testhelpers.FreshTenant(t, testDB)

	tele := seedUserWithDepts(t, define.HrDepartmentTelesale)
	other := seedUserWithDepts(t, define.HrDepartmentMarketing)
	mixed := seedUserWithDepts(t, define.HrDepartmentMarketing, define.HrDepartmentTelesale)

	ids := []uint{
		seedCustomer(t, tenant, &tele),
		seedCustomer(t, tenant, &mixed),
		seedCustomer(t, tenant, &other),
		seedCustomer(t, tenant, nil),
	}
	all := int64(len(ids))

	t.Run("telesale + option OFF chỉ thấy khách mình phụ trách", func(t *testing.T) {
		setOption(t, false)
		require.EqualValues(t, 1, countVisible(t, tele, ids))
	})
	t.Run("telesale + option ON thấy đủ", func(t *testing.T) {
		setOption(t, true)
		require.EqualValues(t, all, countVisible(t, tele, ids))
	})
	t.Run("không thuộc Telesale thấy đủ (option OFF)", func(t *testing.T) {
		setOption(t, false)
		require.EqualValues(t, all, countVisible(t, other, ids))
	})
	t.Run("vừa Telesale vừa phòng khác vẫn bị giới hạn (option OFF)", func(t *testing.T) {
		setOption(t, false)
		require.EqualValues(t, 1, countVisible(t, mixed, ids))
	})
	t.Run("không có row option = OFF", func(t *testing.T) {
		require.NoError(t, testDB.Exec("DELETE FROM user_option WHERE id = ?", define.UserOptionViewLeadOfTele).Error)
		require.EqualValues(t, 1, countVisible(t, tele, ids))
	})
}
