//go:build integration

package debt

// Neo con số của các báo cáo công nợ (theo đơn, theo khách, theo dịch vụ) trên DB dùng-một-lần (TEST_MYSQL_DSN, tên *_tmp/*_test).

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"erpCluvix/internal/deps"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaAll)
	if err != nil {
		if errors.Is(err, testhelpers.ErrNoDSN) {
			log.Printf("testhelpers: %v — skipping integration tests", err)
			os.Exit(m.Run())
		}
		log.Fatalf("testhelpers: đã có TEST_MYSQL_DSN nhưng dựng schema hỏng: %v", err)
	}
	defer s.Close()
	testDB = s.DB
	os.Exit(m.Run())
}

func seedRow(t *testing.T, table string, row map[string]any) uint {
	t.Helper()
	require.NoError(t, testDB.Table(table).Create(&row).Error, "seed %s", table)
	id := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() { testDB.Exec("DELETE FROM "+table+" WHERE id = ?", id) })
	return id
}

func getJSON(t *testing.T, branchID uint, url string) map[string]any {
	t.Helper()
	r := testhelpers.NewGin()
	Register(r.Group("/crm", testhelpers.AuthMiddleware(branchID, 0)), &deps.Deps{DB: testDB})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
	require.Equal(t, 200, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func requireMoney(t *testing.T, want int64, got any, what string) {
	t.Helper()
	d, err := decimal.NewFromString(fmt.Sprint(got))
	require.NoError(t, err, "%s = %v", what, got)
	require.True(t, d.Equal(decimal.NewFromInt(want)), "%s: want %d got %s", what, want, d)
}

// seedPlanService: 1 đơn + 1 dòng dịch vụ của khách tn.
func seedPlanService(t *testing.T, tn *testhelpers.TestTenant, branchID uint, totalEnd, createdDay int64, isLock int) (uint, uint) {
	t.Helper()
	plan := seedRow(t, "order_plan", map[string]any{
		"code": fmt.Sprintf("DH%d", time.Now().UnixNano()%1e9), "customer_id": tn.CustomerID,
		"branch_id": branchID, "created_time": createdDay,
	})
	osu := seedRow(t, "order_service_used", map[string]any{
		"code": fmt.Sprintf("DV%d", time.Now().UnixNano()%1e9), "customer_id": tn.CustomerID,
		"order_plan_id": plan, "service_id": 1, "branch_id": branchID, "quantity": 1,
		"totalEnd": totalEnd, "payment": 0, "created_day": createdDay, "isLock": isLock,
	})
	return plan, osu
}

func TestOrderPlanDebts_PeriodMathIgnoresRefund(t *testing.T) {
	if testDB == nil {
		t.Skip("no TEST_MYSQL_DSN")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	from := time.Now().Add(-24 * time.Hour).Unix()
	to := time.Now().Add(24 * time.Hour).Unix()
	before := from - 3600
	in := from + 3600

	plan, osuOld := seedPlanService(t, tn, tn.BranchID, 1000, before, 1)
	osuNew := seedRow(t, "order_service_used", map[string]any{
		"code": "DVNEW", "customer_id": tn.CustomerID, "order_plan_id": plan, "service_id": 1,
		"branch_id": tn.BranchID, "quantity": 1, "totalEnd": 500, "payment": 0, "created_day": in,
	})
	pay := func(osuID uint, amount int64, at int64, paymentFor int) {
		seedRow(t, "payment", map[string]any{
			"code": fmt.Sprintf("TT%d", time.Now().UnixNano()%1e9), "branch_id": tn.BranchID, "method": 1,
			"amount": amount, "serviceUsed_id": osuID, "payment_for": paymentFor, "parent_id": plan,
			"customer_id": tn.CustomerID, "payment_of_date": at,
		})
	}
	pay(osuOld, 300, before, define.PaymentForOrder)
	pay(osuOld, 200, in, define.PaymentForOrder)
	pay(osuNew, -100, in, define.PaymentForRefundService) // hoàn = huỷ bớt dịch vụ ⇒ không đổi công nợ

	// Đơn ở chi nhánh khác — không được lọt vào.
	other := testhelpers.CreateBranch(t, testDB)
	seedPlanService(t, tn, other, 9999, in, 1)

	body := getJSON(t, tn.BranchID, fmt.Sprintf(
		"/crm/report/debt/order-plan?filter[created_time][gte]=%d&filter[created_time][lte]=%d", from, to))
	total := body["total"].(map[string]any)
	requireMoney(t, 700, total["totalEndBefore"], "totalEndBefore") // 1000 − 300
	requireMoney(t, 500, total["totalEndIn"], "totalEndIn")
	requireMoney(t, 200, total["daThuTrongKy"], "daThuTrongKy")
	requireMoney(t, 1000, total["totalEndOrder"], "totalEndOrder") // 700 + 500 − 200
	items := body["items"].([]any)
	require.Len(t, items, 1)
	row := items[0].(map[string]any)
	require.EqualValues(t, plan, row["id"])
	requireMoney(t, 1000, row["totalEndOrder"], "row.totalEndOrder")

	// Tham số ngày không phải số ⇒ 422, không đi vào SQL.
	r := testhelpers.NewGin()
	Register(r.Group("/crm", testhelpers.AuthMiddleware(tn.BranchID, 0)), &deps.Deps{DB: testDB})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/crm/report/debt/order-plan?filter[created_time][gte]=1abc", nil))
	require.Equal(t, 422, w.Code)
}

func TestCustomerAndServiceDebts_Totals(t *testing.T) {
	if testDB == nil {
		t.Skip("no TEST_MYSQL_DSN")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	require.NoError(t, testDB.Exec("UPDATE customer SET branch_id = ?, totalRevenue = 1000, totalPayment = 400, totalDebt = 600, totalDeposit = 50 WHERE id = ?",
		tn.BranchID, tn.CustomerID).Error)
	body := getJSON(t, tn.BranchID, "/crm/report/debt/customer")
	requireMoney(t, 600, body["totalDebts"], "totalDebts")
	require.Len(t, body["items"].([]any), 1)

	svc := seedRow(t, "service", map[string]any{"name": "DV debt", "code": fmt.Sprintf("S%d", time.Now().UnixNano()%1e9),
		"serviceCategory_id": define.ServiceCategoryService})
	now := time.Now().Unix()
	_, osu := seedPlanService(t, tn, tn.BranchID, 1000, now, 1)
	require.NoError(t, testDB.Exec("UPDATE order_service_used SET service_id = ?, payment = 400 WHERE id = ?", svc, osu).Error)
	body = getJSON(t, tn.BranchID, fmt.Sprintf("/crm/report/debt/service?_orderCreatedTime[gte]=%d&_orderCreatedTime[lte]=%d&_keyword=DV", now-60, now+60))
	total := body["total"].(map[string]any)
	requireMoney(t, 600, total["totalDebt"], "totalDebt")
	requireMoney(t, 50, total["totalDeposit"], "totalDeposit")
	require.Len(t, body["items"].([]any), 1)
}
