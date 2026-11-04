package captcha

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	cfg "github.com/QuantumNous/new-api/setting/captcha_setting"
	tc "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/captcha/v20190722"
	tccommon "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
)

var verificationClient = &http.Client{Timeout: 5 * time.Second}

type PublicChallenge struct {
	ID             string `json:"id"`
	Provider       string `json:"provider"`
	SiteKey        string `json:"site_key,omitempty"`
	AppID          string `json:"app_id,omitempty"`
	AppIDEncrypted string `json:"aid_encrypted,omitempty"`
}

func ChallengeConfig(c cfg.Config, scene, id string) (PublicChallenge, error) {
	result := PublicChallenge{ID: id, Provider: c.Provider(scene)}
	if result.Provider == "turnstile" {
		result.SiteKey = c.TurnstileSiteKey
	}
	if result.Provider == "tencent" {
		result.AppID = c.TencentAppID
		if c.TencentAppIDAuth {
			// Tencent's documented GCM envelope: padded app secret, random 12-byte
			// nonce, then ciphertext and authentication tag. Never expose the secret.
			if c.TencentAppSecret == "" {
				return result, ErrUnavailable
			}
			key := make([]byte, 32)
			for i := range key {
				key[i] = c.TencentAppSecret[i%len(c.TencentAppSecret)]
			}
			block, err := aes.NewCipher(key)
			if err != nil {
				return result, err
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return result, err
			}
			nonce := make([]byte, gcm.NonceSize())
			if _, err = rand.Read(nonce); err != nil {
				return result, err
			}
			plain := fmt.Sprintf("%s&%d&120", c.TencentAppID, time.Now().Unix())
			result.AppIDEncrypted = base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil))
		}
	}
	return result, nil
}

func Verify(ctx context.Context, c cfg.Config, scene, ticket, randstr, ip string) error {
	if ticket == "" || len(ticket) > 8192 || len(randstr) > 1024 || strings.HasPrefix(ticket, "trerror") {
		return ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	switch c.Provider(scene) {
	case "turnstile":
		form := url.Values{"secret": {c.TurnstileSecret}, "response": {ticket}, "remoteip": {ip}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
		if err != nil {
			return ErrUnavailable
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := verificationClient.Do(req)
		if err != nil {
			return ErrUnavailable
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return ErrUnavailable
		}
		var result struct {
			Success bool `json:"success"`
		}
		if err = common.DecodeJson(io.LimitReader(resp.Body, 65536), &result); err != nil {
			return ErrUnavailable
		}
		if !result.Success {
			return ErrInvalid
		}
		return nil
	case "tencent":
		if randstr == "" {
			return ErrInvalid
		}
		p := profile.NewClientProfile()
		p.HttpProfile.ReqTimeout = 5
		client, err := tc.NewClient(tccommon.NewCredential(c.TencentSecretID, c.TencentSecretKey), "", p)
		if err != nil {
			return ErrUnavailable
		}
		if verificationClient.Transport != nil {
			client.WithHttpTransport(verificationClient.Transport)
		}
		req := tc.NewDescribeCaptchaResultRequest()
		id, err := strconv.ParseUint(c.TencentAppID, 10, 64)
		if err != nil {
			return ErrUnavailable
		}
		req.CaptchaAppId = &id
		req.CaptchaType = tccommon.Uint64Ptr(9)
		req.AppSecretKey = &c.TencentAppSecret
		req.Ticket = &ticket
		req.Randstr = &randstr
		req.UserIp = &ip
		response, err := client.DescribeCaptchaResultWithContext(ctx, req)
		if err != nil || response == nil || response.Response == nil {
			return ErrUnavailable
		}
		r := response.Response
		if r.CaptchaCode == nil || *r.CaptchaCode != 1 || (r.EvilLevel != nil && *r.EvilLevel != 0) {
			return ErrInvalid
		}
		return nil
	default:
		return ErrInvalid
	}
}
