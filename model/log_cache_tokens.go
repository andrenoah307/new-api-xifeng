package model

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// CacheTokenCounts follows the usage-log display: creation includes split buckets,
// while split-only historical records still contribute their full write count.
func (log *Log) CacheTokenCounts() (read, write int64) {
	var other map[string]json.RawMessage
	if common.UnmarshalJsonStr(log.Other, &other) != nil {
		return 0, 0
	}
	var counts [4]int64
	for i, key := range []string{"cache_tokens", "cache_creation_tokens", "cache_creation_tokens_5m", "cache_creation_tokens_1h"} {
		var number json.Number
		if common.Unmarshal(other[key], &number) != nil {
			continue
		}
		value, err := number.Int64()
		// Match the UI's safe integer range; the sum of two buckets fits int64.
		if err == nil && value > 0 && value <= 9007199254740991 {
			counts[i] = value
		}
	}
	return counts[0], max(counts[1], counts[2]+counts[3])
}
