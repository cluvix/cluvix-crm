package seed

import (
	"strings"
	"testing"
)

// TestRoutePermMapStory49RenamedRoutes neo gate của các route đổi path/quyền ở crm-standalone story-49
// (định danh trung tính + gộp group đơn hàng). TestRoutePermMapCoverage chỉ kiểm "path có mapping hay
// không" — đổi perm sang một quyền hợp lệ khác, hoặc hạ xuống kind=public, vẫn xanh ở đó. Test này
// assert CỨNG kind + perm mong đợi cho từng route để lưới đỏ khi gate bị nới/sai.
func TestRoutePermMapStory49RenamedRoutes(t *testing.T) {
	entries, err := LoadRoutePermMap()
	if err != nil {
		t.Fatalf("LoadRoutePermMap: %v", err)
	}
	byKey := make(map[string]RoutePermEntry, len(entries))
	for _, e := range entries {
		byKey[e.Method+" "+e.Path] = e
	}

	expected := map[string]string{
		// group dòng dịch vụ đã dùng (path + quyền đổi tên trung tính ở story-49)
		"GET /api/crm/order-service-used":                                  "viewPlanOrder",
		"GET /api/crm/order-service-used/service-used-with-appointment":    "viewServiceUsed",
		"GET /api/crm/order-service-used/service-useds":                    "viewServiceUsed",
		"GET /api/crm/order-service-used/service-used-of-customer":         "viewServiceUsed",
		"GET /api/crm/order-service-used/:id":                              "viewServiceUsed",
		"GET /api/crm/order-service-used/:id/get-service-used-of-customer": "viewServiceUsed",
		"POST /api/crm/order-service-used/:id/lock-service-used":           "lockServiceUsed",
		"GET /api/crm/order-service-used/list-service-used-customer":       "viewServiceUsed",
		"GET /api/crm/order-service-used/report-service-used":              "manageCrmReport",
		// 3 route chuyển từ group URL đơn hàng cũ sang /crm/order-plan/* + 2 route sẵn có
		"POST /api/crm/order-plan":                                "createPlanOrder",
		"PUT /api/crm/order-plan/:id":                             "updatePlanOrder,createPlanOrder",
		"PUT /api/crm/order-plan/service-used/:id/change-service": "updatePlanOrder",
		"GET /api/crm/order-plan/:id":                             "viewPlanOrder",
		"DELETE /api/crm/order-plan/:id":                          "deletePlanOrder",
		// hoàn dịch vụ (quyền đổi tên trung tính ở story-49)
		"POST /api/crm/payment/create-refund-order-plan":       "extRefundService",
		"DELETE /api/crm/payment/:id/delete-refund-order-plan": "extRefundService",
		"GET /api/print/refund-service/:id":                    "viewPayment", // 2026-10-19: gộp quyền in phiếu vào viewPayment
	}

	for key, perm := range expected {
		e, ok := byKey[key]
		if !ok {
			t.Errorf("thiếu entry route_perm_map.json: %s", key)
			continue
		}
		if e.Kind != KindPerm {
			t.Errorf("%s: kind=%q, mong đợi %q (route bị nới gate)", key, e.Kind, KindPerm)
		}
		if e.Perm != perm {
			t.Errorf("%s: perm=%q, mong đợi %q", key, e.Perm, perm)
		}
	}

	// Mọi entry dưới /api/crm/order-service-used PHẢI nằm trong bảng trên (route mới thêm vào group
	// phải khai expected tường minh, không lọt lưới).
	for key := range byKey {
		if strings.Contains(key, " /api/crm/order-service-used") {
			if _, ok := expected[key]; !ok {
				t.Errorf("entry %s chưa có trong bảng expected của test story-49", key)
			}
		}
	}
}
