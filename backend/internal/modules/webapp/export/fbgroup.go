package export

import (
	"net/http"

	"erpCluvix/pkg/define"
	pkgExport "erpCluvix/pkg/export"
	"erpCluvix/pkg/response"
	"erpCluvix/pkg/tz"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ExportFbGroups — xuất Excel danh sách group Facebook theo đúng bộ lọc + sắp xếp của màn danh sách
// (GET /fbgroup/group?_export=1). q đã mang WHERE/ORDER của list; ở đây chỉ thay danh sách cột.
// Dữ liệu là phần công khai của group (không có dữ liệu khách) ⇒ cùng quyền xem viewFbGroup.
func ExportFbGroups(c *gin.Context, q *gorm.DB) {
	loc := tz.LocFromContext(c, q.Session(&gorm.Session{NewDB: true}))
	eq := q.Session(&gorm.Session{}).Select(`fb_group.id, fb_group.facebook_id, fb_group.name, fb_group.url,
		fb_group.member_count, fb_group.posts_per_day, fb_group.privacy, fb_group.province, fb_group.found_keyword,
		fb_group.work_status, fb_group.note, fb_group.last_seen_at,
		(SELECT GROUP_CONCAT(t.name ORDER BY t.sort_order SEPARATOR ', ')
			FROM fb_group_topic_map m JOIN fb_group_topic t ON t.id = m.topic_id
			WHERE m.group_id = fb_group.id AND m.source IN (?, ?)) AS topics`,
		define.FbGroupTopicSourceAuto, define.FbGroupTopicSourceManualAdd)

	url, err := doExport(c, eq, pkgExport.Config{
		FileName: "Group_Facebook",
		Headers: []string{
			"id", "Mã group", "Tên group", "Link", "Thành viên", "Bài/ngày", "Chế độ", "Tỉnh/thành",
			"Chủ đề", "Từ khoá tìm ra", "Trạng thái", "Ghi chú", "Thấy lần cuối",
		},
		ExportDir:   "./storage/export/",
		DownloadURL: DownloadURL,
	}, func(row map[string]interface{}) []interface{} {
		privacy := ""
		switch toInt(row["privacy"]) {
		case define.FbGroupPrivacyPublic:
			privacy = "Công khai"
		case define.FbGroupPrivacyPrivate:
			privacy = "Riêng tư"
		}
		return []interface{}{
			row["id"], row["facebook_id"], row["name"], row["url"], row["member_count"], row["posts_per_day"],
			privacy, row["province"], bytesToString(row["topics"]), row["found_keyword"],
			define.FbGroupWorkStatusLabels[toInt(row["work_status"])], row["note"],
			fmtUnixDate(row["last_seen_at"], loc),
		}
	})
	if err != nil {
		response.HandleError(c, err)
		return
	}
	c.JSON(http.StatusOK, url)
}
