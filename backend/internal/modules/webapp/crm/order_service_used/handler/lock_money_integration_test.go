//go:build integration

package handler

// lens-money M-14 (2026-10-18): khoá dịch vụ (LockServiceUsed) phải tính lại CẢ debtsPlan của đơn (bản cũ chỉ
// tính khách). Mở khoá ⇒ mọi thứ quay lại.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

func TestLockServiceUsed_RecalcOrderDebt(t *testing.T) {
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	planID := seedPlanS37(t, tn, tn.BranchID)
	now := time.Now().Unix()

	// Dịch vụ 9tr, đã thu 4tr ⇒ nợ 5tr ở khách lẫn đơn.
	osu := map[string]any{"code": fmt.Sprintf("SV%d", time.Now().UnixNano()), "customer_id": tn.CustomerID,
		"order_plan_id": planID, "service_id": 1, "branch_id": tn.BranchID, "quantity": 1,
		"totalEnd": 9_000_000, "payment": 4_000_000, "payment_status": define.ServicePaymentStatusPartly,
		"isLock": define.ServiceUsedNotLock, "created_at": now, "updated_at": now}
	require.NoError(t, testDB.Table("order_service_used").Create(&osu).Error)
	osuID := testhelpers.LastInsertID(testDB)
	require.NoError(t, testDB.Exec("INSERT INTO payment (code, customer_id, branch_id, method, amount, payment_for, parent_id, serviceUsed_id, payment_time, payment_of_date, created_at, updated_at) VALUES (?, ?, ?, 1, 4000000, ?, ?, ?, ?, ?, ?, ?)",
		fmt.Sprintf("TT%d", time.Now().UnixNano()), tn.CustomerID, tn.BranchID, define.PaymentForOrder, planID, osuID, now, now, now, now).Error)
	require.NoError(t, testDB.Exec("UPDATE order_plan SET paymentPlan = 4000000, debtsPlan = 5000000 WHERE id = ?", planID).Error)
	require.NoError(t, testDB.Exec("UPDATE customer SET totalRevenue = 9000000, totalPayment = 4000000, totalDebt = 5000000 WHERE id = ?", tn.CustomerID).Error)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM payment WHERE serviceUsed_id = ?", osuID)
		testDB.Exec("DELETE FROM order_service_used WHERE id = ?", osuID)
	})

	h := &Handler{DB: testDB}
	ctx := func(method, url string, body any) (*gin.Context, *httptest.ResponseRecorder) {
		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		c.Request = httptest.NewRequest(method, url, &buf)
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("branchActiveId", tn.BranchID)
		c.Set("branchIds", []uint{tn.BranchID})
		c.Set("userID", uint(1))
		return c, w
	}
	toggle := func(curLock int) {
		t.Helper()
		var ver int64
		require.NoError(t, testDB.Raw("SELECT updated_at FROM order_service_used WHERE id = ?", osuID).Scan(&ver).Error)
		c, w := ctx("POST", fmt.Sprintf("/crm/order-service-used/%d/lock-service-used", osuID), map[string]any{"isLock": curLock, "updated_at": ver})
		c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(osuID), 10)}}
		h.LockServiceUsed(c)
		require.Equal(t, 200, w.Code, "body: %s", w.Body.String())
	}
	dec := func(sql string, args ...any) decimal.Decimal {
		t.Helper()
		var d decimal.Decimal
		require.NoError(t, testDB.Raw(sql, args...).Scan(&d).Error)
		return d
	}
	eq := func(want int64, got decimal.Decimal, what string) {
		t.Helper()
		require.True(t, got.Equal(decimal.NewFromInt(want)), "%s: muốn %d, có %s", what, want, got)
	}
	// Khoá: totalEnd = 4tr (đã thu), phần còn lại 5tr thành giảm trừ.
	toggle(define.ServiceUsedNotLock)
	eq(4_000_000, dec("SELECT totalEnd FROM order_service_used WHERE id = ?", osuID), "khoá: osu.totalEnd")
	eq(4_000_000, dec("SELECT paymentPlan FROM order_plan WHERE id = ?", planID), "khoá: order.paymentPlan")
	eq(0, dec("SELECT debtsPlan FROM order_plan WHERE id = ?", planID), "khoá: order.debtsPlan (M-14)")
	eq(0, dec("SELECT totalDebt FROM customer WHERE id = ?", tn.CustomerID), "khoá: customer.totalDebt")

	// Mở khoá: quay lại 9tr / nợ 5tr.
	toggle(define.ServiceUsedIsLock)
	eq(9_000_000, dec("SELECT totalEnd FROM order_service_used WHERE id = ?", osuID), "mở: osu.totalEnd")
	eq(5_000_000, dec("SELECT debtsPlan FROM order_plan WHERE id = ?", planID), "mở: order.debtsPlan")
	eq(5_000_000, dec("SELECT totalDebt FROM customer WHERE id = ?", tn.CustomerID), "mở: customer.totalDebt")
}
