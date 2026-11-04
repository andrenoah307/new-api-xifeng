package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogCacheTokenCounts(t *testing.T) {
	for _, tc := range []struct {
		name, other string
		read, write int64
	}{
		{"missing", "", 0, 0},
		{"invalid JSON", "{", 0, 0},
		{"no cache", `{}`, 0, 0},
		{"read only", `{"cache_tokens":120}`, 120, 0},
		{"generic write", `{"cache_tokens":12,"cache_creation_tokens":30}`, 12, 30},
		{"total includes buckets", `{"cache_creation_tokens":30,"cache_creation_tokens_5m":10,"cache_creation_tokens_1h":5}`, 0, 30},
		{"split only", `{"cache_creation_tokens_5m":10,"cache_creation_tokens_1h":5}`, 0, 15},
		{"split exceeds total", `{"cache_creation_tokens":10,"cache_creation_tokens_5m":8,"cache_creation_tokens_1h":7}`, 0, 15},
		{"legacy strings", `{"cache_tokens":"120","cache_creation_tokens":"30"}`, 120, 30},
		{"invalid field preserves valid field", `{"cache_tokens":{},"cache_creation_tokens":30}`, 0, 30},
		{"invalid counts", `{"cache_tokens":-1,"cache_creation_tokens":1.5,"cache_creation_tokens_5m":9007199254740992,"cache_creation_tokens_1h":true}`, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := Log{Other: tc.other}
			read, write := log.CacheTokenCounts()
			assert.Equal(t, tc.read, read)
			assert.Equal(t, tc.write, write)
		})
	}
}
