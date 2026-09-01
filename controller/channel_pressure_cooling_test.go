package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateChannelPressureCoolingGroupsMustBelongToChannel(t *testing.T) {
	tests := []struct {
		name    string
		scope   string
		groups  []string
		wantErr bool
	}{
		{name: "default scope", scope: "", groups: nil, wantErr: false},
		{name: "channel scope", scope: "channel", groups: nil, wantErr: false},
		{name: "groups scope with valid groups", scope: "groups", groups: []string{"pro", "cheap"}, wantErr: false},
		{name: "groups scope empty", scope: "groups", groups: nil, wantErr: true},
		{name: "groups scope unknown group", scope: "groups", groups: []string{"other"}, wantErr: true},
		{name: "unknown scope", scope: "zone", groups: []string{"pro"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{Group: "pro,cheap", Key: "key"}
			channel.SetSetting(dto.ChannelSettings{PressureCooling: &dto.PressureCoolingOverride{
				Scope: test.scope, CooldownGroups: test.groups,
			}})
			err := validateChannel(channel, false)
			if test.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "压力冷却")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
