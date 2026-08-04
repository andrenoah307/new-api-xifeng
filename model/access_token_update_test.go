package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAccessTokenUpdateTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}))
}

func TestUpdateUserAccessTokenRejectsDisabledUser(t *testing.T) {
	setupAccessTokenUpdateTest(t)

	oldToken := "old-disabled-token"
	user := &User{
		Username:    "access-token-disabled",
		Password:    "hashed",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusDisabled,
		Group:       "default",
		AccessToken: &oldToken,
	}
	require.NoError(t, DB.Create(user).Error)

	err := UpdateUserAccessToken(user.Id, "new-disabled-token")
	require.ErrorIs(t, err, ErrUserNotEnabled)

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, got.Status)
	assert.Equal(t, oldToken, got.GetAccessToken())
}

func TestUpdateUserAccessTokenPreservesUserAttributes(t *testing.T) {
	setupAccessTokenUpdateTest(t)

	user := &User{
		Username: "access-token-enabled",
		Password: "hashed",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
		Group:    "premium",
	}
	require.NoError(t, DB.Create(user).Error)

	require.NoError(t, UpdateUserAccessToken(user.Id, "new-enabled-token"))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "new-enabled-token", got.GetAccessToken())
	assert.Equal(t, common.UserStatusEnabled, got.Status)
	assert.Equal(t, common.RoleAdminUser, got.Role)
	assert.Equal(t, "premium", got.Group)
}

func TestUpdateUserAccessTokenDoesNotReviveStaleDisabledSnapshot(t *testing.T) {
	setupAccessTokenUpdateTest(t)

	oldToken := "stale-old-token"
	user := &User{
		Username:    "access-token-stale",
		Password:    "hashed",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AccessToken: &oldToken,
	}
	require.NoError(t, DB.Create(user).Error)

	staleSnapshot, err := GetUserById(user.Id, true)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("status", common.UserStatusDisabled).Error)

	staleSnapshot.SetAccessToken("stale-new-token")
	err = UpdateUserAccessToken(staleSnapshot.Id, staleSnapshot.GetAccessToken())
	require.ErrorIs(t, err, ErrUserNotEnabled)

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, got.Status)
	assert.Equal(t, oldToken, got.GetAccessToken())
	assert.Equal(t, common.RoleCommonUser, got.Role)
	assert.Equal(t, "default", got.Group)
}

func TestUpdateUserAccessTokenReturnsDatabaseError(t *testing.T) {
	setupAccessTokenUpdateTest(t)

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

	err = UpdateUserAccessToken(1, "unreachable-token")
	require.Error(t, err)
}
