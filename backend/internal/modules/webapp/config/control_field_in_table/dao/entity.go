package dao

import "gorm.io/datatypes"

type ControlFieldInTable struct {
	ID      uint           `gorm:"primaryKey" json:"id" form:"-"`
	Name    string         ` gorm:"uniqueIndex;size:255;not null;comment:Tên bảng cần custom" json:"name"`
	Control datatypes.JSON `gorm:"type:json;comment:JSON cấu hình các field kiểm soát (required, edit, create...)" json:"control"`
}

func (ControlFieldInTable) TableName() string { return "control_field_in_table" }
