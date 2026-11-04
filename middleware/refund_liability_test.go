package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestRefundLiabilityAdmissionFailsClosed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "guard.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TicketRefund{}, &model.RefundLiability{}))
	prior := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = prior
		connection, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, connection.Close())
	})
	owner := model.User{Username: "liability-owner", AffCode: "liability-owner"}
	require.NoError(t, db.Create(&owner).Error)
	for _, tc := range []struct {
		name          string
		failed, panic bool
		want          int64
	}{
		{"settled", false, false, 0}, {"settlement_failed", true, false, 1}, {"panic", false, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Where("1 = 1").Delete(&model.RefundLiability{}).Error)
			router := gin.New()
			router.Use(gin.Recovery(), func(c *gin.Context) { c.Set("id", owner.Id) }, RefundLiability())
			router.POST("/relay", func(c *gin.Context) {
				value, ok := c.Get(common.RefundLiabilityContextKey)
				require.True(t, ok)
				liability := value.(*common.RefundLiability)
				if tc.panic {
					panic("request interrupted")
				}
				liability.Failed.Store(tc.failed)
				c.Status(http.StatusOK)
			})
			router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/relay", nil))
			var count int64
			require.NoError(t, db.Model(&model.RefundLiability{}).Count(&count).Error)
			assert.Equal(t, tc.want, count)
		})
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", owner.Id) }, RefundLiability())
	called := false
	router.POST("/relay", func(c *gin.Context) { called = true })
	require.NoError(t, db.Create(&model.TicketRefund{UserId: owner.Id, TicketId: 1, RefundStatus: model.RefundStatusPending}).Error)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/relay", nil))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.False(t, called)
	require.NoError(t, db.Where("1 = 1").Delete(&model.TicketRefund{}).Error)
	require.NoError(t, db.Migrator().DropTable(&model.RefundLiability{}))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/relay", nil))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.False(t, called)
}
