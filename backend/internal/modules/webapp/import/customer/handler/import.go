package handler

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"erpCluvix/internal/middleware"
	stagesvc "erpCluvix/internal/modules/webapp/config/customer_stage/service"
	"erpCluvix/pkg/codegen"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/exceldate"
	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tenant"
	"erpCluvix/pkg/tz"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	"strconv"
)

// importBranchAllowed — chi nhánh ghi trong file import phải VỪA thuộc quyền người gọi
// (`middleware.IsBranchAllowed`) VỪA là chi nhánh có thật. Story-40 (E6/D15): thay
// helper middleware kiểm chi nhánh-theo-company (story-41 đã xoá; bỏ điều kiện company, giữ nguyên
// 2 điều kiện trên).
// Lỗi DB ⇒ từ chối (fail-closed như bản cũ).
func importBranchAllowed(c *gin.Context, db *gorm.DB, branchID uint) bool {
	if !middleware.IsBranchAllowed(c, branchID) || db == nil {
		return false
	}
	var count int64
	if err := db.Table("branch").Where("id = ?", branchID).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

var allowedCols = map[string]bool{
	"created_day": true, "old_code": true, "fullname": true, "phone": true,
	"gender": true, "customerType": true, "birthday": true,
	"address": true, "source_id": true, "payment_user_id": true, "branch_id": true,
}

var intCols = map[string]bool{
	"gender": true, "customerType": true, "source_id": true,
	"branch_id": true, "payment_user_id": true,
}

var moneyNoteLabels = map[string]bool{
	"Tổng tiền đã tiêu dùng": true,
	"Công nợ":                true,
}

func (h *Handler) ImportCustomerFromExcel(c *gin.Context) {
	// Công cụ migrate chạy tay khi onboard: file đặt sẵn trên server, đường dẫn đọc từ env (KHÔNG
	// nhận đường dẫn từ request — tránh đọc file tuỳ ý trên máy chủ).
	path := strings.TrimSpace(os.Getenv(define.EnvImportCustomerFile))
	if path == "" {
		response.Validate(c, i18n.T(c, "import_file_not_configured"))
		return
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		response.Validate(c, i18n.TF(c, "excel_read_failed", err.Error()))
		return
	}
	defer f.Close()

	rows, err := f.GetRows(f.GetSheetName(0), excelize.Options{RawCellValue: true})
	if err != nil {
		response.Validate(c, i18n.TF(c, "sheet_read_failed", err.Error()))
		return
	}
	if len(rows) < 3 {
		response.Validate(c, i18n.T(c, "no_data"))
		return
	}

	// colIndex: các cột trong allowlist
	// addrExtraCols: address1 / address2 — không vào DB, dùng để ghép địa chỉ
	// noteIndex: cột có label Row 0 nhưng không có tên DB Row 1
	colIndex := map[int]string{}
	addrExtraCols := map[int]string{} // value = "address1" | "address2"
	noteIndex := map[int]string{}
	for j, label := range rows[0] {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		var dbCol string
		if j < len(rows[1]) {
			dbCol = strings.TrimSpace(rows[1][j])
		}
		switch {
		case dbCol == "address1" || dbCol == "address2":
			addrExtraCols[j] = dbCol
		case dbCol != "" && allowedCols[dbCol]:
			colIndex[j] = dbCol
		case dbCol == "":
			noteIndex[j] = label
		}
	}

	userID := middleware.GetUserID(c)
	now := time.Now().Unix()
	loc := tz.LocFromContext(c, h.DB)
	uid := int(userID)

	tx := h.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()
	// 1 coder cho cả file: cache config_code + mã chi nhánh thay vì tra lại mỗi dòng.
	coder := codegen.NewCustomerCoder(tx)
	// branch_id đến từ FILE (client sửa được) ⇒ kiểm từng dòng: chi nhánh phải thuộc quyền user VÀ
	// là chi nhánh có thật (story-40: bỏ điều kiện company). Memo theo branch trong request.
	branchAllowed := map[uint]bool{}
	// Bước mặc định cho khách MỚI: đọc 1 lần trong cùng tx, lười — chỉ khi gặp dòng tạo mới đầu tiên
	// (file toàn khách đã có không bị chặn vì thiếu cấu hình bước mặc định).
	defaultStageID := 0

	for i, row := range rows[2:] {
		if allEmpty(row) {
			continue
		}

		data := map[string]interface{}{}
		for j, col := range colIndex {
			if j < len(row) && strings.TrimSpace(row[j]) != "" {
				data[col] = strings.TrimSpace(row[j])
			}
		}
		if len(data) == 0 {
			continue
		}

		rowNum := i + 3

		// Ghép address + address1 + address2
		var addrParts []string
		if v, ok := data["address"].(string); ok && v != "" {
			addrParts = append(addrParts, v)
		}
		for j, key := range addrExtraCols {
			_ = key
			if j < len(row) && strings.TrimSpace(row[j]) != "" {
				addrParts = append(addrParts, strings.TrimSpace(row[j]))
			}
		}
		if len(addrParts) > 0 {
			data["address"] = strings.Join(addrParts, ", ")
		}

		if v, ok := data["fullname"].(string); ok {
			data["fullname"] = helpers.ConvertToTitleCase(v)
		}

		if v, ok := data["phone"].(string); ok {
			if len(v) > 20 || (!reDigitsOnly.MatchString(v) && len(v) > 15) {
				delete(data, "phone")
			} else if len(v) == 9 && reDigitsOnly.MatchString(v) {
				data["phone"] = "0" + v
			}
		}

		if v, ok := data["created_day"].(string); ok {
			if t, okCD := exceldate.ParseDateTime(v, loc); okCD {
				data["created_day"] = t.Unix()
			} else {
				delete(data, "created_day")
			}
		}

		if v, ok := data["birthday"].(string); ok {
			if formatted, okBD := exceldate.ParseDateString(v); okBD {
				data["birthday"] = formatted
			} else {
				delete(data, "birthday")
			}
		}

		for col := range intCols {
			v, ok := data[col].(string)
			if !ok {
				continue
			}
			// ParseFloat để xử lý cả "2" lẫn "2.0"
			fv, err2 := strconv.ParseFloat(v, 64)
			if err2 != nil {
				tx.Rollback()
				response.Validate(c, i18n.TF(c, "import_row_error", rowNum,
					fmt.Sprintf("%s phải là số: %s", col, v)))
				return
			}
			data[col] = int(fv)
		}

		if bid := helpers.ToInt64(data["branch_id"]); bid != 0 {
			ok, memo := branchAllowed[uint(bid)]
			if !memo {
				ok = bid > 0 && importBranchAllowed(c, h.DB, uint(bid))
				branchAllowed[uint(bid)] = ok
			}
			if !ok {
				tx.Rollback()
				response.Validate(c, i18n.TF(c, "import_row_error", rowNum, i18n.T(c, "branch_id_not_allowed")))
				return
			}
		}

		var existingID uint
		phone, hasPhone := data["phone"].(string)
		fullname, hasFullname := data["fullname"].(string)
		if hasPhone && phone != "" && hasFullname && fullname != "" {
			// D-3: lỗi tra cứu KHÔNG được coi là "chưa có" (sẽ tạo trùng / nhận 1062 khó hiểu).
			if err2 := tenant.ScopedDBNullBranch(c, tx, "customer").
				Table("customer").Select("id").
				Where("phone = ? AND fullname = ?", phone, fullname).
				Scan(&existingID).Error; err2 != nil {
				tx.Rollback()
				response.ServerError(c, err2)
				return
			}
		}

		if existingID > 0 {
			if code := coder.Ensure(int64(existingID), helpers.ToInt64(data["branch_id"])); code != "" {
				data["code"] = code
			}
			data["updated_by"] = uid // updated_at: callback optlock bump GREATEST(now, updated_at+1)
			if err2 := tenant.ScopedDBNullBranch(c, tx, "customer").
				Table("customer").Where("id = ?", existingID).Updates(data).Error; err2 != nil {
				tx.Rollback()
				response.Validate(c, i18n.TF(c, "import_row_error", rowNum, importRowErrMsg(c, err2)))
				return
			}
		} else {
			data["country_id"] = defaultCountryID
			if _, ok := data["customerType"]; !ok {
				data["customerType"] = 2
			}
			data["data_id"] = 2
			data["created_at"] = now
			data["updated_at"] = now
			data["created_by"] = uid
			data["updated_by"] = uid
			if code := coder.New(helpers.ToInt64(data["branch_id"])); code != "" {
				data["code"] = code
			}
			if defaultStageID == 0 {
				sid, err2 := stagesvc.DefaultStageID(tx)
				if err2 != nil {
					tx.Rollback()
					response.Validate(c, i18n.TF(c, "import_row_error", rowNum, importRowErrMsg(c, err2)))
					return
				}
				defaultStageID = sid
			}
			data["stage_id"] = defaultStageID
			if err2 := tx.Table("customer").Create(data).Error; err2 != nil {
				tx.Rollback()
				response.Validate(c, i18n.TF(c, "import_row_error", rowNum, importRowErrMsg(c, err2)))
				return
			}
			// GORM Create từ map KHÔNG backfill id ⇒ LAST_INSERT_ID() trên chính tx (cùng kết nối).
			var newID int
			if err2 := tx.Raw("SELECT LAST_INSERT_ID()").Scan(&newID).Error; err2 != nil || newID <= 0 {
				tx.Rollback()
				if err2 == nil {
					err2 = fmt.Errorf("không lấy được ID khách hàng")
				}
				response.Validate(c, i18n.TF(c, "import_row_error", rowNum, importRowErrMsg(c, err2)))
				return
			}
			if err2 := stagesvc.RecordStageChange(tx, newID, nil, defaultStageID,
				define.CustomerStageHistorySourceImport, uid, now); err2 != nil {
				tx.Rollback()
				response.Validate(c, i18n.TF(c, "import_row_error", rowNum, importRowErrMsg(c, err2)))
				return
			}

			var noteLines []string
			for j, label := range noteIndex {
				if j >= len(row) || strings.TrimSpace(row[j]) == "" {
					continue
				}
				val := strings.TrimSpace(row[j])
				if moneyNoteLabels[label] {
					val = formatVND(stripThousands(val))
				}
				noteLines = append(noteLines, fmt.Sprintf(
					`<p><strong>%s:</strong> <span style="color:#1976D2;">%s</span></p>`,
					label, val,
				))
			}
			if len(noteLines) > 0 {
				if newID > 0 {
					noteRow := map[string]interface{}{
						"note":        strings.Join(noteLines, ""),
						"customer_id": newID,
						"created_by":  uid,
						"created_at":  now,
					}
					if err2 := tx.Table("note").Create(noteRow).Error; err2 != nil {
						tx.Rollback()
						response.Validate(c, i18n.TF(c, "import_row_error", rowNum, err2.Error()))
						return
					}
				}
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		response.ServerError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": i18n.T(c, "import_success")})
}

// importRowErrMsg — thông điệp lỗi của 1 dòng import khách. Trùng SĐT (1062 trên UNIQUE SĐT, D-3:
// SĐT là duy nhất trong 1 DB — kể cả khác định dạng 0xxx/84xxx, hoặc trùng dòng phía trên cùng file)
// ⇒ "customer_duplicate" (i18n) thay vì chuỗi lỗi MySQL thô.
func importRowErrMsg(c *gin.Context, err error) string {
	if helpers.IsCustomerPhoneDuplicate(err) {
		return i18n.T(c, "customer_duplicate")
	}
	if errors.Is(err, stagesvc.ErrDefaultStageMissing) {
		return i18n.T(c, stagesvc.KeyDefaultMissing)
	}
	return err.Error()
}
