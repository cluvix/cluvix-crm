//go:build integration

package handler

// Story-04 (customer-stage-pipeline) AC5 — GET /crm/customer/:id/timeline-customer có sự kiện chuyển
// bước (`type = stage_change`) đọc từ `customer_stage_history`: from/to stage id, người đổi (LEFT JOIN
// user_profile — changed_by NULL vẫn hiện), thời điểm; xếp đúng thứ tự thời gian cùng sự kiện khác.
// Khách ngoài phạm vi chi nhánh ⇒ vẫn bị chặn như trước, không lộ lịch sử.

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"erpCluvix/internal/testhelpers"
	"erpCluvix/pkg/define"
)

func tlStageID(t *testing.T, name string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testDB.Raw("SELECT id FROM customer_stage WHERE name = ?", name).Scan(&id).Error)
	require.NotZero(t, id, "thiếu bước seed %q (migration 20261102095255)", name)
	return id
}

func tlInsertHistory(t *testing.T, customerID uint, from *int64, to int64, source int, by *uint, at int64) {
	t.Helper()
	require.NoError(t, testDB.Exec(`INSERT INTO customer_stage_history
		(customer_id, from_stage_id, to_stage_id, source, changed_by, changed_at) VALUES (?, ?, ?, ?, ?, ?)`,
		customerID, from, to, source, by, at).Error)
}

func callTimeline(t *testing.T, customerID uint, branchActive uint) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	id := strconv.FormatUint(uint64(customerID), 10)
	c.Request = httptest.NewRequest("GET", "/crm/customer/"+id+"/timeline-customer", nil)
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Set("branchActiveId", branchActive)
	c.Set("branchIds", []uint{branchActive})
	c.Set("userID", uint(1))
	(&Handler{DB: testDB}).TimelineCustomer(c)
	return w
}

type tlEvent struct {
	Title       string `json:"title"`
	Day         int64  `json:"day"`
	Type        string `json:"type"`
	StageChange *struct {
		FromStageID   *int64 `json:"from_stage_id"`
		ToStageID     int64  `json:"to_stage_id"`
		ChangedBy     *int64 `json:"changed_by"`
		ChangedByName string `json:"changed_by_name"`
		ChangedAt     int64  `json:"changed_at"`
		Source        int    `json:"source"`
	} `json:"stage_change"`
}

func TestTimelineCustomer_StageChangeEvents(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", tn.CustomerID) })
	user := testhelpers.CreateScopedUser(t, testDB, "s04tl", tn, tn.BranchID)
	// Fixture FreshTenant đã ghi 1 dòng lịch sử khởi tạo; test tự dựng chuỗi lịch sử nên xoá dòng đó trước.
	require.NoError(t, testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", tn.CustomerID).Error)
	contact, lost, won := tlStageID(t, "Liên hệ"), tlStageID(t, "Từ chối"), tlStageID(t, "Khách hàng đã thuê")

	base := time.Now().Unix() - 3600
	require.NoError(t, testDB.Exec("UPDATE customer SET branch_id = ?, stage_id = ?, created_day = ?, created_at = ? WHERE id = ?",
		tn.BranchID, lost, base, base, tn.CustomerID).Error)
	// 2 lần đổi bước, chèn ngược thứ tự thời gian (thứ tự id ≠ thứ tự thời gian) + 1 nhắc hẹn nằm giữa.
	tlInsertHistory(t, tn.CustomerID, &contact, lost, define.CustomerStageHistorySourceManual, &user, base+300)
	tlInsertHistory(t, tn.CustomerID, nil, contact, define.CustomerStageHistorySourceCreate, nil, base+100)
	// Cùng giây với lần đổi trước (đổi bước 2 lần trong 1 giây) ⇒ thứ tự theo id (thứ tự ghi).
	tlInsertHistory(t, tn.CustomerID, &lost, won, define.CustomerStageHistorySourceManual, &user, base+300)
	remCode := fmt.Sprintf("NH%d", time.Now().UnixNano())
	require.NoError(t, testDB.Exec(`INSERT INTO reminder (code, customer_id, type, start_time, note, updated_at)
		VALUES (?, ?, 1, ?, 'S04 nhắc', 0)`, remCode, tn.CustomerID, base+200).Error)
	t.Cleanup(func() { testDB.Exec("DELETE FROM reminder WHERE code = ?", remCode) })

	w := callTimeline(t, tn.CustomerID, tn.BranchID)
	require.Equal(t, 200, w.Code, w.Body.String())
	var body struct {
		Data struct {
			Timeline []tlEvent `json:"timeline"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	tl := body.Data.Timeline

	var kinds []string
	for i, e := range tl {
		if i > 0 {
			require.LessOrEqual(t, tl[i-1].Day, e.Day, "timeline phải tăng dần theo day: %+v", tl)
		}
		switch {
		case e.Type == define.CustomerTimelineEventStageChange:
			kinds = append(kinds, "stage")
		case e.Title == "Nhắc hẹn":
			kinds = append(kinds, "reminder")
		case e.Title == "Khởi tạo khách hàng":
			kinds = append(kinds, "created")
		}
	}
	require.Equal(t, []string{"created", "stage", "reminder", "stage", "stage"}, kinds, "thứ tự sự kiện theo thời gian")

	var stages []tlEvent
	for _, e := range tl {
		if e.Type == define.CustomerTimelineEventStageChange {
			stages = append(stages, e)
		}
	}
	require.Len(t, stages, 3)

	first := stages[0].StageChange
	require.NotNil(t, first)
	require.Nil(t, first.FromStageID, "lần gán đầu: from_stage_id = null")
	require.Equal(t, contact, first.ToStageID)
	require.Nil(t, first.ChangedBy, "changed_by NULL (hệ thống) vẫn phải hiện — LEFT JOIN")
	require.Equal(t, "", first.ChangedByName)
	require.Equal(t, base+100, first.ChangedAt)
	require.Equal(t, base+100, stages[0].Day)
	require.Equal(t, define.CustomerStageHistorySourceCreate, first.Source)

	second := stages[1].StageChange
	require.NotNil(t, second)
	require.NotNil(t, second.FromStageID)
	require.Equal(t, contact, *second.FromStageID)
	require.Equal(t, lost, second.ToStageID)
	require.NotNil(t, second.ChangedBy)
	require.EqualValues(t, user, *second.ChangedBy)
	require.Equal(t, "NV "+fmt.Sprint(user), second.ChangedByName)
	require.Equal(t, base+300, second.ChangedAt)
	require.Equal(t, define.CustomerStageHistorySourceManual, second.Source)

	third := stages[2].StageChange // cùng changed_at với `second`, ghi sau ⇒ đứng sau
	require.NotNil(t, third)
	require.Equal(t, lost, *third.FromStageID)
	require.Equal(t, won, third.ToStageID)

	// Sự kiện cũ KHÔNG mang khối stage_change (omitempty) — shape timeline cũ không đổi.
	require.Equal(t, 3, strings.Count(w.Body.String(), `"stage_change":`))
}

func TestTimelineCustomer_StageHistoryOutOfScopeBlocked(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", tn.CustomerID) })
	branchB := testhelpers.CreateBranch(t, testDB)
	contact := tlStageID(t, "Liên hệ")
	require.NoError(t, testDB.Exec("UPDATE customer SET branch_id = ?, stage_id = ? WHERE id = ?",
		branchB, contact, tn.CustomerID).Error)
	tlInsertHistory(t, tn.CustomerID, nil, contact, define.CustomerStageHistorySourceCreate, nil, time.Now().Unix())

	// Đối chứng: đúng chi nhánh thì thấy lịch sử.
	ok := callTimeline(t, tn.CustomerID, branchB)
	require.Equal(t, 200, ok.Code, ok.Body.String())
	require.Contains(t, ok.Body.String(), `"stage_change":`)

	w := callTimeline(t, tn.CustomerID, tn.BranchID) // nhân viên chi nhánh A, khách thuộc B
	require.NotEqual(t, 200, w.Code, "khách ngoài phạm vi phải bị chặn: %s", w.Body.String())
	require.NotContains(t, w.Body.String(), "stage_change")
	require.NotContains(t, w.Body.String(), `"timeline"`)
}

// QA pass 1 (P1): `sort.Slice` (pdqsort) chỉ ổn định khi n ≤ 12 — timeline thật dễ vượt. Ca này 15 sự
// kiện đúng khuôn handler: "Khởi tạo khách hàng" + 11 nhắc hẹn (builder trả start_time DESC) + bước
// khởi tạo CÙNG giây với "Khởi tạo" (luồng tạo khách ghi cả hai cùng lúc) + 2 lần đổi bước cùng giây.
// Với ĐÚNG bố cục này `sort.Slice` đẩy bước khởi tạo lên trước "Khởi tạo khách hàng" (mô phỏng: n=15
// đảo, n=14/16 thì không — đổi số nhắc hẹn là test mất răng) ⇒ khoá `sort.SliceStable`.
func TestTimelineCustomer_SameSecondOrderManyEvents(t *testing.T) {
	if testDB == nil {
		t.Fatal("TEST_MYSQL_DSN chưa set — integration test này KHÔNG được skip")
	}
	tn := testhelpers.FreshTenant(t, testDB)
	t.Cleanup(func() { testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", tn.CustomerID) })
	contact, won := tlStageID(t, "Liên hệ"), tlStageID(t, "Khách hàng đã thuê")
	// Fixture FreshTenant đã ghi 1 dòng lịch sử khởi tạo; test tự dựng chuỗi lịch sử nên xoá dòng đó trước.
	require.NoError(t, testDB.Exec("DELETE FROM customer_stage_history WHERE customer_id = ?", tn.CustomerID).Error)

	base := time.Now().Unix() - 7200
	require.NoError(t, testDB.Exec("UPDATE customer SET branch_id = ?, stage_id = ?, created_day = ?, created_at = ? WHERE id = ?",
		tn.BranchID, contact, base, base, tn.CustomerID).Error)
	tlInsertHistory(t, tn.CustomerID, nil, contact, define.CustomerStageHistorySourceCreate, nil, base) // cùng giây với "Khởi tạo"
	for r := 1; r <= 11; r++ {
		code := fmt.Sprintf("NH%d_%02d", time.Now().UnixNano(), r)
		require.NoError(t, testDB.Exec(`INSERT INTO reminder (code, customer_id, type, start_time, note, updated_at)
			VALUES (?, ?, 1, ?, 'S04 nhắc', 0)`, code, tn.CustomerID, base+int64(r)*7).Error)
	}
	t.Cleanup(func() { testDB.Exec("DELETE FROM reminder WHERE customer_id = ?", tn.CustomerID) })
	f1, f2 := contact, won
	tlInsertHistory(t, tn.CustomerID, &f1, won, define.CustomerStageHistorySourceManual, nil, base+500) // 2 lần cùng giây
	tlInsertHistory(t, tn.CustomerID, &f2, contact, define.CustomerStageHistorySourceManual, nil, base+500)

	w := callTimeline(t, tn.CustomerID, tn.BranchID)
	require.Equal(t, 200, w.Code, w.Body.String())
	var body struct {
		Data struct {
			Timeline []tlEvent `json:"timeline"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	tl := body.Data.Timeline
	require.Len(t, tl, 15, "1 khởi tạo + 11 nhắc hẹn + 3 lần chuyển bước — số này quyết định sort.Slice có đảo hay không")

	require.Equal(t, "Khởi tạo khách hàng", tl[0].Title, "⛔ sự kiện cùng giây bị đảo (sort không ổn định?)")
	require.Equal(t, define.CustomerTimelineEventStageChange, tl[1].Type)
	require.Nil(t, tl[1].StageChange.FromStageID, "bước khởi tạo phải đứng NGAY sau 'Khởi tạo khách hàng'")
	last2 := tl[len(tl)-2:]
	require.Equal(t, won, last2[0].StageChange.ToStageID, "2 lần đổi cùng giây phải theo thứ tự ghi (id)")
	require.Equal(t, contact, last2[1].StageChange.ToStageID)
}
