// Package handler — nhận group Facebook từ NextCore Browser:
//   - POST /api/fbgroup/ingest: NextCore GROUPCRAWL › Đẩy dữ liệu › đích API gọi, KHÔNG JWT, xác thực
//     bằng khoá API riêng (Authorization: Bearer <khoá>). Payload theo tài liệu NextCore
//     docs.homestaynextcore.org/developer/day-du-lieu-groupcrawl, phienBan 1.
//   - GET/POST /api/fbgroup/ingest-key: xem / tạo lại khoá (có JWT + quyền).
//   - POST /api/fbgroup/group/import-csv: nhập tệp "Xuất CSV" của GROUPCRAWL (có JWT + quyền).
package handler

import "gorm.io/gorm"

type Deps struct {
	DB *gorm.DB
}

type Handler struct {
	DB *gorm.DB
}

func New(d Deps) *Handler { return &Handler{DB: d.DB} }
