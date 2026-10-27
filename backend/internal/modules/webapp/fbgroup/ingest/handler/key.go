package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"erpCluvix/internal/middleware"
	ingestDao "erpCluvix/internal/modules/webapp/fbgroup/ingest/dao"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/i18n"
	"erpCluvix/pkg/pagination"
	"erpCluvix/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const ctxIngestKeyID = "fbgroupIngestKeyID"

// IngestPath — đường NextCore gọi (hiển thị cho người dùng điền vào NextCore).
const IngestPath = "/api/fbgroup/ingest"

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// RequireIngestKey — middleware cho POST /api/fbgroup/ingest: header "Authorization: Bearer <khoá>",
// khoá phải khớp 1 dòng fb_group_ingest_key đang dùng. Tra theo SHA-256 nên không cần so sánh hằng-thời-gian.
func (h *Handler) RequireIngestKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		key := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if auth == "" || key == auth || !strings.HasPrefix(key, define.FbGroupIngestKeyPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": i18n.T(c, "fbgroup_ingest_key_invalid")})
			return
		}
		var id uint
		tx := h.DB.Model(&ingestDao.IngestKey{}).Select("id").
			Where("key_hash = ? AND status = 1", hashKey(key)).Limit(1).Scan(&id)
		if pagination.IsQueryError(tx.Error) {
			response.ServerError(c, tx.Error)
			c.Abort()
			return
		}
		if id == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": i18n.T(c, "fbgroup_ingest_key_invalid")})
			return
		}
		c.Set(ctxIngestKeyID, id)
		c.Next()
	}
}

type keyInfo struct {
	HasKey         bool   `json:"has_key"`
	KeyHint        string `json:"key_hint"`
	LastUsedAt     *int64 `json:"last_used_at"`
	LastBatchCount *int   `json:"last_batch_count"`
	TotalReceived  int64  `json:"total_received"`
	CreatedAt      int64  `json:"created_at"`
	IngestPath     string `json:"ingest_path"`
}

// GetIngestKey GET /fbgroup/ingest-key — trạng thái khoá đang dùng (KHÔNG trả khoá).
func (h *Handler) GetIngestKey(c *gin.Context) {
	var k ingestDao.IngestKey
	tx := h.DB.Model(&ingestDao.IngestKey{}).
		Select("id", "key_hint", "last_used_at", "last_batch_count", "total_received", "created_at").
		Where("status = 1").Order("id DESC").Limit(1).Find(&k)
	if pagination.IsQueryError(tx.Error) {
		pagination.FailQuery(c, tx.Error)
		return
	}
	info := keyInfo{IngestPath: IngestPath}
	if k.ID > 0 {
		info.HasKey = true
		info.KeyHint = k.KeyHint
		info.LastUsedAt = k.LastUsedAt
		info.LastBatchCount = k.LastBatchCount
		info.TotalReceived = k.TotalReceived
		info.CreatedAt = k.CreatedAt
	}
	response.Data(c, info)
}

// CreateIngestKey POST /fbgroup/ingest-key — tạo khoá mới, thu hồi mọi khoá cũ. Khoá chỉ trả về
// đúng 1 lần ở đây; DB chỉ giữ SHA-256 + 4 ký tự cuối.
func (h *Handler) CreateIngestKey(c *gin.Context) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		response.ServerError(c, err)
		return
	}
	key := define.FbGroupIngestKeyPrefix + base64.RawURLEncoding.EncodeToString(buf)
	uid := middleware.GetUserID(c)
	now := time.Now().Unix()
	k := ingestDao.IngestKey{KeyHash: hashKey(key), KeyHint: key[len(key)-4:]}
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		upd := map[string]any{"status": 0, "updated_at": now}
		if uid > 0 {
			upd["updated_by"] = uid
		}
		if err := tx.Model(&ingestDao.IngestKey{}).Where("status = 1").Updates(upd).Error; err != nil {
			return err
		}
		return tx.Set("userID", uid).Create(&k).Error
	})
	if err != nil {
		response.ServerError(c, err)
		return
	}
	response.CreatedDataWithMessage(c, gin.H{"key": key, "key_hint": k.KeyHint, "ingest_path": IngestPath},
		i18n.T(c, "fbgroup_ingest_key_created"))
}
