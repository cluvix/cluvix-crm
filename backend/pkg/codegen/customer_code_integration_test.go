//go:build integration

package codegen

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"erpCluvix/pkg/brand"
	"erpCluvix/pkg/define"
)

// Hồ sơ doanh nghiệp (`business_profile`, mã CLX = brand.Code) có 3 chi nhánh: 301 mã "Q1", 302 KHÔNG
// có mã (cả 2 có row config_code "customer" theo mặc định SeedConfigCodeForBranch: KH, kèm mã chi
// nhánh, reset theo tháng, 2 số), 303 mã "Q3" CHƯA seed config_code.
//
// Seed phải COMMIT thật (không nằm trong tx test): bộ đếm mã khách chạy trên kết nối riêng, mà FK
// config_code_monthly→config_code sẽ bắt kết nối đó chờ khoá nếu row config_code chưa commit. Khách
// thì insert trong tx trả về (rollback cuối test).
func seedCustomerCodeTenant(t *testing.T) *gorm.DB {
	t.Helper()
	if testDB == nil {
		t.Skip("cần TEST_MYSQL_DSN")
	}
	cleanup := func() {
		testDB.Exec("DELETE FROM config_code_monthly WHERE config_code_id IN (SELECT id FROM config_code WHERE branch_id IN (301, 302, 303))")
		testDB.Exec("DELETE FROM config_code WHERE branch_id IN (301, 302, 303)")
		testDB.Exec("DELETE FROM branch WHERE id IN (301, 302, 303)")
		testDB.Exec("DELETE FROM business_profile")
		brand.Invalidate()
	}
	cleanup()
	for _, q := range []string{
		"INSERT INTO business_profile (name, code) VALUES ('Doanh nghiệp CC', 'CLX')",
		"INSERT INTO branch (id, name, code, address) VALUES (301, 'Chi nhánh Q1', 'Q1', 'A')",
		"INSERT INTO branch (id, name, code, address) VALUES (302, 'Chi nhánh không mã', '', 'B')",
		"INSERT INTO branch (id, name, code, address) VALUES (303, 'Chi nhánh Q3', 'Q3', 'C')",
		`INSERT INTO config_code
			(branch_id, name, parent_table, infix_code, include_branch_code, monthly_reset, serial_length, created_at, updated_at)
			VALUES (301, 'Khách hàng', 'customer', 'KH', 1, 1, 2, 0, 0),
			       (302, 'Khách hàng', 'customer', 'KH', 1, 1, 2, 0, 0)`,
	} {
		if err := testDB.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
	brand.Invalidate()
	tx := testDB.Begin()
	t.Cleanup(func() {
		tx.Rollback()
		cleanup()
	})
	return tx
}

func insertCustomer(tx *gorm.DB, code any) error {
	return tx.Exec(`INSERT INTO customer (fullname, stage_id, created_day, created_at, updated_at, code)
		VALUES ('KH test', (SELECT id FROM customer_stage WHERE is_default = 1), 0, 0, 0, ?)`, code).Error
}

func TestNewCustomerCodeOptions(t *testing.T) {
	cases := []struct {
		option string
		want   func(ym string) []string // mã lần lượt: chi nhánh 301, 301, 302
	}{
		{"", func(ym string) []string {
			return []string{"CLX.Q1.KH." + ym + ".01", "CLX.Q1.KH." + ym + ".02", "CLX.KH." + ym + ".01"}
		}},
		{"abc", func(ym string) []string {
			return []string{"CLX.Q1.KH." + ym + ".01", "CLX.Q1.KH." + ym + ".02", "CLX.KH." + ym + ".01"}
		}},
		{"2", func(ym string) []string { return []string{"Q1." + ym + ".01", "Q1." + ym + ".02", "302." + ym + ".01"} }},
		{"3", func(string) []string { return []string{"Q1.01", "Q1.02", "302.01"} }},
		{"4", func(string) []string { return []string{"301.00001", "301.00002", "302.00001"} }},
		// Đếm chung toàn hệ thống: chi nhánh 302 đếm TIẾP từ 301.
		{"5", func(string) []string { return []string{"KH.00001", "KH.00002", "KH.00003"} }},
		{"6", func(ym string) []string {
			return []string{"CLX.KH." + ym + ".001", "CLX.KH." + ym + ".002", "CLX.KH." + ym + ".003"}
		}},
	}
	for _, c := range cases {
		t.Run("option="+c.option, func(t *testing.T) {
			tx := seedCustomerCodeTenant(t)
			t.Setenv(define.CustomerCodeOptionEnv, c.option)
			want := c.want(time.Now().In(brand.Location(tx)).Format("0601"))
			for i, branch := range []int64{301, 301, 302} {
				got := NewCustomerCode(tx, branch)
				if got != want[i] {
					t.Fatalf("lần %d (chi nhánh %d) = %q, muốn %q", i+1, branch, got, want[i])
				}
				// Lưu như luồng thật để lần sau dò trùng thấy nó.
				if err := insertCustomer(tx, got); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// Đổi option giữa chừng / dữ liệu cũ đã chiếm số ⇒ mã sinh ra trùng thì nhảy số tiếp, không trả mã trùng.
func TestNewCustomerCodeSkipsTakenCode(t *testing.T) {
	tx := seedCustomerCodeTenant(t)
	t.Setenv(define.CustomerCodeOptionEnv, "5")
	for _, code := range []string{"KH.00001", "KH.00002"} {
		if err := insertCustomer(tx, code); err != nil {
			t.Fatal(err)
		}
	}
	// Dò trùng TOÀN DB (1 doanh nghiệp/DB, D23 UNIQUE(code)): mã KH.00003 đã có khách dùng (vd dữ
	// liệu import) ⇒ nhảy qua, không cấp trùng.
	if err := insertCustomer(tx, "KH.00003"); err != nil {
		t.Fatal(err)
	}
	if got := NewCustomerCode(tx, 301); got != "KH.00004" {
		t.Fatalf("NewCustomerCode = %q, muốn KH.00004", got)
	}
}

func TestNewCustomerCodeNoBranch(t *testing.T) {
	tx := seedCustomerCodeTenant(t)
	for _, opt := range []string{"", "3", "5"} {
		t.Setenv(define.CustomerCodeOptionEnv, opt)
		if got := NewCustomerCode(tx, 0); got != "" {
			t.Fatalf("option %q, không chi nhánh ⇒ %q, muốn \"\"", opt, got)
		}
	}
}

func TestEnsureCustomerCode(t *testing.T) {
	tx := seedCustomerCodeTenant(t)
	t.Setenv(define.CustomerCodeOptionEnv, "3")
	if err := insertCustomer(tx, nil); err != nil {
		t.Fatal(err)
	}
	var noCode int64
	tx.Raw("SELECT LAST_INSERT_ID()").Scan(&noCode)
	if err := insertCustomer(tx, "OLD.01"); err != nil {
		t.Fatal(err)
	}
	var hasCode int64
	tx.Raw("SELECT LAST_INSERT_ID()").Scan(&hasCode)

	if got := EnsureCustomerCode(tx, noCode, 301); got != "Q1.01" {
		t.Fatalf("khách chưa có mã ⇒ %q, muốn Q1.01", got)
	}
	if got := EnsureCustomerCode(tx, hasCode, 301); got != "" {
		t.Fatalf("khách đã có mã ⇒ %q, muốn \"\" (không sinh lại)", got)
	}
}

// Schema mới (D23): UNIQUE(code) toàn bảng — trùng mã bị chặn; nhiều khách chưa có mã (NULL) vẫn được.
func TestCustomerCodeUniqueConstraint(t *testing.T) {
	tx := seedCustomerCodeTenant(t)
	if err := insertCustomer(tx, "DUP.01"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := insertCustomer(tx, nil); err != nil {
			t.Fatalf("nhiều khách chưa có mã (NULL) phải được: %v", err)
		}
	}
	if err := insertCustomer(tx, "DUP.01"); err == nil {
		t.Fatal("trùng code phải bị UNIQUE chặn")
	}
}

// [6] Chi nhánh chưa seed config_code ⇒ findConfig mượn row chi nhánh khác (bộ đếm của 301), nhưng
// mã PHẢI mang mã của CHÍNH chi nhánh khách (Q3), không phải Q1 của row mượn.
func TestCustomerCodeDefaultUsesOwnBranchCode(t *testing.T) {
	tx := seedCustomerCodeTenant(t)
	t.Setenv(define.CustomerCodeOptionEnv, "")
	ym := time.Now().In(brand.Location(tx)).Format("0601")
	if got, want := NewCustomerCode(tx, 303), "CLX.Q3.KH."+ym+".01"; got != want {
		t.Fatalf("NewCustomerCode(branch 303 chưa seed) = %q, muốn %q", got, want)
	}
}

// [2] Bộ đếm mã khách tăng trên kết nối riêng, autocommit: (a) tx nghiệp vụ đang mở KHÔNG giữ khoá bộ
// đếm — tx khác sinh mã ngay, không chờ; (b) tx nghiệp vụ rollback thì số đã cấp vẫn bị tiêu.
func TestCustomerSerialOutsideBusinessTx(t *testing.T) {
	txA := seedCustomerCodeTenant(t)
	t.Setenv(define.CustomerCodeOptionEnv, "3")
	if got := NewCustomerCode(txA, 301); got != "Q1.01" {
		t.Fatalf("txA = %q, muốn Q1.01", got)
	}

	txB := testDB.Begin()
	defer txB.Rollback()
	txB.Exec("SET SESSION innodb_lock_wait_timeout = 2")
	start := time.Now()
	got := NewCustomerCode(txB, 301)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("txB chờ %v — bộ đếm vẫn bị khoá bởi txA đang mở", elapsed)
	}
	if got != "Q1.02" {
		t.Fatalf("txB = %q, muốn Q1.02", got)
	}

	txA.Rollback()
	var last int64
	testDB.Raw(`SELECT m.last_number FROM config_code_monthly m JOIN config_code c ON c.id = m.config_code_id
		WHERE c.branch_id = 301 AND c.parent_table = 'customer' AND m.year = 0`).Scan(&last)
	if last != 2 {
		t.Fatalf("last_number sau rollback = %d, muốn 2 (số đã cấp không quay lại)", last)
	}
}

// [7] Sự cố 2026-09-22 (tạo đơn hàng mất 51s): tx nghiệp vụ vừa tăng bộ đếm của bảng KHÁC (mã đơn,
// sinh trong BeforeCreate) rồi mới sinh mã khách. Bộ đếm cũ dùng INSERT ... ON DUPLICATE KEY UPDATE
// cho dòng đã có ⇒ tx giữ khoá supremum của config_code_monthly ⇒ kết nối riêng của bộ đếm mã khách
// chờ chính tx đó tới lock-wait-timeout. Cả 2 bộ đếm phải đã có dòng của kỳ (đúng điều kiện sự cố).
func TestCustomerSerialAfterOtherCounterInSameTx(t *testing.T) {
	tx := seedCustomerCodeTenant(t)
	t.Setenv(define.CustomerCodeOptionEnv, "3")
	if err := testDB.Exec(`INSERT INTO config_code
		(branch_id, name, parent_table, infix_code, include_branch_code, monthly_reset, serial_length, created_at, updated_at)
		VALUES (301, 'Đơn hàng', 'order_plan', 'DH', 1, 1, 2, 0, 0)`).Error; err != nil {
		t.Fatal(err)
	}
	bid := uint(301)
	// Tạo sẵn dòng bộ đếm của cả 2 (autocommit) — lượt sau trong tx là nhánh "đã có dòng".
	if Generate(testDB, "order_plan", &bid) == "" || NewCustomerCode(testDB, 301) != "Q1.01" {
		t.Fatal("không tạo được dòng bộ đếm ban đầu")
	}

	if Generate(tx, "order_plan", &bid) == "" {
		t.Fatal("Generate mã đơn trong tx trả rỗng")
	}
	start := time.Now()
	got := NewCustomerCode(tx, 301)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("sinh mã khách chờ %v — kết nối riêng bị khoá bởi bộ đếm mã đơn của chính tx", elapsed)
	}
	if got != "Q1.02" {
		t.Fatalf("NewCustomerCode = %q, muốn Q1.02", got)
	}
}

// holdCustomerCounter mở tx riêng giữ khoá X dòng bộ đếm mã khách của chi nhánh 301 (tạo sẵn dòng nếu
// chưa có) cho tới khi test kết thúc — dựng tình huống "tx khác đang giữ bộ đếm".
func holdCustomerCounter(t *testing.T) {
	t.Helper()
	if NewCustomerCode(testDB, 301) == "" { // tạo dòng bộ đếm (autocommit)
		t.Fatal("không tạo được dòng bộ đếm")
	}
	holder := testDB.Begin()
	t.Cleanup(func() { holder.Rollback() })
	if err := holder.Exec(`UPDATE config_code_monthly SET last_number = last_number
		WHERE config_code_id IN (SELECT id FROM config_code WHERE branch_id = 301 AND parent_table = 'customer')`).Error; err != nil {
		t.Fatal(err)
	}
}

// [8] Bộ đếm lỗi (lock-wait) ⇒ GenerateErr trả lỗi, Generate trả "" — KHÔNG bịa serial 1 (bản cũ ra
// "Q1.01" trùng mã đã cấp; bảng không UNIQUE như journal_entry.entry_no là trùng số chứng từ im lặng).
func TestGenerateErrOnCounterFailure(t *testing.T) {
	tx := seedCustomerCodeTenant(t)
	t.Setenv(define.CustomerCodeOptionEnv, "3")
	holdCustomerCounter(t)
	tx.Exec("SET SESSION innodb_lock_wait_timeout = 1")
	bid := uint(301)

	code, err := GenerateErr(tx, "customer", &bid)
	if err == nil || code != "" {
		t.Fatalf("GenerateErr khi bộ đếm bị khoá = (%q, %v), muốn (\"\", lỗi)", code, err)
	}
	if got := Generate(tx, "customer", &bid); got != "" {
		t.Fatalf("Generate khi bộ đếm bị khoá = %q, muốn \"\"", got)
	}
	// config_code chưa seed bảng này KHÔNG phải lỗi — giữ hành vi cũ.
	if code, err := GenerateErr(tx, "bang_chua_seed", &bid); code != "" || err != nil {
		t.Fatalf("bảng chưa seed = (%q, %v), muốn (\"\", nil)", code, err)
	}
}

// [9] Kết nối riêng chờ khoá có hạn (customerSerialLockWait), KHÔNG trọn 50s mặc định; đã lùi về tx
// một lần thì các lượt sau của cùng coder KHÔNG mở lại kết nối riêng (import: mỗi dòng còn lại từng
// mất trọn thời gian chờ). Bộ đếm bị tx khác giữ suốt test ⇒ cả 2 lượt đều ra "".
func TestCustomerSerialBoundedWaitAndStickyFallback(t *testing.T) {
	tx := seedCustomerCodeTenant(t)
	t.Setenv(define.CustomerCodeOptionEnv, "3")
	holdCustomerCounter(t)
	tx.Exec("SET SESSION innodb_lock_wait_timeout = 1") // lượt đếm trong tx chờ 1s

	cc := NewCustomerCoder(tx)
	start := time.Now()
	if got := cc.New(301); got != "" {
		t.Fatalf("lượt 1 = %q, muốn \"\" (bộ đếm đang bị khoá)", got)
	}
	limit := time.Duration(customerSerialLockWait+3) * time.Second
	if elapsed := time.Since(start); elapsed > limit {
		t.Fatalf("lượt 1 chờ %v > %v — kết nối riêng không bị giới hạn thời gian chờ khoá", elapsed, limit)
	}
	start = time.Now()
	if got := cc.New(301); got != "" {
		t.Fatalf("lượt 2 = %q, muốn \"\"", got)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("lượt 2 chờ %v — coder đã lùi về tx vẫn mở lại kết nối riêng", elapsed)
	}
}

// [11] 2 request đồng thời Ensure cho CÙNG khách chưa mã: bên thua (đọc ảnh chụp cũ thấy rỗng) KHÔNG
// được ghi đè mã bên thắng đã cấp.
func TestEnsureCustomerCodeConcurrentClaim(t *testing.T) {
	seedCustomerCodeTenant(t)
	t.Setenv(define.CustomerCodeOptionEnv, "3")
	if err := insertCustomer(testDB, nil); err != nil {
		t.Fatal(err)
	}
	var custID int64
	testDB.Raw("SELECT MAX(id) FROM customer WHERE fullname = 'KH test'").Scan(&custID)
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer WHERE fullname = 'KH test'") })

	loser := testDB.Begin()
	defer loser.Rollback()
	var snap string
	loser.Raw("SELECT COALESCE(code, '') FROM customer WHERE id = ?", custID).Scan(&snap) // chốt ảnh chụp: mã rỗng

	winner := testDB.Begin()
	won := EnsureCustomerCode(winner, custID, 301)
	if won == "" {
		winner.Rollback()
		t.Fatal("bên thắng không sinh được mã")
	}
	winner.Commit()

	if got := EnsureCustomerCode(loser, custID, 301); got != "" {
		t.Fatalf("bên thua = %q, muốn \"\" (không ghi đè mã %q đã cấp)", got, won)
	}
	var stored string
	testDB.Raw("SELECT code FROM customer WHERE id = ?", custID).Scan(&stored)
	if stored != won {
		t.Fatalf("mã trong DB = %q, muốn %q", stored, won)
	}
}

// [5] CustomerCoder cache row config_code + mã chi nhánh trong 1 request — nhiều mã liên tiếp vẫn
// tăng đúng bộ đếm.
func TestCustomerCoderSequence(t *testing.T) {
	tx := seedCustomerCodeTenant(t)
	t.Setenv(define.CustomerCodeOptionEnv, "3")
	cc := NewCustomerCoder(tx)
	for i, want := range []string{"Q1.01", "Q1.02", "Q1.03"} {
		if got := cc.New(301); got != want {
			t.Fatalf("lần %d = %q, muốn %q", i+1, got, want)
		}
	}
}
