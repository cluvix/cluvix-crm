// Package handler — API danh mục bước tiến trình khách hàng `/config/customer-stage`
// (CRUD + reorder + set-default). Mẫu: config/customer_category/handler.
package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"erpCluvix/internal/middleware"
	"erpCluvix/internal/modules/webapp/config/customer_stage/dao"
	"erpCluvix/internal/modules/webapp/config/customer_stage/service"
	"erpCluvix/pkg/dexie"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"
)

// dexieTable — khoá bảng Dexie phía FE (table-registry) của danh mục bước.
const dexieTable = "customerStages"

// stageTable — bảng danh mục bước (toàn DB, không branch_id).
const stageTable = "customer_stage"

// stageColumns — cột trả về FE (không SELECT *; `default_key` là cột sinh của DB, không phơi ra).
const stageColumns = "id, name, type, color, sort_order, is_default, status, created_at, updated_at, created_by, updated_by"

// Deps + Handler — module-local (Plan 06 B1).
type Deps struct {
	DB *gorm.DB
}

type Handler struct {
	Deps
	svc *service.Service
}

func New(d Deps) *Handler { return &Handler{Deps: d, svc: service.New(d.DB)} }

// bindError đánh dấu lỗi bind payload (client sai) để tách khỏi lỗi DB (500) khi lỗi đi qua transaction.
type bindError struct{ err error }

func (e bindError) Error() string { return e.err.Error() }
func (e bindError) Unwrap() error { return e.err }

// respondError — một cửa trả lỗi cho mọi handler ghi: 404, trùng tên (1062), lỗi input/i18n → 422,
// còn lại (lỗi DB không lường trước) → ServerError (không lộ chuỗi lỗi thô ra client).
func respondError(c *gin.Context, err error) {
	var be bindError
	var le *errs.LocalizedError
	switch {
	case errors.Is(err, errs.ErrNotFound) || errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c)
	case service.IsDuplicateNameErr(err):
		response.Validate(c, i18n.T(c, service.KeyNameDuplicate))
	case errors.As(err, &be):
		response.HandleError(c, be.err)
	case errors.As(err, &le):
		response.HandleError(c, err)
	default:
		response.ServerError(c, err)
	}
}

func userIDPtr(c *gin.Context) *int {
	uid := middleware.GetUserID(c)
	if uid == 0 {
		return nil
	}
	v := int(uid)
	return &v
}

func (h *Handler) ListCustomerStage(c *gin.Context) {
	query := tenant.ScopedDBNullBranch(c, h.DB, stageTable).Model(&dao.CustomerStage{}).Select(stageColumns)
	if keyword := strings.TrimSpace(c.Query("_keyword")); keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	if status := c.Query("filter[status]"); status != "" {
		query = query.Where("status = ?", status)
	}
	query = query.Order("sort_order ASC, id ASC")
	var items []dao.CustomerStage
	result := pagination.Paginate(c, query, &items)
	c.JSON(http.StatusOK, result)
}

func (h *Handler) GetCustomerStage(c *gin.Context) {
	var item dao.CustomerStage
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	if err := tenant.ScopedDBNullBranch(c, h.DB, stageTable).Select(stageColumns).First(&item, id).Error; err != nil {
		response.NotFound(c)
		return
	}
	response.Data(c, item)
}

func (h *Handler) CreateCustomerStage(c *gin.Context) {
	var item dao.CustomerStage
	if err := c.ShouldBindJSON(&item); err != nil {
		response.HandleError(c, err)
		return
	}
	// Mass-assignment: khoá chính, thứ tự, cờ mặc định, mốc thời gian do server quyết — bỏ giá trị client gửi.
	item.ID = 0
	item.IsDefault = 0
	item.CreatedAt = 0
	item.UpdatedAt = 0
	item.Name = service.NormalizeName(item.Name)
	if err := service.ValidateFields(item.Name, item.Type, item.Color, item.Status); err != nil {
		respondError(c, err)
		return
	}
	if taken, err := service.NameTaken(h.DB, item.Name, 0); err != nil {
		response.ServerError(c, err)
		return
	} else if taken {
		response.Validate(c, i18n.T(c, service.KeyNameDuplicate))
		return
	}
	next, err := service.NextSortOrder(h.DB)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	item.SortOrder = next
	item.CreatedBy = userIDPtr(c)
	item.UpdatedBy = item.CreatedBy
	if err := h.DB.Create(&item).Error; err != nil {
		respondError(c, err)
		return
	}
	dexie.BumpDexieTable(h.DB, dexieTable)
	response.Created(c, item)
}

func (h *Handler) UpdateCustomerStage(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	var item dao.CustomerStage
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		// Khoá dòng: kiểm bất biến "bước mặc định" và ghi phải nhìn cùng một trạng thái (set-default song song).
		if err := tenant.ScopedDBNullBranch(c, tx, stageTable).Clauses(clause.Locking{Strength: "UPDATE"}).
			Select(stageColumns).First(&item, id).Error; err != nil {
			return err
		}
		// sort_order / is_default chỉ đổi qua endpoint riêng; audit tạo giữ nguyên.
		if err := helpers.BindUpdate(c, &item, "SortOrder", "IsDefault", "CreatedAt", "CreatedBy", "UpdatedBy"); err != nil {
			return bindError{err}
		}
		item.Name = service.NormalizeName(item.Name)
		if item.Status == nil {
			return errs.NewLocalizedError(service.KeyStatusInvalid)
		}
		if err := service.ValidateFields(item.Name, item.Type, item.Color, item.Status); err != nil {
			return err
		}
		if err := service.ValidateDefaultInvariant(item.IsDefault, item.Type, item.Status); err != nil {
			return err
		}
		if taken, err := service.NameTaken(tx, item.Name, item.ID); err != nil {
			return err
		} else if taken {
			return errs.NewLocalizedError(service.KeyNameDuplicate)
		}
		item.UpdatedBy = userIDPtr(c)
		return tx.Save(&item).Error
	})
	if err != nil {
		respondError(c, err)
		return
	}
	dexie.BumpDexieTable(h.DB, dexieTable)
	response.Updated(c, item)
}

func (h *Handler) DeleteCustomerStage(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	var probe dao.CustomerStage
	if err := tenant.ScopedDBNullBranch(c, h.DB, stageTable).Select("id").First(&probe, id).Error; err != nil {
		response.NotFound(c)
		return
	}
	if err := h.svc.Delete(id); err != nil {
		respondError(c, err)
		return
	}
	dexie.BumpDexieTable(h.DB, dexieTable)
	response.Deleted(c)
}

// reorderBody — `ids`: id MỌI bước theo thứ tự mới.
type reorderBody struct {
	IDs []int `json:"ids"`
}

// ReorderCustomerStage — PUT /customer-stage/reorder. Thao tác trên TOÀN BỘ danh mục (bảng không có
// branch_id) nên không có "id đơn lẻ" để scope; sai tập id ⇒ 422, không ghi gì.
func (h *Handler) ReorderCustomerStage(c *gin.Context) {
	var body reorderBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.HandleError(c, err)
		return
	}
	if err := h.svc.Reorder(body.IDs, middleware.GetUserID(c)); err != nil {
		respondError(c, err)
		return
	}
	dexie.BumpDexieTable(h.DB, dexieTable)
	response.Updated(c, gin.H{"ids": body.IDs})
}

// SetDefaultCustomerStage — PUT /customer-stage/:id/set-default.
func (h *Handler) SetDefaultCustomerStage(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	var probe dao.CustomerStage
	if err := tenant.ScopedDBNullBranch(c, h.DB, stageTable).Select("id").First(&probe, id).Error; err != nil {
		response.NotFound(c)
		return
	}
	if err := h.svc.SetDefault(id, middleware.GetUserID(c)); err != nil {
		respondError(c, err)
		return
	}
	dexie.BumpDexieTable(h.DB, dexieTable)
	var item dao.CustomerStage
	if err := tenant.ScopedDBNullBranch(c, h.DB, stageTable).Select(stageColumns).First(&item, id).Error; err != nil {
		response.ServerError(c, err)
		return
	}
	response.Updated(c, item)
}
