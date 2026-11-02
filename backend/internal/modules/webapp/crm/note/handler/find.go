package handler

import (
	"net/http"
	"strconv"

	"erpCluvix/internal/modules/webapp/crm/customer/scope"
	exportHandlers "erpCluvix/internal/modules/webapp/export"
	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/handlerutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
)

// noteColumns — danh sách cột tường minh thay cho `SELECT *` (Hard Rule).
// Trùng khít 9 cột của bảng `note`; migration thêm cột mới phải thêm vào đây.
const noteColumns = "note.id, note.customer_id, note.note, " +
	"note.pinned, note.pinned_at, note.pinned_by, note.created_by, note.created_at"

// ListAdvise GET /crm/note/list-advise?id=347
// List notes của 1 customer cụ thể, sort pinned + recent.
func (h *Handler) ListAdvise(c *gin.Context) {
	// Parse ra SỐ trước khi vào query: MySQL ép '347abc' → 347 (warning 1292 bị nuốt)
	// nên chuỗi rác sẽ trả về ghi chú của khách 347. Xem Hard Rule "Path param id PHẢI parse ra số".
	customerID, err := strconv.ParseUint(c.Query("id"), 10, 64)
	if err != nil || customerID == 0 {
		response.Validate(c, i18n.T(c, "id_required"))
		return
	}

	q := tenant.ScopedDBNullBranch(c, h.DB, "note").Table("note").
		Select(noteColumns).
		Where("customer_id = ?", customerID).
		Order("pinned DESC, pinned_at DESC, id DESC")
	// S-10: phạm vi telesale — khách ngoài phạm vi ⇒ danh sách rỗng; lỗi đọc phạm vi ⇒ 500.
	q = scope.WhereCustomerID(c, h.DB, q, "note.customer_id")

	var items []map[string]interface{}
	result := pagination.Paginate(c, q, &items)
	c.JSON(http.StatusOK, result)
}

// noteDateIndexHint / noteHintMaxWindowSec — xem `maybeHintDateIndex`.
const (
	noteDateIndexHint    = "idx_note_created_at"
	noteHintMaxWindowSec = int64(31 * 24 * 60 * 60)
)

// maybeHintDateIndex quyết định có ép MySQL đi theo chỉ mục ngày hay không.
//
// Vấn đề: với `_keyword`, câu SELECT là `… LIKE '%kw%' ORDER BY note.id DESC LIMIT 20`. Optimizer
// BỎ chỉ mục ngày để cược vào early-exit: quét NGƯỢC `fk_note-company_id` và dừng khi đủ 20 dòng
// khớp. Từ khoá phổ biến ⇒ cược thắng (1,3ms); từ khoá hiếm/không khớp ⇒ đi hết 1,34 triệu dòng
// (2,8–3,2s). Chi phí vì thế là BIMODAL và người dùng không đoán được.
//
// `USE INDEX` biến nó thành hằng số theo ĐỘ RỘNG CỬA SỔ NGÀY (đo @2.527.503 ghi chú, 2026-09-09,
// median 3 lượt, `EXPLAIN ANALYZE`):
//
//	cửa sổ         dòng trong cửa sổ   từ khoá HIẾM            từ khoá PHỔ BIẾN
//	1 tháng (mặc định FE)      1.896   3.204ms -> 9,4ms (340×)  1,3ms -> 8,8ms  (phạt 7,5ms)
//	92 ngày                   39.786   2.925ms -> 190ms  (15×)  1,2ms -> 185ms  (phạt 185ms)
//	1 năm                    296.755   3.560ms -> 690ms  (5,2×) 1,3ms -> 680ms  (phạt 680ms)
//
// ⇒ chỉ hint trong cửa sổ ≤ 31 ngày (đúng mặc định FE): đổi 7,5ms lấy 3,2 GIÂY. Rộng hơn thì phần
// phạt cho ca may vượt phần cứu cho ca xui ⇒ để nguyên optimizer tự quyết, KHÔNG cắt kết quả.
//
// ⚠ Biến thể đã LOẠI: `FORCE INDEX`. Bằng số dòng với `USE` ở cửa sổ hẹp, nhưng ở cửa sổ ~toàn
// bảng (5 năm) nó đọc 4.996.165 dòng — TỆ HƠN cả không hint (2.527.504) vì phải quét chỉ mục rồi
// tra dòng; `USE` thì optimizer được phép bỏ hint nên thoái lui êm (2.527.506).
//
// ⚠ Tên chỉ mục nằm trong SQL: đổi tên/xoá chỉ mục ⇒ MySQL trả lỗi 1176 (LỚN TIẾNG, không phải
// hỏng im lặng), và `TestNoteDateIndexHintKhopVoiMigration` đỏ ngay trong repo.
func maybeHintDateIndex(c *gin.Context, hasKeyword bool) bool {
	if !hasKeyword {
		return false
	}
	from, errF := strconv.ParseInt(query.GetOperatorValue(c, "created_at", "gte"), 10, 64)
	to, errT := strconv.ParseInt(query.GetOperatorValue(c, "created_at", "lte"), 10, 64)
	if errF != nil || errT != nil || to < from {
		return false
	}
	return to-from <= noteHintMaxWindowSec
}

// GetAllNote GET /crm/note/get-all-note
// List notes cho admin overview — filter qua customer table (branch/type/source).
func (h *Handler) GetAllNote(c *gin.Context) {
	q := tenant.ScopedDBNullBranch(c, h.DB, "note").Table("note")
	// S-10: phạm vi telesale — chỉ ghi chú của khách mình phụ trách (cả list lẫn export).
	q = scope.WhereCustomerID(c, h.DB, q, "note.customer_id")

	// Range filters
	q = query.ApplyFilter(c, q, "created_at", "note.created_at")

	// Exact filters
	q = query.ApplyFilter(c, q, "created_by", "note.created_by")

	// Customer-table filters chỉ join customer khi cần
	if query.GetFilterValue(c, "branch_id") != "" || query.GetFilterValue(c, "type_service_id") != "" ||
		query.GetFilterValue(c, "_stage_id") != "" || query.GetFilterValue(c, "source_id") != "" ||
		c.Query("filter[branch_id]") != "" || c.Query("filter[type_service_id]") != "" ||
		c.Query("filter[_stage_id]") != "" || c.Query("filter[source_id]") != "" {
		q = q.Joins("LEFT JOIN customer ON customer.id = note.customer_id")
		q = query.ApplyFilter(c, q, "branch_id", "customer.branch_id")
		q = query.ApplyFilter(c, q, "type_service_id", "customer.type_service_id")
		// Bước khách hàng (customer.stage_id) — thay lọc tiến trình cũ của khách (story-05; cột cũ bỏ ở story-12).
		q = query.ApplyFilter(c, q, "_stage_id", "customer.stage_id")
		q = query.ApplyFilter(c, q, "source_id", "customer.source_id")
	}

	// Keyword
	hasKeyword := false
	if kw := c.Query("_keyword"); kw != "" {
		hasKeyword = true
		like := "%" + kw + "%"
		q = q.Where("note.note LIKE ?", like)
	}

	q = query.ApplySort(c, q, "note.id DESC", "note.id")

	if c.Query("_export") == "1" {
		exportHandlers.ExportNote(c, q)
		return
	}

	// Hint đặt SAU nhánh export: `_export=1` không có `LIMIT` nên không dính ván cược early-exit,
	// và đường đó chưa được đo với hint.
	if maybeHintDateIndex(c, hasKeyword) {
		q = q.Table("note USE INDEX (" + noteDateIndexHint + ")")
	}

	var items []map[string]interface{}
	q = q.Select(noteColumns)
	result := pagination.Paginate(c, q, &items)

	// Luôn attach customer — FE cần hiển thị mã KH, tên, SĐT, tiến trình.
	if len(items) > 0 {
		customerIds := handlerutil.CollectIDs(items, "customer_id")
		if len(customerIds) > 0 {
			// Khách của ghi chú CỐ Ý không lọc chi nhánh (story-36: CrossBranchLookup thay CompanyOnly).
			handlerutil.AttachRelationScoped(tenant.CrossBranchLookupScope, c, h.DB, items, "customer_id", "customer",
				"customer", "id", "id, code, fullname, nickname, phone, gender, avatar, stage_id, branch_id, province_id, source_id", customerIds)
			phoneutil.MaskNestedCustomerPhone(c, h.DB, items, "customer")
		}
	}

	c.JSON(http.StatusOK, result)
}

// GetNote GET /crm/note/:id
func (h *Handler) GetNote(c *gin.Context) {
	// Parse id ra số trước khi vào query — xem ghi chú ở ListAdvise.
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	var item map[string]interface{}
	if err := tenant.ScopedDBNullBranch(c, h.DB, "note").Table("note").
		Select(noteColumns).
		Where("id = ?", id).Limit(1).Scan(&item).Error; err != nil {
		response.NotFound(c)
		return
	}
	if len(item) == 0 {
		response.NotFound(c)
		return
	}
	helpers.ParseJSONFieldForTable(item, "note")
	response.Data(c, item)
}
