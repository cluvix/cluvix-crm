package dao

import "erpCluvix/internal/models"

// FbGroup — 1 group Facebook do NextCore GROUPCRAWL crawl về (bảng fb_group, migration 20261027200400).
// Không có branch_id: dữ liệu thị trường dùng chung toàn thương hiệu.
type FbGroup struct {
	models.BaseModel
	FacebookID     string  `gorm:"column:facebook_id;size:64;not null" json:"facebook_id"`
	Name           string  `gorm:"column:name;size:255;not null" json:"name"`
	NameNormalized string  `gorm:"column:name_normalized;size:255;not null" json:"-"`
	URL            string  `gorm:"column:url;size:512;not null" json:"url"`
	Slug           *string `gorm:"column:slug;size:128" json:"slug"`
	MemberCount    *int64  `gorm:"column:member_count" json:"member_count"`
	MemberRounded  bool    `gorm:"column:member_rounded" json:"member_rounded"`
	PostsPerDay    *int    `gorm:"column:posts_per_day" json:"posts_per_day"`
	Privacy        *int    `gorm:"column:privacy" json:"privacy"`
	NcbRegion      *string `gorm:"column:ncb_region;size:128" json:"ncb_region"`
	NcbCategory    *string `gorm:"column:ncb_category;size:160" json:"ncb_category"`
	Province       *string `gorm:"column:province;size:64" json:"province"`
	FoundKeyword   *string `gorm:"column:found_keyword;size:255" json:"found_keyword"`
	FoundByProfile *string `gorm:"column:found_by_profile;size:128" json:"found_by_profile"`
	NcbQuality     *int    `gorm:"column:ncb_quality" json:"ncb_quality"`
	WorkStatus     int     `gorm:"column:work_status;not null;default:0" json:"work_status"`
	Note           *string `gorm:"column:note;type:text" json:"note"`
	FirstSeenAt    *int64  `gorm:"column:first_seen_at" json:"first_seen_at"`
	LastSeenAt     *int64  `gorm:"column:last_seen_at" json:"last_seen_at"`
	SyncedAt       *int64  `gorm:"column:synced_at" json:"synced_at"`
}

func (FbGroup) TableName() string { return "fb_group" }

// TopicMap — group thuộc chủ đề nào (bảng fb_group_topic_map). Source: define.FbGroupTopicSource*.
type TopicMap struct {
	GroupID   uint  `gorm:"column:group_id;primaryKey" json:"group_id"`
	TopicID   uint  `gorm:"column:topic_id;primaryKey" json:"topic_id"`
	BranchID  *uint `gorm:"column:branch_id" json:"branch_id"`
	Source    int   `gorm:"column:source;not null" json:"source"`
	CreatedAt int64 `gorm:"column:created_at" json:"created_at"`
}

func (TopicMap) TableName() string { return "fb_group_topic_map" }
