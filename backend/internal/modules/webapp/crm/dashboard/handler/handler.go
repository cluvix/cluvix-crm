package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"erpCluvix/internal/middleware"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/tz"

	"erpCluvix/pkg/cache"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type Deps struct {
	DB    *gorm.DB
	Cache *cache.RedisCache
}

type Handler struct{ Deps }

func New(d Deps) *Handler { return &Handler{Deps: d} }

// buildDashboardScope trả SQL fragment scope branch cho raw SQL aggregate trên tenant
// tables. Alias = table name nếu không alias trong query.
// Record branch_id IS NULL luôn được lấy (record chưa gán / áp dụng toàn tenant).
// Story-38: bỏ vế company (1 thương hiệu/DB — không cần lọc); GIỮ NGUYÊN hành vi branch.
func buildDashboardScope(c *gin.Context, alias string) (string, []any) {
	branchActiveID := middleware.GetBranchActiveID(c)
	branchIDs := middleware.GetBranchIDs(c)

	frag := ""
	args := make([]any, 0)

	// Branch scope
	if branchActiveID > 0 {
		frag += fmt.Sprintf(" AND (%s.branch_id = ? OR %s.branch_id IS NULL)", alias, alias)
		args = append(args, branchActiveID)
	} else if len(branchIDs) > 0 {
		frag += fmt.Sprintf(" AND (%s.branch_id IN ? OR %s.branch_id IS NULL)", alias, alias)
		args = append(args, branchIDs)
	}

	return frag, args
}

// buildDashboardScopeOpt is like buildDashboardScope but accepts explicit branchID override.
// When branchID > 0, scopes to that branch instead of the middleware's active branch.
// Mọi endpoint data dashboard nhận branch_id qua parseDashboardFilter (filter.go) — đã kiểm quyền ở
// dashboardBranchParam trước khi tới đây.
func buildDashboardScopeOpt(c *gin.Context, alias string, branchID int) (string, []any) {
	if branchID <= 0 {
		return buildDashboardScope(c, alias)
	}
	frag := fmt.Sprintf(" AND (%s.branch_id = ? OR %s.branch_id IS NULL)", alias, alias)
	args := []any{branchID}
	return frag, args
}

// dashboardBranchParam đọc `branch_id` tuỳ chọn từ query (0 = không chọn ⇒ dùng scope phiên).
// ok=false khi chi nhánh được chọn KHÔNG thuộc quyền của user (M-15): buildDashboardScopeOpt thay
// HẲN scope phiên bằng branchID này, nên thiếu kiểm ở đây là xem được số liệu chi nhánh khác.
func dashboardBranchParam(c *gin.Context) (int, bool) {
	b, err := strconv.Atoi(c.Query("branch_id"))
	if err != nil || b <= 0 {
		return 0, true
	}
	if !middleware.IsBranchAllowed(c, uint(b)) {
		return 0, false
	}
	return b, true
}

// ── CRM Dashboard ──

// CrmPaymentInMonth GET /dashboard/crm/payment-in-month — cached 300s
func (h *Handler) CrmPaymentInMonth(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	mFrom, mTo := tz.MonthRange(loc, 0)
	f, ok := h.parseDashboardFilter(c, loc, mFrom, mTo+1)
	if !ok {
		return
	}
	// Các helper tiền dùng `BETWEEN ? AND ?` ⇒ mốc cuối BAO GỒM.
	from, to := f.From, f.ToIncl()
	branchID := f.BranchID

	cacheKey := dashCacheKey(c, "crm_payment_in_month", f)
	if h.Cache != nil {
		var cached gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// Aggregator: mỗi query độc lập — return NGAY khi query đầu tiên lỗi, không chạy tiếp query sau.
	doanhSo, err := h.queryDoanhSo(c, from, to, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}
	doanhThu, err := h.queryDoanhThu(c, from, to, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}
	thucThu, err := h.queryThucThu(c, from, to, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}
	hoanDV, err := h.queryHoanDichVu(c, from, to, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}
	hoanCoc, err := h.queryHoanCoc(c, from, to, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}
	congNo, err := h.queryCongNo(c, from, to, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}

	// So sánh kỳ liền trước: kỳ = đúng 1 tháng dương lịch ⇒ tháng trước (hành vi cũ); khác ⇒ cùng độ dài.
	lastFrom, lastToExcl := lastPeriod(f.From, f.To, loc)
	fromLast, toLast := lastFrom, lastToExcl-1

	doanhSoLast, err := h.queryDoanhSo(c, fromLast, toLast, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}
	thucThuLast, err := h.queryThucThu(c, fromLast, toLast, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}

	doanhSoTyLe, chieuDoanhSo := calcTrend(doanhSo, doanhSoLast)
	thucThuTyLe, chieuThucThu := calcTrend(thucThu, thucThuLast)

	result := gin.H{
		"doanhSo": gin.H{
			"hienTai":  doanhSo,
			"flag":     true,
			"tyLe":     doanhSoTyLe,
			"tangGiam": chieuDoanhSo,
		},
		"doanhThu": doanhThu,
		"thucThu": gin.H{
			"hienTai":  thucThu,
			"flag":     true,
			"tyLe":     thucThuTyLe,
			"tangGiam": chieuThucThu,
		},
		"hoanTra": hoanDV.Add(hoanCoc),
		"congNo":  congNo,
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 300*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// CrmPaymentToday GET /dashboard/crm/payment-today
func (h *Handler) CrmPaymentToday(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	sod := tz.StartOfDay(loc)
	f, ok := h.parseDashboardFilter(c, loc, sod, sod+86400)
	if !ok {
		return
	}
	// Khối theo NGÀY: tính trên ngày tham chiếu (= hôm nay khi không gửi kỳ). Giữ nguyên cặp
	// (đầu ngày, đầu ngày + 86400) code cũ truyền vào `BETWEEN` để kết quả mặc định không đổi.
	from := f.RefDay
	to := from + 86400
	branchID := f.BranchID

	cacheKey := dashCacheKey(c, "crm_payment_today", f)
	if h.Cache != nil {
		var cached gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// Aggregator: mỗi query độc lập — return NGAY khi query đầu tiên lỗi, không chạy tiếp query sau.
	doanhSo, err := h.queryDoanhSo(c, from, to, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}
	doanhThu, err := h.queryDoanhThu(c, from, to, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}
	thucThu, err := h.queryThucThu(c, from, to, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}
	congNo, err := h.queryCongNo(c, from, to, branchID)
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return
	}

	result := gin.H{
		"doanhSo":  doanhSo,
		"doanhThu": doanhThu,
		"thucThu":  thucThu,
		"congNo":   congNo,
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 300*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// CrmQuickStarts GET /dashboard/crm/quick-starts
func (h *Handler) CrmQuickStarts(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	f, ok := h.parseDashboardFilter(c, loc, tz.StartOfMonth(loc, 0), tz.StartOfMonth(loc, 1))
	if !ok {
		return
	}
	// Khối lai: "trong tháng" = kỳ [From, To); "hôm nay" = ngày tham chiếu.
	som, eom := f.From, f.To
	sod := f.RefDay
	eod := sod + 86400

	cacheKey := dashCacheKey(c, "crm_quick_starts", f)
	if h.Cache != nil {
		var cached gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// Tenant scope fragments cho mỗi table
	custScope, custArgs := buildDashboardScopeOpt(c, "customer", f.BranchID)
	apptScope, apptArgs := buildDashboardScopeOpt(c, "a", f.BranchID)
	otpScope, otpArgs := buildDashboardScopeOpt(c, "order_plan", f.BranchID)
	osuScope, osuArgs := buildDashboardScopeOpt(c, "order_service_used", f.BranchID)

	var cust struct {
		Month int64 `gorm:"column:customer_month"`
		Today int64 `gorm:"column:customer_today"`
	}
	// WHERE phải phủ cả 2 cửa sổ: ngày tham chiếu có thể nằm ngoài kỳ (kỳ ở tương lai ⇒ RefDay = hôm nay).
	custLo, custHi := min(som, sod), max(eom, eod)
	custAllArgs := append([]any{som, eom, sod, eod, custLo, custHi}, custArgs...)
	tx := h.DB.Raw(`SELECT
		COALESCE(SUM(CASE WHEN created_day >= ? AND created_day < ? THEN 1 ELSE 0 END), 0) AS customer_month,
		COALESCE(SUM(CASE WHEN created_day >= ? AND created_day < ? THEN 1 ELSE 0 END), 0) AS customer_today
		FROM customer WHERE created_day >= ? AND created_day < ?`+custScope, custAllArgs...).Scan(&cust)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	// Hai chỉ số dưới đây từng nằm chung MỘT câu, với điều kiện ngày CHỈ đặt trong CASE WHEN —
	// `WHERE` không có ràng buộc ngày nào nên MySQL phải quét TOÀN BỘ lịch hẹn của công ty
	// (quá khứ lẫn tương lai) để đếm số của đúng một ngày. Tách làm 2 câu, mỗi câu tự chặn dải
	// ngày trên cột của chính nó ⇒ mỗi câu ăn được chỉ mục.
	// Đo @1.090.355 lịch hẹn: 235.732 dòng InnoDB / 509ms → 7.261 dòng / 2,6ms (1 chi nhánh);
	// 71.737 / 221ms → 21.755 / 8,7ms (17 chi nhánh). Giá trị trả về khớp tuyệt đối cả 2 ca.
	// ⚠ KHÔNG gộp lại bằng cách chỉ thêm `AND (created_at BETWEEN ... OR appointment_date
	// BETWEEN ...)` vào WHERE: đã đo, số dòng đọc KHÔNG đổi (235.732) vì OR trên hai cột khác
	// nhau không dùng được một dải chỉ mục.
	var apptToday int64
	apptTodayArgs := append([]any{sod, eod}, apptArgs...)
	tx = h.DB.Raw(`SELECT COUNT(*)
		FROM appointment a
		INNER JOIN appointment_type t ON a.appointment_type_id = t.id
		WHERE t.isTypeRevenue = 1 AND a.created_at >= ? AND a.created_at < ?`+apptScope,
		apptTodayArgs...).Scan(&apptToday)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	var apptComing int64
	apptComingArgs := append([]any{sod, eod}, apptArgs...)
	tx = h.DB.Raw(`SELECT COUNT(*)
		FROM appointment a
		INNER JOIN appointment_type t ON a.appointment_type_id = t.id
		WHERE t.isTypeRevenue = 1 AND a.appointment_date >= ? AND a.appointment_date < ?`+apptScope,
		apptComingArgs...).Scan(&apptComing)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	var orderNew int64
	otpAllArgs := append([]any{sod, eod}, otpArgs...)
	tx = h.DB.Raw(`SELECT COUNT(*) FROM order_plan WHERE created_time >= ? AND created_time < ?`+otpScope, otpAllArgs...).Scan(&orderNew)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	var serviceNew int64
	osuAllArgs := append([]any{sod, eod}, osuArgs...)
	tx = h.DB.Raw(`SELECT COUNT(*) FROM order_service_used WHERE created_day >= ? AND created_day < ?`+osuScope, osuAllArgs...).Scan(&serviceNew)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	result := gin.H{
		"customerMonth":     cust.Month,
		"customerToday":     cust.Today,
		"appointmentToday":  apptToday,
		"appointmentComing": apptComing,
		"orderNew":          orderNew,
		"serviceNew":        serviceNew,
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 300*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// CrmAppointmentAcceptedStats GET /dashboard/crm/appointment-accepted-stats
// Optional params: from, to (unix), branch_id — default: today, active branch scope.
func (h *Handler) CrmAppointmentAcceptedStats(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	sod := tz.StartOfDay(loc)
	// Khối theo NGÀY: tính trên ngày tham chiếu của kỳ — hôm nay khi không gửi kỳ. from/to là KỲ, không còn
	// là cửa sổ trực tiếp (như ERP). branch_id qua parseDashboardFilter ⇒ dashboardBranchParam kiểm quyền.
	f, ok := h.parseDashboardFilter(c, loc, sod, sod+86400)
	if !ok {
		return
	}
	// Giữ cặp (đầu ngày, đầu ngày + 86400) code cũ truyền vào `BETWEEN` ⇒ mặc định ra y như trước.
	from, to := f.RefDay, f.RefDay+86400
	branchID := f.BranchID

	cacheKey := dashCacheKey(c, "crm_appt_accepted_stats", f)
	if h.Cache != nil {
		var cached []gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	var statuses []struct {
		ID   int    `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	tx := h.DB.Raw(`SELECT id, name FROM appointment_is_accepted WHERE status = 1 ORDER BY id`).Scan(&statuses)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	var actual []struct {
		IsAccepted int   `gorm:"column:is_accepted"`
		Value      int64 `gorm:"column:value"`
	}
	apptScope, apptArgs := buildDashboardScopeOpt(c, "appointment", branchID)
	allArgs := append([]any{from, to}, apptArgs...)
	tx = h.DB.Raw(`SELECT is_accepted, COUNT(*) AS value FROM appointment
		WHERE is_accepted IS NOT NULL AND appointment_date BETWEEN ? AND ?`+apptScope+`
		GROUP BY is_accepted`, allArgs...).Scan(&actual)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	actualMap := make(map[int]int64)
	for _, a := range actual {
		actualMap[a.IsAccepted] = a.Value
	}

	result := make([]gin.H, 0, len(statuses))
	for _, s := range statuses {
		result = append(result, gin.H{
			"name":  s.Name,
			"value": actualMap[s.ID],
		})
	}

	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 60*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// CrmOverviewAppointment GET /dashboard/crm/overview-appointment
func (h *Handler) CrmOverviewAppointment(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	f, ok := h.parseDashboardFilter(c, loc, tz.StartOfMonth(loc, 0), tz.StartOfMonth(loc, 1))
	if !ok {
		return
	}
	cacheKey := dashCacheKey(c, "crm_overview_appointment", f)
	if h.Cache != nil {
		var cached gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// Khối lai: kỳ [From, To) + ngày tham chiếu. Giữ đúng các mốc code cũ truyền vào `BETWEEN`
	// (som, ngày mai, đầu tháng sau) ⇒ mặc định ra y như trước.
	som := f.From
	tomorrow := f.RefDay + 86400
	nextMonth := f.To

	var row struct {
		LichHen          int64 `gorm:"column:lich_hen"`
		DaDen            int64 `gorm:"column:da_den"`
		DongY            int64 `gorm:"column:dong_y"`
		Huy              int64 `gorm:"column:huy"`
		ConLaiTrongThang int64 `gorm:"column:con_lai_trong_thang"`
		ConLai           int64 `gorm:"column:con_lai"`
	}
	// Single scan thay vì 6 subquery riêng: COUNT(DISTINCT CASE WHEN ...) cho mỗi metric.
	// sArgs chỉ cần inject 1 lần vào outer WHERE.
	s, sArgs := buildDashboardScopeOpt(c, "a", f.BranchID)
	isAcceptedIDs := h.getIsAcceptedIDs(c)
	args := []any{
		som, tomorrow, // lich_hen BETWEEN
		som, tomorrow, define.AppointmentStatusCheckedIn, define.AppointmentStatusCheckedOut, // da_den BETWEEN + status
		som, tomorrow, define.AppointmentStatusCheckedIn, define.AppointmentStatusCheckedOut, isAcceptedIDs, // dong_y BETWEEN + status + accepted
		som, tomorrow, define.AppointmentStatusCancel, // huy BETWEEN + status
		tomorrow, nextMonth, // con_lai_trong_thang BETWEEN
		tomorrow, // con_lai >
		som,      // outer WHERE appointment_date >=
	}
	args = append(args, sArgs...)

	tx := h.DB.Raw(`SELECT
		COUNT(DISTINCT CASE WHEN a.appointment_date BETWEEN ? AND ? THEN a.customer_id END) AS lich_hen,
		COUNT(DISTINCT CASE WHEN a.appointment_date BETWEEN ? AND ? AND a.status IN (?,?) THEN a.customer_id END) AS da_den,
		COUNT(DISTINCT CASE WHEN a.appointment_date BETWEEN ? AND ? AND a.status IN (?,?) AND a.is_accepted IN ? THEN a.customer_id END) AS dong_y,
		COUNT(DISTINCT CASE WHEN a.appointment_date BETWEEN ? AND ? AND a.status=? THEN a.customer_id END) AS huy,
		COUNT(DISTINCT CASE WHEN a.appointment_date BETWEEN ? AND ? THEN a.customer_id END) AS con_lai_trong_thang,
		COUNT(DISTINCT CASE WHEN a.appointment_date > ? THEN a.customer_id END) AS con_lai
	FROM appointment a
	INNER JOIN appointment_type t ON a.appointment_type_id = t.id
	WHERE t.isTypeRevenue = 1 AND a.appointment_date >= ?`+s,
		args...).Scan(&row)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	result := gin.H{
		"lichHen":          row.LichHen,
		"daDen":            row.DaDen,
		"dongY":            row.DongY,
		"huy":              row.Huy,
		"conLaiTrongThang": row.ConLaiTrongThang,
		"conLai":           row.ConLai,
	}

	if h.Cache != nil {
		// Mặc định: sống tới đầu tháng sau như cũ (khoá đã đổi theo ngày tham chiếu). Kỳ tường minh: 300s.
		ttl := time.Duration(nextMonth-time.Now().Unix()) * time.Second
		if f.Explicit {
			ttl = 300 * time.Second
		}
		if ttl > 0 {
			h.Cache.Set(cacheKey, result, ttl)
		}
	}
	c.JSON(http.StatusOK, result)
}

// CrmReportStatistics GET /dashboard/crm/report-statistics — cached 300s
func (h *Handler) CrmReportStatistics(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	mFrom, mTo := tz.MonthRange(loc, 0)
	f, ok := h.parseDashboardFilter(c, loc, mFrom, mTo+1)
	if !ok {
		return
	}
	// Câu dùng `BETWEEN ? AND ?` ⇒ mốc cuối BAO GỒM.
	from, to := f.From, f.ToIncl()

	cacheKey := dashCacheKey(c, "crm_report_statistics", f)
	if h.Cache != nil {
		var cached []gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	var rows []struct {
		Name  string `gorm:"column:name"`
		Value int64  `gorm:"column:value"`
	}
	sA, sAArgs := buildDashboardScopeOpt(c, "a", f.BranchID)
	rsArgs := append([]any{from, to}, sAArgs...)
	tx := h.DB.Raw(`SELECT aia.name, COUNT(*) AS value
		FROM appointment a
		INNER JOIN appointment_is_accepted aia ON a.is_accepted = aia.id
		INNER JOIN appointment_type t ON a.appointment_type_id = t.id
		WHERE t.isTypeRevenue = 1 AND aia.status = 1
		AND a.appointment_date BETWEEN ? AND ?`+sA+`
		GROUP BY a.is_accepted
		ORDER BY value DESC`, rsArgs...).Scan(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	if len(rows) == 0 {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	result := make([]gin.H, 0)
	var othersValue int64
	for i, r := range rows {
		if i < 5 {
			name := r.Name
			if name == "" {
				name = "Chưa xác định"
			}
			result = append(result, gin.H{"name": name, "value": r.Value})
		} else {
			othersValue += r.Value
		}
	}
	if othersValue > 0 {
		result = append(result, gin.H{"name": "Khác", "value": othersValue})
	}

	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 300*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// CrmAdditionalStats GET /dashboard/crm/additional-stats
func (h *Handler) CrmAdditionalStats(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	f, ok := h.parseDashboardFilter(c, loc, tz.StartOfMonth(loc, 0), tz.StartOfMonth(loc, 1))
	if !ok {
		return
	}
	// Chỉ còn phần "hôm nay" (nhắc việc) = ngày tham chiếu của kỳ. reminder không có branch_id ⇒ chọn chi
	// nhánh KHÔNG lọc được khối này (vẫn kiểm quyền branch_id + vào khoá cache cho nhất quán).
	sod := f.RefDay
	eod := sod + 86400

	cacheKey := dashCacheKey(c, "crm_additional_stats", f)
	if h.Cache != nil {
		var cached gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	var row struct {
		ReminderTotal int64 `gorm:"column:reminder_total"`
		ReminderDone  int64 `gorm:"column:reminder_done"`
	}
	// reminder không có branch_id (đã audit 2026-07-28) — trước story-38 chỉ lọc theo company;
	// bỏ company (D15, 1 thương hiệu/DB) ⇒ reminder không còn vế scope nào (đếm toàn DB, đúng
	// với hành vi crm/reminder module đã bỏ lọc company từ story-36).
	// story-10 crm-standalone: bỏ 2 chỉ số CTV mới (brokerNew) + mã giảm giá đã dùng (couponUsed).
	// story-11 crm-standalone: bỏ 2 chỉ số chăm sóc khách (cskhTotal, cskhDone) — module chăm sóc đã xoá.
	// story-13 crm-standalone: bỏ customerInfoTotal/customerInfoDone (bảng inbox lead đã xoá).
	statArgs := []any{sod, eod, sod, eod}

	tx := h.DB.Raw(`SELECT
		(SELECT COUNT(*) FROM reminder WHERE start_time >= ? AND start_time < ?) AS reminder_total,
		(SELECT COUNT(*) FROM reminder WHERE start_time >= ? AND start_time < ? AND status = 2) AS reminder_done`,
		statArgs...).Scan(&row)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	additionalResult := gin.H{
		"reminderTotal": row.ReminderTotal,
		"reminderDone":  row.ReminderDone,
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, additionalResult, 60*time.Second)
	}
	c.JSON(http.StatusOK, additionalResult)
}

// CrmCompareAppointment GET /dashboard/crm/compare-appointment — cached 300s
func (h *Handler) CrmCompareAppointment(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	som := tz.StartOfMonth(loc, 0)
	nextMonth := tz.StartOfMonth(loc, 1)
	lastMonth := tz.StartOfMonth(loc, -1)

	cacheKey := fmt.Sprintf("crm_compare_appointment_%s_%d", tenantCacheKey(c), som)
	if h.Cache != nil {
		var cached gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	type dayPoint struct {
		Day   int   `gorm:"column:d"`
		Total int64 `gorm:"column:total"`
	}

	// DAY(...) phải dùng datetime đã chuyển sang TZ company, không thì sẽ lệch ngày khi
	// session TZ != company TZ (vd hẹn 4-5 00:30 ICT lưu UTC 3-5 17:30 → DAY ra 3 thay vì 4).
	dayExpr := "DAY(" + tz.SqlFromUnix("a.appointment_date", tz.Offset(loc)) + ")"

	sA, sAArgs := buildDashboardScope(c, "a")
	var thisRows []dayPoint
	thisArgs := append([]any{som, nextMonth}, sAArgs...)
	tx := h.DB.Raw(`SELECT `+dayExpr+` AS d, COUNT(*) AS total
		FROM appointment a
		INNER JOIN appointment_type t ON a.appointment_type_id=t.id
		WHERE t.isTypeRevenue=1 AND a.appointment_date BETWEEN ? AND ?`+sA+`
		GROUP BY d ORDER BY d DESC`, thisArgs...).Scan(&thisRows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	var lastRows []dayPoint
	lastArgs := append([]any{lastMonth, som}, sAArgs...)
	tx = h.DB.Raw(`SELECT `+dayExpr+` AS d, COUNT(*) AS total
		FROM appointment a
		INNER JOIN appointment_type t ON a.appointment_type_id=t.id
		WHERE t.isTypeRevenue=1 AND a.appointment_date BETWEEN ? AND ?`+sA+`
		GROUP BY d ORDER BY d DESC`, lastArgs...).Scan(&lastRows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	toXY := func(rows []dayPoint) []gin.H {
		var total int64
		for _, r := range rows {
			total += r.Total
		}
		if total == 0 {
			return []gin.H{}
		}
		result := make([]gin.H, len(rows))
		for i, r := range rows {
			result[i] = gin.H{"x": r.Day, "y": r.Total}
		}
		return result
	}

	result := gin.H{
		"dataThisMonth": toXY(thisRows),
		"dataLastMonth": toXY(lastRows),
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 300*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// crmPaymentBranchOverviewRow — 1 dòng response của CrmPaymentBranchOverview.
// Dùng struct thay vì map[string]any: scan generic vào map làm cột DECIMAL/SUM
// rơi về Go string → JSON ra chuỗi có nháy ("1054000000.00"), vỡ client typed.
// decimal.Decimal + decimal.MarshalJSONWithoutQuotes (main.go) → JSON number.
type crmPaymentBranchOverviewRow struct {
	ID                     int64           `gorm:"column:id" json:"id"`
	Name                   string          `gorm:"column:name" json:"name"`
	TotalAccepted          int64           `gorm:"column:totalAccepted" json:"totalAccepted"`
	TotalOrderPlan         decimal.Decimal `gorm:"column:totalOrderPlan" json:"totalOrderPlan"`
	TotalRevenue           decimal.Decimal `gorm:"column:totalRevenue" json:"totalRevenue"`
	TotalPayment           decimal.Decimal `gorm:"column:totalPayment" json:"totalPayment"`
	TotalDeposit           decimal.Decimal `gorm:"column:totalDeposit" json:"totalDeposit"`
	TotalRefundDeposit     decimal.Decimal `gorm:"column:totalRefundDeposit" json:"totalRefundDeposit"`
	TotalRefundServiceUsed decimal.Decimal `gorm:"column:totalRefundServiceUsed" json:"totalRefundServiceUsed"`
}

// CrmPaymentBranchOverview GET /dashboard/crm/payment-branch-overview
// Optional params: from, to (unix), branch_id — default: current month, active branch scope.
func (h *Handler) CrmPaymentBranchOverview(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	mFrom, mTo := tz.MonthRange(loc, 0)
	// parseDashboardFilter ⇒ dashboardBranchParam kiểm branch_id thuộc quyền user (M-15).
	f, ok := h.parseDashboardFilter(c, loc, mFrom, mTo+1)
	if !ok {
		return
	}
	// Mọi câu con dùng `BETWEEN ? AND ?` ⇒ mốc cuối BAO GỒM.
	from, to := f.From, f.ToIncl()
	branchID := f.BranchID

	cacheKey := dashCacheKey(c, "crm_payment_branch_overview", f)
	if h.Cache != nil {
		var cached []crmPaymentBranchOverviewRow
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// Scope cho alias a (appointment), osu (order_service_used), p (payment). branch là bảng config/shared.
	sA, sAArgs := buildDashboardScopeOpt(c, "a", branchID)
	sOsu, sOsuArgs := buildDashboardScopeOpt(c, "osu", branchID)
	sP, sPArgs := buildDashboardScopeOpt(c, "p", branchID)
	isAcceptedIDs := h.getIsAcceptedIDs(c)

	var rows []crmPaymentBranchOverviewRow
	var args []any
	// A (appointment)
	args = append(args, from, to, define.AppointmentStatusCheckedIn, define.AppointmentStatusCheckedOut, isAcceptedIDs)
	args = append(args, sAArgs...)
	// B (order_service_used)
	args = append(args, from, to)
	args = append(args, sOsuArgs...)
	// C (payment + osu)
	args = append(args, define.PaymentForOrder, define.PaymentForMoveDeposit, from, to, from, to)
	args = append(args, sPArgs...)
	// D (payment)
	args = append(args, define.PaymentForOrder, define.PaymentForDeposit, define.PaymentTypeDepositKG, from, to)
	args = append(args, sPArgs...)
	// E (payment) — tiền cọc NHẬN trong kỳ (SUM amount), KHÔNG phải số dư deposit_after (M-19, user chốt 2026-10-18)
	args = append(args, define.PaymentForDeposit, define.PaymentTypeDepositKG, from, to)
	args = append(args, sPArgs...)
	// F (payment)
	args = append(args, define.PaymentForRefundDeposit, from, to)
	args = append(args, sPArgs...)
	// G (payment)
	args = append(args, define.PaymentForRefundService, from, to)
	args = append(args, sPArgs...)

	tx := h.DB.Raw(`SELECT b.id, b.name,
		COALESCE(A.totalAccepted, 0) AS totalAccepted,
		COALESCE(B.totalOrderPlan, 0) AS totalOrderPlan,
		COALESCE(C.totalRevenue, 0) AS totalRevenue,
		COALESCE(D.totalPayment, 0) AS totalPayment,
		COALESCE(E.totalDeposit, 0) AS totalDeposit,
		COALESCE(F.totalRefundDeposit, 0) AS totalRefundDeposit,
		COALESCE(G.totalRefundServiceUsed, 0) AS totalRefundServiceUsed
		FROM branch b
		LEFT JOIN (
			SELECT a.branch_id, COUNT(*) AS totalAccepted FROM appointment a
			WHERE a.branch_id IS NOT NULL AND a.appointment_date BETWEEN ? AND ?
			AND a.status IN (?,?) AND a.is_accepted IN ?`+sA+`
			GROUP BY a.branch_id
		) A ON b.id=A.branch_id
		LEFT JOIN (
			SELECT osu.branch_id, SUM(osu.totalEnd) AS totalOrderPlan FROM order_service_used osu
			WHERE osu.branch_id IS NOT NULL AND osu.created_day BETWEEN ? AND ?`+sOsu+`
			GROUP BY osu.branch_id
		) B ON b.id=B.branch_id
		LEFT JOIN (
			SELECT p.branch_id, SUM(p.amount) AS totalRevenue FROM payment p
			INNER JOIN order_service_used osu ON osu.id=p.serviceUsed_id
			WHERE p.branch_id IS NOT NULL AND p.payment_for IN (?,?)
			AND osu.created_day BETWEEN ? AND ? AND p.payment_of_date BETWEEN ? AND ?`+sP+`
			GROUP BY p.branch_id
		) C ON b.id=C.branch_id
		LEFT JOIN (
			SELECT p.branch_id, SUM(p.amount) AS totalPayment FROM payment p
			WHERE p.branch_id IS NOT NULL AND p.payment_for IN (?,?)
			AND (p.deposit_type IS NULL OR p.deposit_type NOT IN (?)) AND p.payment_time BETWEEN ? AND ?`+sP+`
			GROUP BY p.branch_id
		) D ON b.id=D.branch_id
		LEFT JOIN (
			SELECT p.branch_id, SUM(p.amount) AS totalDeposit FROM payment p
			WHERE p.branch_id IS NOT NULL AND p.payment_for=?
			AND (p.deposit_type IS NULL OR p.deposit_type NOT IN (?)) AND p.payment_time BETWEEN ? AND ?`+sP+`
			GROUP BY p.branch_id
		) E ON b.id=E.branch_id
		LEFT JOIN (
			SELECT p.branch_id, SUM(p.amount) AS totalRefundDeposit FROM payment p
			WHERE p.branch_id IS NOT NULL AND p.payment_for=? AND p.payment_time BETWEEN ? AND ?`+sP+`
			GROUP BY p.branch_id
		) F ON b.id=F.branch_id
		LEFT JOIN (
			SELECT p.branch_id, SUM(p.amount) AS totalRefundServiceUsed FROM payment p
			WHERE p.branch_id IS NOT NULL AND p.payment_for=? AND p.payment_time BETWEEN ? AND ?`+sP+`
			GROUP BY p.branch_id
		) G ON b.id=G.branch_id
		WHERE COALESCE(A.totalAccepted,0)!=0 OR COALESCE(B.totalOrderPlan,0)!=0
			OR COALESCE(C.totalRevenue,0)!=0 OR COALESCE(D.totalPayment,0)!=0
			OR COALESCE(E.totalDeposit,0)!=0 OR COALESCE(F.totalRefundDeposit,0)!=0
			OR COALESCE(G.totalRefundServiceUsed,0)!=0`, args...).Scan(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	if rows == nil {
		rows = []crmPaymentBranchOverviewRow{}
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, rows, 300*time.Second)
	}
	c.JSON(http.StatusOK, rows)
}

// CrmStatusCustomerOverview GET /dashboard/crm/status-customer-overview
func (h *Handler) CrmStatusCustomerOverview(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	mFrom, mTo := tz.MonthRange(loc, 0)
	f, ok := h.parseDashboardFilter(c, loc, mFrom, mTo+1)
	if !ok {
		return
	}
	// Câu dùng `BETWEEN ? AND ?` ⇒ mốc cuối BAO GỒM.
	from, to := f.From, f.ToIncl()

	cacheKey := dashCacheKey(c, "crm_status_customer_overview", f)
	if h.Cache != nil {
		var cached []gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	var row struct {
		TotalAll         int64 `gorm:"column:total_all"`
		TotalTicket      int64 `gorm:"column:total_ticket"`
		TotalLead        int64 `gorm:"column:total_lead"`
		TotalAppointment int64 `gorm:"column:total_appointment"`
		TotalVisited     int64 `gorm:"column:total_visited"`
		TotalPreCustomer int64 `gorm:"column:total_pre_customer"`
		TotalCustomer    int64 `gorm:"column:total_customer"`
	}
	sCust, sCustArgs := buildDashboardScopeOpt(c, "customer", f.BranchID)
	// status IN (...) tường minh — KHÔNG dùng status >= N (CustomerStatusContact=16 là giai
	// đoạn sớm nhưng giá trị số lớn nhất, >= sẽ tính nhầm vào total_visited/total_pre_customer).
	rArgs := []any{
		define.CustomerFunnelTicketUp, define.CustomerFunnelLeadUp, define.CustomerFunnelAppointmentUp,
		define.CustomerFunnelVisitedUp, define.CustomerFunnelPreCustomerUp, define.CustomerStatusCustomer,
		from, to,
	}
	rArgs = append(rArgs, sCustArgs...)
	tx := h.DB.Raw(`SELECT
		COUNT(*) AS total_all,
		SUM(CASE WHEN status IN ? THEN 1 ELSE 0 END) AS total_ticket,
		SUM(CASE WHEN status IN ? THEN 1 ELSE 0 END) AS total_lead,
		SUM(CASE WHEN status IN ? THEN 1 ELSE 0 END) AS total_appointment,
		SUM(CASE WHEN status IN ? THEN 1 ELSE 0 END) AS total_visited,
		SUM(CASE WHEN status IN ? THEN 1 ELSE 0 END) AS total_pre_customer,
		SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS total_customer
		FROM customer WHERE created_day BETWEEN ? AND ?`+sCust, rArgs...).Scan(&row)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	if row.TotalAll == 0 {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	statusResult := []gin.H{
		{"value": row.TotalTicket, "name": "Ticket"},
		{"value": row.TotalLead, "name": "Lead"},
		{"value": row.TotalAppointment, "name": "Đặt hẹn"},
		{"value": row.TotalVisited, "name": "Đã đến"},
		{"value": row.TotalPreCustomer, "name": "Đồng ý"},
		{"value": row.TotalCustomer, "name": "Hoàn thành"},
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, statusResult, 300*time.Second)
	}
	c.JSON(http.StatusOK, statusResult)
}

// CrmReportCustomerByRegion GET /dashboard/crm/report-customer-by-region — cached 300s
func (h *Handler) CrmReportCustomerByRegion(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	mFrom, mTo := tz.MonthRange(loc, 0)
	f, ok := h.parseDashboardFilter(c, loc, mFrom, mTo+1)
	if !ok {
		return
	}
	// Khoá cũ dùng `time.Now().Format("2006_01")` (giờ server, không theo loc) ⇒ chuẩn hoá về khoá chung.
	cacheKey := dashCacheKey(c, "crm_customer_by_region", f)
	if h.Cache != nil {
		var cached []gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	// Câu dùng `BETWEEN ? AND ?` ⇒ mốc cuối BAO GỒM.
	from, to := f.From, f.ToIncl()

	validStatuses := []int{
		define.CustomerStatusPreCustomer, define.CustomerStatusCustomer,
		define.CustomerStatusTreatment, define.CustomerStatusComplaint,
		define.CustomerStatusDoneComplaint, define.CustomerStatusBroker,
	}

	sC, sCArgs := buildDashboardScopeOpt(c, "c", f.BranchID)
	rArgs := append([]any{from, to, validStatuses}, sCArgs...)

	var rows []struct {
		Name  string `gorm:"column:name"`
		Value int64  `gorm:"column:value"`
	}
	tx := h.DB.Raw(`SELECT lp.name, COUNT(c.id) AS value
		FROM customer c
		INNER JOIN location_province lp ON lp.id = c.province_id
		WHERE c.province_id IS NOT NULL AND lp.status = 1
		AND c.created_day BETWEEN ? AND ?
		AND c.status IN ?`+sC+`
		GROUP BY c.province_id
		ORDER BY value DESC`, rArgs...).Scan(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	if len(rows) == 0 {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	result := make([]gin.H, 0)
	var othersValue int64
	for i, r := range rows {
		if i < 8 {
			name := r.Name
			if name == "" {
				name = "Không rõ"
			}
			result = append(result, gin.H{"name": name, "value": r.Value})
		} else {
			othersValue += r.Value
		}
	}
	if othersValue > 0 {
		result = append(result, gin.H{"name": "Khu vực khác", "value": othersValue})
	}

	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 300*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// CrmOverviewServiceTypeAccepted GET /dashboard/crm/overview-service-type-accepted — cached 300s
func (h *Handler) CrmOverviewServiceTypeAccepted(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	mFrom, mTo := tz.MonthRange(loc, 0)
	f, ok := h.parseDashboardFilter(c, loc, mFrom, mTo+1)
	if !ok {
		return
	}
	// Câu dùng `BETWEEN ? AND ?` ⇒ mốc cuối BAO GỒM.
	from, to := f.From, f.ToIncl()

	cacheKey := dashCacheKey(c, "crm_service_type_accepted", f)
	if h.Cache != nil {
		var cached []gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	var rows []struct {
		Name  string `gorm:"column:name"`
		Total int64  `gorm:"column:total"`
	}
	sA, sAArgs := buildDashboardScopeOpt(c, "a", f.BranchID)
	isAcceptedIDs := h.getIsAcceptedIDs(c)
	rArgs := append([]any{from, to, isAcceptedIDs}, sAArgs...)
	tx := h.DB.Raw(`SELECT st.name, COUNT(*) AS total
		FROM appointment a
		INNER JOIN service_type st ON a.type_service_accepted_id = st.id
		INNER JOIN appointment_type t ON a.appointment_type_id = t.id
		WHERE a.type_service_accepted_id IS NOT NULL
		AND a.appointment_date BETWEEN ? AND ?
		AND st.status = 1
		AND t.isTypeRevenue = 1
		AND a.is_accepted IN ?`+sA+`
		GROUP BY a.type_service_accepted_id
		ORDER BY total DESC`, rArgs...).Scan(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	colors := []string{"#5470c6", "#91cc75", "#fac858", "#ee6666"}
	result := make([]gin.H, 0)
	var others int64

	for i, r := range rows {
		if i < 3 {
			result = append(result, gin.H{
				"name":      r.Name,
				"value":     r.Total,
				"itemStyle": gin.H{"color": colors[i]},
			})
		} else {
			others += r.Total
		}
	}

	if others > 0 {
		result = append(result, gin.H{
			"name":      "Nhóm dịch vụ khác",
			"value":     others,
			"itemStyle": gin.H{"color": colors[3]},
		})
	}

	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 300*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// CrmOverviewChartSource GET /dashboard/crm/overview-chart-source — cached 300s
func (h *Handler) CrmOverviewChartSource(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	mFrom, mTo := tz.MonthRange(loc, 0)
	f, ok := h.parseDashboardFilter(c, loc, mFrom, mTo+1)
	if !ok {
		return
	}
	// Câu dùng `BETWEEN ? AND ?` ⇒ mốc cuối BAO GỒM.
	from, to := f.From, f.ToIncl()

	cacheKey := dashCacheKey(c, "crm_overview_chart_source", f)
	if h.Cache != nil {
		var cached []gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	var rows []struct {
		Total    int64  `gorm:"column:total"`
		SourceID int    `gorm:"column:source_id"`
		Name     string `gorm:"column:name_source"`
	}
	sC, sCArgs := buildDashboardScopeOpt(c, "c", f.BranchID)
	rArgs := append([]any{from, to}, sCArgs...)
	tx := h.DB.Raw(`SELECT COUNT(c.source_id) AS total, c.source_id, s.name AS name_source
		FROM customer c
		INNER JOIN source s ON s.id = c.source_id
		WHERE c.source_id IS NOT NULL
		AND c.created_day BETWEEN ? AND ?
		AND s.status = 1`+sC+`
		GROUP BY c.source_id
		ORDER BY total DESC`, rArgs...).Scan(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	if len(rows) == 0 {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	// Map source_id → group name
	type gsRow struct {
		Name     string `gorm:"column:name"`
		SourceId string `gorm:"column:sourceId"`
	}
	var groups []gsRow
	tx = h.DB.Raw(`SELECT name, sourceId FROM group_source ORDER BY SortOrder DESC`).Scan(&groups)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	sourceGroup := make(map[int]string)
	for _, g := range groups {
		var ids []int
		if err := json.Unmarshal([]byte(g.SourceId), &ids); err == nil {
			for _, id := range ids {
				sourceGroup[id] = g.Name
			}
		}
	}

	// Trả toàn bộ nguồn với group info — FE tự giới hạn outer ring và nhóm inner ring.
	result := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		groupName := sourceGroup[r.SourceID]
		if groupName == "" {
			groupName = "Nguồn khác"
		}
		name := r.Name
		if name == "" {
			name = "Không xác định"
		}
		result = append(result, gin.H{
			"name":  name,
			"value": r.Total,
			"group": groupName,
		})
	}

	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 300*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// CrmOverviewServiceTypeInterest GET /dashboard/crm/overview-service-type-interest
func (h *Handler) CrmOverviewServiceTypeInterest(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	mFrom, mTo := tz.MonthRange(loc, 0)
	f, ok := h.parseDashboardFilter(c, loc, mFrom, mTo+1)
	if !ok {
		return
	}
	// Câu dùng `BETWEEN ? AND ?` ⇒ mốc cuối BAO GỒM.
	from, to := f.From, f.ToIncl()

	cacheKey := dashCacheKey(c, "crm_service_type_interest", f)
	if h.Cache != nil {
		var cached []gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	sC, sCArgs := buildDashboardScopeOpt(c, "c", f.BranchID)
	var rows []struct {
		Name  string `gorm:"column:name"`
		Total int64  `gorm:"column:total"`
	}
	args := []any{from, to}
	args = append(args, sCArgs...)
	tx := h.DB.Raw(`SELECT st.name, COUNT(*) AS total
		FROM customer c
		INNER JOIN service_type st ON st.id = c.type_service_id
		WHERE c.type_service_id IS NOT NULL
		AND c.created_day BETWEEN ? AND ?
		AND st.status = 1`+sC+`
		GROUP BY c.type_service_id
		ORDER BY total DESC`, args...).Scan(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	if len(rows) == 0 {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	result := make([]gin.H, 0)
	var others int64
	for i, r := range rows {
		if i < 10 {
			name := r.Name
			if name == "" {
				name = "Không xác định"
			}
			result = append(result, gin.H{"name": name, "value": r.Total})
		} else {
			others += r.Total
		}
	}
	if others > 0 {
		result = append(result, gin.H{"name": "Nhóm dịch vụ khác", "value": others})
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 300*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// CrmServiceUsedStatistics GET /dashboard/crm/service-used-statistics
func (h *Handler) CrmServiceUsedStatistics(c *gin.Context) {
	loc := tz.LocFromContext(c, h.DB)
	mFrom, mTo := tz.MonthRange(loc, 0)
	f, ok := h.parseDashboardFilter(c, loc, mFrom, mTo+1)
	if !ok {
		return
	}
	// Câu dùng `BETWEEN ? AND ?` ⇒ mốc cuối BAO GỒM.
	from, to := f.From, f.ToIncl()

	cacheKey := dashCacheKey(c, "crm_service_used_statistics", f)
	if h.Cache != nil {
		var cached []gin.H
		if err := h.Cache.Get(cacheKey, &cached); err == nil {
			c.JSON(http.StatusOK, cached)
			return
		}
	}

	sOsu, sOsuArgs := buildDashboardScopeOpt(c, "osu", f.BranchID)
	var rows []struct {
		Name     string          `gorm:"column:name"`
		Quantity int64           `gorm:"column:quantity"`
		Revenue  decimal.Decimal `gorm:"column:revenue"`
	}
	args := []any{from, to}
	args = append(args, sOsuArgs...)
	tx := h.DB.Raw(`SELECT s.name, COUNT(osu.id) AS quantity, COALESCE(SUM(osu.payment), 0) AS revenue
		FROM order_service_used osu
		INNER JOIN service s ON s.id = osu.service_id
		WHERE osu.isLock = 1
		AND osu.created_day BETWEEN ? AND ?`+sOsu+`
		GROUP BY osu.service_id
		ORDER BY revenue DESC, quantity DESC`, args...).Scan(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	if len(rows) == 0 {
		c.JSON(http.StatusOK, []gin.H{})
		return
	}

	result := make([]gin.H, 0)
	var othersQty int64
	var othersRev decimal.Decimal
	for i, r := range rows {
		if i < 10 {
			name := r.Name
			if name == "" {
				name = "Không xác định"
			}
			result = append(result, gin.H{"name": name, "quantity": r.Quantity, "revenue": r.Revenue})
		} else {
			othersQty += r.Quantity
			othersRev = othersRev.Add(r.Revenue)
		}
	}
	if othersQty > 0 {
		result = append(result, gin.H{"name": "Dịch vụ khác", "quantity": othersQty, "revenue": othersRev})
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, result, 300*time.Second)
	}
	c.JSON(http.StatusOK, result)
}

// ── Payment query helpers ──
//
// Mọi helper đều nhận gin.Context để scope tenant (branch_id) + include tenant vào cache key
// (chống cache poisoning giữa các chi nhánh).

// tenantCacheKey trả prefix cache cho tenant hiện tại.
// Dùng sorted join thay vì sum để tránh collision: [1,4] và [2,3] cùng sum=5 nhưng khác tenant.
// Story-38: bỏ phần company (cA/c) khỏi key — chỉ 1 thương hiệu/DB, key cũ (có cA/c) tự hết TTL
// (≤300s toàn dashboard), không cần flush cache thủ công.
func tenantCacheKey(c *gin.Context) string {
	branchActive := middleware.GetBranchActiveID(c)
	branchIDs := middleware.GetBranchIDs(c)

	sort.Slice(branchIDs, func(i, j int) bool { return branchIDs[i] < branchIDs[j] })

	bStrs := make([]string, len(branchIDs))
	for i, id := range branchIDs {
		bStrs[i] = strconv.FormatUint(uint64(id), 10)
	}

	return fmt.Sprintf("bA%d_b%s",
		branchActive,
		strings.Join(bStrs, ","),
	)
}

// queryDoanhSo/queryDoanhThu/queryThucThu/queryHoanDichVu/queryHoanCoc trả (giá trị, error) thay vì
// tự gọi pagination.FailQuery — các handler aggregator (CrmPaymentInMonth, CrmPaymentToday) gọi
// NHIỀU helper này rồi gộp field vào 1 response; error phải trả về caller để caller return NGAY khi
// query đầu tiên lỗi, không chạy tiếp các query sau (nguyên tắc aggregator).
func (h *Handler) queryDoanhSo(c *gin.Context, from, to int64, branchID int) (decimal.Decimal, error) {
	cacheKey := helperCacheKey(c, "crm_doanh_so", from, to, branchID)
	if h.Cache != nil {
		var v decimal.Decimal
		if err := h.Cache.Get(cacheKey, &v); err == nil {
			return v, nil
		}
	}
	scope, scopeArgs := buildDashboardScopeOpt(c, "osu", branchID)
	args := append([]any{from, to}, scopeArgs...)
	var v decimal.Decimal
	tx := h.DB.Raw(`SELECT COALESCE(SUM(osu.totalEnd), 0) FROM order_service_used osu WHERE osu.created_day BETWEEN ? AND ?`+scope, args...).Scan(&v)
	if pagination.IsQueryError(tx.Error) {
		return decimal.Zero, tx.Error
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, v, 300*time.Second)
	}
	return v, nil
}

// queryCongNo — công nợ của các dòng dịch vụ lên trong kỳ = Σ phải thu − Σ đã thu của CHÍNH các dòng đó.
// Phải thu = totalEnd (TRƯỚC thuế) + vat_amount; osu.payment là tiền CÓ thuế (bmad/order-vat). Bản cũ lấy
// doanhSo − doanhThu: trừ doanh số (trước thuế, theo dòng dịch vụ) cho tiền thu theo ĐƠN lên trong kỳ — hai tập
// khác nhau, và sai thêm phần VAT khi đơn có thuế. Dòng khoá (isLock=2) chốt phải thu = payment ⇒ tự ra 0.
func (h *Handler) queryCongNo(c *gin.Context, from, to int64, branchID int) (decimal.Decimal, error) {
	cacheKey := helperCacheKey(c, "crm_cong_no", from, to, branchID)
	if h.Cache != nil {
		var v decimal.Decimal
		if err := h.Cache.Get(cacheKey, &v); err == nil {
			return v, nil
		}
	}
	scope, scopeArgs := buildDashboardScopeOpt(c, "osu", branchID)
	args := append([]any{from, to}, scopeArgs...)
	var v decimal.Decimal
	tx := h.DB.Raw(`SELECT COALESCE(SUM(COALESCE(osu.totalEnd, 0) + osu.vat_amount - COALESCE(osu.payment, 0)), 0)
		FROM order_service_used osu WHERE osu.created_day BETWEEN ? AND ?`+scope, args...).Scan(&v)
	if pagination.IsQueryError(tx.Error) {
		return decimal.Zero, tx.Error
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, v, 300*time.Second)
	}
	return v, nil
}

func (h *Handler) queryDoanhThu(c *gin.Context, from, to int64, branchID int) (decimal.Decimal, error) {
	cacheKey := helperCacheKey(c, "crm_doanh_thu", from, to, branchID)
	if h.Cache != nil {
		var v decimal.Decimal
		if err := h.Cache.Get(cacheKey, &v); err == nil {
			return v, nil
		}
	}
	// Scope qua payment (alias p) vì đây là bảng chính có WHERE; order_plan JOIN theo parent_id.
	scope, scopeArgs := buildDashboardScopeOpt(c, "p", branchID)
	args := append([]any{from, to, define.PaymentForOrder, define.PaymentForMoveDeposit, from, to}, scopeArgs...)
	var v decimal.Decimal
	tx := h.DB.Raw(`SELECT COALESCE(SUM(p.amount), 0)
		FROM order_plan o
		INNER JOIN payment p ON p.parent_id = o.id
		WHERE o.created_time BETWEEN ? AND ?
		AND p.payment_for IN (?,?)
		AND p.payment_of_date BETWEEN ? AND ?`+scope, args...).Scan(&v)
	if pagination.IsQueryError(tx.Error) {
		return decimal.Zero, tx.Error
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, v, 300*time.Second)
	}
	return v, nil
}

func (h *Handler) queryThucThu(c *gin.Context, from, to int64, branchID int) (decimal.Decimal, error) {
	cacheKey := helperCacheKey(c, "crm_thuc_thu", from, to, branchID)
	if h.Cache != nil {
		var v decimal.Decimal
		if err := h.Cache.Get(cacheKey, &v); err == nil {
			return v, nil
		}
	}
	scope, scopeArgs := buildDashboardScopeOpt(c, "payment", branchID)
	args := append([]any{from, to, define.PaymentForDeposit, define.PaymentForOrder, define.PaymentTypeDepositKG}, scopeArgs...)
	var v decimal.Decimal
	tx := h.DB.Raw(`SELECT COALESCE(SUM(amount), 0) FROM payment
		WHERE payment_time BETWEEN ? AND ?
		AND payment_for IN (?,?)
		AND (deposit_type IS NULL OR deposit_type NOT IN (?))`+scope, args...).Scan(&v)
	if pagination.IsQueryError(tx.Error) {
		return decimal.Zero, tx.Error
	}
	ttl := 300 * time.Second
	if to-from <= 86400 {
		ttl = 60 * time.Second
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, v, ttl)
	}
	return v, nil
}

func (h *Handler) queryHoanDichVu(c *gin.Context, from, to int64, branchID int) (decimal.Decimal, error) {
	cacheKey := helperCacheKey(c, "crm_hoan_dv", from, to, branchID)
	if h.Cache != nil {
		var v decimal.Decimal
		if err := h.Cache.Get(cacheKey, &v); err == nil {
			return v, nil
		}
	}
	scope, scopeArgs := buildDashboardScopeOpt(c, "payment", branchID)
	args := append([]any{from, to, define.PaymentForRefundService}, scopeArgs...)
	var v decimal.Decimal
	tx := h.DB.Raw(`SELECT COALESCE(SUM(amount), 0) FROM payment WHERE payment_time BETWEEN ? AND ? AND payment_for = ?`+scope, args...).Scan(&v)
	if pagination.IsQueryError(tx.Error) {
		return decimal.Zero, tx.Error
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, v, 300*time.Second)
	}
	return v, nil
}

func (h *Handler) queryHoanCoc(c *gin.Context, from, to int64, branchID int) (decimal.Decimal, error) {
	cacheKey := helperCacheKey(c, "crm_hoan_coc", from, to, branchID)
	if h.Cache != nil {
		var v decimal.Decimal
		if err := h.Cache.Get(cacheKey, &v); err == nil {
			return v, nil
		}
	}
	scope, scopeArgs := buildDashboardScopeOpt(c, "payment", branchID)
	args := append([]any{from, to, define.PaymentForRefundDeposit}, scopeArgs...)
	var v decimal.Decimal
	tx := h.DB.Raw(`SELECT COALESCE(SUM(amount), 0) FROM payment WHERE payment_time BETWEEN ? AND ? AND payment_for = ?`+scope, args...).Scan(&v)
	if pagination.IsQueryError(tx.Error) {
		return decimal.Zero, tx.Error
	}
	if h.Cache != nil {
		h.Cache.Set(cacheKey, v, 300*time.Second)
	}
	return v, nil
}

// getIsAcceptedIDs nhận c *gin.Context để lỗi SQL báo về đúng response của request hiện tại
// (pagination.FailQuery) thay vì bị nuốt — trước đây hàm này không nhận context nên KHÔNG thể
// phân biệt "chưa cấu hình" (len==0 hợp lệ) với "query lỗi" (len==0 do exception).
func (h *Handler) getIsAcceptedIDs(c *gin.Context) []int {
	const ck = "crm_is_accepted_ids"
	if h.Cache != nil {
		var cached []int
		if err := h.Cache.Get(ck, &cached); err == nil {
			return cached
		}
	}
	ids, err := sfDo(ck, func() ([]int, error) {
		var out []int
		tx := h.DB.Raw(`SELECT id FROM appointment_is_accepted WHERE isAccepted = 1 AND status = 1`).Scan(&out)
		if pagination.IsQueryError(tx.Error) {
			return nil, tx.Error
		}
		return out, nil
	})
	if err != nil {
		pagination.FailQuery(c, err)
		return nil
	}
	if h.Cache != nil {
		h.Cache.Set(ck, ids, 300*time.Second)
	}
	return ids
}

// ── Trend helper ──

// calcTrend returns (tyLe, chieu) matching Yii2 logic:
// chieu: 0=no change, 1=giảm, 2=tăng
func calcTrend(current, previous decimal.Decimal) (decimal.Decimal, int) {
	hundred := decimal.NewFromInt(100)
	ten := decimal.NewFromInt(10)
	if previous.GreaterThan(current) {
		if !current.IsZero() {
			raw := previous.Sub(current).Div(current).Mul(hundred).Mul(ten).IntPart()
			return decimal.NewFromInt(raw).Div(ten), 1
		}
		return decimal.Zero, 0
	}
	if previous.LessThan(current) {
		if !previous.IsZero() {
			raw := current.Sub(previous).Div(previous).Mul(hundred).Mul(ten).IntPart()
			return decimal.NewFromInt(raw).Div(ten), 2
		}
		return decimal.Zero, 0
	}
	return decimal.Zero, 0
}
