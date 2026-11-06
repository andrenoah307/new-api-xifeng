package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

// System-owned records are absent from every authenticated user's self/token
// query. ChannelId also makes these events searchable in the admin log view.
func RecordChannelAutomationLog(channelID int, action, reason string, details map[string]any) {
	if channelID <= 0 {
		return
	}
	encoded, err := common.Marshal(details)
	if err != nil || len(encoded) > 8192 || len(reason) > 4096 || len(action) > 80 {
		common.SysError("channel automation audit exceeds payload limits")
		return
	}
	other := common.MapToJsonStr(map[string]any{
		"op":         buildOpField(action, map[string]any{"id": channelID}),
		"admin_info": map[string]any{"channel_automation": details, "reason": reason, "actor": "system"},
	})
	entry := &Log{UserId: 0, TokenId: 0, Username: "system", ChannelId: channelID,
		CreatedAt: common.GetTimestamp(), Type: LogTypeManage, Other: other,
		Content: fmt.Sprintf("Channel #%d: %s; %s; %s", channelID, action, reason, encoded)}
	if err := createLog(entry); err != nil {
		common.SysError("failed to record channel automation audit")
	}
}
