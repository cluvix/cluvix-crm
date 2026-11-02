package dao

import "erpCluvix/internal/models"

type LocationProvince struct {
	models.SimpleModel
	CodeProvince  string ` gorm:"column:codeProvince;size:10" json:"codeProvince"`
	Name          string ` gorm:"size:255" json:"name"`
	Type          *int   ` gorm:"column:Type" json:"Type"`
	TelephoneCode *int   ` gorm:"column:TelephoneCode" json:"TelephoneCode"`
	CountryId     *int   ` gorm:"column:CountryId;index" json:"CountryId"`
	CountryCode   string ` gorm:"column:CountryCode;size:255" json:"CountryCode"`
	SortOrder     *int   ` gorm:"column:SortOrder" json:"SortOrder"`
	CreatedBy     *int   ` gorm:"column:created_by;index;default:1" json:"created_by"`
	UpdatedBy     *int   ` gorm:"column:updated_by;index;default:1" json:"updated_by"`
}

func (LocationProvince) TableName() string { return "location_province" }
