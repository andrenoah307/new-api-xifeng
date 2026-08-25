package model

import (
	"bytes"
	crand "crypto/rand"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserAffCodeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := DB
	originalLogDB := LOG_DB
	originalMainDB := common.MainDatabaseType()
	originalLogDatabase := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&User{}))

	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDB, originalLogDatabase)
		common.RedisEnabled = originalRedisEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestIsAffCodeUniqueViolationMatchesOnlyAffCodeIndex(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "mysql aff code index",
			err:  &mysql.MySQLError{Number: 1062, Message: "Duplicate entry '22222222' for key 'users.idx_users_aff_code'"},
			want: true,
		},
		{
			name: "mysql username index",
			err:  &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'alice' for key 'users.uni_users_username'"},
		},
		{
			name: "postgres aff code index",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "idx_users_aff_code"},
			want: true,
		},
		{
			name: "postgres username index",
			err:  &pgconn.PgError{Code: "23505", ConstraintName: "uni_users_username"},
		},
		{
			name: "sqlite aff code column",
			err:  errors.New("constraint failed: UNIQUE constraint failed: users.aff_code (2067)"),
			want: true,
		},
		{
			name: "sqlite username column",
			err:  errors.New("constraint failed: UNIQUE constraint failed: users.username (2067)"),
		},
		{
			name: "wrapped sqlite aff code column",
			err:  fmt.Errorf("create user: %w", errors.New("UNIQUE constraint failed: users.aff_code")),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("database unavailable"),
		},
		{
			name: "nil error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, IsAffCodeUniqueViolation(test.err))
		})
	}
}

func TestInsertWithTxRetriesAffCodeCollisionAndKeepsOuterTransactionUsable(t *testing.T) {
	db := setupUserAffCodeTestDB(t)
	require.NoError(t, db.Create(&User{Username: "aff-blocker", Password: "hashed", AffCode: "22222222"}).Error)

	originalReader := crand.Reader
	entropy := append(bytes.Repeat([]byte{9}, 16), bytes.Repeat([]byte{0}, 8)...)
	entropy = append(entropy, bytes.Repeat([]byte{1}, 8)...)
	crand.Reader = bytes.NewReader(entropy)
	t.Cleanup(func() { crand.Reader = originalReader })

	createAttempts := 0
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:count_aff_create", func(tx *gorm.DB) {
		user, ok := tx.Statement.Dest.(*User)
		if ok && user.Username == "retry-user" {
			createAttempts++
		}
	}))

	user := &User{Username: "retry-user", Password: "password123", Role: common.RoleCommonUser}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := user.InsertWithTx(tx, 0); err != nil {
			return err
		}
		return tx.Create(&User{Username: "outer-transaction-marker", Password: "hashed", AffCode: "44444444"}).Error
	})
	require.NoError(t, err)
	assert.Equal(t, 2, createAttempts)
	assert.Equal(t, "33333333", user.AffCode)
	assert.True(t, common.ValidatePasswordAndHash("password123", user.Password), "password must be hashed exactly once")

	var markerCount int64
	require.NoError(t, db.Model(&User{}).Where("username = ?", "outer-transaction-marker").Count(&markerCount).Error)
	assert.Equal(t, int64(1), markerCount)
}

func TestInsertWithTxReturnsExhaustedAfterFiveAffCodeCollisions(t *testing.T) {
	db := setupUserAffCodeTestDB(t)
	require.NoError(t, db.Create(&User{Username: "exhaust-blocker", Password: "hashed", AffCode: "22222222"}).Error)

	originalReader := crand.Reader
	crand.Reader = bytes.NewReader(bytes.Repeat([]byte{0}, 5*8))
	t.Cleanup(func() { crand.Reader = originalReader })

	createAttempts := 0
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:count_exhausted_create", func(tx *gorm.DB) {
		user, ok := tx.Statement.Dest.(*User)
		if ok && user.Username == "exhaust-user" {
			createAttempts++
		}
	}))

	user := &User{Username: "exhaust-user"}
	err := db.Transaction(func(tx *gorm.DB) error {
		return user.InsertWithTx(tx, 0)
	})
	require.ErrorIs(t, err, ErrAffCodeGenerationExhausted)
	assert.Equal(t, 5, createAttempts)

	var count int64
	require.NoError(t, db.Model(&User{}).Where("username = ?", user.Username).Count(&count).Error)
	assert.Zero(t, count)
}

func TestInsertWithTxDoesNotRetryUsernameUniqueViolation(t *testing.T) {
	db := setupUserAffCodeTestDB(t)
	require.NoError(t, db.Create(&User{Username: "duplicate-user", Password: "hashed", AffCode: "ABCDEFGH"}).Error)

	originalReader := crand.Reader
	crand.Reader = bytes.NewReader(bytes.Repeat([]byte{1}, 8))
	t.Cleanup(func() { crand.Reader = originalReader })

	createAttempts := 0
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:count_username_create", func(tx *gorm.DB) {
		user, ok := tx.Statement.Dest.(*User)
		if ok && user.AffCode == "33333333" {
			createAttempts++
		}
	}))

	user := &User{Username: "duplicate-user"}
	err := db.Transaction(func(tx *gorm.DB) error {
		return user.InsertWithTx(tx, 0)
	})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrAffCodeGenerationExhausted)
	assert.Contains(t, err.Error(), "users.username")
	assert.Equal(t, 1, createAttempts)
}
