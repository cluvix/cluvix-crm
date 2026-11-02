// Package appointment — Phase 4 expand of refactor-roadmap.
//
// Read layer cho services/crm/appointment.go. Repository pattern cho phép
// unit test các edge case mà không cần MySQL.

package repo

import (
	"gorm.io/gorm"
)

// CustomerInfo cho Create flow validate (giống pattern với OTP).
type CustomerInfo struct {
	BranchID *int `gorm:"column:branch_id"`
}

// Reader exposes read queries dùng trong appointment service.
type Reader interface {
	// CustomerInfo — Create flow load customer (branch_id).
	CustomerInfo(tx *gorm.DB, customerID int) (CustomerInfo, error)
}

func NewGormReader() Reader { return gormReader{} }

type gormReader struct{}

func (gormReader) CustomerInfo(tx *gorm.DB, customerID int) (CustomerInfo, error) {
	var c CustomerInfo
	err := tx.Table("customer").
		Select("branch_id").
		Where("id = ?", customerID).Take(&c).Error
	return c, err
}
