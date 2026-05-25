package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type DiscountCodeSetting struct {
	Enabled bool `json:"enabled"`
}

var discountCodeSetting = DiscountCodeSetting{Enabled: false}

func init() {
	config.GlobalConfig.Register("discount_code_setting", &discountCodeSetting)
}

func GetDiscountCodeSetting() *DiscountCodeSetting {
	return &discountCodeSetting
}

func IsDiscountCodeEnabled() bool {
	return discountCodeSetting.Enabled
}
