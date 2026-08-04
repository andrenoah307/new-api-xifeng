package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupInvitationCodeUpdateTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&InvitationCode{}, &InvitationCodeUsage{}))
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&InvitationCodeUsage{}).Error)
	require.NoError(t, DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&InvitationCode{}).Error)
	t.Cleanup(func() {
		_ = DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&InvitationCodeUsage{}).Error
		_ = DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(&InvitationCode{}).Error
	})
}

func consumeInvitationCodeForTest(t *testing.T, invitationCodeID int, userID int) error {
	t.Helper()
	var invitationCode InvitationCode
	require.NoError(t, DB.First(&invitationCode, invitationCodeID).Error)
	return DB.Transaction(func(tx *gorm.DB) error {
		return ConsumeInvitationCodeWithTx(tx, &invitationCode, userID, "invitation-test-user")
	})
}

func countInvitationCodeUsagesForTest(t *testing.T, invitationCodeID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&InvitationCodeUsage{}).Where("invitation_code_id = ?", invitationCodeID).Count(&count).Error)
	return count
}

func TestConsumeInvitationCodeEnforcesMaxUsesWithConditionalUpdate(t *testing.T) {
	setupInvitationCodeUpdateTest(t)

	code := &InvitationCode{Code: "invite-max-one", Status: InvitationCodeStatusEnabled, MaxUses: 1}
	require.NoError(t, DB.Create(code).Error)

	var stale InvitationCode
	require.NoError(t, DB.First(&stale, code.Id).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return ConsumeInvitationCodeWithTx(tx, &stale, 1, "first-user")
	}))

	// A second transaction can still carry an old, pre-check snapshot. The
	// conditional UPDATE must reject it instead of incrementing past MaxUses.
	stale.UsedCount = 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		return ConsumeInvitationCodeWithTx(tx, &stale, 2, "second-user")
	})
	require.ErrorIs(t, err, ErrInvitationCodeExhausted)

	var got InvitationCode
	require.NoError(t, DB.First(&got, code.Id).Error)
	assert.Equal(t, 1, got.UsedCount)
	assert.Equal(t, int64(1), countInvitationCodeUsagesForTest(t, code.Id))
}

func TestConsumeInvitationCodeAllowsUnlimitedUses(t *testing.T) {
	setupInvitationCodeUpdateTest(t)

	code := &InvitationCode{Code: "invite-unlimited", Status: InvitationCodeStatusEnabled, MaxUses: 0}
	require.NoError(t, DB.Create(code).Error)
	// The legacy schema default is one for omitted values; explicitly persist
	// zero here to exercise the documented unlimited-use semantics.
	require.NoError(t, DB.Model(&InvitationCode{}).Where("id = ?", code.Id).Update("max_uses", 0).Error)

	for userID := 1; userID <= 4; userID++ {
		require.NoError(t, consumeInvitationCodeForTest(t, code.Id, userID))
	}

	var got InvitationCode
	require.NoError(t, DB.First(&got, code.Id).Error)
	assert.Equal(t, 4, got.UsedCount)
	assert.Equal(t, int64(4), countInvitationCodeUsagesForTest(t, code.Id))
}

func TestConsumeInvitationCodeStopsAtFiniteBoundary(t *testing.T) {
	setupInvitationCodeUpdateTest(t)

	code := &InvitationCode{Code: "invite-max-three", Status: InvitationCodeStatusEnabled, MaxUses: 3}
	require.NoError(t, DB.Create(code).Error)

	for userID := 1; userID <= 3; userID++ {
		require.NoError(t, consumeInvitationCodeForTest(t, code.Id, userID))
	}
	require.ErrorIs(t, consumeInvitationCodeForTest(t, code.Id, 4), ErrInvitationCodeExhausted)

	var got InvitationCode
	require.NoError(t, DB.First(&got, code.Id).Error)
	assert.Equal(t, 3, got.UsedCount)
	assert.Equal(t, int64(3), countInvitationCodeUsagesForTest(t, code.Id))
}

func TestGetUsableInvitationCodeWithTxCanBeConsumed(t *testing.T) {
	setupInvitationCodeUpdateTest(t)

	code := &InvitationCode{Code: "invite-lock-helper", Status: InvitationCodeStatusEnabled, MaxUses: 1}
	require.NoError(t, DB.Create(code).Error)

	err := DB.Transaction(func(tx *gorm.DB) error {
		usable, err := GetUsableInvitationCodeWithTx(tx, code.Code)
		if err != nil {
			return err
		}
		return ConsumeInvitationCodeWithTx(tx, usable, 1, "lock-user")
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), countInvitationCodeUsagesForTest(t, code.Id))
}

func TestGetUsableInvitationCodeWithTxRejectsInvalidRecords(t *testing.T) {
	setupInvitationCodeUpdateTest(t)

	tests := []struct {
		name      string
		code      string
		status    int
		maxUses   int
		usedCount int
		expiredAt int64
		wantErr   error
	}{
		{name: "missing code", code: "missing", wantErr: ErrInvitationCodeInvalid},
		{name: "disabled", code: "disabled", status: InvitationCodeStatusDisabled, maxUses: 1, wantErr: ErrInvitationCodeDisabled},
		{name: "expired", code: "expired", status: InvitationCodeStatusEnabled, maxUses: 1, expiredAt: common.GetTimestamp() - 1, wantErr: ErrInvitationCodeExpired},
		{name: "exhausted", code: "exhausted", status: InvitationCodeStatusEnabled, maxUses: 1, usedCount: 1, wantErr: ErrInvitationCodeExhausted},
	}

	for _, tt := range tests[1:] {
		code := &InvitationCode{
			Code:        tt.code,
			Status:      tt.status,
			MaxUses:     tt.maxUses,
			UsedCount:   tt.usedCount,
			ExpiredTime: tt.expiredAt,
		}
		require.NoError(t, DB.Create(code).Error)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := DB.Transaction(func(tx *gorm.DB) error {
				_, err := GetUsableInvitationCodeWithTx(tx, tt.code)
				return err
			})
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestConsumeInvitationCodeWithTxValidatesSnapshot(t *testing.T) {
	setupInvitationCodeUpdateTest(t)

	assert.NoError(t, ConsumeInvitationCodeWithTx(nil, nil, 1, "nil-user"))
	disabled := &InvitationCode{Id: 1, Status: InvitationCodeStatusDisabled, MaxUses: 1}
	require.ErrorIs(t, ConsumeInvitationCodeWithTx(DB, disabled, 1, "disabled-user"), ErrInvitationCodeDisabled)
	exhausted := &InvitationCode{Id: 1, Status: InvitationCodeStatusEnabled, MaxUses: 1, UsedCount: 1}
	require.ErrorIs(t, ConsumeInvitationCodeWithTx(DB, exhausted, 1, "exhausted-user"), ErrInvitationCodeExhausted)
}

func TestGetUsableInvitationCodeWithTxRejectsEmptyAndDatabaseErrors(t *testing.T) {
	setupInvitationCodeUpdateTest(t)

	_, err := GetUsableInvitationCodeWithTx(DB, "   ")
	require.ErrorIs(t, err, ErrInvitationCodeRequired)

	originalDB := DB
	brokenDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = brokenDB
	t.Cleanup(func() {
		DB = originalDB
		sqlDB, err := brokenDB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	_, err = GetUsableInvitationCodeWithTx(brokenDB, "db-error")
	require.Error(t, err)
}

func TestConsumeInvitationCodeWithTxReturnsUpdateAndUsageErrors(t *testing.T) {
	setupInvitationCodeUpdateTest(t)

	code := &InvitationCode{Code: "invite-update-error", Status: InvitationCodeStatusEnabled, MaxUses: 1}
	require.NoError(t, DB.Create(code).Error)
	require.NoError(t, DB.Exec("CREATE TRIGGER invitation_update_error BEFORE UPDATE OF used_count ON invitation_codes BEGIN SELECT RAISE(ABORT, 'forced update error'); END").Error)
	t.Cleanup(func() { _ = DB.Exec("DROP TRIGGER invitation_update_error").Error })
	var snapshot InvitationCode
	require.NoError(t, DB.First(&snapshot, code.Id).Error)
	err := DB.Transaction(func(tx *gorm.DB) error {
		return ConsumeInvitationCodeWithTx(tx, &snapshot, 1, "update-error-user")
	})
	require.Error(t, err)

	setupInvitationCodeUpdateTest(t)
	code = &InvitationCode{Code: "invite-usage-error", Status: InvitationCodeStatusEnabled, MaxUses: 1}
	require.NoError(t, DB.Create(code).Error)
	require.NoError(t, DB.Exec("CREATE TRIGGER invitation_usage_error BEFORE INSERT ON invitation_code_usages BEGIN SELECT RAISE(ABORT, 'forced usage error'); END").Error)
	t.Cleanup(func() { _ = DB.Exec("DROP TRIGGER invitation_usage_error").Error })
	require.NoError(t, DB.First(&snapshot, code.Id).Error)
	err = DB.Transaction(func(tx *gorm.DB) error {
		return ConsumeInvitationCodeWithTx(tx, &snapshot, 1, "usage-error-user")
	})
	require.Error(t, err)
}
