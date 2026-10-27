package dao

import (
	"erpCluvix/internal/models"

	"gorm.io/datatypes"
)

// Topic — chủ đề phân loại group Facebook (bảng fb_group_topic, migration 20261027200400).
type Topic struct {
	models.BaseModel
	Name            string         `gorm:"column:name;size:128;not null" json:"name"`
	Color           string         `gorm:"column:color;size:16;not null" json:"color"`
	IncludeKeywords datatypes.JSON `gorm:"column:include_keywords;type:json;not null" json:"include_keywords"`
	ExcludeKeywords datatypes.JSON `gorm:"column:exclude_keywords;type:json;not null" json:"exclude_keywords"`
	MinMembers      int            `gorm:"column:min_members;not null" json:"min_members"`
	MinPostsPerDay  int            `gorm:"column:min_posts_per_day;not null" json:"min_posts_per_day"`
	PublicOnly      bool           `gorm:"column:public_only;not null" json:"public_only"`
	SortOrder       int            `gorm:"column:sort_order;not null" json:"sort_order"`
	Branches        []TopicBranch  `gorm:"-" json:"branches"`
}

func (Topic) TableName() string { return "fb_group_topic" }

// TopicBranch — nhánh trong chủ đề (bảng fb_group_topic_branch).
type TopicBranch struct {
	ID        uint           `gorm:"primaryKey" json:"id" form:"-"`
	TopicID   uint           `gorm:"column:topic_id;not null" json:"topic_id"`
	Name      string         `gorm:"column:name;size:128;not null" json:"name"`
	Keywords  datatypes.JSON `gorm:"column:keywords;type:json;not null" json:"keywords"`
	SortOrder int            `gorm:"column:sort_order;not null" json:"sort_order"`
	CreatedAt int64          `gorm:"column:created_at" json:"created_at"`
	UpdatedAt int64          `gorm:"column:updated_at" json:"updated_at"`
}

func (TopicBranch) TableName() string { return "fb_group_topic_branch" }
