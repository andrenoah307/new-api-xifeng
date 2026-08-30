package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskModelDtoRedactsGroupForUsersButKeepsItForAdmins(t *testing.T) {
	task := &model.Task{
		ID:        41,
		TaskID:    "task-redaction-sentinel",
		Platform:  constant.TaskPlatformSuno,
		UserId:    9,
		Group:     "internal-task-group-sentinel",
		ChannelId: 733,
	}

	userDto := TaskModel2Dto(task)
	adminDto := TaskModel2AdminDto(task)
	userJSON, err := common.Marshal(userDto)
	require.NoError(t, err)
	adminJSON, err := common.Marshal(adminDto)
	require.NoError(t, err)

	assert.NotContains(t, string(userJSON), `"group"`)
	assert.Contains(t, string(adminJSON), `"group":"internal-task-group-sentinel"`)
	assert.Contains(t, string(userJSON), `"channel_id":733`)
	assert.Contains(t, string(adminJSON), `"channel_id":733`)
}
