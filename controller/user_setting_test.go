package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyUserNotificationSetting(t *testing.T) {
	base := dto.UserSetting{
		SidebarModules:                   "dashboard,usage",
		BillingPreference:                "wallet_first",
		Language:                         "ja",
		WebhookUrl:                       "https://old.example/webhook",
		WebhookSecret:                    "old-secret",
		NotificationEmail:                "old@example.com",
		BarkUrl:                          "https://old.example/bark",
		GotifyUrl:                        "https://old.example/gotify",
		GotifyToken:                      "old-token",
		GotifyPriority:                   7,
		UpstreamModelUpdateNotifyEnabled: false,
	}

	tests := []struct {
		name     string
		existing dto.UserSetting
		req      UpdateUserSettingRequest
		upstream bool
		want     dto.UserSetting
	}{
		{
			name:     "email preserves unrelated settings",
			existing: base,
			req: UpdateUserSettingRequest{
				QuotaWarningType:           dto.NotifyTypeEmail,
				QuotaWarningThreshold:      2.5,
				NotificationEmail:          "new@example.com",
				AcceptUnsetModelRatioModel: true,
				RecordIpLog:                true,
			},
			upstream: true,
			want: dto.UserSetting{
				NotifyType:                       dto.NotifyTypeEmail,
				QuotaWarningThreshold:            2.5,
				NotificationEmail:                "new@example.com",
				UpstreamModelUpdateNotifyEnabled: true,
				AcceptUnsetRatioModel:            true,
				RecordIpLog:                      true,
				SidebarModules:                   "dashboard,usage",
				BillingPreference:                "wallet_first",
				Language:                         "ja",
			},
		},
		{
			name:     "webhook clears other channels",
			existing: base,
			req: UpdateUserSettingRequest{
				QuotaWarningType:      dto.NotifyTypeWebhook,
				WebhookUrl:            "https://new.example/webhook",
				WebhookSecret:         "new-secret",
				QuotaWarningThreshold: 1,
			},
			upstream: false,
			want: dto.UserSetting{
				NotifyType:                       dto.NotifyTypeWebhook,
				QuotaWarningThreshold:            1,
				WebhookUrl:                       "https://new.example/webhook",
				WebhookSecret:                    "new-secret",
				UpstreamModelUpdateNotifyEnabled: false,
				SidebarModules:                   "dashboard,usage",
				BillingPreference:                "wallet_first",
				Language:                         "ja",
			},
		},
		{
			name:     "empty webhook secret preserves existing secret",
			existing: base,
			req: UpdateUserSettingRequest{
				QuotaWarningType: dto.NotifyTypeWebhook,
				WebhookUrl:       "https://new.example/webhook",
			},
			upstream: true,
			want: dto.UserSetting{
				NotifyType:                       dto.NotifyTypeWebhook,
				WebhookUrl:                       "https://new.example/webhook",
				WebhookSecret:                    "old-secret",
				UpstreamModelUpdateNotifyEnabled: true,
				SidebarModules:                   "dashboard,usage",
				BillingPreference:                "wallet_first",
				Language:                         "ja",
			},
		},
		{
			name:     "leaving webhook clears webhook fields",
			existing: base,
			req: UpdateUserSettingRequest{
				QuotaWarningType:  dto.NotifyTypeEmail,
				NotificationEmail: "new@example.com",
			},
			upstream: false,
			want: dto.UserSetting{
				NotifyType:                       dto.NotifyTypeEmail,
				WebhookUrl:                       "",
				WebhookSecret:                    "",
				NotificationEmail:                "new@example.com",
				UpstreamModelUpdateNotifyEnabled: false,
				SidebarModules:                   "dashboard,usage",
				BillingPreference:                "wallet_first",
				Language:                         "ja",
			},
		},
		{
			name:     "bark keeps only bark channel",
			existing: base,
			req: UpdateUserSettingRequest{
				QuotaWarningType: dto.NotifyTypeBark,
				BarkUrl:          "https://new.example/bark",
			},
			upstream: true,
			want: dto.UserSetting{
				NotifyType:                       dto.NotifyTypeBark,
				BarkUrl:                          "https://new.example/bark",
				UpstreamModelUpdateNotifyEnabled: true,
				SidebarModules:                   "dashboard,usage",
				BillingPreference:                "wallet_first",
				Language:                         "ja",
			},
		},
		{
			name:     "gotify priority below range defaults to five",
			existing: base,
			req: UpdateUserSettingRequest{
				QuotaWarningType: dto.NotifyTypeGotify,
				GotifyUrl:        "https://new.example/gotify",
				GotifyToken:      "new-token",
				GotifyPriority:   -1,
			},
			want: dto.UserSetting{
				NotifyType:        dto.NotifyTypeGotify,
				GotifyUrl:         "https://new.example/gotify",
				GotifyToken:       "new-token",
				GotifyPriority:    5,
				SidebarModules:    "dashboard,usage",
				BillingPreference: "wallet_first",
				Language:          "ja",
			},
		},
		{
			name:     "gotify priority above range defaults to five",
			existing: base,
			req: UpdateUserSettingRequest{
				QuotaWarningType: dto.NotifyTypeGotify,
				GotifyUrl:        "https://new.example/gotify",
				GotifyToken:      "new-token",
				GotifyPriority:   11,
			},
			want: dto.UserSetting{
				NotifyType:        dto.NotifyTypeGotify,
				GotifyUrl:         "https://new.example/gotify",
				GotifyToken:       "new-token",
				GotifyPriority:    5,
				SidebarModules:    "dashboard,usage",
				BillingPreference: "wallet_first",
				Language:          "ja",
			},
		},
		{
			name:     "gotify priority boundaries are preserved",
			existing: base,
			req: UpdateUserSettingRequest{
				QuotaWarningType: dto.NotifyTypeGotify,
				GotifyUrl:        "https://new.example/gotify",
				GotifyToken:      "new-token",
				GotifyPriority:   0,
			},
			want: dto.UserSetting{
				NotifyType:        dto.NotifyTypeGotify,
				GotifyUrl:         "https://new.example/gotify",
				GotifyToken:       "new-token",
				GotifyPriority:    0,
				SidebarModules:    "dashboard,usage",
				BillingPreference: "wallet_first",
				Language:          "ja",
			},
		},
		{
			name:     "gotify priority upper boundary is preserved",
			existing: base,
			req: UpdateUserSettingRequest{
				QuotaWarningType: dto.NotifyTypeGotify,
				GotifyUrl:        "https://new.example/gotify",
				GotifyToken:      "new-token",
				GotifyPriority:   10,
			},
			want: dto.UserSetting{
				NotifyType:        dto.NotifyTypeGotify,
				GotifyUrl:         "https://new.example/gotify",
				GotifyToken:       "new-token",
				GotifyPriority:    10,
				SidebarModules:    "dashboard,usage",
				BillingPreference: "wallet_first",
				Language:          "ja",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := applyUserNotificationSetting(tc.existing, tc.req, tc.upstream)

			assert.Equal(t, tc.want.SidebarModules, result.SidebarModules)
			assert.Equal(t, tc.want.BillingPreference, result.BillingPreference)
			assert.Equal(t, tc.want.Language, result.Language)
			require.Equal(t, tc.want, result)
		})
	}
}
