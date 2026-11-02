package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"erpCluvix/internal/modules/webapp/crm/customer/scope"
	"erpCluvix/internal/phoneutil"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/query"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/storage"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
)

var reminderKeySearch = []string{
	"reminder.code", "reminder.note", "customer.code", "customer.fullname", "customer.phone",
}

// ───────────────────────────────────────────────────────────────────────────────
// Tối ưu màn danh sách lời nhắc. Số đo trên DB clone schema y hệt (mysqldump --no-data),
// 524.288 lời nhắc / 100.000 khách, MySQL 9.7 (2026-09-07). ~30ms mỗi số là overhead kết
// nối mysql client; máy đo đang tải nặng nên con số tuyệt đối nhiễu, tỉ lệ thì ổn định.
//
//  1. BỎ `GROUP BY reminder.id`. Join là `customer.id = reminder.customer_id` — khoá chính
//     bên phải nên KHÔNG THỂ nhân dòng ⇒ GROUP BY không loại được dòng nào, chỉ tốn tiền.
//     Nó ép MySQL dựng "Temporary table with deduplication" trên TOÀN BỘ tập khớp (471.860
//     dòng, kèm cột TEXT `note`) rồi mới sort + LIMIT 20. EXPLAIN trước khi bỏ:
//         -> Sort: reminder.id DESC, limit input to 20 row(s) per chunk
//             -> Table scan on <temporary>  (cost=192315 rows=259644)
//                 -> Temporary table with deduplication
//     Không lọc, trang 1:  SELECT 5,07–6,08s → 1,73–1,92s  ·  COUNT 7,05–7,92s → 4,13–5,83s
//     Đối chiếu 13 bộ lọc/từ khoá: số dòng trả về giống hệt trước/sau (xem gate bên dưới).
//
//  2. COUNT của phân trang KHÔNG đi qua `pagination.Paginate` nữa.
//     `Paginate` chạy `SELECT COUNT(*) FROM (<nguyên query>) AS cnt` — bọc cả danh sách cột
//     lẫn LEFT JOIN lẫn ORDER BY vào derived table rồi đếm, tức trả giá join hai lần mỗi
//     lần load. Ở đây COUNT chỉ cần bảng `reminder`; `customer` chỉ bị đọc bởi từ khoá và
//     `_groupCountry` nên chỉ join KHI CÓ hai thứ đó (needsCustomerJoin).
//     MySQL KHÔNG tự loại LEFT JOIN thừa (khác SQL Server/Oracle) nên phải bỏ bằng tay.
//     Không lọc:        COUNT 4,13–5,83s → 0,10–0,12s
//     Trang 1000 sâu:   COUNT 10,1–10,4s → 0,10–0,11s
//     ⚠ SELECT thì LUÔN phải join — FE đọc 7 cột customer (gồm customer_stage_id) để dựng object lồng bên dưới.
//
//  3. Chỉ mục dẫn đầu `start_time` — đo hồi ERP trên `(company_id, start_time)` (migration
//     20260907071500); CRM 1 thương hiệu/DB dùng `idx_reminder_start_status (start_time, status)`. `start_time` trước đó KHÔNG có chỉ mục nào, mà đó là bộ
//     lọc mặc định của màn (FE gửi filter[start_time][gte|lte] = đầu tháng → hết hôm nay).
//     Khoảng 1 tháng (43.200 dòng khớp), sau khi đã có (1)+(2):
//         SELECT 2,66–10,16s → 0,64–0,66s   ·   COUNT 1,13–3,37s → 0,04–0,23s
//     Khoảng 1 ngày (1.440 dòng):  SELECT 0,58–0,68s → 0,02s  ·  COUNT 0,61–0,72s → 0,04s
//     Chi phí: +2,9s dựng trên 524k dòng; index mới 13,5MB (đo qua `mysql.innodb_index_stats`
//     sau ANALYZE TABLE), so với 46,5MB của 5 secondary index sẵn có.
//
//  4. Tìm kiếm từ khoá đi qua FULLTEXT ngram thay cho `LIKE '%kw%'` xuyên join
//     (migration 20260907080400 thêm `ft_reminder_note_ngram`). Xem applyReminderKeywordFast.
//     `%` đứng đầu ⇒ không chỉ mục nào dùng được, và `reminder.note` là TEXT tự do nên MySQL
//     quét toàn bảng bất kể ta làm gì với join. Đo tách riêng lúc điều tra: bỏ hẳn `note LIKE`
//     khỏi mệnh đề OR đưa COUNT từ ~4,5s về 0,32–0,46s ⇒ `note` là nút thắt, KHÔNG phải join.
//
//  5. Deferred join: tra id của trang bằng query gọn rồi mới nạp đủ cột cho ≤20 id đó.
//     MySQL sort TRƯỚC khi join và payload filesort là `reminder.*` (gồm TEXT `note`) + 7 cột
//     customer. Sort 471.860 dòng payload rộng = 1,73–1,92s; sort cùng tập chỉ mang `id`
//     (index phủ) = 0,12–0,18s. Xem buildReminderPageIDQuery / buildReminderRowQuery.
//
//  6. `_groupCountry` trỏ nhóm rỗng nay trả 0 dòng thay vì bỏ qua điều kiện (sai im lặng).
//
// ── Tổng kết đo TOÀN BỘ endpoint (COUNT + tra id + nạp dòng), chạy chính code này trên bench:
//     1 tháng (mặc định FE)    ~4–13s  → 0,13–0,33s
//     1 ngày                   ~1,3s   → 0,00–0,02s
//     không lọc                ~12s    → 0,18–0,39s
//     trang 1000               ~15s    → 0,22–0,45s
//     từ khoá KHÔNG khớp       ~9,3s   → 0,24–0,41s
//     từ khoá CÓ khớp          ~10,5s  → 2,7–6,3s   ⟵ xem cảnh báo dưới
// Máy đo bị tải nặng (load average 8–29) trong phần lớn phiên đo nên con số tuyệt đối dao động
// mạnh giữa các lượt; tỉ lệ trước/sau thì ổn định qua mọi lượt.
//
// ⚠ CÒN LẠI — từ khoá khớp RỘNG vẫn chậm. Một họ phổ biến khớp 8.333/100.000 khách ⇒ danh sách
// 8.333 id truyền vào `IN (...)`, MySQL bỏ chỉ mục `fk-reminder-customer_id` và quét 471k dòng
// lọc trong bộ nhớ (`eq_range_index_dive_limit` mặc định 200 ⇒ không index-dive, ước lượng sai).
// Hai cách chữa ĐÃ THỬ VÀ LOẠI, số đo đầy đủ ở đầu migration 20260907080400 — đừng làm lại:
// thêm index `(customer_id, company_id)` (thắng là do optimizer đổi plan tình cờ, ép dùng chính
// nó lại CHẬM HƠN) và `FORCE INDEX (fk-reminder-customer_id)` (vẫn > 0,5s + hint cứng).
//
// ⚠ Từ khoá vượt trần `reminderKeywordIDCap` hoặc không an toàn với ngram RƠI VỀ đường LIKE cũ
// (6–9,7s ở bench). Đó là CỐ Ý: chậm-mà-đúng, không cắt bớt kết quả.
// ───────────────────────────────────────────────────────────────────────────────

// reminderCustomerJoin — mệnh đề join dùng chung cho cả COUNT lẫn SELECT. Một chuỗi duy
// nhất để hai đường không bao giờ lệch điều kiện join.
const reminderCustomerJoin = "LEFT JOIN customer ON customer.id = reminder.customer_id"

// buildReminderBaseQuery dựng phần thân query (scope + mọi filter trên bảng `reminder`) và
// cho biết có điều kiện nào ĐỌC cột `customer` hay không.
//
// Gọi hai lần (một cho COUNT, một cho SELECT) thay vì tái dùng một `*gorm.DB`: chain GORM
// mang trạng thái, dùng lại một query đã gắn Select/Joins cho mục đích khác là lớp bug đã
// biết trong repo này (xem skill `debugging-backend-queries`, mục GORM read/write pitfalls).
//
// ⚠ Thêm filter mới đụng `customer.*` thì PHẢI bật needsJoin ở đây, nếu không COUNT sẽ vỡ
// với "Unknown column 'customer.x'".
func (h *Handler) buildReminderBaseQuery(c *gin.Context) (q *gorm.DB, needsJoin bool) {
	q = tenant.ScopedDBNullBranch(c, h.DB, "reminder").Table("reminder")

	q = query.ApplyFilter(c, q, "start_time", "reminder.start_time")
	q = query.ApplyFilter(c, q, "created_at", "reminder.created_at")
	q = query.ApplyFilter(c, q, "updated_at", "reminder.updated_at")

	q = query.ApplyFilter(c, q, "status", "reminder.status")
	q = query.ApplyFilter(c, q, "type", "reminder.type")
	q = query.ApplyFilter(c, q, "customer_id", "reminder.customer_id")
	q = query.ApplyFilter(c, q, "created_by", "reminder.created_by")

	// S-10: phạm vi telesale — áp ở gốc nên câu đếm và câu tra id cùng một tập.
	q = whereReminderCustomerScope(c, h.DB, q)

	// _relatedUser: JSON_OVERLAPS
	if vals := query.GetFilterValues(c, "_relatedUser"); len(vals) > 0 {
		idsJSON, _ := json.Marshal(vals)
		q = q.Where("JSON_OVERLAPS(reminder.relatedUser, CAST(? AS JSON))", string(idsJSON))
	}

	// _groupCountry — đọc `customer.country_id` ⇒ bắt buộc join.
	//
	// ⚠ Nhóm quốc gia rỗng (chưa gán quốc gia nào, hoặc id nhóm không tồn tại) ⇒ PHẢI trả 0 dòng.
	// Bản cũ bỏ qua luôn điều kiện khi `countryIds` rỗng, tức người dùng lọc "nhóm X" lại nhận
	// NGUYÊN danh sách như thể không lọc gì — sai im lặng, không lỗi, không cảnh báo. Cùng lớp
	// với luật `1 = 0` ở đường từ khoá bên dưới.
	if vals := query.GetFilterValues(c, "_groupCountry"); len(vals) > 0 {
		countryIds := query.GetCountryIdsByGroupCountry(h.DB, vals)
		//
		// KHÔNG `return` sớm ở đây: từ khoá phía dưới vẫn có thể bật needsJoin, mà
		// ApplyKeywordFilter tham chiếu `customer.code/fullname/phone` ⇒ thiếu join là lỗi
		// "Unknown column", không phải chỉ thừa một điều kiện.
		if len(countryIds) == 0 {
			q = q.Where("1 = 0")
		} else {
			q = q.Where("customer.country_id IN ?", countryIds)
			needsJoin = true
		}
	}

	// ⚠ Từ khoá KHÔNG bật needsJoin ở đây. Nó chỉ cần join khi đường ngram không đi được và phải
	// rơi về `LIKE` xuyên join — quyết định đó nằm ở buildReminderFilteredQuery. Bật sẵn ở đây
	// (bản đầu của bản vá này đã làm vậy) khiến MỌI lần tìm kiếm vẫn trả giá join, tức mất đúng
	// phần lợi mà đường nhanh sinh ra.

	return q, needsJoin
}

// reminderKeywordIDCap — trần số id nạp về cho đường nhanh. Vượt trần ⇒ rơi về đường LIKE cũ
// (chậm nhưng ĐÚNG) thay vì cắt bớt danh sách. Cắt bớt = trả thiếu kết quả im lặng.
//
// Giữ riêng ở đây, KHÔNG gom vào pkg/query: mỗi màn tra id trên bộ bảng phụ khác nhau nên trần
// có thể phải chỉnh lệch (xem chú thích đầu pkg/query/ngram.go).
const reminderKeywordIDCap = 10000

// applyReminderKeywordFast thử áp bộ lọc từ khoá bằng đường ngram thay cho `LIKE '%kw%'`.
//
// Trả (query đã gắn điều kiện, true) khi đi được đường nhanh — gọi bên ngoài KHÔNG cần join
// customer cho từ khoá. Trả (query nguyên trạng, false) khi phải rơi về đường LIKE cũ — gọi bên
// ngoài PHẢI tự gắn LEFT JOIN + ApplyKeywordFilter.
//
// Ba nguồn id, tương ứng ĐÚNG 5 cột của reminderKeySearch:
//   - customer.code/fullname/phone → MATCH `customer.search_ngram` (cột sinh gộp 6 cột) rồi AND
//     LIKE lại trên đúng 3 cột đó. AND LIKE là BẮT BUỘC: search_ngram phủ RỘNG hơn (thêm
//     nickname/other_phone/old_code) nên thiếu nó là THỪA kết quả so với đường cũ; và parser
//     ngram bỏ mọi ký tự phân cách nên "ID Q1 KH" khớp cả "ID.Q1.KH".
//   - reminder.note → MATCH ngram + AND LIKE, cùng lý do.
//   - reminder.code → LIKE, KHÔNG ngram. Bài học đã đo ở `crm/appointment`: mọi mã dùng chung
//     bigram nên phrase search phải giao posting list gần-như-toàn-bảng, đắt hơn quét covering
//     index `code`. ngram hợp văn bản tự do, không hợp mã.
func (h *Handler) applyReminderKeywordFast(c *gin.Context, q *gorm.DB) (*gorm.DB, bool) {
	kw := strings.TrimSpace(c.Query("_keyword"))
	if kw == "" {
		return q, true // không có từ khoá ⇒ không cần join, coi như xong
	}
	if utf8.RuneCountInString(kw) < query.NgramMinLen || !query.NgramPhraseSafe(kw) {
		return q, false
	}

	phrase := query.NgramPhrase(kw)
	like := "%" + kw + "%"

	// KHÔNG scope tenant ở các truy vấn tra id — CỐ Ý, giống `crm/appointment`. Đường cũ join
	// `customer` trần và lọc `note`/`code` trần; mọi lọc tenant nằm ở query ngoài trên `reminder`.
	// Thêm scope vào đây sẽ làm tập kết quả KHÁC đường cũ.
	var custIDs []int64
	if err := h.DB.Table("customer").
		Where("MATCH(search_ngram) AGAINST (? IN BOOLEAN MODE)", phrase).
		Where("code LIKE ? OR fullname LIKE ? OR phone LIKE ?", like, like, like).
		Limit(reminderKeywordIDCap+1).
		Pluck("id", &custIDs).Error; err != nil {
		return q, false // vd lỗi 1191 (thiếu chỉ mục ngram) ⇒ lùi về LIKE, KHÔNG vỡ endpoint
	}
	if len(custIDs) > reminderKeywordIDCap {
		return q, false
	}

	var noteIDs []int64
	if err := h.DB.Table("reminder").
		Where("MATCH(note) AGAINST (? IN BOOLEAN MODE)", phrase).
		Where("note LIKE ?", like).
		Limit(reminderKeywordIDCap+1).
		Pluck("id", &noteIDs).Error; err != nil {
		return q, false
	}
	if len(noteIDs) > reminderKeywordIDCap {
		return q, false
	}

	var codeIDs []int64
	if err := h.DB.Table("reminder").
		Where("code LIKE ?", like).
		Limit(reminderKeywordIDCap+1).
		Pluck("id", &codeIDs).Error; err != nil {
		return q, false
	}
	if len(noteIDs)+len(codeIDs) > reminderKeywordIDCap {
		return q, false
	}
	remIDs := append(noteIDs, codeIDs...)

	// Cả hai rỗng ⇒ không có gì khớp. PHẢI trả 0 dòng, KHÔNG được bỏ qua điều kiện
	// (bỏ qua = trả về toàn bộ lời nhắc như thể không tìm gì).
	switch {
	case len(custIDs) == 0 && len(remIDs) == 0:
		return q.Where("1 = 0"), true
	case len(custIDs) == 0:
		return q.Where("reminder.id IN ?", remIDs), true
	case len(remIDs) == 0:
		return q.Where("reminder.customer_id IN ?", custIDs), true
	default:
		return q.Where("reminder.customer_id IN ? OR reminder.id IN ?", custIDs, remIDs), true
	}
}

// buildReminderFilteredQuery — thân query dùng chung cho ĐẾM và cho bước tra id của trang:
// scope + filter + từ khoá, KHÔNG cột rộng, join `customer` chỉ khi thật sự cần.
func (h *Handler) buildReminderFilteredQuery(c *gin.Context) *gorm.DB {
	q, needsJoin := h.buildReminderBaseQuery(c)

	q, fast := h.applyReminderKeywordFast(c, q)
	if !fast {
		// Đường lui: LIKE xuyên join như bản cũ.
		needsJoin = true
	}
	if needsJoin {
		q = q.Joins(reminderCustomerJoin)
	}
	if !fast {
		q = query.ApplyKeywordFilter(c, q, reminderKeySearch...)
	}
	return q
}

// buildReminderCountQuery — query ĐẾM. Chỉ bảng `reminder`; join `customer` khi và chỉ khi có
// điều kiện đọc cột của nó — đó là thứ DUY NHẤT ta phải tự bỏ, vì MySQL không loại LEFT JOIN
// thừa. (Select và Order thì `.Count()` của GORM tự thay/tự bỏ, không cần và không test được.)
//
// ⚠ Hàm này (và buildReminderListQuery) là ĐIỂM DỰNG QUERY DUY NHẤT của endpoint; test
// list_query_test.go khoá hình dạng SQL qua chính hai hàm này. Dựng query thẳng trong
// ListReminder ⇒ test không nhìn thấy, và lớp bug ở đầu file quay lại mà lưới vẫn xanh.
func (h *Handler) buildReminderCountQuery(c *gin.Context) *gorm.DB {
	return h.buildReminderFilteredQuery(c)
}

// reminderRowColumns — danh sách cột của bước nạp dữ liệu. Tách ra hằng số để bước tra id và
// bước nạp không bao giờ lệch nhau.
const reminderRowColumns = "reminder.*, customer.code AS `customer_code`, customer.fullname AS `customer_fullname`, customer.nickname AS `customer_nickname`, customer.phone AS `customer_phone`, customer.gender AS `customer_gender`, customer.avatar AS `customer_avatar`, customer.stage_id AS `customer_stage_id`"

// buildReminderPageIDQuery — BƯỚC 1 của deferred join: lấy id của đúng trang đang xem.
//
// Vì sao tách hai bước: MySQL sort TRƯỚC khi join, và payload của filesort là toàn bộ `reminder.*`
// (gồm cột TEXT `note`) cộng 7 cột customer. Sort tập 471.860 dòng với payload rộng như vậy tốn
// 1,73–1,92s cho trang 1; sort cùng tập đó khi chỉ mang `id` (index phủ) còn 0,12–0,18s.
//
// ⚠ ApplySort phải gọi Ở CẢ hai bước với CÙNG tham số — bước 2 lọc theo `IN (danh sách id)` nên
// không tự có thứ tự; thiếu ORDER BY ở bước 2 là trả đúng bộ dòng nhưng SAI THỨ TỰ, và MySQL
// không có nghĩa vụ giữ thứ tự của mệnh đề IN.
func (h *Handler) buildReminderPageIDQuery(c *gin.Context) *gorm.DB {
	q := h.buildReminderFilteredQuery(c).Select("reminder.id")
	return query.ApplySort(c, q, "reminder.id DESC", "reminder.id")
}

// buildReminderRowQuery — BƯỚC 2 của deferred join: nạp đủ cột cho đúng các id đã chọn.
// Vẫn đi qua tenant scope: id đã được scope ở bước 1, giữ thêm ở đây là lưới thứ hai, gần như
// miễn phí (đã lọc còn ≤ per-page dòng).
func (h *Handler) buildReminderRowQuery(c *gin.Context, ids []int64) *gorm.DB {
	q := tenant.ScopedDBNullBranch(c, h.DB, "reminder").Table("reminder").
		Joins(reminderCustomerJoin).
		Where("reminder.id IN ?", ids).
		Select(reminderRowColumns)
	return query.ApplySort(c, q, "reminder.id DESC", "reminder.id")
}

// reminderScopeCond — seam cho test: DryRun DB của list_query_test không đọc được user_option/user_profile.
var reminderScopeCond = scope.CustomerIDCond

// whereReminderCustomerScope — S-10: telesale chỉ thấy nhắc việc gắn khách mình phụ trách. Nhắc việc
// KHÔNG gắn khách (customer_id NULL — việc nội bộ) không chứa dữ liệu khách nên vẫn hiện. Lỗi đọc
// phạm vi ⇒ query mang lỗi, không trả dòng (fail-closed).
func whereReminderCustomerScope(c *gin.Context, db *gorm.DB, q *gorm.DB) *gorm.DB {
	cond, args, err := reminderScopeCond(c, db, "reminder.customer_id")
	if err != nil {
		_ = q.AddError(err)
		return q
	}
	if cond == "" {
		return q
	}
	return q.Where("(reminder.customer_id IS NULL OR "+cond+")", args...)
}

// ListReminder GET /crm/reminder
// Returns reminders với nested customer object (join + manual nest, không qua attachRelation
// để tránh thêm query — customer fields đã join sẵn).
func (h *Handler) ListReminder(c *gin.Context) {
	var total int64
	if tx := h.buildReminderCountQuery(c).Count(&total); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	page, perPage, offset := pagination.PageParams(c)

	var items []map[string]interface{}
	if total > 0 {
		var ids []int64
		tx := h.buildReminderPageIDQuery(c).Offset(offset).Limit(perPage).Pluck("reminder.id", &ids)
		if pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		if len(ids) > 0 {
			if tx := h.buildReminderRowQuery(c, ids).Find(&items); pagination.IsQueryError(tx.Error) {
				pagination.FailQuery(c, tx.Error)
				return
			}
		}
	}

	result := pagination.ResultForPage(c, &items, total, page, perPage, "reminder", "customer")

	canSeePhone := phoneutil.CanSeePhone(c, h.DB)
	// Build nested customer object — gom 7 flat field thành object rồi xoá.
	for _, item := range items {
		phone, _ := item["customer_phone"].(string)
		if !canSeePhone {
			phone = phoneutil.MaskPhone(phone)
		}
		cust := map[string]interface{}{
			"id":       item["customer_id"],
			"code":     item["customer_code"],
			"fullname": item["customer_fullname"],
			"nickname": item["customer_nickname"],
			"phone":    phone,
			"gender":   item["customer_gender"],
			"avatar":   item["customer_avatar"],
			"stage_id": item["customer_stage_id"],
		}
		storage.ResolveFields(cust, "avatar")
		item["customer"] = cust
		delete(item, "customer_code")
		delete(item, "customer_fullname")
		delete(item, "customer_nickname")
		delete(item, "customer_phone")
		delete(item, "customer_gender")
		delete(item, "customer_avatar")
		delete(item, "customer_stage_id")
	}

	c.JSON(http.StatusOK, result)
}

// ReminderOfCustomer GET /crm/reminder/reminder-of-customer?idCustomer=123
// FE context đã có customer info → KHÔNG attach nested customer.
func (h *Handler) ReminderOfCustomer(c *gin.Context) {
	// Parse ra SỐ trước khi vào query: bind dạng chuỗi thì MySQL ép '777abc' → 777
	// (warning 1292 bị nuốt) ⇒ URL rác trả về lời nhắc của khách KHÁC. Kiểu uint để
	// compiler gác đường cũ, không dựa vào việc nhớ.
	customerID, err := strconv.ParseUint(c.Query("idCustomer"), 10, 64)
	if err != nil || customerID == 0 {
		response.Validate(c, i18n.T(c, "missing_id_customer_param"))
		return
	}

	q := tenant.ScopedDBNullBranch(c, h.DB, "reminder").Table("reminder").
		Where("reminder.customer_id = ?", customerID)
	q = whereReminderCustomerScope(c, h.DB, q)

	q = query.ApplyFilter(c, q, "start_time", "reminder.start_time")
	q = query.ApplyFilter(c, q, "status", "reminder.status")
	q = query.ApplyFilter(c, q, "type", "reminder.type")
	q = query.ApplyFilter(c, q, "created_at", "reminder.created_at")
	q = query.ApplyFilter(c, q, "updated_at", "reminder.updated_at")

	// Relations (user, company) FE lookup từ IndexedDB.
	q = q.Select("reminder.id, reminder.code, reminder.start_time, reminder.note, reminder.status, reminder.type, reminder.relatedUser, reminder.customer_id, reminder.created_by, reminder.created_at, reminder.updated_by, reminder.updated_at")
	q = query.ApplySort(c, q, "reminder.id DESC", "reminder.id")

	var items []map[string]interface{}
	result := pagination.Paginate(c, q, &items)

	for _, item := range items {
		helpers.ParseJSONFieldForTable(item, "reminder")
	}

	c.JSON(http.StatusOK, result)
}

// GetReminder GET /crm/reminder/:id
func (h *Handler) GetReminder(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.NotFound(c)
		return
	}
	var item map[string]interface{}
	if err := tenant.ScopedDBNullBranch(c, h.DB, "reminder").Table("reminder").
		Where("id = ?", id).Limit(1).Scan(&item).Error; err != nil {
		response.NotFound(c)
		return
	}
	helpers.ParseJSONFieldForTable(item, "reminder")
	h.attachCustomerToReminder(c, item)
	response.Data(c, item)
}

// attachCustomerToReminder load nested customer cho 1 item (Get / sau Update).
// List dùng join, không gọi helper này.
func (h *Handler) attachCustomerToReminder(c *gin.Context, item map[string]interface{}) {
	if item == nil {
		return
	}
	custID := helpers.ToInt64(item["customer_id"])
	if custID == 0 {
		return
	}
	var cust map[string]interface{}
	tx := tenant.ScopedDBNullBranch(c, h.DB, "customer").Table("customer").
		Select("id, code, fullname, nickname, phone, gender, avatar, stage_id").
		Where("id = ?", custID).Scan(&cust)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	if cust != nil {
		storage.ResolveFields(cust, "avatar")
		item["customer"] = cust
	}
}
