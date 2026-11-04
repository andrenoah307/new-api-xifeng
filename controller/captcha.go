package controller

import (
	"net/http"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/requestip"
	"github.com/QuantumNous/new-api/service/captcha"
	cfg "github.com/QuantumNous/new-api/setting/captcha_setting"
	"github.com/gin-gonic/gin"
)

var captchaConfigMu sync.Mutex

func VerifyCaptcha(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16384)
	var req struct {
		ID      string `json:"id"`
		Ticket  string `json:"ticket"`
		Randstr string `json:"randstr"`
	}
	if common.DecodeJson(c.Request.Body, &req) != nil {
		middleware.CaptchaFailure(c, captcha.ErrInvalid)
		return
	}
	config, err := cfg.Load()
	if err != nil {
		middleware.CaptchaFailure(c, err)
		return
	}
	browser, _ := c.Cookie(middleware.CaptchaCookie)
	if len(browser) != 64 {
		middleware.CaptchaFailure(c, captcha.ErrInvalid)
		return
	}
	claim, err := captcha.ConsumeChallenge(c.Request.Context(), captcha.ActiveStore(), req.ID, captcha.Digest(browser), config.Fingerprint())
	if err != nil {
		middleware.CaptchaFailure(c, err)
		return
	}
	if err = captcha.Verify(c.Request.Context(), config, claim.Scene, req.Ticket, req.Randstr, requestip.GetClientIP(c)); err != nil {
		middleware.CaptchaFailure(c, err)
		return
	}
	// A configuration change during a provider request must not mint old proofs.
	current, err := cfg.Load()
	if err != nil || current.Fingerprint() != claim.Revision {
		middleware.CaptchaFailure(c, captcha.ErrInvalid)
		return
	}
	proof, err := captcha.Issue(c.Request.Context(), captcha.ActiveStore(), "proof:", claim)
	if err != nil {
		middleware.CaptchaFailure(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"proof": proof})
}

func CaptchaSettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil || user == nil || user.Role != common.RoleRootUser || user.Status != common.UserStatusEnabled {
		middleware.CaptchaFailure(c, captcha.ErrInvalid)
		return
	}
	captchaConfigMu.Lock()
	defer captchaConfigMu.Unlock()
	config, previous, err := cfg.LoadSnapshot()
	if err != nil {
		middleware.CaptchaFailure(c, err)
		return
	}
	if c.Request.Method == http.MethodPut {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32768)
		var update struct {
			Config       cfg.Config `json:"config"`
			ClearSecrets []string   `json:"clear_secrets"`
			Revision     string     `json:"revision"`
		}
		if common.DecodeJson(c.Request.Body, &update) != nil || update.Revision != config.Fingerprint() {
			middleware.CaptchaFailure(c, captcha.ErrInvalid)
			return
		}
		if update.Config.Scenes == nil {
			update.Config.Scenes = map[string]string{}
		}
		oldSecrets := map[string]string{"turnstile_secret": config.TurnstileSecret, "tencent_app_secret": config.TencentAppSecret, "tencent_secret_id": config.TencentSecretID, "tencent_secret_key": config.TencentSecretKey}
		fields := map[string]*string{"turnstile_secret": &update.Config.TurnstileSecret, "tencent_app_secret": &update.Config.TencentAppSecret, "tencent_secret_id": &update.Config.TencentSecretID, "tencent_secret_key": &update.Config.TencentSecretKey}
		for k, p := range fields {
			if *p == "" {
				*p = oldSecrets[k]
			}
		}
		for _, k := range update.ClearSecrets {
			p, ok := fields[k]
			if !ok {
				middleware.CaptchaFailure(c, captcha.ErrInvalid)
				return
			}
			*p = ""
		}
		if update.Config.Validate() != nil {
			middleware.CaptchaFailure(c, captcha.ErrInvalid)
			return
		}
		raw, err := common.Marshal(update.Config)
		if err != nil {
			middleware.CaptchaFailure(c, err)
			return
		}
		if err = model.UpdateCaptchaConfig(previous, string(raw)); err != nil {
			middleware.CaptchaFailure(c, err)
			return
		}
		config = update.Config
		recordManageAudit(c, "captcha.settings_update", map[string]interface{}{"enabled": config.Enabled, "scenes": config.Scenes})
	}
	revision := config.Fingerprint()
	configured := map[string]bool{"turnstile_secret": config.TurnstileSecret != "", "tencent_app_secret": config.TencentAppSecret != "", "tencent_secret_id": config.TencentSecretID != "", "tencent_secret_key": config.TencentSecretKey != ""}
	config.TurnstileSecret = ""
	config.TencentAppSecret = ""
	config.TencentSecretID = ""
	config.TencentSecretKey = ""
	common.ApiSuccess(c, gin.H{"config": config, "configured": configured, "revision": revision, "scenes": cfg.Scenes})
}
