package handler

import (
	"time"

	"erpCluvix/internal/middleware"
	noteDao "erpCluvix/internal/modules/webapp/crm/note/dao"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
)

// CreateNote POST /crm/note
func (h *Handler) CreateNote(c *gin.Context) {
	var item noteDao.Note
	if err := c.ShouldBindJSON(&item); err != nil {
		response.ValidateErr(c, err)
		return
	}
	// Create: khóa chính do DB cấp — reset `id` client gửi kèm trong body/form (mass-assignment).
	item.ID = 0
	if !response.RequireFields(c, [][2]any{{item.Note, "Nội dung ghi chú"}}) {
		return
	}

	if item.CustomerId == nil || *item.CustomerId == 0 {
		response.Validate(c, i18n.T(c, "id_customer_required"))
		return
	}
	// Khách phải CÓ THẬT; CỐ Ý không lọc chi nhánh (ghi chú cho khách chi nhánh khác vẫn được).
	// Story-36 (D15): trước đây suy company của khách; nay không đọc company nữa.
	var custCount int64
	if err := tenant.CrossBranchLookup(h.DB, "customer").Table("customer").
		Where("id = ?", *item.CustomerId).Count(&custCount).Error; err != nil {
		response.ServerError(c, err)
		return
	}
	if custCount == 0 {
		response.Validate(c, i18n.T(c, "customer_not_found"))
		return
	}
	userID := int(middleware.GetUserID(c))
	item.CreatedBy = &userID

	if err := h.DB.Create(&item).Error; err != nil {
		response.ServerError(c, err)
		return
	}

	// Customer side effects — last_contact_date.
	// (Tách khỏi AfterCreate hook để dùng tenant-scoped DB, tránh cross-tenant write.)
	// D14 (2026-09-23, QA story-33): bỏ nhánh nâng Ticket(2)→Lead(4). Tiến trình cũ (`customer.status`)
	// đã bỏ hẳn ở customer-stage-pipeline story-12 — ghi chú không đổi bước khách.
	if item.CustomerId != nil {
		updates := map[string]interface{}{"last_contact_date": item.CreatedAt}

		tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
			Where("id = ?", *item.CustomerId).Updates(updates)
	}

	response.Created(c, item)
}

// PinNote POST /crm/note/:id/pin-note
// Toggle pinned state, store pinned_at + pinned_by snapshot khi pin.
func (h *Handler) PinNote(c *gin.Context) {
	// id LUÔN lấy từ path param và phải parse ra số. Trước đây khi path param không parse
	// được thì code rơi về `id` trong body/query ⇒ URL rác + body tự đặt id = ghim nhầm note.
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "missing_id_or_pinned"))
		return
	}

	var body struct {
		Pinned string `json:"pinned" form:"pinned"`
	}
	_ = c.ShouldBindQuery(&body)
	if body.Pinned == "" {
		_ = c.ShouldBindJSON(&body)
	}
	isPinned := body.Pinned == "true" || body.Pinned == "1"

	var item noteDao.Note
	if err := tenant.ScopedDBNullBranch(c, h.DB, "note").First(&item, id).Error; err != nil {
		response.NotFound(c)
		return
	}

	if isPinned {
		item.Pinned = new(1)
		item.PinnedAt = new(time.Now().Unix())
		item.PinnedBy = new(int(middleware.GetUserID(c)))
	} else {
		item.Pinned = new(0)
		item.PinnedAt = nil
		item.PinnedBy = nil
	}

	// Dùng Select để update cả nil pointer (PinnedAt/PinnedBy → NULL khi unpin)
	if err := tenant.ScopedDBNullBranch(c, h.DB, "note").
		Model(&item).
		Select("pinned", "pinned_at", "pinned_by").
		Updates(item).Error; err != nil {
		response.ServerError(c, err)
		return
	}

	response.Updated(c, item)
}
