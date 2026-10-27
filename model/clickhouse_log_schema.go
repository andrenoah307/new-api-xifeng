package model

import (
	"fmt"

	"gorm.io/gorm"
)

// Keep the source columns nullable and full width: historical SQL rows may not
// satisfy the narrower defaults used by current application writes.
var clickHouseLogColumns = []struct{ name, dataType, defaultSQL string }{
	{"id", "Int64", "0"},
	{"user_id", "Nullable(Int64)", "0"},
	{"created_at", "Nullable(Int64)", "0"},
	{"type", "Nullable(Int64)", "0"},
	{"content", "Nullable(String)", "''"},
	{"username", "Nullable(String)", "''"},
	{"token_name", "Nullable(String)", "''"},
	{"model_name", "Nullable(String)", "''"},
	{"quota", "Nullable(Int64)", "0"},
	{"prompt_tokens", "Nullable(Int64)", "0"},
	{"completion_tokens", "Nullable(Int64)", "0"},
	{"use_time", "Nullable(Int64)", "0"},
	{"is_stream", "Nullable(Int8)", "0"},
	{"channel_id", "Nullable(Int64)", "0"},
	{"channel_name", "Nullable(String)", "''"},
	{"token_id", "Nullable(Int64)", "0"},
	{"group", "Nullable(String)", "''"},
	{"ip", "Nullable(String)", "''"},
	{"other", "Nullable(String)", "''"},
	{"request_id", "Nullable(String)", "''"},
	{"session_id", "Nullable(String)", "''"},
	{"upstream_request_id", "Nullable(String)", "''"},
	{"event_id", "String", "if(id > 0, concat('legacy:', toString(id)), toString(generateUUIDv4()))"},
	{"migration_batch", "String", "''"},
	{"migration_source", "String", "''"},
}

func validateClickHouseLogSchema(db *gorm.DB) error {
	var columns []struct{ Name, Type string }
	if err := db.Raw("DESCRIBE TABLE logs").Scan(&columns).Error; err != nil {
		return err
	}
	types := make(map[string]string, len(columns))
	for _, column := range columns {
		types[column.Name] = column.Type
	}
	for _, column := range clickHouseLogColumns {
		if types[column.name] != column.dataType {
			return fmt.Errorf("ClickHouse logs schema incompatible at %s: expected %s; migrate to a new log table before switching (no automatic table rewrite)", column.name, column.dataType)
		}
	}
	var table struct{ Engine, SortingKey, PartitionKey string }
	if err := db.Raw("SELECT engine, sorting_key, partition_key FROM system.tables WHERE database = currentDatabase() AND name = 'logs'").Scan(&table).Error; err != nil {
		return err
	}
	if table.Engine != "MergeTree" || table.SortingKey != "ifNull(created_at, 0), event_id" || table.PartitionKey != "(toYYYYMM(toDateTime(ifNull(created_at, 0), 'UTC')), migration_batch)" {
		return fmt.Errorf("ClickHouse logs requires the batch-isolated MergeTree schema; migrate to a new log table before switching")
	}
	return nil
}
