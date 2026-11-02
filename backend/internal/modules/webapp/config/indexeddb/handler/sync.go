package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"erpCluvix/pkg/helpers"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/storage"
	"erpCluvix/pkg/tenant"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type tableConfig struct {
	Table   string
	Select  string
	OrderBy string
	Where   string
	// NullableBranch=true → dùng ScopedDBNullBranch (row branch_id NULL = áp dụng toàn company,
	// phải hiển thị ở mọi active branch). Mặc định ScopedDB sẽ filter `branch_id = X` và bỏ sót.
	NullableBranch bool
}

var configTables = map[string]tableConfig{
	"ethnicity":            {Table: "ethnicity"},
	"job":                  {Table: "job"},
	"controlFieldInTables": {Table: "control_field_in_table", OrderBy: "id ASC"},
	"groupSources":         {Table: "group_source"},
	"groupLocals":          {Table: "group_local"},
	"groupCountries":       {Table: "group_country"},
	"customerCategories":   {Table: "customer_category"},
	"hrDepartments":        {Table: "hr_department"},
	"noteTemplates":        {Table: "note_template"},
	"paymentMethods":       {Table: "payment_method"},
	"units":                {Table: "unit"},
	"locationCountries":    {Table: "location_country"},
	"locationProvinces":    {Table: "location_province"},
}

var customTables = map[string]func(c *gin.Context, db *gorm.DB) interface{}{
	"appointmentTypes": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, d.Table("appointment_type").Order("SortOrder DESC"))
	},
	"appointmentIsAccepteds": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, d.Table("appointment_is_accepted").Order("SortOrder DESC"))
	},
	"appointmentReasons": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, d.Table("appointment_reason").Order("SortOrder DESC"))
	},
	"appointmentNotBookedReasons": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, d.Table("appointment_not_booked_reasons").Order("SortOrder DESC"))
	},
	"paymentTypes": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, d.Table("card_type").Order("SortOrder DESC"))
	},
	"sources": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, tenant.ScopedDBNullBranch(c, d, "source").Table("source").Order("SortOrder DESC"))
	},
	// Story-40 (E6/D15 — 1 thương hiệu/DB): services/serviceTypes bỏ lọc company (trước: đọc thẳng
	// khoá context danh sách company). Hai bảng không thuộc branchTables — không có vế chi nhánh (trước cũng không).
	// Cột ảnh service/service_type/branch đã gỡ 2026-10-19 (chỉ Zalo Mini App đọc) ⇒ không còn ghép URL.
	"services": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, d.Table("service").Order("SortOrder DESC"))
	},
	"serviceTypes": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, d.Table("service_type").Order("SortOrder DESC"))
	},
	"branches": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, tenant.ScopedDBNullBranch(c, d, "branch").Table("branch").Order("SortOrder DESC"))
	},
	"users": func(c *gin.Context, d *gorm.DB) interface{} {
		return withImageURLs(findRows(c, tenant.ScopedDBNullBranch(c, d, "user_profile", "p").Table("user_profile p").
			Select("p.user_id as id, p.user_id, p.fullname, p.nickname, p.avatar, p.departmentList, u.status, u.branchId").
			Joins("JOIN user u ON u.id = p.user_id").
			Joins("JOIN rbac_auth_assignment a ON a.user_id = u.id").
			Where("u.id <> 1 AND a.item_name <> 'partner'")), "avatar")
	},
	"roles": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, d.Table("rbac_auth_item").
			Select("name as value, description as label, type").
			Where("type = 1 AND name NOT IN ('admin', 'develop', 'partner')"))
	},
	"settings": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, d.Table("setting"))
	},
	"userOptions": func(c *gin.Context, d *gorm.DB) interface{} {
		return findRows(c, d.Table("user_option").Order("SortOrder DESC"))
	},
}

// withImageURLs ghép URL công khai cho các cột ảnh (giá trị DB là KEY; URL tuyệt đối cũ giữ nguyên).
func withImageURLs(rows []map[string]interface{}, fields ...string) []map[string]interface{} {
	storage.ResolveRows(rows, fields...)
	return rows
}

// findRows — query + parse JSON fields
func findRows(c *gin.Context, q *gorm.DB) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0)
	tx := q.Find(&rows)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return rows
	}
	parseJSONFields(rows)
	if len(rows) > 0 {
		table := ""
		if tx.Statement != nil {
			table = tx.Statement.Table
		}
		if table != "" {
			helpers.ParseDecimalFieldsForTables(rows, table)
		}
	}
	return rows
}

// defaultDropdown — query for configTables with tenant scope
func defaultDropdown(c *gin.Context, db *gorm.DB, cfg tableConfig) interface{} {
	sel := cfg.Select
	if sel == "" {
		sel = "*"
	}
	orderBy := cfg.OrderBy
	if orderBy == "" {
		orderBy = "SortOrder DESC"
	}
	var scoped *gorm.DB
	if cfg.NullableBranch {
		scoped = tenant.ScopedDBNullBranch(c, db, cfg.Table)
	} else {
		scoped = tenant.ScopedDBNullBranch(c, db, cfg.Table)
	}
	q := scoped.Table(cfg.Table).Select(sel).Order(orderBy)
	if cfg.Where != "" {
		q = q.Where(cfg.Where)
	}
	return findRows(c, q)
}

// parseJSONFields — MySQL JSON columns scan as string in map[string]interface{}
// Convert to json.RawMessage so gin serializes as raw JSON (array/object/null)
func parseJSONFields(rows []map[string]interface{}) {
	for _, row := range rows {
		for k, v := range row {
			var s string
			switch val := v.(type) {
			case string:
				s = val
			case []byte:
				s = string(val)
			default:
				continue
			}
			if len(s) < 1 {
				continue
			}
			if s == "null" {
				row[k] = json.RawMessage("null")
				continue
			}
			if (s[0] == '[' && s[len(s)-1] == ']') || (s[0] == '{' && s[len(s)-1] == '}') {
				row[k] = json.RawMessage(s)
			}
		}
	}
}

// Sync — GET /crm/indexeddb/sync?tables=...
func (h *Handler) Sync(c *gin.Context) {
	tablesParam := c.Query("tables")
	if tablesParam == "" {
		tablesParam = "ethnicity,job,controlFieldInTables,settings,userOptions,groupSources,groupLocals,groupCountries,roles,appointmentReasons,customerCategories,hrDepartments,noteTemplates,appointmentNotBookedReasons,services,sources,paymentTypes,paymentMethods,taxRates,branches,users,appointmentTypes,units,serviceTypes,locationCountries,locationProvinces,appointmentIsAccepteds"
	}

	requested := strings.Split(tablesParam, ",")
	resp := gin.H{}

	for _, t := range requested {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if fn, ok := customTables[t]; ok {
			resp[t] = fn(c, h.DB)
			continue
		}
		if cfg, ok := configTables[t]; ok {
			resp[t] = defaultDropdown(c, h.DB, cfg)
			continue
		}
	}

	c.JSON(http.StatusOK, resp)
}
