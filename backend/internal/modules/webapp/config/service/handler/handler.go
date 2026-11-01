package handler

import (
	"erpCluvix/pkg/cache"

	"gorm.io/gorm"

	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"erpCluvix/internal/middleware"
	"erpCluvix/internal/models"
	authSvc "erpCluvix/internal/modules/webapp/auth/rbac/service"
	configServiceDao "erpCluvix/internal/modules/webapp/config/service/dao"
	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/pkg/codegen"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/dexie"
	"erpCluvix/pkg/errs"
	pkgExport "erpCluvix/pkg/export"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/optlock"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// Deps + Handler — module-local (Plan 06). Không còn Store: cột ảnh dịch vụ đã gỡ 2026-10-19.
// RbacSvc: guard quyền tường minh export/import (module config chỉ có JWTAuth, không có
// PermissionEnforcer toàn cục đang audit-mode → tự check quyền manageConfig/super trong handler).
type Deps struct {
	DB      *gorm.DB
	Cache   *cache.RedisCache
	RbacSvc *authSvc.RbacService
}

type Handler struct {
	Deps
}

func New(d Deps) *Handler { return &Handler{Deps: d} }

// errCommissionOutOfRange — hoa hồng âm, hoặc vượt 100% khi type=percent.
var errCommissionOutOfRange = errors.New("commission_out_of_range")

// serviceDiscountTypePercent — giá trị commission_*_type khi tính theo % (khớp FE
// SERVICE_DISCOUNT_TYPE_SELECT.SERVICE_DISCOUNT_TYPE_PERCENT, service.define.ts).
const serviceDiscountTypePercent = 2

// commissionFieldPairs — (field giá trị, field loại) của hoa hồng tư vấn viên (telesale) — dùng để validate
// range ở server. [min]/[max] trên p-inputnumber (FE) chỉ ràng buộc UI, client gọi thẳng API
// vẫn bypass được — chặn lại ở đây trước khi ghi DB.
var commissionFieldPairs = [][2]string{
	{"commission_telesale", "commission_telesale_type"},
	{"commission_telesale_new", "commission_telesale_type"},
}

// validateCommissionRangeMap validate map-based payload (multipart create + update path).
// Field vắng mặt trong map (partial update) bỏ qua — chỉ check field THỰC SỰ có trong request.
func validateCommissionRangeMap(updates map[string]interface{}) error {
	hundred := decimal.NewFromInt(100)
	for _, pair := range commissionFieldPairs {
		valField, typeField := pair[0], pair[1]
		raw, ok := updates[valField]
		if !ok || raw == nil {
			continue
		}
		val := helpers.ToDecimal(raw)
		if val.IsNegative() {
			return errCommissionOutOfRange
		}
		if typeRaw, hasType := updates[typeField]; hasType {
			if int(helpers.ToInt64(typeRaw)) == serviceDiscountTypePercent && val.GreaterThan(hundred) {
				return errCommissionOutOfRange
			}
		}
	}
	return nil
}

// validateCommissionRangeStruct validate path JSON struct binding (CreateService).
func validateCommissionRangeStruct(item *configServiceDao.Service) error {
	hundred := decimal.NewFromInt(100)
	pairs := []struct {
		val *decimal.Decimal
		typ *int
	}{
		{item.CommissionTelesale, item.CommissionTelesaleType},
		{item.CommissionTelesaleNew, item.CommissionTelesaleType},
	}
	for _, p := range pairs {
		if p.val == nil {
			continue
		}
		if p.val.IsNegative() {
			return errCommissionOutOfRange
		}
		if p.typ != nil && *p.typ == serviceDiscountTypePercent && p.val.GreaterThan(hundred) {
			return errCommissionOutOfRange
		}
	}
	return nil
}

// errCatalogNotFound / errCatalogAmbiguous — kết quả tra danh mục theo TÊN khi import.
// Ambiguous phải là LỖI chứ không phải "lấy cái đầu tiên": 2 nhóm dịch vụ trùng tên mà chọn
// bừa thì dịch vụ gắn sai danh mục mà không ai phát hiện.
var (
	errCatalogNotFound  = errors.New("catalog_not_found")
	errCatalogAmbiguous = errors.New("catalog_ambiguous")
)

// catalogRefError — bọc lỗi tra danh mục kèm tên cột + chuỗi user gõ, để thông báo dòng nói đúng
// chỗ sai thay vì một câu chung chung cho cả 4 cột danh mục.
type catalogRefError struct {
	col string
	raw string
	err error
}

func (e catalogRefError) Error() string { return e.err.Error() }
func (e catalogRefError) Unwrap() error { return e.err }

// catalogImportErrMsg — thông báo lỗi dòng cho ô danh mục.
func catalogImportErrMsg(c *gin.Context, err error) string {
	var ce catalogRefError
	if !errors.As(err, &ce) {
		return err.Error()
	}
	switch {
	case errors.Is(ce.err, errCatalogAmbiguous):
		return i18n.TF(c, "import_option_ambiguous", ce.col, ce.raw)
	case errors.Is(ce.err, errCatalogNotFound):
		return i18n.TF(c, "import_option_not_found", ce.col, ce.raw)
	default:
		return ce.err.Error()
	}
}

// lookupCatalogIDByName tra id của một mục danh mục theo TÊN, trong toàn danh mục đang bật của DB
// (story-39, D15: không còn phạm vi company — 2 mục trùng tên ở 2 thương hiệu cũ nay là mơ hồ thật).
// So khớp không phân biệt hoa thường/khoảng trắng thừa — user gõ tay hiếm khi khớp từng ký tự.
func lookupCatalogIDByName(tx *gorm.DB, src exportHandlers.CatalogSource, name string) (int64, error) {
	var ids []int64
	q := src.Apply(tx.Table(src.Table)).
		Where("LOWER(TRIM(name)) = ?", strings.ToLower(strings.TrimSpace(name)))
	if err := q.Order("id ASC").Limit(2).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	switch len(ids) {
	case 0:
		return 0, errCatalogNotFound
	case 1:
		return ids[0], nil
	default:
		return 0, errCatalogAmbiguous
	}
}

// catalogValueUnchanged — ô danh mục có ghi lại ĐÚNG giá trị bản ghi đang giữ không.
// cur == nil (tạo mới) → luôn coi là có đổi, phải kiểm.
func catalogValueUnchanged(col string, cur *configServiceDao.Service, id int64) bool {
	if cur == nil {
		return false
	}
	switch col {
	case "service_type_id":
		return cur.ServiceTypeId != nil && int64(*cur.ServiceTypeId) == id
	case "unit_id":
		return cur.UnitId != nil && int64(*cur.UnitId) == id
	}
	return false
}

// catalogIDBelongs — id danh mục có tồn tại (đang bật) trong danh mục không. Dùng đúng
// CatalogSource.Apply như lúc tra theo tên, nên luật lọc chỉ có một nguồn sự thật cho cả hai đường.
func catalogIDBelongs(tx *gorm.DB, src exportHandlers.CatalogSource, id int64) (bool, error) {
	var ids []int64
	if err := src.Apply(tx.Table(src.Table)).
		Where("id = ?", id).Limit(1).Pluck("id", &ids).Error; err != nil {
		return false, err
	}
	return len(ids) == 1, nil
}

// normalizeFormPayload chuẩn hoá payload dạng map trước khi ghi DB — DÙNG CHUNG cho cả create và
// update, vì `toFormData` (FE `share/services/utils.service.ts`) serialize object → key lồng
// (`installment_config[periods]`, `installment_config[interest_rate]`...).
//
// Trước đây chỉ `UpdateService` gom key lồng, `CreateService` thì không → tạo dịch vụ qua multipart
// sinh `INSERT ... installment_config[periods]` = cột không tồn tại → Error 1054, luôn 500.
func normalizeFormPayload(updates map[string]interface{}) {
	// Gom nested form keys (installment_config[xxx]) thành JSON object
	for _, prefix := range []string{"installment_config"} {
		nested := map[string]interface{}{}
		var toDelete []string
		for k, v := range updates {
			if strings.HasPrefix(k, prefix+"[") && strings.HasSuffix(k, "]") {
				field := k[len(prefix)+1 : len(k)-1]
				nested[field] = v
				toDelete = append(toDelete, k)
			}
		}
		for _, k := range toDelete {
			delete(updates, k)
		}
		if len(nested) > 0 {
			b, _ := json.Marshal(nested)
			updates[prefix] = string(b)
		} else if v, ok := updates[prefix]; ok {
			switch v.(type) {
			case map[string]interface{}, []interface{}:
				b, _ := json.Marshal(v)
				updates[prefix] = string(b)
			}
		}
	}
}

// dropEmptyStringsForCreate bỏ các key có giá trị chuỗi rỗng khỏi payload CREATE multipart.
// `toFormData` map `null` → `""` (utils.service.ts), nên field số/ngày chưa nhập tới BE là `""` →
// `INSERT INTO service (time_reminder) VALUES (”)` fail khi MySQL bật strict mode (Error 1366).
// Bỏ hẳn key để cột nhận DEFAULT của schema — đúng ngữ nghĩa "chưa nhập" hơn là ép NULL (nhiều cột
// NOT NULL có default).
//
// CHỈ dùng cho CREATE: ở UPDATE, `""` là hành vi hợp lệ của người dùng (xoá nội dung 1 field text),
// bỏ key sẽ nuốt mất thao tác xoá đó.
//
// Ngoại lệ `name`: cột NOT NULL và KHÔNG có DEFAULT (baseline) — bỏ key sẽ đổi lỗi "tên rỗng" thành
// lỗi SQL khó hiểu, nên giữ nguyên `”` như trước. `code` không cần liệt kê: được gán SAU hàm này.
//
// ⚠ BẢO TRÌ (user chốt giữ cách tiếp cận này 2026-08-10, không đổi sang đọc schema động): danh sách
// `keep` PHẢI được cập nhật khi thêm cột `NOT NULL` KHÔNG có `DEFAULT` vào bảng `service`. Bỏ sót thì
// tạo dịch vụ qua multipart fail với `Field 'x' doesn't have a default value` — lỗi chỉ xuất hiện ở
// nhánh multipart nên rất dễ lọt QA. Hiện mọi cột của `service` đều nullable hoặc có DEFAULT, trừ
// `name` / `code` (đều đã được xử lý).
func dropEmptyStringsForCreate(updates map[string]interface{}) {
	keep := map[string]bool{"name": true}
	for k, v := range updates {
		if keep[k] {
			continue
		}
		if s, ok := v.(string); ok && s == "" {
			delete(updates, k)
		}
	}
}

// BuildServiceQuery builds service list query with filters
func (h *Handler) BuildServiceQuery(c *gin.Context) *gorm.DB {
	q := tenant.ScopedDBNullBranch(c, h.DB, "service").Table("service")
	q = query.ApplyKeywordFilter(c, q, "service.name", "service.code", "service.code_old")
	q = query.ApplyFilter(c, q, "status", "service.status")
	q = query.ApplyFilter(c, q, "service_type_id", "service.service_type_id")
	q = query.ApplyFilter(c, q, "serviceCategory_id", "service.serviceCategory_id")
	q = query.ApplyFilter(c, q, "child", "service.child")
	return q.Order("SortOrder DESC")
}

// serviceListSelectFields — cột thực sự dùng bởi ServiceList (FE service-list.html/ts) qua toàn bộ
// displayCols có thể toggle. KHÔNG select `installment_config` (JSON nặng, chỉ dùng ở form detail
// qua GetService) hay các field `_new`/`_type` hoa hồng, `id_old` — list không hiển thị các cột
// này (Hard Rule: KHÔNG SELECT *).
var serviceListSelectFields = []string{
	"service.id", "service.code", "service.code_old", "service.name",
	"service.status",
	"service.service_type_id", "service.serviceCategory_id", "service.SortOrder",
	"service.min_price", "service.max_price", "service.time_reminder",
	"service.child", "service.unit_id",
	"service.commission_telesale",
}

// removedServicePayloadKeys — key của các cột đã xoá khỏi bảng `service` (ảnh/mô tả/nội dung/điểm…
// của Zalo Mini App cũ) cùng field FE chưa từng có cột (`time_expired_*`, `numTreatment` — bảng
// keep-set CRM không có cột này) và file upload `fileImage`.
// FE bản cũ còn cache (PWA) vẫn có thể gửi các key này trong payload dạng map — bỏ đi để không lỗi
// 1054 (unknown column), cùng cách xử lý key `company_id`.
var removedServicePayloadKeys = []string{
	"image", "fileImage", "desc", "content", "treatment_duration", "validity_days",
	"companion_expired_at", "companion_expired_type", "point", "time_expired_at", "time_expired_type",
	"numTreatment",
}

func (h *Handler) ListService(c *gin.Context) {
	query := h.BuildServiceQuery(c)

	if c.Query("_export") == "1" {
		// Route dùng chung với list thường — export vẫn phải tự guard tại đây vì
		// PermissionEnforcer đang audit-mode (không chặn thật).
		if !h.guardExportPermission(c) {
			return
		}
		exportHandlers.ExportService(c, query)
		return
	}

	query = query.Select(serviceListSelectFields)
	var items []map[string]interface{}
	result := pagination.Paginate(c, query, &items)
	c.JSON(http.StatusOK, result)
}

// Plan 09 (2026-05-16): scope qua tenant. Fix IDOR — `service` là tenant
// table; First() cũ không scope. BaseRepo không dùng vì cần Preload — đi
// raw GORM + ScopedDBNullBranch theo convention `pkg/repository/README.md`.
func (h *Handler) GetService(c *gin.Context) {
	var item configServiceDao.Service
	// id không parse được ("undefined"/"NaN") → 422, KHÔNG 404: interceptor FE coi mọi 404 là lỗi
	// toàn cục và redirect (Hard Rule response format).
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_id"))
		return
	}
	if err := tenant.ScopedDBNullBranch(c, h.DB, "service").First(&item, id).Error; err != nil {
		response.NotFound(c)
		return
	}
	response.Data(c, item)
}

// POST /config/service — JSON hoặc multipart (form-data)
func (h *Handler) CreateService(c *gin.Context) {
	if strings.Contains(c.ContentType(), "multipart") {
		// Trần body do middleware.RequestSizeLimit (MaxBytesReader) áp — vượt ⇒ lỗi nổi lên ở đây.
		if err := c.Request.ParseMultipartForm(middleware.MaxMultipartBodyBytes); response.IsPayloadTooLarge(err) {
			response.PayloadTooLarge(c)
			return
		}
		updates := map[string]interface{}{}
		for k, v := range c.Request.PostForm {
			if len(v) > 0 {
				updates[k] = v[0]
			}
		}
		for _, k := range removedServicePayloadKeys {
			delete(updates, k)
		}
		// Mass-assignment guard: field kỹ thuật server tự set, KHÔNG tin giá trị client gửi qua
		// form (FE service-form.ts không có formControlName cho các field này — client hợp lệ
		// không bao giờ gửi; chỉ chặn request giả mạo trực tiếp vào API).
		for _, f := range []string{"id", "code", "created_by", "updated_by", "created_at", "updated_at", "id_old"} {
			delete(updates, f)
		}
		// FE cũ (tới story-43) còn gửi key thương hiệu — bảng không còn cột ⇒ bỏ để không lỗi 1054.
		delete(updates, "company_id")

		var bidPtr *uint
		if bidStr, _ := c.Request.PostForm["branch_id"]; len(bidStr) > 0 && bidStr[0] != "" {
			if bid := parseUint(bidStr[0]); bid > 0 {
				bidPtr = &bid
			}
		}

		stripBhytMappingFields(updates)

		// Chuẩn hoá payload multipart: gom `installment_config[...]`, marshal mảng — DÙNG CHUNG với
		// UpdateService. Bỏ chuỗi rỗng để cột nhận DEFAULT thay vì '' (chỉ áp dụng cho create).
		normalizeFormPayload(updates)
		dropEmptyStringsForCreate(updates)

		if err := validateCommissionRangeMap(updates); err != nil {
			response.HandleError(c, err)
			return
		}

		tx := h.DB.Begin()
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
				panic(r)
			}
		}()

		// codegen chạy TRONG tx: rollback thì serial được trả lại (trước đây dùng h.DB nên mỗi lần
		// insert lỗi vẫn đốt mất 1 số thứ tự), và cặp `Exec`/`SELECT LAST_INSERT_ID()` bên trong
		// codegen chắc chắn đi cùng 1 connection.
		genCode, gErr := codegen.GenerateErr(tx, "service", bidPtr)
		if gErr != nil {
			tx.Rollback()
			response.HandleError(c, gErr)
			return
		}
		updates["code"] = genCode

		uid := middleware.GetUserID(c)
		now := time.Now().Unix()
		updates["created_at"] = now
		updates["updated_at"] = now
		updates["created_by"] = uid
		updates["updated_by"] = uid

		if err := tenant.ScopedDBNullBranch(c, tx, "service").Table("service").Create(updates).Error; err != nil {
			tx.Rollback()
			response.ServerError(c, err)
			return
		}

		// GORM KHÔNG backfill primary key khi Create từ map — lấy id qua LAST_INSERT_ID() ngay
		// trên cùng connection (tx dùng 1 connection suốt vòng đời transaction, an toàn).
		// Trước đây tra ngược theo `code` — im lặng bỏ qua SortOrder nếu miss, giờ reject rõ ràng
		// thay vì tạo record mồ côi không có SortOrder.
		var insertedID uint
		tx.Raw("SELECT LAST_INSERT_ID()").Scan(&insertedID)
		if insertedID == 0 {
			tx.Rollback()
			response.ServerError(c, errors.New("service_insert_id_missing"))
			return
		}
		if err := tx.Table("service").Where("id = ?", insertedID).Update("SortOrder", insertedID).Error; err != nil {
			tx.Rollback()
			response.ServerError(c, err)
			return
		}

		if err := tx.Commit().Error; err != nil {
			response.ServerError(c, err)
			return
		}
		dexie.BumpDexieTable(h.DB, "services")
		response.Created(c, nil)
		return
	}

	var item configServiceDao.Service
	if err := c.ShouldBindJSON(&item); err != nil {
		response.HandleError(c, err)
		return
	}
	// Mass-assignment guard: struct binding từ JSON body có thể set thẳng các field kỹ thuật
	// này (json tag không omitempty) — server tự quản, KHÔNG tin giá trị client gửi.
	// `UpdatedAt`: clone gửi lại updated_at của bản gốc — reset để bản mới nhận thời điểm tạo.
	item.ID = 0
	item.Code = ""
	item.CreatedBy = nil
	item.UpdatedBy = nil
	item.UpdatedAt = 0

	if err := validateCommissionRangeStruct(&item); err != nil {
		response.HandleError(c, err)
		return
	}

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := tx.Set("userID", middleware.GetUserID(c)).Create(&item).Error; err != nil {
		tx.Rollback()
		response.ServerError(c, err)
		return
	}
	tx.Model(&item).Update("SortOrder", item.ID)

	if err := tx.Commit().Error; err != nil {
		response.ServerError(c, err)
		return
	}
	dexie.BumpDexieTable(h.DB, "services")
	response.Created(c, item)
}

func (h *Handler) UpdateService(c *gin.Context) {
	var item configServiceDao.Service
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_id"))
		return
	}
	if err := tenant.ScopedDBNullBranch(c, h.DB, "service").First(&item, id).Error; err != nil {
		response.NotFound(c)
		return
	}

	updates := map[string]interface{}{}
	if strings.Contains(c.ContentType(), "multipart") {
		// Trần body do middleware.RequestSizeLimit (MaxBytesReader) áp — vượt ⇒ lỗi nổi lên ở đây.
		if err := c.Request.ParseMultipartForm(middleware.MaxMultipartBodyBytes); response.IsPayloadTooLarge(err) {
			response.PayloadTooLarge(c)
			return
		}
		for k, v := range c.Request.PostForm {
			if len(v) > 0 {
				updates[k] = v[0]
			}
		}
	} else {
		if err := c.ShouldBindJSON(&updates); err != nil {
			response.HandleError(c, err)
			return
		}
	}

	// FE cũ (tới story-43) còn gửi key thương hiệu — bảng không còn cột ⇒ bỏ để không lỗi 1054.
	delete(updates, "company_id")
	for _, k := range removedServicePayloadKeys {
		delete(updates, k)
	}

	stripBhytMappingFields(updates)

	// Khoá lạc quan theo updated_at (D6, pkg/optlock) — đọc TRƯỚC khi updates["updated_at"] bị gỡ.
	v, ok := updates["updated_at"]
	if !ok {
		response.Validate(c, i18n.T(c, "version_missing"))
		return
	}
	if helpers.ToInt64(v) != item.UpdatedAt {
		response.Conflict(c, errs.ErrVersionConflict.Error())
		return
	}
	expectedUpdatedAt := item.UpdatedAt

	delete(updates, "id")
	// Mass-assignment guard: field kỹ thuật server tự set, KHÔNG tin giá trị client gửi qua
	// update payload (FE service-form.ts không có formControlName cho các field này).
	// `code` không cho đổi sau khi tạo — không có UI nào chỉnh sửa nó, chỉ codegen kiểm soát.
	for _, f := range []string{"code", "created_by", "updated_by", "created_at", "id_old"} {
		delete(updates, f)
	}
	updates["updated_by"] = middleware.GetUserID(c)
	delete(updates, "updated_at") // callback optlock bump GREATEST(now, updated_at+1)
	normalizeFormPayload(updates)

	if err := validateCommissionRangeMap(updates); err != nil {
		response.HandleError(c, err)
		return
	}

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	if _, err := optlock.Updates(tx.Model(&item).Where("id = ?", item.ID), expectedUpdatedAt, updates); err != nil {
		tx.Rollback()
		if errors.Is(err, errs.ErrVersionConflict) {
			response.Conflict(c, errs.ErrVersionConflict.Error())
			return
		}
		response.ServerError(c, err)
		return
	}

	if err := tx.Commit().Error; err != nil {
		response.ServerError(c, err)
		return
	}
	dexie.BumpDexieTable(h.DB, "services")
	tenant.ScopedDBNullBranch(c, h.DB, "service").First(&item, item.ID)
	response.Updated(c, item)
}

func (h *Handler) DeleteService(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_id"))
		return
	}
	var item configServiceDao.Service
	if err := tenant.ScopedDBNullBranch(c, h.DB, "service").First(&item, id).Error; err != nil {
		response.NotFound(c)
		return
	}
	if err := models.DeleteWithCheck(h.DB, &configServiceDao.Service{}, id); err != nil {
		response.HandleError(c, err)
		return
	}
	dexie.BumpDexieTable(h.DB, "services")
	response.Deleted(c)
}

// GET /config/service/load-service-by-type?listType[]=1&listType[]=2
func (h *Handler) LoadServiceByType(c *gin.Context) {
	listType := c.QueryArray("listType[]")
	if len(listType) == 0 {
		listType = c.QueryArray("listType")
	}
	if len(listType) == 0 {
		c.JSON(http.StatusOK, make([]uint, 0))
		return
	}
	ids := make([]uint, 0)
	tenant.ScopedDBNullBranch(c, h.DB, "service").Table("service").Select("service.id").
		Joins("JOIN service_type st ON st.id = service.service_type_id AND st.status = 1").
		Where("service.service_type_id IN ? AND service.status = 1", listType).Pluck("service.id", &ids)
	c.JSON(http.StatusOK, ids)
}

// activeServiceQuery — base query dùng chung cho 3 endpoint keyword-search (load-by-keyword,
// load-list-service-by-keyword, load-service): service active + service_type active, scope tenant.
// Tách để tránh lặp Joins/Where giữa 3 handler gần giống nhau (khác nhau ở Select + cách match keyword).
func (h *Handler) activeServiceQuery(c *gin.Context) *gorm.DB {
	return tenant.ScopedDBNullBranch(c, h.DB, "service").Table("service").
		Joins("JOIN service_type st ON st.id = service.service_type_id AND st.status = 1").
		Where("service.status = 1")
}

// GET /config/service/load-by-keyword?keyword=y
// Story-39 (D15): tham số lọc thương hiệu cũ (nếu FE còn gửi) bị bỏ qua.
func (h *Handler) LoadServiceByKeyword(c *gin.Context) {
	keyword := c.Query("keyword")
	q := h.activeServiceQuery(c).Select(serviceListSelectFields)
	if keyword != "" {
		tokens := strings.Fields(strings.TrimSpace(keyword))
		for _, t := range tokens {
			q = q.Where("service.name LIKE ?", "%"+t+"%")
		}
	}
	items := make([]map[string]interface{}, 0)
	tx := q.Order("service.SortOrder ASC").Limit(100).Find(&items)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	helpers.ParseDecimalFieldsForTables(items, "service")
	c.JSON(http.StatusOK, items)
}

// GET /config/service/load-list-service-by-keyword?keyword=y
func (h *Handler) LoadListServiceByKeyword(c *gin.Context) {
	keyword := c.Query("keyword")
	q := h.activeServiceQuery(c).Select("service.id as value, service.name as label")
	if keyword != "" {
		q = q.Where("service.name LIKE ?", "%"+strings.TrimSpace(keyword)+"%")
	}
	items := make([]map[string]interface{}, 0)
	tx := q.Order("service.SortOrder ASC").Limit(50).Find(&items)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	c.JSON(http.StatusOK, items)
}

// serviceIDOldOwner — dịch vụ đang giữ một cặp (company_id, id_old) trong DB. Giữ cả `code` để
// thông báo lỗi import chỉ đúng bản ghi người dùng phải xử lý, không bắt họ tự tra id.
type serviceIDOldOwner struct {
	id   uint
	code string
}

// POST /config/service/import-service-from-excel
func (h *Handler) ImportServiceFromExcel(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Validate(c, i18n.T(c, "file_required"))
		return
	}
	rows, err := readImportRows(file)
	if err != nil {
		response.Validate(c, i18n.TF(c, "excel_read_failed", err.Error()))
		return
	}
	if len(rows) < 2 {
		response.Validate(c, i18n.T(c, "no_data"))
		return
	}

	colIndex, dataStart, dupCol := resolveServiceImportColumns(rows)
	if len(colIndex) == 0 {
		response.Validate(c, i18n.T(c, "import_no_recognized_column"))
		return
	}
	if dupCol != "" {
		response.Validate(c, i18n.TF(c, "import_duplicate_column", dupCol))
		return
	}
	if len(rows) <= dataStart {
		response.Validate(c, i18n.T(c, "no_data"))
		return
	}
	if len(rows)-dataStart > MaxImportRows {
		response.Validate(c, i18n.TF(c, "import_too_many_rows", MaxImportRows))
		return
	}

	// "cột|tên" → id danh mục đã tra. Cùng lý do cache với taxRateRefCache: file 5000 dòng thường lặp
	// lại vài chục tên nhóm dịch vụ/đơn vị.
	catalogRefCache := map[string]int64{}
	// "cột|#id" → id đó có trong danh mục không. File 5000 dòng thường chỉ dùng đi dùng lại vài chục
	// id nhóm dịch vụ/đơn vị.
	catalogIDExists := map[string]bool{}
	// Bảng nguồn danh mục dựng MỘT lần: file 5000 dòng × ~44 cột là ~220.000 lần dựng map nếu gọi
	// trong vòng lặp ô.
	catalogSources := exportHandlers.ServiceCatalogSources()
	var created, updated int

	uid := middleware.GetUserID(c)
	now := time.Now().Unix()

	// Batch-load bản ghi service của các dòng UPDATE (cột "id" trong file) — 1 query thay vì 1 query
	// mỗi dòng (file tối đa MaxImportRows = 5000). Scope tenant ngay tại đây: id không thuộc tenant
	// sẽ vắng mặt trong map → row đó báo "not found" như cũ.
	existingByID := map[int64]configServiceDao.Service{}
	idCol, idOldCol := -1, -1
	for _, ic := range colIndex {
		switch ic.col {
		case "id":
			idCol = ic.idx
		case "id_old":
			idOldCol = ic.idx
		}
	}
	// id_old là khoá UNIQUE — schema cũ (company_id, id_old) (migration 20260814091602), đích E4
	// UNIQUE(id_old). Story-39 (D15, 1 thương hiệu/DB): kiểm trùng TOÀN DB theo đích E4. Không
	// pre-check thì dòng trùng chỉ nổ ở tầng MySQL ("Duplicate entry '1-365' for key
	// 'service.id_old'"), rollback cả file với thông báo không đọc được và KHÔNG cho biết dòng nào sai.
	// idOldOwner: id_old đang lưu trong DB → dịch vụ giữ nó.
	// idOldSeenRow: id_old đã ghi ở dòng TRƯỚC trong cùng file → số dòng Excel.
	idOldOwner := map[int64]serviceIDOldOwner{}
	idOldSeenRow := map[int64]int{}
	importIDOlds := make([]int64, 0, len(rows)-dataStart)
	importIDs := make([]int64, 0, len(rows)-dataStart)
	hasCreateRow := false
	for _, row := range rows[dataStart:] {
		if allEmpty(row) {
			continue
		}
		// Gom TRƯỚC nhánh rawID: dòng tạo mới (ô "id" trống) cũng ghi id_old nên cũng phải đối chiếu.
		if idOldCol >= 0 && idOldCol < len(row) {
			if raw := normalizeExcelIntCell("id_old", pkgExport.Unsanitize(strings.TrimSpace(row[idOldCol]))); raw != "" {
				if n, ok := parseImportUint(raw); ok {
					importIDOlds = append(importIDOlds, n)
				}
			}
		}
		rawID := ""
		if idCol >= 0 && idCol < len(row) {
			// Chuẩn hoá Y HỆT vòng lặp đọc ô bên dưới. Lệch nhau ở đây là dòng lọt vào nhánh update
			// mà id lại không nằm trong importIDs → tra map rỗng → báo "Không tồn tại" oan.
			rawID = normalizeExcelIntCell("id", strings.TrimSpace(row[idCol]))
		}
		if rawID == "" {
			hasCreateRow = true
			continue
		}
		if id := helpers.ToInt64(rawID); id > 0 {
			importIDs = append(importIDs, id)
		}
	}

	// Guard quyền theo đúng thao tác file yêu cầu (module config KHÔNG có permission middleware
	// enforce — permEnforce mặc định false — nên phải tự chặn tại handler, cùng lý do với
	// guardExportPermission). Import ghi thẳng giá/hoa hồng của tối đa MaxImportRows dịch vụ, không
	// thể để mọi user đăng nhập gọi được.
	if len(importIDs) > 0 && !h.hasServiceWritePerm(c, define.PermManageConfig) {
		response.Forbidden(c)
		return
	}
	if hasCreateRow && !h.hasServiceWritePerm(c, define.PermManageConfig) {
		response.Forbidden(c)
		return
	}

	if len(importIDs) > 0 {
		var existing []configServiceDao.Service
		tenant.ScopedDBNullBranch(c, h.DB, "service").Where("id IN ?", importIDs).Find(&existing)
		for i := range existing {
			existingByID[int64(existing[i].ID)] = existing[i]
		}
	}

	// 1 query cho cả file (cùng lý do batch với existingByID). Phạm vi = toàn bảng dịch vụ của DB
	// (story-39: không còn phần company trong khoá).
	if len(importIDOlds) > 0 {
		var owners []struct {
			ID    uint   `gorm:"column:id"`
			Code  string `gorm:"column:code"`
			IDOld *int   `gorm:"column:id_old"`
		}
		tenant.ScopedDBNullBranch(c, h.DB, "service").Table("service").
			Select("service.id, service.code, service.id_old").
			Where("service.id_old IN ?", importIDOlds).Scan(&owners)
		for _, o := range owners {
			if o.IDOld == nil {
				continue
			}
			// Bản ghi nằm trong CHÍNH file này sẽ bị nhả id_old ở pha release bên dưới, nên không
			// tính là đụng độ — nếu không thì file hoán đổi id_old giữa hai dòng của nó bị chặn oan.
			if _, inFile := existingByID[int64(o.ID)]; inFile && idOldCol >= 0 {
				continue
			}
			idOldOwner[int64(*o.IDOld)] = serviceIDOldOwner{id: o.ID, code: o.Code}
		}
	}

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// Pha "nhả id_old": xoá id_old của MỌI dòng update trong file trước khi ghi lại từng dòng.
	// Không có pha này thì một file TỰ NHẤT QUÁN vẫn chết vì THỨ TỰ dòng — sự cố thật 2026-09-06
	// (DB một khách): dòng 122 nhận id_old 365 trong khi dòng 212 mới là dòng trả 365 lại (đổi sang
	// 386), mà MySQL kiểm UNIQUE của id_old ngay tại từng câu UPDATE chứ không hoãn tới
	// commit. Chỉ chạy khi file CÓ cột "id cũ": khi đó mọi dòng trong file đều ghi lại id_old (ô
	// trống = NULL theo serviceImportClearOnEmptyCols) nên không bản ghi nào bị bỏ lại NULL ngoài ý
	// muốn; và mọi lỗi dòng sau đó đều rollback cả transaction, kể cả pha này.
	if idOldCol >= 0 && len(existingByID) > 0 {
		releaseIDs := make([]int64, 0, len(existingByID))
		for id := range existingByID {
			releaseIDs = append(releaseIDs, id)
		}
		if err := tenant.ScopedDBNullBranch(c, tx, "service").Table("service").
			Where("id IN ?", releaseIDs).Update("id_old", nil).Error; err != nil {
			tx.Rollback()
			response.ServerError(c, err)
			return
		}
	}

	for i, row := range rows[dataStart:] {
		if allEmpty(row) {
			continue
		}
		rowNum := i + dataStart + 1
		// failRow — rollback + trả lỗi kèm số dòng Excel. Caller PHẢI `return` ngay sau khi gọi.
		failRow := func(msg string) {
			tx.Rollback()
			response.Validate(c, i18n.TF(c, "import_row_error", rowNum, msg))
		}

		data := map[string]interface{}{}
		for _, ic := range colIndex {
			raw := ""
			if ic.idx < len(row) {
				// Unsanitize: export chèn dấu nháy đơn vào đầu chuỗi bắt đầu bằng ký tự công thức
				// (=, +, -, @). Không bóc ra thì "- Combo A" quay lại DB thành "'- Combo A", và mỗi
				// vòng export/import lại chồng thêm một dấu.
				raw = pkgExport.Unsanitize(strings.TrimSpace(row[ic.idx]))
				// "3.0" → "3" trước khi vào bất kỳ parser số nào (xem normalizeExcelIntCell).
				raw = normalizeExcelIntCell(ic.col, raw)
			}
			if raw == "" {
				switch {
				case serviceImportKeepOnEmptyCols[ic.col]:
					// giữ nguyên giá trị đang lưu
				case serviceImportRequiredCols[ic.col]:
					failRow(i18n.TF(c, "import_column_required_empty", ic.col))
					return
				default:
					if clear, ok := serviceImportClearOnEmptyCols[ic.col]; ok {
						data[ic.col] = clear
					}
				}
				continue
			}
			if serviceImportMoneyCols[ic.col] {
				amount, ok := helpers.ParseExcelMoney(raw)
				if !ok {
					failRow(i18n.TF(c, "import_invalid_number", ic.col, raw))
					return
				}
				data[ic.col] = amount
				continue
			}
			if serviceImportBoolCols[ic.col] {
				flag, ok := parseImportBool(raw)
				if !ok {
					failRow(i18n.TF(c, "import_invalid_flag", ic.col, raw))
					return
				}
				data[ic.col] = flag
				continue
			}
			if serviceImportUintCols[ic.col] {
				n, ok := parseImportUint(raw)
				if !ok {
					failRow(i18n.TF(c, "import_invalid_number", ic.col, raw))
					return
				}
				data[ic.col] = n
				continue
			}
			if opts, isEnum := serviceImportEnumCols[ic.col]; isEnum {
				v, valid := parseEnumValue(opts, raw)
				if !valid {
					failRow(i18n.TF(c, "import_invalid_option", ic.col, enumLabelList(opts), raw))
					return
				}
				data[ic.col] = v
				continue
			}
			if _, isCatalog := catalogSources[ic.col]; isCatalog {
				// Bóc "12 — Nhóm A" thành id ngay tại đây (không cần DB). Còn lại là dạng chỉ có
				// tên → giữ nguyên chuỗi, resolve theo tên ở bước sau.
				if id, _ := parseCatalogRef(raw); id > 0 {
					data[ic.col] = id
					continue
				}
				data[ic.col] = raw
				continue
			}
			data[ic.col] = raw
		}
		if len(data) == 0 {
			continue
		}

		// Hoa hồng: import từng là cửa sau bỏ qua hoàn toàn kiểm tra range mà đường form (cả UI lẫn
		// API) đều enforce — file Excel ghi thẳng được giá trị âm hoặc >100% khi type=percent.
		// Hạn chế còn lại (giống partial update ở form): nếu file KHÔNG có cột "Loại HH ..." thì không
		// đối chiếu được với type đang lưu trong DB, chỉ chặn được giá trị âm.
		if err := validateCommissionRangeMap(data); err != nil {
			failRow(i18n.T(c, err.Error()))
			return
		}

		// resolveCatalogRefs — cột FK người dùng gõ bằng TÊN ("Nhóm A") thay vì id: tra ngược ra id
		// trong danh mục của DB. Ô đã là số (gõ id, hoặc chọn dropdown "12 — ...") thì chỉ kiểm tồn
		// tại. current = bản ghi đang lưu (nil khi tạo mới).
		resolveCatalogRefs := func(current *configServiceDao.Service) error {
			for col, src := range catalogSources {
				raw, ok := data[col]
				if !ok || raw == nil {
					continue
				}
				name, isName := raw.(string)
				if !isName || strings.TrimSpace(name) == "" {
					// Ô đã là id: PHẢI kiểm tồn tại trong danh mục (skill patterns-bulk-import
					// §3). Nhận bừa số làm FK thì sai chỉ lộ ra lúc tx.Create dưới dạng MySQL 1452
					// thô — không dịch được, không chỉ ra dòng nào, và rollback cả file.
					id := helpers.ToInt64(raw)
					if id <= 0 {
						return catalogRefError{col: col, raw: fmt.Sprintf("%v", raw), err: errCatalogNotFound}
					}
					// Giá trị KHÔNG đổi so với bản ghi đang lưu thì không kiểm lại — Hard Rule "chỉ
					// validate FK/scope khi giá trị body khác giá trị đang lưu" (sự cố
					// UpdateCustomerCare 2026-07-27).
					// Bản ghi cũ trỏ danh mục đã tắt là dữ liệu CÓ SẴN trong DB; kiểm vô điều kiện
					// sẽ khoá cứng nó, user không sửa nổi cả cái tên.
					if catalogValueUnchanged(col, current, id) {
						continue
					}
					key := fmt.Sprintf("%s|#%d", col, id)
					if ok, cached := catalogIDExists[key]; cached {
						if !ok {
							return catalogRefError{col: col, raw: strconv.FormatInt(id, 10), err: errCatalogNotFound}
						}
						continue
					}
					found, err := catalogIDBelongs(tx, src, id)
					if err != nil {
						return err
					}
					catalogIDExists[key] = found
					if !found {
						return catalogRefError{col: col, raw: strconv.FormatInt(id, 10), err: errCatalogNotFound}
					}
					continue
				}
				key := fmt.Sprintf("%s|%s", col, name)
				if id, cached := catalogRefCache[key]; cached {
					data[col] = id
					continue
				}
				id, err := lookupCatalogIDByName(tx, src, name)
				if err != nil {
					return catalogRefError{col: col, raw: name, err: err}
				}
				catalogRefCache[key] = id
				data[col] = id
			}
			return nil
		}

		// checkIDOld — chặn trùng khoá UNIQUE id_old (toàn DB) trước khi ghi, báo đúng số dòng
		// Excel. Hai nguồn đụng độ: dòng TRƯỚC trong cùng file (idOldSeenRow — dòng trước đã ghi
		// trong cùng transaction) và bản ghi có sẵn trong DB (idOldOwner). selfID = id của chính
		// dòng đang update (0 khi tạo mới): ghi lại đúng id_old mình đang giữ không phải đụng độ.
		// Caller PHẢI `return` khi hàm trả false (failRow đã rollback + trả response).
		checkIDOld := func(selfID uint) bool {
			raw, has := data["id_old"]
			if !has || raw == nil {
				return true // cột vắng mặt → giữ nguyên; ô trống → ghi NULL, không đụng khoá unique
			}
			idOld := helpers.ToInt64(raw)
			key := idOld
			if prevRow, dup := idOldSeenRow[key]; dup {
				failRow(i18n.TF(c, "import_id_old_duplicated_in_file", idOld, prevRow))
				return false
			}
			if owner, taken := idOldOwner[key]; taken && owner.id != selfID {
				failRow(i18n.TF(c, "import_id_old_taken", idOld, owner.id, owner.code))
				return false
			}
			idOldSeenRow[key] = rowNum
			return true
		}

		if idVal, ok := data["id"]; ok && idVal != "" {
			// UPDATE — đọc bản ghi trước (scope tenant) thay vì dựa vào RowsAffected: RowsAffected==0
			// cũng xảy ra khi row trong file TRÙNG HỆT dữ liệu đang lưu, trước đây bị báo nhầm là
			// "not found" và rollback cả file import.
			current, found := existingByID[helpers.ToInt64(idVal)]
			if !found {
				failRow(i18n.T(c, "not_found"))
				return
			}
			// Chỉ kiểm ô danh mục ĐỔI so với bản ghi đang lưu (catalogValueUnchanged).
			if err := resolveCatalogRefs(&current); err != nil {
				failRow(catalogImportErrMsg(c, err))
				return
			}

			if !checkIDOld(current.ID) {
				return
			}

			delete(data, "id")
			data["updated_at"] = now
			data["updated_by"] = uid
			// updated_at: callback optlock bump GREATEST(now, updated_at+1) — form đang mở với
			// updated_at cũ sẽ nhận 409 thay vì âm thầm ghi đè kết quả import.

			if err := tenant.ScopedDBNullBranch(c, tx, "service").Table("service").
				Where("id = ?", current.ID).Updates(data).Error; err != nil {
				failRow(err.Error())
				return
			}
			updated++
		} else {
			// Create new
			delete(data, "id")

			if err := resolveCatalogRefs(nil); err != nil {
				failRow(catalogImportErrMsg(c, err))
				return
			}
			if !checkIDOld(0) {
				return
			}

			data["created_at"] = now
			data["updated_at"] = now
			data["created_by"] = uid
			data["updated_by"] = uid

			if codeVal, _ := data["code"].(string); codeVal == "" {
				// Cột mã UNIQUE optional — 2-step insert (Hard Rule): temp code unique →
				// Create → update lại mã thật. KHÔNG insert thẳng "" — codegen.Generate có thể
				// trả "" nếu config_code chưa seed parent_table="service" →
				// 2 row cùng để trống mã fail "Duplicate entry ''".
				data["code"] = fmt.Sprintf("_tmp%d", time.Now().UnixNano())
				if err := tx.Table("service").Create(data).Error; err != nil {
					failRow(err.Error())
					return
				}
				var insertedID uint
				tx.Raw("SELECT LAST_INSERT_ID()").Scan(&insertedID)
				// codegen trong tx: rollback trả lại serial thay vì đốt mất số thứ tự (đồng bộ với
				// CreateService), và cặp Exec/LAST_INSERT_ID() bên trong codegen đi cùng 1 connection.
				finalCode := codegen.Generate(tx, "service", nil)
				if finalCode == "" {
					failRow(i18n.T(c, "code_generation_failed"))
					return
				}
				if err := tx.Table("service").Where("id = ?", insertedID).Update("code", finalCode).Error; err != nil {
					failRow(err.Error())
					return
				}
			} else if err := tx.Table("service").Create(data).Error; err != nil {
				failRow(err.Error())
				return
			}
			created++
		}
	}
	if err := tx.Commit().Error; err != nil {
		response.ServerError(c, err)
		return
	}
	dexie.BumpDexieTable(h.DB, "services")
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"created": created, "updated": updated, "skipped": 0}})
}

// GET /config/service/load-service?keyword=y (alias)
// Story-39 (D15): tham số lọc thương hiệu cũ (nếu FE còn gửi) bị bỏ qua.
func (h *Handler) LoadService(c *gin.Context) {
	keyword := c.Query("keyword")
	q := h.activeServiceQuery(c).Select(serviceListSelectFields)
	if keyword != "" {
		q = q.Where("service.name LIKE ?", "%"+strings.TrimSpace(keyword)+"%")
	}
	items := make([]map[string]interface{}, 0)
	tx := q.Order("service.SortOrder ASC").Limit(20).Find(&items)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	helpers.ParseDecimalFieldsForTables(items, "service")
	c.JSON(http.StatusOK, items)
}
