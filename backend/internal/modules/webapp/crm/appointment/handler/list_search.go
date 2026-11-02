package handler

import (
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"erpCluvix/pkg/query"
)

// ───────────────────────────────────────────────────────────────────────────────
// Tối ưu màn danh sách lịch hẹn. Số đo trên DB clone schema y hệt, 524.288 lịch hẹn /
// 100.000 khách (2026-09-06); ~40ms trong mỗi số là overhead kết nối mysql client.
//
//  1. LEFT JOIN customer chỉ gắn KHI CẦN.
//     `customer.id` là khoá chính nên join KHÔNG THỂ làm đổi số dòng — về mặt logic bỏ
//     được khi không ai đọc cột customer. Nhưng MySQL KHÔNG làm tối ưu loại-bỏ-left-join
//     (khác SQL Server/Oracle) nên nó vẫn join đủ mọi dòng khi đếm:
//     COUNT có join = 1091–1272ms  ·  COUNT bỏ join = 641–773ms
//     `pagination.Paginate` chạy COUNT MỌI lần load danh sách ⇒ chi phí trả đều.
//
//  2. Tìm kiếm từ khoá đi qua FULLTEXT ngram (cho `customer`) thay vì LIKE '%kw%' trên join.
//     ⚠ `appointment.code` thì KHÔNG — đo trên dữ liệu thật cho thấy ngram thua LIKE ở mọi ca
//     có khớp; chỉ mục đó đã bị bỏ ở migration 20260906191504. Xem khối apptIDs bên dưới.
//     LIKE có % đứng đầu ⇒ không index nào dùng được ⇒ MySQL quét toàn bộ 524k lịch hẹn,
//     join customer từng dòng, lọc LIKE tới khi đủ 15 kết quả:
//     có khớp = 1351ms  ·  hiếm/không khớp = 6814ms
//     COUNT của pagination LUÔN rơi vào ca xấu vì không bao giờ thoát sớm được.
//
//     Cách làm: tra id trước bằng ngram (2 truy vấn nhỏ), rồi lọc bằng
//     `customer_id IN (...) OR id IN (...)` — MySQL dùng index_merge. Tổng ~170ms.
//
//     ⚠ ĐÃ THỬ VÀ LOẠI, đừng làm lại:
//     · `MATCH(appointment.code) OR customer_id IN (SELECT … MATCH …)` = 1811–2968ms
//     (MySQL không kết hợp được FULLTEXT trong OR, tụt về quét toàn bảng)
//     · `id IN ((MATCH …) UNION (SELECT …))`                          = 3601ms
//     · Viết lại join thành IN(subquery) mà vẫn dùng LIKE             = 1520ms
//     · `customer_id IN (SELECT … MATCH …) OR code LIKE 'kw%'`        = 1108ms
//     CHỈ dạng "tra id ra trước ở Go rồi truyền danh sách literal" mới nhanh (36ms).
//
//     ⚠ Đường này CHỈ đúng nhờ 2 chỉ mục ngram dựng với stopword TẮT trong migration
//     `20260906122048_rebuild_ngram_omi_and_customer.up.sql`. Đọc phần đầu file đó trước
//     khi đụng vào: parser ngram loại token CHỨA stopword, mà danh sách mặc định có `a`
//     và `i` một ký tự ⇒ chỉ mục dựng sai sẽ khiến tìm "Văn", "Mai", "Nam" trả RỖNG mà
//     không có lỗi nào. Và cấu hình stopword gắn theo BẢNG, cố định bởi chỉ mục FULLTEXT
//     đầu tiên — thêm một FULLTEXT khác lên `customer`/`appointment` sau này mà không bỏ
//     hết chỉ mục cũ trước là làm hỏng lại toàn bộ tìm kiếm.
// ───────────────────────────────────────────────────────────────────────────────

// keywordIDCap — trần số id nạp về cho đường nhanh. Vượt trần ⇒ rơi về đường LIKE cũ
// (chậm nhưng ĐÚNG) thay vì cắt bớt danh sách. Cắt bớt = trả thiếu kết quả im lặng,
// đúng loại bug tệ nhất: không lỗi, không cảnh báo, người dùng tưởng khách không tồn tại.
const keywordIDCap = 10000

// customerFilterColumns — các filter đọc cột của bảng `customer`. Có bất kỳ cái nào
// ⇒ bắt buộc phải LEFT JOIN customer.
//
// ⚠ Thêm filter mới đụng `customer.*` trong BuildAppointmentQuery thì PHẢI thêm khoá
// vào đây, nếu không MySQL báo "Unknown column 'customer.x'".
var customerFilterColumns = []string{
	"_stage_id", "_gender", "customerType", "category",
	"_groupLocal", "_groupCountry",
}

// needsCustomerJoin — có filter nào (ngoài từ khoá) đọc cột customer không.
//
// `_export=1` LUÔN cần join: `export.ExportAppointment` nhận chính query này rồi select
// `customer.code/fullname/phone` và join `location_province ON prov.id = customer.province_id`.
// Bỏ join ⇒ export chết ngay với "Unknown column 'customer.code'". Ép join ở ĐÂY (một điểm
// quyết định duy nhất) thay vì để ExportAppointment tự join — nó tự join thì khi filter
// customer cũng có mặt sẽ thành join trùng: "Not unique table/alias: 'customer'".
func needsCustomerJoin(c query.Params) bool {
	if c.Query("_export") == "1" {
		return true
	}
	for _, k := range customerFilterColumns {
		if len(query.GetFilterValues(c, k)) > 0 {
			return true
		}
	}
	return false
}

// applyKeywordFast thử áp bộ lọc từ khoá bằng đường ngram.
//
// Trả về (query đã gắn điều kiện, true) khi đi được đường nhanh — gọi bên ngoài KHÔNG cần join.
// Trả về (query nguyên trạng, false) khi phải rơi về đường LIKE cũ — gọi bên ngoài PHẢI tự gắn
// LEFT JOIN + ApplyKeywordFilter.
func (h *Handler) applyKeywordFast(c query.Params, q *gorm.DB) (*gorm.DB, bool) {
	kw := strings.TrimSpace(c.Query("_keyword"))
	if kw == "" {
		return q, true // không có từ khoá ⇒ không cần join, coi như xong
	}
	if utf8.RuneCountInString(kw) < query.NgramMinLen {
		return q, false
	}
	// ⚠ Cụm chứa "từ" 1 ký tự ⇒ ngram không sinh được bigram cho từ đó nên CẢ CỤM trượt:
	// đo trên 100.000 khách (2026-09-06) "Văn A" cho customer LIKE 635 · MATCH 0 — tìm ra
	// RỖNG mà không lỗi, không cảnh báo. Ca này BẮT BUỘC đi đường LIKE cũ.
	if !query.NgramPhraseSafe(kw) {
		return q, false
	}

	// Nháy kép BẮT BUỘC: AGAINST('"0912"' IN BOOLEAN MODE) = tìm đúng chuỗi con đó, khớp
	// bằng LIKE '%0912%'. Thiếu nháy kép ⇒ ngram tách thành các bigram rời rồi OR ⇒ THỪA
	// kết quả. Nháy kép có sẵn trong từ khoá phải bỏ, không thì cú pháp BOOLEAN MODE vỡ.
	phrase := query.NgramPhrase(kw)
	like := "%" + kw + "%"

	// KHÔNG scope tenant ở đây — CỐ Ý. Bản cũ join `customer` trần, mọi lọc tenant nằm ở phía
	// `appointment`. Giữ nguyên để tập kết quả không đổi; lịch hẹn vẫn bị scope đầy đủ.
	//
	// ⚠ AND LIKE là BẮT BUỘC, không phải cho chắc. Parser ngram BỎ mọi ký tự phân cách, nên
	// dấu chấm/gạch/khoảng trắng trong từ khoá bị nuốt và MATCH khớp theo chuỗi TOKEN kề nhau
	// bất kể phân cách gì. Mã Cluvix đầy dấu chấm (`ID.Q1.KH.0101.0000123`) nên gõ sai phân
	// cách là trả về hàng chục nghìn dòng lẽ ra phải rỗng — đo trên 100.000 khách / 524.288
	// lịch hẹn (2026-09-06):
	//     "ID Q1 KH"  → customer LIKE 0 · MATCH 16.666
	//     "ID-Q1-KH"  → customer LIKE 0 · MATCH 16.666
	//     "IB Q1 LH"  → appointment.code LIKE 0 · MATCH 87.382
	// Cùng một câu tìm kiếm cho 16.666 dòng qua đường nhanh và 0 dòng qua đường lui — sai
	// theo nghĩa KHÔNG NHẤT QUÁN, và không có lỗi nào báo ra.
	//
	// AND LIKE kéo cả 8 ca đo về đúng bằng LIKE. Chi phí gần như bằng 0 vì MATCH đã thu tập
	// nhỏ trước: 0,039→0,037s · 0,043→0,041s · 0,014→0,012s (ca xấu nhất 0,468→0,615s, là ca
	// vốn đã vượt keywordIDCap nên sẽ rơi về đường lui).
	//
	// Danh sách cột PHẢI khớp phần `customer.*` của `appointmentKeySearch`; `customer.id` xử
	// lý riêng ở khối isAllDigits bên dưới.
	// ⚠ MATCH đi trên CỘT SINH `customer.search_ngram` (gộp 6 cột, migration 20260906173738),
	// KHÔNG phải chỉ mục 6 cột như bản đầu. Lý do đo được trên 76.999 khách thật: FULLTEXT
	// NHIỀU CỘT bỏ SÓT dòng khi một cột đứng trước là chuỗi RỖNG — "Nguyễn" LIKE 7.419 nhưng
	// MATCH chỉ 6.509; riêng nhóm `fullname = ''` mà tên nằm ở `nickname` chỉ ra 68/977.
	// 6% khách của DB đó có `fullname = ''` ⇒ vô hình với tìm kiếm. `NULL` thì không sao, chỉ
	// chuỗi rỗng mới hỏng. Gộp về một cột (CONCAT_WS + NULLIF) đưa về 13/13 khớp LIKE.
	var custIDs []int64
	if err := h.DB.Table("customer").
		Where("MATCH(search_ngram) AGAINST (? IN BOOLEAN MODE)", phrase).
		Where("fullname LIKE ? OR code LIKE ? OR phone LIKE ? OR other_phone LIKE ? OR old_code LIKE ? OR nickname LIKE ?",
			like, like, like, like, like, like).
		Limit(keywordIDCap+1).
		Pluck("id", &custIDs).Error; err != nil {
		return q, false
	}
	if len(custIDs) > keywordIDCap {
		return q, false
	}

	// `customer.id` nằm trong appointmentKeySearch (tìm theo id nội bộ, dạng chuỗi con).
	// ngram không index được cột INT nên giữ LIKE — chỉ chạy khi từ khoá toàn số, và chỉ quét
	// index PRIMARY (covering) nên rẻ.
	if isAllDigits(kw) {
		var byID []int64
		if err := h.DB.Table("customer").
			Where("CAST(id AS CHAR) LIKE ?", "%"+kw+"%").
			Limit(keywordIDCap+1).
			Pluck("id", &byID).Error; err != nil {
			return q, false
		}
		if len(custIDs)+len(byID) > keywordIDCap {
			return q, false
		}
		custIDs = append(custIDs, byID...)
	}

	// `appointment.code` đi LIKE chuỗi con, KHÔNG đi ngram — CỐ Ý, đã đo và bỏ chỉ mục
	// (migration 20260906191504). ngram thua ở MỌI ca có khớp trên 218.071 lịch hẹn thật:
	//     dán trọn mã  ngram 0,176s · LIKE 0,045s   |   "ABC.LD.LH"  ngram 0,222s · LIKE 0,016s
	//     gõ đuôi mã   ngram 0,072s · LIKE 0,043s   |   "ABC.CG"     ngram 0,200s · LIKE 0,007s
	// Chỉ thắng ca không khớp (0,000s so với 0,040s). Lý do: mọi mã dùng chung bigram
	// (AB·BC·LH·26·09…) nên phrase search phải giao posting list gần-như-toàn-bảng, đắt hơn
	// quét covering index `code` mà LIKE đang làm. ngram hợp văn bản tự do, không hợp mã.
	//
	// Không scope tenant ở đây — truy vấn ngoài đã scope đủ; thêm điều kiện company/branch
	// vào đây chỉ làm hỏng covering scan trên UNIQUE KEY `code`.
	var apptIDs []int64
	if err := h.DB.Table("appointment").
		Where("code LIKE ?", like).
		Limit(keywordIDCap+1).
		Pluck("id", &apptIDs).Error; err != nil {
		return q, false
	}
	if len(apptIDs) > keywordIDCap {
		return q, false
	}

	// Cả hai rỗng ⇒ không có gì khớp. Phải trả 0 dòng, KHÔNG được bỏ qua điều kiện
	// (bỏ qua = trả về toàn bộ lịch hẹn như thể không tìm gì).
	if len(custIDs) == 0 && len(apptIDs) == 0 {
		return q.Where("1 = 0"), true
	}
	if len(custIDs) == 0 {
		return q.Where("appointment.id IN ?", apptIDs), true
	}
	if len(apptIDs) == 0 {
		return q.Where("appointment.customer_id IN ?", custIDs), true
	}
	return q.Where("appointment.customer_id IN ? OR appointment.id IN ?", custIDs, apptIDs), true
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
