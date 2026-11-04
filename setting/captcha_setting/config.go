package captcha_setting

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

// The suffix keeps the complete secret-bearing document out of /api/option.
const OptionKey = "CaptchaConfigSecret"

var Scenes = []string{"login", "register", "email_verification", "password_reset_email", "password_reset", "discount_code", "redeem", "ticket_create", "ticket_reply", "ticket_attachment", "checkin"}

type Config struct {
	Enabled          bool              `json:"enabled"`
	Scenes           map[string]string `json:"scenes"`
	TurnstileSiteKey string            `json:"turnstile_site_key"`
	TurnstileSecret  string            `json:"turnstile_secret"`
	TencentAppID     string            `json:"tencent_app_id"`
	TencentAppSecret string            `json:"tencent_app_secret"`
	TencentSecretID  string            `json:"tencent_secret_id"`
	TencentSecretKey string            `json:"tencent_secret_key"`
	TencentAppIDAuth bool              `json:"tencent_app_id_auth"`
}

func KnownScene(scene string) bool {
	for _, s := range Scenes {
		if s == scene {
			return true
		}
	}
	return false
}

func (c Config) Provider(scene string) string {
	if !c.Enabled || c.Scenes[scene] == "" {
		return "off"
	}
	return c.Scenes[scene]
}

func (c Config) Validate() error {
	for scene, p := range c.Scenes {
		if !KnownScene(scene) || (p != "off" && p != "turnstile" && p != "tencent") {
			return errors.New("invalid captcha policy")
		}
		if !c.Enabled || p == "off" {
			continue
		}
		if p == "turnstile" && (c.TurnstileSiteKey == "" || c.TurnstileSecret == "") {
			return errors.New("incomplete Turnstile configuration")
		}
		if p == "tencent" {
			id, err := strconv.ParseUint(c.TencentAppID, 10, 64)
			if err != nil || id == 0 || c.TencentAppSecret == "" || c.TencentSecretID == "" || c.TencentSecretKey == "" {
				return errors.New("incomplete Tencent configuration")
			}
		}
	}
	for _, v := range []string{c.TurnstileSiteKey, c.TurnstileSecret, c.TencentAppID, c.TencentAppSecret, c.TencentSecretID, c.TencentSecretKey} {
		if len(v) > 2048 {
			return errors.New("captcha setting too long")
		}
	}
	if c.TencentAppIDAuth && len(c.TencentAppSecret) > 32 {
		return errors.New("invalid Tencent AppSecretKey length for GCM")
	}
	return nil
}

func (c Config) Fingerprint() string {
	b, _ := common.Marshal(c)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Read a snapshot under the existing option lock. No mutable configuration is
// exposed to request handlers; multi-node updates use the normal options sync.
func Load() (Config, error) {
	c, _, err := LoadSnapshot()
	if err != nil {
		return c, err
	}
	return c, c.Validate()
}

func LoadSnapshot() (Config, string, error) {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[OptionKey]
	enabled := common.OptionMap["TurnstileCheckEnabled"] == "true"
	site, secret := common.OptionMap["TurnstileSiteKey"], common.OptionMap["TurnstileSecretKey"]
	common.OptionMapRWMutex.RUnlock()
	c := Config{Enabled: enabled, Scenes: map[string]string{}, TurnstileSiteKey: site, TurnstileSecret: secret}
	if raw != "" {
		var stored *Config
		if err := common.UnmarshalJsonStr(raw, &stored); err != nil {
			return Config{}, raw, err
		}
		if stored == nil {
			return Config{}, raw, errors.New("invalid captcha configuration")
		}
		c = *stored
	} else if enabled {
		for _, s := range []string{"login", "register", "email_verification", "password_reset_email", "checkin"} {
			c.Scenes[s] = "turnstile"
		}
	}
	if c.Scenes == nil {
		c.Scenes = map[string]string{}
	}
	return c, raw, nil
}
