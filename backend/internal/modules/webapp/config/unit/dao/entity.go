package dao

import (
	"erpCluvix/internal/models"

	"gorm.io/datatypes"
)

type Unit struct {
	models.SimpleModel
	Name        string         `gorm:"size:255;not null;comment:Tên của đơn vị tính" json:"name"`
	Description string         `gorm:"comment:Mô tả đơn vị tính đó" json:"description"`
	CategoryId  datatypes.JSON `gorm:"column:categoryId;type:json;default:'[]';comment:Danh sách danh mục" json:"categoryId"`
	SortOrder   *int           `gorm:"column:SortOrder;comment:Vị trí hiển thị" json:"SortOrder"`
	CreatedBy   *int           `gorm:"column:created_by;index" json:"created_by"`
	UpdatedBy   *int           `gorm:"column:updated_by;index" json:"updated_by"`
}

func (Unit) TableName() string { return "unit" }
