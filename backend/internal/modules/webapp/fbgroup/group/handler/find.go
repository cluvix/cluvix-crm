package handler

import (
	"net/http"
	"strconv"
	"strings"

	"erpCluvix/internal/modules/webapp/fbgroup/classify"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// topicRef — chủ đề của 1 group, trả lồng trong group (Hard Rule: nested, không flatten).
type topicRef struct {
	GroupID    uint    `json:"-"`
	TopicID    uint    `json:"topic_id"`
	Name       string  `json:"name"`
	Color      string  `json:"color"`
	BranchID   *uint   `json:"branch_id"`
	BranchName *string `json:"branch_name"`
	Source     int     `json:"source"`
}

type GroupRow struct {
	ID            uint       `json:"id"`
	FacebookID    string     `json:"facebook_id"`
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	MemberCount   *int64     `json:"member_count"`
	MemberRounded bool       `json:"member_rounded"`
	PostsPerDay   *int       `json:"posts_per_day"`
	Privacy       *int       `json:"privacy"`
	Province      *string    `json:"province"`
	FoundKeyword  *string    `json:"found_keyword"`
	NcbQuality    *int       `json:"ncb_quality"`
	WorkStatus    int        `json:"work_status"`
	LastSeenAt    *int64     `json:"last_seen_at"`
	SyncedAt      *int64     `json:"synced_at"`
	Topics        []topicRef `gorm:"-" json:"topics"`
}

type groupDetail struct {
	GroupRow
	Slug           *string `json:"slug"`
	NcbRegion      *string `json:"ncb_region"`
	NcbCategory    *string `json:"ncb_category"`
	FoundByProfile *string `json:"found_by_profile"`
	Note           *string `json:"note"`
	FirstSeenAt    *int64  `json:"first_seen_at"`
	CreatedAt      int64   `json:"created_at"`
	UpdatedAt      int64   `json:"updated_at"`
}

var listColumns = []string{
	"fb_group.id", "fb_group.facebook_id", "fb_group.name", "fb_group.url", "fb_group.member_count",
	"fb_group.member_rounded", "fb_group.posts_per_day", "fb_group.privacy", "fb_group.province",
	"fb_group.found_keyword", "fb_group.ncb_quality", "fb_group.work_status", "fb_group.last_seen_at",
	"fb_group.synced_at",
}

// Cột được phép sắp xếp (tham số sort=member_count | -member_count …).
var sortColumns = map[string]string{
	"member_count":  "fb_group.member_count",
	"posts_per_day": "fb_group.posts_per_day",
	"last_seen_at":  "fb_group.last_seen_at",
	"name":          "fb_group.name",
	"created_at":    "fb_group.created_at",
}

// Điều kiện "group được gán chủ đề" — bỏ dòng bỏ-tay (source 3).
var activeSources = []int{define.FbGroupTopicSourceAuto, define.FbGroupTopicSourceManualAdd}

// qualifiedSQL — group đạt ngưỡng mục tiêu của ÍT NHẤT 1 chủ đề nó thuộc (hoặc của đúng topicId).
const qualifiedSQL = `EXISTS (SELECT 1 FROM fb_group_topic_map qm JOIN fb_group_topic qt ON qt.id = qm.topic_id
	WHERE qm.group_id = fb_group.id AND qm.source IN ? AND qt.status = 1 %s
	AND fb_group.member_count >= qt.min_members AND fb_group.posts_per_day >= qt.min_posts_per_day
	AND (qt.public_only = 0 OR fb_group.privacy = ?))`

// parseUintQuery — query param số dương; vắng ⇒ (0, true); có mà sai ⇒ (0, false).
func parseUintQuery(c *gin.Context, name string) (uint64, bool) {
	s := strings.TrimSpace(c.Query(name))
	if s == "" {
		return 0, true
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || v == 0 {
		return 0, false
	}
	return v, true
}

// applyFilters gắn bộ lọc của màn danh sách. Trả false nếu tham số sai (đã trả 422).
func applyFilters(c *gin.Context, q *gorm.DB) (*gorm.DB, bool) {
	topicID, ok1 := parseUintQuery(c, "topicId")
	branchID, ok2 := parseUintQuery(c, "branchId")
	minMembers, ok3 := parseUintQuery(c, "minMembers")
	minPosts, ok4 := parseUintQuery(c, "minPostsPerDay")
	if !ok1 || !ok2 || !ok3 || !ok4 {
		response.Validate(c, i18n.T(c, "invalid_data"))
		return q, false
	}
	if topicID > 0 {
		sub := "EXISTS (SELECT 1 FROM fb_group_topic_map m WHERE m.group_id = fb_group.id AND m.topic_id = ? AND m.source IN ?"
		args := []any{topicID, activeSources}
		if branchID > 0 {
			sub += " AND m.branch_id = ?"
			args = append(args, branchID)
		}
		q = q.Where(sub+")", args...)
	}
	if s := strings.TrimSpace(c.Query("workStatus")); s != "" {
		vals := make([]int, 0, 4)
		for _, p := range strings.Split(s, ",") {
			v, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || !define.FbGroupWorkStatuses[v] {
				response.Validate(c, i18n.T(c, "invalid_status"))
				return q, false
			}
			vals = append(vals, v)
		}
		q = q.Where("fb_group.work_status IN ?", vals)
	}
	if s := strings.TrimSpace(c.Query("privacy")); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || (v != define.FbGroupPrivacyPublic && v != define.FbGroupPrivacyPrivate) {
			response.Validate(c, i18n.T(c, "invalid_data"))
			return q, false
		}
		q = q.Where("fb_group.privacy = ?", v)
	}
	if minMembers > 0 {
		q = q.Where("fb_group.member_count >= ?", minMembers)
	}
	if minPosts > 0 {
		q = q.Where("fb_group.posts_per_day >= ?", minPosts)
	}
	if p := strings.TrimSpace(c.Query("province")); p != "" {
		q = q.Where("fb_group.province = ?", p)
	}
	if c.Query("qualified") == "1" {
		extra, args := "", []any{activeSources}
		if topicID > 0 {
			extra = "AND qm.topic_id = ?"
			args = append(args, topicID)
		}
		args = append(args, define.FbGroupPrivacyPublic)
		q = q.Where(strings.Replace(qualifiedSQL, "%s", extra, 1), args...)
	}
	if kw := classify.Normalize(c.Query("_keyword")); kw != "" {
		q = q.Where("(fb_group.name_normalized LIKE ? OR fb_group.facebook_id = ?)", "%"+kw+"%", strings.TrimSpace(c.Query("_keyword")))
	}
	return q, true
}

func applySort(c *gin.Context, q *gorm.DB) *gorm.DB {
	s := strings.TrimSpace(c.Query("sort"))
	desc := strings.HasPrefix(s, "-")
	col, ok := sortColumns[strings.TrimPrefix(s, "-")]
	if !ok {
		col, desc = sortColumns["member_count"], true
	}
	dir := " ASC"
	if desc {
		dir = " DESC"
	}
	return q.Order(col + dir).Order("fb_group.id DESC")
}

// attachTopics nạp chủ đề (đang hiệu lực) cho các group của trang và gắn lồng vào từng dòng.
func attachTopics(db *gorm.DB, rows []GroupRow) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uint, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	refs := make([]topicRef, 0)
	if err := db.Table("fb_group_topic_map m").
		Select("m.group_id, m.topic_id, t.name, t.color, m.branch_id, b.name AS branch_name, m.source").
		Joins("JOIN fb_group_topic t ON t.id = m.topic_id").
		Joins("LEFT JOIN fb_group_topic_branch b ON b.id = m.branch_id").
		Where("m.group_id IN ? AND m.source IN ?", ids, activeSources).
		Order("t.sort_order ASC, t.id ASC").Scan(&refs).Error; err != nil {
		return err
	}
	byGroup := map[uint][]topicRef{}
	for _, r := range refs {
		byGroup[r.GroupID] = append(byGroup[r.GroupID], r)
	}
	for i := range rows {
		rows[i].Topics = byGroup[rows[i].ID]
		if rows[i].Topics == nil {
			rows[i].Topics = make([]topicRef, 0)
		}
	}
	return nil
}

// ListGroup GET /fbgroup/group — danh sách có lọc, sắp xếp, phân trang; kèm chủ đề từng group.
func (h *Handler) ListGroup(c *gin.Context) {
	q := tenant.ScopedDBNullBranch(c, h.DB, "fb_group").Table("fb_group").Select(listColumns)
	q, ok := applyFilters(c, q)
	if !ok {
		return
	}
	q = applySort(c, q)
	items := make([]GroupRow, 0)
	result := pagination.PaginateTyped(c, q, &items)
	if c.Writer.Written() {
		return // Paginate đã trả 500 khi lỗi SQL
	}
	if err := attachTopics(h.DB, items); err != nil {
		pagination.FailQuery(c, err)
		return
	}
	result.Items = items
	c.JSON(http.StatusOK, result)
}

// loadDetail đọc 1 group theo id (đã parse) kèm chủ đề. found=false khi không có.
func (h *Handler) loadDetail(c *gin.Context, id uint) (groupDetail, bool, error) {
	var d groupDetail
	cols := append(append([]string{}, listColumns...), "fb_group.slug", "fb_group.ncb_region", "fb_group.ncb_category",
		"fb_group.found_by_profile", "fb_group.note", "fb_group.first_seen_at", "fb_group.created_at", "fb_group.updated_at")
	tx := tenant.ScopedDBNullBranch(c, h.DB, "fb_group").Table("fb_group").Select(cols).
		Where("fb_group.id = ?", id).Limit(1).Scan(&d)
	if pagination.IsQueryError(tx.Error) {
		return d, false, tx.Error
	}
	if d.ID == 0 {
		return d, false, nil
	}
	rows := []GroupRow{d.GroupRow}
	if err := attachTopics(h.DB, rows); err != nil {
		return d, false, err
	}
	d.GroupRow = rows[0]
	return d, true, nil
}

// GetGroup GET /fbgroup/group/:id
func (h *Handler) GetGroup(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_data"))
		return
	}
	d, found, err := h.loadDetail(c, id)
	if err != nil {
		pagination.FailQuery(c, err)
		return
	}
	if !found {
		response.NotFound(c)
		return
	}
	response.Data(c, d)
}
