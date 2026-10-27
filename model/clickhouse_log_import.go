package model

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/proto"
	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ClickHouseLogArchiveRow preserves every source column, including SQL NULLs.
// Archives must contain all 22 keys and integer JSON numbers, not float casts.
type ClickHouseLogArchiveRow struct {
	ID                int64   `json:"id"`
	UserID            *int64  `json:"user_id"`
	CreatedAt         *int64  `json:"created_at" gorm:"autoCreateTime:false"`
	Type              *int64  `json:"type"`
	Content           *string `json:"content"`
	Username          *string `json:"username"`
	TokenName         *string `json:"token_name"`
	ModelName         *string `json:"model_name"`
	Quota             *int64  `json:"quota"`
	PromptTokens      *int64  `json:"prompt_tokens"`
	CompletionTokens  *int64  `json:"completion_tokens"`
	UseTime           *int64  `json:"use_time"`
	IsStream          *int8   `json:"is_stream"`
	ChannelID         *int64  `json:"channel_id"`
	ChannelName       *string `json:"channel_name"`
	TokenID           *int64  `json:"token_id"`
	Group             *string `json:"group"`
	IP                *string `json:"ip"`
	Other             *string `json:"other"`
	RequestID         *string `json:"request_id"`
	SessionID         *string `json:"session_id"`
	UpstreamRequestID *string `json:"upstream_request_id"`
}

// Each immutable manifest selects source IDs in one UTC month. A 60-day
// operation consists of multiple independently verified, non-overlapping batches.
type ClickHouseLogImportManifest struct {
	Source    string `json:"source"`
	AfterID   int64  `json:"after_id"`
	ThroughID int64  `json:"through_id"`
	Month     uint32 `json:"month"`
	Rows      int64  `json:"rows"`
	SHA256    string `json:"sha256"`
}

type ClickHouseLogImportResult struct {
	BatchID          string `json:"batch_id"`
	Rows             int64  `json:"rows"`
	AlreadyPublished bool   `json:"already_published"`
}

type ClickHouseLogImportOptions struct {
	InsertRows  int
	InsertBytes int
}

// ImportClickHouseLogs never connects to or deletes from the source database.
// Publication replaces one isolated partition atomically; retrying an identical
// manifest verifies the committed rows instead of inserting them a second time.
func ImportClickHouseLogs(ctx context.Context, db *gorm.DB, manifest ClickHouseLogImportManifest, input io.Reader) (result ClickHouseLogImportResult, err error) {
	return ImportClickHouseLogsWithOptions(ctx, db, manifest, input, ClickHouseLogImportOptions{1000, 4 * 1024 * 1024})
}

func ImportClickHouseLogsWithOptions(ctx context.Context, db *gorm.DB, manifest ClickHouseLogImportManifest, input io.Reader, options ClickHouseLogImportOptions) (result ClickHouseLogImportResult, err error) {
	if options.InsertRows < 1 || options.InsertRows > 100000 || options.InsertBytes < 1024 || options.InsertBytes > 64*1024*1024 {
		return result, errors.New("invalid import insert limits")
	}
	ctx = chdriver.Context(ctx, chdriver.WithSettings(chdriver.Settings{
		"async_insert": 0, "wait_for_async_insert": 1, "max_threads": 2,
		"max_memory_usage": 2147483648, "max_bytes_before_external_sort": 536870912,
	}))
	digest, decodeErr := hex.DecodeString(manifest.SHA256)
	if manifest.Source == "" || len(manifest.Source) > 128 || manifest.AfterID < 0 || manifest.ThroughID <= manifest.AfterID || manifest.Rows < 1 || manifest.Rows > 1000000 || decodeErr != nil || len(digest) != sha256.Size || manifest.Month < 197001 || manifest.Month > 210602 || manifest.Month%100 < 1 || manifest.Month%100 > 12 {
		return result, errors.New("invalid ClickHouse import manifest")
	}
	if err := validateClickHouseLogSchema(db.WithContext(ctx)); err != nil {
		return result, err
	}
	var ddl string
	if err := db.WithContext(ctx).Raw("SHOW CREATE TABLE logs").Scan(&ddl).Error; err != nil {
		return result, err
	}
	if clickHouseCreateTableHasTTL(ddl) {
		return result, errors.New("disable log TTL before importing historical records")
	}
	key := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%d", manifest.Source, manifest.AfterID, manifest.ThroughID, manifest.Month)))
	result.BatchID = hex.EncodeToString(key[:])
	batchLock := "logs_import_lock_" + result.BatchID
	// A crashed owner leaves its gate for inspection, never timer-based stealing.
	if err := db.WithContext(ctx).Exec("CREATE TABLE " + batchLock + " (id UInt8) ENGINE = Memory").Error; err != nil {
		return result, fmt.Errorf("cannot acquire log import lock: %w", err)
	}
	publicationLocked := false
	stage := "logs_import_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	defer func() {
		if ctx.Err() != nil {
			// A canceled client cannot establish whether its last server-side
			// operation finished. Retain the gate until that is checked.
			err = errors.Join(err, fmt.Errorf("import canceled; inspect %s, %s and logs_import_lock before recovery", stage, batchLock))
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cleanupDB := db.WithContext(cleanupCtx)
		if cleanupErr := cleanupDB.Exec("DROP TABLE IF EXISTS " + stage + " SYNC").Error; cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
			return
		}
		if publicationLocked {
			if cleanupErr := cleanupDB.Exec("DROP TABLE logs_import_lock SYNC").Error; cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
				return
			}
		}
		err = errors.Join(err, cleanupDB.Exec("DROP TABLE "+batchLock+" SYNC").Error)
	}()
	if err := db.WithContext(ctx).Exec("CREATE TABLE " + stage + " AS logs").Error; err != nil {
		return result, err
	}
	type importedRow struct {
		ClickHouseLogArchiveRow `gorm:"embedded"`
		EventID                 string
		MigrationBatch          string
		MigrationSource         string
	}
	fileHash, sourceHash := sha256.New(), sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(input, fileHash))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	batch := make([]importedRow, 0, options.InsertRows)
	batchBytes := 0
	lastID := manifest.AfterID
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		line := scanner.Bytes()
		var fields map[string]json.RawMessage
		if err := common.Unmarshal(line, &fields); err != nil {
			return result, fmt.Errorf("invalid archive JSON: %w", err)
		}
		if len(fields) != 22 {
			return result, errors.New("archive must include exactly 22 source columns")
		}
		for _, column := range clickHouseLogColumns[:22] {
			if _, exists := fields[column.name]; !exists {
				return result, fmt.Errorf("archive missing source column %s", column.name)
			}
		}
		var row ClickHouseLogArchiveRow
		if err := common.Unmarshal(line, &row); err != nil {
			return result, err
		}
		if row.ID <= lastID || row.ID > manifest.ThroughID {
			return result, errors.New("archive IDs must be unique, ascending and inside the manifest interval")
		}
		stamp := int64(0)
		if row.CreatedAt != nil {
			stamp = *row.CreatedAt
		}
		if stamp < 0 || stamp > 4294967295 {
			return result, errors.New("archive timestamp exceeds ClickHouse DateTime partition range")
		}
		if month := time.Unix(stamp, 0).UTC().Format("200601"); month != strconv.FormatUint(uint64(manifest.Month), 10) {
			return result, errors.New("archive row is outside the manifest UTC month")
		}
		canonical, err := common.Marshal(row)
		if err != nil {
			return result, err
		}
		_, _ = sourceHash.Write(canonical)
		_, _ = sourceHash.Write([]byte{'\n'})
		batch = append(batch, importedRow{row, uuid.NewSHA1(uuid.NameSpaceOID, []byte(manifest.Source+":"+strconv.FormatInt(row.ID, 10))).String(), result.BatchID, manifest.Source})
		lastID = row.ID
		result.Rows++
		batchBytes += len(line)
		if len(batch) == options.InsertRows || batchBytes >= options.InsertBytes {
			if err := db.WithContext(ctx).Table(stage).Create(&batch).Error; err != nil {
				return result, err
			}
			batch = batch[:0]
			batchBytes = 0
		}
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	if result.Rows != manifest.Rows || hex.EncodeToString(fileHash.Sum(nil)) != strings.ToLower(manifest.SHA256) {
		return result, errors.New("archive count or SHA256 does not match manifest")
	}
	if len(batch) > 0 {
		if err := db.WithContext(ctx).Table(stage).Create(&batch).Error; err != nil {
			return result, err
		}
	}
	stagedCount, stagedHash, err := hashClickHouseLogArchive(ctx, db.Table(stage), manifest.Source, result.BatchID)
	if err != nil {
		return result, err
	}
	expectedHash := hex.EncodeToString(sourceHash.Sum(nil))
	if stagedCount != manifest.Rows || stagedHash != expectedHash {
		return result, errors.New("staged rows failed full-column verification")
	}
	// Only overlap checking and publication serialize; staging is independent.
	for {
		lockErr := db.WithContext(ctx).Exec("CREATE TABLE logs_import_lock (id UInt8) ENGINE = Memory").Error
		if lockErr == nil {
			publicationLocked = true
			break
		}
		var exception *proto.Exception
		if !errors.As(lockErr, &exception) || exception.Code != 57 {
			return result, fmt.Errorf("cannot acquire publication lock: %w", lockErr)
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	var overlaps int64
	if err := db.WithContext(ctx).Raw("SELECT count() FROM (SELECT 1 FROM logs WHERE migration_source = ? AND id > ? AND id <= ? AND migration_batch != ? AND id IN (SELECT id FROM "+stage+") LIMIT 1)", manifest.Source, manifest.AfterID, manifest.ThroughID, result.BatchID).Scan(&overlaps).Error; err != nil {
		return result, err
	}
	if overlaps != 0 {
		return result, errors.New("import overlaps a previously published batch")
	}
	target := db.Table("logs").Where("migration_batch = ?", result.BatchID)
	count, hash, err := hashClickHouseLogArchive(ctx, target, manifest.Source, result.BatchID)
	if err != nil {
		return result, err
	}
	if count > 0 {
		if count != manifest.Rows || hash != expectedHash {
			return result, errors.New("published batch differs from archive; refusing replacement")
		}
		result.AlreadyPublished = true
		return result, nil
	}
	if err := db.WithContext(ctx).Exec("ALTER TABLE logs REPLACE PARTITION (?, ?) FROM "+stage, manifest.Month, result.BatchID).Error; err != nil {
		return result, err
	}
	// The batch gate protects this partition during verification. Other batches
	// can publish once these rows are visible to their overlap check.
	// Do not retry an ambiguous DROP: another publisher may already own the gate.
	publicationLocked = false
	if err := db.WithContext(ctx).Exec("DROP TABLE logs_import_lock SYNC").Error; err != nil {
		return result, err
	}
	count, hash, err = hashClickHouseLogArchive(ctx, target, manifest.Source, result.BatchID)
	if err != nil {
		return result, err
	}
	if count != manifest.Rows || hash != expectedHash {
		return result, errors.New("published rows failed full-column verification; retain the source archive")
	}
	return result, nil
}

func hashClickHouseLogArchive(ctx context.Context, query *gorm.DB, source, batch string) (int64, string, error) {
	rows, err := query.WithContext(ctx).Order("id").Rows()
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	hash := sha256.New()
	var count int64
	var lastID int64
	for rows.Next() {
		var row struct {
			ClickHouseLogArchiveRow `gorm:"embedded"`
			EventID                 string
			MigrationSource         string
			MigrationBatch          string
		}
		if err := query.ScanRows(rows, &row); err != nil {
			return 0, "", err
		}
		expectedID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(source+":"+strconv.FormatInt(row.ID, 10))).String()
		if row.ID <= lastID || row.EventID != expectedID || row.MigrationSource != source || row.MigrationBatch != batch {
			return 0, "", errors.New("import identity verification failed")
		}
		lastID = row.ID
		encoded, err := common.Marshal(row.ClickHouseLogArchiveRow)
		if err != nil {
			return 0, "", err
		}
		_, _ = hash.Write(encoded)
		_, _ = hash.Write([]byte{'\n'})
		count++
	}
	return count, hex.EncodeToString(hash.Sum(nil)), rows.Err()
}
