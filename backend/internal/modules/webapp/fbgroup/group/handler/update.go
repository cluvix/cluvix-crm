package handler

import (
	"strings"
	"time"
	"unicode/utf8"

	"erpCluvix/internal/middleware"
	groupDao "erpCluvix/internal/modules/webapp/fbgroup/group/dao"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const noteMaxRunes = 5000

// updateGroupInput — chỉ 2 trường người dùng sửa được; số liệu group do NextCore cập nhật.
// Con trỏ: nil = client không gửi (giữ nguyên).
type updateGroupInput struct {
	WorkStatus *int    `json:"work_status"`
	Note       *string `json:"note"`
}

// UpdateGroup PUT /fbgroup/group/:id — đổi trạng thái làm việc / ghi chú.
func (h *Handler) UpdateGroup(c *gin.Context) {
	id, ok := helpers.ParseID(c)
	if !ok {
		response.Validate(c, i18n.T(c, "invalid_data"))
		return
	}
	var in updateGroupInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.HandleError(c, err)
		return
	}
	var cur struct {
		ID         uint
		WorkStatus int
		Note       *string
	}
	tx := tenant.ScopedDBNullBranch(c, h.DB, "fb_group").Table("fb_group").
		Select("id", "work_status", "note").Where("id = ?", id).Limit(1).Scan(&cur)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	if cur.ID == 0 {
		response.NotFound(c)
		return
	}
	upd := map[string]any{}
	if in.WorkStatus != nil && *in.WorkStatus != cur.WorkStatus {
		if !define.FbGroupWorkStatuses[*in.WorkStatus] {
			response.Validate(c, i18n.T(c, "invalid_status"))
			return
		}
		upd["work_status"] = *in.WorkStatus
	}
	if in.Note != nil {
		n := strings.TrimSpace(*in.Note)
		if utf8.RuneCountInString(n) > noteMaxRunes {
			response.Validate(c, i18n.TF(c, "data_too_long", i18n.T(c, "fbgroup_note")))
			return
		}
		curNote := ""
		if cur.Note != nil {
			curNote = *cur.Note
		}
		if n != curNote {
			if n == "" {
				upd["note"] = nil
			} else {
				upd["note"] = n
			}
		}
	}
	if len(upd) > 0 {
		upd["updated_at"] = time.Now().Unix()
		if uid := middleware.GetUserID(c); uid > 0 {
			upd["updated_by"] = uid
		}
		if err := tenant.ScopedDBNullBranch(c, h.DB, "fb_group").Table("fb_group").
			Where("id = ?", id).Updates(upd).Error; err != nil {
			response.ServerError(c, err)
			return
		}
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
	response.Updated(c, d)
}

// bulkInput — thao tác trên nhiều group: đặt trạng thái và/hoặc gán / bỏ 1 chủ đề.
type bulkInput struct {
	IDs           []uint `json:"ids" binding:"required"`
	WorkStatus    *int   `json:"work_status"`
	AddTopicID    *uint  `json:"add_topic_id"`
	RemoveTopicID *uint  `json:"remove_topic_id"`
}

// BulkGroup POST /fbgroup/group/bulk
// Gán/bỏ chủ đề ghi dòng tay (source 2/3) — phân loại tự động về sau KHÔNG ghi đè.
func (h *Handler) BulkGroup(c *gin.Context) {
	var in bulkInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.HandleError(c, err)
		return
	}
	if in.WorkStatus == nil && in.AddTopicID == nil && in.RemoveTopicID == nil {
		response.Validate(c, i18n.T(c, "invalid_data"))
		return
	}
	if in.WorkStatus != nil && !define.FbGroupWorkStatuses[*in.WorkStatus] {
		response.Validate(c, i18n.T(c, "invalid_status"))
		return
	}
	seen := map[uint]bool{}
	ids := make([]uint, 0, len(in.IDs))
	for _, id := range in.IDs {
		if id > 0 && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 || len(ids) > define.FbGroupBulkMaxIDs {
		response.Validate(c, i18n.TF(c, "fbgroup_bulk_limit", define.FbGroupBulkMaxIDs))
		return
	}
	// Chỉ thao tác trên group có thật — id lạ bị bỏ, không tạo dòng chủ đề mồ côi.
	valid := make([]uint, 0, len(ids))
	if tx := tenant.ScopedDBNullBranch(c, h.DB, "fb_group").Table("fb_group").
		Where("id IN ?", ids).Pluck("id", &valid); pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	for _, topicID := range []*uint{in.AddTopicID, in.RemoveTopicID} {
		if topicID == nil {
			continue
		}
		var n int64
		if tx := h.DB.Table("fb_group_topic").Where("id = ?", *topicID).Count(&n); pagination.IsQueryError(tx.Error) {
			pagination.FailQuery(c, tx.Error)
			return
		}
		if n == 0 {
			response.Validate(c, i18n.T(c, "fbgroup_topic_not_found"))
			return
		}
	}
	if len(valid) > 0 {
		now := time.Now().Unix()
		uid := middleware.GetUserID(c)
		err := h.DB.Transaction(func(tx *gorm.DB) error {
			if in.WorkStatus != nil {
				upd := map[string]any{"work_status": *in.WorkStatus, "updated_at": now}
				if uid > 0 {
					upd["updated_by"] = uid
				}
				if err := tx.Table("fb_group").Where("id IN ? AND work_status <> ?", valid, *in.WorkStatus).
					Updates(upd).Error; err != nil {
					return err
				}
			}
			if in.AddTopicID != nil {
				if err := upsertManualTopic(tx, valid, *in.AddTopicID, define.FbGroupTopicSourceManualAdd, now); err != nil {
					return err
				}
			}
			if in.RemoveTopicID != nil {
				if err := upsertManualTopic(tx, valid, *in.RemoveTopicID, define.FbGroupTopicSourceManualRemoved, now); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			response.ServerError(c, err)
			return
		}
	}
	response.DataWithMessage(c, gin.H{"affected": len(valid), "ignored": len(ids) - len(valid)}, i18n.T(c, "updated"))
}

// upsertManualTopic ghi dòng tay cho (group, topic): có rồi thì chỉ đổi source, giữ nhánh tự động.
func upsertManualTopic(tx *gorm.DB, groupIDs []uint, topicID uint, source int, now int64) error {
	rows := make([]groupDao.TopicMap, 0, len(groupIDs))
	for _, gid := range groupIDs {
		rows = append(rows, groupDao.TopicMap{GroupID: gid, TopicID: topicID, Source: source, CreatedAt: now})
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "group_id"}, {Name: "topic_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"source"}),
	}).CreateInBatches(&rows, 500).Error
}
