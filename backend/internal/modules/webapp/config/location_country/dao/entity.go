package dao

import "erpCluvix/internal/models"

type LocationCountry struct {
	models.SimpleModel
	CountryCode   string ` gorm:"column:CountryCode;size:255" json:"CountryCode"`
	CommonName    string ` gorm:"column:CommonName;size:255" json:"CommonName"`
	FormalName    string ` gorm:"column:FormalName;size:255" json:"FormalName"`
	Capital       string ` gorm:"column:Capital;size:255" json:"Capital"`
	CurrencyCode  string ` gorm:"column:CurrencyCode;size:255" json:"CurrencyCode"`
	CurrencyName  string ` gorm:"column:CurrencyName;size:255" json:"CurrencyName"`
	TelephoneCode string ` gorm:"column:TelephoneCode;size:255" json:"TelephoneCode"`
	SortOrder     *int   ` gorm:"column:SortOrder" json:"SortOrder"`
	CreatedBy     *int   ` gorm:"column:created_by;index;default:1" json:"created_by"`
	UpdatedBy     *int   ` gorm:"column:updated_by;index;default:1" json:"updated_by"`
}

func (LocationCountry) TableName() string { return "location_country" }
