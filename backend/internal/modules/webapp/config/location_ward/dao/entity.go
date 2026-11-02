package dao

import "erpCluvix/internal/models"

type LocationWard struct {
	models.SimpleModel
	CodeWard   string ` gorm:"column:codeWard;size:10" json:"codeWard"`
	Name       string ` gorm:"size:255;not null" json:"name"`
	Path       string ` gorm:"size:255" json:"path"`
	Type       *int   ` gorm:"column:Type" json:"Type"`
	ProvinceId int    ` gorm:"column:province_id;index;not null" json:"province_id"`
	SortOrder  *int   ` gorm:"column:SortOrder" json:"SortOrder"`
	CreatedBy  *int   ` gorm:"column:created_by;index;default:1" json:"created_by"`
	UpdatedBy  *int   ` gorm:"column:updated_by;index;default:1" json:"updated_by"`
}

func (LocationWard) TableName() string { return "location_ward" }
