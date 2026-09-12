package setting

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	goahocorasick "github.com/anknown/ahocorasick"
	"golang.org/x/net/idna"
)

const (
	RequestBlacklistModeOff     = "off"
	RequestBlacklistModeObserve = "observe"
	RequestBlacklistModeEnforce = "enforce"

	requestBlacklistMaxContentWords = 5000
	requestBlacklistMaxDomains      = 5000
	requestBlacklistMaxGroups       = 200
	requestBlacklistMaxWordRunes    = 128
	requestBlacklistMaxDomainBytes  = 253
	requestBlacklistMaxMessageRunes = 500

	defaultRequestBlacklistMessage = "请求内容不符合网关策略，已被拦截"

	requestBlacklistPatternContent uint8 = 1
	requestBlacklistPatternDomain  uint8 = 2
)

type RequestBlacklistConfig struct {
	Mode            string   `json:"mode"`
	EnabledGroups   []string `json:"enabled_groups,omitempty"`
	ContentWords    []string `json:"content_words,omitempty"`
	Domains         []string `json:"domains,omitempty"`
	BlockMessage    string   `json:"block_message,omitempty"`
	BlockStatusCode int      `json:"block_status_code,omitempty"`
}

// RequestBlacklistSnapshot is compiled completely before being atomically
// published. Its maps, slices, and AC machine are immutable after publication.
type RequestBlacklistSnapshot struct {
	config            RequestBlacklistConfig
	enabledGroups     map[string]struct{}
	machine           *goahocorasick.Machine
	patternKinds      map[string]uint8
	maxPatternRunes   int
	hasDomainPatterns bool
	configHash        string
}

var requestBlacklistSnapshot atomic.Pointer[RequestBlacklistSnapshot]

func init() {
	snapshot, err := parseRequestBlacklist("")
	if err != nil {
		panic("failed to initialize request blacklist: " + err.Error())
	}
	requestBlacklistSnapshot.Store(snapshot)
}

func RequestBlacklist2JSONString() string {
	snapshot := GetRequestBlacklistSnapshot()
	jsonBytes, err := common.Marshal(snapshot.config)
	if err != nil {
		common.SysLog("error marshalling request blacklist: " + err.Error())
		return ""
	}
	return string(jsonBytes)
}

func UpdateRequestBlacklistByJSONString(jsonStr string) error {
	snapshot, err := parseRequestBlacklist(jsonStr)
	if err != nil {
		return err
	}
	requestBlacklistSnapshot.Store(snapshot)
	return nil
}

func CheckRequestBlacklist(jsonStr string) error {
	_, err := parseRequestBlacklist(jsonStr)
	return err
}

func GetRequestBlacklistSnapshot() *RequestBlacklistSnapshot {
	snapshot := requestBlacklistSnapshot.Load()
	if snapshot != nil {
		return snapshot
	}
	snapshot, _ = parseRequestBlacklist("")
	return snapshot
}

func (s *RequestBlacklistSnapshot) Mode() string {
	if s == nil {
		return RequestBlacklistModeOff
	}
	return s.config.Mode
}

func (s *RequestBlacklistSnapshot) ShouldInspectGroup(group string) bool {
	if s == nil || s.config.Mode == RequestBlacklistModeOff || s.machine == nil {
		return false
	}
	if len(s.enabledGroups) == 0 {
		return true
	}
	_, ok := s.enabledGroups[group]
	return ok
}

func (s *RequestBlacklistSnapshot) BlockMessage() string {
	if s == nil || s.config.BlockMessage == "" {
		return defaultRequestBlacklistMessage
	}
	return s.config.BlockMessage
}

func (s *RequestBlacklistSnapshot) BlockStatusCode() int {
	if s == nil || s.config.BlockStatusCode == 0 {
		return 403
	}
	return s.config.BlockStatusCode
}

func (s *RequestBlacklistSnapshot) ConfigHash() string {
	if s == nil {
		return ""
	}
	return s.configHash
}

func (s *RequestBlacklistSnapshot) Machine() *goahocorasick.Machine {
	if s == nil {
		return nil
	}
	return s.machine
}

func (s *RequestBlacklistSnapshot) MaxPatternRunes() int {
	if s == nil {
		return 0
	}
	return s.maxPatternRunes
}

func (s *RequestBlacklistSnapshot) HasDomainPatterns() bool {
	return s != nil && s.hasDomainPatterns
}

func (s *RequestBlacklistSnapshot) PatternKinds(pattern []rune) (content, domain bool) {
	if s == nil {
		return false, false
	}
	kinds := s.patternKinds[string(pattern)]
	return kinds&requestBlacklistPatternContent != 0, kinds&requestBlacklistPatternDomain != 0
}

func parseRequestBlacklist(jsonStr string) (*RequestBlacklistSnapshot, error) {
	config := RequestBlacklistConfig{}
	trimmed := strings.TrimSpace(jsonStr)
	if trimmed != "" {
		if err := decodeRequestBlacklistDocument(trimmed, &config); err != nil {
			return nil, err
		}
	}

	if err := normalizeRequestBlacklistConfig(&config); err != nil {
		return nil, err
	}
	normalizedJSON, err := common.Marshal(config)
	if err != nil {
		return nil, err
	}
	var roundTrip RequestBlacklistConfig
	if err = common.Unmarshal(normalizedJSON, &roundTrip); err != nil {
		return nil, err
	}
	if config.Mode != roundTrip.Mode ||
		config.BlockMessage != roundTrip.BlockMessage ||
		config.BlockStatusCode != roundTrip.BlockStatusCode ||
		!slices.Equal(config.EnabledGroups, roundTrip.EnabledGroups) ||
		!slices.Equal(config.ContentWords, roundTrip.ContentWords) ||
		!slices.Equal(config.Domains, roundTrip.Domains) {
		return nil, fmt.Errorf("request blacklist cannot be losslessly normalized")
	}
	return buildRequestBlacklistSnapshot(config)
}

func decodeRequestBlacklistDocument(jsonStr string, config *RequestBlacklistConfig) error {
	rawDocument := json.RawMessage(jsonStr)
	if common.GetJsonType(rawDocument) != "object" {
		return fmt.Errorf("request blacklist must be a JSON object")
	}

	var fields map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(jsonStr, &fields); err != nil {
		return err
	}
	expectedTypes := map[string]string{
		"mode":              "string",
		"enabled_groups":    "array",
		"content_words":     "array",
		"domains":           "array",
		"block_message":     "string",
		"block_status_code": "number",
	}
	for key, rawValue := range fields {
		expectedType, ok := expectedTypes[key]
		if !ok {
			return fmt.Errorf("unknown request blacklist field %q", key)
		}
		if actualType := common.GetJsonType(rawValue); actualType != expectedType {
			return fmt.Errorf("request blacklist field %q must be %s, got %s", key, expectedType, actualType)
		}
	}
	if err := common.UnmarshalJsonStr(jsonStr, config); err != nil {
		return err
	}
	return nil
}

func normalizeRequestBlacklistConfig(config *RequestBlacklistConfig) error {
	if config.Mode == "" {
		config.Mode = RequestBlacklistModeOff
	}
	switch config.Mode {
	case RequestBlacklistModeOff, RequestBlacklistModeObserve, RequestBlacklistModeEnforce:
	default:
		return fmt.Errorf("request blacklist mode must be off, observe, or enforce")
	}

	switch config.BlockStatusCode {
	case 0:
		config.BlockStatusCode = 403
	case 400, 403, 503:
	default:
		return fmt.Errorf("request blacklist status code must be 400, 403, or 503")
	}

	normalizedGroups, err := normalizeRequestBlacklistGroups(config.EnabledGroups)
	if err != nil {
		return err
	}
	config.EnabledGroups = normalizedGroups

	normalizedWords, err := normalizeRequestBlacklistWords(config.ContentWords)
	if err != nil {
		return err
	}
	config.ContentWords = normalizedWords

	normalizedDomains, err := normalizeRequestBlacklistDomains(config.Domains)
	if err != nil {
		return err
	}
	config.Domains = normalizedDomains

	config.BlockMessage = strings.TrimSpace(config.BlockMessage)
	if utf8.RuneCountInString(config.BlockMessage) > requestBlacklistMaxMessageRunes {
		return fmt.Errorf("request blacklist block message must not exceed %d characters", requestBlacklistMaxMessageRunes)
	}
	if config.BlockMessage == "" {
		config.BlockMessage = defaultRequestBlacklistMessage
	}
	if common.MaskSensitiveInfo(config.BlockMessage) != config.BlockMessage {
		return fmt.Errorf("request blacklist block message must not contain URLs, domains, IP addresses, or API keys")
	}
	return nil
}

func normalizeRequestBlacklistGroups(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) > requestBlacklistMaxGroups {
		return nil, fmt.Errorf("request blacklist enabled groups must not exceed %d entries", requestBlacklistMaxGroups)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeRequestBlacklistWords(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.Map(unicode.ToLower, strings.TrimSpace(value))
		if value == "" {
			continue
		}
		length := utf8.RuneCountInString(value)
		if length < 1 || length > requestBlacklistMaxWordRunes {
			return nil, fmt.Errorf("request blacklist content words must contain 1 to %d characters", requestBlacklistMaxWordRunes)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) > requestBlacklistMaxContentWords {
		return nil, fmt.Errorf("request blacklist content words must not exceed %d entries", requestBlacklistMaxContentWords)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeRequestBlacklistDomains(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		domain, err := normalizeRequestBlacklistDomain(value)
		if err != nil {
			return nil, err
		}
		if domain == "" {
			continue
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		normalized = append(normalized, domain)
	}
	if len(normalized) > requestBlacklistMaxDomains {
		return nil, fmt.Errorf("request blacklist domains must not exceed %d entries", requestBlacklistMaxDomains)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeRequestBlacklistDomain(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("request blacklist domain must not be empty")
	}
	if strings.Contains(value, "://") || strings.ContainsAny(value, "/?#*") {
		return "", fmt.Errorf("request blacklist domain %q must be a host name without scheme, path, query, fragment, or wildcard", value)
	}
	for _, r := range value {
		if unicode.IsSpace(r) {
			return "", fmt.Errorf("request blacklist domain %q must not contain spaces", value)
		}
	}

	if strings.Contains(value, ":") {
		host, port, err := net.SplitHostPort(value)
		if err != nil || host == "" || port == "" {
			return "", fmt.Errorf("request blacklist domain %q has an invalid port", value)
		}
		portNumber, err := strconv.ParseUint(port, 10, 16)
		if err != nil || portNumber == 0 {
			return "", fmt.Errorf("request blacklist domain %q has an invalid port", value)
		}
		value = host
	}
	value = strings.TrimSuffix(strings.ToLower(value), ".")
	if value == "" {
		return "", fmt.Errorf("request blacklist domain must not be empty")
	}

	// Unicode hosts are stored as punycode; request text is not IDNA-normalized during scanning.
	ascii, err := idna.Lookup.ToASCII(value)
	if err != nil {
		return "", fmt.Errorf("invalid request blacklist domain %q: %w", value, err)
	}
	ascii = strings.ToLower(strings.TrimSuffix(ascii, "."))
	if ascii == "" {
		return "", fmt.Errorf("request blacklist domain must not be empty")
	}
	if len(ascii) > requestBlacklistMaxDomainBytes {
		return "", fmt.Errorf("request blacklist domain %q must not exceed %d bytes", ascii, requestBlacklistMaxDomainBytes)
	}
	for _, label := range strings.Split(ascii, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid request blacklist domain %q", ascii)
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return "", fmt.Errorf("invalid request blacklist domain %q", ascii)
		}
	}
	return ascii, nil
}

func buildRequestBlacklistSnapshot(config RequestBlacklistConfig) (*RequestBlacklistSnapshot, error) {
	enabledGroups := make(map[string]struct{}, len(config.EnabledGroups))
	for _, group := range config.EnabledGroups {
		enabledGroups[group] = struct{}{}
	}

	patternKinds := make(map[string]uint8, len(config.ContentWords)+2*len(config.Domains))
	for _, word := range config.ContentWords {
		patternKinds[word] |= requestBlacklistPatternContent
	}
	for _, domain := range config.Domains {
		patternKinds[domain] |= requestBlacklistPatternDomain
		patternKinds["."+domain] |= requestBlacklistPatternDomain
	}

	patterns := make([]string, 0, len(patternKinds))
	maxPatternRunes := 0
	for pattern := range patternKinds {
		patterns = append(patterns, pattern)
		if length := utf8.RuneCountInString(pattern); length > maxPatternRunes {
			maxPatternRunes = length
		}
	}
	sort.Strings(patterns)

	machine, err := buildRequestBlacklistMachine(patterns)
	if err != nil {
		return nil, err
	}
	configJSON, err := common.Marshal(config)
	if err != nil {
		return nil, err
	}
	configHash := fmt.Sprintf("%x", sha256.Sum256(configJSON))
	return &RequestBlacklistSnapshot{
		config:            config,
		enabledGroups:     enabledGroups,
		machine:           machine,
		patternKinds:      patternKinds,
		maxPatternRunes:   maxPatternRunes,
		hasDomainPatterns: len(config.Domains) > 0,
		configHash:        configHash,
	}, nil
}

func buildRequestBlacklistMachine(patterns []string) (machine *goahocorasick.Machine, err error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			machine = nil
			err = fmt.Errorf("failed to build request blacklist matcher: %v", recovered)
		}
	}()

	keywords := make([][]rune, 0, len(patterns))
	for _, pattern := range patterns {
		keywords = append(keywords, []rune(pattern))
	}
	machine = new(goahocorasick.Machine)
	if err = machine.Build(keywords); err != nil {
		return nil, fmt.Errorf("failed to build request blacklist matcher: %w", err)
	}
	return machine, nil
}
