//go:build integration

package handler

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"erpCluvix/internal/modules/webapp/fbgroup/service"
	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var testDB *gorm.DB

func TestMain(m *testing.M) {
	// SchemaFull: cần seed 3 chủ đề (migration 20261027200500).
	s, err := testhelpers.New(testhelpers.MigrationsDir(), testhelpers.SchemaFull)
	if err != nil {
		log.Printf("testhelpers: %v — skipping integration tests", err)
		os.Exit(m.Run())
	}
	defer s.Close()
	testDB = s.DB
	os.Exit(m.Run())
}

func need(t *testing.T) {
	t.Helper()
	if testDB == nil {
		t.Skip("TEST_MYSQL_DSN chưa đặt")
	}
}

func reset(t *testing.T) {
	t.Helper()
	for _, q := range []string{"DELETE FROM fb_group_topic_map", "DELETE FROM fb_group", "DELETE FROM fb_group_ingest_key"} {
		if err := testDB.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func topicID(t *testing.T, name string) uint {
	t.Helper()
	var id uint
	if err := testDB.Table("fb_group_topic").Where("name = ?", name).Pluck("id", &id).Error; err != nil || id == 0 {
		t.Fatalf("chủ đề seed %q không có (err=%v)", name, err)
	}
	return id
}

type mapRow struct {
	TopicID    uint
	BranchName *string
	Source     int
}

func topicsOf(t *testing.T, fid string) map[uint]mapRow {
	t.Helper()
	rows := make([]mapRow, 0)
	if err := testDB.Table("fb_group_topic_map m").
		Select("m.topic_id, b.name AS branch_name, m.source").
		Joins("JOIN fb_group g ON g.id = m.group_id").
		Joins("LEFT JOIN fb_group_topic_branch b ON b.id = m.branch_id").
		Where("g.facebook_id = ?", fid).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	out := map[uint]mapRow{}
	for _, r := range rows {
		out[r.TopicID] = r
	}
	return out
}

func in(fid, name string, members int64) service.GroupInput {
	return service.GroupInput{FacebookID: fid, Name: name, URL: "https://www.facebook.com/groups/" + fid + "/", MemberCount: &members}
}

func TestUpsertClassifyAndProvince(t *testing.T) {
	need(t)
	reset(t)
	meBau, viCon, daDay := topicID(t, "Mẹ bầu"), topicID(t, "Vì con"), topicID(t, "Dạ dày")
	first := int64(1700000000)
	g1 := in("1", "HỎI ĐÁP MẸ BẦU HÀ NỘI", 600394)
	g1.FirstSeenAt = &first
	res, err := service.UpsertGroups(testDB, []service.GroupInput{
		g1,
		in("2", "Hội Mẹ Bỉm Sữa Nuôi Con", 605996),
		in("3", "CHIA SẺ KINH NGHIỆM CHỮA BỆNH DẠ DÀY , TRÀO NGƯỢC DẠ DÀY", 211390),
		in("4", "Hội Mê Dạ Tweed - Thanh Lý", 306844),
		{FacebookID: "", Name: "thiếu mã", URL: "https://www.facebook.com/groups/x/"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Received != 4 || res.Created != 4 || res.Updated != 0 {
		t.Fatalf("kết quả ghi sai: %+v", res)
	}
	if m := topicsOf(t, "1"); len(m) != 1 || m[meBau].BranchName == nil || *m[meBau].BranchName != "mang thai" {
		t.Errorf("group 1 phải thuộc Mẹ bầu / mang thai, được %+v", m)
	}
	if m := topicsOf(t, "2"); len(m) != 2 || m[meBau].Source != define.FbGroupTopicSourceAuto {
		t.Errorf("group 2 phải thuộc Mẹ bầu + Vì con, được %+v", m)
	} else if _, ok := m[viCon]; !ok {
		t.Errorf("group 2 thiếu Vì con: %+v", m)
	}
	if m := topicsOf(t, "3"); len(m) != 1 || m[daDay].BranchName == nil || *m[daDay].BranchName != "trào ngược" {
		t.Errorf("group 3 phải thuộc Dạ dày / trào ngược, được %+v", m)
	}
	if m := topicsOf(t, "4"); len(m) != 0 {
		t.Errorf("group 4 (áo dạ) không được thuộc chủ đề nào, được %+v", m)
	}
	var prov struct{ Province *string }
	testDB.Table("fb_group").Select("province").Where("facebook_id = ?", "1").Scan(&prov)
	if prov.Province == nil || *prov.Province != "Hà Nội" {
		t.Errorf("tỉnh group 1 phải là Hà Nội, được %v", prov.Province)
	}

	// Nhận lại: cập nhật số liệu, GIỮ first_seen_at + work_status người dùng đặt.
	testDB.Table("fb_group").Where("facebook_id = ?", "1").Update("work_status", define.FbGroupWorkTarget)
	later := int64(1800000000)
	g1b := in("1", "HỎI ĐÁP MẸ BẦU HÀ NỘI", 700000)
	g1b.FirstSeenAt = &later
	res, err = service.UpsertGroups(testDB, []service.GroupInput{g1b}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 0 || res.Updated != 1 {
		t.Fatalf("lần nhận 2 phải là cập nhật: %+v", res)
	}
	var cur struct {
		MemberCount int64
		FirstSeenAt int64
		WorkStatus  int
	}
	testDB.Table("fb_group").Select("member_count, first_seen_at, work_status").Where("facebook_id = ?", "1").Scan(&cur)
	if cur.MemberCount != 700000 || cur.FirstSeenAt != first || cur.WorkStatus != define.FbGroupWorkTarget {
		t.Errorf("ghi đè sai: %+v", cur)
	}
}

func TestManualTopicSurvivesReclassify(t *testing.T) {
	need(t)
	reset(t)
	meBau, daDay := topicID(t, "Mẹ bầu"), topicID(t, "Dạ dày")
	if _, err := service.UpsertGroups(testDB, []service.GroupInput{in("10", "Hội các mẹ bầu thông thái", 1224894)}, false); err != nil {
		t.Fatal(err)
	}
	var gid uint
	testDB.Table("fb_group").Where("facebook_id = ?", "10").Pluck("id", &gid)
	// bỏ tay Mẹ bầu, gán tay Dạ dày
	testDB.Exec("UPDATE fb_group_topic_map SET source = ? WHERE group_id = ? AND topic_id = ?", define.FbGroupTopicSourceManualRemoved, gid, meBau)
	testDB.Exec("INSERT INTO fb_group_topic_map (group_id, topic_id, source, created_at) VALUES (?, ?, ?, 0)", gid, daDay, define.FbGroupTopicSourceManualAdd)
	if _, err := service.ReclassifyAll(testDB); err != nil {
		t.Fatal(err)
	}
	// Tên còn khớp "các mẹ" của Vì con ⇒ dòng tự động đó vẫn có; 2 dòng tay phải giữ nguyên.
	m := topicsOf(t, "10")
	if m[meBau].Source != define.FbGroupTopicSourceManualRemoved || m[daDay].Source != define.FbGroupTopicSourceManualAdd {
		t.Errorf("phân loại lại không được đè dòng tay, được %+v", m)
	}
}

func TestPreview(t *testing.T) {
	need(t)
	reset(t)
	if _, err := service.UpsertGroups(testDB, []service.GroupInput{
		in("20", "Đau Dạ Dày - Trào Ngược", 98774),
		in("21", "HỘI THANH LÝ ĐẦM DẠ HỘI đau dạ dày", 71088),
		in("22", "Mẹ và Bé", 540254),
	}, false); err != nil {
		t.Fatal(err)
	}
	res, err := service.Preview(testDB, []string{"dạ dày"}, []string{"dạ hội"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Matched != 1 || res.Excluded != 1 || len(res.Samples) != 1 {
		t.Errorf("xem trước sai: %+v", res)
	}
}

func ingestRouter() *gin.Engine {
	r := testhelpers.NewGin()
	h := New(Deps{DB: testDB})
	r.POST("/api/fbgroup/ingest", h.RequireIngestKey(), h.Ingest)
	return r
}

func post(r *gin.Engine, key string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/fbgroup/ingest", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestIngestEndpoint(t *testing.T) {
	need(t)
	reset(t)
	key := define.FbGroupIngestKeyPrefix + "khoa-thu-nghiem-1234"
	if err := testDB.Exec("INSERT INTO fb_group_ingest_key (key_hash, key_hint, status, created_at, updated_at) VALUES (?, '1234', 1, 0, 0)",
		hashKey(key)).Error; err != nil {
		t.Fatal(err)
	}
	r := ingestRouter()
	payload := map[string]any{
		"nguon": "nextcore-browser", "phienBan": 1, "kiem": false, "soGroup": 2,
		"groups": []map[string]any{
			{"id": "1481410332813761", "ten": "Homestay Đà Lạt Giá Rẻ", "url": "https://www.facebook.com/groups/homestaydalatgiare/",
				"thanhVien": 48307, "baiNgay": 90, "cheDo": "PUBLIC", "lanDau": "2026-10-17T06:00:00.000Z", "lanCuoi": "2026-10-19T01:12:00.000Z"},
			{"id": "x", "ten": "link lạ", "url": "https://evil.example.com/groups/x"},
		},
	}
	if w := post(r, "", payload); w.Code != http.StatusUnauthorized {
		t.Errorf("thiếu khoá phải 401, được %d", w.Code)
	}
	if w := post(r, define.FbGroupIngestKeyPrefix+"sai", payload); w.Code != http.StatusUnauthorized {
		t.Errorf("sai khoá phải 401, được %d", w.Code)
	}
	if w := post(r, key, map[string]any{"phienBan": 1, "kiem": true, "groups": []any{}}); w.Code != http.StatusOK {
		t.Errorf("kiem phải 200, được %d %s", w.Code, w.Body.String())
	}
	if w := post(r, key, map[string]any{"phienBan": 2, "groups": []any{}}); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("phienBan lạ phải 422, được %d", w.Code)
	}
	w := post(r, key, payload)
	if w.Code != http.StatusOK {
		t.Fatalf("ingest phải 200, được %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Data service.UpsertResult `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Data.Received != 1 || body.Data.Skipped != 1 {
		t.Errorf("phải nhận 1, bỏ 1 (link không phải facebook.com): %+v", body.Data)
	}
	var g struct {
		PostsPerDay int
		Privacy     int
		Province    string
		FirstSeenAt int64
	}
	testDB.Table("fb_group").Select("posts_per_day, privacy, province, first_seen_at").Where("facebook_id = ?", "1481410332813761").Scan(&g)
	if g.PostsPerDay != 90 || g.Privacy != define.FbGroupPrivacyPublic || g.Province != "Lâm Đồng" || g.FirstSeenAt != 1792216800 {
		t.Errorf("group nhận sai: %+v", g)
	}
	var used struct {
		TotalReceived  int64
		LastBatchCount int
	}
	testDB.Table("fb_group_ingest_key").Select("total_received, last_batch_count").Scan(&used)
	if used.TotalReceived != 1 || used.LastBatchCount != 1 {
		t.Errorf("thống kê khoá sai: %+v", used)
	}
}

func TestReadGroupCSV(t *testing.T) {
	bom := string([]byte{0xEF, 0xBB, 0xBF}) // tệp Xuất CSV của GROUPCRAWL là UTF-8 có BOM
	csv := bom + "group_id,ten,url,thanh_vien,thanh_vien_lam_tron,bai_ngay,che_do,khu_vuc,danh_muc,bai_customerpost,bai_customerpost_7_ngay,tu_khoa,profile,lan_dau,lan_cuoi\n" +
		"939853694105892,MẸ THÔNG THÁI CHỌN SỮA CÔNG THỨC,https://www.facebook.com/groups/939853694105892/,54000,co,8,Công khai,,Sữa & dinh dưỡng trẻ em,0,0,mẹ và bé,M 01,2026-10-27T08:02:55.298Z,2026-10-27T08:02:55.298Z\n" +
		",thiếu mã,https://www.facebook.com/groups/1/,1,khong,,,,,,,,,,\n"
	rows, skipped, err := readGroupCSV(bytes.NewReader([]byte(csv)))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || skipped != 1 {
		t.Fatalf("phải đọc 1 dòng, bỏ 1: %d / %d", len(rows), skipped)
	}
	r := rows[0]
	if !r.MemberRounded || *r.PostsPerDay != 8 || *r.Privacy != define.FbGroupPrivacyPublic ||
		*r.FoundKeyword != "mẹ và bé" || *r.FoundByProfile != "M 01" || *r.NcbQuality != define.FbGroupNcbQualityKept {
		t.Errorf("đọc CSV sai: %+v", r)
	}
	if _, _, err := readGroupCSV(bytes.NewReader([]byte("ten,url\nx,y\n"))); err == nil {
		t.Error("thiếu cột group_id phải báo lỗi")
	}
}
