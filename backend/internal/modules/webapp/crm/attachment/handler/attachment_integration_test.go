//go:build integration

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appdb "erpCluvix/db"
	customerEntity "erpCluvix/internal/modules/webapp/crm/customer/dao"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
	"erpCluvix/pkg/logger"
	"erpCluvix/pkg/storage"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaAll)
	if err != nil {
		log.Printf("testhelpers: %v — skipping integration tests", err)
		os.Exit(m.Run())
	}
	defer s.Close()
	testDB = s.DB
	appdb.DB = s.DB // pagination.Paginate đếm qua db.DB toàn cục
	if logger.L == nil {
		logger.L = zap.NewNop() // response.ServerError ghi log qua logger.L
	}
	os.Exit(m.Run())
}

const testPDF = "%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n"

type attEnv struct {
	pub, priv *storage.Local
	h         *Handler
}

func newAttEnv(t *testing.T) *attEnv {
	t.Helper()
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	dir := t.TempDir()
	pub, err := storage.NewLocal(filepath.Join(dir, "uploads"), "/uploads", "", true)
	require.NoError(t, err)
	priv, err := storage.NewLocal(filepath.Join(dir, "private"), "", "", false)
	require.NoError(t, err)
	return &attEnv{pub: pub, priv: priv, h: New(Deps{DB: testDB, PrivateStore: priv, PublicStore: pub})}
}

// engine dựng đúng các route của module + static /uploads của kho public (như router thật khi
// driver=local) để chứng minh file private KHÔNG lộ qua đường static.
func (e *attEnv) engine(uid uint, h *Handler) *gin.Engine {
	r := testhelpers.NewGin()
	r.StaticFS(e.pub.PublicPrefix(), gin.Dir(e.pub.Root(), false))
	g := r.Group("/api/crm", testhelpers.AuthMiddleware(0, uid))
	g.GET("/customer/:id/attachments", h.ListByCustomer)
	g.POST("/customer/:id/attachments", h.UploadForCustomer)
	g.GET("/attachment/:id/download", h.Download)
	g.DELETE("/attachment/:id", h.Delete)
	return r
}

func do(r *gin.Engine, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func multipartBody(t *testing.T, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, _ = fw.Write(content)
	require.NoError(t, mw.Close())
	return &buf, mw.FormDataContentType()
}

// seedCustomer — khách chưa gán chi nhánh, người phụ trách = owner.
func seedCustomer(t *testing.T, owner uint) uint {
	t.Helper()
	now := time.Now().Unix()
	row := map[string]any{"fullname": fmt.Sprintf("KH att %d", time.Now().UnixNano()), "stage_id": testhelpers.MustDefaultCustomerStageID(t, testDB),
		"payment_user_id": owner, "created_day": now, "created_at": now, "updated_at": now}
	require.NoError(t, testDB.Table("customer").Create(&row).Error)
	id := testhelpers.LastInsertID(testDB)
	t.Cleanup(func() {
		testDB.Exec("DELETE FROM attachment WHERE customer_id = ?", id)
		testDB.Exec("DELETE FROM customer WHERE id = ?", id)
	})
	return id
}

// seedTelesale — user thuộc phòng Telesale (bị giới hạn "khách mình phụ trách" khi option 5 tắt).
func seedTelesale(t *testing.T) uint {
	t.Helper()
	uid := testhelpers.CreateUser(t, testDB, "tele", nil)
	require.NoError(t, testDB.Table("user_profile").Create(map[string]any{
		"user_id": uid, "fullname": "Tele", "departmentList": fmt.Sprintf("[%d]", define.HrDepartmentTelesale),
	}).Error)
	t.Cleanup(func() { testDB.Exec("DELETE FROM user_profile WHERE user_id = ?", uid) })
	return uid
}

// optionTeleOff ép user_option 5 (telesale thấy mọi khách) = TẮT trong lúc test.
func optionTeleOff(t *testing.T) {
	t.Helper()
	var prev []int
	testDB.Table("user_option").Where("id = ?", define.UserOptionViewLeadOfTele).Pluck("options", &prev)
	testDB.Table("user_option").Where("id = ?", define.UserOptionViewLeadOfTele).Update("options", 0)
	t.Cleanup(func() {
		if len(prev) > 0 {
			testDB.Table("user_option").Where("id = ?", define.UserOptionViewLeadOfTele).Update("options", prev[0])
		}
	})
}

func decodeData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body.Data
}

func TestAttachment_PrivateUploadDownloadDelete(t *testing.T) {
	e := newAttEnv(t)
	optionTeleOff(t)
	owner := testhelpers.CreateUser(t, testDB, "own", nil)
	tele := seedTelesale(t)
	cid := seedCustomer(t, owner)
	r := e.engine(owner, e.h)

	// ── upload ⇒ nằm ở kho PRIVATE, response không có URL công khai
	body, ct := multipartBody(t, "Ho So Nguyen Van A.pdf", []byte(testPDF))
	w := do(r, http.MethodPost, fmt.Sprintf("/api/crm/customer/%d/attachments", cid), body, ct)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	data := decodeData(t, w)
	_, hasFileURL := data["file_url"]
	require.False(t, hasFileURL, "response KHÔNG được trả file_url")
	attID := uint(data["id"].(float64))
	require.Equal(t, fmt.Sprintf("/crm/attachment/%d/download", attID), data["download_path"])
	require.Equal(t, "application/pdf", data["file_type"])

	var att customerEntity.Attachment
	require.NoError(t, testDB.Where("id = ?", attID).First(&att).Error)
	require.False(t, storage.IsAbsoluteURL(att.FileUrl), "DB lưu KEY, không phải URL: %q", att.FileUrl)
	require.Equal(t, ".pdf", filepath.Ext(att.FileUrl), "đuôi theo MIME đã sniff")
	require.NotContains(t, strings.ToLower(att.FileUrl), "nguyen", "tên gốc không lộ vào key")
	privPath := filepath.Join(e.priv.Root(), filepath.FromSlash(att.FileUrl))
	_, err := os.Stat(privPath)
	require.NoError(t, err, "file phải nằm trong thư mục private")
	_, err = os.Stat(filepath.Join(e.pub.Root(), filepath.FromSlash(att.FileUrl)))
	require.True(t, os.IsNotExist(err), "file KHÔNG được nằm trong thư mục public")

	// ── không truy cập được qua static /uploads
	require.Equal(t, http.StatusNotFound, do(r, http.MethodGet, "/uploads/"+att.FileUrl, nil, "").Code)
	require.Equal(t, http.StatusNotFound, do(r, http.MethodGet, "/uploads/../private/"+att.FileUrl, nil, "").Code)

	// ── list: có download_path, không có file_url
	w = do(r, http.MethodGet, fmt.Sprintf("/api/crm/customer/%d/attachments", cid), nil, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list struct {
		Items []map[string]any `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	require.Equal(t, data["download_path"], list.Items[0]["download_path"])
	_, hasFileURL = list.Items[0]["file_url"]
	require.False(t, hasFileURL)

	// ── download có quyền ⇒ 200 đúng nội dung
	w = do(r, http.MethodGet, fmt.Sprintf("/api/crm/attachment/%d/download", attID), nil, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, testPDF, w.Body.String())
	require.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Contains(t, w.Header().Get("Content-Disposition"), "inline")
	require.Contains(t, w.Header().Get("Content-Disposition"), ".pdf")
	require.Contains(t, w.Header().Get("Cache-Control"), "no-store")

	// ── ngoài phạm vi telesale ⇒ 404 ở mọi đường, file còn nguyên
	rt := e.engine(tele, e.h)
	require.Equal(t, http.StatusNotFound, do(rt, http.MethodGet, fmt.Sprintf("/api/crm/attachment/%d/download", attID), nil, "").Code)
	require.Equal(t, http.StatusNotFound, do(rt, http.MethodGet, fmt.Sprintf("/api/crm/customer/%d/attachments", cid), nil, "").Code)
	require.Equal(t, http.StatusNotFound, do(rt, http.MethodDelete, fmt.Sprintf("/api/crm/attachment/%d", attID), nil, "").Code)
	b2, ct2 := multipartBody(t, "x.pdf", []byte(testPDF))
	require.Equal(t, http.StatusNotFound, do(rt, http.MethodPost, fmt.Sprintf("/api/crm/customer/%d/attachments", cid), b2, ct2).Code)
	_, err = os.Stat(privPath)
	require.NoError(t, err)
	var n int64
	testDB.Table("attachment").Where("customer_id = ?", cid).Count(&n)
	require.EqualValues(t, 1, n, "telesale ngoài phạm vi không được tạo/xoá bản ghi")

	// Đối chứng: khách do chính telesale phụ trách thì mở được — chứng minh 404 ở trên là do scope.
	cidTele := seedCustomer(t, tele)
	b3, ct3 := multipartBody(t, "y.pdf", []byte(testPDF))
	require.Equal(t, http.StatusOK, do(rt, http.MethodPost, fmt.Sprintf("/api/crm/customer/%d/attachments", cidTele), b3, ct3).Code)

	// ── id rác / không tồn tại ⇒ 404
	for _, p := range []string{"/api/crm/attachment/abc/download", fmt.Sprintf("/api/crm/attachment/%dabc/download", attID), "/api/crm/attachment/999999999/download"} {
		require.Equal(t, http.StatusNotFound, do(r, http.MethodGet, p, nil, "").Code, p)
	}

	// ── xoá ⇒ file mất, bản ghi mất
	w = do(r, http.MethodDelete, fmt.Sprintf("/api/crm/attachment/%d", attID), nil, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	_, err = os.Stat(privPath)
	require.True(t, os.IsNotExist(err), "file phải bị xoá khỏi kho private")
	testDB.Table("attachment").Where("id = ?", attID).Count(&n)
	require.Zero(t, n)
	require.Equal(t, http.StatusNotFound, do(r, http.MethodGet, fmt.Sprintf("/api/crm/attachment/%d/download", attID), nil, "").Code)
}

// Bản ghi cũ (file_url là URL tuyệt đối public) ⇒ download redirect 302 tới URL cũ.
func TestAttachment_LegacyAbsoluteURLRedirects(t *testing.T) {
	e := newAttEnv(t)
	owner := testhelpers.CreateUser(t, testDB, "own", nil)
	cid := seedCustomer(t, owner)
	legacy := customerEntity.Attachment{RefType: "customer", RefId: int(cid), CustomerID: cid,
		FileName: "cu.pdf", FileUrl: "https://cdn.old.example.com/crm/attachments/cu.pdf", FileType: "application/pdf"}
	require.NoError(t, testDB.Create(&legacy).Error)

	w := do(e.engine(owner, e.h), http.MethodGet, fmt.Sprintf("/api/crm/attachment/%d/download", legacy.ID), nil, "")
	require.Equal(t, http.StatusFound, w.Code)
	require.Equal(t, legacy.FileUrl, w.Header().Get("Location"))
}

// failingDeleteStore — kho mà Delete luôn lỗi (mô phỏng S3 sập).
type failingDeleteStore struct{ storage.Store }

func (failingDeleteStore) Delete(context.Context, string) error { return errors.New("s3 down") }

// Xoá file trong kho lỗi ⇒ 500 và bản ghi KHÔNG bị xoá (rollback) — không để file mồ côi.
func TestAttachment_DeleteStorageErrorKeepsRecord(t *testing.T) {
	e := newAttEnv(t)
	owner := testhelpers.CreateUser(t, testDB, "own", nil)
	cid := seedCustomer(t, owner)
	r := e.engine(owner, e.h)
	body, ct := multipartBody(t, "a.pdf", []byte(testPDF))
	w := do(r, http.MethodPost, fmt.Sprintf("/api/crm/customer/%d/attachments", cid), body, ct)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	attID := uint(decodeData(t, w)["id"].(float64))

	broken := New(Deps{DB: testDB, PrivateStore: failingDeleteStore{e.priv}, PublicStore: e.pub})
	w = do(e.engine(owner, broken), http.MethodDelete, fmt.Sprintf("/api/crm/attachment/%d", attID), nil, "")
	require.Equal(t, http.StatusInternalServerError, w.Code)
	var n int64
	testDB.Table("attachment").Where("id = ?", attID).Count(&n)
	require.EqualValues(t, 1, n, "xoá file lỗi ⇒ rollback, bản ghi phải còn")
}
