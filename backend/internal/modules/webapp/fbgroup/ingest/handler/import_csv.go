package handler

import (
	"bufio"
	"encoding/csv"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"erpCluvix/internal/middleware"
	"erpCluvix/internal/modules/webapp/fbgroup/service"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/errs"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/response"

	"github.com/gin-gonic/gin"
)

// Cột bắt buộc của tệp "Xuất CSV" GROUPCRAWL (tài liệu NextCore, mục Xuất CSV).
var csvRequiredCols = []string{"group_id", "ten", "url"}

// ImportCSV POST /fbgroup/group/import-csv (multipart, field "file") — nhập tệp Xuất CSV của
// GROUPCRAWL. Tệp này chỉ gồm group NextCore Phân tích lại GIỮ, kèm từ khoá + profile tìm ra.
func (h *Handler) ImportCSV(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		response.Validate(c, i18n.T(c, "fbgroup_import_file_required"))
		return
	}
	if !strings.EqualFold(filepath.Ext(fh.Filename), ".csv") {
		response.Validate(c, i18n.T(c, "fbgroup_import_csv_only"))
		return
	}
	f, err := fh.Open()
	if err != nil {
		response.ServerError(c, err)
		return
	}
	defer f.Close()

	inputs, skipped, err := readGroupCSV(f)
	if err != nil {
		response.HandleError(c, err) // LocalizedError ⇒ 422 + thông điệp i18n
		return
	}
	uid := middleware.GetUserID(c)
	total := service.UpsertResult{}
	const chunk = define.FbGroupClassifyBatch
	for i := 0; i < len(inputs); i += chunk {
		end := min(i+chunk, len(inputs))
		r, err := service.UpsertGroups(h.DB.Set("userID", uid), inputs[i:end], true)
		if err != nil {
			response.ServerError(c, err)
			return
		}
		total.Received += r.Received
		total.Created += r.Created
		total.Updated += r.Updated
	}
	total.Skipped = skipped
	response.DataWithMessage(c, total, i18n.TF(c, "fbgroup_import_done", total.Received))
}

// readGroupCSV đọc tệp CSV (UTF-8, có thể có BOM) thành GroupInput. Trả số dòng bị bỏ.
func readGroupCSV(r io.Reader) ([]service.GroupInput, int, error) {
	br := bufio.NewReader(r)
	if b, _ := br.Peek(3); len(b) == 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		_, _ = br.Discard(3)
	}
	cr := csv.NewReader(br)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	header, err := cr.Read()
	if err != nil {
		return nil, 0, errs.NewLocalizedError("fbgroup_import_no_header")
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(strings.ToLower(h))] = i
	}
	for _, need := range csvRequiredCols {
		if _, ok := col[need]; !ok {
			return nil, 0, errs.NewLocalizedError("fbgroup_import_missing_column", need)
		}
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	inputs := make([]service.GroupInput, 0, 1024)
	skipped := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			skipped++
			continue
		}
		if len(inputs)+skipped >= define.FbGroupImportMaxRows {
			return nil, 0, errs.NewLocalizedError("fbgroup_import_too_many_rows", define.FbGroupImportMaxRows)
		}
		g := ingestGroup{
			ID:               get(rec, "group_id"),
			Ten:              get(rec, "ten"),
			URL:              get(rec, "url"),
			ThanhVienLamTron: get(rec, "thanh_vien_lam_tron") == "co",
			CheDo:            strPtr(get(rec, "che_do")),
			KhuVuc:           strPtr(get(rec, "khu_vuc")),
			DanhMuc:          strPtr(get(rec, "danh_muc")),
			LanDau:           strPtr(get(rec, "lan_dau")),
			LanCuoi:          strPtr(get(rec, "lan_cuoi")),
		}
		if v, err := strconv.ParseInt(get(rec, "thanh_vien"), 10, 64); err == nil {
			g.ThanhVien = &v
		}
		if v, err := strconv.ParseFloat(get(rec, "bai_ngay"), 64); err == nil && !math.IsNaN(v) {
			g.BaiNgay = &v
		}
		in, ok := fromPayload(g)
		if !ok {
			skipped++
			continue
		}
		in.FoundKeyword = trimPtr(strPtr(get(rec, "tu_khoa")), 255)
		in.FoundByProfile = trimPtr(strPtr(get(rec, "profile")), 128)
		kept := define.FbGroupNcbQualityKept
		in.NcbQuality = &kept
		inputs = append(inputs, in)
	}
	return inputs, skipped, nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
