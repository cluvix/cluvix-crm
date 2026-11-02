package seed

import (
	"sort"
	"strings"
	"testing"
)

// crmDashboardPathPrefix / crmMobilePathPrefix — 2 cửa vào dashboard CRM (web + mobile BFF
// delegate). mobile/v1/crm/<x> gọi thẳng xuống cùng handler với crm-dashboard/<x> (xem
// "Đổi kiểu field handler webapp/* có delegate mobile" — skill/skills/backend-hard-rules).
const (
	crmDashboardPathPrefix = "/api/crm/crm-dashboard/"
	crmMobilePathPrefix    = "/api/mobile/v1/crm/"
)

// crmModulePermCSV — tập quyền raw (OR-gate) gate "quyền module CRM" cho dashboard CRM.
// story-16 (be-rbac-trim, 2026-09-24, prd D17/E7 chuẩn bị): trim 7 → 3 quyền — bỏ
// viewCheckUp/viewServiceUsed (story-49 đổi tên quyền liệu trình cũ)/viewLaboOrder/viewWarrantyCard khỏi tập GATE MENU dashboard CRM.
// QA Pass 1 (2026-09-24) soát lại: `viewCheckUp`/`viewLaboOrder`/`viewWarrantyCard` KHÔNG còn gate
// route nào (0 entry route_perm_map.json, 0 caller Go) — route CRUD check-up/labo-order/
// customer-warranty-card đã mồ côi từ trước (xoá ở story-13/14), 3 quyền này đã xoá khỏi
// permissions.gen.json luôn (55→52). Riêng `viewServiceUsed` GIỮ trong catalog — vẫn gate 7
// route thật đang sống (`/api/crm/order-service-used/*`; story-49 đổi tên quyền + path sang tên
// trung tính và bỏ route duyệt thư mục đính kèm).
// Nguồn khai gốc (test này KHÔNG đọc được FE/mobile — sửa tập ở BẤT KỲ nơi nào trong 3 nơi dưới
// đây thì PHẢI sửa đồng thời cả 2 nơi còn lại, kể cả hằng số này):
//  1. frontend/src/app/define/permission.define.ts — hằng CRM_MODULE_PERMISSIONS (dùng bởi
//     layout/components/system-module-menu.ts + modules/utilities/home/home.ts) — story-22
//  2. backend/internal/modules/webapp/auth/rbac/seed/route_perm_map.json — 17 entry
//     GET /api/crm/crm-dashboard/* (kind=perm). Mobile /api/mobile/v1/crm/* đã xoá cả (module
//     mobile không còn route mount nào — story-16 trim luôn 20 entry mồ côi đó, xem
//     TestCrmDashboardMobileParity/TestCrmMobileCoverageSurvey bên dưới).
//  3. mobile/packages/core_domain/lib/src/permissions.dart — kPermsCrmModule (mobile app không còn
//     tồn tại trong repo crm-standalone — N/A, giữ ghi chú cho lịch sử/tham chiếu ERP gốc)
const crmModulePermCSV = "viewAppointment,viewCustomer,viewPlanOrder"

// crmDashboardMoneyPermCSV — R-5 (rereview crm-prod-readiness, 2026-10-18): widget dashboard trả SỐ TIỀN
// (doanh thu/thực thu/công nợ) KHÔNG dùng tập quyền module CRM (lễ tân chỉ có viewAppointment cũng mở
// được) mà đòi quyền xem tiền: viewPayment (đã xem được danh sách phiếu + tổng tiền) OR manageCrmReport
// (báo cáo doanh thu). FE ẩn đúng 3 widget này bằng *hasPerm cùng tập (modules/crm/dashboard).
const crmDashboardMoneyPermCSV = "viewPayment,manageCrmReport"

// crmDashboardMoneySuffixes — endpoint dashboard trả tiền (phần path sau crmDashboardPathPrefix).
var crmDashboardMoneySuffixes = map[string]bool{
	"payment-in-month":        true,
	"payment-branch-overview": true,
	"service-used-statistics": true,
}

// crmDashboardBrandDefaultSuffixes — route ghi "mặc định bố cục thương hiệu" (dashboard tuỳ biến, 2026-10-19):
// chỉ người có manageConfig (handler kiểm thêm bằng HasAnyPermStrict). 3 route bố cục CỦA CHÍNH user
// (`layout` GET/PUT/DELETE) dùng tập quyền module CRM như các khối không phải tiền.
var crmDashboardBrandDefaultSuffixes = map[string]bool{
	"layout/brand-default": true,
}

// crmModulePermSet parse CSV thành set (không phụ thuộc thứ tự, trim khoảng trắng).
func crmModulePermSet() map[string]bool {
	parts := strings.Split(crmModulePermCSV, ",")
	set := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		set[p] = true
	}
	return set
}

// permSetOf tách CSV của 1 entry.perm thành set — cùng cơ chế middleware.splitPerms (OR-gate).
func permSetOf(csv string) map[string]bool {
	parts := strings.Split(csv, ",")
	set := make(map[string]bool, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		set[p] = true
	}
	return set
}

// setEqual so 2 set như tập hợp — không quan tâm thứ tự phần tử.
func setEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// formatSet in set theo thứ tự ổn định (sort) để thông báo lỗi dễ đọc/dễ diff.
func formatSet(set map[string]bool) string {
	names := make([]string, 0, len(set))
	for k := range set {
		names = append(names, k)
	}
	sort.Strings(names)
	return "{" + strings.Join(names, ",") + "}"
}

// crmModuleFixItHint — nhắc đủ 3 nơi phải sửa đồng thời khi tập quyền lệch, tránh vá 1 nơi mà
// mobile/dashboard/FE menu trôi khỏi nhau (đúng bối cảnh sự cố: sửa 1 entry quên entry khác =
// khoá/lộ dashboard im lặng, không lỗi, không cảnh báo).
const crmModuleFixItHint = "sửa ĐỒNG THỜI 3 nơi: " +
	"frontend/src/app/define/permission.define.ts (CRM_MODULE_PERMISSIONS, dùng bởi " +
	"system-module-menu.ts + utilities/home/home.ts), " +
	"backend/internal/modules/webapp/auth/rbac/seed/route_perm_map.json, " +
	"mobile/packages/core_domain/lib/src/permissions.dart (kPermsCrmModule)"

// TestCrmDashboardModulePermSet — FAIL CỨNG nếu bất kỳ entry GET /api/crm/crm-dashboard/*
// (kind=perm) mang tập quyền KHÁC đúng 7 quyền module CRM đã chốt (crmModuleFixItHint).
//
// Vì sao: dashboard CRM (web + mobile) gate bằng "quyền module CRM" = OR đúng 7 quyền raw. Không
// có lưới nào giữ 41 entry (21 web + 20 mobile) đồng bộ trước test này — sửa 1 entry (vd đổi tên 1
// quyền, thêm/bớt 1 quyền) mà quên entry khác thì dashboard bị khoá (thiếu quyền hợp lệ) hoặc lộ
// (thừa quyền lỏng hơn) cho một nửa mặt trận (web hoặc mobile), IM LẶNG — build xanh, test cũ xanh.
func TestCrmDashboardModulePermSet(t *testing.T) {
	entries, err := LoadRoutePermMap()
	if err != nil {
		t.Fatalf("LoadRoutePermMap: %v", err)
	}
	moduleSet := crmModulePermSet()
	moneySet := permSetOf(crmDashboardMoneyPermCSV)

	brandSet := permSetOf("manageConfig")

	matched, moneyMatched, brandMatched := 0, 0, 0
	for _, e := range entries {
		if e.Kind != KindPerm || !strings.HasPrefix(e.Path, crmDashboardPathPrefix) {
			continue
		}
		matched++
		want := moduleSet
		suffix := strings.TrimPrefix(e.Path, crmDashboardPathPrefix)
		if crmDashboardMoneySuffixes[suffix] {
			want = moneySet
			moneyMatched++
		}
		if crmDashboardBrandDefaultSuffixes[suffix] {
			want = brandSet
			brandMatched++
		}
		got := permSetOf(e.Perm)
		if !setEqual(got, want) {
			t.Errorf("%s %s: tập quyền THỰC TẾ %s KHÁC tập KỲ VỌNG %s (module CRM dashboard) — %s",
				e.Method, e.Path, formatSet(got), formatSet(want), crmModuleFixItHint)
		}
	}

	// Sàn chống xanh giả: nếu path prefix đổi (route map đổi cấu trúc, vd đổi tên
	// "crm-dashboard" → tên khác) thì matched tụt về 0 và test PASS-giả (vòng for không chạy gì).
	// Khoá đúng số đếm hiện tại (17, story-16 be-rbac-trim: 21 → 19 — 2 entry
	// estimate-price-marketing/marketing-view đã mồ côi, route handler bị xoá ở story trước,
	// route_perm_map.json dọn theo permission_coverage_test.go 0-drift; crm-standalone 19 → 18 —
	// `GET /api/crm/crm-dashboard/overview-chart-event` (biểu đồ chiến dịch, domain bỏ) đã xoá khỏi
	// route_perm_map.json, ghi nhận ở story-35; 18 → 17 (2026-10-17) — `GET .../clv-overview` xoá
	// cùng tính năng CLV); 17 → 20 (2026-10-19, dashboard tuỳ biến) — bỏ customer-status-marketing +
	// overview-online, thêm 5 route bố cục (layout GET/PUT/DELETE + layout/brand-default PUT/DELETE;
	// scan này đếm MỌI method dưới prefix); 20 → 19 (2026-11-01) — bỏ khối "Doanh thu trong ngày"
	// (`payment-today`, trùng bộ lọc ngày của payment-in-month) — lệch số (kể cả THÊM/BỚT
	// entry hợp lệ) là tín hiệu phải rà tay xem entry mới/bị xoá có cần mirror ở mobile hay không,
	// không phải lỗi test.
	const wantDashboardEntryCount = 19
	// Sàn cho nhánh tiền: đổi tên/xoá 1 endpoint tiền mà quên map ⇒ entry đó rơi về tập module (lỏng)
	// mà test vẫn xanh — đếm đúng số entry tiền đã khớp.
	if moneyMatched != len(crmDashboardMoneySuffixes) {
		t.Errorf("số entry dashboard TRẢ TIỀN khớp = %d, kỳ vọng %d (%v) — R-5", moneyMatched,
			len(crmDashboardMoneySuffixes), crmDashboardMoneySuffixes)
	}
	if brandMatched != 2 {
		t.Errorf("số entry layout/brand-default (PUT+DELETE) khớp = %d, kỳ vọng 2", brandMatched)
	}
	if matched != wantDashboardEntryCount {
		t.Errorf("số entry GET %s* (kind=perm) = %d, kỳ vọng %d — route_perm_map.json vừa đổi cấu trúc "+
			"(thêm/bớt endpoint dashboard, hoặc đổi path prefix khiến scan này không còn khớp gì). "+
			"Rà tay: entry mới có cần cùng tập 7 quyền + mirror bên mobile không? %s",
			crmDashboardPathPrefix, matched, wantDashboardEntryCount, crmModuleFixItHint)
	}
}

// dashboardSuffixSet trả map suffix (phần path SAU prefix) → entry, cho các entry kind=perm khớp
// prefix đã cho. Dùng để đối chiếu web↔mobile theo TÊN endpoint, không theo thứ tự trong file.
func dashboardSuffixSet(entries []RoutePermEntry, prefix string) map[string]RoutePermEntry {
	out := make(map[string]RoutePermEntry)
	for _, e := range entries {
		if e.Kind != KindPerm || !strings.HasPrefix(e.Path, prefix) {
			continue
		}
		out[strings.TrimPrefix(e.Path, prefix)] = e
	}
	return out
}

// TestCrmDashboardMobileParity — FAIL CỨNG nếu 1 cặp endpoint web↔mobile CÙNG TÊN
// (/api/crm/crm-dashboard/<x> và /api/mobile/v1/crm/<x>) mang 2 tập quyền KHÁC NHAU.
//
// Phạm vi: CHỈ các entry mobile CÓ đối ứng crm-dashboard cùng tên (mobile là bản delegate cùng
// endpoint của crm-dashboard — xem backend-hard-rules §"Đổi kiểu field handler webapp/* có
// delegate mobile"). Entry mobile KHÔNG đối ứng dashboard (nếu có, vd customer/…) KHÔNG thuộc
// phạm vi module CRM dashboard này — không assert, tự liệt kê ở TestCrmMobileCoverageSurvey.
func TestCrmDashboardMobileParity(t *testing.T) {
	entries, err := LoadRoutePermMap()
	if err != nil {
		t.Fatalf("LoadRoutePermMap: %v", err)
	}
	dash := dashboardSuffixSet(entries, crmDashboardPathPrefix)
	mobile := dashboardSuffixSet(entries, crmMobilePathPrefix)

	compared := 0
	suffixes := make([]string, 0, len(mobile))
	for suffix := range mobile {
		suffixes = append(suffixes, suffix)
	}
	sort.Strings(suffixes)

	for _, suffix := range suffixes {
		mobileEntry := mobile[suffix]
		dashEntry, ok := dash[suffix]
		if !ok {
			continue // không đối ứng dashboard → ngoài phạm vi test này
		}
		compared++
		wantSet := permSetOf(dashEntry.Perm)
		gotSet := permSetOf(mobileEntry.Perm)
		if !setEqual(gotSet, wantSet) {
			t.Errorf("%s %s: tập quyền %s KHÁC tập của đối ứng web %s %s = %s — %s",
				mobileEntry.Method, mobileEntry.Path, formatSet(gotSet),
				dashEntry.Method, dashEntry.Path, formatSet(wantSet), crmModuleFixItHint)
		}
	}

	// story-16 (be-rbac-trim, 2026-09-24): module `mobile` (BFF app staff) đã xoá HẲN — 0 route
	// /api/mobile/v1/* còn mount (xác nhận bằng permission_coverage_test.go realRoutes()) — 20 entry
	// /api/mobile/v1/crm/* trong route_perm_map.json là mồ côi 100%, đã trim cùng đợt cùng 1571
	// entry orphan khác. compared==0 giờ là trạng thái ĐÚNG (không phải xanh giả) vì mobile[...] rỗng
	// từ nguồn — khác tình huống gốc (path prefix đổi mà mobile vẫn tồn tại thật).
	const wantPairCount = 0
	if compared != wantPairCount {
		t.Errorf("số cặp web↔mobile được so = %d, kỳ vọng %d — route_perm_map.json vừa đổi cấu trúc "+
			"(thêm/bớt endpoint mobile/crm, đổi path prefix, hoặc 1 entry mobile mất đối ứng dashboard). %s",
			compared, wantPairCount, crmModuleFixItHint)
	}
}

// TestCrmMobileCoverageSurvey — liệt kê (KHÔNG assert nghiệp vụ) 20 entry GET /api/mobile/v1/crm/*
// hiện có: entry nào đối ứng crm-dashboard cùng tên (thuộc phạm vi TestCrmDashboardMobileParity),
// entry nào KHÔNG (ngoài phạm vi 2 test trên). Ghi lại bằng chứng khảo sát yêu cầu trong story —
// FAIL nếu tổng số trôi khỏi 20 (đổi cấu trúc), để không âm thầm bỏ sót entry mới khi rà tay.
func TestCrmMobileCoverageSurvey(t *testing.T) {
	entries, err := LoadRoutePermMap()
	if err != nil {
		t.Fatalf("LoadRoutePermMap: %v", err)
	}
	dash := dashboardSuffixSet(entries, crmDashboardPathPrefix)
	mobile := dashboardSuffixSet(entries, crmMobilePathPrefix)

	suffixes := make([]string, 0, len(mobile))
	for suffix := range mobile {
		suffixes = append(suffixes, suffix)
	}
	sort.Strings(suffixes)

	withDash := 0
	withoutDash := 0
	for _, suffix := range suffixes {
		if _, ok := dash[suffix]; ok {
			withDash++
			t.Logf("ĐỐI ỨNG dashboard: %s%s <-> %s%s", crmMobilePathPrefix, suffix, crmDashboardPathPrefix, suffix)
		} else {
			withoutDash++
			t.Logf("KHÔNG đối ứng dashboard (ngoài phạm vi): %s%s", crmMobilePathPrefix, suffix)
		}
	}

	// story-16: module mobile đã xoá hẳn, 0 entry /api/mobile/v1/crm/* còn lại trong route_perm_map.
	const wantMobileEntryCount = 0
	if len(mobile) != wantMobileEntryCount {
		t.Errorf("tổng số entry GET %s* (kind=perm) = %d, kỳ vọng %d — route_perm_map.json vừa đổi cấu trúc, "+
			"rà tay lại danh sách đối ứng/không đối ứng dashboard", crmMobilePathPrefix, len(mobile), wantMobileEntryCount)
	}
	t.Logf("Tổng kết: %d/%d entry mobile/crm có đối ứng dashboard, %d không đối ứng (ngoài phạm vi)",
		withDash, len(mobile), withoutDash)
}

// TestCrmModulePermsExistInCatalog — mọi quyền trong tập 7 quyền module CRM phải tồn tại trong
// catalog artifact permissions.gen.json (cùng cơ chế TestRoutePermMapPermsInCatalog, nhưng khoá
// riêng đúng 7 quyền của module CRM — không phụ thuộc catalog có đổi cấu trúc khác đi ở nơi khác).
func TestCrmModulePermsExistInCatalog(t *testing.T) {
	known := catalogNames(t)
	want := crmModulePermSet()

	names := make([]string, 0, len(want))
	for p := range want {
		names = append(names, p)
	}
	sort.Strings(names)

	for _, p := range names {
		if !known[p] {
			t.Errorf("quyền %q (tập module CRM dashboard) KHÔNG có trong permissions.gen.json — "+
				"khai key ở frontend/src/app/define/permission.define.ts rồi chạy `node scripts/perm-codegen.mjs`. %s",
				p, crmModuleFixItHint)
		}
	}
}
