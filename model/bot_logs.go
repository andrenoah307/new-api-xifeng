package model

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type BotLogFilter struct {
	Type      int    `json:"type"`
	Start     int64  `json:"start_timestamp"`
	End       int64  `json:"end_timestamp"`
	Username  string `json:"username"`
	ModelName string `json:"model_name"`
	TokenName string `json:"token_name"`
	Group     string `json:"group"`
	RequestID string `json:"request_id"`
	PageSize  int    `json:"page_size"`
}

// BotUsageLog deliberately cannot carry Other, internal routing, or raw errors.
type BotUsageLog struct {
	UserId           int    `json:"user_id"`
	Username         string `json:"username"`
	CreatedAt        int64  `json:"created_at"`
	Type             int    `json:"type"`
	ModelName        string `json:"model_name"`
	Quota            int    `json:"quota"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	UseTime          int    `json:"use_time"`
	IsStream         bool   `json:"is_stream"`
	RequestId        string `json:"request_id"`
	Content          string `json:"content"`
}

type BotLogPage struct {
	Items      []BotUsageLog `json:"items"`
	HasMore    bool          `json:"has_more"`
	NextCursor string        `json:"next_cursor"`
}

type botLogCursor struct {
	Version   int    `json:"v"`
	ID        int    `json:"id"`
	CreatedAt int64  `json:"t"`
	EventID   string `json:"e"`
	Start     int64  `json:"s"`
	End       int64  `json:"u"`
	Expires   int64  `json:"x"`
}

// GetBotLogs authorizes bounded candidate batches before assembling the public
// page. This works with separate user/log databases without a cross-DB join or
// materializing every ordinary user ID. No unfiltered counts are exposed.
func GetBotLogs(ctx context.Context, actorID int, filter BotLogFilter, encoded string) (*BotLogPage, error) {
	if actorID <= 0 || filter.PageSize < 1 || filter.PageSize > 100 || filter.Start < 0 || filter.End < 0 || len(encoded) > 2048 ||
		(filter.Type != 0 && filter.Type != LogTypeConsume && filter.Type != LogTypeError) {
		return nil, ErrInvalidLogCursor
	}
	for _, value := range []string{filter.Username, filter.ModelName, filter.TokenName, filter.Group, filter.RequestID} {
		if len(value) > 256 {
			return nil, ErrInvalidLogCursor
		}
	}
	for _, value := range []string{filter.Username, filter.ModelName} {
		if err := validateLikePattern(value); err != nil {
			return nil, ErrInvalidLogCursor
		}
	}
	filterJSON, err := common.Marshal(filter)
	if err != nil {
		return nil, err
	}
	// Authenticated encryption binds the cursor to its caller, filters and storage,
	// and conceals scan positions that may belong to non-public records.
	aad := []byte(fmt.Sprintf("bot-logs:v1:%d:%s:%s", actorID, common.LogDatabaseType(), filterJSON))
	key := sha256.Sum256([]byte("bot-log-cursor:" + common.CryptoSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	position := botLogCursor{Version: 1, Start: filter.Start, End: filter.End, Expires: now + 3600}
	if encoded != "" {
		payload, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || len(payload) < aead.NonceSize()+aead.Overhead() {
			return nil, ErrInvalidLogCursor
		}
		plain, err := aead.Open(nil, payload[:aead.NonceSize()], payload[aead.NonceSize():], aad)
		if err != nil || common.Unmarshal(plain, &position) != nil || position.Version != 1 || position.Expires <= now {
			return nil, ErrInvalidLogCursor
		}
	} else {
		if position.End == 0 {
			position.End = now
		}
		if position.Start == 0 {
			position.Start = position.End - 30*86400
		}
	}
	if position.Start < 0 || position.End < position.Start || position.End-position.Start > 31*86400 || position.End > now {
		return nil, ErrInvalidLogCursor
	}
	page := &BotLogPage{Items: make([]BotUsageLog, 0, filter.PageSize)}
	clickHouse := common.UsingLogDatabase(common.DatabaseTypeClickHouse)
	// Fetch only fields that can be projected safely; never fetch admin_info.
	columns := "id, user_id, username, created_at, type, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream, request_id"
	if clickHouse {
		columns += ", event_id"
	}
	const batchSize = 250
	const maxCandidates = 5000
	for scanned := 0; scanned < maxCandidates; scanned += batchSize {
		query := buildAllLogsQuery(filter.Type, position.Start, position.End, filter.ModelName, filter.Username, filter.TokenName, 0, filter.Group, filter.RequestID, "").
			WithContext(ctx).Model(&Log{}).Select(columns).Where("logs.type IN ?", []int{LogTypeConsume, LogTypeError})
		if clickHouse {
			query = query.Order(clickHouseLogOrder("logs."))
			if position.EventID != "" {
				query = query.Where("(ifNull(logs.created_at, 0), logs.event_id) < (?, ?)", position.CreatedAt, position.EventID)
			}
		} else {
			query = query.Order("logs.id desc")
			if position.ID > 0 {
				query = query.Where("logs.id < ?", position.ID)
			}
		}
		var rows []*Log
		if err := query.Limit(batchSize).Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return page, nil
		}
		ids := make([]int, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.UserId)
		}
		var allowedIDs []int
		if err := DB.WithContext(ctx).Model(&User{}).Where("id IN ? AND role = ?", ids, common.RoleCommonUser).Pluck("id", &allowedIDs).Error; err != nil {
			return nil, err
		}
		allowed := make(map[int]bool, len(allowedIDs))
		for _, id := range allowedIDs {
			allowed[id] = true
		}
		for _, row := range rows {
			if allowed[row.UserId] && len(page.Items) == filter.PageSize {
				page.HasMore = true
				break
			}
			if clickHouse && row.EventID == "" {
				return nil, ErrInvalidLogCursor
			}
			position.ID, position.CreatedAt, position.EventID = row.Id, row.CreatedAt, row.EventID
			if !allowed[row.UserId] {
				continue
			}
			item := BotUsageLog{UserId: row.UserId, Username: row.Username, CreatedAt: row.CreatedAt, Type: row.Type,
				ModelName: row.ModelName, Quota: row.Quota, PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens,
				UseTime: row.UseTime, IsStream: row.IsStream, RequestId: row.RequestId}
			if row.Type == LogTypeError {
				item.Content = common.PublicErrorMessage(0, row.RequestId)
			}
			page.Items = append(page.Items, item)
		}
		if page.HasMore {
			break
		}
		if len(rows) < batchSize {
			return page, nil
		}
		if scanned+batchSize == maxCandidates {
			page.HasMore = true
		}
	}
	plain, err := common.Marshal(position)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	page.NextCursor = base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, plain, aad))
	return page, nil
}
