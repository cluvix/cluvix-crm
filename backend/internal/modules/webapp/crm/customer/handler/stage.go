package handler

import (
	"errors"

	"erpCluvix/internal/middleware"
	stagesvc "erpCluvix/internal/modules/webapp/config/customer_stage/service"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/response"

	"github.com/gin-gonic/gin"
)

// changeStageBody — chỉ nhận đúng 1 trường. *int: thiếu/null ⇒ nil; chuỗi/số thực ⇒ lỗi bind.
type changeStageBody struct {
	StageID *int `json:"stage_id"`
}

// ChangeStage PUT /crm/customer/:id/stage {stage_id} — đổi bước tiến trình (cửa ghi bước duy nhất của
// người dùng; form sửa khách bỏ qua stage_id). Trả {id, stage_id, updated_at}.
//
//	:id không phải số dương / khách ngoài phạm vi của GetCustomer/UpdateCustomer ⇒ 404 (không lộ tồn tại)
//	stage_id không phải số / bước ẩn / không tồn tại ⇒ 422
func (h *Handler) ChangeStage(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	var body changeStageBody
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Validate(c, i18n.T(c, "invalid_data"))
		return
	}
	if body.StageID == nil || *body.StageID <= 0 {
		response.Validate(c, i18n.T(c, stagesvc.KeyNotSelectable))
		return
	}

	// Đổi bước là một đường UPDATE khách ⇒ CÙNG phạm vi với GetCustomer/UpdateCustomer: customerByIDScope
	// (chi nhánh, bỏ lọc khi option cross-branch id=18 bật) + phạm vi telesale như GetCustomer (khách
	// telesale không mở được thì cũng không đổi bước được). KHÔNG dùng phạm vi riêng: lệch phạm vi ⇒
	// khách mở/sửa được mà đổi bước 404 (Story Change Log 02/11/2026). Lỗi đọc phạm vi ⇒ fail-closed.
	scoped, err := applyTelesaleScope(c, h.DB, h.customerByIDScope(c).Table("customer"))
	if err != nil {
		response.ServerError(c, err)
		return
	}
	item, err := h.CustomerSvc.ChangeStage(scoped, int(id), *body.StageID, int(middleware.GetUserID(c)))
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrNotFound):
			response.NotFound(c)
		case isLocalized(err):
			response.HandleError(c, err) // 422 + i18n
		default:
			response.ServerError(c, err)
		}
		return
	}
	response.DataWithMessage(c, item, i18n.T(c, "customer_stage_changed"))
}

func isLocalized(err error) bool {
	_, ok := errors.AsType[*errs.LocalizedError](err)
	return ok
}
