// Package handler — chủ đề phân loại group Facebook: danh sách, thêm/sửa/xoá (kèm nhánh), xem trước
// số group khớp và phân loại lại toàn kho.
package handler

import "gorm.io/gorm"

type Deps struct {
	DB *gorm.DB
}

type Handler struct {
	DB *gorm.DB
}

func New(d Deps) *Handler { return &Handler{DB: d.DB} }
