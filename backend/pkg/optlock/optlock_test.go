package optlock

import (
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Model giả lập entity CRM: có hook BeforeUpdate tự set UpdatedAt = now (như models.BaseModel)
// + autoUpdateTime theo quy ước tên field UpdatedAt int64.
type appt struct {
	ID        uint `gorm:"primaryKey"`
	Content   string
	Status    int
	UpdatedAt int64
}

func (appt) TableName() string { return "appointment" }

func (a *appt) BeforeUpdate(*gorm.DB) error { a.UpdatedAt = 1; return nil }

// printTemplateFixture — DryRun fixture cho bảng thêm ở story-57 (D21).
type printTemplateFixture struct {
	ID        uint `gorm:"primaryKey"`
	Name      string
	UpdatedAt int64
}

func (printTemplateFixture) TableName() string { return "print_templates" }

type other struct {
	ID        uint `gorm:"primaryKey"`
	Name      string
	UpdatedAt int64
}

// TableName cố tình KHÁC "print_templates" (bảng thật, đã tracked từ story-57) — đây là fixture
// bảng KHÔNG liên quan, chỉ để kiểm callback bỏ qua bảng ngoài tập `tables`.
func (other) TableName() string { return "unrelated_table" }

func dryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "u:p@tcp(127.0.0.1:1)/x",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := Register(db); err != nil {
		t.Fatal(err)
	}
	return db
}

const bumpSQL = "`updated_at`=GREATEST(?, updated_at + 1)"

func assertBump(t *testing.T, name string, tx *gorm.DB) string {
	t.Helper()
	if tx.Error != nil {
		t.Fatalf("%s: %v", name, tx.Error)
	}
	sql := tx.Statement.SQL.String()
	if !strings.HasPrefix(sql, "UPDATE") {
		t.Fatalf("%s: not an UPDATE: %s", name, sql)
	}
	if n := strings.Count(sql, "`updated_at`="); n != 1 {
		t.Fatalf("%s: want exactly 1 updated_at assignment, got %d: %s", name, n, sql)
	}
	if !strings.Contains(sql, bumpSQL) {
		t.Fatalf("%s: updated_at not bumped with GREATEST: %s", name, sql)
	}
	return sql
}

func TestCallback_BumpsEveryUpdateForm(t *testing.T) {
	db := dryDB(t)
	cases := map[string]func() *gorm.DB{
		"Save": func() *gorm.DB {
			return db.Save(&appt{ID: 5, Content: "x", UpdatedAt: 100})
		},
		"Updates(map)": func() *gorm.DB {
			return db.Model(&appt{ID: 5}).Updates(map[string]any{"content": "x"})
		},
		"Updates(map) caller sets updated_at": func() *gorm.DB {
			return db.Model(&appt{ID: 5}).Updates(map[string]any{"content": "x", "updated_at": int64(123)})
		},
		"Updates(struct)": func() *gorm.DB {
			return db.Model(&appt{ID: 5}).Updates(appt{Content: "x", Status: 2})
		},
		"Update(col)": func() *gorm.DB {
			return db.Model(&appt{}).Where("id = ?", 5).Update("status", 2)
		},
		"UpdateColumn": func() *gorm.DB {
			return db.Model(&appt{}).Where("id = ?", 5).UpdateColumn("status", 2)
		},
		"UpdateColumns(map)": func() *gorm.DB {
			return db.Model(&appt{}).Where("id = ?", 5).UpdateColumns(map[string]any{"status": 2})
		},
		"Table(no schema).Updates(map)": func() *gorm.DB {
			return db.Table("customer").Where("id = ?", 5).Updates(map[string]any{"nickname": "x", "updated_at": 9})
		},
		"Table(alias).Updates(map)": func() *gorm.DB {
			return db.Table("reminder AS r").Where("r.id = ?", 5).Updates(map[string]any{"status": 2})
		},
		"Select(cols).Updates(struct)": func() *gorm.DB {
			return db.Model(&appt{ID: 5}).Select("status").Updates(&appt{Status: 3})
		},
		"print_templates Save": func() *gorm.DB {
			return db.Save(&printTemplateFixture{ID: 7, Name: "x", UpdatedAt: 100})
		},
		"print_templates Select(cols).Updates(struct)": func() *gorm.DB {
			return db.Model(&printTemplateFixture{ID: 3}).Select("name").Updates(&printTemplateFixture{Name: "x"})
		},
	}
	for name, run := range cases {
		assertBump(t, name, run())
	}
}

func TestCallback_UntrackedTableUntouched(t *testing.T) {
	db := dryDB(t)
	sql := db.Model(&other{ID: 1}).Updates(map[string]any{"name": "x"}).Statement.SQL.String()
	if strings.Contains(sql, "GREATEST") {
		t.Fatalf("untracked table must not be bumped: %s", sql)
	}
	sql = db.Table("service_type").Where("id = ?", 1).Updates(map[string]any{"name": "x"}).Statement.SQL.String()
	if strings.Contains(sql, "GREATEST") {
		t.Fatalf("service_type is not 'service': %s", sql)
	}
}

// SET do callback dựng phải bị xoá sau câu lệnh: chained instance (clone=0) tái dùng Statement,
// còn sót SET ⇒ lần Updates sau ghi lại cột của lần trước. (DryRun giữ nguyên SQL buffer nên
// soi thẳng Clauses thay vì so SQL lần 2.)
func TestCallback_ChainedReuseDoesNotLeakSet(t *testing.T) {
	db := dryDB(t)
	q := db.Model(&appt{}).Where("id = ?", 5)
	assertBump(t, "first", q.Updates(map[string]any{"content": "first"}))
	if _, ok := q.Statement.Clauses["SET"]; ok {
		t.Fatal("SET clause built by callback leaked into reused statement")
	}
}

// TestSkipBump — story-57 review-loop 1: ghi hệ thống qua `SkipBump` KHÔNG được
// đổi `updated_at`, kể cả auto-update-time mặc định của GORM cho field tên `UpdatedAt` (không chỉ
// bump GREATEST của callback này) — nếu không mỗi lần scheduler chạy vẫn làm 409 giả cho form đang
// mở, chỉ đổi từ GREATEST sang giá trị `now()` thường.
func TestSkipBump(t *testing.T) {
	db := dryDB(t)

	sql := SkipBump(db.Model(&printTemplateFixture{})).Where("id = ?", 5).Update("name", "y").Statement.SQL.String()
	if strings.Contains(sql, "updated_at") {
		t.Fatalf("SkipBump: updated_at KHÔNG được có trong SET: %s", sql)
	}

	sql = SkipBump(db.Model(&printTemplateFixture{})).Where("id = ?", 5).
		Updates(map[string]any{"name": "y"}).Statement.SQL.String()
	if strings.Contains(sql, "updated_at") {
		t.Fatalf("SkipBump Updates(map): updated_at KHÔNG được có trong SET: %s", sql)
	}

	// Đối chứng: KHÔNG SkipBump vẫn phải bump như cũ (chứng minh SkipBump thật sự làm khác đi,
	// không phải bumpCallback vô tình đã bỏ qua printTemplateFixture vì lý do khác).
	sql = db.Model(&printTemplateFixture{}).Where("id = ?", 5).Update("name", "y").Statement.SQL.String()
	if !strings.Contains(sql, bumpSQL) {
		t.Fatalf("đối chứng: print_templates không SkipBump phải bump như mọi bảng tracked khác: %s", sql)
	}
}

func TestHelpers_AddLockWhere(t *testing.T) {
	db := dryDB(t)
	tx := db.Session(&gorm.Session{})
	// Updates: DryRun ⇒ RowsAffected 0 ⇒ ErrVersionConflict là đúng; chỉ soi SQL.
	stmtDB := tx.Table("payment").Where("id = ?", 5)
	_, _ = Updates(stmtDB, 1700000000, map[string]any{"amount": 1})
	sql := stmtDB.Statement.SQL.String()
	if !strings.Contains(sql, "updated_at = ?") || !strings.Contains(sql, bumpSQL) {
		t.Fatalf("Updates helper SQL missing lock where/bump: %s", sql)
	}
	m := &appt{ID: 9, Content: "c", UpdatedAt: 50}
	saveDB := db.Session(&gorm.Session{})
	if err := Save(saveDB, m, 50); err == nil {
		t.Fatal("DryRun Save should report conflict (RowsAffected 0)")
	}
}

func TestNext(t *testing.T) {
	if Next(100, 100) != 101 || Next(100, 99) != 100 || Next(100, 50) != 100 || Next(100, 200) != 201 {
		t.Fatal("Next must be strictly greater than expected and >= now")
	}
}

func TestIsTracked(t *testing.T) {
	for _, s := range []string{"customer", "`user`", "customer c", "db.user_profile", "Payment", "print_templates"} {
		if !IsTracked(s) {
			t.Fatalf("%q should be tracked", s)
		}
	}
	for _, s := range []string{"", "service_type", "user_option", "print_template"} {
		if IsTracked(s) {
			t.Fatalf("%q should not be tracked", s)
		}
	}
}
