// Helpers — private ownership guards.
//
// Story-37 (E6/D15 — 1 thương hiệu/DB): bỏ `companyAllowed` / `verifyPaymentCompanyMatches` (kiểm
// company_id payload) — BE không nhận company_id từ client nữa (DTO bỏ field ⇒ JSON thừa bị bỏ qua);
// schema mới (story-62) không còn cột này.
package handler

import (
	"time"

	"erpCluvix/internal/middleware"
	paymentDao "erpCluvix/internal/modules/webapp/crm/payment/dao"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"
	"erpCluvix/pkg/tz"

	"github.com/gin-gonic/gin"
)

// verifyPaymentOwnership scope-check payment id thuộc tenant hiện tại
// trước khi gọi service (service không có *gin.Context).
func (h *Handler) verifyPaymentOwnership(c *gin.Context, id uint) bool {
	var p paymentDao.Payment
	if err := h.paymentScope(c).First(&p, id).Error; err != nil {
		response.NotFound(c)
		return false
	}
	return true
}

// verifyCanEditPaymentCustomer kiểm tra user có quyền sửa thanh toán của khách.
//
// Logic (mirror FE checkPaymentUpdate):
//   - Có quyền updatePayment → luôn được phép.
//   - Không có quyền updatePayment → phải đủ CẢ 4 điều kiện:
//     1. Có quyền createPayment (prerequisite).
//     2. user_option id=1 (OPTION_UPDATE_PAYMENT) = 1 (bật).
//     3. Là người tạo ra payment đó (created_by = userID).
//     4. Vẫn trong cùng ngày tạo (theo timezone của thương hiệu).
//
// Không lọc chi nhánh: phiếu của khách sửa được ở mọi chi nhánh (option 23 cũ, đã gỡ — luôn cho phép).
// Trả về branch_id HIỆN TẠI của phiếu để caller chỉ kiểm branch khi FE đổi sang chi nhánh khác.
func (h *Handler) verifyCanEditPaymentCustomer(c *gin.Context, id uint) (int, bool) {
	var p paymentDao.Payment
	if err := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "payment", true).First(&p, id).Error; err != nil {
		response.NotFound(c)
		return 0, false
	}

	userID := middleware.GetUserID(c)

	if h.hasPermission(userID, "updatePayment") {
		return p.BranchId, true
	}

	if !h.hasPermission(userID, "createPayment") {
		response.Forbidden(c)
		return 0, false
	}

	var optionVal int
	h.DB.Table("user_option").Where("id = 1").Limit(1).Pluck("options", &optionVal)
	if optionVal != 1 {
		response.Forbidden(c)
		return 0, false
	}

	if p.CreatedBy == nil || *p.CreatedBy != userID {
		response.Forbidden(c)
		return 0, false
	}

	now := time.Now().In(tz.LocFromContext(c, h.DB))
	createdAt := time.Unix(p.CreatedAt, 0).In(now.Location())
	if now.Year() != createdAt.Year() || now.Month() != createdAt.Month() || now.Day() != createdAt.Day() {
		response.Forbidden(c)
		return 0, false
	}

	return p.BranchId, true
}

// fkBranch — branch_id THẬT của 1 record cha (order_plan, order_service_used, customer, payment...)
// khi record đó tồn tại. ok=false khi record không tồn tại — dùng để CHẶN write tới FK rác và KHÔNG
// tin branch_id từ payload FE cho các endpoint tạo payment có FK tới record cha (Hard Rule
// "branch_id khi CREATE record con có FK tới record cha").
//
// CỐ Ý không lọc chi nhánh (tenant.CrossBranchLookup — thay helper company-only cũ, story-37 D15):
// đơn/dịch vụ/khách/phiếu cọc có thể ở chi nhánh khác chi nhánh active (thanh toán chéo chi nhánh).
func (h *Handler) fkBranch(_ *gin.Context, table string, id int) (branchID int, ok bool) {
	var row struct {
		BranchId *int `gorm:"column:branch_id"`
	}
	if err := tenant.CrossBranchLookup(h.DB, table).
		Table(table).
		Select("branch_id").
		Where("id = ?", id).
		Take(&row).Error; err != nil {
		return 0, false
	}
	if row.BranchId != nil {
		branchID = *row.BranchId
	}
	return branchID, true
}

// serviceUsedIDsInTenant — TẤT CẢ id trong danh sách phải tồn tại (dùng cho CreatePaymentCustomer
// thanh toán nhiều dịch vụ cùng lúc — count phải khớp đúng len(ids), thiếu 1 id nghĩa là có id rác
// lẫn vào). CỐ Ý không lọc chi nhánh (tenant.CrossBranchLookup — dịch vụ có thể ở chi nhánh khác).
func (h *Handler) serviceUsedIDsInTenant(_ *gin.Context, ids []int) bool {
	if len(ids) == 0 {
		return false
	}
	var count int64
	tenant.CrossBranchLookup(h.DB, "order_service_used").
		Table("order_service_used").
		Where("id IN ?", ids).
		Count(&count)
	return count == int64(len(ids))
}

// branchAllowed: true khi branchID = 0 (unassigned) hoặc nằm trong branch list của user.
func branchAllowed(c *gin.Context, branchID int) bool {
	if branchID == 0 {
		return true
	}
	if active := middleware.GetBranchActiveID(c); active > 0 {
		return int(active) == branchID
	}
	for _, bid := range middleware.GetBranchIDs(c) {
		if int(bid) == branchID {
			return true
		}
	}
	return false
}

// hasPermission kiểm tra user có permission code nhất định (qua RbacSvc với Redis cache).
func (h *Handler) hasPermission(userID uint, permCode string) bool {
	if h.RbacSvc == nil {
		return false
	}
	perms, _ := h.RbacSvc.GetUserPermissions(userID)
	for _, p := range perms {
		if p == permCode {
			return true
		}
	}
	return false
}
