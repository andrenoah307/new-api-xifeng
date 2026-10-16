package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/attachment"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsPreviewableMime(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"image/png", true},
		{"image/jpeg; charset=binary", true},
		{"image/svg+xml", false}, // 硬性禁 svg 预览
		{"application/json", false},
		{"application/xml", false},
		{"text/plain", false},
		{"text/csv", false},
		{"text/html", false},
		{"image/unknown", false},
		{"application/pdf", false}, // pdf 下载而非 inline
		{"", false},
		{"application/octet-stream", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, isPreviewableMime(c.in), c.in)
	}
}

func TestDownloadTicketAttachmentContentSafety(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.TicketAttachment{}))
	oldStorage, oldPath := setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath
	setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath = "local", t.TempDir()
	t.Cleanup(func() {
		setting.TicketAttachmentStorage, setting.TicketAttachmentLocalPath = oldStorage, oldPath
	})
	storage, err := attachment.Current()
	require.NoError(t, err)

	for _, tc := range []struct {
		name, body, storedMime, query, wantMime, disposition string
	}{
		{"legacy HTML", "<!DOCTYPE html><html>untrusted</html>", "text/html", "?inline=1", "application/octet-stream", "attachment;"},
		{"HTML mislabeled as PNG", "<!DOCTYPE html><html>untrusted</html>", "image/png", "?inline=1", "application/octet-stream", "attachment;"},
		{"XML", "<?xml version=\"1.0\"?><root/>", "application/xml", "?inline=1", "application/octet-stream", "attachment;"},
		{"SVG", "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", "image/svg+xml", "?inline=1", "application/octet-stream", "attachment;"},
		{"PDF", "%PDF-1.7\n", "application/pdf", "?inline=1", "application/octet-stream", "attachment;"},
		{"PNG with stale MIME", "\x89PNG\r\n\x1a\n", "text/html", "?inline=1", "image/png", "inline;"},
		{"PNG download", "\x89PNG\r\n\x1a\n", "image/png", "", "image/png", "attachment;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := strings.ReplaceAll(tc.name, " ", "_") + ".txt"
			require.NoError(t, storage.Put(context.Background(), key, strings.NewReader(tc.body), int64(len(tc.body)), tc.storedMime))
			record := model.TicketAttachment{UserId: 1, FileName: "report.txt", StorageType: "local", StorageKey: key, MimeType: tc.storedMime, Size: int64(len(tc.body))}
			require.NoError(t, db.Create(&record).Error)
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Set("id", 1)
			c.Set("role", common.RoleCommonUser)
			c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(record.Id)}}
			c.Request = httptest.NewRequest(http.MethodGet, "/api/ticket/attachment/"+strconv.Itoa(record.Id)+tc.query, nil)
			DownloadTicketAttachment(c)
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, tc.wantMime, response.Header().Get("Content-Type"))
			assert.True(t, strings.HasPrefix(response.Header().Get("Content-Disposition"), tc.disposition))
			assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
			assert.Contains(t, response.Header().Get("Content-Security-Policy"), "sandbox")
			assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
			assert.Equal(t, tc.body, response.Body.String())
		})
	}
}

func TestDownloadTicketAttachmentCloudContentSafety(t *testing.T) {
	db := setupUserRedemptionControllerDB(t)
	require.NoError(t, db.AutoMigrate(&model.TicketAttachment{}))
	const body = "<!DOCTYPE html><html>legacy cloud attachment</html>"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/attachments/legacy.txt", r.URL.Path)
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	oldStorage := setting.TicketAttachmentStorage
	oldEndpoint, oldBucket, oldRegion := setting.TicketAttachmentS3Endpoint, setting.TicketAttachmentS3Bucket, setting.TicketAttachmentS3Region
	oldID, oldSecret := setting.TicketAttachmentS3AccessKeyId, setting.TicketAttachmentS3AccessKeySecret
	setting.TicketAttachmentStorage = "s3"
	setting.TicketAttachmentS3Endpoint, setting.TicketAttachmentS3Bucket, setting.TicketAttachmentS3Region = server.URL, "attachments", "us-east-1"
	setting.TicketAttachmentS3AccessKeyId, setting.TicketAttachmentS3AccessKeySecret = "test-id", "test-secret"
	t.Cleanup(func() {
		setting.TicketAttachmentStorage = oldStorage
		setting.TicketAttachmentS3Endpoint, setting.TicketAttachmentS3Bucket, setting.TicketAttachmentS3Region = oldEndpoint, oldBucket, oldRegion
		setting.TicketAttachmentS3AccessKeyId, setting.TicketAttachmentS3AccessKeySecret = oldID, oldSecret
	})
	record := model.TicketAttachment{UserId: 1, FileName: "legacy.txt", StorageType: "s3", StorageKey: "legacy.txt", MimeType: "image/png", Size: int64(len(body))}
	require.NoError(t, db.Create(&record).Error)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Set("id", 1)
	c.Set("role", common.RoleCommonUser)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(record.Id)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ticket/attachment/"+strconv.Itoa(record.Id)+"?inline=1", nil)
	DownloadTicketAttachment(c)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Empty(t, response.Header().Get("Location"))
	assert.Equal(t, "application/octet-stream", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Header().Get("Content-Disposition"), "attachment;")
	assert.Contains(t, response.Header().Get("Content-Security-Policy"), "sandbox")
	assert.Equal(t, body, response.Body.String())
}
