package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"erpCluvix/internal/middleware"
	"erpCluvix/internal/modules/webapp/crm/customer/scope"
	crmServices "erpCluvix/internal/modules/webapp/crm/service"
	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/codegen"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/imaging"
	"erpCluvix/pkg/logger"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/seclog"
	"erpCluvix/pkg/storage"
	"erpCluvix/pkg/tenant"
	"erpCluvix/pkg/validation"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	mysqldriver "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var customerStatuses = define.CustomerStatuses

var customerKeySearch = []string{
	"customer.phone", "customer.code", "customer.fullname", "customer.nickname",
	"customer.old_code", "customer.other_phone", "customer.link_message", "customer.loai_message",
}

// customerListSelect — danh sách cột của màn danh sách khách hàng.
//
// ⚠ CỐ Ý tách khỏi phần lọc (`buildCustomerFilters`): câu `COUNT` của phân trang KHÔNG được
// kéo theo danh sách cột này. Lý do đo được trên bench 615.992 khách
// (2026-09-07): `pagination.Paginate` bọc NGUYÊN query vào derived table rồi đếm, nên MySQL
// mất tính covering và phải quét bảng — 1,368s. Cùng bộ lọc mà đếm trần (không cột, không
// join, không ORDER BY) chạy trên chỉ mục phủ `idx_customer_scope_status`: **0,170s**.
const customerListSelect = `customer.id, customer.code, customer.old_code, customer.fullname, customer.nickname, customer.phone, customer.other_phone, customer.email, customer.gender, customer.status, customer.category,
			customer.avatar, customer.address, DATE_FORMAT(customer.birthday, '%Y-%m-%d') as birthday, customer.year_born, customer.created_day, customer.created_at, customer.updated_at,
			customer.source_id, customer.branch_id, customer.province_id, customer.country_id,
			customer.payment_user_id, customer.connect_id, customer.created_by, customer.updated_by,
			customer.type_service_id,
			customer.appointment_not_booked_reasons_id, customer.loai_message,
			customer.customerType, customer.job_id, customer.ethnicity_id,
			customer.tags,
			customer.totalRevenue, customer.totalPayment, customer.totalDebt, customer.totalDeposit, customer.totalBonus,
			customer.refundDeposit, customer.refundService, customer.consignment, customer.refundConsignment`

// customerListPresentation gắn phần HIỂN THỊ (cột + sắp xếp) lên một query đã đủ bộ lọc.
// (story-10 crm-standalone: bỏ `LEFT JOIN broker` lấy `affiliate_fullname` cùng module CTV.)
func customerListPresentation(q *gorm.DB, selectCols string) *gorm.DB {
	return q.Select(selectCols).
		Order("customer.id DESC")
}

// applyTelesaleScope — seam cho test: DryRun DB của list_query_test không đọc được user_profile
// (gorm trả ErrDryRunModeUnsupported cho Scan) nên test đó thay bằng pass-through.
var applyTelesaleScope = scope.Apply

// BuildCustomerQuery builds customer list query with all filters.
//
// Giữ nguyên chữ ký cũ cho các consumer ngoài (export). Bên trong nay = bộ lọc + phần hiển thị.
func (h *Handler) BuildCustomerQuery(c *gin.Context) *gorm.DB {
	return customerListPresentation(h.buildCustomerFilters(c, false), customerListSelect)
}

// buildCustomerFilters — CHỈ mệnh đề WHERE trên bảng `customer`: không cột, không join,
// không ORDER BY.
//
// ⚠ Câu hiển thị và câu đếm BẮT BUỘC dùng CHUNG hàm này. Dựng hai chuỗi lọc song song là
// cách chắc chắn nhất để hai câu lệch phạm vi rồi trả tổng số sai mà không ai thấy — đúng
// lớp lỗi đã ghi ở `perf-list-endpoints.md` §"câu baseline PHẢI có cùng phạm vi lọc".
//
// forceLike ⇒ ép tìm kiếm đi đường LIKE cũ (xem applyCustomerKeyword).
func (h *Handler) buildCustomerFilters(c *gin.Context, forceLike bool) *gorm.DB {
	q := tenant.ScopedDBNullBranch(c, h.DB, "customer").
		Table("customer").
		Where("customer.status IN ?", customerStatuses)

	q = h.applyCustomerKeyword(c, q, forceLike)
	q = query.ApplyFilter(c, q, "_status", "customer.status")
	q = query.ApplyFilter(c, q, "status", "customer.status")
	q = query.ApplyFilter(c, q, "gender", "customer.gender")
	q = query.ApplyFilter(c, q, "category", "customer.category")
	q = query.ApplyFilter(c, q, "customerType", "customer.customerType")
	q = query.ApplyFilter(c, q, "source_id", "customer.source_id")
	q = query.ApplyFilter(c, q, "branch_id", "customer.branch_id")
	q = query.ApplyFilter(c, q, "province_id", "customer.province_id")
	q = query.ApplyFilter(c, q, "payment_user_id", "customer.payment_user_id")
	q = query.ApplyFilter(c, q, "type_service_id", "customer.type_service_id")
	q = query.ApplyFilter(c, q, "loai_message", "customer.loai_message")
	q = query.ApplyFilter(c, q, "created_by", "customer.created_by")
	q = query.ApplyFilter(c, q, "created_day", "customer.created_day")
	q = query.ApplyFilter(c, q, "created_at", "customer.created_at")
	q = query.ApplyFilter(c, q, "updated_at", "customer.updated_at")
	// Age range: convert to birthday range (birthday <= today - ageFrom years)
	if v := c.Query("_ageFrom"); v != "" {
		q = q.Where("customer.birthday <= DATE_SUB(CURDATE(), INTERVAL ? YEAR)", v)
	}
	if v := c.Query("_ageTo"); v != "" {
		q = q.Where("customer.birthday >= DATE_SUB(CURDATE(), INTERVAL ? YEAR)", v)
	}

	// Cùng lớp lọc với GetCustomer (`:id`) — BẮT BUỘC giữ song song.
	// Trước 2026-08-14 chỉ GetCustomer áp scope telesale, list thì không: telesale
	// thấy đủ mọi khách trong danh sách nhưng bấm vào là 404 (query detail thêm
	// `payment_user_id = mình` nên không ra row). Story-32 tưởng list đã có scope này
	// vì nhìn nhầm handler `ListTicket` (đã xoá — D14) thành `ListCustomer`.
	// Muốn telesale xem hết → bật `user_option` define.UserOptionViewLeadOfTele (id=5), KHÔNG gỡ dòng này.
	// Fail-closed (D2): scope lỗi ⇒ KHÔNG trả bộ lọc hở. `1 = 0` + AddError làm mọi câu
	// Count/Find dựng từ q đều trả lỗi (customerListTotal/export ⇒ 500), không bao giờ lộ dữ liệu.
	scoped, err := applyTelesaleScope(c, h.DB, q)
	if err != nil {
		logger.Default().Error("customer scope: đọc phạm vi telesale lỗi — chặn truy vấn", zap.Error(err))
		q = q.Where("1 = 0")
		_ = q.AddError(err)
		return q
	}
	return scoped
}

// ListCustomer GET /crm/customer
func (h *Handler) ListCustomer(c *gin.Context) {
	if c.Query("_export") == "1" {
		// Export ép đường LIKE: nó không phân trang (không có COUNT để bắt lỗi thiếu chỉ mục
		// FULLTEXT rồi chạy lại), và ngram lợi ít khi phải kéo về toàn bộ dòng khớp.
		q := customerListPresentation(h.buildCustomerFilters(c, true), customerListSelect)
		exportHandlers.ExportCustomer(c, q)
		return
	}

	base, total, ok := h.customerListTotal(c)
	if !ok {
		return // customerListTotal đã ghi 500 + niêm phong response
	}

	var items []map[string]interface{}
	var result pagination.Result
	page, perPage, offset := pagination.PageParams(c)
	if branchID, single := customerListSingleBranch(c); single && customerListSplitWorthIt(total, offset, perPage) {
		// Phạm vi 1 chi nhánh: tra id của trang qua 2 nhánh UNION ALL (bench C-4: 392 → 0,15ms).
		// Lý do + bảng plan: list_branch_split.go.
		if err := customerListSplitPage(h.DB, base, branchID, offset, perPage).Find(&items).Error; pagination.IsQueryError(err) {
			pagination.FailQuery(c, err)
			return
		}
		result = pagination.ResultForPage(c, &items, total, page, perPage, "customer")
	} else {
		q := customerListPresentation(base.Session(&gorm.Session{}), customerListSelect)
		result = pagination.PaginateWithTotal(c, q, &items, total)
	}
	phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), items)
	storage.ResolveRows(items, "avatar")
	c.JSON(http.StatusOK, result)
}

// customerListTotal đếm tổng số dòng bằng câu GỌN (không cột, không join, không ORDER BY) và
// trả về luôn bộ lọc đã dựng để câu hiển thị dùng lại — bảo đảm hai câu KHÔNG THỂ lệch phạm vi.
//
// Trả ok=false khi đã ghi lỗi ra response; caller phải return ngay.
//
// ⚠ Câu đếm CÒN LÀ chỗ dò của đường ngram: nó chạy TRƯỚC câu hiển thị, nên nếu `MATCH(...)`
// không dùng được thì bắt được ở đây và dựng lại bộ lọc bằng LIKE — người dùng chỉ thấy chậm,
// không thấy 500. Hai nguyên nhân được bắt, CỐ Ý tách rời (lặp lý lẽ đã ghi kỹ ở
// `teamchat/service/search.go` — đọc đó trước khi gộp lại cho gọn):
//
//   - 1191 = chỉ mục KHÔNG TỒN TẠI (chưa chạy migration `20260907081100`, hoặc ai đó
//     `DROP INDEX ft_customer_ngram` để lùi). Chữa bằng: dựng chỉ mục.
//   - 188  = chỉ mục CÒN NGUYÊN nhưng tập trung gian vượt `innodb_ft_result_cache_limit`.
//     Chữa bằng: nâng hạn mức ở cấu hình máy chủ / thu hẹp phạm vi — KHÔNG phải dựng lại chỉ mục.
//
// Ghi log riêng từng mã để người vận hành không đi sai hướng hàng giờ. Bắt bằng
// `errors.As(*mysqldriver.MySQLError)` + so `Number`, KHÔNG khớp chuỗi ("188" lọt vào thông
// điệp lỗi khác quá dễ) và KHÔNG nuốt mọi lỗi SQL (nuốt hết = biến lỗi hạ tầng thành list rỗng).
func (h *Handler) customerListTotal(c *gin.Context) (*gorm.DB, int64, bool) {
	base := h.buildCustomerFilters(c, false)

	var total int64
	err := customerCountQuery(c, base, &total).Error
	if degraded, reason := customerFulltextUnusable(err); degraded {
		logger.Default().Warn("customer list: ngram không dùng được, thoái hoá xuống LIKE",
			zap.String("reason", reason), zap.Error(err))
		base = h.buildCustomerFilters(c, true)
		err = customerCountQuery(c, base, &total).Error
	}
	if pagination.IsQueryError(err) {
		pagination.FailQuery(c, err)
		return nil, 0, false
	}
	return base, total, true
}

// Mã lỗi MySQL được bắt tường minh (đặt tên thay vì rải số trần trong `if`).
const (
	customerMySQLErrNoFulltextIndex     = 1191 // Can't find FULLTEXT index matching the column list
	customerMySQLErrFTSResultCacheLimit = 188  // FTS query exceeds result cache limit
)

// customerFulltextUnusable — lỗi này có phải "ngram không dùng được, hãy đi LIKE" không.
// Trả kèm nguyên nhân để câu log nói đúng việc người vận hành phải làm.
func customerFulltextUnusable(err error) (bool, string) {
	if err == nil {
		return false, ""
	}
	var me *mysqldriver.MySQLError
	if !errors.As(err, &me) {
		return false, ""
	}
	switch me.Number {
	case customerMySQLErrNoFulltextIndex:
		return true, "thiếu chỉ mục ft_customer_ngram (chạy migration 20260907081100)"
	case customerMySQLErrFTSResultCacheLimit:
		return true, "vượt innodb_ft_result_cache_limit (chỉ mục vẫn còn, cần nâng hạn mức máy chủ)"
	}
	return false, ""
}

// GetCustomer GET /crm/customer/:id
// GetCustomerByCode GET /crm/customer/get-customer-by-code?code=xxx
func (h *Handler) GetCustomerByCode(c *gin.Context) {
	code := strings.TrimSpace(c.Query("code"))
	if code == "" || code == "null" {
		response.NotFound(c)
		return
	}
	q, err := applyTelesaleScope(c, h.DB, tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Select(`id, code, fullname, nickname, avatar, phone, status, branch_id`).
		Where("code = ?", code))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	var item map[string]interface{}
	if err := q.Take(&item).Error; err != nil {
		response.NotFound(c)
		return
	}
	phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), []map[string]interface{}{item})
	storage.ResolveFields(item, "avatar")
	response.Data(c, item)
}

// customerByIDScope — phạm vi GET/UPDATE khách theo id: lọc chi nhánh như list (khách chưa gán chi
// nhánh — branch_id NULL — luôn thấy), chỉ bỏ lọc khi user_option cross-branch (id=18) bật, GIỐNG
// SearchGlobal — để khách tìm thấy qua tìm kiếm toàn cục vẫn mở được. GET và UPDATE PHẢI dùng chung.
// Trước đây cả hai luôn skipBranch=true ⇒ ai có viewCustomer/updateCustomer dò id là đọc/sửa được
// khách của mọi chi nhánh (IDOR xuyên chi nhánh).
func (h *Handler) customerByIDScope(c *gin.Context) *gorm.DB {
	return tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "customer", tenant.IsCrossBranchEnabled(c, h.DB))
}

// customerBranchWriteAllowed — branch_id client gửi khi tạo/sửa khách phải nằm trong phạm vi chi
// nhánh của user (rỗng/NULL = chưa gán ⇒ cho qua). Thiếu bước này user kéo được khách sang chi nhánh mình.
func customerBranchWriteAllowed(c *gin.Context, fields map[string]interface{}) bool {
	if v := helpers.ToInt64(fields["branch_id"]); v > 0 {
		return middleware.IsBranchAllowed(c, uint(v))
	}
	return true
}

func (h *Handler) GetCustomer(c *gin.Context) {
	// Scope GIỐNG UpdateCustomer (customerByIDScope). Phạm vi người phụ trách áp qua scope.Apply.
	// Story-36 (D15 — 1 thương hiệu/DB): bỏ nhánh fail-closed theo company (không còn vế company).
	id, ok := parseCustomerID(c)
	if !ok {
		return
	}
	q := h.customerByIDScope(c).Table("customer").
		Select("customer.*").
		Where("customer.id = ?", id)
	q, err := applyTelesaleScope(c, h.DB, q)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	var item map[string]interface{}
	if err := q.Take(&item).Error; err != nil {
		response.NotFound(c)
		return
	}
	helpers.ParseJSONFieldForTable(item, "customer")
	// Cột DECIMAL scan vào map[string]interface{} là []uint8 → phải parse để ra JSON number
	// (không phải string).
	helpers.ParseDecimalFieldsForTables([]map[string]interface{}{item}, "customer")
	if wardID := helpers.ToInt64(item["ward_id"]); wardID > 0 {
		var ward map[string]interface{}
		tx := h.DB.Table("location_ward").Select("id, name").Where("id = ?", wardID).Scan(&ward)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		if ward != nil {
			item["ward"] = ward
		}
	}
	phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), []map[string]interface{}{item})
	storage.ResolveFields(item, "avatar")
	response.Data(c, item)
}

// CreateCustomer POST /crm/customer
func (h *Handler) CreateCustomer(c *gin.Context) {
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.HandleError(c, err)
		return
	}
	// Đường TẠO: avatar CHỈ đến từ upload (POST /:id/create-avatar) — chuỗi client gửi bị bỏ qua.
	h.applyPayloadAvatar(body, "", true)

	// Validate required
	if helpers.ToString(body["fullname"]) == "" {
		response.Validate(c, i18n.TF(c, "required", i18n.FieldLabel(c, "fullname")))
		return
	}

	// Auto status (D14, crm-standalone): khách mới LUÔN Contact(16), mọi phòng ban — để hiện
	// ngay trong danh sách khách CRM (status IN define.CustomerStatuses). Trước đây phòng
	// sale/telesale/convert/CSKH/marketing ra Lead(4), chỉ hiện ở module Sale đã xoá ⇒ mất khỏi
	// mọi danh sách. appointment_date vẫn override như cũ (Visited/Appointment).
	uid := int(middleware.GetUserID(c))
	status := define.CustomerStatusContact
	if apptDate, ok := body["appointment_date"]; ok && apptDate != nil {
		if helpers.ToInt64(apptDate) <= time.Now().Unix() {
			status = define.CustomerStatusVisited
		} else {
			status = define.CustomerStatusAppointment
		}
	}
	body["status"] = status

	// Duplicate check: cùng SĐT trong TOÀN DB, không phân biệt branch.
	//
	// D-3 (user chốt 2026-10-18: SĐT là duy nhất trong 1 DB): trước đây khoá là phone + fullname;
	// nay chỉ SĐT, khớp UNIQUE `define.CustomerPhoneUniqueIndex` ở DB. Kiểm trước chỉ để trả lỗi
	// sớm/rẻ — lưới THẬT là UNIQUE (2 request song song cùng lọt qua bước này ⇒ request sau nhận
	// 1062 ⇒ isCustomerPhoneDup ⇒ cùng lỗi i18n).
	//
	// THIẾT KẾ CỐ Ý — KHÔNG revert về ScopedDBNullBranch:
	//   • Story-36 (D15 — 1 thương hiệu/DB): trước đây kiểm trong phạm vi company_id của body; nay
	//     mỗi DB chỉ 1 thương hiệu nên phạm vi đúng là toàn bảng.
	//   • Không lọc branch: khách trùng ở bất kỳ branch nào đều bị chặn.
	//   • Phone so khớp mọi định dạng đã lưu (`0906904884` / `84906904884` /
	//     `+84906904884` / `906904884`) — ZMA lưu dạng Zalo trả về, web lưu dạng
	//     người dùng gõ; `phone = ?` thuần sẽ trượt và cho tạo trùng.
	if dup, err := h.customerPhoneTaken(helpers.ToString(body["phone"]), 0); err != nil {
		response.ServerError(c, err) // D-3: lỗi Count KHÔNG được coi là "không trùng"
		return
	} else if dup {
		response.Validate(c, i18n.T(c, "customer_duplicate"))
		return
	}

	// Audit fields
	now := time.Now().Unix()
	body["created_at"] = now
	body["updated_at"] = now
	body["created_by"] = uid
	body["updated_by"] = uid
	body["created_day"] = now

	// Remove non-column fields
	apptDate := body["appointment_date"]
	note := helpers.ToString(body["note"])
	if note == "" {
		note = helpers.ToString(body["noteCustomer"])
	}
	delete(body, "note")
	delete(body, "noteCustomer")
	delete(body, "appointment_date")

	// Filter whitelist — loại bỏ các field không thuộc bảng customer (company_id client gửi cũng bị
	// bỏ: story-36 không nhận company từ client).
	filtered := h.CustomerSvc.FilterAllowed(body)
	if err := h.CustomerSvc.ValidateFromMap(filtered, true); err != nil {
		respondCustomerSvcError(c, err)
		return
	}
	if !customerBranchWriteAllowed(c, filtered) {
		response.Validate(c, i18n.T(c, "branch_id_not_allowed"))
		return
	}
	helpers.SerializeJSONFieldsForTable(filtered, "customer")

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	// Validate branch_id tồn tại — FE có thể gửi stale branch_id từ localStorage
	if helpers.ToInt64(filtered["branch_id"]) > 0 {
		var branchExists int64
		tx.Table("branch").Where("id = ?", filtered["branch_id"]).Count(&branchExists)
		if branchExists == 0 {
			filtered["branch_id"] = nil
		}
	}

	// Gen code nếu có branch_id (beforeSave). TODO(story-59): codegen bỏ tham số company.
	if code := codegen.NewCustomerCode(tx, helpers.ToInt64(filtered["branch_id"])); code != "" {
		filtered["code"] = code
	}

	if err := tx.Table("customer").Create(filtered).Error; err != nil {
		tx.Rollback()
		respondCustomerWriteError(c, err)
		return
	}

	// Lấy ID vừa insert
	var customerID int64
	tx.Raw("SELECT LAST_INSERT_ID()").Scan(&customerID)
	if customerID == 0 {
		tx.Rollback()
		response.ServerError(c, fmt.Errorf("không lấy được ID khách hàng"))
		return
	}

	// Tạo appointment kèm theo nếu có appointment_date

	if apptDate != nil && helpers.ToInt64(apptDate) > 0 {
		apptStatus := define.AppointmentStatusNotCheckin
		var checkinDate interface{} = nil
		if helpers.ToInt64(apptDate) <= now {
			apptStatus = define.AppointmentStatusCheckedIn
			checkinDate = apptDate
		}
		// Nếu không có branch_id thì không thể tạo lịch hẹn
		if helpers.ToInt64(body["branch_id"]) == 0 {
			tx.Rollback()
			response.Validate(c, i18n.T(c, "branch_required_for_appointment"))
			return
		}
		appt := map[string]interface{}{
			"customer_id":            customerID,
			"branch_id":              body["branch_id"],
			"type_service_id":        body["type_service_id"],
			"revenue_id":             body["payment_user_id"],
			"source_id":              body["source_id"],
			"appointment_date":       apptDate,
			"appointment_date_first": apptDate,
			"schedule_time":          body["schedule_time"],
			"status":                 apptStatus,
			"checkin_date":           checkinDate,
			"appointment_type_id":    body["appointment_type_id"],
			"bookType":               body["bookType"],
			"content":                helpers.ToString(body["content"]),
			"contentPrivate":         helpers.ToString(body["contentPrivate"]),
			"appointment_number":     1,
			"created_at":             now,
			"updated_at":             now,
			"created_by":             uid,
			"updated_by":             uid,
		}
		// Gen code cho appointment (insert bằng map nên BeforeCreate hook không chạy).
		// TODO(story-59): codegen bỏ tham số company.
		{
			var apptBranchIDPtr *uint
			if v := helpers.ToInt64(body["branch_id"]); v > 0 {
				apptBranchIDPtr = new(uint(v))
			}
			code, err := codegen.GenerateErr(tx, "appointment", apptBranchIDPtr)
			if err != nil {
				tx.Rollback()
				response.HandleError(c, err)
				return
			}
			if code != "" {
				appt["code"] = code
			}
		}
		if err := tx.Table("appointment").Create(appt).Error; err != nil {
			tx.Rollback()
			response.ServerError(c, err)
			return
		}
	}

	// Tạo note nếu có
	if note != "" {
		noteRow := map[string]interface{}{
			"note":        note,
			"customer_id": customerID,
			"created_by":  uid,
			"created_at":  now,
		}
		if err := tx.Table("note").Create(noteRow).Error; err != nil {
			tx.Rollback()
			response.ServerError(c, err)
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		response.ServerError(c, err)
		return
	}

	// Đọc lại bản vừa tạo qua CÙNG phạm vi GetCustomer (skipBranch — khách tạo cho chi nhánh khác
	// chi nhánh đang active vẫn phải trả về được, không 500 sau khi đã commit).
	var item map[string]interface{}
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "customer", true).Table("customer").
		Where("id = ?", customerID).Take(&item).Error; err != nil {
		response.ServerError(c, err)
		return
	}
	helpers.ParseJSONFieldForTable(item, "customer")
	// Cột DECIMAL scan vào map[string]interface{} là []uint8 → parse để JSON ra number.
	helpers.ParseDecimalFieldsForTables([]map[string]interface{}{item}, "customer")
	// Mask SĐT như List/Get (user_option id=14): FE web dùng row trả về để cập nhật list,
	// nên user không có quyền xem số thật phải nhận bản mask ở đây y hệt khi reload list.
	phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), []map[string]interface{}{item})
	storage.ResolveFields(item, "avatar")
	response.Created(c, item)
}

// CreateCustomerSwitch POST /crm/customer/:id/create-customer-switch
// Clone customer cũ sang customer mới, reset fields marketing/financial
func (h *Handler) CreateCustomerSwitch(c *gin.Context) {
	oldID, ok := parseCustomerID(c)
	if !ok {
		return
	}

	// Scope theo company: không cho clone customer của clinic khác (chống IDOR)
	var old map[string]interface{}
	if err := tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Where("id = ?", oldID).Take(&old).Error; err != nil {
		response.NotFound(c)
		return
	}

	// Check mã cũ đã tồn tại chưa — chỉ trong phạm vi tenant hiện tại
	oldCode := helpers.ToString(old["code"])
	if oldCode != "" {
		var count int64
		tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
			Where("old_code = ?", oldCode).Count(&count)
		if count > 0 {
			response.Validate(c, i18n.T(c, "customer_already_exists"))
			return
		}
	}

	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.HandleError(c, err)
		return
	}
	// Nhân bản: avatar giữ nguyên của bản gốc (copy từ `old` ở dưới) hoặc rỗng — key khác bị bỏ qua.
	h.applyPayloadAvatar(body, helpers.ToString(old["avatar"]), true)

	// Clone từ old, override bằng body FE gửi
	new := make(map[string]interface{})
	for k, v := range old {
		new[k] = v
	}
	for k, v := range body {
		new[k] = v
	}

	now := time.Now().Unix()
	uid := int(middleware.GetUserID(c))

	// Reset fields
	delete(new, "id")
	new["code"] = nil
	new["old_code"] = oldCode
	new["connect_id"] = nil
	new["link_message"] = nil
	new["chat_message"] = nil
	new["loai_message"] = nil
	new["appointment_not_booked_reasons_id"] = nil
	new["totalRevenue"] = 0
	new["totalPayment"] = 0
	new["totalDebt"] = 0
	new["totalDeposit"] = 0
	new["totalBonus"] = 0
	new["refundDeposit"] = 0
	new["refundService"] = 0
	new["consignment"] = 0
	new["refundConsignment"] = 0
	new["customerType"] = define.CustomerTypeNew
	new["created_at"] = now
	new["updated_at"] = now
	new["created_by"] = uid
	new["updated_by"] = uid
	new["created_day"] = now

	// Auto status — khách mới (clone) LUÔN Contact(16) như CreateCustomer (D14); appointment_date
	// override như cũ.
	apptDate := new["appointment_date"]
	status := define.CustomerStatusContact
	if apptDate != nil && helpers.ToInt64(apptDate) > 0 {
		if helpers.ToInt64(apptDate) <= now {
			status = define.CustomerStatusVisited
		} else {
			status = define.CustomerStatusAppointment
		}
	}
	new["status"] = status

	// Duplicate check: cùng SĐT trong TOÀN DB (D-3) — xem ghi chú ở CreateCustomer. Bản clone giữ SĐT
	// của khách cũ ⇒ có SĐT là bị chặn (trước D-3 cũng đã bị chặn: clone giữ nguyên cả phone lẫn
	// fullname, khớp khoá phone+fullname cũ).
	if dup, err := h.customerPhoneTaken(helpers.ToString(new["phone"]), 0); err != nil {
		response.ServerError(c, err)
		return
	} else if dup {
		response.Validate(c, i18n.T(c, "customer_duplicate"))
		return
	}

	note := helpers.ToString(new["note"])
	delete(new, "note")
	delete(new, "appointment_date")

	filtered := h.CustomerSvc.FilterAllowed(new)
	if err := h.CustomerSvc.ValidateFromMap(filtered, true); err != nil {
		respondCustomerSvcError(c, err)
		return
	}
	if !customerBranchWriteAllowed(c, filtered) {
		response.Validate(c, i18n.T(c, "branch_id_not_allowed"))
		return
	}
	helpers.SerializeJSONFieldsForTable(filtered, "customer")

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	// Gen code nếu có branch_id (beforeSave). TODO(story-59): codegen bỏ tham số company.
	if code := codegen.NewCustomerCode(tx, helpers.ToInt64(filtered["branch_id"])); code != "" {
		filtered["code"] = code
	}

	if err := tx.Table("customer").Create(filtered).Error; err != nil {
		tx.Rollback()
		respondCustomerWriteError(c, err)
		return
	}

	var customerID int64
	tx.Raw("SELECT LAST_INSERT_ID()").Scan(&customerID)
	if customerID == 0 {
		tx.Rollback()
		response.ServerError(c, fmt.Errorf("không lấy được ID khách hàng"))
		return
	}

	// Tạo appointment kèm nếu có
	if apptDate != nil && helpers.ToInt64(apptDate) > 0 {
		apptStatus := define.AppointmentStatusNotCheckin
		var checkinDate interface{} = nil
		if helpers.ToInt64(apptDate) <= now {
			apptStatus = define.AppointmentStatusCheckedIn
			checkinDate = apptDate
		}
		appt := map[string]interface{}{
			"customer_id": customerID, "branch_id": new["branch_id"],
			"type_service_id": new["type_service_id"], "source_id": new["source_id"],
			"revenue_id":       new["payment_user_id"],
			"appointment_date": apptDate, "appointment_date_first": apptDate,
			"schedule_time": body["schedule_time"], "status": apptStatus,
			"checkin_date": checkinDate, "appointment_type_id": body["appointment_type_id"],
			"bookType": body["bookType"], "content": helpers.ToString(body["content"]),
			"contentPrivate":     helpers.ToString(body["contentPrivate"]),
			"appointment_number": 1,
			"created_at":         now, "updated_at": now, "created_by": uid, "updated_by": uid,
		}
		if err := tx.Table("appointment").Create(appt).Error; err != nil {
			tx.Rollback()
			response.ServerError(c, err)
			return
		}
	}

	if note != "" {
		noteRow := map[string]interface{}{
			"note": note, "parent_table": "customer", "parent_id": customerID,
			"customer_id": customerID,
			"created_by":  uid, "created_at": now, "updated_at": now,
		}
		if err := tx.Table("note").Create(noteRow).Error; err != nil {
			tx.Rollback()
			response.ServerError(c, err)
			return
		}
	}

	if err := tx.Commit().Error; err != nil {
		response.ServerError(c, err)
		return
	}

	// Đọc lại bản vừa tạo qua CÙNG phạm vi GetCustomer (skipBranch — khách tạo cho chi nhánh khác
	// chi nhánh đang active vẫn phải trả về được, không 500 sau khi đã commit).
	var item map[string]interface{}
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "customer", true).Table("customer").
		Where("id = ?", customerID).Take(&item).Error; err != nil {
		response.ServerError(c, err)
		return
	}
	helpers.ParseJSONFieldForTable(item, "customer")
	// Cột DECIMAL scan vào map[string]interface{} là []uint8 → parse để JSON ra number.
	helpers.ParseDecimalFieldsForTables([]map[string]interface{}{item}, "customer")
	// Mask SĐT như List/Get (user_option id=14): FE web dùng row trả về để cập nhật list,
	// nên user không có quyền xem số thật phải nhận bản mask ở đây y hệt khi reload list.
	phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), []map[string]interface{}{item})
	storage.ResolveFields(item, "avatar")
	response.Created(c, item)
}

// UpdateCustomer PUT /crm/customer/:id
func (h *Handler) UpdateCustomer(c *gin.Context) {
	id, ok := parseCustomerID(c)
	if !ok {
		return
	}

	// Check exists + get updated_at (khoá lạc quan D6) — cùng phạm vi chi nhánh với GetCustomer
	// (customerByIDScope) để chống IDOR xuyên chi nhánh.
	var current struct {
		UpdatedAt  int64  `gorm:"column:updated_at"`
		Phone      string `gorm:"column:phone"`
		OtherPhone string `gorm:"column:other_phone"`
		Avatar     string `gorm:"column:avatar"`
	}
	if err := h.customerByIDScope(c).Table("customer").
		Select("updated_at, phone, other_phone, avatar").Where("id = ?", id).First(&current).Error; err != nil {
		response.NotFound(c)
		return
	}

	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.HandleError(c, err)
		return
	}

	// Optimistic locking theo updated_at (D6) — bắt buộc để chống bypass
	v, ok := body["updated_at"]
	if !ok || v == nil {
		response.Validate(c, i18n.T(c, "version_missing"))
		return
	}
	expectedUpdatedAt := helpers.ToInt64(v)
	if expectedUpdatedAt != current.UpdatedAt {
		response.HandleError(c, errs.ErrVersionConflict)
		return
	}
	// Đặt SAU kiểm version: client cũ gửi URL ảnh đã bị đổi phải nhận 409, không phải 422.
	if err := h.applyPayloadAvatar(body, current.Avatar, false); err != nil {
		response.Validate(c, i18n.T(c, err.Error()))
		return
	}

	// Set audit fields
	now := time.Now().Unix()
	uid := int(middleware.GetUserID(c))
	delete(body, "updated_at") // callback optlock tự bump GREATEST(now, updated_at+1)
	body["updated_by"] = uid
	// Bất kỳ chỉnh sửa nào trên hồ sơ khách hàng = nhân sự đã có 1 lần tương tác
	// (đối chiếu thông tin, cập nhật sau cuộc gọi…) → ghi nhận last_contact_date.
	// FE có thể override bằng cách gửi explicit `last_contact_date`; nếu không gửi, BE auto-fill.
	if _, sent := body["last_contact_date"]; !sent {
		body["last_contact_date"] = now
	}

	// Nếu FE gửi lại masked phone (user không có quyền xem số thật), giữ nguyên số thật từ DB.
	// Pattern giống AiConfig.IsMaskedKey: compare exact thay vì heuristic check 'x'.
	if v, ok := body["phone"]; ok {
		if s, isStr := v.(string); isStr && s == phoneutil.MaskPhone(current.Phone) {
			delete(body, "phone")
		}
	}
	if v, ok := body["other_phone"]; ok {
		if s, isStr := v.(string); isStr && s == phoneutil.MaskPhone(current.OtherPhone) {
			delete(body, "other_phone")
		}
	}

	// Validate telesale cross-update permission trước khi đổi payment_user_id
	if err := validatePaymentUserIDChange(c, h.DB, body); err != nil {
		response.HandleError(c, err)
		return
	}

	// Remove fields không được update
	delete(body, "id")
	delete(body, "created_at")
	delete(body, "created_by")
	delete(body, "code")
	delete(body, "created_day")

	if !customerBranchWriteAllowed(c, body) {
		response.Validate(c, i18n.T(c, "branch_id_not_allowed"))
		return
	}

	item, err := h.CustomerSvc.Update(id, body, expectedUpdatedAt, uid)
	if err != nil {
		respondCustomerSvcError(c, err)
		return
	}

	helpers.ParseJSONFieldForTable(item, "customer")
	phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), []map[string]interface{}{item})
	storage.ResolveFields(item, "avatar")
	response.Updated(c, item)
}

// respondCustomerSvcError map lỗi từ CustomerSvc (Update/ValidateFromMap) ra HTTP status
// theo KIỂU lỗi, không theo message.
//
// KHÔNG dùng thẳng response.HandleError cho mọi lỗi: fallback cuối của nó là
// Validate(c, err.Error()) — echo raw error ra client, nên lỗi gorm/MySQL không lường
// trước sẽ lộ tên bảng/constraint nội bộ. Ngược lại cũng KHÔNG gộp hết về ServerError:
// version-conflict và lỗi nghiệp vụ/validate sẽ ra 500 generic, FE không phân biệt được.
//
//	errs.ErrVersionConflict     → 409 (giống nhánh version-check tiền kiểm ở UpdateCustomer)
//	*errs.LocalizedError        → 422 + message i18n (vd customer_status_invalid_transition)
//	validator.ValidationErrors  → 422 + message translate theo tag
//	*validation.Errors          → 422 + message đã user-facing (validateCustomerFromMap)
//	còn lại (DB/unknown)        → ServerError: MySQL code đã biết → 422, còn lại 500 generic + log
func respondCustomerSvcError(c *gin.Context, err error) {
	if errors.Is(err, errs.ErrVersionConflict) {
		response.HandleError(c, err) // → Conflict 409 qua i18n key "version_conflict"
		return
	}
	if _, ok := errors.AsType[*errs.LocalizedError](err); ok {
		response.HandleError(c, err) // → 422 với i18n.TF(key, args...)
		return
	}
	if ve, ok := errors.AsType[validator.ValidationErrors](err); ok {
		response.ValidateErr(c, ve)
		return
	}
	if verr, ok := errors.AsType[*validation.Errors](err); ok {
		response.Validate(c, verr.Error())
		return
	}
	respondCustomerWriteError(c, err)
}

// respondCustomerWriteError — lỗi ghi bảng customer: trùng SĐT (1062 trên UNIQUE SĐT, D-3) ⇒ 422
// "customer_duplicate" (cùng key bước kiểm trước); còn lại ⇒ ServerError như cũ.
func respondCustomerWriteError(c *gin.Context, err error) {
	if helpers.IsCustomerPhoneDuplicate(err) {
		response.Validate(c, i18n.T(c, "customer_duplicate"))
		return
	}
	response.ServerError(c, err)
}

// customerPhoneTaken — đã có khách (khác excludeID) mang SĐT này ở MỌI định dạng đã lưu
// (helpers.PhoneMatchCond). phone rỗng ⇒ false. Lỗi Count trả ra (D-3: trước đây bị bỏ qua ⇒
// count=0 ⇒ cho tạo trùng khi DB lỗi).
func (h *Handler) customerPhoneTaken(phone string, excludeID int) (bool, error) {
	if strings.TrimSpace(phone) == "" {
		return false, nil
	}
	var count int64
	phoneCond, phoneArgs := helpers.PhoneMatchCond("phone", phone, define.TelephoneCodeVN)
	q := h.DB.Table("customer").Where(phoneCond, phoneArgs...)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateAppointmentNotBookedReasons POST /crm/customer/:id/update-appointment-not-booked-reasons?appointmentNotBookedReasons=Y
func (h *Handler) UpdateAppointmentNotBookedReasons(c *gin.Context) {
	id, ok := parseCustomerID(c)
	if !ok {
		return
	}
	if !customerInScope(c, h.DB, id) {
		response.NotFound(c)
		return
	}
	val := nullableIntFromQuery(c.Query("appointmentNotBookedReasons"))
	if err := tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Where("id = ?", id).Update("appointment_not_booked_reasons_id", val).Error; err != nil {
		response.ServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// customerInScope — check customer tồn tại trong tenant scope của user.
// Tách check ra khỏi update để tránh nhầm "RowsAffected=0 do value không đổi" thành 404.
func customerInScope(c *gin.Context, db *gorm.DB, id int) bool {
	var cnt int64
	tenant.ScopedDBNullBranch(c, db, "customer").Table("customer").
		Where("id = ?", id).Count(&cnt)
	return cnt > 0
}

// nullableIntFromQuery — convert query string sang *int. FE có thể gửi "null"/"" để clear,
// hoặc số nguyên để set. Trả nil cho clear, *int cho value.
// Tránh bug "Incorrect integer value: 'null'" khi truyền raw string vào nullable INT column.
func nullableIntFromQuery(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" || s == "undefined" || s == "0" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

// AddTag POST /crm/customer/:id/add-tag?tag=Y
func (h *Handler) AddTag(c *gin.Context) {
	id, ok := parseCustomerID(c)
	if !ok {
		return
	}
	tag := c.Query("tag")
	if tag == "" {
		response.Validate(c, i18n.T(c, "id_and_tag_required"))
		return
	}

	var tagsJSON *string
	if err := tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Select("tags").Where("id = ?", id).Scan(&tagsJSON).Error; err != nil {
		response.NotFound(c)
		return
	}

	var tags []string
	if tagsJSON != nil {
		json.Unmarshal([]byte(*tagsJSON), &tags)
	}

	for _, t := range tags {
		if t == tag {
			c.JSON(http.StatusOK, tags)
			return
		}
	}
	tags = append(tags, tag)

	tagsBytes, _ := json.Marshal(tags)
	tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Where("id = ?", id).Update("tags", string(tagsBytes))
	c.JSON(http.StatusOK, tags)
}

// RemoveTag POST /crm/customer/:id/remove-tag?tag=Y
func (h *Handler) RemoveTag(c *gin.Context) {
	id, ok := parseCustomerID(c)
	if !ok {
		return
	}
	tag := c.Query("tag")
	if tag == "" {
		response.Validate(c, i18n.T(c, "id_and_tag_required"))
		return
	}

	var tagsJSON *string
	if err := tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Select("tags").Where("id = ?", id).Scan(&tagsJSON).Error; err != nil {
		response.NotFound(c)
		return
	}

	var tags []string
	if tagsJSON != nil {
		json.Unmarshal([]byte(*tagsJSON), &tags)
	}

	filtered := make([]string, 0, len(tags))
	for _, t := range tags {
		if t != tag {
			filtered = append(filtered, t)
		}
	}

	tagsBytes, _ := json.Marshal(filtered)
	tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Where("id = ?", id).Update("tags", string(tagsBytes))
	c.JSON(http.StatusOK, filtered)
}

// searchGlobalMinRunes — ngưỡng tối thiểu của ô tìm kiếm toàn cục, tính bằng KÝ TỰ (D112).
// Thấp hơn `loadByKeywordMinRunes` vì đây là ô tìm kiếm chủ động của người dùng (có `Limit(50)` và
// lọc lead), không phải autocomplete bắn theo từng phím của 24 màn hình.
const searchGlobalMinRunes = 2

// searchGlobalSelect / searchGlobalLimit — cột + trần dòng của search-global (topbar). Dùng chung
// cho đường cũ và đường tách UNION (list_branch_split.go) để hai đường không lệch cột/số dòng.
const (
	searchGlobalSelect = "id, fullname, nickname, code, phone"
	searchGlobalLimit  = 50
)

// SearchGlobal GET /crm/customer/search-global?keyword=X
// Endpoint duy nhất tôn trọng user_option id=18 (cross-branch):
// khi bật → bỏ branch filter để tìm khách toàn hệ thống.
func (h *Handler) SearchGlobal(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	// ⛔ D112 — ngưỡng tối thiểu đếm KÝ TỰ, KHÔNG đếm BYTE (cùng lớp lỗi với `loadByKeywordMinRunes`).
	// `len("Â")` = 2 byte ⇒ 1 ký tự CÓ DẤU lọt qua ngưỡng, trong khi `len("a")` = 1 byte thì bị chặn:
	// hành vi tuỳ thuộc chữ có dấu hay không, không ai đoán được. `utf8.RuneCountInString` cho đúng
	// thứ người dùng đếm.
	if utf8.RuneCountInString(keyword) < searchGlobalMinRunes {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	q, err := h.searchGlobalQuery(c, keyword)
	if err != nil {
		response.ServerError(c, err)
		return
	}

	var items []map[string]interface{}
	tx := q.Find(&items)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	phoneutil.MaskPhoneInItems(phoneutil.CanSeePhone(c, h.DB), items)
	c.JSON(http.StatusOK, items)
}

// searchGlobalQuery dựng câu search-global HOÀN CHỈNH (đã ORDER/LIMIT) cho keyword đã qua ngưỡng —
// tách khỏi SearchGlobal để test DryRun/parity gọi CHÍNH hàm sản phẩm. Lỗi = scope telesale lỗi.
func (h *Handler) searchGlobalQuery(c *gin.Context, keyword string) (*gorm.DB, error) {
	like := "%" + keyword + "%"

	q := h.DB.Table("customer").Select(searchGlobalSelect)

	// Branch scope: bỏ qua khi user_option id=18 (cross-branch) bật. Story-36 (D15): bỏ vế company.
	// Ngoặc tường minh: GORM chỉ tự bọc vế OR khi WHERE có ≥2 vế — không dựa vào đó.
	// splitBranch: phạm vi rút về ĐÚNG 1 chi nhánh (+ NULL) ⇒ câu cuối đi đường tách UNION ALL
	// (bench C-4 vòng 2, bảng plan ở list_branch_split.go §search-global). Nhiều chi nhánh /
	// cross-branch / không giới hạn ⇒ splitBranch = 0 ⇒ SQL y hệt trước bản vá.
	var splitBranch uint
	if !tenant.IsCrossBranchEnabled(c, h.DB) {
		if branchActiveID := middleware.GetBranchActiveID(c); branchActiveID > 0 {
			q = q.Where("(customer.branch_id = ? OR customer.branch_id IS NULL)", branchActiveID)
			splitBranch = branchActiveID
		} else if branchIDs := middleware.GetBranchIDs(c); len(branchIDs) > 0 {
			q = q.Where("(customer.branch_id IN ? OR customer.branch_id IS NULL)", branchIDs)
			if len(branchIDs) == 1 {
				splitBranch = branchIDs[0]
			}
		}
	}

	// Phone condition: match 9 chữ số cuối khi input là SĐT (10-11 số)
	//
	// ⚠ CỐ Ý đếm BYTE ở đây, KHÁC ngưỡng phía trên (D112): `isNumeric(keyword)` bảo đảm chuỗi toàn
	// chữ số ASCII ⇒ byte = ký tự, hai cách đếm cho cùng kết quả. Quan trọng hơn: `keyword[len(...)-9:]`
	// là phép cắt theo BYTE — đổi vế trái sang đếm rune mà vế cắt vẫn theo byte là mở đường cho lệch
	// chỉ số. Giữ nguyên cả cụm theo byte.
	phoneCond := "phone LIKE ? OR other_phone LIKE ?"
	phoneArgs := []interface{}{like, like}
	if len(keyword) >= 10 && len(keyword) <= 11 && isNumeric(keyword) {
		phoneFragment := "%" + keyword[len(keyword)-9:] + "%"
		phoneArgs = []interface{}{phoneFragment, phoneFragment}
	}

	q = q.Where("fullname LIKE ? OR code LIKE ? OR old_code LIKE ? OR nickname LIKE ? OR "+phoneCond,
		append([]interface{}{like, like, like, like}, phoneArgs...)...)

	// Kết quả search bấm vào là điều hướng thẳng sang CustomerDetail (`app.topbar.ts`),
	// nên phải lọc cùng lớp với GetCustomer — nếu không, telesale tìm thấy khách
	// của người khác rồi bấm vào nhận 404 (và tên/mã khách bị lộ ngay ở kết quả tìm kiếm).
	q, err := applyTelesaleScope(c, h.DB, q)
	if err != nil {
		return nil, err
	}

	if splitBranch > 0 {
		q = customerSplitByBranch(h.DB, q, splitBranch, 0, searchGlobalLimit, searchGlobalSelect)
	} else {
		q = q.Order("id DESC").Limit(searchGlobalLimit)
	}
	return q, nil
}

// loadByKeywordMinRunes — ngưỡng tìm kiếm rộng (LIKE %…%) tính bằng KÝ TỰ, KHÔNG bằng byte.
//
// ⛔ story-35 AC3 (D109): `len(keyword)` đếm BYTE. Với tiếng Việt UTF-8, "Ân" là 2 ký tự nhưng 3
// byte ⇒ LỌT ngưỡng, trong khi "An" (2 ký tự, 2 byte) thì KHÔNG — ngưỡng tuỳ hứng theo chuyện chữ
// có dấu hay không, không ai đoán được. `utf8.RuneCountInString` cho đúng thứ người dùng đếm.
//
// ⚠ ĐỔI HÀNH VI CÓ CHỦ ĐÍCH cho 24 chỗ gọi: 2 ký tự tiếng Việt CÓ DẤU trước đây tìm được theo tên
// thì nay rơi xuống nhánh hẹp (`code` chính xác / `phone` theo tiền tố), y hệt 2 ký tự không dấu.
// Đây là NHẤT QUÁN, không phải hồi quy — nhánh hẹp tồn tại để chặn full-scan bảng khách hàng.
const loadByKeywordMinRunes = 3

// D110 — số dòng trả về của `load-by-keyword`.
//
//	Default: giữ ĐÚNG con số cũ (10). 24 chỗ gọi không truyền `limit` phải nhận y hệt hôm nay.
//	Max:     trần cứng. Đây là autocomplete gõ-tới-đâu-bắn-tới-đó, không phải màn hình danh sách
//	         (danh sách đã có `GET /crm/customer` + `pagination.Paginate`). Không có trần thì
//	         `?limit=100000` biến nó thành cửa quét cả bảng khách hàng qua LIKE '%…%'.
const (
	loadByKeywordDefaultLimit = 10
	loadByKeywordMaxLimit     = 50
)

// hasWithField — đọc query `?with=a,b,c` (danh sách field OPT-IN, phân tách bằng dấu phẩy).
//
// Vì sao opt-in chứ không trả hết: endpoint dùng chung cho nhiều màn hình, thêm một field vào
// payload mặc định là phát tán nó tới MỌI chỗ gọi (story-35 AC4 — `birthday` của story-34).
func hasWithField(c *gin.Context, field string) bool {
	for _, f := range strings.Split(c.Query("with"), ",") {
		if strings.TrimSpace(f) == field {
			return true
		}
	}
	return false
}

func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// LoadCustomerByKeyword GET /crm/customer/load-by-keyword?keyword=X[&with=birthday][&limit=N][&with_total=1]
//
// ⚠ Endpoint DÙNG CHUNG cho 24 chỗ gọi ở FE (mọi autocomplete khách hàng). Các luật của nó:
//
//	· HÌNH DẠNG response mặc định là MẢNG TRẦN và phải giữ nguyên như vậy (D110). `total` chỉ được
//	  gói kèm khi người gọi hỏi `?with_total=1` → `{items, total}`. Lý do chọn cờ riêng thay vì đổi
//	  shape vô điều kiện: 24 chỗ gọi đọc thẳng mảng, đổi hết một lượt thì TypeScript ở FE không
//	  bắt được (kiểu trả về là `any`) — hỏng lúc chạy, ở 24 màn hình cùng lúc.
//	· `total` để FE biết CÓ THẬT còn khách khác hay không. Trước D110, teamchat đoán bằng
//	  `len(rows) >= 10` — đúng 10 dòng có thể là "vừa hết đúng 10", nên dòng "Xem tất cả" hiện thừa.
//	· NGƯỠNG đếm bằng KÝ TỰ, không phải byte (story-35 AC3) — xem `loadByKeywordMinRunes`.
//	· Field nặng/nhạy cảm là OPT-IN qua `?with=` (story-35 AC4) — mặc định payload KHÔNG có chúng.
//	  `birthday` (story-34) chỉ đúng một nơi cần: hộp thoại chọn khách của teamchat, để phân biệt
//	  hai khách trùng tên. 23 chỗ gọi còn lại không dùng tới mà vẫn nhận ⇒ phát tán ngày sinh
//	  không vì lý do gì. Khác `phone` — SĐT có cửa quyền riêng `phoneutil.CanSeePhone`; `birthday`
//	  không có cổng che nào, nên cách đóng đúng là không gửi khi không ai hỏi.
func (h *Handler) LoadCustomerByKeyword(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	withBirthday := hasWithField(c, "birthday")

	// D110 — `limit` là TUỲ CHỌN, mặc định giữ đúng 10 như trước. Trần cứng để không ai biến
	// autocomplete thành cửa quét cả bảng khách hàng.
	limit := loadByKeywordDefaultLimit
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 {
		limit = n
		if limit > loadByKeywordMaxLimit {
			limit = loadByKeywordMaxLimit
		}
	}
	// D110 — `with_total=1` mới đổi HÌNH DẠNG response sang `{items, total}`. Mặc định vẫn là MẢNG
	// TRẦN: endpoint này có 24 chỗ gọi (FE + mobile) đọc thẳng mảng, đổi shape vô điều kiện là làm
	// hỏng cả 24 chỗ cùng lúc mà build vẫn xanh.
	withTotal := c.Query("with_total") == "1" || c.Query("with_total") == "true"

	// Điều kiện lọc dùng CHUNG cho câu lấy dòng và câu COUNT — viết một lần, không chép: hai bản
	// chép sẽ trôi lệch và `total` sẽ đếm một tập khác tập đang hiển thị, im lặng.
	applyKeyword := func(q *gorm.DB) *gorm.DB {
		if utf8.RuneCountInString(keyword) >= loadByKeywordMinRunes {
			like := "%" + keyword + "%"
			// 3 columns only (7-column OR defeats all indexes; phone uses prefix for partial index benefit)
			return q.Where("fullname LIKE ? OR code LIKE ? OR phone LIKE ?", like, like, keyword+"%")
		}
		if keyword != "" {
			// < 3 chars: exact code or phone prefix only (avoids full scan)
			return q.Where("code = ? OR phone LIKE ?", keyword, keyword+"%")
		}
		return q
	}

	cols := "id, code, phone, fullname, nickname"
	if withBirthday {
		cols += ", DATE_FORMAT(birthday, '%Y-%m-%d') as birthday"
	}
	// S-10: phạm vi telesale (khách mình phụ trách) — autocomplete không được là cửa liệt kê
	// khách của người khác. Áp trên CẢ câu lấy dòng lẫn câu COUNT.
	scopedBase := func() (*gorm.DB, error) {
		return applyTelesaleScope(c, h.DB, applyKeyword(tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer")))
	}
	q, err := scopedBase()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	q = q.Select(cols).Limit(limit)
	var rows []map[string]any
	tx := q.Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	// COUNT trên CÙNG bộ điều kiện, KHÔNG `Limit`. Dựng từ một `ScopedDBNullBranch` MỚI chứ không
	// tái dùng `q` — `q` đã mang `Select`/`Limit`, và tái dùng một `*gorm.DB` đã chạy là bẫy đã có
	// tiền lệ trong repo này.
	var total int64
	if withTotal {
		ctx, err := scopedBase()
		if err != nil {
			response.ServerError(c, err)
			return
		}
		if ctx2 := ctx.Count(&total); pagination.IsQueryError(ctx2.Error) {
			pagination.FailQuery(c, ctx2.Error)
			return
		}
	}

	canSeePhone := phoneutil.CanSeePhone(c, h.DB)
	result := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		fullname, _ := r["fullname"].(string)
		nickname, _ := r["nickname"].(string)
		label := strings.TrimSpace(fullname)
		if fullname != "" && nickname != "" {
			label = fullname + " - " + nickname
		} else if nickname != "" {
			label = nickname
		}
		code, _ := r["code"].(string)
		phone, _ := r["phone"].(string)
		if !canSeePhone {
			phone = phoneutil.MaskPhone(phone)
		}
		// Story-36 (D15): bỏ khoá company_id khỏi payload (1 thương hiệu/DB).
		row := gin.H{"id": r["id"], "code": code, "phone": phone, "fullname": label}
		if withBirthday {
			// Chỉ có mặt khi người gọi HỎI — 23 chỗ gọi còn lại giữ nguyên hình dạng payload cũ.
			birthday, _ := r["birthday"].(string)
			row["birthday"] = birthday
		}
		result = append(result, row)
	}
	if withTotal {
		c.JSON(http.StatusOK, gin.H{"items": result, "total": total})
		return
	}
	c.JSON(http.StatusOK, result)
}

// CreateAvatar POST /crm/customer/:id/create-avatar
func (h *Handler) CreateAvatar(c *gin.Context) {
	id, ok := parseCustomerID(c)
	if !ok {
		return
	}

	var customer map[string]any
	tx := tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Select("id, code, avatar").Where("id = ?", id).Scan(&customer)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	if customer == nil {
		response.Validate(c, i18n.T(c, "customer_not_found"))
		return
	}
	code, _ := customer["code"].(string)
	oldAvatar, _ := customer["avatar"].(string)
	// Dùng id làm fallback khi chưa có mã (Lead chưa có chi nhánh)
	s3Key := code
	if s3Key == "" {
		s3Key = fmt.Sprintf("tmp_%d", id)
	}

	file, err := c.FormFile("fileImage")
	if response.IsPayloadTooLarge(err) {
		response.PayloadTooLarge(c)
		return
	}
	if err != nil || file == nil {
		response.Validate(c, i18n.T(c, "image_required"))
		return
	}

	if h.Store == nil {
		response.Validate(c, i18n.T(c, "s3_not_configured"))
		return
	}

	// Crop center 500×500, target ~100KB webp (FE cho upload tới 10MB — nén lại lúc lưu trữ).
	// SkipBelowKB: -1 (never skip) — avatar CẦN luôn crop vuông 500×500 đồng nhất, khác ảnh
	// thường (banner/logo) chấp nhận passthrough khi ảnh gốc đã nhỏ. Điện thoại đời mới nén
	// JPEG rất hiệu quả (ảnh 4000×3000 có thể ≤200KB) — không set -1 sẽ bỏ qua crop, lưu ảnh
	// full-resolution không vuông, không đồng nhất với các avatar đã qua xử lý.
	// KHÔNG dùng imaging.OptFill500() (TargetKB=100, SkipBelowKB mặc định 200) — preset dùng chung,
	// đổi chung sẽ ảnh hưởng ngoài scope.
	key, err := imaging.ProcessAndUpload(c.Request.Context(), h.Store, file, "customer/"+s3Key+"/avatar/", &imaging.Options{
		Mode: imaging.ModeFill, MaxWidth: 500, MaxHeight: 500, TargetKB: 100, Sharpen: true, SkipBelowKB: -1,
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}

	// Ghi DB lỗi ⇒ DB vẫn trỏ ảnh cũ: dọn ảnh MỚI vừa upload (không để mồ côi) và TUYỆT ĐỐI không
	// xoá ảnh cũ — trước đây bỏ qua `.Error` rồi xoá ảnh cũ ⇒ avatar khách hỏng vĩnh viễn.
	if err := tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Where("id = ?", id).Updates(map[string]any{
		"avatar":     key,
		"updated_at": time.Now().Unix(),
	}).Error; err != nil {
		imaging.CleanupNewOnError(h.Store, oldAvatar, key, customerAvatarPrefixes(code, id)...)
		response.ServerError(c, err)
		return
	}
	imaging.DeleteOldIfChanged(h.Store, oldAvatar, key, customerAvatarPrefixes(code, id)...)
	response.CreatedDataWithMessage(c, gin.H{"avatar": storage.PublicURL(h.Store, key)}, i18n.T(c, "created"))
}

// applyPayloadAvatar — chính sách chuỗi `avatar` trong payload (2026-10-19). Ảnh mới CHỈ đến từ upload
// (POST /:id/create-avatar); trước đây nhận mọi key ⇒ nhân viên gán key ảnh/logo người khác (hoặc
// `private/...`) cho khách rồi upload avatar mới là CreateAvatar xoá file của người khác.
//
//   - TẠO (isCreate): rỗng ⇒ ""; trùng `current` (ảnh bản gốc khi nhân bản) ⇒ giữ dạng KEY; khác ⇒
//     BỎ key khỏi body (bỏ qua, không lỗi).
//   - SỬA: imaging.ResolvePayloadImage — rỗng (xoá) hoặc trùng ảnh hiện tại, khác ⇒
//     ErrImageKeyNotAllowed (handler trả 422).
func (h *Handler) applyPayloadAvatar(body map[string]interface{}, current string, isCreate bool) error {
	v, ok := body["avatar"]
	if !ok {
		return nil
	}
	resolved, err := imaging.ResolvePayloadImage(h.Store, current, helpers.ToString(v))
	if err != nil {
		if isCreate {
			delete(body, "avatar")
			return nil
		}
		return err
	}
	body["avatar"] = resolved
	return nil
}

// customerAvatarPrefixes — prefix upload avatar của khách (theo mã, hoặc tmp_<id> khi chưa có mã);
// cũng là phạm vi DUY NHẤT được xoá ảnh cũ/dọn file.
func customerAvatarPrefixes(code string, id int) []string {
	p := []string{fmt.Sprintf("customer/tmp_%d/avatar/", id)}
	if code != "" {
		p = append(p, "customer/"+code+"/avatar/")
	}
	return p
}

// GetNumberCustomerByPhone GET /crm/customer/get-number-customer-by-phone
// Tìm khách hàng theo số điện thoại, trả về số lượng + danh sách
func (h *Handler) GetNumberCustomerByPhone(c *gin.Context) {
	phone := strings.TrimSpace(c.Query("phone"))
	if phone == "" {
		c.JSON(http.StatusOK, make([]interface{}, 0))
		return
	}
	if len(phone) > 9 {
		phone = phone[len(phone)-9:]
	}

	var data []struct {
		ID            uint   `json:"id"`
		Fullname      string `json:"fullname"`
		Nickname      string `json:"nickname"`
		Phone         string `json:"phone"`
		PaymentUserID *int   `json:"payment_user_id"`
		CreatedBy     *int   `json:"created_by"`
		CreatedAt     *int64 `json:"created_at"`
	}

	q := tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Select("id, fullname, nickname, phone, payment_user_id, created_by, created_at").
		Where("phone LIKE ?", "%"+phone+"%").
		Order("id DESC").
		Limit(20)
	q.Find(&data)

	if !phoneutil.CanSeePhone(c, h.DB) {
		for i := range data {
			data[i].Phone = phoneutil.MaskPhone(data[i].Phone)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"num":  len(data),
		"data": data,
	})
}

// GetNumberCustomerByOldCode GET /crm/customer/get-number-customer-by-old-code
// Cảnh báo trùng mã khách hàng cũ khi tạo mới — không chặn, chỉ để FE hiển thị popup xác nhận.
func (h *Handler) GetNumberCustomerByOldCode(c *gin.Context) {
	oldCode := strings.TrimSpace(c.Query("old_code"))
	if oldCode == "" {
		c.JSON(http.StatusOK, make([]interface{}, 0))
		return
	}

	var data []struct {
		ID            uint   `json:"id"`
		Fullname      string `json:"fullname"`
		Nickname      string `json:"nickname"`
		Phone         string `json:"phone"`
		OldCode       string `json:"old_code"`
		PaymentUserID *int   `json:"payment_user_id"`
		CreatedBy     *int   `json:"created_by"`
		CreatedAt     *int64 `json:"created_at"`
	}

	q := tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Select("id, fullname, nickname, phone, old_code, payment_user_id, created_by, created_at").
		Where("old_code = ?", oldCode).
		Order("id DESC").
		Limit(20)
	q.Find(&data)

	if !phoneutil.CanSeePhone(c, h.DB) {
		for i := range data {
			data[i].Phone = phoneutil.MaskPhone(data[i].Phone)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"num":  len(data),
		"data": data,
	})
}

// DeleteCustomer DELETE /crm/customer/:id
func (h *Handler) DeleteCustomer(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}

	// Kiểm tra customer tồn tại trong company hiện tại (chống IDOR).
	// Lấy luôn ảnh chụp định danh + độ lớn tài chính để ghi audit — PHẢI đọc TRƯỚC khi
	// xoá, vì sau đó `customer` không còn (xem deleteAuditSnapshot).
	snap, found, snapErr := h.deleteAuditSnapshot(c, id)
	if pagination.IsQueryError(snapErr) {
		pagination.FailQuery(c, snapErr)
		return
	}
	if !found {
		response.NotFound(c)
		return
	}

	// Khách đã có đơn hàng / phiếu thu-chi ⇒ chặn (LocalizedError 422) — xem crmServices.DeleteCustomer.
	if err := crmServices.DeleteCustomer(h.DB, id, int(middleware.GetUserID(c))); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c)
			return
		}
		// LocalizedError (lỗi nghiệp vụ có key i18n) ⇒ 422. Chỉ LocalizedError
		// mới qua HandleError — lỗi hạ tầng qua đó sẽ thành 422 lộ nguyên chuỗi SQL.
		var le *errs.LocalizedError
		if errors.As(err, &le) {
			response.HandleError(c, err)
			return
		}
		// FK 1451 → 422 fk_constraint; infra errors → 500 (response.ServerError handles both)
		response.ServerError(c, err)
		return
	}

	// Audit SAU khi transaction commit thành công — đặt trước đó thì rollback vẫn để lại
	// dòng audit của một lần xoá KHÔNG xảy ra. Đường này dùng chung web + mobile
	// (mobile delegate handler này) nên 1 chỗ ghi là phủ cả 2 kênh.
	seclog.Event(c, evCustomerDeleted,
		zap.Uint("customer_id", id),
		zap.String("code", snap.Code),
		zap.String("fullname", snap.Fullname),
		zap.Int("status", snap.Status),
		zap.String("total_revenue", snap.TotalRevenue),
		zap.String("total_payment", snap.TotalPayment),
		zap.Int64("orders_deleted", snap.Orders),
		zap.Int64("payments_deleted", snap.Payments),
		zap.Int64("appointments_deleted", snap.Appointments),
	)

	response.Deleted(c)
}

// evCustomerDeleted — tên event trong `security_audit_log`. Giữ ≤ seclog.MaxEventLen.
const evCustomerDeleted = "customer_deleted"

// customerDeleteSnapshot — ảnh chụp trước khi xoá, dùng dựng lại "cái gì đã mất".
// Tiền để dạng string: chỉ để đọc lại trong audit, KHÔNG tính toán (Hard Rule §Tiền).
type customerDeleteSnapshot struct {
	Code         string
	Fullname     string
	Status       int
	TotalRevenue string
	TotalPayment string
	Orders       int64
	Payments     int64
	Appointments int64
}

// deleteAuditSnapshot đọc định danh + độ lớn thiệt hại của customer TRƯỚC khi xoá.
//
// Vì sao cần: DeleteWithCleanup là hard-delete cascade — sau khi chạy thì `customer` biến mất;
// `log_customer` chỉ giữ 1 dòng xoá với snapshot định danh, KHÔNG có độ lớn tài chính (đã mất
// bao nhiêu đơn/thanh toán/doanh thu). Ảnh chụp này là nguồn duy nhất của phần đó, nằm trong
// `security_audit_log`.
//
// Kiêm luôn vai trò check tồn tại + tenant scope (chống IDOR) — trả found=false thay cho
// Count cũ, không thêm round-trip nào so với trước.
func (h *Handler) deleteAuditSnapshot(c *gin.Context, id uint) (customerDeleteSnapshot, bool, error) {
	var snap customerDeleteSnapshot
	var row map[string]interface{}
	tx := tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Select("customer.code, customer.fullname, customer.status, customer.totalRevenue, customer.totalPayment").
		Where("customer.id = ?", id).
		Scan(&row)
	if tx.Error != nil {
		return snap, false, tx.Error
	}
	if len(row) == 0 {
		return snap, false, nil
	}
	str := func(k string) string {
		switch v := row[k].(type) {
		case nil:
			return ""
		case string:
			return v
		case []byte:
			return string(v)
		default:
			return fmt.Sprintf("%v", v)
		}
	}
	snap.Code, snap.Fullname = str("code"), str("fullname")
	snap.TotalRevenue, snap.TotalPayment = str("totalRevenue"), str("totalPayment")
	// helpers.ToInt64 chứ KHÔNG type-assert `.(int64)`: map-scan trả kiểu tuỳ
	// driver/ScanType (int32, uint64, []byte…), assert cứng sẽ im lặng ra 0.
	snap.Status = int(helpers.ToInt64(row["status"]))

	// Đếm 4 nhóm con có giá trị nghiệp vụ cao nhất. KHÔNG đếm cả 32 bảng — mục tiêu là
	// biết "mất bao nhiêu", không phải kiểm kê từng bảng. Lỗi đếm KHÔNG chặn xoá: audit
	// thiếu 1 con số vẫn hơn là chặn thao tác user đã xác nhận.
	counts := []struct {
		table, col string
		dst        *int64
	}{
		{"order_service_used", "customer_id", &snap.Orders},
		{"appointment", "customer_id", &snap.Appointments},
	}
	for _, cnt := range counts {
		h.DB.Table(cnt.table).Where(cnt.col+" = ?", id).Count(cnt.dst)
	}
	h.DB.Table("payment").
		Where("serviceUsed_id IN (SELECT id FROM order_service_used WHERE customer_id = ?)", id).
		Count(&snap.Payments)

	return snap, true, nil
}

// parseCustomerID parse path param `id` ra số dương; không hợp lệ ⇒ 404 và trả false.
// (Chuyển từ clv.go khi bỏ tính năng CLV 2026-10-17 — vẫn dùng chung cho các handler customer.)
func parseCustomerID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		response.NotFound(c)
		return 0, false
	}
	return id, true
}
