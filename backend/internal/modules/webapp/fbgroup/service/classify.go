// Package service — nghiệp vụ dùng chung của module Group Facebook: nạp luật chủ đề, ghi đè group nhận
// từ NextCore (API hoặc tệp CSV) và phân loại chủ đề.
//
// Bảng fb_group* KHÔNG có branch_id (dữ liệu toàn thương hiệu, 1 DB = 1 thương hiệu) nên ở đây dùng
// thẳng *gorm.DB truyền vào: đường nhận dữ liệu từ NextCore chạy ngoài phiên đăng nhập, không có scope
// chi nhánh để áp; đường có phiên (handler) cũng không có gì để lọc thêm trên các bảng này.
package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"erpCluvix/internal/modules/webapp/fbgroup/classify"
	groupDao "erpCluvix/internal/modules/webapp/fbgroup/group/dao"
	topicDao "erpCluvix/internal/modules/webapp/fbgroup/topic/dao"
	"erpCluvix/pkg/define"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ParseKeywords đọc cột JSON mảng chuỗi; JSON hỏng/rỗng ⇒ mảng rỗng.
func ParseKeywords(raw datatypes.JSON) []string {
	out := make([]string, 0)
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

// LoadRules nạp luật của mọi chủ đề đang dùng (status=1), xếp theo sort_order; nhánh theo sort_order.
func LoadRules(db *gorm.DB) ([]classify.Rule, error) {
	topics := make([]topicDao.Topic, 0)
	if err := db.Model(&topicDao.Topic{}).
		Select("id", "include_keywords", "exclude_keywords", "sort_order").
		Where("status = 1").Order("sort_order ASC, id ASC").Find(&topics).Error; err != nil {
		return nil, err
	}
	branches := make([]topicDao.TopicBranch, 0)
	if err := db.Model(&topicDao.TopicBranch{}).
		Select("id", "topic_id", "keywords", "sort_order").
		Order("sort_order ASC, id ASC").Find(&branches).Error; err != nil {
		return nil, err
	}
	byTopic := map[uint][]classify.Branch{}
	for _, b := range branches {
		byTopic[b.TopicID] = append(byTopic[b.TopicID], classify.Branch{ID: b.ID, Matcher: classify.NewMatcher(ParseKeywords(b.Keywords))})
	}
	rules := make([]classify.Rule, 0, len(topics))
	for _, t := range topics {
		rules = append(rules, classify.Rule{
			TopicID:  t.ID,
			Include:  classify.NewMatcher(ParseKeywords(t.IncludeKeywords)),
			Exclude:  classify.NewMatcher(ParseKeywords(t.ExcludeKeywords)),
			Branches: byTopic[t.ID],
		})
	}
	return rules, nil
}

type groupName struct {
	ID             uint
	NameNormalized string
	Province       *string
}

// ClassifyGroups gắn lại chủ đề tự động + tỉnh cho các group ids theo luật hiện hành.
// Dòng gán tay (source 2) và bỏ tay (source 3) giữ nguyên: chỉ xoá dòng source=1 rồi INSERT IGNORE,
// PK (group_id, topic_id) chặn ghi đè dòng tay.
func ClassifyGroups(tx *gorm.DB, rules []classify.Rule, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	rows := make([]groupName, 0, len(ids))
	if err := tx.Table("fb_group").Select("id", "name_normalized", "province").
		Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return err
	}
	now := time.Now().Unix()
	maps := make([]groupDao.TopicMap, 0, len(rows))
	provinceIDs := map[string][]uint{} // tỉnh mới → các group cần đổi ("" = xoá tỉnh)
	for _, r := range rows {
		for _, m := range classify.Classify(r.NameNormalized, rules) {
			tm := groupDao.TopicMap{GroupID: r.ID, TopicID: m.TopicID, Source: define.FbGroupTopicSourceAuto, CreatedAt: now}
			if m.BranchID > 0 {
				b := m.BranchID
				tm.BranchID = &b
			}
			maps = append(maps, tm)
		}
		p := classify.Province(r.NameNormalized)
		cur := ""
		if r.Province != nil {
			cur = *r.Province
		}
		if p != cur {
			provinceIDs[p] = append(provinceIDs[p], r.ID)
		}
	}
	if err := tx.Where("source = ? AND group_id IN ?", define.FbGroupTopicSourceAuto, ids).
		Delete(&groupDao.TopicMap{}).Error; err != nil {
		return err
	}
	if len(maps) > 0 {
		if err := tx.Clauses(clause.Insert{Modifier: "IGNORE"}).CreateInBatches(&maps, 500).Error; err != nil {
			return err
		}
	}
	for p, gids := range provinceIDs {
		var val any
		if p != "" {
			val = p
		}
		if err := tx.Table("fb_group").Where("id IN ?", gids).Update("province", val).Error; err != nil {
			return err
		}
	}
	return nil
}

// ReclassifyAll phân loại lại toàn kho theo lô define.FbGroupClassifyBatch. Trả số group đã xét.
func ReclassifyAll(db *gorm.DB) (int, error) {
	rules, err := LoadRules(db)
	if err != nil {
		return 0, err
	}
	total := 0
	var lastID uint
	for {
		ids := make([]uint, 0, define.FbGroupClassifyBatch)
		if err := db.Table("fb_group").Where("id > ?", lastID).Order("id ASC").
			Limit(define.FbGroupClassifyBatch).Pluck("id", &ids).Error; err != nil {
			return total, err
		}
		if len(ids) == 0 {
			return total, nil
		}
		if err := db.Transaction(func(tx *gorm.DB) error { return ClassifyGroups(tx, rules, ids) }); err != nil {
			return total, fmt.Errorf("phân loại lô từ id %d: %w", ids[0], err)
		}
		total += len(ids)
		lastID = ids[len(ids)-1]
	}
}

// PreviewResult — kết quả đếm thử 1 bộ luật trên toàn kho (không ghi gì).
type PreviewResult struct {
	Matched  int      `json:"matched"`
	Excluded int      `json:"excluded"` // khớp từ khoá gồm nhưng bị từ loại trừ chặn
	Samples  []string `json:"samples"`  // tên group mẫu, nhiều thành viên trước
}

// Preview đếm số group 1 luật sẽ khớp. Đọc theo lô để không nạp cả kho vào bộ nhớ một lúc.
func Preview(db *gorm.DB, include, exclude []string) (PreviewResult, error) {
	res := PreviewResult{Samples: make([]string, 0)}
	inc := classify.NewMatcher(include)
	exc := classify.NewMatcher(exclude)
	if inc.Empty() {
		return res, nil
	}
	type row struct {
		ID             uint
		Name           string
		NameNormalized string
		MemberCount    *int64
	}
	type sample struct {
		name    string
		members int64
	}
	samples := make([]sample, 0)
	var lastID uint
	for {
		rows := make([]row, 0, define.FbGroupClassifyBatch)
		if err := db.Table("fb_group").Select("id", "name", "name_normalized", "member_count").
			Where("id > ?", lastID).Order("id ASC").Limit(define.FbGroupClassifyBatch).Scan(&rows).Error; err != nil {
			return res, err
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			padded := " " + r.NameNormalized + " "
			if !inc.MatchPadded(padded) {
				continue
			}
			if exc.MatchPadded(padded) {
				res.Excluded++
				continue
			}
			res.Matched++
			var m int64
			if r.MemberCount != nil {
				m = *r.MemberCount
			}
			samples = append(samples, sample{r.Name, m})
		}
		lastID = rows[len(rows)-1].ID
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].members > samples[j].members })
	for i := 0; i < len(samples) && i < define.FbGroupPreviewSampleMax; i++ {
		res.Samples = append(res.Samples, samples[i].name)
	}
	return res, nil
}
