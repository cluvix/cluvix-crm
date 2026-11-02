//go:build integration

package service

import (
	"errors"
	"log"
	"os"
	"testing"

	"gorm.io/gorm"

	"erpCluvix/internal/testhelpers"
)

// testDB được TestMain set khi có DB integration; nil nếu không có.
var testDB *gorm.DB

func TestMain(m *testing.M) {
	// SchemaAll: chạy đủ migration (cần customer.stage_id + customer_stage_history).
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaAll)
	if err != nil {
		if errors.Is(err, testhelpers.ErrNoDSN) {
			log.Printf("testhelpers: %v — skipping integration tests", err)
			os.Exit(m.Run())
		}
		// DSN có mà dựng DB hỏng ⇒ KHÔNG được im lặng skip (xanh giả).
		log.Fatalf("testhelpers: %v", err)
	}
	defer s.Close()
	testDB = s.DB
	os.Exit(m.Run())
}
