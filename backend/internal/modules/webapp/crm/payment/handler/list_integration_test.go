//go:build integration

package handler

// Integration test cho các list phiếu (handler/list.go). Neo: điều kiện riêng từng list (payment_for /
// deposit_type / deposit_after / amount ≠ 0), scope chi nhánh, dòng tổng khớp danh sách, `_keyword`.

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	authSvc "erpCluvix/internal/modules/webapp/auth/rbac/service"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

type listPayRow struct {
	branchID     uint
	amount       int64
	paymentFor   int
	depositType  *int
	depositAfter int64
}

func seedListPayment(t *testing.T, tn *testhelpers.TestTenant, r listPayRow) (uint, string) {
	t.Helper()
	now := time.Now().Unix()
	code := fmt.Sprintf("PL%d", time.Now().UnixNano())
	row := map[string]any{
		"code":          code,
		"customer_id":   tn.CustomerID,
		"branch_id":     r.branchID,
		"amount":        r.amount,
		"deposit_after": r.depositAfter,
		"method":        1,
		"payment_for":   r.paymentFor,
		"payment_time":  now,
		"created_at":    now,
		"updated_at":    now,
	}
	if r.depositType != nil {
		row["deposit_type"] = *r.depositType
	} else {
		row["deposit_type"] = nil
	}
	require.NoError(t, testDB.Table("payment").Create(&row).Error)
	id := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() { testDB.Exec("DELETE FROM payment WHERE id = ?", id) })
	return id, code
}

func callPaymentList(t *testing.T, branchID uint, h gin.HandlerFunc, rawQuery string) (int, map[string]any) {
	t.Helper()
	r := testhelpers.NewGin()
	r.Use(testhelpers.AuthMiddleware(branchID, 1))
	r.GET("/list", h)
	req := httptest.NewRequest("GET", "/list?"+rawQuery, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body map[string]any
	if w.Body.Len() > 0 {
		dec := json.NewDecoder(strings.NewReader(w.Body.String()))
		dec.UseNumber()
		require.NoError(t, dec.Decode(&body), "body=%s", w.Body.String())
	}
	return w.Code, body
}

func listIDs(t *testing.T, body map[string]any) []string {
	t.Helper()
	obj, ok := body["object"].(map[string]any)
	require.True(t, ok, "body=%v", body)
	items, ok := obj["items"].([]any)
	require.True(t, ok, "object=%v", obj)
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, fmt.Sprint(it.(map[string]any)["id"]))
	}
	sort.Strings(ids)
	return ids
}

func idStrs(ids ...uint) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, fmt.Sprint(id))
	}
	sort.Strings(out)
	return out
}

func intp(v int) *int { return &v }

func TestListReceipts_DieuKienVaScope(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	branchB := testhelpers.CreateBranch(t, testDB)
	a := tn.BranchID

	order, _ := seedListPayment(t, tn, listPayRow{branchID: a, amount: 100_000, paymentFor: define.PaymentForOrder})
	dep, depCode := seedListPayment(t, tn, listPayRow{branchID: a, amount: 200_000, paymentFor: define.PaymentForDeposit, depositType: intp(1), depositAfter: 200_000})
	depNull, _ := seedListPayment(t, tn, listPayRow{branchID: a, amount: 50_000, paymentFor: define.PaymentForDeposit})
	refund, _ := seedListPayment(t, tn, listPayRow{branchID: a, amount: -30_000, paymentFor: define.PaymentForRefundDeposit})
	income, _ := seedListPayment(t, tn, listPayRow{branchID: a, amount: 70_000, paymentFor: define.PaymentForClinicIncome})
	// Loại trừ: ký gửi, số tiền 0, chuyển cọc (không thuộc tập thu/chi), chi nhánh khác.
	seedListPayment(t, tn, listPayRow{branchID: a, amount: 90_000, paymentFor: define.PaymentForDeposit, depositType: intp(define.PaymentTypeDepositKG), depositAfter: 90_000})
	seedListPayment(t, tn, listPayRow{branchID: a, amount: 0, paymentFor: define.PaymentForOrder})
	seedListPayment(t, tn, listPayRow{branchID: a, amount: 40_000, paymentFor: define.PaymentForMoveDeposit})
	seedListPayment(t, tn, listPayRow{branchID: branchB, amount: 999_000, paymentFor: define.PaymentForOrder})

	h := New(Deps{DB: testDB})
	code, body := callPaymentList(t, a, h.ListReceipts, "per-page=50")
	require.Equal(t, 200, code, "body=%v", body)
	require.Equal(t, idStrs(order, dep, depNull, refund, income), listIDs(t, body))
	// 100k + 200k + 50k − 30k + 70k
	require.Equal(t, "390000", fmt.Sprint(body["totalSum"]))
	obj := body["object"].(map[string]any)
	require.Equal(t, "5", fmt.Sprint(obj["_meta"].(map[string]any)["totalCount"]), "câu tổng khớp danh sách")

	// Quan hệ lồng: khách hàng được gắn.
	first := obj["items"].([]any)[0].(map[string]any)
	cust, ok := first["customer"].(map[string]any)
	require.True(t, ok && fmt.Sprint(cust["id"]) == fmt.Sprint(tn.CustomerID), "customer=%v", first["customer"])

	// `_keyword` theo mã phiếu: chỉ còn đúng phiếu đó, tổng theo đúng tập lọc.
	code, body = callPaymentList(t, a, h.ListReceipts, "_keyword="+depCode)
	require.Equal(t, 200, code, "body=%v", body)
	require.Equal(t, idStrs(dep), listIDs(t, body))
	require.Equal(t, "200000", fmt.Sprint(body["totalSum"]))

	// Lọc loại phiếu (filter[payment_for][in][]).
	code, body = callPaymentList(t, a, h.ListReceipts, "filter[payment_for][in][0]=7")
	require.Equal(t, 200, code, "body=%v", body)
	require.Equal(t, idStrs(income), listIDs(t, body))
}

func TestListDeposit_TachKyGuiVaConLai(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	a := tn.BranchID

	depA, _ := seedListPayment(t, tn, listPayRow{branchID: a, amount: 300_000, paymentFor: define.PaymentForDeposit, depositType: intp(1), depositAfter: 120_000})
	depB, _ := seedListPayment(t, tn, listPayRow{branchID: a, amount: 100_000, paymentFor: define.PaymentForDeposit, depositAfter: 100_000})
	seedListPayment(t, tn, listPayRow{branchID: a, amount: 80_000, paymentFor: define.PaymentForDeposit, depositType: intp(1), depositAfter: 0}) // dùng hết
	// Phiếu ký gửi (dữ liệu cũ — tính năng ký gửi đã gỡ) không được lọt vào list đặt cọc.
	seedListPayment(t, tn, listPayRow{branchID: a, amount: 500_000, paymentFor: define.PaymentForDeposit, depositType: intp(define.PaymentTypeDepositKG), depositAfter: 400_000})

	h := New(Deps{DB: testDB})
	code, body := callPaymentList(t, a, h.ListDeposit, "")
	require.Equal(t, 200, code, "body=%v", body)
	require.Equal(t, idStrs(depA, depB), listIDs(t, body))
	require.Equal(t, "400000", fmt.Sprint(body["totalSumDeposit"]))
	require.Equal(t, "220000", fmt.Sprint(body["totalSumDepositAfter"]))

	// Lọc loại cọc: dòng tổng đi CÙNG bộ lọc với danh sách.
	code, body = callPaymentList(t, a, h.ListDeposit, "filter[deposit_type][in][0]=1")
	require.Equal(t, 200, code, "body=%v", body)
	require.Equal(t, idStrs(depA), listIDs(t, body))
	require.Equal(t, "120000", fmt.Sprint(body["totalSumDepositAfter"]))
}

// Nhánh `_export=1` của cả 5 list: câu export (hàm có sẵn trong package export) phải chạy được trên
// khung lọc chung — alias JOIN của export (`ose`, `svc`, `otp`…) không được đụng bảng JOIN của khung.
func TestListPayments_ExportChayDuoc(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	a := tn.BranchID
	planID, suID := seedPaymentOrderRefs(t, tn)
	seedPaymentRow(t, tn, planID, suID, 100_000, define.PaymentForOrder)
	seedListPayment(t, tn, listPayRow{branchID: a, amount: 300_000, paymentFor: define.PaymentForDeposit, depositType: intp(1), depositAfter: 300_000})
	seedListPayment(t, tn, listPayRow{branchID: a, amount: -10_000, paymentFor: define.PaymentForRefundDeposit})
	seedListPayment(t, tn, listPayRow{branchID: a, amount: -20_000, paymentFor: define.PaymentForRefundService})

	// Export ghi file vào ./storage/export tương đối với cwd ⇒ chạy trong thư mục tạm của test.
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(wd) })

	// Export cần quyền extPayment — chạy bằng user role admin (super role tự có mọi quyền).
	uid := seedRbacUser(t, "admin", []string{define.PermExtPayment}) // super role lấy mọi rbac_auth_item type 2
	h := New(Deps{DB: testDB, RbacSvc: &authSvc.RbacService{DB: testDB}})
	for name, fn := range map[string]gin.HandlerFunc{
		"payment": h.ListPayment, "receipts": h.ListReceipts, "deposit": h.ListDeposit,
		"refund-deposit": h.ListRefundDeposit, "refund-order-plan": h.ListRefundOrderPlan,
	} {
		r := testhelpers.NewGin()
		r.Use(testhelpers.AuthMiddleware(a, uid))
		r.GET("/list", fn)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/list?_export=1&sort=-payment_time", nil))
		require.Equal(t, 200, w.Code, "%s: body=%s", name, w.Body.String())
		require.Contains(t, w.Body.String(), ".xlsx", "%s: phải trả URL file", name)
	}
}

// seedRbacUser tạo user + gán `role`; `perms` ≠ nil ⇒ role đó được cấp đúng các quyền này
// (rbac_auth_item type 2 + item_child). Dọn hết khi test kết thúc.
func seedRbacUser(t *testing.T, role string, perms []string) uint {
	t.Helper()
	uid := testhelpers.CreateUser(t, testDB, "rbac", nil)
	require.NoError(t, testDB.Exec("INSERT IGNORE INTO rbac_auth_item (name, type, description) VALUES (?, 1, ?)", role, role).Error)
	for _, p := range perms {
		require.NoError(t, testDB.Exec("INSERT IGNORE INTO rbac_auth_item (name, type, description) VALUES (?, 2, ?)", p, p).Error)
		require.NoError(t, testDB.Exec("INSERT IGNORE INTO rbac_auth_item_child (parent, child) VALUES (?, ?)", role, p).Error)
	}
	require.NoError(t, testDB.Exec("INSERT INTO rbac_auth_assignment (item_name, user_id, created_at) VALUES (?, ?, 0)", role, uid).Error)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM rbac_auth_assignment WHERE user_id = ?", uid)
		for _, p := range perms {
			testDB.Exec("DELETE FROM rbac_auth_item_child WHERE parent = ? AND child = ?", role, p)
		}
	})
	return uid
}

// Export 5 list phiếu phải có quyền extPayment: không có ⇒ 403 (kể cả có viewPayment), có ⇒ 200.
func TestListPayments_ExportCanQuyenExtPayment(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(wd) })

	// Mỗi list export cần ≥1 dòng (rỗng ⇒ 422 "không có dữ liệu"): seed đủ cho 5 list.
	planID, suID := seedPaymentOrderRefs(t, tn)
	seedPaymentRow(t, tn, planID, suID, 100_000, define.PaymentForOrder)
	seedListPayment(t, tn, listPayRow{branchID: tn.BranchID, amount: 300_000, paymentFor: define.PaymentForDeposit, depositType: intp(1), depositAfter: 300_000})
	seedListPayment(t, tn, listPayRow{branchID: tn.BranchID, amount: -10_000, paymentFor: define.PaymentForRefundDeposit})
	seedListPayment(t, tn, listPayRow{branchID: tn.BranchID, amount: -20_000, paymentFor: define.PaymentForRefundService})

	h := New(Deps{DB: testDB, RbacSvc: &authSvc.RbacService{DB: testDB}})
	noExt := seedRbacUser(t, fmt.Sprintf("r_noext_%d", time.Now().UnixNano()), []string{"viewPayment"})
	withExt := seedRbacUser(t, fmt.Sprintf("r_ext_%d", time.Now().UnixNano()), []string{"viewPayment", define.PermExtPayment})

	call := func(uid uint, fn gin.HandlerFunc, q string) *httptest.ResponseRecorder {
		r := testhelpers.NewGin()
		r.Use(testhelpers.AuthMiddleware(tn.BranchID, uid))
		r.GET("/list", fn)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/list?"+q, nil))
		return w
	}
	for name, fn := range map[string]gin.HandlerFunc{
		"payment": h.ListPayment, "receipts": h.ListReceipts, "deposit": h.ListDeposit,
		"refund-deposit": h.ListRefundDeposit, "refund-order-plan": h.ListRefundOrderPlan,
	} {
		w := call(noExt, fn, "_export=1")
		require.Equal(t, 403, w.Code, "%s thiếu extPayment phải 403: %s", name, w.Body.String())
		require.Equal(t, 200, call(noExt, fn, "").Code, "%s: list thường KHÔNG bị chặn", name)
		w = call(withExt, fn, "_export=1")
		require.Equal(t, 200, w.Code, "%s có extPayment phải 200: %s", name, w.Body.String())
	}
}
