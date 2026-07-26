package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUpstreamRequestContextPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	parentContext, cancelParent := context.WithCancel(context.Background())
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(parentContext)

	nonStreamRequest, err := newUpstreamRequest(c, &relaycommon.RelayInfo{}, http.MethodPost, "http://upstream.invalid/v1", nil)
	require.NoError(t, err)
	streamRequest, err := newUpstreamRequest(c, &relaycommon.RelayInfo{IsStream: true}, http.MethodPost, "http://upstream.invalid/v1", nil)
	require.NoError(t, err)

	cancelParent()
	assert.ErrorIs(t, nonStreamRequest.Context().Err(), context.Canceled)
	assert.NoError(t, streamRequest.Context().Err())
}

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

func TestDoRequestNonStreamBodyCloseCancelsDerivedContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bodyText := "response body remains readable"
	service.ResetProxyClientCache()
	defer service.ResetProxyClientCache()
	proxyKey := "http://test-proxy-body-close"
	client, err := service.NewProxyHttpClient(proxyKey)
	require.NoError(t, err)
	requestContext := make(chan context.Context, 1)
	bodyClosed := make(chan struct{})
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestContext <- req.Context()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{common.RequestIdKey: []string{"upstream-request"}},
			Body: &trackingReadCloser{
				Reader: strings.NewReader(bodyText),
				closed: bodyClosed,
			},
			Request: req,
		}, nil
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		IsStream: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: proxyKey},
		},
	}

	resp, err := doRequestWithTimeouts(c, req, info, 200*time.Millisecond, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, bodyText, string(data))
	assert.Equal(t, "upstream-request", c.GetString(common.UpstreamRequestIdKey))
	ctx := <-requestContext
	select {
	case <-ctx.Done():
		t.Fatal("request context was canceled before response body close")
	default:
	}

	require.NoError(t, resp.Body.Close())
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("response body close did not cancel request context")
	}
	select {
	case <-bodyClosed:
	default:
		t.Fatal("response body close did not close the underlying body")
	}
}

func TestDoRequestNonStreamDeadlineCancelsBodyRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ResetProxyClientCache()
	defer service.ResetProxyClientCache()
	proxyKey := "http://test-proxy-body-timeout"
	client, err := service.NewProxyHttpClient(proxyKey)
	require.NoError(t, err)
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       &contextBlockingReadCloser{ctx: req.Context()},
			Request:    req,
		}, nil
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		IsStream: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: proxyKey},
		},
	}

	resp, err := doRequestWithTimeouts(c, req, info, 25*time.Millisecond, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	_, err = io.ReadAll(resp.Body)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, resp.Body.Close())
}

func TestDoRequestStreamSkipsNonStreamDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	generalSetting := operation_setting.GetGeneralSetting()
	originalGeneralSetting := *generalSetting
	generalSetting.PingIntervalEnabled = true
	generalSetting.PingIntervalSeconds = 1
	t.Cleanup(func() { *generalSetting = originalGeneralSetting })
	service.ResetProxyClientCache()
	defer service.ResetProxyClientCache()
	proxyKey := "http://test-proxy-stream"
	client, err := service.NewProxyHttpClient(proxyKey)
	require.NoError(t, err)
	deadlineSeen := make(chan bool, 1)
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		_, hasDeadline := req.Context().Deadline()
		deadlineSeen <- hasDeadline
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: proxyKey},
		},
	}

	resp, err := doRequestWithTimeouts(c, req, info, 25*time.Millisecond, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
	assert.False(t, <-deadlineSeen)
}

func TestDoRequestStreamHeaderTimeoutDisarmsBeforeBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newDirectProxyTestClient(t, "http://test-proxy-stream-header-body")
	chunks := []string{"first", "second", "third"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		for _, chunk := range chunks {
			time.Sleep(20 * time.Millisecond)
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: "http://test-proxy-stream-header-body"},
		},
	}

	resp, err := doRequestWithTimeouts(c, req, info, 0, 10*time.Millisecond)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Request)
	streamContext := resp.Request.Context()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, strings.Join(chunks, ""), string(body))
	require.NoError(t, resp.Body.Close())
	assert.Error(t, streamContext.Err())
}

func TestDoRequestStreamHeaderTimeoutReturnsReadableError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newDirectProxyTestClient(t, "http://test-proxy-stream-header-timeout")
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-releaseHandler
	}))
	defer func() {
		close(releaseHandler)
		server.Close()
	}()

	parentContext, cancelParent := context.WithTimeout(context.Background(), time.Second)
	defer cancelParent()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequestWithContext(parentContext, http.MethodPost, server.URL, strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: "http://test-proxy-stream-header-timeout"},
		},
	}

	_, err = doRequestWithTimeouts(c, req, info, 0, 15*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wait for upstream response headers timed out")
}

func TestDoRequestStreamHeaderTimeoutZeroLeavesContextUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newDirectProxyTestClient(t, "http://test-proxy-stream-header-disabled")
	requestContext := make(chan context.Context, 1)
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestContext <- req.Context()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: "http://test-proxy-stream-header-disabled"},
		},
	}

	resp, err := doRequestWithTimeouts(c, req, info, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	ctx := <-requestContext
	assert.NoError(t, ctx.Err())
	require.NoError(t, resp.Body.Close())
	assert.NoError(t, ctx.Err())
}

func TestDoRequestNonStreamIgnoresStreamHeaderTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newDirectProxyTestClient(t, "http://test-proxy-non-stream-header")
	requestContext := make(chan context.Context, 1)
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestContext <- req.Context()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: &delayedContextReadCloser{
				ctx:    req.Context(),
				delay:  15 * time.Millisecond,
				reader: strings.NewReader("complete"),
			},
			Request: req,
		}, nil
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		IsStream: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: "http://test-proxy-non-stream-header"},
		},
	}

	resp, err := doRequestWithTimeouts(c, req, info, 0, 5*time.Millisecond)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "complete", string(body))
	ctx := <-requestContext
	_, hasDeadline := ctx.Deadline()
	assert.False(t, hasDeadline)
	require.NoError(t, resp.Body.Close())
}

func TestDoRequestNonStreamDeadlineUsesRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newDirectProxyTestClient(t, "http://test-proxy-non-stream-parent-context")
	requestContext := make(chan context.Context, 1)
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestContext <- req.Context()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})

	parentContext, cancelParent := context.WithCancel(context.Background())
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request")).WithContext(parentContext)
	req, err := http.NewRequestWithContext(parentContext, http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		IsStream: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: "http://test-proxy-non-stream-parent-context"},
		},
	}

	resp, err := doRequestWithTimeouts(c, req, info, time.Second, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	upstreamContext := <-requestContext
	cancelParent()
	select {
	case <-upstreamContext.Done():
	case <-time.After(time.Second):
		t.Fatal("downstream request cancellation did not reach upstream")
	}
	require.NoError(t, resp.Body.Close())
}

func TestDoRequestConfiguredStreamHeaderTimeoutUsesGlobalConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := newDirectProxyTestClient(t, "http://test-proxy-stream-header-configured")
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})
	originalTimeout := common.RelayStreamResponseHeaderTimeout
	common.RelayStreamResponseHeaderTimeout = 1
	t.Cleanup(func() { common.RelayStreamResponseHeaderTimeout = originalTimeout })

	newContext := func() *gin.Context {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
		return c
	}
	newRequest := func() *http.Request {
		req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
		require.NoError(t, err)
		return req
	}
	info := &relaycommon.RelayInfo{
		IsStream: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: "http://test-proxy-stream-header-configured"},
		},
	}

	resp, err := doRequest(newContext(), newRequest(), info)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())

	resp, err = doRequestWithTimeouts(newContext(), newRequest(), info, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
}

func TestDoRequestUsesDefaultClientWithoutProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalRelayTimeout := common.RelayTimeout
	originalHeaderTimeout := common.RelayResponseHeaderTimeout
	originalNonStreamTimeout := common.RelayNonStreamTimeout
	originalStreamHeaderTimeout := common.RelayStreamResponseHeaderTimeout
	common.RelayTimeout = 0
	common.RelayResponseHeaderTimeout = 0
	common.RelayNonStreamTimeout = 0
	common.RelayStreamResponseHeaderTimeout = 0
	service.InitHttpClient()
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
		common.RelayResponseHeaderTimeout = originalHeaderTimeout
		common.RelayNonStreamTimeout = originalNonStreamTimeout
		common.RelayStreamResponseHeaderTimeout = originalStreamHeaderTimeout
		service.InitHttpClient()
	})

	client := service.GetHttpClient()
	require.NotNil(t, client)
	originalTransport := client.Transport
	t.Cleanup(func() { client.Transport = originalTransport })
	deadlineSeen := make(chan bool, 1)
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		_, hasDeadline := req.Context().Deadline()
		deadlineSeen <- hasDeadline
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	resp, err := doRequestWithTimeouts(c, req, info, 0, 0)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
	assert.False(t, <-deadlineSeen)
}

func TestDoRequestConfiguredNonStreamTimeoutZeroDisablesDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ResetProxyClientCache()
	defer service.ResetProxyClientCache()
	proxyKey := "http://test-proxy-zero-timeout"
	client, err := service.NewProxyHttpClient(proxyKey)
	require.NoError(t, err)
	deadlineSeen := make(chan bool, 1)
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		_, hasDeadline := req.Context().Deadline()
		deadlineSeen <- hasDeadline
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})

	originalTimeout := common.RelayNonStreamTimeout
	common.RelayNonStreamTimeout = 0
	defer func() { common.RelayNonStreamTimeout = originalTimeout }()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		IsStream: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: proxyKey},
		},
	}

	resp, err := doRequest(c, req, info)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
	assert.False(t, <-deadlineSeen)
}

func TestDoRequestConfiguredNonStreamTimeoutAppliesDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ResetProxyClientCache()
	defer service.ResetProxyClientCache()
	proxyKey := "http://test-proxy-configured-timeout"
	client, err := service.NewProxyHttpClient(proxyKey)
	require.NoError(t, err)
	deadlineSeen := make(chan bool, 1)
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		_, hasDeadline := req.Context().Deadline()
		deadlineSeen <- hasDeadline
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})

	originalTimeout := common.RelayNonStreamTimeout
	common.RelayNonStreamTimeout = 1
	defer func() { common.RelayNonStreamTimeout = originalTimeout }()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		IsStream: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{Proxy: proxyKey},
		},
	}

	resp, err := doRequest(c, req, info)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
	assert.True(t, <-deadlineSeen)
}

func TestDoRequestCancelsDerivedContextOnClientError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.ResetProxyClientCache()
	defer service.ResetProxyClientCache()
	proxyKey := "http://test-proxy-client-error"
	client, err := service.NewProxyHttpClient(proxyKey)
	require.NoError(t, err)
	requestContext := make(chan context.Context, 1)
	client.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestContext <- req.Context()
		return nil, assert.AnError
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{Proxy: proxyKey}},
	}

	_, err = doRequestWithTimeouts(c, req, info, time.Second, 0)
	require.Error(t, err)
	ctx := <-requestContext
	select {
	case <-ctx.Done():
	default:
		t.Fatal("request context was not canceled on client error")
	}
}

func TestDoRequestRejectsInvalidProxyBeforeDial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "http://client", strings.NewReader("request"))
	req, err := http.NewRequest(http.MethodPost, "http://upstream.invalid/v1", strings.NewReader("request"))
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelSetting: dto.ChannelSettings{Proxy: "http://%"}},
	}

	_, err = doRequestWithTimeouts(c, req, info, time.Second, 0)
	assert.Error(t, err)
}

func TestAttachResponseCancellationRejectsInvalidResponses(t *testing.T) {
	t.Run("nil response", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		resp, err := attachResponseCancellation(nil, cancel)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Error(t, ctx.Err())
	})

	t.Run("nil body", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		resp, err := attachResponseCancellation(&http.Response{}, cancel)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Error(t, ctx.Err())
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type trackingReadCloser struct {
	io.Reader
	closed chan struct{}
}

func (body *trackingReadCloser) Close() error {
	select {
	case <-body.closed:
	default:
		close(body.closed)
	}
	return nil
}

type contextBlockingReadCloser struct {
	ctx context.Context
}

func (body *contextBlockingReadCloser) Read([]byte) (int, error) {
	<-body.ctx.Done()
	return 0, body.ctx.Err()
}

func (body *contextBlockingReadCloser) Close() error {
	return nil
}

type delayedContextReadCloser struct {
	ctx    context.Context
	delay  time.Duration
	reader io.Reader
}

func (body *delayedContextReadCloser) Read(p []byte) (int, error) {
	timer := time.NewTimer(body.delay)
	defer timer.Stop()
	select {
	case <-body.ctx.Done():
		return 0, body.ctx.Err()
	case <-timer.C:
		return body.reader.Read(p)
	}
}

func (body *delayedContextReadCloser) Close() error {
	return nil
}

func newDirectProxyTestClient(t *testing.T, proxyKey string) *http.Client {
	t.Helper()
	service.ResetProxyClientCache()
	t.Cleanup(service.ResetProxyClientCache)
	client, err := service.NewProxyHttpClient(proxyKey)
	require.NoError(t, err)
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	require.True(t, ok)
	client.Transport = baseTransport.Clone()
	t.Cleanup(func() {
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	})
	return client
}
