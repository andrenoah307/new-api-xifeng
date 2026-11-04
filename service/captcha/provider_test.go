package captcha

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	cfg "github.com/QuantumNous/new-api/setting/captcha_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTencentAppIDAuthenticationEnvelope(t *testing.T) {
	config := cfg.Config{Enabled: true, Scenes: map[string]string{"login": "tencent"}, TencentAppID: "123456789", TencentAppSecret: "1234567891011121314151516", TencentAppIDAuth: true}
	challenge, err := ChallengeConfig(config, "login", "challenge-id")
	require.NoError(t, err)
	envelope, err := base64.StdEncoding.DecodeString(challenge.AppIDEncrypted)
	require.NoError(t, err)
	require.Greater(t, len(envelope), 28)
	key := []byte("12345678910111213141515161234567")
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	gcm, err := cipher.NewGCM(block)
	require.NoError(t, err)
	plain, err := gcm.Open(nil, envelope[:12], envelope[12:], nil)
	require.NoError(t, err)
	assert.Regexp(t, `^123456789&[0-9]+&120$`, string(plain))
	assert.Equal(t, "123456789", challenge.AppID)
}

type captchaTransport func(*http.Request) (*http.Response, error)

func (f captchaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProviderRequiresServerVerification(t *testing.T) {
	old := verificationClient
	t.Cleanup(func() { verificationClient = old })
	for _, tc := range []struct {
		name, provider, response string
		want                     error
	}{
		{"turnstile accepted", "turnstile", `{"success":true}`, nil},
		{"turnstile rejected", "turnstile", `{"success":false}`, ErrInvalid},
		{"turnstile malformed", "turnstile", `invalid`, ErrUnavailable},
		{"tencent accepted", "tencent", `{"Response":{"CaptchaCode":1,"EvilLevel":0}}`, nil},
		{"tencent rejected", "tencent", `{"Response":{"CaptchaCode":6}}`, ErrInvalid},
		{"tencent risk", "tencent", `{"Response":{"CaptchaCode":1,"EvilLevel":100}}`, ErrInvalid},
		{"tencent absent result", "tencent", `{"Response":{}}`, ErrInvalid},
		{"tencent API error", "tencent", `{"Response":{"Error":{"Code":"AuthFailure","Message":"denied"}}}`, ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			verificationClient = &http.Client{Transport: captchaTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if tc.provider == "tencent" {
					assert.Equal(t, "DescribeCaptchaResult", r.Header.Get("X-TC-Action"))
					var request map[string]any
					require.NoError(t, common.DecodeJson(r.Body, &request))
					assert.Equal(t, float64(9), request["CaptchaType"])
					assert.Equal(t, "203.0.113.5", request["UserIp"])
					assert.Equal(t, "nonce", request["Randstr"])
				} else {
					require.NoError(t, r.ParseForm())
					assert.Equal(t, "203.0.113.5", r.Form.Get("remoteip"))
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})}
			config := cfg.Config{Enabled: true, Scenes: map[string]string{"login": tc.provider}, TencentAppID: "123", TencentAppSecret: "app-secret", TencentSecretID: "id", TencentSecretKey: "key", TurnstileSecret: "secret"}
			err := Verify(context.Background(), config, "login", "ticket", "nonce", "203.0.113.5")
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
			assert.Equal(t, 1, calls)
			require.ErrorIs(t, Verify(context.Background(), config, "login", "trerror_disaster", "nonce", "203.0.113.5"), ErrInvalid)
			assert.Equal(t, 1, calls, "disaster tickets must not reach the provider")
		})
	}
}
