package system_setting

import "github.com/QuantumNous/new-api/setting/config"

type CnDisclaimerSettings struct {
	Enabled          bool     `json:"enabled"`
	Title            string   `json:"title"`
	Content          string   `json:"content"`
	BlockedCountries []string `json:"blocked_countries"`
	SilenceMinutes   int      `json:"silence_minutes"`
}

const defaultCnDisclaimerTitle = "需要被授权的地区访问许可"

const defaultCnDisclaimerContent = "我司检测到您的 IP 地址来源于中国大陆，此在我们所授权的可访问的服务地区以外。我司不会以任何形式存储您的信息，仅提供面向模型服务提供商的转发能力。请您务必知晓我司用户协议与您所在地区的相关法规并取得相关授权，我司不对您的任何行为负责。"

var defaultCnDisclaimerSettings = CnDisclaimerSettings{
	Enabled:          true,
	Title:            defaultCnDisclaimerTitle,
	Content:          defaultCnDisclaimerContent,
	BlockedCountries: []string{"CN"},
	SilenceMinutes:   60,
}

func init() {
	config.GlobalConfig.Register("cn_disclaimer", &defaultCnDisclaimerSettings)
}

func GetCnDisclaimerSettings() *CnDisclaimerSettings {
	return &defaultCnDisclaimerSettings
}

func IsCnDisclaimerCountry(code string) bool {
	if !defaultCnDisclaimerSettings.Enabled {
		return false
	}
	if code == "" {
		return false
	}
	for _, c := range defaultCnDisclaimerSettings.BlockedCountries {
		if c == code {
			return true
		}
	}
	return false
}
