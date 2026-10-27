// Package handler — thống kê group Facebook cho màn "Thống kê".
//
// "Group đúng chủ đề" = group có ≥1 dòng gán chủ đề đang hiệu lực (source tự động/gán tay) với chủ đề
// đang dùng. Mọi SUM/AVG được CAST về SIGNED để scan thẳng vào int64 (MySQL trả DECIMAL cho SUM/AVG).
package handler

import (
	"strconv"
	"strings"
	"time"

	"erpCluvix/pkg/define"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Deps struct {
	DB *gorm.DB
}

type Handler struct {
	DB *gorm.DB
}

func New(d Deps) *Handler { return &Handler{DB: d.DB} }

var activeSources = []int{define.FbGroupTopicSourceAuto, define.FbGroupTopicSourceManualAdd}

// onTopic trả điều kiện "group thuộc chủ đề đang dùng" (thuộc đúng topicID nếu > 0) cho bảng alias g.
func onTopic(topicID uint64) (string, []any) {
	sql := "EXISTS (SELECT 1 FROM fb_group_topic_map m JOIN fb_group_topic t ON t.id = m.topic_id AND t.status = 1 " +
		"WHERE m.group_id = g.id AND m.source IN ?"
	args := []any{activeSources}
	if topicID > 0 {
		sql += " AND m.topic_id = ?"
		args = append(args, topicID)
	}
	return sql + ")", args
}

// qualified — g đạt ngưỡng mục tiêu của ≥1 chủ đề nó thuộc (hoặc của đúng topicID).
func qualified(topicID uint64) (string, []any) {
	sql := "EXISTS (SELECT 1 FROM fb_group_topic_map qm JOIN fb_group_topic qt ON qt.id = qm.topic_id AND qt.status = 1 " +
		"WHERE qm.group_id = g.id AND qm.source IN ? AND g.member_count >= qt.min_members " +
		"AND g.posts_per_day >= qt.min_posts_per_day AND (qt.public_only = 0 OR g.privacy = ?)"
	args := []any{activeSources, define.FbGroupPrivacyPublic}
	if topicID > 0 {
		sql += " AND qm.topic_id = ?"
		args = append(args, topicID)
	}
	return sql + ")", args
}

// topicParam — ?topicId= tuỳ chọn; có mà sai ⇒ 422.
func topicParam(c *gin.Context) (uint64, bool) {
	s := strings.TrimSpace(c.Query("topicId"))
	if s == "" {
		return 0, true
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || v == 0 {
		response.Validate(c, i18n.T(c, "invalid_data"))
		return 0, false
	}
	return v, true
}

type overview struct {
	TotalGroups   int64 `gorm:"column:total_groups" json:"total_groups"`
	OnTopicGroups int64 `gorm:"column:on_topic_groups" json:"on_topic_groups"`
	Qualified     int64 `gorm:"column:qualified" json:"qualified"`
	Members       int64 `gorm:"column:members" json:"members"`
	BigGroups     int64 `gorm:"column:big_groups" json:"big_groups"`
	New7d         int64 `gorm:"column:new_7d" json:"new_7d"`
	Excluded      int64 `gorm:"column:excluded" json:"excluded"`
	LastSyncedAt  int64 `gorm:"column:last_synced_at" json:"last_synced_at"`
}

type byTopic struct {
	TopicID   uint   `gorm:"column:topic_id" json:"topic_id"`
	Name      string `gorm:"column:name" json:"name"`
	Color     string `gorm:"column:color" json:"color"`
	Groups    int64  `gorm:"column:groups" json:"groups"`
	Members   int64  `gorm:"column:members" json:"members"`
	Qualified int64  `gorm:"column:qualified" json:"qualified"`
	BigGroups int64  `gorm:"column:big_groups" json:"big_groups"`
}

type byWorkStatus struct {
	WorkStatus int   `gorm:"column:work_status" json:"work_status"`
	Groups     int64 `gorm:"column:groups" json:"groups"`
}

// Overview GET /fbgroup/stats/overview?topicId= — thẻ số liệu + theo chủ đề + theo trạng thái.
func (h *Handler) Overview(c *gin.Context) {
	topicID, ok := topicParam(c)
	if !ok {
		return
	}
	onSQL, onArgs := onTopic(topicID)
	qSQL, qArgs := qualified(topicID)
	weekAgo := time.Now().Add(-7 * 24 * time.Hour).Unix()

	var o overview
	sel := "COUNT(*) AS total_groups, " +
		"CAST(COALESCE(SUM(CASE WHEN " + onSQL + " AND g.work_status <> ? THEN 1 ELSE 0 END), 0) AS SIGNED) AS on_topic_groups, " +
		"CAST(COALESCE(SUM(CASE WHEN " + qSQL + " AND g.work_status <> ? THEN 1 ELSE 0 END), 0) AS SIGNED) AS qualified, " +
		"CAST(COALESCE(SUM(CASE WHEN " + onSQL + " AND g.work_status <> ? THEN g.member_count ELSE 0 END), 0) AS SIGNED) AS members, " +
		"CAST(COALESCE(SUM(CASE WHEN " + onSQL + " AND g.work_status <> ? AND g.member_count >= ? THEN 1 ELSE 0 END), 0) AS SIGNED) AS big_groups, " +
		"CAST(COALESCE(SUM(CASE WHEN " + onSQL + " AND g.created_at >= ? THEN 1 ELSE 0 END), 0) AS SIGNED) AS new_7d, " +
		"CAST(COALESCE(SUM(CASE WHEN g.work_status = ? THEN 1 ELSE 0 END), 0) AS SIGNED) AS excluded, " +
		"COALESCE(MAX(g.synced_at), 0) AS last_synced_at"
	ex := define.FbGroupWorkExcluded
	args := make([]any, 0, 16)
	args = append(append(args, onArgs...), ex)
	args = append(append(args, qArgs...), ex)
	args = append(append(args, onArgs...), ex)
	args = append(append(args, onArgs...), ex, define.FbGroupBigMembers)
	args = append(append(args, onArgs...), weekAgo)
	args = append(args, ex)
	if tx := h.DB.Table("fb_group g").Select(sel, args...).Scan(&o); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	topics := make([]byTopic, 0)
	if tx := h.DB.Table("fb_group_topic t").
		Select(`t.id AS topic_id, t.name, t.color,
			COUNT(g.id) AS `+"`groups`"+`,
			CAST(COALESCE(SUM(g.member_count), 0) AS SIGNED) AS members,
			CAST(COALESCE(SUM(CASE WHEN g.member_count >= t.min_members AND g.posts_per_day >= t.min_posts_per_day
				AND (t.public_only = 0 OR g.privacy = ?) THEN 1 ELSE 0 END), 0) AS SIGNED) AS qualified,
			CAST(COALESCE(SUM(CASE WHEN g.member_count >= ? THEN 1 ELSE 0 END), 0) AS SIGNED) AS big_groups`,
			define.FbGroupPrivacyPublic, define.FbGroupBigMembers).
		Joins("LEFT JOIN fb_group_topic_map m ON m.topic_id = t.id AND m.source IN ?", activeSources).
		Joins("LEFT JOIN fb_group g ON g.id = m.group_id AND g.work_status <> ?", define.FbGroupWorkExcluded).
		Where("t.status = 1").
		Group("t.id, t.name, t.color, t.sort_order").Order("t.sort_order ASC, t.id ASC").
		Scan(&topics); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}

	statuses := make([]byWorkStatus, 0)
	q := h.DB.Table("fb_group g").Select("g.work_status, COUNT(*) AS `groups`")
	if topicID > 0 {
		q = q.Where(onSQL, onArgs...)
	}
	if tx := q.Group("g.work_status").Order("g.work_status").Scan(&statuses); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	response.Data(c, gin.H{"overview": o, "by_topic": topics, "by_work_status": statuses})
}

type byBranch struct {
	TopicID     uint    `gorm:"column:topic_id" json:"topic_id"`
	TopicName   string  `gorm:"column:topic_name" json:"topic_name"`
	TopicColor  string  `gorm:"column:topic_color" json:"topic_color"`
	BranchID    *uint   `gorm:"column:branch_id" json:"branch_id"`
	BranchName  *string `gorm:"column:branch_name" json:"branch_name"` // null = chưa thuộc nhánh nào
	Groups      int64   `gorm:"column:groups" json:"groups"`
	Members     int64   `gorm:"column:members" json:"members"`
	Over10k     int64   `gorm:"column:over10k" json:"over_10k"`
	Active10Day int64   `gorm:"column:active10_day" json:"active_10_per_day"`
}

// Branches GET /fbgroup/stats/branches?topicId= — theo nhánh trong chủ đề (bỏ group "Loại").
func (h *Handler) Branches(c *gin.Context) {
	topicID, ok := topicParam(c)
	if !ok {
		return
	}
	rows := make([]byBranch, 0)
	q := h.DB.Table("fb_group_topic_map m").
		Select(`t.id AS topic_id, t.name AS topic_name, t.color AS topic_color, b.id AS branch_id, b.name AS branch_name,
			COUNT(*) AS `+"`groups`"+`,
			CAST(COALESCE(SUM(g.member_count), 0) AS SIGNED) AS members,
			CAST(COALESCE(SUM(CASE WHEN g.member_count >= 10000 THEN 1 ELSE 0 END), 0) AS SIGNED) AS over10k,
			CAST(COALESCE(SUM(CASE WHEN g.posts_per_day >= 10 THEN 1 ELSE 0 END), 0) AS SIGNED) AS active10_day`).
		Joins("JOIN fb_group_topic t ON t.id = m.topic_id AND t.status = 1").
		Joins("JOIN fb_group g ON g.id = m.group_id AND g.work_status <> ?", define.FbGroupWorkExcluded).
		Joins("LEFT JOIN fb_group_topic_branch b ON b.id = m.branch_id").
		Where("m.source IN ?", activeSources)
	if topicID > 0 {
		q = q.Where("m.topic_id = ?", topicID)
	}
	if tx := q.Group("t.id, t.name, t.color, t.sort_order, b.id, b.name, b.sort_order").
		Order("t.sort_order ASC, t.id ASC, members DESC").Scan(&rows); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	response.Data(c, rows)
}

type byProvince struct {
	Province string `gorm:"column:province" json:"province"`
	Groups   int64  `gorm:"column:groups" json:"groups"`
	Members  int64  `gorm:"column:members" json:"members"`
}

// Provinces GET /fbgroup/stats/provinces?topicId= — tỉnh có nhiều thành viên nhất (group đúng chủ đề).
func (h *Handler) Provinces(c *gin.Context) {
	topicID, ok := topicParam(c)
	if !ok {
		return
	}
	onSQL, onArgs := onTopic(topicID)
	rows := make([]byProvince, 0)
	if tx := h.DB.Table("fb_group g").
		Select("g.province, COUNT(*) AS `groups`, CAST(COALESCE(SUM(g.member_count), 0) AS SIGNED) AS members").
		Where("g.province IS NOT NULL AND g.work_status <> ?", define.FbGroupWorkExcluded).
		Where(onSQL, onArgs...).
		Group("g.province").Order("members DESC").Limit(define.FbGroupStatsTopLimit).
		Scan(&rows); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	response.Data(c, rows)
}

type byKeyword struct {
	Keyword    string `gorm:"column:keyword" json:"keyword"`
	OnTopic    int64  `gorm:"column:on_topic" json:"on_topic"`
	OffTopic   int64  `gorm:"column:off_topic" json:"off_topic"`
	AvgMembers int64  `gorm:"column:avg_members" json:"avg_members"`
	Over10k    int64  `gorm:"column:over10k" json:"over_10k"`
}

// Keywords GET /fbgroup/stats/keywords — hiệu quả từ khoá GROUPCRAWL (cột found_keyword từ tệp CSV).
func (h *Handler) Keywords(c *gin.Context) {
	onSQL, onArgs := onTopic(0)
	rows := make([]byKeyword, 0)
	sel := "g.found_keyword AS keyword, " +
		"CAST(SUM(CASE WHEN " + onSQL + " THEN 1 ELSE 0 END) AS SIGNED) AS on_topic, " +
		"CAST(SUM(CASE WHEN " + onSQL + " THEN 0 ELSE 1 END) AS SIGNED) AS off_topic, " +
		"CAST(COALESCE(ROUND(AVG(CASE WHEN " + onSQL + " THEN g.member_count END)), 0) AS SIGNED) AS avg_members, " +
		"CAST(SUM(CASE WHEN " + onSQL + " AND g.member_count >= 10000 THEN 1 ELSE 0 END) AS SIGNED) AS over10k"
	args := make([]any, 0, 8)
	for i := 0; i < 4; i++ {
		args = append(args, onArgs...)
	}
	if tx := h.DB.Table("fb_group g").Select(sel, args...).
		Where("g.found_keyword IS NOT NULL").
		Group("g.found_keyword").Order("avg_members DESC").Scan(&rows); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	response.Data(c, rows)
}
