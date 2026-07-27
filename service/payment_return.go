package service

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/system_setting"
)

func GetPaymentReturnURL(page string, query string) string {
	base := strings.TrimRight(system_setting.ServerAddress, "/")

	var path string
	switch page {
	case "billing":
		path = "/topup-history"
	case "wallet":
		path = "/wallet"
	default:
		path = "/"
	}

	if query != "" {
		return base + path + "?" + query
	}
	return base + path
}
