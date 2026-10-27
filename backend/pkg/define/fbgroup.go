package define

// Module Group Facebook (bmad/fb-group/thiet-ke.md): group do NextCore GROUPCRAWL crawl về, phân
// loại theo chủ đề và quản lý trạng thái làm việc. Giá trị số lưu thẳng xuống DB — KHÔNG đổi.

// Trạng thái làm việc của 1 group (cột fb_group.work_status) — người dùng đặt.
const (
	FbGroupWorkNew      = 0 // Mới — chưa xử lý
	FbGroupWorkTarget   = 1 // Mục tiêu
	FbGroupWorkWatch    = 2 // Theo dõi
	FbGroupWorkExcluded = 3 // Loại (lạc đề, chất lượng kém…)
)

// FbGroupWorkStatusLabels — nhãn trạng thái ghi vào file Excel xuất (FE hiển thị qua i18n riêng).
var FbGroupWorkStatusLabels = map[int]string{
	FbGroupWorkNew: "Mới", FbGroupWorkTarget: "Mục tiêu", FbGroupWorkWatch: "Theo dõi", FbGroupWorkExcluded: "Loại",
}

// FbGroupWorkStatuses — tập giá trị hợp lệ của work_status (validate payload).
var FbGroupWorkStatuses = map[int]bool{
	FbGroupWorkNew: true, FbGroupWorkTarget: true, FbGroupWorkWatch: true, FbGroupWorkExcluded: true,
}

// Chế độ group (cột fb_group.privacy).
const (
	FbGroupPrivacyPublic  = 1
	FbGroupPrivacyPrivate = 2
)

// Chất lượng do NextCore "Phân tích lại" chấm (cột fb_group.ncb_quality) — chỉ biết qua tệp CSV
// xuất từ GROUPCRAWL (tệp chỉ gồm group được GIỮ); luồng đẩy API không gửi trường này.
const (
	FbGroupNcbQualityKept = 1
)

// Nguồn của 1 dòng gán chủ đề (cột fb_group_topic_map.source).
const (
	FbGroupTopicSourceAuto          = 1 // bộ phân loại theo từ khoá tự gán — bị ghi lại mỗi lần phân loại
	FbGroupTopicSourceManualAdd     = 2 // người dùng gán tay — phân loại tự động KHÔNG ghi đè
	FbGroupTopicSourceManualRemoved = 3 // người dùng bỏ tay — phân loại tự động KHÔNG gán lại
)

// Giới hạn đầu vào.
const (
	FbGroupIngestMaxBatch   = 200   // NextCore gửi ≤100 group/lô; nới gấp đôi để không vỡ khi họ đổi
	FbGroupImportMaxRows    = 50000 // số dòng tối đa của 1 tệp CSV GROUPCRAWL
	FbGroupClassifyBatch    = 1000  // số group mỗi lô khi phân loại lại toàn kho
	FbGroupBulkMaxIDs       = 1000  // số group tối đa mỗi thao tác hàng loạt
	FbGroupPreviewSampleMax = 20    // số tên group mẫu trả về khi xem trước chủ đề
	FbGroupStatsTopLimit    = 15    // số dòng tối đa của bảng thống kê theo tỉnh
	FbGroupBigMembers       = 100000
	FbGroupIngestPayloadVer = 1 // phienBan payload NextCore hỗ trợ
)

// FbGroupIngestKeyPrefix — tiền tố khoá API cấp cho NextCore (nhận dạng khi lộ trong log).
const FbGroupIngestKeyPrefix = "ncb_"
