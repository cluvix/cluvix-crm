package export

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"erpCluvix/pkg/export"
	"erpCluvix/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ExportUnit exports unit list to Excel/CSV
func ExportUnit(c *gin.Context, q *gorm.DB) {
	categoryMap := map[int]string{1: "Đơn vị dịch vụ", 2: "Đơn vị kho", 3: "Đơn vị thuốc"}

	url, err := doExport(c, q, export.Config{
		FileName: "Don_vi_tinh",
		Headers: []string{
			"ID", "Tên đơn vị", "Mô tả",
			"Danh mục (categoryId)", "Tên danh mục",
			"Vị trí", "Trạng thái", "Trạng thái (text)",
		},
		ExportDir:   "./storage/export/",
		DownloadURL: DownloadURL,
	}, func(row map[string]interface{}) []interface{} {
		var catIDs []int
		if raw, ok := row["categoryId"]; ok && raw != nil {
			json.Unmarshal([]byte(fmt.Sprintf("%v", raw)), &catIDs)
		}
		var catNames []string
		for _, id := range catIDs {
			if name, ok := categoryMap[id]; ok {
				catNames = append(catNames, name)
			}
		}
		status := toInt(row["status"])
		statusText := "Ẩn"
		if status == 1 {
			statusText = "Hoạt động"
		}
		return []interface{}{
			row["id"], row["name"], row["description"],
			fmtJSON(catIDs), strings.Join(catNames, ", "),
			row["SortOrder"], status, statusText,
		}
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, url)
}

func fmtJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
