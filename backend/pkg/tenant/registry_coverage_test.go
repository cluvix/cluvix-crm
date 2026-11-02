package tenant

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// dryRunDBForScope — GORM dựng SQL rồi dừng, KHÔNG mở kết nối tới database nào.
type nopConnScope struct{}

func (nopConnScope) Connect(context.Context) (driver.Conn, error) {
	return nil, errors.New("dry run: test này không được phép mở kết nối")
}
func (nopConnScope) Driver() driver.Driver { return nil }

func dryRunDBForScope(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sql.OpenDB(nopConnScope{}),
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("mở DryRun DB: %v", err)
	}
	return db
}

// Lưới chặn LỚP lỗi "tên bảng truyền cho ScopedDB* không có trong registry".
//
// `applyNullBranchScope` chỉ thêm vế scope khi `branchTables[table]` = true. Tên KHÔNG có trong registry ⇒ helper trả nguyên `db`: **không
// thêm vế nào, không lỗi, không cảnh báo**. Build xanh, test xanh, màn hình vẫn ra số — chỉ là ra
// số của mọi chi nhánh. Rà 2026-09-09 tìm thấy 6 chỗ như vậy trên 1.004 điểm gọi:
// `order_service_used` bị truyền ALIAS `"osu"` (report/service-type-statistics), và 5 bảng thật
// chưa từng được đăng ký (`bank_accounts`, `gcal_account`, `gcal_subscription`, `med_route`,
// `warehouse`).
//
// Test này bắt buộc MỌI tên literal truyền cho ScopedDB* phải hoặc nằm trong registry, hoặc nằm
// trong `khongCoCotScope` dưới đây kèm lý do. Thêm điểm gọi mới với tên lạ ⇒ đỏ ngay, người thêm
// phải trả lời "bảng này có cột scope không" thay vì để helper im lặng bỏ qua.
//
// ⚠ Test KHÔNG đọc được database, nên `khongCoCotScope` là lời khai của con người. Đã đối chiếu
// `information_schema` ngày 2026-09-09; thêm tên mới vào đây thì PHẢI kiểm lại bằng:
//
//	SELECT COLUMN_NAME FROM information_schema.COLUMNS
//	 WHERE TABLE_NAME='<bảng>' AND COLUMN_NAME = 'branch_id';
var khongCoCotScope = map[string]string{
	"branch":             "bảng chi nhánh — chính nó LÀ phạm vi, không có branch_id",
	"coupon_catagory":    "danh mục dùng chung, không có cột scope",
	"customer_care_type": "danh mục dùng chung, không có cột scope",
	"customer_category":  "danh mục dùng chung, không có cột scope",
	"customer_stage":     "danh mục dùng chung, không có cột scope",
	"group_source":       "danh mục dùng chung, không có cột scope",
	"material":           "danh mục vật tư, không có cột scope",
	"source":             "danh mục nguồn, không có cột scope",
	"user":               "phạm vi chi nhánh lưu ở cột JSON `branchId`, bản thân bảng không có cột branch_id",
	"user_profile":       "khoá chính là user_id, không có cột scope",
	"canary_scope":       "bảng giả trong test của chính package này",
	"warehouse":          "DB dựng từ migration chỉ có `branchId` (camelCase), KHÔNG có `branch_id`; cột `branch_id` là THỪA của riêng DB dev. Đăng ký vào branchTables ⇒ lỗi 1054 trên DB khách — xem ghi chú ở scope.go",
	"osu":                "CỐ Ý: đối chứng trong service_type_stats_scope_test.go — dạng SAI phải KHÔNG sinh vế scope",
	// Không có branch_id trên schema mới (`backend/migrations`, story-62) ⇒ không sinh vế scope nào
	// (1 doanh nghiệp/DB, dữ liệu dùng chung mọi chi nhánh hoặc đã scope qua bản ghi cha):
	"bank_accounts":          "không có branch_id — tài khoản ngân hàng dùng chung doanh nghiệp",
	"log_customer":           "không có branch_id — log theo khách, khách cha đã qua scope",
	"customer_stage_history": "không có branch_id — theo khách, khách cha đã qua scope",
	"note":                   "không có branch_id — khách cha đã qua scope",
	"reminder":               "không có branch_id — nhắc việc theo người nhận/khách",
	"service":                "không có branch_id — danh mục dịch vụ dùng chung doanh nghiệp",
	"service_type":           "không có branch_id — danh mục dùng chung doanh nghiệp",
	"dashboard_layout":       "không có branch_id — bố cục dashboard theo user (session) / mặc định thương hiệu (migration 20261019090925)",
	// Có branch_id nhưng KHÔNG phải quyền sở hữu (lý do đầy đủ ở scopeExempt, canary_integration_test.go):
	"attachment":      "branch_id là snapshot lúc upload; file theo ref cha xem cross-branch",
	"print_templates": "branch_id là THAM SỐ mẫu in (0 = mẫu chung), handler lọc tường minh",
	"medical_record":  "KHÔNG PHẢI BẢNG THẬT (bảng thật là emr_medical_record) — crm/attachment/handler/upload.go, nhánh ref_type='medical_record' hiện luôn lỗi runtime; nợ riêng, không phải lỗi scope",
}

var scopedCallRe = regexp.MustCompile(
	`Scoped(?:DBNullBranchSkipBranch|DBNullBranch|DB)\(\s*[^,]+,\s*[^,]+,\s*"([^"]+)"`)

func TestMoiTenBangTruyenChoScopedDBDeuPhaiDuocKhaiBao(t *testing.T) {
	root := filepath.Join("..", "..")
	found := map[string][]string{}

	for _, dir := range []string{"internal", "pkg"} {
		err := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range scopedCallRe.FindAllStringSubmatch(string(b), -1) {
				found[m[1]] = append(found[m[1]], path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("quét %s: %v", dir, err)
		}
	}

	// Đối chứng: phép quét phải THẤY các điểm gọi. Không có mốc này thì regex hỏng cũng "xanh".
	//
	// Mốc hạ từ 50 → 15 (story-15 crm-standalone, 2026-09-24): mốc 50 tính trên ERP đầy đủ (EMR,
	// med, inventory, omnichat, teamchat, gcal, zalo, comp, approval, accountant...). Cắt CRM
	// standalone xoá hết các domain đó (story-04..17) ⇒ số điểm gọi ScopedDB* thật giảm còn 24 ở
	// mốc này — mốc mới chỉ giữ vai trò "regex không vỡ", KHÔNG phải KPI coverage.
	if len(found) < 15 {
		t.Fatalf("chỉ thấy %d tên bảng — regex hỏng, phép quét vô nghĩa", len(found))
	}
	if _, ok := found["customer"]; !ok {
		t.Fatalf(`không thấy điểm gọi nào cho "customer" — phép quét vô nghĩa`)
	}

	var thieu []string
	for name, locs := range found {
		if branchTables[name] {
			continue
		}
		if _, ok := khongCoCotScope[name]; ok {
			continue
		}
		sort.Strings(locs)
		thieu = append(thieu, name+" ("+locs[0]+", "+itoa(len(locs))+" chỗ)")
	}
	if len(thieu) > 0 {
		sort.Strings(thieu)
		t.Fatalf("tên bảng truyền cho ScopedDB* không có trong branchTables và cũng "+
			"không được khai vào khongCoCotScope ⇒ scope bị bỏ IM LẶNG:\n  %s",
			strings.Join(thieu, "\n  "))
	}
}

// Không để `khongCoCotScope` phình ra bằng tên đã đăng ký (khai thừa = che mất lỗi thật sau này).
func TestKhongKhaiTrungTenDaDangKy(t *testing.T) {
	for name := range khongCoCotScope {
		if name == "osu" {
			continue // cố ý: alias của order_service_used, xem lý do trong map
		}
		// Story-35: dưới tag `integration`, TestMain của canary_integration_test.go ĐĂNG KÝ
		// `canary_scope` vào cả 2 registry (bảng giả của chính package) — build thường thì không.
		// Tên này phải nằm trong khongCoCotScope cho build thường (TestMoiTenBang... quét thấy literal
		// trong file canary) ⇒ bỏ qua ở đây, không phải khai trùng thật.
		if name == "canary_scope" {
			continue
		}
		if branchTables[name] {
			t.Errorf("%q vừa nằm trong registry vừa được khai là 'không có cột scope' — gỡ khỏi khongCoCotScope", name)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// Neo HÀNH VI của 4 bảng đã đăng ký (2026-09-09; `warehouse` đã bị GỠ — xem khongCoCotScope) — registry đúng chưa đủ, phải thấy vế scope
// thật sự xuất hiện trong SQL (vế branch).
// D14/story-15 crm-standalone (2026-09-24): xoá 3 case gcal_account/gcal_subscription/med_route
// — module `gcal` (story-06) và `med` (story-12) đã xoá toàn bộ, 2 bảng này không còn điểm gọi
// ScopedDB* nào trong code sống ⇒ đã gỡ khỏi `tenantTables` (xem TestMoiTenBangTruyenChoScopedDB...
// ở trên, quét source thật). Giữ `bank_accounts` (module config/bank_account còn sống).
func TestNamBangVuaDangKySinhDungVeScope(t *testing.T) {
	cases := []struct {
		table      string
		muonBranch bool
		vi_sao     string
	}{
		{"bank_accounts", false, "không có branch_id — không sinh vế nào"},
		// Đối chứng vế branch trên bảng có branch_id.
		{"customer", true, "có branch_id — sinh vế branch"},
	}
	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			db := dryRunDBForScope(t)
			var rows []map[string]any
			sqlText := applyNullBranchScope(db, Bounds{BranchActiveID: 2},
				tc.table, false).Table(tc.table).Select("id").Find(&rows).Statement.SQL.String()

			coBranch := strings.Contains(sqlText, tc.table+".branch_id")
			if coBranch != tc.muonBranch {
				t.Errorf("vế branch_id: có=%v, muốn=%v (%s)\nSQL: %s", coBranch, tc.muonBranch, tc.vi_sao, sqlText)
			}
		})
	}
}
