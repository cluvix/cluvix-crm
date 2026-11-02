//go:build integration

package handler

import (
	"log"
	"os"
	"testing"

	"gorm.io/gorm"

	"erpCluvix/db"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/helpers"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	// SchemaAll: endpoint chi tiết select các cột thêm sau baseline (`activated_at`,
	// `installment_config`…).
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaAll)
	if err != nil {
		log.Printf("testhelpers: %v — skipping integration tests", err)
		os.Exit(m.Run())
	}
	defer s.Close()
	testDB = s.DB
	db.DB = testDB
	// Y như bootstrap production (internal/bootstrap/app.go:75-76) — thiếu thì ParseJSON/ParseDecimal
	// là NO-OP câm lặng và test chạy trên hình dạng response khác production.
	helpers.InitJSONColumnMap(testDB)
	helpers.InitDecimalColumnMap(testDB)
	os.Exit(m.Run())
}
