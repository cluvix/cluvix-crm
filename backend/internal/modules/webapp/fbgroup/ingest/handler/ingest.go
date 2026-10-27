package handler

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"

	ingestDao "erpCluvix/internal/modules/webapp/fbgroup/ingest/dao"
	"erpCluvix/internal/modules/webapp/fbgroup/service"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ingestPayload — đúng hình NextCore gửi (tài liệu "Đẩy dữ liệu GROUP CRAWL", phienBan 1).
type ingestPayload struct {
	Nguon    string        `json:"nguon"`
	PhienBan int           `json:"phienBan"`
	Kiem     bool          `json:"kiem"` // true = bấm "Kiểm kết nối": lô rỗng, trả 2xx là đạt, không ghi gì
	Groups   []ingestGroup `json:"groups"`
}

type ingestGroup struct {
	ID               string   `json:"id"`
	Ten              string   `json:"ten"`
	URL              string   `json:"url"`
	RutGon           *string  `json:"rutGon"`
	ThanhVien        *int64   `json:"thanhVien"`
	ThanhVienLamTron bool     `json:"thanhVienLamTron"`
	BaiNgay          *float64 `json:"baiNgay"`
	CheDo            *string  `json:"cheDo"` // PUBLIC | PRIVATE
	KhuVuc           *string  `json:"khuVuc"`
	DanhMuc          *string  `json:"danhMuc"`
	LanDau           *string  `json:"lanDau"` // ISO 8601 UTC
	LanCuoi          *string  `json:"lanCuoi"`
}

// Ingest POST /api/fbgroup/ingest — NextCore đẩy 1 lô group. Ghi đè theo mã group, phân loại chủ đề
// ngay. Trả 2xx ⇒ NextCore đánh dấu lô đã gửi; 4xx (trừ 429) ⇒ NextCore dừng, không thử lại.
func (h *Handler) Ingest(c *gin.Context) {
	var p ingestPayload
	if err := c.ShouldBindJSON(&p); err != nil {
		response.HandleError(c, err)
		return
	}
	if p.PhienBan != define.FbGroupIngestPayloadVer {
		response.Validate(c, i18n.TF(c, "fbgroup_ingest_version_unsupported", p.PhienBan))
		return
	}
	if p.Kiem {
		response.Data(c, gin.H{"ok": true})
		return
	}
	if len(p.Groups) > define.FbGroupIngestMaxBatch {
		response.Validate(c, i18n.TF(c, "fbgroup_ingest_batch_too_large", define.FbGroupIngestMaxBatch))
		return
	}
	inputs := make([]service.GroupInput, 0, len(p.Groups))
	for _, g := range p.Groups {
		if in, ok := fromPayload(g); ok {
			inputs = append(inputs, in)
		}
	}
	res, err := service.UpsertGroups(h.DB, inputs, false)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	if keyID, ok := c.Get(ctxIngestKeyID); ok {
		now := time.Now().Unix()
		// Số liệu sử dụng khoá — lỗi ở đây không làm hỏng lô đã ghi, nhưng vẫn phải báo.
		if err := h.DB.Model(&ingestDao.IngestKey{}).Where("id = ?", keyID).Updates(map[string]any{
			"last_used_at":     now,
			"last_batch_count": res.Received,
			"total_received":   gorm.Expr("total_received + ?", res.Received),
			"updated_at":       now,
		}).Error; err != nil {
			response.ServerError(c, err)
			return
		}
	}
	res.Skipped = len(p.Groups) - res.Received
	response.Data(c, res)
}

// fromPayload kiểm + quy 1 group của payload về GroupInput. Thiếu mã/tên/link, hoặc link không phải
// facebook.com ⇒ bỏ dòng (đếm vào skipped), không làm hỏng cả lô.
func fromPayload(g ingestGroup) (service.GroupInput, bool) {
	id := strings.TrimSpace(g.ID)
	name := strings.TrimSpace(g.Ten)
	url := strings.TrimSpace(g.URL)
	if id == "" || len(id) > 64 || name == "" || !isFacebookURL(url) || len(url) > 512 {
		return service.GroupInput{}, false
	}
	in := service.GroupInput{
		FacebookID:    id,
		Name:          truncRunes(name, 255),
		URL:           url,
		Slug:          trimPtr(g.RutGon, 128),
		MemberCount:   g.ThanhVien,
		MemberRounded: g.ThanhVienLamTron,
		NcbRegion:     trimPtr(g.KhuVuc, 128),
		NcbCategory:   trimPtr(g.DanhMuc, 160),
		FirstSeenAt:   parseISO(g.LanDau),
		LastSeenAt:    parseISO(g.LanCuoi),
	}
	if g.BaiNgay != nil && *g.BaiNgay >= 0 {
		v := int(math.Round(*g.BaiNgay))
		in.PostsPerDay = &v
	}
	if g.CheDo != nil {
		in.Privacy = privacyFromText(*g.CheDo)
	}
	return in, true
}

func isFacebookURL(u string) bool {
	return strings.HasPrefix(u, "https://www.facebook.com/") || strings.HasPrefix(u, "https://facebook.com/") ||
		strings.HasPrefix(u, "https://m.facebook.com/")
}

// privacyFromText — payload API gửi PUBLIC/PRIVATE, tệp CSV ghi "Công khai"/"Riêng tư".
func privacyFromText(s string) *int {
	var v int
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "public", "công khai":
		v = define.FbGroupPrivacyPublic
	case "private", "riêng tư":
		v = define.FbGroupPrivacyPrivate
	default:
		return nil
	}
	return &v
}

func parseISO(s *string) *int64 {
	if s == nil || *s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, *s)
	if err != nil {
		return nil
	}
	u := t.Unix()
	return &u
}

func trimPtr(s *string, max int) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	v = truncRunes(v, max)
	return &v
}

// truncRunes cắt theo ký tự (không cắt đôi ký tự UTF-8) cho vừa cột varchar(max).
func truncRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}
