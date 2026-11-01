package handler

import (
	"github.com/gin-gonic/gin"

	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/handlerutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"
)

// paymentDetailColumns — cột của PHIẾU THU cho màn chi tiết.
//
// Endpoint riêng `/:id/detail`, KHÔNG dùng lại `GET /crm/payment/:id`: cái kia trả nguyên entity
// `paymentDao.Payment` và đang phục vụ các form sửa phiếu — đổi hình dạng ở đó là đụng luồng ghi.
// ⚠ KHÔNG có `revenue_id`, `doctorRevenue_id`, `source_id` (+ cột direct-sale cũ): các cột này CÓ trong
// baseline nhưng đã bị migration sau đó DROP khỏi `payment`. Viết Select theo baseline là câu SQL
// chết (Unknown column ⇒ 500 mọi request) — integration test bắt được đúng lỗi này. Tư vấn viên /
// bác sĩ của phiếu lấy từ `orderServiceUsed` như `PaymentService.addLabel` bên FE đang làm.
const paymentDetailColumns = `id, code, customer_id, parent_id, serviceUsed_id, appointment_id,
	branch_id,
	method, card_type_id, bank_code,
	payment_time, payment_of_date, amount, deposit_after, deposit_id, deposit_type, payment_for,
	note, created_by, created_at, updated_by, updated_at`

// paymentAllocationColumns — cột cho DÒNG PHÂN BỔ (chính là các phiếu thu khác cùng đơn hàng).
//
// `osu.service_id` lấy qua JOIN: tên dịch vụ nằm sau 2 lớp (payment → order_service_used → service),
// không có nó thì cột "dịch vụ" của bảng phân bổ trống ở mọi dòng — mất đúng câu trả lời chính
// ("tiền của đơn chảy vào dịch vụ nào"). FE tra tên từ cache IndexedDB theo `service_id`.
const paymentAllocationColumns = `p.id, p.code, p.serviceUsed_id, p.amount, p.payment_for, p.method,
	p.payment_time, p.created_at, osu.service_id AS service_id`

// GetPaymentDetail GET /crm/payment/:id/detail
//
// ⚠ VỀ "PHÂN BỔ" — đọc trước khi sửa: hệ thống KHÔNG có bảng phân bổ riêng. Khi thu tiền cho đơn,
// `allocatePaymentWaterfall` (internal/modules/webapp/crm/service/payment_crud.go) chia số tiền theo
// thứ tự `created_day` của từng dịch vụ rồi **tạo NHIỀU dòng `payment`**, mỗi dòng gắn một
// `serviceUsed_id` và có mã riêng (cột `code` UNIQUE nên không thể dùng chung mã).
//
// Vì vậy "bảng phân bổ" ở đây = **toàn bộ phiếu thu cùng `parent_id` (cùng đơn hàng)**, dòng đang
// xem là một trong số đó. Đó là sự thật có trong DB; đừng dựng một khái niệm "nhóm phiếu" không tồn
// tại bằng cách gom theo `payment_time`/`created_at` — 2 lần thu khác nhau trong cùng một giây sẽ
// dính vào nhau, và tiền là chỗ không được đoán.
func (h *Handler) GetPaymentDetail(c *gin.Context) {
	// id sai định dạng là INPUT SAI → 422; 404 dành cho id hợp lệ mà không tồn tại/ngoài tenant.
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_id"))
		return
	}

	scoped := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "payment", tenant.IsCrossBranchServiceEnabled(c, h.DB))

	var items []map[string]interface{}
	tx := scoped.Table("payment").
		Select(paymentDetailColumns).
		Where("id = ?", id).Limit(1).Find(&items)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	if len(items) == 0 {
		response.NotFound(c)
		return
	}
	helpers.ParseJSONFieldsForTables(items, "payment")
	helpers.ParseDecimalFieldsForTables(items, "payment")
	item := items[0]

	h.attachPaymentRelations(c, items)
	if !h.attachPaymentAllocations(c, item) {
		return
	}

	response.Data(c, item)
}

// attachPaymentRelations gắn khách hàng, đơn hàng, dịch vụ, lịch hẹn của phiếu thu — CỐ Ý không lọc
// chi nhánh (tenant.CrossBranchLookupScope, thay company-only — story-37 D15): các record này có thể ở
// chi nhánh khác chi nhánh của phiếu.
// Không trả lỗi: các helper `AttachRelation*` tự nuốt/không sinh lỗi truy vấn (khác
// `attachPaymentAllocations` — chỗ đó có `FailQuery` thật).
func (h *Handler) attachPaymentRelations(c *gin.Context, items []map[string]interface{}) {
	if customerIds := handlerutil.CollectIDs(items, "customer_id"); len(customerIds) > 0 {
		handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "customer_id", "customer",
			"customer", "id", "id, code, old_code, fullname, nickname, phone", customerIds)
		phoneutil.MaskNestedCustomerPhone(c, h.DB, items, "customer")
	}
	// `parent_id` là FK sang đơn hàng (tên cột lịch sử từ Yii2, không phải quan hệ cha-con của payment).
	if planIds := handlerutil.CollectIDs(items, "parent_id"); len(planIds) > 0 {
		handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "parent_id", "orderPlan",
			"order_plan", "id", "id, code, orderPlanType", planIds)
	}
	if suIds := handlerutil.CollectIDs(items, "serviceUsed_id"); len(suIds) > 0 {
		handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "serviceUsed_id", "serviceUsed",
			// vat_amount (bmad/order-vat): phải thu dòng = totalEnd + vat_amount — FE so với payment ra còn nợ.
			"order_service_used", "id", "id, code, service_id, totalEnd, vat_amount, payment", suIds)
	}
	if apptIds := handlerutil.CollectIDs(items, "appointment_id"); len(apptIds) > 0 {
		handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "appointment_id", "appointment",
			"appointment", "id", "id, code, appointment_date", apptIds)
	}
}

// attachPaymentAllocations gắn `allocations` = mọi phiếu thu cùng đơn hàng (xem docblock endpoint).
// Phiếu không gắn đơn (thu/chi phòng khám, cọc lẻ) → mảng RỖNG, không phải null.
func (h *Handler) attachPaymentAllocations(c *gin.Context, item map[string]interface{}) bool {
	rows := make([]map[string]interface{}, 0)
	item["allocations"] = rows

	parentID := helpers.ToInt64(item["parent_id"])
	if parentID <= 0 {
		return true
	}

	// ⚠ Có alias `p` trong Table() ⇒ PHẢI truyền alias cho ScopedDB, nếu không điều kiện scope sinh ra
	// `payment.company_id` trong khi FROM đã đổi tên → MySQL 1054 (500 mọi request, lỗi runtime).
	tx := tenant.ScopedDBNullBranchSkipBranch(c, h.DB, "payment", tenant.IsCrossBranchServiceEnabled(c, h.DB), "p").
		Table("payment p").
		Select(paymentAllocationColumns).
		Joins("LEFT JOIN order_service_used osu ON osu.id = p.serviceUsed_id").
		Where("p.parent_id = ?", parentID).
		Order("p.payment_time ASC, p.id ASC").
		Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return false
	}
	helpers.ParseDecimalFieldsForTables(rows, "payment")
	item["allocations"] = rows
	return true
}
