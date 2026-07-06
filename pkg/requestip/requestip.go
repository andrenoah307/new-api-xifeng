package requestip

import (
	"net"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/pkg/geoip"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

var diagnosisHeaderOrder = []string{
	"X-Real-IP",
	"CF-Connecting-IP",
	"True-Client-IP",
	"X-Forwarded-For",
	"Forwarded",
	"X-Client-IP",
	"X-Original-Forwarded-For",
	"X-Cluster-Client-IP",
	"Fly-Client-IP",
	"Fastly-Client-IP",
}

var recommendationHeaderOrder = []string{
	"X-Real-IP",
	"CF-Connecting-IP",
	"True-Client-IP",
	"X-Forwarded-For",
	"Forwarded",
	"X-Client-IP",
	"X-Original-Forwarded-For",
	"X-Cluster-Client-IP",
	"Fly-Client-IP",
	"Fastly-Client-IP",
}

type DiagnosisItem struct {
	Name           string `json:"name"`
	Source         string `json:"source"`
	RawValue       string `json:"raw_value"`
	ParsedIP       string `json:"parsed_ip"`
	Present        bool   `json:"present"`
	Valid          bool   `json:"valid"`
	Classification string `json:"classification"`
	IsCurrent      bool   `json:"is_current"`
}

// ModePreview shows what IP each detection mode would resolve for the current
// request, so admins can pick the mode whose result matches their real IP.
type ModePreview struct {
	Mode      string `json:"mode"`
	IP        string `json:"ip"`
	Source    string `json:"source"`
	IsCurrent bool   `json:"is_current"`
}

type Diagnosis struct {
	CurrentMode           string          `json:"current_mode"`
	CurrentHeader         string          `json:"current_header"`
	CurrentXFFIndex       int             `json:"current_xff_index"`
	EffectiveClientIP     string          `json:"effective_client_ip"`
	EffectiveSource       string          `json:"effective_source"`
	RecommendedMode       string          `json:"recommended_mode"`
	RecommendedHeader     string          `json:"recommended_header"`
	RecommendationMessage string          `json:"recommendation_message"`
	ModePreviews          []ModePreview   `json:"mode_previews"`
	XFFIPs                []string        `json:"xff_ips"`
	Items                 []DiagnosisItem `json:"items"`
}

// GetClientCountry returns the ISO country code for the client.
// Prefers CDN-provided Cf-Ipcountry header (authoritative for Cloudflare-proxied
// requests), falls back to ip2region offline lookup.
func GetClientCountry(c *gin.Context) string {
	if cc := c.GetHeader("Cf-Ipcountry"); cc != "" && cc != "XX" && cc != "T1" {
		return strings.ToUpper(cc)
	}
	return geoip.LookupCountry(GetClientIP(c))
}

// GetClientIP returns the client IP used by security-sensitive modules
// (logs, rate limiting, token allow_ips, risk control, region restriction).
// The detection mode is admin-configurable — see resolveClientIP.
func GetClientIP(c *gin.Context) string {
	ip, _, _ := resolveClientIP(c)
	return ip
}

// resolveClientIP derives the client IP according to the configured IPMode.
// source is "header" / "gin_client_ip" / "remote_addr"; headerName is set only
// when source is "header".
func resolveClientIP(c *gin.Context) (ip string, source string, headerName string) {
	if c == nil || c.Request == nil {
		return "", "", ""
	}
	cfg := operation_setting.GetRiskControlSetting()
	switch operation_setting.EffectiveIPMode(cfg) {
	case operation_setting.IPModeRemoteAddr:
		return parseRemoteAddrIP(c.Request.RemoteAddr), "remote_addr", ""
	case operation_setting.IPModeTrustedHeader:
		return resolveTrustedHeaderIP(c, cfg.TrustedIPHeader)
	case operation_setting.IPModeXFF:
		if ip := xffIPAtIndex(c.GetHeader("X-Forwarded-For"), cfg.XFFIndex); ip != "" {
			return ip, "header", "X-Forwarded-For"
		}
		return parseRemoteAddrIP(c.Request.RemoteAddr), "remote_addr", ""
	}
	return resolveAutoIP(c)
}

// resolveTrustedHeaderIP trusts only the single configured header (strict
// mode, prevents spoofing via other headers) and falls back to the TCP
// RemoteAddr when the header is absent or invalid.
func resolveTrustedHeaderIP(c *gin.Context, header string) (string, string, string) {
	header = strings.TrimSpace(header)
	if header != "" {
		if ip := extractHeaderIP(header, c.GetHeader(header)); ip != "" {
			return ip, "header", header
		}
	}
	return parseRemoteAddrIP(c.Request.RemoteAddr), "remote_addr", ""
}

// resolveAutoIP is the default heuristic: try all common proxy headers in
// priority order, use the first valid public IP found. This handles
// CDN/reverse-proxy chains (Cloudflare, nginx, etc.) without requiring
// explicit configuration.
func resolveAutoIP(c *gin.Context) (string, string, string) {
	for _, headerName := range diagnosisHeaderOrder {
		raw := c.GetHeader(headerName)
		if raw == "" {
			continue
		}
		ip := extractHeaderIP(headerName, raw)
		if ip == "" {
			continue
		}
		parsed := net.ParseIP(ip)
		if parsed != nil && !isPrivateOrSpecialIP(parsed) {
			return ip, "header", headerName
		}
	}
	if ip := c.ClientIP(); ip != "" {
		return ip, "gin_client_ip", ""
	}
	return parseRemoteAddrIP(c.Request.RemoteAddr), "remote_addr", ""
}

// parseXFFIPs returns every parseable IP in an X-Forwarded-For value, in
// original order (leftmost first).
func parseXFFIPs(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var ips []string
	for _, segment := range strings.Split(value, ",") {
		if ip := normalizeIPToken(segment); ip != "" {
			ips = append(ips, ip)
		}
	}
	return ips
}

// xffIPAtIndex picks one entry from an X-Forwarded-For value.
// index 0 = leftmost (first), -1 = last, -N = N-th from the end.
// Returns "" when the header is empty or the index is out of range so the
// caller can fall back to RemoteAddr.
func xffIPAtIndex(value string, index int) string {
	ips := parseXFFIPs(value)
	if len(ips) == 0 {
		return ""
	}
	if index < 0 {
		index = len(ips) + index
	}
	if index < 0 || index >= len(ips) {
		return ""
	}
	return ips[index]
}

// DiagnoseRequest inspects the current request and recommends how to derive the
// client IP for security-sensitive modules. It is intended for admin-facing
// diagnostics so deployment-specific proxy behavior can be verified safely.
func DiagnoseRequest(c *gin.Context) Diagnosis {
	diag := Diagnosis{
		CurrentMode:     "remote_addr",
		RecommendedMode: "remote_addr",
	}
	if c == nil || c.Request == nil {
		diag.RecommendationMessage = "当前请求上下文不可用，建议保持关闭信任上游 IP 头。"
		return diag
	}

	cfg := operation_setting.GetRiskControlSetting()
	if cfg != nil {
		diag.CurrentHeader = strings.TrimSpace(cfg.TrustedIPHeader)
		diag.CurrentXFFIndex = cfg.XFFIndex
		diag.CurrentMode = operation_setting.EffectiveIPMode(cfg)
	}
	diag.XFFIPs = parseXFFIPs(c.GetHeader("X-Forwarded-For"))
	diag.ModePreviews = buildModePreviews(c, cfg, diag.CurrentMode)

	items := make([]DiagnosisItem, 0, len(diagnosisHeaderOrder)+1)
	itemIndex := make(map[string]int, len(diagnosisHeaderOrder)+1)

	appendItem := func(name, source, rawValue string) {
		key := strings.ToLower(name)
		if _, exists := itemIndex[key]; exists {
			return
		}
		itemIndex[key] = len(items)
		items = append(items, buildDiagnosisItem(name, source, rawValue))
	}

	appendItem("RemoteAddr", "remote_addr", c.Request.RemoteAddr)
	if diag.CurrentHeader != "" {
		appendItem(diag.CurrentHeader, "header", c.GetHeader(diag.CurrentHeader))
	}
	for _, headerName := range diagnosisHeaderOrder {
		appendItem(headerName, "header", c.GetHeader(headerName))
	}
	for headerName, values := range c.Request.Header {
		if !isLikelyIPHeader(headerName) {
			continue
		}
		appendItem(headerName, "header", strings.Join(values, ", "))
	}

	// The effective IP must match what GetClientIP actually returns for this
	// request, whatever the configured mode is.
	effectiveIP, effectiveSource, effectiveHeader := resolveClientIP(c)
	diag.EffectiveClientIP = effectiveIP
	diag.EffectiveSource = effectiveSource
	markKey := ""
	switch effectiveSource {
	case "header":
		markKey = strings.ToLower(effectiveHeader)
	case "remote_addr":
		markKey = "remoteaddr"
	}
	if markKey != "" {
		if idx, ok := itemIndex[markKey]; ok {
			items[idx].IsCurrent = true
		}
	}

	for _, headerName := range recommendationHeaderOrder {
		idx, ok := itemIndex[strings.ToLower(headerName)]
		if !ok {
			continue
		}
		item := items[idx]
		if item.Classification == "public" {
			diag.RecommendedMode = "trusted_header"
			diag.RecommendedHeader = item.Name
			diag.RecommendationMessage = "检测到可信候选请求头 " + item.Name + "，其值可解析为公网 IP，建议开启“信任上游 IP 头”并使用该头。"
			diag.Items = items
			return diag
		}
	}

	if idx, ok := itemIndex["remoteaddr"]; ok && items[idx].Classification == "public" {
		diag.RecommendationMessage = "RemoteAddr 已直接表现为公网 IP，建议关闭“信任上游 IP 头”，直接使用 TCP RemoteAddr。"
	} else {
		diag.RecommendationMessage = "未检测到可靠的公网 IP 请求头，建议保持关闭“信任上游 IP 头”，并检查上游代理是否已正确覆盖写入真实客户端 IP。"
	}
	diag.Items = items
	return diag
}

// buildModePreviews resolves what IP each of the four detection modes would
// produce for the current request, so the admin UI can display them side by
// side and let the admin pick the mode whose result matches their real IP.
func buildModePreviews(c *gin.Context, cfg *operation_setting.RiskControlSetting, currentMode string) []ModePreview {
	remoteIP := parseRemoteAddrIP(c.Request.RemoteAddr)

	autoIP, autoSource, autoHeader := resolveAutoIP(c)

	trustedHeader := ""
	xffIndex := 0
	if cfg != nil {
		trustedHeader = strings.TrimSpace(cfg.TrustedIPHeader)
		xffIndex = cfg.XFFIndex
	}
	trustedIP, trustedSource, trustedHeaderUsed := resolveTrustedHeaderIP(c, trustedHeader)

	xffIP := xffIPAtIndex(c.GetHeader("X-Forwarded-For"), xffIndex)
	xffSource := "header:X-Forwarded-For"
	if xffIP == "" {
		xffIP = remoteIP
		xffSource = "remote_addr"
	}

	previews := []ModePreview{
		{Mode: operation_setting.IPModeAuto, IP: autoIP, Source: previewSource(autoSource, autoHeader)},
		{Mode: operation_setting.IPModeTrustedHeader, IP: trustedIP, Source: previewSource(trustedSource, trustedHeaderUsed)},
		{Mode: operation_setting.IPModeXFF, IP: xffIP, Source: xffSource},
		{Mode: operation_setting.IPModeRemoteAddr, IP: remoteIP, Source: "remote_addr"},
	}
	for i := range previews {
		previews[i].IsCurrent = previews[i].Mode == currentMode
	}
	return previews
}

func previewSource(source string, headerName string) string {
	if source == "header" && headerName != "" {
		return "header:" + headerName
	}
	return source
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

func extractHeaderIP(headerName string, value string) string {
	if strings.EqualFold(headerName, "Forwarded") {
		return parseForwardedHeaderIP(value)
	}
	return parseHeaderIP(value)
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

func parseForwardedHeaderIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, entry := range strings.Split(value, ",") {
		for _, part := range strings.Split(entry, ";") {
			part = strings.TrimSpace(part)
			if !strings.HasPrefix(strings.ToLower(part), "for=") {
				continue
			}
			if ip := normalizeIPToken(strings.TrimSpace(part[4:])); ip != "" {
				return ip
			}
		}
	}
	return ""
}

func buildDiagnosisItem(name string, source string, rawValue string) DiagnosisItem {
	item := DiagnosisItem{
		Name:           name,
		Source:         source,
		RawValue:       strings.TrimSpace(rawValue),
		Classification: "missing",
	}
	if item.RawValue == "" {
		return item
	}
	item.Present = true
	if source == "remote_addr" {
		item.ParsedIP = parseRemoteAddrIP(item.RawValue)
	} else {
		item.ParsedIP = extractHeaderIP(name, item.RawValue)
	}
	if item.ParsedIP == "" {
		item.Classification = "invalid"
		return item
	}
	item.Valid = true
	ip := net.ParseIP(item.ParsedIP)
	if ip == nil {
		item.Classification = "invalid"
		return item
	}
	switch {
	case isPrivateOrSpecialIP(ip):
		item.Classification = "private"
	default:
		item.Classification = "public"
	}
	return item
}

func isLikelyIPHeader(name string) bool {
	normalized := strings.ToLower(http.CanonicalHeaderKey(name))
	if slices.ContainsFunc(diagnosisHeaderOrder, func(candidate string) bool {
		return strings.EqualFold(candidate, normalized)
	}) {
		return true
	}
	return strings.Contains(normalized, "forwarded") || strings.Contains(normalized, "ip")
}

func isPrivateOrSpecialIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsPrivate() ||
		ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() ||
		ip.IsInterfaceLocalMulticast()
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
