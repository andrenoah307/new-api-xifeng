package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// RewriteStreamOverloadErrorCode rewrites fatal upstream overload codes into
// the retryable server_error code while preserving the rest of the payload.
func RewriteStreamOverloadErrorCode(data string) (patched string, originalCode string, changed bool) {
	if data == "" || !gjson.Valid(data) {
		return data, "", false
	}

	patched = data
	for _, path := range [...]string{"response.error.code", "error.code"} {
		code := gjson.Get(data, path)
		if !code.Exists() {
			continue
		}
		normalizedCode := strings.ToLower(strings.TrimSpace(code.String()))
		if normalizedCode != "server_is_overloaded" && normalizedCode != "slow_down" {
			continue
		}

		if originalCode == "" {
			originalCode = code.String()
		}
		next, err := sjson.Set(patched, path, "server_error")
		if err != nil {
			return data, originalCode, false
		}
		patched = next
		changed = true
	}

	return patched, originalCode, changed
}
