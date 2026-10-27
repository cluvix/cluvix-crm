package handler

import (
	"net/http"
	"time"

	"erpCluvix/internal/middleware"
	"erpCluvix/internal/models"
	"erpCluvix/internal/modules/webapp/fbgroup/service"
	topicDao "erpCluvix/internal/modules/webapp/fbgroup/topic/dao"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var topicColumns = []string{
	"id", "name", "color", "include_keywords", "exclude_keywords", "min_members", "min_posts_per_day",
	"public_only", "sort_order", "status", "created_at", "updated_at",
}

// topicWithStats — chủ đề + số group đang thuộc + số group đạt ngưỡng mục tiêu.
type topicWithStats struct {
	topicDao.Topic
	GroupCount     int64 `json:"group_count"`
	QualifiedCount int64 `json:"qualified_count"`
}

type topicCount struct {
	TopicID        uint
	GroupCount     int64
	QualifiedCount int64
}

// loadBranches gắn nhánh cho các chủ đề.
func loadBranches(db *gorm.DB, topics []topicDao.Topic) error {
	if len(topics) == 0 {
		return nil
	}
	ids := make([]uint, len(topics))
	for i, t := range topics {
		ids[i] = t.ID
	}
	branches := make([]topicDao.TopicBranch, 0)
	if err := db.Model(&topicDao.TopicBranch{}).
		Select("id", "topic_id", "name", "keywords", "sort_order", "created_at", "updated_at").
		Where("topic_id IN ?", ids).Order("sort_order ASC, id ASC").Find(&branches).Error; err != nil {
		return err
	}
	by := map[uint][]topicDao.TopicBranch{}
	for _, b := range branches {
		by[b.TopicID] = append(by[b.TopicID], b)
	}
	for i := range topics {
		topics[i].Branches = by[topics[i].ID]
		if topics[i].Branches == nil {
			topics[i].Branches = make([]topicDao.TopicBranch, 0)
		}
	}
	return nil
}

// ListTopic GET /fbgroup/topic — mọi chủ đề (ít, không phân trang) kèm nhánh + số group.
func (h *Handler) ListTopic(c *gin.Context) {
	topics := make([]topicDao.Topic, 0)
	if tx := tenant.ScopedDBNullBranch(c, h.DB, "fb_group_topic").Model(&topicDao.Topic{}).
		Select(topicColumns).Order("sort_order ASC, id ASC").Find(&topics); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	if err := loadBranches(h.DB, topics); err != nil {
		pagination.FailQuery(c, err)
		return
	}
	counts := make([]topicCount, 0)
	if tx := h.DB.Table("fb_group_topic_map m").
		Select(`m.topic_id,
			COUNT(*) AS group_count,
			SUM(CASE WHEN g.member_count >= t.min_members AND g.posts_per_day >= t.min_posts_per_day
				AND (t.public_only = 0 OR g.privacy = ?) THEN 1 ELSE 0 END) AS qualified_count`, define.FbGroupPrivacyPublic).
		Joins("JOIN fb_group g ON g.id = m.group_id").
		Joins("JOIN fb_group_topic t ON t.id = m.topic_id").
		Where("m.source IN ?", []int{define.FbGroupTopicSourceAuto, define.FbGroupTopicSourceManualAdd}).
		Group("m.topic_id").Scan(&counts); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	by := map[uint]topicCount{}
	for _, ct := range counts {
		by[ct.TopicID] = ct
	}
	out := make([]topicWithStats, 0, len(topics))
	for _, t := range topics {
		out = append(out, topicWithStats{Topic: t, GroupCount: by[t.ID].GroupCount, QualifiedCount: by[t.ID].QualifiedCount})
	}
	response.Data(c, out)
}

// GetTopic GET /fbgroup/topic/:id
func (h *Handler) GetTopic(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_data"))
		return
	}
	t, found, err := h.load(c, id)
	if err != nil {
		pagination.FailQuery(c, err)
		return
	}
	if !found {
		response.NotFound(c)
		return
	}
	response.Data(c, t)
}

func (h *Handler) load(c *gin.Context, id uint) (topicDao.Topic, bool, error) {
	topics := make([]topicDao.Topic, 0, 1)
	if err := tenant.ScopedDBNullBranch(c, h.DB, "fb_group_topic").Model(&topicDao.Topic{}).
		Select(topicColumns).Where("id = ?", id).Limit(1).Find(&topics).Error; err != nil {
		return topicDao.Topic{}, false, err
	}
	if len(topics) == 0 {
		return topicDao.Topic{}, false, nil
	}
	if err := loadBranches(h.DB, topics); err != nil {
		return topicDao.Topic{}, false, err
	}
	return topics[0], true, nil
}

// nameTaken — tên chủ đề đã dùng bởi chủ đề khác (UNIQUE uq_fb_group_topic_name).
func (h *Handler) nameTaken(name string, exceptID uint) (bool, error) {
	var n int64
	err := h.DB.Model(&topicDao.Topic{}).Where("name = ? AND id <> ?", name, exceptID).Count(&n).Error
	return n > 0, err
}

// CreateTopic POST /fbgroup/topic — tạo chủ đề + nhánh rồi phân loại lại toàn kho.
func (h *Handler) CreateTopic(c *gin.Context) {
	var in topicInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.HandleError(c, err)
		return
	}
	if err := in.normalize(); err != nil {
		response.HandleError(c, err)
		return
	}
	if taken, err := h.nameTaken(in.Name, 0); err != nil {
		response.ServerError(c, err)
		return
	} else if taken {
		response.Validate(c, i18n.TF(c, "fbgroup_topic_name_taken", in.Name))
		return
	}
	uid := middleware.GetUserID(c)
	status := 1
	if in.Status != nil {
		status = *in.Status
	}
	t := topicDao.Topic{
		BaseModel:       models.BaseModel{Status: &status},
		Name:            in.Name,
		Color:           in.Color,
		IncludeKeywords: toJSON(in.IncludeKeywords),
		ExcludeKeywords: toJSON(in.ExcludeKeywords),
		MinMembers:      in.MinMembers,
		MinPostsPerDay:  in.MinPostsPerDay,
		PublicOnly:      in.PublicOnly,
		SortOrder:       in.SortOrder,
	}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("userID", uid).Create(&t).Error; err != nil {
			return err
		}
		return saveBranches(tx, t.ID, in.Branches)
	})
	if err != nil {
		response.ServerError(c, err)
		return
	}
	h.reclassifyAndRespond(c, t.ID, http.StatusCreated)
}

// UpdateTopic PUT /fbgroup/topic/:id — sửa chủ đề + nhánh rồi phân loại lại toàn kho.
func (h *Handler) UpdateTopic(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_data"))
		return
	}
	var in topicInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.HandleError(c, err)
		return
	}
	if err := in.normalize(); err != nil {
		response.HandleError(c, err)
		return
	}
	_, found, err := h.load(c, id)
	if err != nil {
		pagination.FailQuery(c, err)
		return
	}
	if !found {
		response.NotFound(c)
		return
	}
	if taken, err := h.nameTaken(in.Name, id); err != nil {
		response.ServerError(c, err)
		return
	} else if taken {
		response.Validate(c, i18n.TF(c, "fbgroup_topic_name_taken", in.Name))
		return
	}
	upd := map[string]any{
		"name":              in.Name,
		"color":             in.Color,
		"include_keywords":  string(toJSON(in.IncludeKeywords)),
		"exclude_keywords":  string(toJSON(in.ExcludeKeywords)),
		"min_members":       in.MinMembers,
		"min_posts_per_day": in.MinPostsPerDay,
		"public_only":       in.PublicOnly,
		"sort_order":        in.SortOrder,
		"updated_at":        time.Now().Unix(),
	}
	if in.Status != nil {
		upd["status"] = *in.Status
	}
	if uid := middleware.GetUserID(c); uid > 0 {
		upd["updated_by"] = uid
	}
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("fb_group_topic").Where("id = ?", id).Updates(upd).Error; err != nil {
			return err
		}
		return saveBranches(tx, id, in.Branches)
	})
	if err != nil {
		response.ServerError(c, err)
		return
	}
	h.reclassifyAndRespond(c, id, http.StatusOK)
}

// saveBranches lưu mảng nhánh theo id (Hard Rule: CẤM delete-all-recreate — dòng map của group trỏ
// branch_id, xoá-tạo-lại sẽ làm mọi group mất nhánh). Nhánh có id ⇒ cập nhật; id 0 ⇒ tạo; nhánh cũ
// không còn trong payload ⇒ xoá (FK ON DELETE SET NULL ở fb_group_topic_map).
func saveBranches(tx *gorm.DB, topicID uint, in []branchInput) error {
	now := time.Now().Unix()
	existing := make([]uint, 0)
	if err := tx.Model(&topicDao.TopicBranch{}).Where("topic_id = ?", topicID).Pluck("id", &existing).Error; err != nil {
		return err
	}
	own := map[uint]bool{}
	for _, id := range existing {
		own[id] = true
	}
	keep := map[uint]bool{}
	for _, b := range in {
		if b.ID > 0 && own[b.ID] {
			keep[b.ID] = true
			if err := tx.Model(&topicDao.TopicBranch{}).Where("id = ? AND topic_id = ?", b.ID, topicID).
				Updates(map[string]any{"name": b.Name, "keywords": string(toJSON(b.Keywords)), "sort_order": b.SortOrder, "updated_at": now}).Error; err != nil {
				return err
			}
			continue
		}
		// id lạ (không thuộc chủ đề này) coi như nhánh mới — KHÔNG ghi vào nhánh của chủ đề khác.
		nb := topicDao.TopicBranch{TopicID: topicID, Name: b.Name, Keywords: toJSON(b.Keywords), SortOrder: b.SortOrder, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&nb).Error; err != nil {
			return err
		}
	}
	del := make([]uint, 0)
	for _, id := range existing {
		if !keep[id] {
			del = append(del, id)
		}
	}
	if len(del) > 0 {
		if err := tx.Where("topic_id = ? AND id IN ?", topicID, del).Delete(&topicDao.TopicBranch{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// reclassifyAndRespond phân loại lại toàn kho theo luật mới rồi trả chủ đề vừa lưu.
func (h *Handler) reclassifyAndRespond(c *gin.Context, id uint, status int) {
	n, err := service.ReclassifyAll(h.DB)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	t, found, err := h.load(c, id)
	if err != nil {
		pagination.FailQuery(c, err)
		return
	}
	if !found {
		response.NotFound(c)
		return
	}
	msg := i18n.TF(c, "fbgroup_topic_saved_reclassified", n)
	if status == http.StatusCreated {
		response.CreatedDataWithMessage(c, t, msg)
		return
	}
	response.DataWithMessage(c, t, msg)
}

// DeleteTopic DELETE /fbgroup/topic/:id — xoá chủ đề; nhánh + dòng gán của chủ đề xoá theo (FK CASCADE).
func (h *Handler) DeleteTopic(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_data"))
		return
	}
	res := tenant.ScopedDBNullBranch(c, h.DB, "fb_group_topic").Where("id = ?", id).Delete(&topicDao.Topic{})
	if res.Error != nil {
		response.HandleError(c, res.Error)
		return
	}
	if res.RowsAffected == 0 {
		response.NotFound(c)
		return
	}
	response.Deleted(c)
}

type previewInput struct {
	IncludeKeywords []string `json:"include_keywords"`
	ExcludeKeywords []string `json:"exclude_keywords"`
}

// PreviewTopic POST /fbgroup/topic/preview — đếm thử bộ từ khoá trên toàn kho, không ghi gì.
func (h *Handler) PreviewTopic(c *gin.Context) {
	var in previewInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.HandleError(c, err)
		return
	}
	inc, err := cleanKeywords(in.IncludeKeywords)
	if err != nil {
		response.HandleError(c, err)
		return
	}
	exc, err := cleanKeywords(in.ExcludeKeywords)
	if err != nil {
		response.HandleError(c, err)
		return
	}
	res, err := service.Preview(h.DB, inc, exc)
	if err != nil {
		pagination.FailQuery(c, err)
		return
	}
	response.Data(c, res)
}

// ReclassifyTopic POST /fbgroup/topic/reclassify — phân loại lại toàn kho theo luật hiện hành.
func (h *Handler) ReclassifyTopic(c *gin.Context) {
	n, err := service.ReclassifyAll(h.DB)
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.DataWithMessage(c, gin.H{"processed": n}, i18n.TF(c, "fbgroup_reclassified", n))
}
