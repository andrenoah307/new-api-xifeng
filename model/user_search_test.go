package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertSearchTestUser(t *testing.T, user User) *User {
	t.Helper()
	if user.Password == "" {
		user.Password = "password123"
	}
	if user.Role == 0 {
		user.Role = common.RoleCommonUser
	}
	if user.Status == 0 {
		user.Status = common.UserStatusEnabled
	}
	if user.AffCode == "" {
		user.AffCode = "search-" + user.Username
		if user.Id != 0 {
			user.AffCode += "-" + strconv.Itoa(user.Id)
		}
	}
	require.NoError(t, DB.Create(&user).Error)
	return &user
}

func userIDs(users []*User) []int {
	ids := make([]int, len(users))
	for i, user := range users {
		ids[i] = user.Id
	}
	return ids
}

func TestSearchUsers_EmptyKeyword(t *testing.T) {
	tests := []struct {
		name          string
		group         string
		users         []User
		wantUsernames []string
	}{
		{
			name:  "group filter",
			group: "group-a",
			users: []User{
				{Username: "group-a-1", Group: "group-a"},
				{Username: "group-a-2", Group: "group-a"},
				{Username: "group-b-1", Group: "group-b"},
			},
			wantUsernames: []string{"group-a-1", "group-a-2"},
		},
		{
			name: "without conditions",
			users: []User{
				{Username: "all-users-1", Group: "group-a"},
				{Username: "all-users-2", Group: "group-b"},
			},
			wantUsernames: []string{"all-users-1", "all-users-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			for _, user := range tt.users {
				insertSearchTestUser(t, user)
			}

			users, total, err := SearchUsers("", tt.group, nil, nil, 0, 20)
			require.NoError(t, err)
			assert.EqualValues(t, len(tt.wantUsernames), total)
			assert.Len(t, users, len(tt.wantUsernames))
			gotUsernames := make([]string, len(users))
			for i, user := range users {
				gotUsernames[i] = user.Username
			}
			assert.ElementsMatch(t, tt.wantUsernames, gotUsernames)
		})
	}
}

func TestSearchUsers_Keyword(t *testing.T) {
	tests := []struct {
		name    string
		keyword string
		users   []User
		wantIDs []int
	}{
		{
			name:    "numeric matches ID and username",
			keyword: "4201",
			users: []User{
				{Id: 4201, Username: "id-match"},
				{Id: 4202, Username: "agent-4201"},
				{Id: 4203, Username: "unrelated"},
			},
			wantIDs: []int{4201, 4202},
		},
		{
			name:    "nonnumeric matches username email and display name",
			keyword: "needle",
			users: []User{
				{Id: 4301, Username: "needle-username"},
				{Id: 4302, Username: "email-user", Email: "needle@example.com"},
				{Id: 4303, Username: "display-user", DisplayName: "Needle Display"},
				{Id: 4304, Username: "unrelated-user", Email: "other@example.com", DisplayName: "Other"},
			},
			wantIDs: []int{4301, 4302, 4303},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			for _, user := range tt.users {
				insertSearchTestUser(t, user)
			}

			users, total, err := SearchUsers(tt.keyword, "", nil, nil, 0, 20)
			require.NoError(t, err)
			assert.EqualValues(t, len(tt.wantIDs), total)
			assert.ElementsMatch(t, tt.wantIDs, userIDs(users))
		})
	}
}

func TestSearchUsers_GroupRoleStatusFilters(t *testing.T) {
	tests := []struct {
		name    string
		keyword string
		group   string
		role    int
		status  int
		users   []User
		wantIDs []int
	}{
		{
			name:    "combined filters",
			keyword: "",
			group:   "paid",
			role:    common.RoleAdminUser,
			status:  common.UserStatusEnabled,
			users: []User{
				{Id: 4401, Username: "target", Group: "paid", Role: common.RoleAdminUser, Status: common.UserStatusEnabled},
				{Id: 4402, Username: "wrong-group", Group: "free", Role: common.RoleAdminUser, Status: common.UserStatusEnabled},
				{Id: 4403, Username: "wrong-role", Group: "paid", Role: common.RoleCommonUser, Status: common.UserStatusEnabled},
				{Id: 4404, Username: "wrong-status", Group: "paid", Role: common.RoleAdminUser, Status: common.UserStatusDisabled},
			},
			wantIDs: []int{4401},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			for _, user := range tt.users {
				insertSearchTestUser(t, user)
			}

			users, total, err := SearchUsers(tt.keyword, tt.group, &tt.role, &tt.status, 0, 20)
			require.NoError(t, err)
			assert.EqualValues(t, len(tt.wantIDs), total)
			assert.Equal(t, tt.wantIDs, userIDs(users))
		})
	}
}

func TestSearchUsers_Status(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantIDs []int
	}{
		{name: "soft deleted only", status: -1, wantIDs: []int{4503}},
		{name: "enabled excludes soft deleted", status: common.UserStatusEnabled, wantIDs: []int{4501}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			insertSearchTestUser(t, User{Id: 4501, Username: "active", Status: common.UserStatusEnabled})
			insertSearchTestUser(t, User{Id: 4502, Username: "disabled", Status: common.UserStatusDisabled})
			deleted := insertSearchTestUser(t, User{Id: 4503, Username: "deleted", Status: common.UserStatusEnabled})
			require.NoError(t, DB.Delete(deleted).Error)

			status := tt.status
			users, total, err := SearchUsers("", "", nil, &status, 0, 20)
			require.NoError(t, err)
			assert.EqualValues(t, len(tt.wantIDs), total)
			assert.Equal(t, tt.wantIDs, userIDs(users))
		})
	}
}

func TestSearchUsers_PaginationAndFilteredTotal(t *testing.T) {
	tests := []struct {
		name     string
		startIdx int
		num      int
		wantIDs  []int
	}{
		{name: "middle page", startIdx: 1, num: 2, wantIDs: []int{4, 3}},
		{name: "last page", startIdx: 3, num: 2, wantIDs: []int{2, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			for id := 1; id <= 5; id++ {
				insertSearchTestUser(t, User{Id: id, Username: "page-user-" + strconv.Itoa(id), Group: "page-group"})
			}

			users, total, err := SearchUsers("", "page-group", nil, nil, tt.startIdx, tt.num)
			require.NoError(t, err)
			assert.EqualValues(t, 5, total)
			assert.Equal(t, tt.wantIDs, userIDs(users))
		})
	}
}
