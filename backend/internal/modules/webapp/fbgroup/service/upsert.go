package service

import (
	"time"

	"erpCluvix/internal/modules/webapp/fbgroup/classify"
	groupDao "erpCluvix/internal/modules/webapp/fbgroup/group/dao"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GroupInput — 1 group nhận từ NextCore (payload API hoặc 1 dòng CSV), đã quy về kiểu của CRM.
type GroupInput struct {
	FacebookID     string
	Name           string
	URL            string
	Slug           *string
	MemberCount    *int64
	MemberRounded  bool
	PostsPerDay    *int
	Privacy        *int
	NcbRegion      *string
	NcbCategory    *string
	FirstSeenAt    *int64
	LastSeenAt     *int64
	FoundKeyword   *string // chỉ có ở tệp CSV
	FoundByProfile *string // chỉ có ở tệp CSV
	NcbQuality     *int    // chỉ có ở tệp CSV
}

// Cột ghi đè khi group đã có (theo facebook_id). KHÔNG gồm first_seen_at (giữ lần thấy đầu tiên),
// work_status/note (người dùng quản), created_*.
var upsertColumnsAPI = []string{
	"name", "name_normalized", "url", "slug", "member_count", "member_rounded", "posts_per_day",
	"privacy", "ncb_region", "ncb_category", "last_seen_at", "synced_at", "updated_at",
}

// Tệp CSV có thêm từ khoá/profile tìm ra và cờ "được NextCore giữ" mà luồng API không gửi.
var upsertColumnsCSV = append(append([]string{}, upsertColumnsAPI...), "found_keyword", "found_by_profile", "ncb_quality")

// UpsertResult — số liệu sau khi ghi.
type UpsertResult struct {
	Received int `json:"received"`
	Created  int `json:"created"`
	Updated  int `json:"updated"`
	Skipped  int `json:"skipped"` // dòng bị bỏ vì thiếu mã/tên/link hoặc sai định dạng (handler điền)
}

// UpsertGroups ghi đè các group theo facebook_id rồi phân loại chủ đề cho đúng các group đó, tất cả
// trong 1 transaction. fromCSV = true thì ghi thêm các cột chỉ tệp CSV mới có.
// Trùng facebook_id trong cùng lô: dòng sau thắng.
func UpsertGroups(db *gorm.DB, inputs []GroupInput, fromCSV bool) (UpsertResult, error) {
	res := UpsertResult{}
	byID := make(map[string]GroupInput, len(inputs))
	order := make([]string, 0, len(inputs))
	for _, in := range inputs {
		if in.FacebookID == "" || in.Name == "" || in.URL == "" {
			continue
		}
		if _, ok := byID[in.FacebookID]; !ok {
			order = append(order, in.FacebookID)
		}
		byID[in.FacebookID] = in
	}
	res.Received = len(order)
	if len(order) == 0 {
		return res, nil
	}
	cols := upsertColumnsAPI
	if fromCSV {
		cols = upsertColumnsCSV
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		var existing int64
		if err := tx.Table("fb_group").Where("facebook_id IN ?", order).Count(&existing).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		rows := make([]groupDao.FbGroup, 0, len(order))
		for _, fid := range order {
			in := byID[fid]
			g := groupDao.FbGroup{
				FacebookID:     in.FacebookID,
				Name:           in.Name,
				NameNormalized: classify.Normalize(in.Name),
				URL:            in.URL,
				Slug:           in.Slug,
				MemberCount:    in.MemberCount,
				MemberRounded:  in.MemberRounded,
				PostsPerDay:    in.PostsPerDay,
				Privacy:        in.Privacy,
				NcbRegion:      in.NcbRegion,
				NcbCategory:    in.NcbCategory,
				FirstSeenAt:    in.FirstSeenAt,
				LastSeenAt:     in.LastSeenAt,
				SyncedAt:       &now,
			}
			if fromCSV {
				g.FoundKeyword, g.FoundByProfile, g.NcbQuality = in.FoundKeyword, in.FoundByProfile, in.NcbQuality
			}
			rows = append(rows, g)
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "facebook_id"}},
			DoUpdates: clause.AssignmentColumns(cols),
		}).CreateInBatches(&rows, 200).Error; err != nil {
			return err
		}
		ids := make([]uint, 0, len(order))
		if err := tx.Table("fb_group").Where("facebook_id IN ?", order).Pluck("id", &ids).Error; err != nil {
			return err
		}
		rules, err := LoadRules(tx)
		if err != nil {
			return err
		}
		if err := ClassifyGroups(tx, rules, ids); err != nil {
			return err
		}
		res.Updated = int(existing)
		res.Created = len(order) - int(existing)
		return nil
	})
	return res, err
}
