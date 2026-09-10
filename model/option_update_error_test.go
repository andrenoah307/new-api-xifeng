package model

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionPropagatesDatabaseErrorsWithoutChangingMemory(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(*testing.T, *gorm.DB, string)
		failCall func(*testing.T, *gorm.DB)
	}{
		{
			name:    "first or create",
			prepare: func(*testing.T, *gorm.DB, string) {},
			failCall: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:fail-option-create", func(tx *gorm.DB) {
					tx.AddError(errors.New("forced create failure"))
				}))
			},
		},
		{
			name: "save",
			prepare: func(t *testing.T, db *gorm.DB, key string) {
				require.NoError(t, db.Create(&Option{Key: key, Value: "database-old"}).Error)
			},
			failCall: func(t *testing.T, db *gorm.DB) {
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail-option-save", func(tx *gorm.DB) {
					tx.AddError(errors.New("forced save failure"))
				}))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previousDB := DB
			previousOptionMap := common.OptionMap
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "option.db")), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&Option{}))
			DB = db
			common.OptionMapRWMutex.Lock()
			common.OptionMap = map[string]string{"FailureOption": "memory-old"}
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				DB = previousDB
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptionMap
				common.OptionMapRWMutex.Unlock()
				if sqlDB, dbErr := db.DB(); dbErr == nil {
					require.NoError(t, sqlDB.Close())
				}
			})

			test.prepare(t, db, "FailureOption")
			test.failCall(t, db)
			err = UpdateOption("FailureOption", "memory-new")

			require.Error(t, err)
			common.OptionMapRWMutex.RLock()
			assert.Equal(t, "memory-old", common.OptionMap["FailureOption"])
			common.OptionMapRWMutex.RUnlock()
		})
	}
}
