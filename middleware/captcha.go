package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service/captcha"
	cfg "github.com/QuantumNous/new-api/setting/captcha_setting"
	"github.com/gin-gonic/gin"
)

const CaptchaCookie = "new_api_captcha"

func CaptchaFailure(c *gin.Context, err error) {
	status := http.StatusOK
	code := "captcha_failed"
	if !errors.Is(err, captcha.ErrInvalid) {
		status = http.StatusServiceUnavailable
		code = "captcha_unavailable"
	}
	logger.LogWarn(c, "captcha rejected: "+code)
	c.AbortWithStatusJSON(status, gin.H{"success": false, "message": "Security verification failed. Please try again.", "code": code})
}

// CaptchaCheck executes after existing authentication/rate limits and before
// any business effects. The challenge is bound to this exact JSON operation.
func CaptchaCheck(scene string) gin.HandlerFunc {
	admission := rateLimitFactory(60, 60, "CAPTCHA_CHALLENGE")
	return func(c *gin.Context) {
		config, err := cfg.Load()
		if err != nil {
			CaptchaFailure(c, err)
			return
		}
		if config.Provider(scene) == "off" {
			c.Next()
			return
		}
		var body []byte
		if c.Request.Body != nil && !strings.HasPrefix(c.ContentType(), "multipart/") {
			body, err = io.ReadAll(io.LimitReader(c.Request.Body, 65537))
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			if err != nil || len(body) > 65536 {
				CaptchaFailure(c, captcha.ErrInvalid)
				return
			}
		}
		if scene == "discount_code" {
			var fields struct {
				DiscountCode string `json:"discount_code"`
				Code         string `json:"code"`
			}
			if common.Unmarshal(body, &fields) != nil {
				CaptchaFailure(c, captcha.ErrInvalid)
				return
			}
			if fields.DiscountCode == "" && fields.Code == "" {
				c.Next()
				return
			}
		}
		admission(c)
		if c.IsAborted() {
			return
		}
		browser, _ := c.Cookie(CaptchaCookie)
		if len(browser) != 64 {
			browser, err = captcha.RandomToken()
			if err != nil {
				CaptchaFailure(c, err)
				return
			}
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie(CaptchaCookie, browser, 3600, "/api", "", c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https", true)
		}
		query := c.Request.URL.Query()
		query.Del("turnstile")
		target := c.Request.Method + "\n" + c.Request.Host + "\n" + c.Request.URL.Path + "\n" + query.Encode() + "\n" + strconv.Itoa(c.GetInt("id")) + "\n" + string(body)
		claim := captcha.Claim{Scene: scene, Browser: captcha.Digest(browser), Target: captcha.Digest(target), Revision: config.Fingerprint()}
		proof := c.GetHeader("X-Captcha-Proof")
		if proof != "" {
			if err = captcha.ConsumeProof(c.Request.Context(), captcha.ActiveStore(), proof, claim); err != nil {
				CaptchaFailure(c, err)
				return
			}
			c.Next()
			return
		}
		id, err := captcha.Issue(c.Request.Context(), captcha.ActiveStore(), "challenge:", claim)
		if err != nil {
			CaptchaFailure(c, err)
			return
		}
		challenge, err := captcha.ChallengeConfig(config, scene, id)
		if err != nil {
			CaptchaFailure(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.AbortWithStatusJSON(http.StatusOK, gin.H{"success": false, "code": "captcha_required", "message": "Security verification required", "data": challenge})
	}
}
