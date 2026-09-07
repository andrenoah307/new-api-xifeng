package common

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPageQuery(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		wantPageSize int
		wantPage     int
	}{
		{name: "explicit page size", query: "?p=1&page_size=20", wantPageSize: 20, wantPage: 1},
		{name: "negative page size", query: "?p=1&page_size=-1", wantPageSize: ItemsPerPage, wantPage: 1},
		{name: "large negative page size", query: "?p=1&page_size=-999", wantPageSize: ItemsPerPage, wantPage: 1},
		{name: "zero page size", query: "?p=1&page_size=0", wantPageSize: ItemsPerPage, wantPage: 1},
		{name: "page size above maximum", query: "?p=1&page_size=1000", wantPageSize: 100, wantPage: 1},
		{name: "maximum page size", query: "?p=1&page_size=100", wantPageSize: 100, wantPage: 1},
		{name: "defaults", query: "", wantPageSize: ItemsPerPage, wantPage: 1},
		{name: "ps compatibility path", query: "?ps=30", wantPageSize: 30, wantPage: 1},
		{name: "size compatibility path", query: "?size=30", wantPageSize: 30, wantPage: 1},
		{name: "negative page compatibility", query: "?p=-5&page_size=20", wantPageSize: 20, wantPage: -5},
	}

	gin.SetMode(gin.TestMode)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/test"+tt.query, nil)

			pageInfo := GetPageQuery(ctx)
			require.NotNil(t, pageInfo)
			assert.Equal(t, tt.wantPageSize, pageInfo.GetPageSize())
			assert.Equal(t, tt.wantPage, pageInfo.GetPage())
		})
	}
}

func TestGetPageQueryNegativePageSizeStartIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/test?p=1&page_size=-1", nil)

	pageInfo := GetPageQuery(ctx)
	require.NotNil(t, pageInfo)
	assert.Equal(t, 0, pageInfo.GetStartIdx())
	assert.GreaterOrEqual(t, pageInfo.GetStartIdx(), 0)
}
