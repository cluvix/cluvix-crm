// Package repo — read query layer cho OrderPlan service.
//
// Reader interface ngăn rò rỉ GORM/SQL ra service layer; test inject
// fake Reader để unit-test pre-flight checks (Create customer-lookup,
// Delete blocking-children check) không cần MySQL.
//
// Moved 2026-05-16 từ internal/repository/order_plan/ vào
// layered module (Plan B.1 — CRM migration).
package repo

import (
	"gorm.io/gorm"
)

// CustomerInfo aggregate cho Customer table — đủ cho Create flow validate.
type CustomerInfo struct {
	BranchID     *int `gorm:"column:branch_id"`
	CustomerType *int `gorm:"column:customerType"`
}

// Reader exposes các read query mà OrderPlanService dùng trong
// Create/Update/Delete flow. Mở rộng dần khi extract thêm.
type Reader interface {
	// CustomerInfo returns 2 field từ customer cho Create validate.
	// Trả error khi customer không tồn tại (Take semantic).
	CustomerInfo(tx *gorm.DB, customerID int) (CustomerInfo, error)

	// CustomerBranchID returns customer.branch_id (NULL → nil).
	CustomerBranchID(tx *gorm.DB, customerID int) (*int, error)

	// OrderServiceUsedIDs returns IDs của order_service_used với order_plan_id = orderID.
	OrderServiceUsedIDs(tx *gorm.DB, orderID uint) ([]uint, error)

	// CountPaymentsForServices — số payment đã thu/chi trên các service_used.
	// > 0 → Delete bị block với errs.ErrServiceHasPayment.
	CountPaymentsForServices(tx *gorm.DB, serviceUsedIDs []uint) (int64, error)
}

// NewGormReader returns the production Reader backed by GORM.
func NewGormReader() Reader { return gormReader{} }

type gormReader struct{}

func (gormReader) CustomerInfo(tx *gorm.DB, customerID int) (CustomerInfo, error) {
	var c CustomerInfo
	err := tx.Table("customer").
		Select("branch_id, customerType").
		Where("id = ?", customerID).Take(&c).Error
	return c, err
}

func (gormReader) CustomerBranchID(tx *gorm.DB, customerID int) (*int, error) {
	var bid *int
	err := tx.Table("customer").Select("branch_id").Where("id = ?", customerID).Scan(&bid).Error
	return bid, err
}

func (gormReader) OrderServiceUsedIDs(tx *gorm.DB, orderID uint) ([]uint, error) {
	var ids []uint
	err := tx.Table("order_service_used").Where("order_plan_id = ?", orderID).Pluck("id", &ids).Error
	return ids, err
}

func (gormReader) CountPaymentsForServices(tx *gorm.DB, serviceUsedIDs []uint) (int64, error) {
	if len(serviceUsedIDs) == 0 {
		return 0, nil
	}
	var n int64
	err := tx.Table("payment").Where("serviceUsed_id IN ?", serviceUsedIDs).Count(&n).Error
	return n, err
}
