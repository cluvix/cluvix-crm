//go:build integration

package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

// Integration test cho GET /crm/order-plan/:id/detail.
//
// Vì sao cần test ở tầng HTTP thật chứ không phải unit test: toàn bộ rủi ro của endpoint này nằm ở
// SQL sinh ra lúc chạy — tenant scope gắn alias, cột có tồn tại sau migration hay không, mệnh đề
// lọc trong subquery tiền. `go build`/`go vet` không thấy gì trong số đó. Một lỗi alias tenant
// scope (MySQL 1054 → 500 mọi request) từng lọt qua 4 lớp kiểm tra tĩnh và chỉ có thể bị bắt bằng
// một request thật như dưới đây.

// --- fixtures ---------------------------------------------------------------

func seedOrderPlan(t *testing.T, tenant *testhelpers.TestTenant) uint {
	t.Helper()
	now := time.Now().Unix()
	row := testhelpers.WithTenantCols(tenant, map[string]any{
		"code":          fmt.Sprintf("DH%d", time.Now().UnixNano()),
		"orderPlanType": 1,
		"customer_id":   tenant.CustomerID,
		"note":          "ghi chú đơn",
		"created_time":  now,
		"status":        1,
		"created_at":    now,
		"updated_at":    now,
	})
	require.NoError(t, testDB.Table("order_plan").Create(&row).Error)
	id := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM order_service_used WHERE order_plan_id = ?", id)
		testDB.Exec("DELETE FROM payment WHERE parent_id = ?", id)
		testDB.Exec("DELETE FROM order_plan WHERE id = ?", id)
	})
	return id
}

func seedOrderService(t *testing.T, tenant *testhelpers.TestTenant, orderID uint, totalEnd, payment, refund int64) uint {
	t.Helper()
	now := time.Now().Unix()
	row := testhelpers.WithTenantCols(tenant, map[string]any{
		"code":                     fmt.Sprintf("SV%d", time.Now().UnixNano()),
		"customer_id":              tenant.CustomerID,
		"order_plan_id":            orderID,
		"service_id":               1,
		"quantity":                 1,
		"unit_price":               totalEnd,
		"totalEnd":                 totalEnd,
		"payment":                  payment,
		"refundServiceUsed":        refund,
		"isLock":                   1,
		"commission_telesale_snap": 12.5, // cố ý seed: test dưới khẳng định cột này KHÔNG lọt ra response
		"created_day":              now,
		"created_at":               now,
		"updated_at":               now,
	})
	require.NoError(t, testDB.Table("order_service_used").Create(&row).Error)
	return testhelpers.LastInsertID(testDB)
}

func seedOrderPayment(t *testing.T, tenant *testhelpers.TestTenant, orderID uint, amount int64, paymentFor int) {
	t.Helper()
	now := time.Now().Unix()
	row := testhelpers.WithTenantCols(tenant, map[string]any{
		// `payment.code` NOT NULL không default ở schema sau migration (fixture CreatePayment của
		// testhelpers chạy trên baseline nên không cần) — phải tự sinh.
		"code":         fmt.Sprintf("PT%d", time.Now().UnixNano()),
		"customer_id":  tenant.CustomerID,
		"parent_id":    orderID,
		"amount":       amount,
		"payment_for":  paymentFor,
		"method":       1, // NOT NULL không default (hình thức thanh toán)
		"payment_time": now,
		"created_at":   now,
		"updated_at":   now,
	})
	require.NoError(t, testDB.Table("payment").Create(&row).Error)
}

// callDetail gọi endpoint qua router thật (có middleware tenant) và trả status + body đã decode.
func callDetail(t *testing.T, branchID uint, path string) (int, map[string]any) {
	t.Helper()
	h := New(Deps{DB: testDB})
	r := testhelpers.NewGin()
	r.Use(testhelpers.AuthMiddleware(branchID, 1))
	r.GET("/:id/detail", h.GetOrderPlanDetail)

	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body map[string]any
	if w.Body.Len() > 0 {
		dec := json.NewDecoder(strings.NewReader(w.Body.String()))
		dec.UseNumber() // giữ nguyên số tiền, không để float64 làm tròn
		_ = dec.Decode(&body)
	}
	return w.Code, body
}

// --- tests ------------------------------------------------------------------

// Đường chính: đơn có 2 dịch vụ, 2 phiếu thu hợp lệ + 1 phiếu KHÔNG tính.
func TestGetOrderPlanDetail_ReturnsOrderWithServicesAndMoney(t *testing.T) {
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}

	tenant := testhelpers.FreshTenant(t, testDB)
	orderID := seedOrderPlan(t, tenant)
	svcVat := seedOrderService(t, tenant, orderID, 1_000_000, 400_000, 0)
	seedOrderService(t, tenant, orderID, 500_000, 100_000, 50_000)
	// VAT (bmad/order-vat) chỉ trên 1 dòng — dòng kia vat_amount = 0 (DEFAULT) ⇒ vat_total chỉ là của dòng này.
	require.NoError(t, testDB.Exec("UPDATE order_service_used SET vat_amount = 80000 WHERE id = ?", svcVat).Error)

	seedOrderPayment(t, tenant, orderID, 400_000, define.PaymentForOrder)
	seedOrderPayment(t, tenant, orderID, 100_000, define.PaymentForMoveDeposit)
	// Phiếu KHÔNG thuộc nhóm tính vào đơn — đối chứng cho mệnh đề payment_for IN (...):
	// bỏ điều kiện đó thì `payment` thành 700.000 và test này đỏ.
	seedOrderPayment(t, tenant, orderID, 200_000, define.PaymentForDeposit)

	code, body := callDetail(t, tenant.BranchID, fmt.Sprintf("/%d/detail", orderID))
	require.Equal(t, 200, code, "endpoint phải trả 200; 500 ở đây thường là lỗi SQL runtime (alias/cột thiếu)")

	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "response phải có object `data`")

	services, ok := data["orderServiceUseds"].([]any)
	require.True(t, ok, "phải có mảng orderServiceUseds")
	require.Len(t, services, 2)

	assert.Equal(t, "1500000", fmt.Sprint(data["total"]), "total = tổng totalEnd của dịch vụ con (TRƯỚC thuế)")
	assert.Equal(t, "80000", fmt.Sprint(data["vat_total"]), "vat_total = tổng vat_amount của dịch vụ con")
	assert.Equal(t, "1580000", fmt.Sprint(data["total_after_vat"]), "total_after_vat = total + vat_total")
	assert.Contains(t, data, "vat_rate", "chi tiết đơn phải trả mức VAT của đơn")
	assert.Equal(t, "500000", fmt.Sprint(data["payment"]), "chỉ cộng phiếu thu payment_for IN (order, move_deposit)")
	assert.Equal(t, "50000", fmt.Sprint(data["refundServiceTotal"]))

	// story-10 crm-standalone: mã giảm giá + CTV giới thiệu đã gỡ cùng module loyalty — response
	// không còn 2 khoá này, và đơn không còn cột broker_id trong danh sách cột chi tiết.
	assert.NotContains(t, data, "couponUsedByOrderPlan")
	assert.NotContains(t, data, "broker")
	assert.NotContains(t, data, "broker_id")
}

// Response KHÔNG được chứa nhóm hoa hồng — đây là lý do endpoint này tồn tại thay vì dùng lại
// `GET /:id` (endpoint kia SELECT * nên trả cả `commission_rate_snap`).
func TestGetOrderPlanDetail_DoesNotLeakCommissionFields(t *testing.T) {
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}

	tenant := testhelpers.FreshTenant(t, testDB)
	orderID := seedOrderPlan(t, tenant)
	seedOrderService(t, tenant, orderID, 1_000_000, 0, 0)

	code, body := callDetail(t, tenant.BranchID, fmt.Sprintf("/%d/detail", orderID))
	require.Equal(t, 200, code)

	data := body["data"].(map[string]any)
	services := data["orderServiceUseds"].([]any)
	require.Len(t, services, 1)
	svc := services[0].(map[string]any)

	// ĐỐI CHỨNG: khẳng định dịch vụ CÓ trả field bình thường, để test không xanh nhờ mảng rỗng
	// hay object rỗng (khi đó mọi assert "không chứa" đều đúng một cách vô nghĩa).
	require.Contains(t, svc, "totalEnd", "dịch vụ phải có totalEnd — nếu thiếu, các assert bên dưới vô nghĩa")

	for _, leaked := range []string{"commission_rate_snap", "status_commission", "commission_telesale_snap"} {
		assert.NotContains(t, svc, leaked, "cột hoa hồng không được lọt ra response")
	}
}

// id không parse được là INPUT SAI → 422. KHÔNG được trả 404: interceptor FE coi mọi 404 là lỗi
// toàn cục (điều hướng + toast toàn app).
func TestGetOrderPlanDetail_InvalidID_Returns422(t *testing.T) {
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}

	code, _ := callDetail(t, 1, "/undefined/detail")
	assert.Equal(t, 422, code)
}

// id hợp lệ nhưng không tồn tại → 404 (nhánh khác hẳn ca 422 ở trên).
func TestGetOrderPlanDetail_NotFound_Returns404(t *testing.T) {
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}

	code, _ := callDetail(t, 1, "/99999999/detail")
	assert.Equal(t, 404, code)
}

// Đơn của tenant khác → 404, không rò dữ liệu.
func TestGetOrderPlanDetail_OtherTenant_Returns404(t *testing.T) {
	if testDB == nil {
		t.Skip("integration DB not available (set TEST_MYSQL_DSN)")
	}

	owner := testhelpers.FreshTenant(t, testDB)
	orderID := seedOrderPlan(t, owner)
	seedOrderService(t, owner, orderID, 1_000_000, 0, 0)

	other := testhelpers.FreshTenant(t, testDB)
	code, body := callDetail(t, other.BranchID, fmt.Sprintf("/%d/detail", orderID))

	assert.Equal(t, 404, code)
	assert.NotContains(t, fmt.Sprint(body), "orderServiceUseds")

	// ĐỐI CHỨNG: cùng đơn đó, đúng tenant thì phải 200 — chứng minh 404 ở trên đến từ tenant scope
	// chứ không phải vì fixture hỏng / đơn không tồn tại.
	codeOwner, _ := callDetail(t, owner.BranchID, fmt.Sprintf("/%d/detail", orderID))
	require.Equal(t, 200, codeOwner)
}
