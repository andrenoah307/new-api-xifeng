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

func TestValidateChannelPressureCoolingUpstreamErrorFields(t *testing.T) {
	tests := []struct {
		name       string
		percent    int
		minSamples int
		mode       string
		wantErr    bool
	}{
		{name: "valid lower bounds", percent: 0, minSamples: 1, mode: "any"},
		{name: "valid upper bounds", percent: 100, minSamples: 10000, mode: "ALL"},
		{name: "percent below zero", percent: -1, minSamples: 1, mode: "any", wantErr: true},
		{name: "percent above one hundred", percent: 101, minSamples: 1, mode: "any", wantErr: true},
		{name: "minimum samples below one", percent: 50, minSamples: 0, mode: "any", wantErr: true},
		{name: "minimum samples above limit", percent: 50, minSamples: 10001, mode: "any", wantErr: true},
		{name: "invalid condition mode", percent: 50, minSamples: 10, mode: "sometimes", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{Group: "pro", Key: "key"}
			channel.SetSetting(dto.ChannelSettings{PressureCooling: &dto.PressureCoolingOverride{
				UpstreamErrorTriggerPercent: &test.percent,
				UpstreamErrorMinSamples:     &test.minSamples,
				ConditionMode:               test.mode,
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

func TestValidateChannelPressureCoolingTriggerPercent(t *testing.T) {
	tests := []struct {
		name       string
		percent    int
		setPercent bool
		wantErr    bool
	}{
		{name: "nil inherits global setting"},
		{name: "one is valid", percent: 1, setPercent: true},
		{name: "one hundred is valid", percent: 100, setPercent: true},
		{name: "zero is invalid", percent: 0, setPercent: true, wantErr: true},
		{name: "negative is invalid", percent: -1, setPercent: true, wantErr: true},
		{name: "above one hundred is invalid", percent: 101, setPercent: true, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{Group: "pro", Key: "key"}
			override := &dto.PressureCoolingOverride{}
			if test.setPercent {
				override.TriggerPercent = &test.percent
			}
			channel.SetSetting(dto.ChannelSettings{PressureCooling: override})

			err := validateChannel(channel, false)
			if test.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "压力冷却触发百分比")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
