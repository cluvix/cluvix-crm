//go:build integration

package handler_test

import (
	"errors"
	"log"
	"os"
	"testing"

	"gorm.io/gorm"

	"erpCluvix/db"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/logger"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	logger.Init(logger.Config{Env: "test", AppCode: "customer_stage_handler_test"})
	// SchemaAll: toàn bộ migration trừ file seed (`*_seed_*`) — migration create_customer_stage tự seed 8 bước,
	// nên mỗi test PHẢI tự dọn danh mục về trạng thái biết trước (resetCatalog), không dựa vào seed.
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
	// pagination.Paginate đếm bằng db.DB toàn cục ⇒ trỏ vào DB test (nil ⇒ panic ở handler list).
	db.DB = s.DB
	os.Exit(m.Run())
}
