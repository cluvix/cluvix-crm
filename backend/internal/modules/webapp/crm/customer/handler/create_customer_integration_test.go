//go:build integration

package handler

// Story-03 (customer-stage-pipeline): mỗi ca kiểm khách nhận bước mặc định + 1 dòng lịch sử
// khởi tạo (assertInitialStage). Story-12 gỡ cột `status` ⇒ bỏ vế assert status cũ
// (Contact/Appointment/Visited, khác Lead/Ticket); ngày hẹn không còn đổi tiến trình khách.
//
// D14 (crm-standalone, story-33): khách mới hiện ngay trong CRM — mọi phòng ban. Trước đây phòng
// Sale/Telesale/Convert/CSKH/Marketing tạo khách ra Lead(4) ⇒ sau khi xoá module Sale, khách đó
// biến mất khỏi MỌI danh sách.
//
// Bảng ca: phòng ban × {không hẹn, hẹn tương lai, hẹn quá khứ}.
// Mỗi ca còn kiểm AC2: khách vừa tạo hiện trong ListCustomer của chính người tạo.

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	customerService "erpCluvix/internal/modules/webapp/crm/customer/service"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

func callCreateCustomer(t *testing.T, userID uint, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/crm/customer", strings.NewReader(string(raw)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("userID", userID)
	h := &Handler{DB: testDB, CustomerSvc: customerService.NewCustomerService(testDB)}
	h.CreateCustomer(c)
	return w
}

func listCustomerAs(t *testing.T, rawQuery string, userID uint) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/crm/customer?"+rawQuery, nil)
	c.Set("userID", userID)
	h := &Handler{DB: testDB, CustomerSvc: customerService.NewCustomerService(testDB)}
	h.ListCustomer(c)
	return w
}

func TestCreateCustomer_NewCustomerAlwaysContact(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	t.Cleanup(func() { cleanupS33Customers(tn) })
	// Ca có ngày hẹn tạo kèm lịch hẹn ⇒ cần config_code "appointment" (SchemaAll không seed).
	seedS33AppointmentConfigCode(t, tn)

	depts := []struct {
		name string
		dept int // 0 = không thuộc phòng ban nào
	}{
		{"Sale", define.HrDepartmentSaleStaff},
		{"Telesale", define.HrDepartmentTelesale},
		{"Convert", define.HrDepartmentConvert},
		{"CSKH", define.HrDepartmentCustomerCare},
		{"Marketing", define.HrDepartmentMarketing},
		{"LeTan", define.HrDepartmentReceptionist},
		{"KhongPhongBan", 0},
	}
	now := time.Now().Unix()
	appts := []struct {
		name string
		date any // nil = không gửi appointment_date
	}{
		{"KhongHen", nil},
		{"HenTuongLai", now + 86400},
		{"HenQuaKhu", now - 3600},
	}

	for _, d := range depts {
		uid := testhelpers.CreateScopedUser(t, testDB, "s33", tn, tn.BranchID)
		if d.dept != 0 {
			require.NoError(t, testDB.Exec("UPDATE user_profile SET departmentList = ? WHERE user_id = ?",
				fmt.Sprintf("[%d]", d.dept), uid).Error)
		}
		for _, a := range appts {
			t.Run(d.name+"_"+a.name, func(t *testing.T) {
				name := fmt.Sprintf("S33 %s %s %d", d.name, a.name, time.Now().UnixNano())
				body := map[string]any{
					"fullname":  name,
					"phone":     fmt.Sprintf("09%08d", time.Now().UnixNano()%100000000),
					"branch_id": tn.BranchID,
					// FE (customer-form-v2) mặc định người phụ trách = người tạo — cần để telesale
					// (scope payment_user_id = mình) thấy khách của chính mình ở list.
					"payment_user_id": uid,
				}
				if a.date != nil {
					body["appointment_date"] = a.date
				}
				w := callCreateCustomer(t, uid, body)
				require.Equal(t, 201, w.Code, "body: %s", w.Body.String())

				var row struct {
					ID uint
				}
				require.NoError(t, testDB.Table("customer").Select("id").
					Where("fullname = ?", name).Take(&row).Error)

				// Story-03 AC1: cả 3 ca (không hẹn / hẹn quá khứ / hẹn tương lai) đều nhận bước đang
				// mặc định + đúng 1 dòng lịch sử khởi tạo (source Create, changed_by = người tạo).
				assertInitialStage(t, row.ID, defaultStageID(t), uid, define.CustomerStageHistorySourceCreate)

				// AC2: khách vừa tạo hiện trong danh sách khách CRM của chính người tạo.
				found := listContainsID(t, listCustomerAs(t, "per-page=50", uid), row.ID)
				require.True(t, found, "khách %d KHÔNG có trong ListCustomer — phòng %s ca %s",
					row.ID, d.name, a.name)
			})
		}
	}
}

func listContainsID(t *testing.T, w *httptest.ResponseRecorder, id uint) bool {
	t.Helper()
	b := decodeList(t, w)
	for _, it := range b.Items {
		if v, ok := it["id"].(float64); ok && uint(v) == id {
			return true
		}
	}
	return false
}

// seedS33AppointmentConfigCode — CreateCustomer có appointment_date sinh mã lịch hẹn qua
// codegen; SchemaAll không seed config_code cho appointment (khuôn walkin_episode_http_test.go).
func seedS33AppointmentConfigCode(t *testing.T, tn *testhelpers.TestTenant) {
	t.Helper()
	now := time.Now().Unix()
	row := testhelpers.WithTenantCols(tn, map[string]any{
		"name":         "Lịch hẹn",
		"parent_table": "appointment",
		"infix_code":   "LH",
		"created_at":   now,
		"updated_at":   now,
	})
	require.NoError(t, testDB.Table("config_code").Create(&row).Error)
	id := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM config_code_monthly WHERE config_code_id = ?", id)
		testDB.Exec("DELETE FROM config_code WHERE id = ?", id)
	})
}

// Story-36/62: FE cũ còn gửi khoá `company_id` trong body — handler phải bỏ qua (không 422, không
// lỗi cột không tồn tại) và vẫn tạo khách + note kèm theo đúng chi nhánh.
func TestCreateCustomer_BoQuaKhoaThuongHieuCuTuFE(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM note WHERE customer_id IN (SELECT id FROM (SELECT id FROM customer WHERE fullname LIKE 'S36 CC %') x)")
		testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id IN (SELECT id FROM (SELECT id FROM customer WHERE fullname LIKE 'S36 CC %') x)")
		testDB.Exec("DELETE FROM customer WHERE fullname LIKE 'S36 CC %'")
	})
	uid := testhelpers.CreateScopedUser(t, testDB, "s36cc", tn, tn.BranchID)

	for _, ca := range []struct {
		name   string
		legacy any // nil = không gửi
	}{{"KhongGuiKhoaCu", nil}, {"GuiKhoaCu", 987654}} {
		t.Run(ca.name, func(t *testing.T) {
			name := fmt.Sprintf("S36 CC %s %d", ca.name, time.Now().UnixNano())
			body := map[string]any{"fullname": name, "branch_id": tn.BranchID, "payment_user_id": uid, "note": "ghi chu s36"}
			if ca.legacy != nil {
				body["company_id"] = ca.legacy // FE-compat: khoá cũ phải bị bỏ qua
			}
			w := callCreateCustomer(t, uid, body)
			require.Equal(t, 201, w.Code, "body: %s", w.Body.String())
			var row struct {
				ID       uint
				BranchID uint `gorm:"column:branch_id"`
			}
			require.NoError(t, testDB.Table("customer").Select("id, branch_id").Where("fullname = ?", name).Take(&row).Error)
			require.Equal(t, tn.BranchID, row.BranchID, "khách phải ghi đúng chi nhánh gửi lên")
			var noteCount int64
			require.NoError(t, testDB.Table("note").Where("customer_id = ?", row.ID).Count(&noteCount).Error)
			require.EqualValues(t, 1, noteCount, "note kèm theo phải được tạo")
		})
	}
}

// cleanupS33Customers xoá khách (và lịch hẹn/note kèm theo) do các test S33 tạo ra.
func cleanupS33Customers(tn *testhelpers.TestTenant) {
	sub := "SELECT id FROM (SELECT id FROM customer WHERE fullname LIKE 'S33 %' AND id <> ?) x"
	testDB.Exec("DELETE FROM appointment WHERE customer_id IN ("+sub+")", tn.CustomerID)
	testDB.Exec("DELETE FROM note WHERE customer_id IN ("+sub+")", tn.CustomerID)
	// FK tắt ở session test ⇒ CASCADE không chạy: tự dọn lịch sử bước (story-03).
	testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id IN ("+sub+")", tn.CustomerID)
	testDB.Exec("DELETE FROM customer WHERE fullname LIKE 'S33 %' AND id <> ?", tn.CustomerID)
}
