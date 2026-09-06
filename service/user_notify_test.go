package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureNotifyHTTPTest(t *testing.T) {
	t.Helper()

	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	originalWorkerURL := system_setting.WorkerUrl
	originalWorkerKey := system_setting.WorkerValidKey
	originalWorkerHTTPEnabled := system_setting.WorkerAllowHttpImageRequestEnabled
	originalHTTPClient := httpClient
	originalProtectedHTTPClient := ssrfProtectedHTTPClient
	t.Cleanup(func() {
		*fetchSetting = originalFetchSetting
		system_setting.WorkerUrl = originalWorkerURL
		system_setting.WorkerValidKey = originalWorkerKey
		system_setting.WorkerAllowHttpImageRequestEnabled = originalWorkerHTTPEnabled
		httpClient = originalHTTPClient
		ssrfProtectedHTTPClient = originalProtectedHTTPClient
	})

	fetchSetting.EnableSSRFProtection = false
	system_setting.WorkerUrl = ""
	httpClient = &http.Client{}
	ssrfProtectedHTTPClient = &http.Client{}
}

func TestRenderNotifyContent(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		values     []interface{}
		want       string
		linkCount  int
		noTemplate bool
	}{
		{
			name:       "quota warning template",
			content:    "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>",
			values:     []interface{}{"您的额度即将用尽", "42", "https://example.test/topup", "https://example.test/topup"},
			want:       "您的额度即将用尽，当前剩余额度为 42，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='https://example.test/topup'>https://example.test/topup</a>",
			linkCount:  2,
			noTemplate: true,
		},
		{
			name:       "format-looking values stay literal",
			content:    "{{value}} | {{value}} | {{value}}",
			values:     []interface{}{"%s", "%d", "100%"},
			want:       "%s | %d | 100%",
			noTemplate: true,
		},
		{
			name:    "fewer values leave placeholders",
			content: "a {{value}} b {{value}}",
			values:  []interface{}{"x"},
			want:    "a x b {{value}}",
		},
		{
			name:    "extra values are ignored",
			content: "{{value}}",
			values:  []interface{}{"x", "y"},
			want:    "x",
		},
		{
			name:    "nil values preserve original",
			content: "unchanged {{value}}",
			want:    "unchanged {{value}}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := renderNotifyContent(tc.content, tc.values)
			if tc.want != "" {
				assert.Equal(t, tc.want, got)
			}
			if tc.noTemplate {
				assert.NotContains(t, got, dto.ContentValueParam)
				assert.NotContains(t, got, "%!")
			}
			if tc.linkCount > 0 {
				assert.Equal(t, tc.linkCount, strings.Count(got, "https://example.test/topup"))
			}
		})
	}
}

func TestDoNotifyRequest(t *testing.T) {
	configureNotifyHTTPTest(t)

	tests := []struct {
		name       string
		statusCode int
		body       string
		wantError  bool
	}{
		{name: "ok", statusCode: http.StatusOK},
		{name: "no content", statusCode: http.StatusNoContent},
		{name: "body included in error", statusCode: http.StatusInternalServerError, body: "upstream said no", wantError: true},
		{name: "multiline body is flattened", statusCode: http.StatusInternalServerError, body: "line one\nline two\nline three", wantError: true},
		{name: "error body is limited", statusCode: http.StatusInternalServerError, body: strings.Repeat("x", notifyResponseBodyLimit+1024), wantError: true},
		{name: "empty error body", statusCode: http.StatusBadRequest, wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod string
			var gotHeader string
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				gotMethod = request.Method
				gotHeader = request.Header.Get("X-Test-Header")
				writer.WriteHeader(tc.statusCode)
				_, _ = writer.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)

			err := doNotifyRequest("test", http.MethodPut, server.URL, map[string]string{"X-Test-Header": "expected"}, nil, []byte("request body"))
			assert.Equal(t, http.MethodPut, gotMethod)
			assert.Equal(t, "expected", gotHeader)
			if tc.wantError {
				require.Error(t, err)
				if tc.body != "" && tc.name != "error body is limited" {
					expectedBody := strings.TrimSpace(tc.body)
					if tc.name == "multiline body is flattened" {
						expectedBody = strings.Join(strings.Fields(expectedBody), " ")
					}
					assert.Contains(t, err.Error(), expectedBody)
				}
				assert.Contains(t, err.Error(), "test request failed with status code")
				if tc.name == "body included in error" {
					assert.Contains(t, err.Error(), "500")
				}
				if tc.name == "multiline body is flattened" {
					assert.NotContains(t, err.Error(), "\n")
				}
				if tc.name == "error body is limited" {
					assert.Less(t, len(err.Error()), 6000)
				}
				if tc.name == "empty error body" {
					assert.Contains(t, err.Error(), "empty response body")
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestSendWebhookNotify(t *testing.T) {
	configureNotifyHTTPTest(t)

	tests := []struct {
		name              string
		secret            string
		wantSignature     bool
		wantAuthorization bool
	}{
		{name: "secret is signed without authorization", secret: "webhook-secret", wantSignature: true, wantAuthorization: false},
		{name: "empty secret has no secret headers", wantSignature: false, wantAuthorization: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var payload WebhookPayload
			var gotSignature string
			var gotAuthorization string
			var handlerErr error
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					handlerErr = err
					return
				}
				handlerErr = common.Unmarshal(body, &payload)
				gotSignature = request.Header.Get("X-Webhook-Signature")
				gotAuthorization = request.Header.Get("Authorization")
				writer.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(server.Close)

			data := dto.Notify{
				Type:    dto.NotifyTypeQuotaExceed,
				Title:   "Quota warning",
				Content: "{{value}}，当前剩余额度为 {{value}}，充值链接：<a href='{{value}}'>{{value}}</a>",
				Values:  []interface{}{"您的额度即将用尽", "42", "https://example.test/topup", "https://example.test/topup"},
			}
			require.NoError(t, SendWebhookNotify(server.URL, tc.secret, data))
			require.NoError(t, handlerErr)

			assert.Equal(t, data.Type, payload.Type)
			assert.Equal(t, data.Title, payload.Title)
			assert.Equal(t, data.Values, payload.Values)
			assert.NotContains(t, payload.Content, dto.ContentValueParam)
			assert.NotContains(t, payload.Content, "%!(EXTRA")
			assert.Contains(t, payload.Content, "您的额度即将用尽")
			assert.Contains(t, payload.Content, "42")
			assert.Contains(t, payload.Content, "https://example.test/topup")
			assert.Equal(t, 2, strings.Count(payload.Content, "https://example.test/topup"))
			if tc.wantSignature {
				assert.NotEmpty(t, gotSignature)
			} else {
				assert.Empty(t, gotSignature)
			}
			if tc.wantAuthorization {
				assert.NotEmpty(t, gotAuthorization)
			} else {
				assert.Empty(t, gotAuthorization)
			}
		})
	}
}
