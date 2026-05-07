package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func GetPaymentReturnURL(page string, query string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")
	theme := common.GetTheme()

	var path string
	switch page {
	case "billing":
		if theme == "default" {
			path = "/topup-history"
		} else {
			path = "/console/log"
		}
	case "wallet":
		if theme == "default" {
			path = "/wallet"
		} else {
			path = "/console/topup"
		}
	default:
		path = "/"
	}

	if query != "" {
		return base + path + "?" + query
	}
	return base + path
}
