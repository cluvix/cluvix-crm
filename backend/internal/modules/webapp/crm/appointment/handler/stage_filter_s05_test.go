package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Story-05 AC2: khoá `_stage_id` quyết định JOIN customer (customerFilterColumns); khoá cũ `_status`
// không còn kéo theo JOIN (không còn ApplyFilter nào đọc nó ⇒ join thừa là chi phí thuần).
func TestNeedsCustomerJoin_StageID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mk := func(qs string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/crm/appointment?"+qs, nil)
		return c
	}
	for _, tc := range []struct {
		qs   string
		want bool
	}{
		{"", false},
		{"filter[_stage_id]=3", true},
		{"filter[_stage_id][0]=3&filter[_stage_id][1]=4", true},
		{"filter[_status]=1", false},
		{"filter[status]=1", false},
		{"filter[_gender]=1", true},
	} {
		if got := needsCustomerJoin(mk(tc.qs)); got != tc.want {
			t.Errorf("needsCustomerJoin(%q) = %v, muốn %v", tc.qs, got, tc.want)
		}
	}
}
