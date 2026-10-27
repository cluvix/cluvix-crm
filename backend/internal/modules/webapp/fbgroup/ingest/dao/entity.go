package dao

import "erpCluvix/internal/models"

// IngestKey — khoá API NextCore dùng để đẩy group vào POST /api/fbgroup/ingest
// (bảng fb_group_ingest_key, migration 20261027200400). Chỉ lưu SHA-256 của khoá.
type IngestKey struct {
	models.BaseModel
	KeyHash        string `gorm:"column:key_hash;size:64;not null" json:"-"`
	KeyHint        string `gorm:"column:key_hint;size:8;not null" json:"key_hint"`
	LastUsedAt     *int64 `gorm:"column:last_used_at" json:"last_used_at"`
	LastBatchCount *int   `gorm:"column:last_batch_count" json:"last_batch_count"`
	TotalReceived  int64  `gorm:"column:total_received;not null" json:"total_received"`
}

func (IngestKey) TableName() string { return "fb_group_ingest_key" }
