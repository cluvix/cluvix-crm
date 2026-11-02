//go:build integration

// Package integration — test tích hợp xuyên module cho đợt crm-prod-readiness B1:
//   - S-10: phạm vi telesale (khách mình phụ trách) áp ở MỌI bề mặt trả dữ liệu khách, không chỉ màn khách;
//   - S-8:  lịch sử thay đổi che SĐT theo option 14;
//   - R-5:  widget doanh thu dashboard đòi quyền xem tiền;
//   - D-3:  SĐT duy nhất trong 1 DB (UNIQUE thật + lỗi i18n) và migration chịu được dữ liệu trùng có sẵn.
//
// Dựng engine như nhóm `authed` của router (PermissionEnforcer + route thật của module) với auth test
// đọc uid từ header; RbacService THẬT trên DB dùng-một-lần. Mọi ca chặn đều có đối chứng chiều cho qua.
package integration

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/db"
	"erpCluvix/internal/deps"
	"erpCluvix/internal/middleware"
	"erpCluvix/internal/modules/webapp/auth/rbac/seed"
	authSvc "erpCluvix/internal/modules/webapp/auth/rbac/service"
	"erpCluvix/internal/modules/webapp/crm"
	"erpCluvix/internal/modules/webapp/history"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/logger"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	logger.Init(logger.Config{Env: "test", AppCode: "b1_prod_readiness_test"})
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaAll)
	if err != nil {
		log.Printf("testhelpers: %v — skipping integration tests", err)
		os.Exit(m.Run())
	}
	testDB = s.DB
	db.DB = s.DB // pagination.Paginate đọc global db.DB
	code := m.Run()
	s.Close()
	os.Exit(code)
}

const hdrUID = "X-Test-Uid"

var seq atomic.Int64

func skipNoDB(t *testing.T) {
	t.Helper()
	if testDB == nil {
		t.Skip("TEST_MYSQL_DSN chưa set")
	}
}

func engine(t *testing.T, branchID uint) *gin.Engine {
	t.Helper()
	entries, err := seed.LoadRoutePermMap()
	if err != nil {
		t.Fatalf("LoadRoutePermMap: %v", err)
	}
	rbac := &authSvc.RbacService{DB: testDB}
	r := testhelpers.NewGin()
	authed := r.Group("/api", func(c *gin.Context) {
		uid, _ := strconv.ParseUint(c.GetHeader(hdrUID), 10, 64)
		c.Set("branchActiveId", branchID)
		c.Set("branchIds", []uint{branchID})
		c.Set("userID", uint(uid))
		c.Next()
	}, middleware.PermissionEnforcer(rbac, entries))
	d := deps.Deps{DB: testDB, RbacSvc: rbac}
	crm.Register(authed, d)
	history.Register(authed, d)
	return r
}

func withEnforce(t *testing.T, on bool) {
	t.Helper()
	prev := middleware.PermEnforcing()
	middleware.SetPermEnforce(on)
	t.Cleanup(func() { middleware.SetPermEnforce(prev) })
}

func mustExec(t *testing.T, q string, args ...any) {
	t.Helper()
	if err := testDB.Exec(q, args...).Error; err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func call(r *gin.Engine, method, url string, uid uint, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	req.Header.Set(hdrUID, strconv.FormatUint(uint64(uid), 10))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func expect(t *testing.T, w *httptest.ResponseRecorder, want int, what string) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("%s: HTTP %d, mong đợi %d — body=%.400s", what, w.Code, want, w.Body.String())
	}
}

// setOption ghi user_option[id] = val, trả lại giá trị cũ khi test xong.
func setOption(t *testing.T, id, val int) {
	t.Helper()
	var prev []int
	testDB.Table("user_option").Where("id = ?", id).Pluck("options", &prev)
	mustExec(t, "INSERT INTO user_option (id, name, options, status) VALUES (?, ?, ?, 1) "+
		"ON DUPLICATE KEY UPDATE options = VALUES(options)", id, fmt.Sprintf("b1_opt_%d", id), val)
	t.Cleanup(func() {
		if len(prev) == 0 {
			testDB.Exec("DELETE FROM user_option WHERE id = ?", id)
		} else {
			testDB.Exec("UPDATE user_option SET options = ? WHERE id = ?", prev[0], id)
		}
	})
}

func setDepartments(t *testing.T, uid uint, depts string) {
	t.Helper()
	mustExec(t, "UPDATE user_profile SET departmentList = ? WHERE user_id = ?", depts, uid)
}

func grant(t *testing.T, uid uint, perms ...string) {
	t.Helper()
	now := time.Now().Unix()
	role := fmt.Sprintf("b1_role_%d_%d", uid, seq.Add(1))
	mustExec(t, "INSERT INTO rbac_auth_item (name, type, created_at, updated_at) VALUES (?, 1, ?, ?)", role, now, now)
	for _, p := range perms {
		mustExec(t, "INSERT IGNORE INTO rbac_auth_item (name, type, created_at, updated_at) VALUES (?, 2, ?, ?)", p, now, now)
		mustExec(t, "INSERT INTO rbac_auth_item_child (parent, child) VALUES (?, ?)", role, p)
	}
	mustExec(t, "INSERT INTO rbac_auth_assignment (item_name, user_id, created_at) VALUES (?, ?, ?)", role, uid, now)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM rbac_auth_assignment WHERE item_name = ?", role)
		testDB.Exec("DELETE FROM rbac_auth_item_child WHERE parent = ?", role)
		testDB.Exec("DELETE FROM rbac_auth_item WHERE name = ?", role)
	})
}

// uniquePhone — SĐT 10 số khác nhau ở 9 số cuối (khoá UNIQUE) giữa các lần gọi.
func uniquePhone() string {
	n := (time.Now().UnixNano()/1000 + seq.Add(1)) % 100000000
	return fmt.Sprintf("09%08d", n)
}

// collectIDs gom mọi giá trị số của khoá `key` ở mọi tầng của JSON.
func collectIDs(t *testing.T, body []byte, key string) map[int64]bool {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("JSON không hợp lệ: %v — %.300s", err, body)
	}
	out := map[int64]bool{}
	var walk func(any)
	walk = func(x any) {
		switch y := x.(type) {
		case map[string]any:
			for k, val := range y {
				if k == key {
					switch n := val.(type) {
					case float64:
						out[int64(n)] = true
					case string:
						if i, err := strconv.ParseInt(n, 10, 64); err == nil {
							out[i] = true
						}
					}
				}
				walk(val)
			}
		case []any:
			for _, e := range y {
				walk(e)
			}
		}
	}
	walk(v)
	return out
}

type scopeFixture struct {
	tn                 *testhelpers.TestTenant
	teleA, teleB, boss uint
	custA, custB       uint
	keyword            string
}

// seedScope: 2 telesale A/B (phòng Telesale), 1 nhân viên phòng khác (đối chứng thấy hết); mỗi telesale
// phụ trách 1 khách có đủ đơn hàng, phiếu thu, công nợ, lịch hẹn, nhắc việc, ghi chú và log lịch sử.
func seedScope(t *testing.T) scopeFixture {
	t.Helper()
	tn := testhelpers.FreshTenant(t, testDB)
	f := scopeFixture{tn: tn, keyword: fmt.Sprintf("B1S10K%d", time.Now().UnixNano()%1e9)}
	f.teleA = testhelpers.CreateScopedUser(t, testDB, "b1_teleA", tn, tn.BranchID)
	f.teleB = testhelpers.CreateScopedUser(t, testDB, "b1_teleB", tn, tn.BranchID)
	f.boss = testhelpers.CreateScopedUser(t, testDB, "b1_other", tn, tn.BranchID)
	tele := fmt.Sprintf("[%d]", define.HrDepartmentTelesale)
	setDepartments(t, f.teleA, tele)
	setDepartments(t, f.teleB, tele)
	setDepartments(t, f.boss, fmt.Sprintf("[%d]", define.HrDepartmentMarketing))

	now := time.Now().Unix()
	var orderPlans, payments, appts, notes, reminders, logs []uint
	mkCustomer := func(suffix string, owner uint) uint {
		row := map[string]any{
			"fullname": f.keyword + suffix, "phone": uniquePhone(), "status": define.CustomerStatusCustomer,
			"branch_id": tn.BranchID, "payment_user_id": owner,
			"totalRevenue": 100, "totalPayment": 0, "totalDebt": 100,
			"created_day": now, "created_at": now, "updated_at": now,
		}
		if err := testDB.Table("customer").Create(&row).Error; err != nil {
			t.Fatalf("seed customer: %v", err)
		}
		id := testhelpers.LastInsertID(testDB)
		ins := func(dst *[]uint, table string, row map[string]any) {
			if err := testDB.Table(table).Create(&row).Error; err != nil {
				t.Fatalf("seed %s: %v", table, err)
			}
			*dst = append(*dst, testhelpers.LastInsertID(testDB))
		}
		ins(&orderPlans, "order_plan", map[string]any{"code": fmt.Sprintf("DH%d", time.Now().UnixNano()),
			"customer_id": id, "branch_id": tn.BranchID, "created_time": now, "created_at": now, "updated_at": now})
		ins(&payments, "payment", map[string]any{"code": fmt.Sprintf("PT%d", time.Now().UnixNano()),
			"customer_id": id, "branch_id": tn.BranchID, "method": 1, "amount": 100,
			"payment_for": define.PaymentForOrder, "payment_time": now, "payment_of_date": now,
			"created_at": now, "updated_at": now})
		ins(&appts, "appointment", map[string]any{"code": fmt.Sprintf("LH%d", time.Now().UnixNano()),
			"customer_id": id, "branch_id": tn.BranchID, "appointment_date": now, "status": 1,
			"created_at": now, "updated_at": now})
		ins(&notes, "note", map[string]any{"note": "b1 note", "customer_id": id, "created_at": now})
		ins(&reminders, "reminder", map[string]any{"code": fmt.Sprintf("NV%d", time.Now().UnixNano()),
			"customer_id": id, "start_time": now, "note": "b1", "status": 1, "type": 1,
			"created_at": now, "updated_at": now})
		ins(&logs, "log_customer", map[string]any{"customer_id": id, "action_type": 2,
			"changed_fields": `{"phone": {"from": "0901234567", "to": "0907654321"}}`, "changed_at": now})
		t.Cleanup(func() { testDB.Exec("DELETE FROM customer WHERE id = ?", id) })
		return id
	}
	f.custA = mkCustomer("A", f.teleA)
	f.custB = mkCustomer("B", f.teleB)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM log_customer WHERE id IN ?", logs)
		testDB.Exec("DELETE FROM reminder WHERE id IN ?", reminders)
		testDB.Exec("DELETE FROM note WHERE id IN ?", notes)
		testDB.Exec("DELETE FROM appointment WHERE id IN ?", appts)
		testDB.Exec("DELETE FROM payment WHERE id IN ?", payments)
		testDB.Exec("DELETE FROM order_plan WHERE id IN ?", orderPlans)
	})
	return f
}

// S-10: telesale A KHÔNG thấy khách của telesale B ở mọi bề mặt; đối chứng: nhân viên phòng khác thấy cả 2,
// và option "telesale xem mọi khách" BẬT thì A thấy cả 2.
func TestTelesaleScope_AllSurfaces(t *testing.T) {
	skipNoDB(t)
	f := seedScope(t)
	r := engine(t, f.tn.BranchID)
	withEnforce(t, false) // ca này kiểm PHẠM VI DỮ LIỆU, không kiểm quyền route
	setOption(t, define.UserOptionViewLeadOfTele, 0)
	setOption(t, 14, 0)

	from := time.Now().Add(-24 * time.Hour).Unix()
	to := time.Now().Add(24 * time.Hour).Unix()
	surfaces := []struct{ name, url, key string }{
		{"order-plan list", "/api/crm/order-plan?per-page=200", "customer_id"},
		{"payment list-receipts", "/api/crm/payment/list-receipts?per-page=200", "customer_id"},
		{"payment list", "/api/crm/payment?per-page=200", "customer_id"},
		{"debt customer", "/api/crm/report/debt/customer?per-page=200&_keyword=" + f.keyword, "id"},
		{"revenue branch-receipts", fmt.Sprintf("/api/crm/report/revenue/branch-receipts?per-page=200&filter[payment_time][gte]=%d&filter[payment_time][lte]=%d", from, to), "customer_id"},
		{"log customer", "/api/history/log?_fieldName=customer&per-page=200", "customer_id"},
		{"autocomplete", "/api/crm/customer/load-by-keyword?limit=50&keyword=" + f.keyword, "id"},
		{"appointment list", "/api/crm/appointment?per-page=200", "customer_id"},
		{"note get-all", "/api/crm/note/get-all-note?per-page=200", "customer_id"},
		{"reminder list", "/api/crm/reminder?per-page=200", "customer_id"},
	}
	ida, idb := int64(f.custA), int64(f.custB)
	for _, s := range surfaces {
		w := call(r, http.MethodGet, s.url, f.boss, "")
		expect(t, w, http.StatusOK, s.name+" (đối chứng nhân viên phòng khác)")
		got := collectIDs(t, w.Body.Bytes(), s.key)
		if !got[ida] || !got[idb] {
			t.Fatalf("%s: đối chứng phải thấy cả 2 khách (%d, %d) — seed/endpoint sai, ca chặn vô nghĩa. body=%.300s",
				s.name, ida, idb, w.Body.String())
		}

		w = call(r, http.MethodGet, s.url, f.teleA, "")
		expect(t, w, http.StatusOK, s.name+" (telesale A)")
		got = collectIDs(t, w.Body.Bytes(), s.key)
		if !got[ida] {
			t.Errorf("%s: telesale A phải thấy khách mình phụ trách %d", s.name, ida)
		}
		if got[idb] {
			t.Errorf("%s: ⛔ telesale A thấy khách %d của telesale B (S-10)", s.name, idb)
		}
	}

	// Danh sách ghi chú theo 1 khách (màn chi tiết): khách ngoài phạm vi ⇒ rỗng.
	w := call(r, http.MethodGet, fmt.Sprintf("/api/crm/note/list-advise?id=%d", f.custB), f.teleA, "")
	expect(t, w, http.StatusOK, "list-advise khách B")
	if got := collectIDs(t, w.Body.Bytes(), "customer_id"); got[idb] {
		t.Errorf("list-advise: ⛔ telesale A đọc được ghi chú khách B")
	}

	// Option BẬT ⇒ telesale A thấy cả 2 (đối chứng nhánh không giới hạn).
	setOption(t, define.UserOptionViewLeadOfTele, 1)
	w = call(r, http.MethodGet, "/api/crm/order-plan?per-page=200", f.teleA, "")
	expect(t, w, http.StatusOK, "order-plan option ON")
	if got := collectIDs(t, w.Body.Bytes(), "customer_id"); !got[ida] || !got[idb] {
		t.Errorf("option xem mọi khách BẬT: telesale A phải thấy cả 2 khách")
	}
}

// S-8: option 14 BẬT ⇒ lịch sử trả SĐT đã che (giá trị cũ/mới); TẮT ⇒ số thật (đối chứng).
func TestHistoryLog_MasksPhoneByOption14(t *testing.T) {
	skipNoDB(t)
	f := seedScope(t)
	r := engine(t, f.tn.BranchID)
	withEnforce(t, false)
	setOption(t, define.UserOptionViewLeadOfTele, 1)

	phoneOf := func() (string, string) {
		t.Helper()
		w := call(r, http.MethodGet, "/api/history/log?_fieldName=customer&per-page=200", f.boss, "")
		expect(t, w, http.StatusOK, "history log")
		var res struct {
			Items []struct {
				CustomerID    int64 `json:"customer_id"`
				ChangedFields struct {
					Phone struct {
						From string `json:"from"`
						To   string `json:"to"`
					} `json:"phone"`
				} `json:"changed_fields"`
			} `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, it := range res.Items {
			if it.CustomerID == int64(f.custA) {
				return it.ChangedFields.Phone.From, it.ChangedFields.Phone.To
			}
		}
		t.Fatalf("không thấy dòng log của khách %d", f.custA)
		return "", ""
	}

	setOption(t, 14, 0)
	from, to := phoneOf()
	if from != "0901234567" || to != "0907654321" {
		t.Fatalf("option 14 TẮT phải trả số thật, được %q → %q", from, to)
	}
	setOption(t, 14, 1)
	from, to = phoneOf()
	if from != "xxxxxx4567" || to != "xxxxxx4321" {
		t.Fatalf("⛔ option 14 BẬT phải che SĐT trong lịch sử (S-8), được %q → %q", from, to)
	}
}

// R-5: user chỉ có viewAppointment ⇒ 403 ở widget doanh thu dashboard; đối chứng: widget lịch hẹn vẫn 200,
// user có viewPayment / manageCrmReport ⇒ 200.
func TestDashboardRevenue_RequiresMoneyPerm(t *testing.T) {
	skipNoDB(t)
	tn := testhelpers.FreshTenant(t, testDB)
	r := engine(t, tn.BranchID)
	withEnforce(t, true)
	receptionist := testhelpers.CreateScopedUser(t, testDB, "b1_recep", tn, tn.BranchID)
	grant(t, receptionist, "viewAppointment")
	cashier := testhelpers.CreateScopedUser(t, testDB, "b1_cash", tn, tn.BranchID)
	grant(t, cashier, "viewPayment")
	reporter := testhelpers.CreateScopedUser(t, testDB, "b1_rep", tn, tn.BranchID)
	grant(t, reporter, define.PermManageCrmReport)

	money := []string{"payment-in-month", "payment-branch-overview", "service-used-statistics"}
	for _, m := range money {
		url := "/api/crm/crm-dashboard/" + m
		expect(t, call(r, http.MethodGet, url, receptionist, ""), http.StatusForbidden, "chỉ viewAppointment "+m)
		expect(t, call(r, http.MethodGet, url, cashier, ""), http.StatusOK, "viewPayment "+m)
		expect(t, call(r, http.MethodGet, url, reporter, ""), http.StatusOK, "manageCrmReport "+m)
	}
	expect(t, call(r, http.MethodGet, "/api/crm/crm-dashboard/overview-appointment", receptionist, ""),
		http.StatusOK, "đối chứng: widget lịch hẹn vẫn mở cho viewAppointment")
}

func customerDuplicateMsg() string {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	return i18n.T(c, "customer_duplicate")
}

func msgOf(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var res map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if m, ok := res["message"].(string); ok {
		return m
	}
	return w.Body.String()
}

// D-3: 2 request tạo khách CÙNG SĐT chạy song song ⇒ đúng 1 thành công, 1 lỗi i18n "customer_duplicate";
// và đường 1062 (bước kiểm trước không bắt được định dạng lạ) cũng ra đúng lỗi i18n — cả tạo lẫn sửa.
func TestCustomerPhoneUnique(t *testing.T) {
	skipNoDB(t)
	tn := testhelpers.FreshTenant(t, testDB)
	r := engine(t, tn.BranchID)
	withEnforce(t, false)
	uid := testhelpers.CreateScopedUser(t, testDB, "b1_creator", tn, tn.BranchID)
	dupMsg := customerDuplicateMsg()
	cleanupPhone := func(phone string) {
		t.Cleanup(func() {
			testDB.Exec("DELETE FROM note WHERE customer_id IN (SELECT id FROM customer WHERE phone_unique_key = RIGHT(?, 9))", phone)
			testDB.Exec("DELETE FROM customer WHERE phone_unique_key = RIGHT(?, 9) OR phone LIKE ?", phone, "%"+phone[len(phone)-4:])
		})
	}

	t.Run("song song cùng SĐT", func(t *testing.T) {
		phone := uniquePhone()
		cleanupPhone(phone)
		var wg sync.WaitGroup
		res := make([]*httptest.ResponseRecorder, 2)
		for i := range res {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				body := fmt.Sprintf(`{"fullname": "B1 Song Song %d", "phone": %q, "branch_id": %d}`, i, phone, tn.BranchID)
				res[i] = call(r, http.MethodPost, "/api/crm/customer", uid, body)
			}(i)
		}
		wg.Wait()
		ok, dup := 0, 0
		for _, w := range res {
			switch {
			case w.Code == http.StatusCreated || w.Code == http.StatusOK:
				ok++
			case w.Code == http.StatusUnprocessableEntity && msgOf(t, w) == dupMsg:
				dup++
			default:
				t.Errorf("response lạ: HTTP %d %.300s", w.Code, w.Body.String())
			}
		}
		if ok != 1 || dup != 1 {
			t.Fatalf("⛔ phải đúng 1 tạo + 1 lỗi trùng, được tạo=%d trùng=%d", ok, dup)
		}
		var n int64
		testDB.Table("customer").Where("phone = ?", phone).Count(&n)
		if n != 1 {
			t.Fatalf("⛔ DB có %d khách cùng SĐT %s", n, phone)
		}
	})

	t.Run("1062 khi bước kiểm trước không bắt được (SĐT lưu có khoảng trắng)", func(t *testing.T) {
		phone := uniquePhone()
		cleanupPhone(phone)
		spaced := phone[:3] + " " + phone[3:6] + " " + phone[6:]
		now := time.Now().Unix()
		mustExec(t, "INSERT INTO customer (fullname, phone, status, created_day, created_at, updated_at) VALUES ('B1 Gốc', ?, 2, ?, ?, ?)",
			spaced, now, now, now)
		// Đối chứng: bước kiểm trước (PhoneMatchCond) KHÔNG khớp số có khoảng trắng ⇒ lỗi dưới đây chắc chắn đến từ UNIQUE.
		var pre int64
		testDB.Table("customer").Where("phone IN ? OR phone LIKE ?", []string{phone}, "%"+phone[1:]).Count(&pre)
		if pre != 0 {
			t.Fatalf("đối chứng hỏng: kiểm trước đã khớp %d dòng", pre)
		}
		w := call(r, http.MethodPost, "/api/crm/customer", uid,
			fmt.Sprintf(`{"fullname": "B1 Khác Tên", "phone": %q, "branch_id": %d}`, phone, tn.BranchID))
		expect(t, w, http.StatusUnprocessableEntity, "tạo trùng qua 1062")
		if got := msgOf(t, w); got != dupMsg {
			t.Fatalf("⛔ 1062 phải trả %q, được %q", dupMsg, got)
		}

		// Sửa: đổi SĐT của 1 khách khác sang số đã tồn tại ⇒ cùng lỗi i18n.
		other := uniquePhone()
		cleanupPhone(other)
		w = call(r, http.MethodPost, "/api/crm/customer", uid,
			fmt.Sprintf(`{"fullname": "B1 Khách Sửa", "phone": %q, "branch_id": %d}`, other, tn.BranchID))
		expect(t, w, http.StatusCreated, "tạo khách để sửa")
		var created struct {
			Data struct {
				ID        int64 `json:"id"`
				UpdatedAt int64 `json:"updated_at"`
			} `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &created)
		if created.Data.ID == 0 {
			t.Fatalf("không đọc được id khách vừa tạo: %.300s", w.Body.String())
		}
		w = call(r, http.MethodPut, fmt.Sprintf("/api/crm/customer/%d", created.Data.ID), uid,
			fmt.Sprintf(`{"updated_at": %d, "phone": %q}`, created.Data.UpdatedAt, phone))
		expect(t, w, http.StatusUnprocessableEntity, "sửa sang SĐT trùng")
		if got := msgOf(t, w); got != dupMsg {
			t.Fatalf("⛔ sửa trùng SĐT phải trả %q, được %q", dupMsg, got)
		}
	})
}

// D-3: UNIQUE SĐT ở tầng DB (cột sinh `phone_unique_key` = 9 số cuối + UNIQUE, có từ create_customer —
// migration nâng cấp riêng đã gộp vào file tạo bảng 2026-10-19). Neo 3 hành vi: các định dạng khác
// nhau của cùng một số (0…, 84…, +84 có dấu cách) đụng nhau; dòng `phone_dup_legacy = 1` được miễn
// (khoá NULL); SĐT rỗng không bị ràng buộc.
func TestCustomerPhoneUniqueKey_Schema(t *testing.T) {
	skipNoDB(t)
	phone := uniquePhone()
	local := phone[1:]
	now := time.Now().Unix()
	var ids []uint
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer WHERE id IN ?", ids) })
	insert := func(name, p string, legacy int) error {
		err := testDB.Exec("INSERT INTO customer (fullname, phone, phone_dup_legacy, status, created_day, created_at, updated_at) VALUES (?, ?, ?, 2, ?, ?, ?)",
			name, p, legacy, now, now, now).Error
		if err == nil {
			ids = append(ids, testhelpers.LastInsertID(testDB))
		}
		return err
	}

	if err := insert("B1 Gốc", phone, 0); err != nil {
		t.Fatalf("chèn bản gốc: %v", err)
	}
	for _, p := range []string{"84" + local, "+84 " + local[:3] + " " + local[3:]} {
		if err := insert("B1 Trùng", p, 0); err == nil {
			t.Fatalf("⛔ chèn được khách trùng SĐT dạng %q", p)
		}
	}
	if err := insert("B1 Legacy", "84"+local, 1); err != nil {
		t.Fatalf("dòng phone_dup_legacy = 1 phải được miễn UNIQUE: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := insert(fmt.Sprintf("B1 Rỗng %d", i), "", 0); err != nil {
			t.Fatalf("SĐT rỗng không được bị ràng buộc: %v", err)
		}
	}

	var rows []struct {
		Legacy int     `gorm:"column:phone_dup_legacy"`
		Key    *string `gorm:"column:phone_unique_key"`
	}
	if err := testDB.Table("customer").Select("phone_dup_legacy, phone_unique_key").Where("id IN ?", ids).
		Order("id").Scan(&rows).Error; err != nil {
		t.Fatalf("đọc lại: %v", err)
	}
	if len(rows) != 4 || rows[0].Key == nil || *rows[0].Key != local {
		t.Fatalf("bản gốc phải mang khoá %s, được %+v", local, rows)
	}
	if rows[1].Legacy != 1 || rows[1].Key != nil || rows[2].Key != nil || rows[3].Key != nil {
		t.Fatalf("dòng legacy và SĐT rỗng phải có khoá NULL, được %+v", rows)
	}
	var idx int64
	testDB.Raw(`SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE()
		AND TABLE_NAME = 'customer' AND INDEX_NAME = ? AND NON_UNIQUE = 0`, define.CustomerPhoneUniqueIndex).Scan(&idx)
	if idx == 0 {
		t.Fatalf("UNIQUE %s phải tồn tại", define.CustomerPhoneUniqueIndex)
	}
}
