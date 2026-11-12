package common

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

const UpstreamErrorMessagesOption = "UpstreamErrorMessages"

type UpstreamErrorMessages struct {
	Client string `json:"client"`
	Server string `json:"server"`
}

var DefaultUpstreamErrorMessages = UpstreamErrorMessages{
	Client: "请求参数有误，请检查请求内容",
	Server: "上游服务暂不可用，请稍后重试",
}

var upstreamErrorMessages atomic.Pointer[UpstreamErrorMessages]

func GetUpstreamErrorMessages() UpstreamErrorMessages {
	if value := upstreamErrorMessages.Load(); value != nil {
		return *value
	}
	return DefaultUpstreamErrorMessages
}

func ParseUpstreamErrorMessages(value string) (UpstreamErrorMessages, error) {
	result := DefaultUpstreamErrorMessages
	var fields map[string]string
	if len(value) > 8192 || UnmarshalJsonStr(value, &fields) != nil || fields == nil {
		return result, errors.New("invalid error messages")
	}
	for key, text := range fields {
		text = strings.TrimSpace(text)
		if !utf8.ValidString(text) || utf8.RuneCountInString(text) > 300 || strings.Contains(text, "{{") || strings.Contains(text, "${") || strings.IndexFunc(text, unicode.IsControl) >= 0 {
			return result, errors.New("invalid error message text")
		}
		var target *string
		switch key {
		case "client":
			target = &result.Client
		case "server":
			target = &result.Server
		default:
			return result, errors.New("unknown error message category")
		}
		if text != "" {
			*target = text
		}
	}
	return result, nil
}

func UpdateUpstreamErrorMessages(value string) error {
	parsed, err := ParseUpstreamErrorMessages(value)
	if err != nil {
		return err
	}
	upstreamErrorMessages.Store(&parsed)
	return nil
}

func PublicErrorMessage(status int, requestID string) string {
	settings := GetUpstreamErrorMessages()
	message := settings.Server
	switch {
	case status >= 400 && status < 500:
		message = settings.Client
	}
	if requestID != "" {
		return MessageWithRequestId(message, requestID)
	}
	return message
}

// Match operational identifiers and credentials, not ordinary token counts or
// the word "group" when it is merely a rejected request field.
var sensitiveErrorPattern = regexp.MustCompile(`(?i)(分组|渠道|供应商|供应方|账号|账户|密钥|凭证|\b(?:groups?|channels?|providers?|tenants?|organizations?|accounts?)(?:[_ -](?:id|name))?\b|\bapi[_ -]?key\b|\bauthorization\b|\bbearer\s+|\b(?:access|refresh)[_ -]?token\b|\b(?:password|secret|credentials?)\b|\bsk-[a-z0-9_-]{8,}|\bAIza[a-z0-9_-]{10,}|https?://|\b[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}\b)`)

var rejectedGroupFieldPattern = regexp.MustCompile(`^(?:status_code=\d+,\s*)?group: Extra inputs are not permitted$`)

func HasSensitiveErrorDetails(message string) bool {
	if rejectedGroupFieldPattern.MatchString(strings.TrimSpace(message)) {
		return false
	}
	return sensitiveErrorPattern.MatchString(message) || maskIPPattern.MatchString(message)
}

func RedactPublicErrorMessage(message string, status int, requestID string) (string, bool) {
	if !HasSensitiveErrorDetails(message) {
		return message, false
	}
	return PublicErrorMessage(status, requestID), true
}

// JSON is decoded only at output boundaries. RawMessage keeps unrelated usage,
// sequence numbers and successful content unchanged, including large integers.
func RedactPublicErrorJSON(data []byte, status int, requestID string) ([]byte, bool) {
	var root map[string]json.RawMessage
	if Unmarshal(data, &root) != nil || root == nil {
		return data, false
	}
	changed := false
	if raw := root["response"]; len(raw) > 0 {
		if replacement, ok := RedactPublicErrorJSON(raw, status, requestID); ok {
			root["response"] = replacement
			for key, value := range root {
				if key == "response" {
					continue
				}
				if (key == "type" || key == "sequence_number" || key == "event_id") && !errorJSONSensitive(value) {
					continue
				}
				delete(root, key)
			}
			changed = true
		}
	}
	var eventType string
	_ = Unmarshal(root["type"], &eventType)
	raw, hasError := root["error"]
	if hasError && string(raw) != "null" {
		if errorJSONSensitive(data) {
			root["error"] = publicErrorObject(raw, status, requestID)
			// Keep protocol fields only; arbitrary sibling diagnostics can repeat secrets.
			for key, value := range root {
				switch key {
				case "error":
					continue
				case "type", "id", "object", "sequence_number", "event_id", "usage", "status", "created_at", "output", "model":
					if !errorJSONSensitive(value) {
						continue
					}
				}
				delete(root, key)
			}
			changed = true
		}
	} else if eventType == "error" && errorJSONSensitive(data) {
		clean := publicErrorObject(data, status, requestID)
		var fields map[string]json.RawMessage
		_ = Unmarshal(clean, &fields)
		for key := range root {
			if (key != "type" && key != "sequence_number" && key != "event_id") || errorJSONSensitive(root[key]) {
				delete(root, key)
			}
		}
		root["message"], root["code"] = fields["message"], fields["code"]
		changed = true
	}
	if !changed {
		return data, false
	}
	result, err := Marshal(root)
	if err != nil {
		return data, false
	}
	return result, true
}

func errorJSONSensitive(raw json.RawMessage) bool {
	var message string
	if Unmarshal(raw, &message) == nil {
		return HasSensitiveErrorDetails(message)
	}
	var fields map[string]json.RawMessage
	if Unmarshal(raw, &fields) == nil && fields != nil {
		for key, value := range fields {
			switch strings.ToLower(key) {
			case "key", "token", "endpoint", "base_url", "upstream", "cookie", "set-cookie":
				return true
			}
			if HasSensitiveErrorDetails(key) || errorJSONSensitive(value) {
				return true
			}
		}
		return false
	}
	var items []json.RawMessage
	if Unmarshal(raw, &items) == nil {
		for _, item := range items {
			if errorJSONSensitive(item) {
				return true
			}
		}
	}
	return false
}

func publicErrorObject(raw json.RawMessage, status int, requestID string) json.RawMessage {
	var fields map[string]json.RawMessage
	_ = Unmarshal(raw, &fields)
	if status < 400 || status > 599 {
		for _, key := range []string{"status_code", "status"} {
			var value int
			if Unmarshal(fields[key], &value) == nil && value >= 400 && value <= 599 {
				status = value
				break
			}
		}
	}
	code := "upstream_error"
	var upstreamCode string
	_ = Unmarshal(fields["code"], &upstreamCode)
	// Only public protocol codes may survive replacement of sensitive details.
	switch upstreamCode {
	case "model_not_found", "invalid_request_error", "rate_limit_exceeded", "context_length_exceeded", "content_policy_violation", "insufficient_quota", "channel:rate_limited":
		code = upstreamCode
	}
	result, _ := Marshal(map[string]string{"message": PublicErrorMessage(status, requestID), "type": "upstream_error", "code": code})
	return result
}
