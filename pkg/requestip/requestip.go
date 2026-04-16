package requestip

import (
	"net"
	"strings"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// GetClientIP returns the client IP used by security-sensitive modules.
// Default behavior is conservative: trust no proxy headers and use RemoteAddr.
// Admins may explicitly enable a trusted upstream IP header for reverse proxies/CDNs.
func GetClientIP(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	cfg := operation_setting.GetRiskControlSetting()
	if cfg != nil && cfg.TrustedIPHeaderEnabled {
		headerName := strings.TrimSpace(cfg.TrustedIPHeader)
		if headerName != "" {
			if ip := parseHeaderIP(c.GetHeader(headerName)); ip != "" {
				return ip
			}
		}
	}
	return parseRemoteAddrIP(c.Request.RemoteAddr)
}

func parseHeaderIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, segment := range strings.Split(value, ",") {
		if ip := normalizeIPToken(segment); ip != "" {
			return ip
		}
	}
	return ""
}

func parseRemoteAddrIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return ""
	}
	if ip := normalizeIPToken(remoteAddr); ip != "" {
		return ip
	}
	return remoteAddr
}

func normalizeIPToken(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"'`))
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	value = strings.Trim(strings.TrimSpace(value), "[]")
	ip := net.ParseIP(value)
	if ip == nil {
		return ""
	}
	return ip.String()
}
