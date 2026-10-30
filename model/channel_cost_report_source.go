package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
)

// ChannelCostReportSourceIdentity fences resumable reports against a replaced log source.
// Only the digest is persisted; connection credentials never leave this function.
func ChannelCostReportSourceIdentity(ctx context.Context) (string, error) {
	if LOG_DB == nil {
		return "", fmt.Errorf("log database unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	db := LOG_DB.WithContext(ctx)
	var dsn string
	var identity struct {
		DatabaseName string
		TableID      string
	}
	switch dialector := LOG_DB.Dialector.(type) {
	case *clickhouse.Dialector:
		dsn = dialector.DSN
		if err := db.Raw("SELECT currentDatabase() AS database_name, toString(uuid) AS table_id FROM system.tables WHERE database = currentDatabase() AND name = 'logs'").Scan(&identity).Error; err != nil {
			return "", err
		}
		if identity.TableID == "00000000-0000-0000-0000-000000000000" {
			return "", fmt.Errorf("log source has no stable table identity")
		}
	case *mysql.Dialector:
		dsn = dialector.DSN
		if err := db.Raw("SELECT DATABASE() AS database_name, CONCAT(TABLE_NAME, ':', COALESCE(CAST(CREATE_TIME AS CHAR), 'unknown')) AS table_id FROM information_schema.tables WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'logs'").Scan(&identity).Error; err != nil {
			return "", err
		}
	case *postgres.Dialector:
		dsn = dialector.DSN
		if err := db.Raw("SELECT current_database() AS database_name, CAST(to_regclass('logs')::oid AS text) AS table_id").Scan(&identity).Error; err != nil {
			return "", err
		}
	case *sqlite.Dialector:
		dsn = dialector.DSN
		var databases []struct {
			Name string
			File string
		}
		if err := db.Raw("PRAGMA database_list").Scan(&databases).Error; err != nil {
			return "", err
		}
		for _, database := range databases {
			if database.Name == "main" {
				identity.DatabaseName = database.File
			}
		}
		if identity.DatabaseName == "" {
			// Independent in-memory databases can have identical DSNs and schemas.
			// Their connection pool is the lifetime of the source data itself.
			pool, err := LOG_DB.DB()
			if err != nil {
				return "", err
			}
			identity.DatabaseName = fmt.Sprintf("memory:%p", pool)
		}
		var version int64
		if err := db.Raw("PRAGMA schema_version").Scan(&version).Error; err != nil {
			return "", err
		}
		var schema struct {
			Rootpage int64
			SQL      string
		}
		if err := db.Raw("SELECT rootpage, sql FROM sqlite_master WHERE type = 'table' AND name = 'logs'").Scan(&schema).Error; err != nil {
			return "", err
		}
		if schema.SQL != "" {
			identity.TableID = fmt.Sprintf("%d:%d:%s", version, schema.Rootpage, schema.SQL)
		}
	default:
		return "", fmt.Errorf("unsupported log database")
	}
	if identity.TableID == "" {
		return "", fmt.Errorf("log source identity unavailable")
	}
	data, err := common.Marshal([]string{LOG_DB.Dialector.Name(), dsn, identity.DatabaseName, identity.TableID})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}
