package common

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

var (
	inclusivePromptChannelIDsOnce sync.Once
	inclusivePromptChannelIDs     map[int]struct{}
)

func parseInclusivePromptChannelIDs(raw string) map[int]struct{} {
	channelIDs := make(map[int]struct{})
	for _, item := range strings.Split(raw, ",") {
		channelID, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || channelID <= 0 {
			continue
		}
		channelIDs[channelID] = struct{}{}
	}
	return channelIDs
}

// IsInclusivePromptChannel reports channels known to include cached tokens in upstream prompt_tokens.
// The cache fingerprint alone is not proof of inclusive semantics, so this stays allowlist-only; see docs/dev/68.
func IsInclusivePromptChannel(channelId int) bool {
	inclusivePromptChannelIDsOnce.Do(func() {
		// Package initialization runs before main loads .env through godotenv, so the snapshot must be lazy.
		inclusivePromptChannelIDs = parseInclusivePromptChannelIDs(GetEnvOrDefaultString("INCLUSIVE_PROMPT_CHANNEL_IDS", ""))
	})
	_, ok := inclusivePromptChannelIDs[channelId]
	return ok
}

func GetEnvOrDefault(env string, defaultValue int) int {
	if env == "" || os.Getenv(env) == "" {
		return defaultValue
	}
	num, err := strconv.Atoi(os.Getenv(env))
	if err != nil {
		SysError(fmt.Sprintf("failed to parse %s: %s, using default value: %d", env, err.Error(), defaultValue))
		return defaultValue
	}
	return num
}

func GetEnvOrDefaultInt64(env string, defaultValue int64) int64 {
	if env == "" || os.Getenv(env) == "" {
		return defaultValue
	}
	num, err := strconv.ParseInt(os.Getenv(env), 10, 64)
	if err != nil {
		SysError(fmt.Sprintf("failed to parse %s: %s, using default value: %d", env, err.Error(), defaultValue))
		return defaultValue
	}
	return num
}

func GetEnvOrDefaultString(env string, defaultValue string) string {
	if env == "" || os.Getenv(env) == "" {
		return defaultValue
	}
	return os.Getenv(env)
}

func GetEnvOrDefaultBool(env string, defaultValue bool) bool {
	if env == "" || os.Getenv(env) == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(os.Getenv(env))
	if err != nil {
		SysError(fmt.Sprintf("failed to parse %s: %s, using default value: %t", env, err.Error(), defaultValue))
		return defaultValue
	}
	return b
}
