//go:build integration

package contracttest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	treatmentServiceHandler "erpCluvix/internal/modules/webapp/crm/order_service_used/handler"
	crmReportHandler "erpCluvix/internal/modules/webapp/crm/report/handler"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

// crmDecimalCases — case của story-05 (cụm `webapp/crm/**`, 18 mục).
//
// Vì sao ở FILE RIÊNG thay vì append thẳng vào `decimalCases` (decimal_cases_test.go):
// story-05/08/09 chạy song song và cùng phải thêm case; `decimal_cases_test.go` là file dùng
// chung. Append qua `init()` là edit thuần-additive, không đụng một dòng nào của story khác.
// Go khởi tạo mọi package-level var TRƯỚC khi chạy init(), nên bảng luôn đầy đủ khi TestMain chạy.
//
// Bao phủ CẢ HAI cơ chế vá của epic:
//   - cột THẬT của bảng   → helpers.ParseDecimalFieldsForTables (#9, #10)
//   - alias SUM(...)/biểu thức → helpers.ParseDecimalFields      (#12, #18)
//
// và cả hai hàm respond dùng chung của cụm report:
// `paginateAndRespond` (#12/#13/#14) và `paginateAndRespondWithPercentages` (#16; #17 doctor-advise bỏ ở story-14).
var crmDecimalCases = []DecimalCase{
	{
		// story-05 #10 — cột thật của `order_service_used` + object `service` lồng bên trong
		// (2 query riêng, phải vá CẢ HAI: thiếu query `service` thì min_price/max_price vẫn ra string).
		Name:     "crm_treatment_service_get_service_used_of_customer",
		Endpoint: "GET /crm/order-service-used/:id/get-service-used-of-customer",
		Handler:  "webapp/crm/order_service_used/handler.GetServiceUsedOfCustomer",
		Call:     callCRMGetServiceUsedOfCustomer,
		NumberFields: []string{
			"data.totalEnd",
			"data.payment",
			"data.unit_price",
			"data.service.min_price",
			"data.service.max_price",
		},
	},
	{
		// story-05 #12 — [SUM-alias]. `dimensionStatistics` dùng chung cho 8 endpoint
		// (/source-, /pathology-, /province-, /country-, /category-, /page-facebook-,
		// /campain-event-, /interested-statistics); cả 3 mục #12/#13/#14 đều đi qua
		// `paginateAndRespond` nên case này là lưới canh cho cả cụm.
		Name:     "crm_report_source_statistics",
		Endpoint: "GET /crm/report/source-statistics",
		Handler:  "webapp/crm/report/handler.SourceStatistics (→ dimensionStatistics)",
		Call:     callCRMReportSourceStatistics,
		NumberFields: []string{
			"items.*.totalOrderPlan",
			"items.*.totalRevenue",
			"items.*.totalPayment",
			"items.*.totalDeposit",
			"items.*.totalRefundDeposit",
			"items.*.totalRefundServiceUsed",
			"items.*.duKienThu",
			"items.*.tbKhach",
		},
	},
	{
		// story-05 #18 — [SUM-alias]. `service_discount_amount` là alias của biểu thức CASE
		// (không trùng tên cột nào) → chỉ ParseDecimalFieldsForTables là KHÔNG đủ.
		Name:     "crm_report_service_type_statistics",
		Endpoint: "GET /crm/report/service-type-statistics",
		Handler:  "webapp/crm/report/handler.ServiceTypeStatistics",
		Call:     callCRMReportServiceTypeStatistics,
		NumberFields: []string{
			"items.*.totalEnd",
			"items.*.payment",
			"items.*.discount_other",
			"items.*.rank_discount_amount",
			"items.*.refundServiceUsed",
			"items.*.service_discount_amount",
		},
	},
	{
		// story-05 pass 2 (QA patch-1) — `ReportServiceUsed`: alias `service_discount_amount`
		// (SUM(CASE …)) KHÔNG phải cột của bảng nào ⇒ `pagination.Paginate` (khớp theo TÊN CỘT)
		// không phủ. Cùng alias, cùng file với #18 `ServiceTypeStatistics` đã vá ở pass 1 —
		// bất đối xứng list/summary. Các alias tiền còn lại trùng tên cột order_service_used
		// nên Paginate đã lo; vẫn assert để bắt regression nếu cột bị đổi tên/drop.
		Name:     "crm_report_service_used",
		Endpoint: "GET /crm/order-service-used/report-service-used",
		Handler:  "webapp/crm/report/handler.ReportServiceUsed",
		Call:     callCRMReportServiceUsed,
		NumberFields: []string{
			"object.items.*.service_discount_amount",
			"object.items.*.totalEnd",
			"object.items.*.payment",
			"object.items.*.discount_other",
			"object.items.*.rank_discount_amount",
			"object.items.*.refundServiceUsed",
		},
	},
}

// crm_order_plan_list_nested_service_used (nested-attach sau pagination.Paginate,
// GET /accountant/order-plan) XOÁ ở story-07: handler đó thuộc module `accountant` (đã xoá cùng
// module). Hành vi decimal của order_plan/order_service_used vẫn còn được canh gián
// tiếp qua các case khác trong file này (seedOrderServiceUsed vẫn dùng chung).

func init() { decimalCases = append(decimalCases, crmDecimalCases...) }

// ---------------------------------------------------------------------------
// pass 2 — GET /crm/order-service-used/report-service-used (alias sau Paginate)
// ---------------------------------------------------------------------------

func callCRMReportServiceUsed(t *testing.T, db *gorm.DB) *httptest.ResponseRecorder {
	t.Helper()

	tenant := testhelpers.FreshTenant(t, db)
	serviceTypeID := seedServiceType(t, db, tenant)
	serviceID := seedService(t, db, tenant, serviceTypeID)
	// service_discount_type = 1 ⇒ nhánh ELSE của CASE (lấy thẳng service_discount = 150000).
	seedOrderServiceUsed(t, db, tenant, map[string]any{"service_id": serviceID, "isLock": 1})

	h := crmReportHandler.New(crmReportHandler.Deps{DB: db})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// Guard dải ngày (2026-09-09): endpoint bắt buộc có `created_day` gte/lte và trần 1 năm —
	// thiếu là 422 và case decimal này sẽ không bao giờ chạm tới response thật.
	// `seedOrderServiceUsed` đặt `created_day = time.Now().Unix()` nên cửa sổ ±1 ngày là đủ.
	now := time.Now().Unix()
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf(
		"/crm/order-service-used/report-service-used?filter[created_day][gte]=%d&filter[created_day][lte]=%d",
		now-86400, now+86400), nil)
	setCRMScopeContext(c, tenant)

	h.ReportServiceUsed(c)
	return w
}

// seedOrderPlan tạo 1 đơn điều trị với mọi cột DECIMAL top-level khác 0.
func seedOrderPlan(t *testing.T, db *gorm.DB, tenant *testhelpers.TestTenant) uint {
	t.Helper()
	now := time.Now().Unix()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)

	row := testhelpers.WithTenantCols(tenant, map[string]any{
		"code":           "OTP" + suffix[:10],
		"customer_id":    tenant.CustomerID,
		"orderPlanType":  1,
		"discount_other": "120000.00",
		// KHÔNG seed `total`: cột thật đã bị DROP (migration 20260601221056); trong response
		// nó là alias COALESCE(SUM(osu.totalEnd),0) tính từ order_service_used.
		"paymentPlan":  "3000000.00",
		"debtsPlan":    "6000000.00",
		"status":       1,
		"created_time": now,
		"created_at":   now,
		"updated_at":   now,
	})
	if err := db.Table("order_plan").Create(&row).Error; err != nil {
		t.Fatalf("seed order_plan: %v", err)
	}
	id := testhelpers.LastInsertID(db)
	t.Cleanup(func() { db.Exec("DELETE FROM order_plan WHERE id = ?", id) })
	return id
}

// ---------------------------------------------------------------------------
// Seed dùng chung
// ---------------------------------------------------------------------------

// seedOrderServiceUsed tạo 1 order_service_used với MỌI cột DECIMAL được assert đặt giá trị
// khác NULL và khác 0. Giá trị đặt bằng string literal (không qua decimal.Decimal) để đường đi
// kiểm chứng đúng như runtime: DB → driver ([]uint8) → handler.
func seedOrderServiceUsed(t *testing.T, db *gorm.DB, tenant *testhelpers.TestTenant, extra map[string]any) uint {
	t.Helper()
	now := time.Now().Unix()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)

	row := testhelpers.WithTenantCols(tenant, map[string]any{
		"code":                  "OSU" + suffix[:10],
		"customer_id":           tenant.CustomerID,
		"service_id":            1,
		"order_plan_id":         1,
		"quantity":              2,
		"unit_price":            "4500000.00",
		"total":                 "9000000.00",
		"totalEnd":              "8100000.00",
		"payment":               "3000000.00",
		"discount_other":        "200000.00",
		"service_discount":      "150000.00",
		"service_discount_type": 1,
		"discountServiceLock":   "50000.00",
		"refundServiceUsed":     "75000.00",
		"rank_discount_amount":  "100000.00",
		"rank_discount_percent": "5.50",
		"isLock":                1,
		"created_day":           now,
		"created_at":            now,
		"updated_at":            now,
	})
	for k, v := range extra {
		row[k] = v
	}
	if err := db.Table("order_service_used").Create(&row).Error; err != nil {
		t.Fatalf("seed order_service_used: %v", err)
	}
	id := testhelpers.LastInsertID(db)
	t.Cleanup(func() { db.Exec("DELETE FROM order_service_used WHERE id = ?", id) })
	return id
}

// seedService tạo 1 service (có min_price/max_price DECIMAL) và trả id.
func seedService(t *testing.T, db *gorm.DB, tenant *testhelpers.TestTenant, serviceTypeID uint) uint {
	t.Helper()
	now := time.Now().Unix()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)

	row := testhelpers.WithTenantCols(tenant, map[string]any{
		"name":            "Test Service " + suffix,
		"code":            "SV" + suffix[:10],
		"service_type_id": serviceTypeID,
		"min_price":       "1200000.00",
		"max_price":       "7800000.00",
		"status":          1,
		"created_at":      now,
		"updated_at":      now,
	})
	delete(row, "branch_id") // service KHÔNG có cột branch_id
	if err := db.Table("service").Create(&row).Error; err != nil {
		t.Fatalf("seed service: %v", err)
	}
	id := testhelpers.LastInsertID(db)
	t.Cleanup(func() { db.Exec("DELETE FROM service WHERE id = ?", id) })
	return id
}

// setCRMScopeContext đặt các key session mà handler CRM đọc (branch active + ids).
func setCRMScopeContext(c *gin.Context, tenant *testhelpers.TestTenant) {
	c.Set("branchIds", []uint{tenant.BranchID})
	c.Set("branchActiveId", tenant.BranchID)
	c.Set("userID", uint(1))
}

// ---------------------------------------------------------------------------
// #10 — GET /crm/order-service-used/:id/get-service-used-of-customer
// ---------------------------------------------------------------------------

func callCRMGetServiceUsedOfCustomer(t *testing.T, db *gorm.DB) *httptest.ResponseRecorder {
	t.Helper()

	tenant := testhelpers.FreshTenant(t, db)
	serviceID := seedService(t, db, tenant, 0)
	osuID := seedOrderServiceUsed(t, db, tenant, map[string]any{"service_id": serviceID})
	id := strconv.FormatUint(uint64(osuID), 10)

	h := treatmentServiceHandler.New(treatmentServiceHandler.Deps{DB: db})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/crm/order-service-used/"+id+"/get-service-used-of-customer", nil)
	c.Params = gin.Params{{Key: "id", Value: id}}
	setCRMScopeContext(c, tenant)

	h.GetServiceUsedOfCustomer(c)
	return w
}

// ---------------------------------------------------------------------------
// #12 — GET /crm/report/source-statistics  (dimensionStatistics → paginateAndRespond)
// ---------------------------------------------------------------------------

func callCRMReportSourceStatistics(t *testing.T, db *gorm.DB) *httptest.ResponseRecorder {
	t.Helper()

	tenant := testhelpers.FreshTenant(t, db)
	sourceID := seedSource(t, db)

	// customer.source_id nuôi nhánh totalLead (điều kiện WHERE ... != 0 để dòng xuất hiện)
	// + các nhánh S/T (deposit) vốn join qua customer.
	if err := db.Table("customer").Where("id = ?", tenant.CustomerID).
		Updates(map[string]any{"source_id": sourceID}).Error; err != nil {
		t.Fatalf("set customer.source_id: %v", err)
	}
	osuID := seedOrderServiceUsed(t, db, tenant, map[string]any{"source_id": sourceID})
	seedSourceReportPayments(t, db, tenant, osuID)

	h := crmReportHandler.New(crmReportHandler.Deps{DB: db})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// dimensionStatistics BẮT BUỘC dải ngày (filter[created_at][gte]/[lte]) — thiếu ⇒ 422
	// report_date_range_required (guard chặn quét không giới hạn, xem crm.go dimensionStatistics).
	now := time.Now().Unix()
	url := fmt.Sprintf("/crm/report/source-statistics?filter[created_at][gte]=%d&filter[created_at][lte]=%d",
		now-86400, now+86400)
	c.Request = httptest.NewRequest(http.MethodGet, url, nil)
	setCRMScopeContext(c, tenant)

	h.SourceStatistics(c)
	return w
}

func seedSource(t *testing.T, db *gorm.DB) uint {
	t.Helper()
	now := time.Now().Unix()
	row := map[string]any{
		"name":       "Test Source " + strconv.FormatInt(time.Now().UnixNano(), 10),
		"status":     1,
		"created_at": now,
		"updated_at": now,
	}
	if err := db.Table("source").Create(&row).Error; err != nil {
		t.Fatalf("seed source: %v", err)
	}
	id := testhelpers.LastInsertID(db)
	t.Cleanup(func() { db.Exec("DELETE FROM source WHERE id = ?", id) })
	return id
}

// seedSourceReportPayments nuôi 4 nhánh subquery tiền của dimensionStatistics
// (Q/R qua osu.serviceUsed_id, S/T/U qua customer.source_id).
func seedSourceReportPayments(t *testing.T, db *gorm.DB, tenant *testhelpers.TestTenant, osuID uint) {
	t.Helper()
	now := time.Now().Unix()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)

	payments := []map[string]any{
		{"payment_for": define.PaymentForOrder, "amount": "2000000.00", "deposit_after": "0.00", "serviceUsed_id": osuID},
		{"payment_for": define.PaymentForDeposit, "amount": "500000.00", "deposit_after": "1200000.00", "serviceUsed_id": osuID},
		{"payment_for": define.PaymentForRefundDeposit, "amount": "300000.00", "deposit_after": "0.00"},
		{"payment_for": define.PaymentForRefundService, "amount": "150000.00", "deposit_after": "0.00", "serviceUsed_id": osuID},
	}
	for i, p := range payments {
		row := testhelpers.WithTenantCols(tenant, map[string]any{
			"code":            fmt.Sprintf("PM%s%d", suffix[:10], i),
			"customer_id":     tenant.CustomerID,
			"method":          1,
			"payment_time":    now,
			"payment_of_date": now,
			"created_at":      now,
			"updated_at":      now,
		})
		for k, v := range p {
			row[k] = v
		}
		if err := db.Table("payment").Create(&row).Error; err != nil {
			t.Fatalf("seed payment #%d: %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// #18 — GET /crm/report/service-type-statistics
// ---------------------------------------------------------------------------

func callCRMReportServiceTypeStatistics(t *testing.T, db *gorm.DB) *httptest.ResponseRecorder {
	t.Helper()

	tenant := testhelpers.FreshTenant(t, db)
	serviceTypeID := seedServiceType(t, db, tenant)
	serviceID := seedService(t, db, tenant, serviceTypeID)
	seedOrderServiceUsed(t, db, tenant, map[string]any{"service_id": serviceID, "isLock": 1})

	h := crmReportHandler.New(crmReportHandler.Deps{DB: db})

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/crm/report/service-type-statistics", nil)
	setCRMScopeContext(c, tenant)

	h.ServiceTypeStatistics(c)
	return w
}

func seedServiceType(t *testing.T, db *gorm.DB, tenant *testhelpers.TestTenant) uint {
	t.Helper()
	now := time.Now().Unix()
	row := testhelpers.WithTenantCols(tenant, map[string]any{
		"name":       "Test Service Type " + strconv.FormatInt(time.Now().UnixNano(), 10),
		"status":     1,
		"created_at": now,
		"updated_at": now,
	})
	delete(row, "branch_id") // service_type KHÔNG có cột branch_id
	if err := db.Table("service_type").Create(&row).Error; err != nil {
		t.Fatalf("seed service_type: %v", err)
	}
	id := testhelpers.LastInsertID(db)
	t.Cleanup(func() { db.Exec("DELETE FROM service_type WHERE id = ?", id) })
	return id
}
