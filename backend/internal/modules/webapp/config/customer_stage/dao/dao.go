package dao

import (
	"erpCluvix/pkg/repository"

	"gorm.io/gorm"
)

type IDao interface {
	repository.BaseInterface[CustomerStage]
}

type Dao struct {
	*repository.BaseRepo[CustomerStage]
}

var _ IDao = (*Dao)(nil)

func New(db *gorm.DB) *Dao {
	return &Dao{repository.New[CustomerStage](db, "customer_stage")}
}
