package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTicketListContext(t *testing.T, query string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ticket/self"+query, nil)
	return c, w
}

func TestParseTicketListFilters(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		wantStatusIn []int
		wantType     string
		wantTypeIn   []string
		wantOk       bool
	}{
		{name: "no filters", query: "", wantOk: true},
		{name: "single status", query: "?status=1", wantStatusIn: []int{1}, wantOk: true},
		{name: "multi status", query: "?status=1,2", wantStatusIn: []int{1, 2}, wantOk: true},
		{name: "junk status dropped", query: "?status=abc,2,-1", wantStatusIn: []int{2}, wantOk: true},
		{name: "single type stays single", query: "?type=general", wantType: "general", wantOk: true},
		{name: "multi type uses TypeIn", query: "?type=general,refund", wantTypeIn: []string{"general", "refund"}, wantOk: true},
		{name: "invalid type rejected", query: "?type=general,bogus", wantOk: false},
		{name: "combined", query: "?status=2,3&type=invoice", wantStatusIn: []int{2, 3}, wantType: "invoice", wantOk: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := newTicketListContext(t, tt.query)
			statusIn, ticketType, typeIn, ok := parseTicketListFilters(c)
			require.Equal(t, tt.wantOk, ok)
			if !tt.wantOk {
				// 非法 type 必须已写出错误响应（保留原单值接口的 400 语义）
				assert.NotEmpty(t, w.Body.String())
				return
			}
			assert.Equal(t, tt.wantStatusIn, statusIn)
			assert.Equal(t, tt.wantType, ticketType)
			assert.Equal(t, tt.wantTypeIn, typeIn)
		})
	}
}
