package model

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var ErrInvalidLogCursor = errors.New("invalid log cursor")

type LogCursorPage struct {
	Items      []*Log `json:"items"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor"`
}

type clickHouseLogCursor struct {
	Version   int    `json:"v"`
	CreatedAt int64  `json:"t"`
	EventID   string `json:"e"`
}

func GetAllLogsCursor(ctx context.Context, logType int, start, end int64, modelName, username, tokenName string, limit, channel int, group, requestID, upstreamRequestID, cursor string) (*LogCursorPage, error) {
	query := buildAllLogsQuery(logType, start, end, modelName, username, tokenName, channel, group, requestID, upstreamRequestID)
	page, err := getClickHouseLogCursorPage(ctx, query, limit, cursor)
	if err != nil {
		return nil, err
	}
	resolveChannelNames(page.Items)
	assignDisplayLogIds(page.Items, 0)
	return page, nil
}

func GetUserLogsCursor(ctx context.Context, userID, logType int, start, end int64, modelName, tokenName string, limit int, group, requestID, upstreamRequestID, cursor string) (*LogCursorPage, error) {
	if userID <= 0 {
		return nil, ErrInvalidLogCursor
	}
	query := buildUserLogsQuery(userID, logType, start, end, modelName, tokenName, group, requestID, upstreamRequestID)
	page, err := getClickHouseLogCursorPage(ctx, query, limit, cursor)
	if err != nil {
		return nil, err
	}
	formatUserLogs(page.Items, 0)
	return page, nil
}

func getClickHouseLogCursorPage(ctx context.Context, query *gorm.DB, limit int, encoded string) (*LogCursorPage, error) {
	if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) || limit < 1 || limit > 10000 || len(encoded) > 512 {
		return nil, ErrInvalidLogCursor
	}
	if encoded != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, ErrInvalidLogCursor
		}
		var cursor clickHouseLogCursor
		if common.Unmarshal(decoded, &cursor) != nil || cursor.Version != 1 || cursor.CreatedAt < 0 || cursor.EventID == "" || len(cursor.EventID) > 128 {
			return nil, ErrInvalidLogCursor
		}
		query = query.Where("(ifNull(logs.created_at, 0), logs.event_id) < (?, ?)", cursor.CreatedAt, cursor.EventID)
	}
	page := &LogCursorPage{Items: make([]*Log, 0)}
	if err := query.WithContext(ctx).Order(clickHouseLogOrder("logs.")).Limit(limit + 1).Find(&page.Items).Error; err != nil {
		return nil, err
	}
	page.HasMore = len(page.Items) > limit
	if page.HasMore {
		page.Items = page.Items[:limit]
		last := page.Items[len(page.Items)-1]
		if last.EventID == "" {
			return nil, errors.New("ClickHouse log is missing its event key")
		}
		cursor, err := common.Marshal(clickHouseLogCursor{Version: 1, CreatedAt: last.CreatedAt, EventID: last.EventID})
		if err != nil {
			return nil, err
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString(cursor)
	}
	return page, nil
}
