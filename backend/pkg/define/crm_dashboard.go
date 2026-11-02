package define

// Dashboard CRM tuỳ biến (port từ ERP epic crm-dashboard-customizable).
const (
	// CrmDashboardRangeMaxDays — trần độ dài bộ lọc khoảng ngày (tối đa 1 năm).
	CrmDashboardRangeMaxDays = 366

	// CrmDashboardKey — giá trị `dashboard_layout.dashboard_key` của dashboard CRM.
	CrmDashboardKey = "crm"
	// CrmDashboardLayoutVersion — phiên bản schema `dashboard_layout.config` (FE `DashboardLayoutConfig`).
	CrmDashboardLayoutVersion  = 1
	CrmDashboardLayoutMaxItems = 50
	CrmDashboardLayoutMaxBytes = 16 << 10

	CrmDashboardZoneTop  = "top"
	CrmDashboardZoneMain = "main"
	CrmDashboardZoneSide = "side"
	CrmDashboardSizeHalf = "half"
	CrmDashboardSizeFull = "full"

	// Nguồn bố cục hiệu lực trả về GET /crm/crm-dashboard/layout. 1 DB = 1 thương hiệu ⇒ "brand" thay
	// "company" của ERP.
	CrmDashboardLayoutSourceUser   = "user"
	CrmDashboardLayoutSourceBrand  = "brand"
	CrmDashboardLayoutSourceSystem = "system"
)

// CrmDashboardWidgetKeys — 13 khối dashboard CRM (khớp registry FE). Key ngoài danh sách bị từ chối khi GHI
// bố cục; FE tự bỏ qua key lạ khi ĐỌC. So với ERP (20 khối) KHÔNG có: clv_overview, marketing_view,
// estimate_price_marketing, event_overview (CRM standalone không có nguồn dữ liệu), customer_status_marketing,
// convert_online (user chốt 2026-10-19 bỏ 2 khối marketing), payment_today (user chốt 2026-11-01: trùng với bộ lọc ngày
// của payment_in_month).
var CrmDashboardWidgetKeys = map[string]bool{
	"payment_in_month": true, "quick_stats": true, "appointment_accepted_stats": true,
	"payment_branch_overview": true, "service_statistics": true, "source_overview": true,
	"service_type_accepted": true, "service_type_interest": true,
	"overview_appointment": true, "additional_stats": true, "appointment_status": true,
	"customer_status_overview": true, "customer_region": true,
}
