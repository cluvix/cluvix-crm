package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"erpCluvix/internal/middleware"
	appointmentDao "erpCluvix/internal/modules/webapp/crm/appointment/dao"
	"erpCluvix/internal/modules/webapp/crm/customer/scope"
	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/storage"
	"erpCluvix/pkg/tenant"
	"erpCluvix/pkg/tz"
)

var appointmentKeySearch = []string{
	"customer.code", "appointment.code", "customer.fullname", "customer.phone",
	"customer.id", "customer.other_phone", "customer.old_code", "customer.nickname",
}

// verifyAppointmentOwnership scope-checks appointment id thuộc tenant hiện tại
// trước khi gọi service (service không có *gin.Context).
func (h *Handler) verifyAppointmentOwnership(c *gin.Context, id uint) bool {
	var a appointmentDao.Appointment
	if err := tenant.ScopedDBNullBranch(c, h.DB, "appointment").First(&a, id).Error; err != nil {
		response.NotFound(c)
		return false
	}
	return true
}

// appointmentListColumns — cột dùng cho MÀN DANH SÁCH (và export). Giữ hẹp: list kéo hàng trăm dòng.
const appointmentListColumns = `appointment.id, appointment.code, appointment.customer_id,
	appointment.appointment_type_id, appointment.branch_id,
	appointment.appointment_date, appointment.appointment_date_first,
	appointment.appointmentListDate, appointment.schedule_time, appointment.appointment_number,
	appointment.checkin_date, appointment.checkout_date,
	appointment.content, appointment.contentPrivate, appointment.noteAppointment, appointment.confirmAppointment,
	appointment.bookType, appointment.status, appointment.is_accepted,
	appointment.appointment_reason_id, appointment.type_service_id,
	appointment.type_service_accepted_id, appointment.service_id, appointment.order_service_used_id,
	appointment.source_id,
	appointment.revenue_id, appointment.convert_id,
	appointment.created_by, appointment.created_at, appointment.updated_by, appointment.updated_at,
	appointment.noteTimeline`

// appointmentListPageHint — optimizer hint CHỈ gắn vào câu lấy TRANG của màn danh sách (không gắn
// COUNT, export, chi tiết). Đi cặp với index `idx_appointment_date_id (appointment_date, id DESC)`
// trong migration create_appointment.
//
// Bối cảnh (bench L 2026-10-19, 1,2M lịch hẹn, 5 chi nhánh): FE mặc định `sort=appointment_date`
// ⇒ ORDER BY `appointment_date ASC, appointment.id DESC` (vế id DESC do pkg/query withTiebreaker
// tự nối). Không index nào trả sẵn thứ tự đó ⇒ quét/sort 1,2M dòng: tháng 0,86s · không lọc ngày
// 1,2–1,7s · trang 500 1,74s. Thêm index (appointment_date, id DESC) — InnoDB tự gắn PK ASC vào
// cuối index phụ nên index chỉ có `appointment_date` KHÔNG phục vụ được id DESC; phải khai rõ.
//
// Index một mình chỉ chữa ca NHIỀU chi nhánh `(branch_id IN (…) OR IS NULL)` (EXPLAIN ANALYZE,
// mỗi ca đo 1 lần): tháng 0,13ms · không lọc 0,07ms · trang 500 61ms · hôm nay 0,14ms.
// Ca MỘT chi nhánh `(branch_id = ? OR branch_id IS NULL)` — màn mặc định của mọi nhân viên 1 chi
// nhánh — optimizer vẫn chọn ref_or_null trên `fk-appointment_branch_id` (481k dòng) + sort:
// tháng 721ms · không lọc 776ms · trang 500 846ms; sort mặc định `id DESC` cũng dính y hệt (547ms,
// cùng cơ chế mục C-4 của customer). NO_INDEX(fk-appointment_branch_id) ⇒ 0,10ms · 0,15ms · 80ms;
// id DESC đi PRIMARY ngược 0,05ms; hôm nay vẫn range trên index mới 0,42ms.
//
// ĐÃ THỬ VÀ LOẠI:
//   - index (appointment_date, id DESC, branch_id): plan 1 chi nhánh y hệt (vẫn ref_or_null + sort).
//   - histogram trên branch_id: không đổi plan.
//   - hint ORDER_INDEX(appointment idx_appointment_date_id): biến range tháng thành quét cả index
//     (5,5s) — tệ hơn bản cũ.
//   - gắn hint vào cả COUNT: COUNT 1 chi nhánh không lọc ngày 79ms → 539ms ⇒ COUNT chạy riêng.
//
// Đánh đổi đã biết: chi nhánh RẤT NHỎ (vd 0,1% lịch hẹn) xem trang CUỐI khi không lọc ngày phải đi
// dọc index ngày tới khi gom đủ OFFSET dòng của chi nhánh đó (có thể tới cả bảng), trong khi
// fk-appointment_branch_id lẽ ra chỉ đọc vài nghìn dòng. Chấp nhận: ca phổ biến (chi nhánh lớn,
// trang đầu, lọc ngày) trước đây trả ~0,8s mọi lần mở màn.
//
// Hint (không phải FORCE/IGNORE INDEX): index bị đổi tên ⇒ MySQL chỉ cảnh báo, không lỗi 1176.
// Hint thay đổi PLAN, không đổi tập dòng hay thứ tự (ORDER BY vẫn là thứ tự toàn phần) — neo bằng
// TestListAppointment_ThuTuKhongDoi.
const appointmentListPageHint = "/*+ NO_INDEX(appointment `fk-appointment_branch_id`) */ "

// appointmentListCountQuery — bản sao của query danh sách dùng làm thân câu COUNT: CHỈ chọn
// `appointment.id` và BỎ ORDER BY. Không đổi số dòng (query không có DISTINCT/GROUP BY; các JOIN
// giữ nguyên), chỉ đổi plan.
//
// Vì sao (bench L 2026-10-19, round 2, EXPLAIN ANALYZE 1 lượt/ô, SQL đúng như handler sinh ra):
// câu COUNT cũ `SELECT COUNT(*) FROM (SELECT <32 cột> … ORDER BY appointment_date, id DESC)` —
// MySQL gộp bảng dẫn xuất nhưng vẫn tính `appointment_date` của ORDER BY vào tập cột cần đọc ⇒
// index `fk-appointment_branch_id (branch_id, id)` KHÔNG còn phủ ⇒ tra 481k dòng:
//
//	scope                     | cũ (cột đủ + ORDER BY)            | chỉ id + ORDER BY | chỉ id, bỏ ORDER BY
//	1 chi nhánh, không lọc    | lookup fk-branch tra dòng · 505ms | như cũ · 494ms    | COVERING lookup fk-branch · 121ms
//	5 chi nhánh, không lọc    | covering date_scope · 268ms       | 375ms             | covering fk-branch · 216ms
//
// (Đây là phần lớn chênh lệch HTTP 1 chi nhánh 484ms vs admin 227ms.) Phải bỏ CẢ HAI: chỉ thu cột
// mà giữ ORDER BY thì plan y nguyên.
//
// `Session` đặt clone=2 ⇒ `Select` trả bản Statement đã CLONE (map Clauses chép mới) ⇒ xoá
// "ORDER BY" ở bản sao không đụng `q` của câu trang — TestListAppointment_ThuTuKhongDoi canh.
func appointmentListCountQuery(q *gorm.DB) *gorm.DB {
	cq := q.Session(&gorm.Session{}).Select("appointment.id")
	delete(cq.Statement.Clauses, "ORDER BY")
	return cq
}

// appointmentDetailColumns — MÀN CHI TIẾT: hiện trùng cột của list (nhóm tiếp nhận/sinh hiệu
// EMR đã bỏ ở story-12 crm-standalone). Giữ hằng riêng để màn chi tiết thêm cột mà không đụng list.
const appointmentDetailColumns = appointmentListColumns

// BuildAppointmentQuery builds appointment list query (prepareIndexProvider) — đường HTTP.
//
// Phạm vi tenant lấy từ SESSION; tham số lọc lấy từ query string. Hai thứ đó tách rời nhau ở
// BuildAppointmentQueryIn để cron/worker dựng lại đúng query này từ bộ lọc đã lưu trong DB
// (đăng ký đồng bộ Google Calendar) mà không phải bịa *gin.Context — bịa context là bịa luôn
// phạm vi tenant, và phạm vi sai ở đây nghĩa là lấy dữ liệu của phòng khám khác.
func (h *Handler) BuildAppointmentQuery(c *gin.Context) *gorm.DB {
	return h.BuildAppointmentQueryIn(c, tenant.BoundsFromContext(c))
}

// BuildAppointmentQueryIn — thân thật. `p` là nguồn tham số lọc, `b` là phạm vi tenant.
// Caller ngoài HTTP PHẢI tự dựng Bounds đúng của chủ sở hữu bộ lọc; Bounds không có chi nhánh =
// mọi chi nhánh (story-41, D15: không còn phạm vi company để chặn).
func (h *Handler) BuildAppointmentQueryIn(p query.Params, b tenant.Bounds) *gorm.DB {
	c := p // giữ tên `c` cho phần thân bên dưới — 40 lời gọi query.* không phải sửa
	q := tenant.ScopedDBNullBranchBounds(h.DB, b, "appointment").
		Table("appointment")

	// LEFT JOIN customer chỉ gắn khi thật sự cần, và tìm kiếm từ khoá đi đường ngram khi được
	// — xem list_search.go để biết số đo và các cách đã thử rồi loại.
	joinCustomer := needsCustomerJoin(c)
	if !joinCustomer {
		var handled bool
		if q, handled = h.applyKeywordFast(c, q); !handled {
			joinCustomer = true // ngram không phủ được ca này ⇒ rơi về LIKE, cần join
		}
	}
	if joinCustomer {
		q = q.Joins("LEFT JOIN customer ON customer.id = appointment.customer_id")
		// Keyword search across appointment + customer fields. No-op khi không có `_keyword`.
		q = query.ApplyKeywordFilter(c, q, appointmentKeySearch...)
	}

	// Exact filters on appointment
	q = query.ApplyFilter(c, q, "appointment_type_id", "appointment.appointment_type_id")
	q = query.ApplyFilter(c, q, "branch_id", "appointment.branch_id")
	q = query.ApplyFilter(c, q, "status", "appointment.status")
	q = query.ApplyFilter(c, q, "source_id", "appointment.source_id")
	q = query.ApplyFilter(c, q, "revenue_id", "appointment.revenue_id")
	q = query.ApplyFilter(c, q, "convert_id", "appointment.convert_id")
	q = query.ApplyFilter(c, q, "type_service_id", "appointment.type_service_id")
	q = query.ApplyFilter(c, q, "created_by", "appointment.created_by")
	q = query.ApplyFilter(c, q, "is_accepted", "appointment.is_accepted")
	q = query.ApplyFilter(c, q, "type_service_accepted_id", "appointment.type_service_accepted_id")
	q = query.ApplyFilter(c, q, "customer_id", "appointment.customer_id")

	// Range filters (ActiveDataFilter integer fields)
	q = query.ApplyFilter(c, q, "appointment_date", "appointment.appointment_date")
	q = query.ApplyFilter(c, q, "appointment_date_first", "appointment.appointment_date_first")
	q = query.ApplyFilter(c, q, "checkin_date", "appointment.checkin_date")
	q = query.ApplyFilter(c, q, "checkout_date", "appointment.checkout_date")
	q = query.ApplyFilter(c, q, "created_at", "appointment.created_at")
	q = query.ApplyFilter(c, q, "updated_at", "appointment.updated_at")

	// _stage_id: filter bước (stage) của khách — thay `_status` lọc tiến trình cũ của khách (story-05).
	q = query.ApplyFilter(c, q, "_stage_id", "customer.stage_id")

	// _gender: filter customer gender
	q = query.ApplyFilter(c, q, "_gender", "customer.gender")

	// customerType: filter customer customerType
	q = query.ApplyFilter(c, q, "customerType", "customer.customerType")

	// category: filter customer category
	q = query.ApplyFilter(c, q, "category", "customer.category")

	// _groupSource: filter by group source → get source IDs from group
	if vals := query.GetFilterValues(c, "_groupSource"); len(vals) > 0 {
		sourceIds := query.GetSourceIdsByGroupSource(h.DB, vals)
		if len(sourceIds) > 0 {
			q = q.Where("appointment.source_id IN ?", sourceIds)
		}
	}

	// _groupLocal: filter by group local → get province IDs
	if vals := query.GetFilterValues(c, "_groupLocal"); len(vals) > 0 {
		provinceIds := query.GetProvinceIdsByGroupLocal(h.DB, vals)
		if len(provinceIds) > 0 {
			q = q.Where("customer.province_id IN ?", provinceIds)
		}
	}

	// _groupCountry: filter by group country → get country IDs
	if vals := query.GetFilterValues(c, "_groupCountry"); len(vals) > 0 {
		countryIds := query.GetCountryIdsByGroupCountry(h.DB, vals)
		if len(countryIds) > 0 {
			q = q.Where("customer.country_id IN ?", countryIds)
		}
	}

	// _department: filter by revenue user's departmentList (JSON_CONTAINS)
	if vals := query.GetFilterValues(c, "_department"); len(vals) > 0 {
		q = q.Joins("LEFT JOIN user_profile ON user_profile.user_id = appointment.revenue_id")
		conditions := make([]string, len(vals))
		args := make([]interface{}, len(vals))
		for i, v := range vals {
			conditions[i] = "JSON_CONTAINS(user_profile.departmentList, ?)"
			args[i] = v
		}
		orClause := ""
		for i, cond := range conditions {
			if i > 0 {
				orClause += " OR "
			}
			orClause += cond
		}
		q = q.Where(orClause, args...)
	}

	// Sort — whitelist các field cho phép sort. Field không match → fallback default.
	q = query.ApplySort(c, q, "appointment.id DESC",
		"appointment.id",
		"appointment.appointment_date",
		"appointment.appointment_date_first",
		"appointment.appointment_number",
		"appointment.code",
		"appointment.status",
		"appointment.created_at",
		"appointment.updated_at",
		"appointment.checkin_date",
		"appointment.checkout_date",
	)

	return q.Select(appointmentListColumns)
}

// scopedAppointmentQuery — BuildAppointmentQuery + phạm vi telesale (S-10: chỉ lịch hẹn của khách
// mình phụ trách). Chỉ đường HTTP (list/detail/export); BuildAppointmentQueryIn của cron/worker
// không có user nên không áp. Lỗi đọc phạm vi ⇒ error (fail-closed).
func (h *Handler) scopedAppointmentQuery(c *gin.Context) (*gorm.DB, error) {
	return scope.ApplyCustomerID(c, h.DB, h.BuildAppointmentQuery(c), "appointment.customer_id")
}

// ListAppointment GET /crm/appointment
func (h *Handler) ListAppointment(c *gin.Context) {
	q, err := h.scopedAppointmentQuery(c)
	if err != nil {
		response.ServerError(c, err)
		return
	}

	if c.Query("_export") == "1" {
		exportHandlers.ExportAppointment(c, q)
		return
	}

	// COUNT chạy RIÊNG, KHÔNG hint, trên thân rút gọn (chỉ id, bỏ ORDER BY — appointmentListCountQuery).
	// Câu trang thì gắn appointmentListPageHint (lý do + số đo ở hằng đó). Tách vì hint làm COUNT 1
	// chi nhánh không lọc ngày CHẬM đi (bench L: 79ms qua fk-appointment_branch_id → 539ms quét
	// idx_appointment_created_scope).
	var total int64
	if cnt := h.DB.Raw("SELECT COUNT(*) FROM (?) AS cnt", appointmentListCountQuery(q)).Scan(&total); pagination.IsQueryError(cnt.Error) {
		pagination.FailQuery(c, cnt.Error)
		return
	}
	pageQ := q.Session(&gorm.Session{}).Select(appointmentListPageHint + appointmentListColumns)

	items := make([]map[string]interface{}, 0)
	result := pagination.PaginateWithTotal(c, pageQ, &items, total)

	// Attach nested customer object
	if len(items) > 0 {
		var customerIds []interface{}
		for _, item := range items {
			if cid, ok := item["customer_id"]; ok && cid != nil {
				customerIds = append(customerIds, cid)
			}
		}
		if len(customerIds) > 0 {
			var customers []map[string]interface{}
			tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").
				Select("id, code, old_code, fullname, nickname, phone, gender, avatar, stage_id, province_id, address, ward_id, country_id").
				Where("id IN ?", customerIds).
				Find(&customers)
			if pagination.IsQueryError(tx.Error) {
				pagination.FailQuery(c, tx.Error)
				return
			}
			storage.ResolveRows(customers, "avatar")

			wardIds := make([]interface{}, 0)
			for _, cu := range customers {
				if wardID := helpers.ToInt64(cu["ward_id"]); wardID > 0 {
					wardIds = append(wardIds, wardID)
				}
			}
			wardMap := map[int64]map[string]interface{}{}
			if len(wardIds) > 0 {
				var wards []map[string]interface{}
				tx := h.DB.Table("location_ward").Select("id, name").Where("id IN ?", wardIds).Find(&wards)
				if pagination.IsQueryError(tx.Error) {
					pagination.FailQuery(c, tx.Error)
					return
				}
				for _, ward := range wards {
					wardMap[helpers.ToInt64(ward["id"])] = ward
				}
			}
			for _, cu := range customers {
				attachWardToCustomer(c, cu, wardMap, h.DB)
			}

			phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), customers)
			cMap := make(map[int64]map[string]interface{})
			for _, cu := range customers {
				cMap[helpers.ToInt64(cu["id"])] = cu
			}
			for _, item := range items {
				item["customer"] = cMap[helpers.ToInt64(item["customer_id"])]
			}
		}
	}

	c.JSON(http.StatusOK, result)
}

// GetAppointment GET /crm/appointment/:id
func (h *Handler) GetAppointment(c *gin.Context) {
	// id rỗng/không phải số/bằng 0 ⇒ input SAI → 422. 404 chỉ dành cho id hợp lệ nhưng không tồn tại:
	// interceptor FE coi MỌI 404 là lỗi toàn cục (điều hướng + toast toàn app).
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_id"))
		return
	}

	// Query as map để có thể attach customer (cùng pattern với ListAppointment).
	// ScopedDBNullBranch kiểm tra ownership — không thể fetch appointment của tenant khác.
	// Select riêng của màn chi tiết, KHÔNG dùng chuỗi cột của list.
	q, err := h.scopedAppointmentQuery(c)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	q = q.Select(appointmentDetailColumns).Where("appointment.id = ?", id)
	var items []map[string]interface{}
	if err := q.Limit(1).Find(&items).Error; err != nil || len(items) == 0 {
		response.NotFound(c)
		return
	}
	helpers.ParseJSONFieldsForTables(items, "appointment")
	// Cột DECIMAL map-scan trả về CHUỖI ("62.50") → parse để FE nhận number.
	helpers.ParseDecimalFieldsForTables(items, "appointment")
	item := items[0]

	if customerID := helpers.ToInt64(item["customer_id"]); customerID > 0 {
		var customers []map[string]interface{}
		// birthday: màn chi tiết hiển thị ngày sinh + tuổi khách (list không cần nên select của list
		// không có cột này).
		tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").
			Select("id, code, old_code, fullname, nickname, phone, gender, birthday, avatar, stage_id, province_id, address, ward_id, country_id").
			Where("id = ?", customerID).
			Limit(1).Find(&customers)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		storage.ResolveRows(customers, "avatar")
		if len(customers) > 0 {
			attachWardToCustomer(c, customers[0], nil, h.DB)
			phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), customers)
			item["customer"] = customers[0]
		}
	}

	if !h.attachAppointmentDetailRelations(c, item) {
		return
	}

	response.Data(c, item)
}

// attachAppointmentDetailRelations gắn các quan hệ CHỈ màn chi tiết cần: dịch vụ điều trị. Trả false khi đã ghi response lỗi (caller dừng).
//
// FK lấy từ chính record appointment đã qua tenant scope; lookup CỐ Ý không lọc chi nhánh
// (tenant.CrossBranchLookup — story-36 thay helper company-only cũ, D15 bỏ vế company).
func (h *Handler) attachAppointmentDetailRelations(c *gin.Context, item map[string]interface{}) bool {
	lookup := func(table, columns string, id int64, key string) bool {
		if id <= 0 {
			return true
		}
		var rows []map[string]interface{}
		tx := tenant.CrossBranchLookup(h.DB, table).Table(table).
			Select(columns).Where("id = ?", id).Limit(1).Find(&rows)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return false
		}
		if len(rows) > 0 {
			item[key] = rows[0]
		}
		return true
	}

	// order_service_used: chỉ field màn chi tiết hiển thị (tên dịch vụ FE tra từ cache theo service_id).
	if !lookup("order_service_used", "id, code, service_id", helpers.ToInt64(item["order_service_used_id"]), "orderServiceUsed") {
		return false
	}
	return true
}

// GetAppointmentChartStatus GET /crm/appointment/get-appointment-chart-status
func (h *Handler) GetAppointmentChartStatus(c *gin.Context) {
	from := c.Query("_time[gte]")
	to := c.Query("_time[lte]")
	if from == "" {
		from = "0"
	}
	if to == "" {
		to = fmt.Sprintf("%d", time.Now().AddDate(0, 0, 1).Truncate(24*time.Hour).Unix())
	}

	scopeFilter, scopeArgs := buildAppointmentBranchScopeSQL(c, "appointment")

	// dayAppointment = midnight unix của day-bucket trong company TZ (không phụ thuộc MySQL session TZ).
	// FLOOR((unix + offset) / 86400) * 86400 - offset = midnight UTC của ngày company-local đó.
	offsetSec := tz.OffsetSec(tz.LocFromContext(c, h.DB))
	baseSelect := fmt.Sprintf("FLOOR((appointment_date + %d) / 86400) * 86400 - %d as dayAppointment, COUNT(*) as total", offsetSec, offsetSec)
	baseJoin := "LEFT JOIN appointment_type ON appointment.appointment_type_id = appointment_type.id"
	baseWhere := "appointment_type.isTypeRevenue = 1 AND appointment_date BETWEEN ? AND ?"

	// Lịch hẹn
	var lichHen []map[string]any
	tx := h.DB.Raw("SELECT "+baseSelect+" FROM appointment "+baseJoin+" WHERE "+baseWhere+scopeFilter+" GROUP BY dayAppointment ORDER BY dayAppointment DESC",
		append([]any{from, to}, scopeArgs...)...).Find(&lichHen)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	// Khách đến (status IN 2,3)
	var khachDen []map[string]any
	tx = h.DB.Raw("SELECT "+baseSelect+" FROM appointment "+baseJoin+" WHERE "+baseWhere+" AND appointment.status IN (2,3)"+scopeFilter+" GROUP BY dayAppointment ORDER BY dayAppointment DESC",
		append([]any{from, to}, scopeArgs...)...).Find(&khachDen)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	// Khách làm (status IN 2,3 AND is_accepted=1)
	var khachLam []map[string]any
	tx = h.DB.Raw("SELECT "+baseSelect+" FROM appointment "+baseJoin+" WHERE "+baseWhere+" AND appointment.status IN (2,3) AND is_accepted = 1"+scopeFilter+" GROUP BY dayAppointment ORDER BY dayAppointment DESC",
		append([]any{from, to}, scopeArgs...)...).Find(&khachLam)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"lichHen":  formatChartXY(lichHen),
		"khachDen": formatChartXY(khachDen),
		"khachLam": formatChartXY(khachLam),
	})
}

func formatChartXY(rows []map[string]any) []gin.H {
	result := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		ts := helpers.ToInt64(r["dayAppointment"])
		if ts == 0 {
			continue
		}
		result = append(result, gin.H{
			// dash — FE (appointment-chart-scatter.ts) tự build lookup key cùng format "dd-MM-yyyy"
			// rồi match string y hệt với field "x" này; đổi sang slash sẽ làm map.get() luôn miss,
			// chart hiện toàn 0 mà không lỗi gì cả. KHÔNG đổi khi chưa sửa đồng thời FE.
			"x": time.Unix(ts, 0).Format("02-01-2006"),
			"y": helpers.ToInt64(r["total"]),
		})
	}
	return result
}

// CreateAppointment POST /crm/appointment/create-appointment
func (h *Handler) CreateAppointment(c *gin.Context) {
	var appt appointmentDao.Appointment
	if err := c.ShouldBindJSON(&appt); err != nil {
		response.ValidateErr(c, err)
		return
	}
	// Create: khóa chính do DB cấp — reset `id` client gửi kèm trong body/form (mass-assignment).
	appt.ID = 0
	// BE: branch_id/customer_id đến từ body — phải verify thuộc phạm vi session hiện tại, không
	// tin client (tránh forge sang tenant khác qua body).
	if appt.BranchId != nil && !appointmentBranchAllowed(c, appt.BranchId) {
		response.Validate(c, i18n.T(c, "branch_id_not_allowed"))
		return
	}
	if appt.CustomerId != nil && !h.appointmentCustomerInTenant(*appt.CustomerId) {
		response.NotFound(c)
		return
	}

	if appt.BranchId != nil && appt.AppointmentDate != nil {
		if msg := h.checkSlotCapacity(c, *appt.BranchId, appt.ServiceId, *appt.AppointmentDate, 0); msg != "" {
			response.Validate(c, msg)
			return
		}
	}

	userID := int(middleware.GetUserID(c))
	if err := h.AppointmentSvc.Create(&appt, userID); err != nil {
		response.HandleError(c, err)
		return
	}
	response.Created(c, appt)
}

// UpdateAppointment PUT /crm/appointment/update-appointment/:id
func (h *Handler) UpdateAppointment(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if !h.verifyAppointmentOwnership(c, uint(id)) {
		return
	}
	var appt appointmentDao.Appointment
	if err := c.ShouldBind(&appt); err != nil {
		response.ValidateErr(c, err)
		return
	}
	appt.ID = uint(id)

	// BE: branch_id/customer_id đến từ body — phải verify thuộc phạm vi session hiện tại, không
	// tin client (tránh forge sang tenant khác qua body).
	if appt.BranchId != nil && !appointmentBranchAllowed(c, appt.BranchId) {
		response.Validate(c, i18n.T(c, "branch_id_not_allowed"))
		return
	}
	if appt.CustomerId != nil && !h.appointmentCustomerInTenant(*appt.CustomerId) {
		response.NotFound(c)
		return
	}

	if appt.BranchId != nil && appt.AppointmentDate != nil {
		if msg := h.checkSlotCapacity(c, *appt.BranchId, appt.ServiceId, *appt.AppointmentDate, uint(id)); msg != "" {
			response.Validate(c, msg)
			return
		}
	}

	userID := int(middleware.GetUserID(c))
	scopedDB := tenant.ScopedDBNullBranch(c, h.DB, "appointment")

	if err := h.AppointmentSvc.Update(&appt, userID, scopedDB); err != nil {
		response.HandleError(c, err)
		return
	}

	// Reload + attach customer
	var result map[string]interface{}
	tx := tenant.ScopedDBNullBranch(c, h.DB, "appointment").Table("appointment").Where("id = ?", id).Scan(&result)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	helpers.ParseJSONFieldForTable(result, "appointment")
	// Cột DECIMAL scan vào map là []uint8 → parse để JSON ra number.
	helpers.ParseDecimalFieldsForTables([]map[string]interface{}{result}, "appointment")
	h.attachCustomerToAppointment(c, result)
	response.Updated(c, result)
}

// CustomerCheckin PUT /crm/appointment/customer-checkin/:id
func (h *Handler) CustomerCheckin(c *gin.Context) {
	// Parse ra SỐ trước khi vào query: MySQL ép '5abc' → 5 và nuốt warning 1292 ⇒ URL rác GHI
	// trúng lịch hẹn thật (Hard Rule "Path param id"). Route POST legacy nạp param từ ?id=.
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	if !h.verifyAppointmentOwnership(c, id) {
		return
	}

	// Read raw body once to detect fields the client explicitly sent as EMPTY (null hoặc "").
	// ShouldBindJSON không phân biệt được null với key vắng mặt (đều ra nil/""), mà mergeAppointment
	// lại bỏ qua nil/"" → không có bước này thì ghi chú/ngày check-in đã nhập KHÔNG BAO GIỜ xoá được,
	// chỉ ghi đè được (phát hiện khi smoke test 2026-08-12).
	//
	// Điều kiện là key CÓ MẶT trong body — không phải giá trị rỗng sau khi unmarshal. Nhờ vậy client
	// gửi payload PARTIAL (mobile `workspace/handler/appointment.go` truyền map rỗng) không bị xoá oan.
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	var rawMap map[string]json.RawMessage
	json.Unmarshal(bodyBytes, &rawMap) //nolint:errcheck
	explicitClears := map[string]bool{}
	for k, v := range rawMap {
		if s := string(v); s == "null" || s == `""` {
			explicitClears[k] = true
		}
	}

	var appt appointmentDao.Appointment
	if err := json.Unmarshal(bodyBytes, &appt); err != nil {
		response.ValidateErr(c, err)
		return
	}
	appt.ID = id

	if appt.BranchId != nil && !appointmentBranchAllowed(c, appt.BranchId) {
		response.Validate(c, i18n.T(c, "branch_id_not_allowed"))
		return
	}
	if appt.CustomerId != nil && !h.appointmentCustomerInTenant(*appt.CustomerId) {
		response.NotFound(c)
		return
	}

	userID := int(middleware.GetUserID(c))
	scopedDB := tenant.ScopedDBNullBranch(c, h.DB, "appointment")
	if err := h.AppointmentSvc.Checkin(&appt, explicitClears, userID, scopedDB); err != nil {
		response.HandleError(c, err)
		return
	}

	var result map[string]interface{}
	tx := tenant.ScopedDBNullBranch(c, h.DB, "appointment").Table("appointment").Where("id = ?", id).Scan(&result)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	helpers.ParseJSONFieldForTable(result, "appointment")
	// Cột DECIMAL scan vào map là []uint8 → parse để JSON ra number.
	helpers.ParseDecimalFieldsForTables([]map[string]interface{}{result}, "appointment")
	h.attachCustomerToAppointment(c, result)
	response.Updated(c, result)
}

// UpdateAppointmentCancel PUT /crm/appointment/update-appointment-cancel/:id
func (h *Handler) UpdateAppointmentCancel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if !h.verifyAppointmentOwnership(c, uint(id)) {
		return
	}
	var appt appointmentDao.Appointment
	if err := c.ShouldBind(&appt); err != nil {
		response.ValidateErr(c, err)
		return
	}
	appt.ID = uint(id)

	if appt.BranchId != nil && !appointmentBranchAllowed(c, appt.BranchId) {
		response.Validate(c, i18n.T(c, "branch_id_not_allowed"))
		return
	}
	if appt.CustomerId != nil && !h.appointmentCustomerInTenant(*appt.CustomerId) {
		response.NotFound(c)
		return
	}

	userID := int(middleware.GetUserID(c))
	scopedDB := tenant.ScopedDBNullBranch(c, h.DB, "appointment")
	if err := h.AppointmentSvc.Cancel(&appt, userID, scopedDB); err != nil {
		response.HandleError(c, err)
		return
	}
	var result map[string]interface{}
	tx := tenant.ScopedDBNullBranch(c, h.DB, "appointment").Table("appointment").Where("id = ?", id).Scan(&result)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	helpers.ParseJSONFieldForTable(result, "appointment")
	// Cột DECIMAL scan vào map là []uint8 → parse để JSON ra number.
	helpers.ParseDecimalFieldsForTables([]map[string]interface{}{result}, "appointment")
	h.attachCustomerToAppointment(c, result)
	response.Updated(c, result)
}

// UpdateConfirmAppointment PUT /crm/appointment/update-confirm-appointment/:id
func (h *Handler) UpdateConfirmAppointment(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if !h.verifyAppointmentOwnership(c, uint(id)) {
		return
	}
	var appt appointmentDao.Appointment
	if err := c.ShouldBind(&appt); err != nil {
		response.ValidateErr(c, err)
		return
	}
	appt.ID = uint(id)

	userID := int(middleware.GetUserID(c))
	scopedDB := tenant.ScopedDBNullBranch(c, h.DB, "appointment")
	if err := h.AppointmentSvc.UpdateConfirm(&appt, userID, scopedDB); err != nil {
		response.HandleError(c, err)
		return
	}
	var result map[string]interface{}
	tx := tenant.ScopedDBNullBranch(c, h.DB, "appointment").Table("appointment").Where("id = ?", id).Scan(&result)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	helpers.ParseJSONFieldForTable(result, "appointment")
	// Cột DECIMAL scan vào map là []uint8 → parse để JSON ra number.
	helpers.ParseDecimalFieldsForTables([]map[string]interface{}{result}, "appointment")
	h.attachCustomerToAppointment(c, result)
	response.Updated(c, result)
}

// AddNote POST /crm/appointment/:id/add-note
func (h *Handler) AddNote(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	if !h.verifyAppointmentOwnership(c, uint(id)) {
		return
	}
	var body struct {
		Note string `json:"note" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.ValidateErr(c, err)
		return
	}
	userID := int(middleware.GetUserID(c))
	if err := h.AppointmentSvc.AddNote(uint(id), body.Note, userID); err != nil {
		response.HandleError(c, err)
		return
	}
	var result map[string]interface{}
	// Đọc lại qua scoped DB (dù ownership đã verify ở trên) — giữ đúng pattern để chỗ này không bị
	// copy sang handler khác không có bước verify.
	tx := tenant.ScopedDBNullBranch(c, h.DB, "appointment").Table("appointment").
		Select("noteTimeline").Where("id = ?", id).Scan(&result)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	helpers.ParseJSONFieldForTable(result, "appointment")
	response.Updated(c, result)
}

// DeleteAppointment DELETE /crm/appointment/:id
func (h *Handler) DeleteAppointment(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if !h.verifyAppointmentOwnership(c, uint(id)) {
		return
	}
	userID := int(middleware.GetUserID(c))
	scopedDB := tenant.ScopedDBNullBranch(c, h.DB, "appointment")
	if err := h.AppointmentSvc.Delete(uint(id), userID, scopedDB); err != nil {
		response.ServerError(c, err)
		return
	}
	response.Deleted(c)
}

// AppointmentDateInDay GET /crm/appointment/appointment-date-in-day?appointmentDate=X&idCustomer=X
func (h *Handler) AppointmentDateInDay(c *gin.Context) {
	// Parse ra SỐ: MySQL ép '12abc' → 12 (warning 1292 bị nuốt) ⇒ URL rác đọc trúng khách khác.
	appointmentDate := c.Query("appointmentDate")
	idCustomer, errParse := strconv.ParseUint(c.Query("idCustomer"), 10, 64)
	if appointmentDate == "" || errParse != nil || idCustomer == 0 {
		c.JSON(http.StatusOK, []any{})
		return
	}

	ts, _ := strconv.ParseInt(appointmentDate, 10, 64)
	t := time.Unix(ts, 0)
	startOfDay := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Unix()
	endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location()).Unix()

	var rows []map[string]any
	tx := tenant.ScopedDBNullBranch(c, h.DB, "appointment", "a").Table("appointment a").
		Select("a.id, a.branch_id, a.revenue_id, a.source_id, a.appointment_date, at.name as type_name").
		Joins("LEFT JOIN appointment_type at ON at.id = a.appointment_type_id").
		Where("a.customer_id = ? AND a.status != 4 AND a.appointment_date BETWEEN ? AND ?", idCustomer, startOfDay, endOfDay).
		// LIMIT 5 + ORDER BY khong duy nhat => 5 dong NAO duoc tra ve la khong xac dinh
		Order("a.appointment_date DESC, a.id DESC").Limit(5).Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	result := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		typeName, _ := r["type_name"].(string)
		apptDate := helpers.ToInt64(r["appointment_date"])
		label := typeName + " ngày " + time.Unix(apptDate, 0).Format("02/01/2006")
		result = append(result, gin.H{
			"value": r["id"], "label": label,
			"branch_id": r["branch_id"], "revenue_id": r["revenue_id"], "source_id": r["source_id"],
		})
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) attachCustomerToAppointment(c *gin.Context, item map[string]interface{}) {
	custID := helpers.ToInt64(item["customer_id"])
	if custID == 0 {
		return
	}
	var cust map[string]interface{}
	tx := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").
		Select("id, fullname, nickname, phone, gender, birthday, code, old_code, avatar, province_id, address, ward_id, country_id").
		Where("id = ?", custID).Scan(&cust)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	attachWardToCustomer(c, cust, nil, h.DB)
	if cust != nil {
		phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), []map[string]interface{}{cust})
		storage.ResolveFields(cust, "avatar")
		item["customer"] = cust
	}
}

// checkSlotCapacity trả về error message nếu slot đã đủ công suất, "" nếu còn chỗ.
// excludeID > 0 khi update để không tự đếm appointment đang sửa.
//
// Capacity check theo cấu hình slot đã tắt (bảng khung giờ đã bỏ); field
// đếm nhân sự phục vụ slot đã bỏ khỏi Go (story-46, D17/D19 bỏ vai trò y tế) — cột DB xoá thật ở E4.
func (h *Handler) checkSlotCapacity(c *gin.Context, branchID int, serviceID *int, dateTime int64, excludeID uint) string {
	return ""
}

func attachWardToCustomer(c *gin.Context, customer map[string]interface{}, wardMap map[int64]map[string]interface{}, db *gorm.DB) {
	if customer == nil {
		return
	}
	wardID := helpers.ToInt64(customer["ward_id"])
	if wardID == 0 {
		return
	}
	if wardMap != nil {
		if ward, ok := wardMap[wardID]; ok {
			customer["ward"] = ward
		}
		return
	}
	var ward map[string]interface{}
	tx := db.Table("location_ward").Select("id, name").Where("id = ?", wardID).Scan(&ward)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	if ward != nil {
		customer["ward"] = ward
	}
}
