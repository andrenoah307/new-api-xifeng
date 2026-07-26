package service

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayTransportResponseHeaderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	transport := newRelayTransport(nil, nil)
	transport.ResponseHeaderTimeout = 25 * time.Millisecond
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "timeout")
}

func TestRelayTransportResponseHeaderTimeoutCanBeDisabled(t *testing.T) {
	originalResponseHeaderTimeout := common.RelayResponseHeaderTimeout
	common.RelayResponseHeaderTimeout = 0
	defer func() { common.RelayResponseHeaderTimeout = originalResponseHeaderTimeout }()
	requestStarted := make(chan struct{})
	releaseHeaders := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-releaseHeaders
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	defer func() {
		select {
		case <-releaseHeaders:
		default:
			close(releaseHeaders)
		}
	}()

	transport := newRelayTransport(nil, nil)
	transport.ResponseHeaderTimeout = 0
	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	result := make(chan struct {
		resp *http.Response
		err  error
	}, 1)
	go func() {
		resp, err := client.Do(req)
		result <- struct {
			resp *http.Response
			err  error
		}{resp: resp, err: err}
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("server did not receive request")
	}
	close(releaseHeaders)
	requestResult := <-result
	resp, err := requestResult.resp, requestResult.err
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NoError(t, resp.Body.Close())
}

func TestRelayTransportCustomDialerPreservesContextCancellation(t *testing.T) {
	dialStarted := make(chan struct{})
	transport := newRelayTransport(nil, func(ctx context.Context, network, address string) (net.Conn, error) {
		close(dialStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := transport.DialContext(ctx, "tcp", "unused:443")
	require.Error(t, err)
	select {
	case <-dialStarted:
	case <-time.After(time.Second):
		t.Fatal("custom dialer was not called")
	}
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRelayTimeoutEnvironmentDefaultsAreDisabled(t *testing.T) {
	for _, name := range []string{
		"RELAY_TIMEOUT",
		"RELAY_RESPONSE_HEADER_TIMEOUT",
		"RELAY_NON_STREAM_TIMEOUT",
		"RELAY_STREAM_RESPONSE_HEADER_TIMEOUT",
	} {
		originalValue, wasSet := os.LookupEnv(name)
		name, originalValue, wasSet := name, originalValue, wasSet
		require.NoError(t, os.Unsetenv(name))
		t.Cleanup(func() {
			if wasSet {
				_ = os.Setenv(name, originalValue)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}

	// Production measurements reached 6220s for non-stream responses and about
	// 1216s for streaming FRT; any non-zero default would cut real traffic.
	assert.Zero(t, common.GetEnvOrDefault("RELAY_TIMEOUT", 0))
	assert.Zero(t, common.GetEnvOrDefault("RELAY_RESPONSE_HEADER_TIMEOUT", 0))
	assert.Zero(t, common.GetEnvOrDefault("RELAY_NON_STREAM_TIMEOUT", 0))
	assert.Zero(t, common.GetEnvOrDefault("RELAY_STREAM_RESPONSE_HEADER_TIMEOUT", 0))
}

func TestAllRelayHTTPClientsConfigureTransportTimeouts(t *testing.T) {
	originalResponseHeaderTimeout := common.RelayResponseHeaderTimeout
	originalIdleConnTimeout := common.RelayIdleConnTimeout
	originalMaxIdleConns := common.RelayMaxIdleConns
	originalMaxIdleConnsPerHost := common.RelayMaxIdleConnsPerHost
	originalRelayTimeout := common.RelayTimeout
	originalStreamResponseHeaderTimeout := common.RelayStreamResponseHeaderTimeout
	originalTLSInsecureSkipVerify := common.TLSInsecureSkipVerify
	originalHTTPClient := httpClient
	originalProtectedClient := ssrfProtectedHTTPClient
	originalRelayTimeoutEnv, hadRelayTimeoutEnv := os.LookupEnv("RELAY_TIMEOUT")
	originalResponseTimeoutEnv, hadResponseTimeoutEnv := os.LookupEnv("RELAY_RESPONSE_HEADER_TIMEOUT")
	originalNonStreamTimeoutEnv, hadNonStreamTimeoutEnv := os.LookupEnv("RELAY_NON_STREAM_TIMEOUT")
	originalStreamResponseTimeoutEnv, hadStreamResponseTimeoutEnv := os.LookupEnv("RELAY_STREAM_RESPONSE_HEADER_TIMEOUT")
	t.Cleanup(func() {
		common.RelayResponseHeaderTimeout = originalResponseHeaderTimeout
		common.RelayIdleConnTimeout = originalIdleConnTimeout
		common.RelayMaxIdleConns = originalMaxIdleConns
		common.RelayMaxIdleConnsPerHost = originalMaxIdleConnsPerHost
		common.RelayTimeout = originalRelayTimeout
		common.RelayStreamResponseHeaderTimeout = originalStreamResponseHeaderTimeout
		common.TLSInsecureSkipVerify = originalTLSInsecureSkipVerify
		httpClient = originalHTTPClient
		ssrfProtectedHTTPClient = originalProtectedClient
		if hadRelayTimeoutEnv {
			_ = os.Setenv("RELAY_TIMEOUT", originalRelayTimeoutEnv)
		} else {
			_ = os.Unsetenv("RELAY_TIMEOUT")
		}
		if hadResponseTimeoutEnv {
			_ = os.Setenv("RELAY_RESPONSE_HEADER_TIMEOUT", originalResponseTimeoutEnv)
		} else {
			_ = os.Unsetenv("RELAY_RESPONSE_HEADER_TIMEOUT")
		}
		if hadNonStreamTimeoutEnv {
			_ = os.Setenv("RELAY_NON_STREAM_TIMEOUT", originalNonStreamTimeoutEnv)
		} else {
			_ = os.Unsetenv("RELAY_NON_STREAM_TIMEOUT")
		}
		if hadStreamResponseTimeoutEnv {
			_ = os.Setenv("RELAY_STREAM_RESPONSE_HEADER_TIMEOUT", originalStreamResponseTimeoutEnv)
		} else {
			_ = os.Unsetenv("RELAY_STREAM_RESPONSE_HEADER_TIMEOUT")
		}
		ResetProxyClientCache()
	})

	common.RelayResponseHeaderTimeout = 0
	defaultTransport := newRelayTransport(nil, nil)
	assert.Zero(t, defaultTransport.ResponseHeaderTimeout)

	common.RelayResponseHeaderTimeout = 7
	common.RelayIdleConnTimeout = 11
	common.RelayMaxIdleConns = 13
	common.RelayMaxIdleConnsPerHost = 17
	_ = os.Unsetenv("RELAY_TIMEOUT")
	_ = os.Unsetenv("RELAY_RESPONSE_HEADER_TIMEOUT")
	_ = os.Unsetenv("RELAY_NON_STREAM_TIMEOUT")
	_ = os.Unsetenv("RELAY_STREAM_RESPONSE_HEADER_TIMEOUT")
	common.RelayTimeout = common.GetEnvOrDefault("RELAY_TIMEOUT", 0)
	assert.Zero(t, common.RelayTimeout)
	assert.Zero(t, common.GetEnvOrDefault("RELAY_RESPONSE_HEADER_TIMEOUT", 0))
	assert.Zero(t, common.GetEnvOrDefault("RELAY_NON_STREAM_TIMEOUT", 0))
	assert.Zero(t, common.GetEnvOrDefault("RELAY_STREAM_RESPONSE_HEADER_TIMEOUT", 0))
	common.TLSInsecureSkipVerify = false

	InitHttpClient()
	mainTransport, ok := httpClient.Transport.(*http.Transport)
	require.True(t, ok)

	protectedRoundTripper, ok := ssrfProtectedHTTPClient.Transport.(*ssrfProtectedRoundTripper)
	require.True(t, ok)
	protectedTransport := protectedRoundTripper.transportFor(nil)

	httpProxyClient, err := NewProxyHttpClient("http://127.0.0.1:1")
	require.NoError(t, err)
	httpProxyTransport, ok := httpProxyClient.Transport.(*http.Transport)
	require.True(t, ok)

	socksProxyClient, err := NewProxyHttpClient("socks5://127.0.0.1:1")
	require.NoError(t, err)
	socksProxyTransport, ok := socksProxyClient.Transport.(*http.Transport)
	require.True(t, ok)

	transports := []*http.Transport{mainTransport, protectedTransport, httpProxyTransport, socksProxyTransport}
	for _, transport := range transports {
		require.NotNil(t, transport)
		assert.Equal(t, 7*time.Second, transport.ResponseHeaderTimeout)
		assert.Equal(t, 10*time.Second, transport.TLSHandshakeTimeout)
		assert.Equal(t, time.Second, transport.ExpectContinueTimeout)
		assert.NotNil(t, transport.DialContext)
	}

	assert.Zero(t, httpClient.Timeout)
	assert.Zero(t, ssrfProtectedHTTPClient.Timeout)
	assert.Zero(t, httpProxyClient.Timeout)
	assert.Zero(t, socksProxyClient.Timeout)

	common.TLSInsecureSkipVerify = true
	tlsTransport := newRelayTransport(nil, nil)
	assert.Same(t, common.InsecureTLSConfig, tlsTransport.TLSClientConfig)
}

func TestRelayClientTimeoutRemainsOptIn(t *testing.T) {
	originalRelayTimeout := common.RelayTimeout
	originalResponseHeaderTimeout := common.RelayResponseHeaderTimeout
	originalIdleConnTimeout := common.RelayIdleConnTimeout
	originalMaxIdleConns := common.RelayMaxIdleConns
	originalMaxIdleConnsPerHost := common.RelayMaxIdleConnsPerHost
	originalHTTPClient := httpClient
	originalProtectedClient := ssrfProtectedHTTPClient
	t.Cleanup(func() {
		common.RelayTimeout = originalRelayTimeout
		common.RelayResponseHeaderTimeout = originalResponseHeaderTimeout
		common.RelayIdleConnTimeout = originalIdleConnTimeout
		common.RelayMaxIdleConns = originalMaxIdleConns
		common.RelayMaxIdleConnsPerHost = originalMaxIdleConnsPerHost
		httpClient = originalHTTPClient
		ssrfProtectedHTTPClient = originalProtectedClient
		ResetProxyClientCache()
	})

	common.RelayTimeout = 2
	common.RelayResponseHeaderTimeout = 0
	common.RelayIdleConnTimeout = 0
	common.RelayMaxIdleConns = 0
	common.RelayMaxIdleConnsPerHost = 0

	InitHttpClient()
	assert.Equal(t, 2*time.Second, httpClient.Timeout)

	proxyClient, err := NewProxyHttpClient("http://127.0.0.1:2")
	require.NoError(t, err)
	assert.Equal(t, 2*time.Second, proxyClient.Timeout)
}

func TestProxyClientConstructionHandlesConfiguredVariants(t *testing.T) {
	originalHTTPClient := httpClient
	originalRelayTimeout := common.RelayTimeout
	t.Cleanup(func() {
		httpClient = originalHTTPClient
		common.RelayTimeout = originalRelayTimeout
		ResetProxyClientCache()
	})

	common.RelayTimeout = 0
	httpClient = &http.Client{}
	client, err := GetHttpClientWithProxy("")
	require.NoError(t, err)
	assert.Same(t, httpClient, client)

	proxyURL := "http://127.0.0.1:3"
	first, err := NewProxyHttpClient(proxyURL)
	require.NoError(t, err)
	viaHelper, err := GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)
	assert.Same(t, first, viaHelper)
	second, err := NewProxyHttpClient(proxyURL)
	require.NoError(t, err)
	assert.Same(t, first, second)
	fromEmpty, err := NewProxyHttpClient("")
	require.NoError(t, err)
	assert.Same(t, httpClient, fromEmpty)
	httpClient = nil
	fromDefault, err := NewProxyHttpClient("")
	require.NoError(t, err)
	assert.Same(t, http.DefaultClient, fromDefault)

	_, err = NewProxyHttpClient("http://%")
	assert.Error(t, err)
	_, err = NewProxyHttpClient("ftp://127.0.0.1:3")
	assert.Error(t, err)

	authClient, err := NewProxyHttpClient("socks5://user:password@127.0.0.1:4")
	require.NoError(t, err)
	assert.NotNil(t, authClient.Transport)
}

func TestGetSSRFProtectedHTTPClientReturnsConfiguredClient(t *testing.T) {
	originalHTTPClient := httpClient
	originalProtectedClient := ssrfProtectedHTTPClient
	t.Cleanup(func() {
		httpClient = originalHTTPClient
		ssrfProtectedHTTPClient = originalProtectedClient
	})

	httpClient = &http.Client{}
	ssrfProtectedHTTPClient = &http.Client{}
	fetchSetting := system_setting.GetFetchSetting()
	originalSetting := *fetchSetting
	t.Cleanup(func() { *fetchSetting = originalSetting })
	fetchSetting.EnableSSRFProtection = true
	assert.Same(t, ssrfProtectedHTTPClient, GetSSRFProtectedHTTPClient())
}
