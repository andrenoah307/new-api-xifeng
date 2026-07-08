package setting

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type InvitationCodePolicy struct {
	MinAccountAgeDays    int            `json:"min_account_age_days"`
	DefaultGenerateQuota int            `json:"default_generate_quota"`
	DefaultCodeMaxUses   int            `json:"default_code_max_uses"`
	DefaultCodeValidDays int            `json:"default_code_valid_days"`
	GroupQuotas          map[string]int `json:"group_quotas"`
	RoleQuotas           map[string]int `json:"role_quotas"`
}

var defaultInvitationCodePolicy = InvitationCodePolicy{
	MinAccountAgeDays:    7,
	DefaultGenerateQuota: 5,
	DefaultCodeMaxUses:   1,
	DefaultCodeValidDays: 30,
	GroupQuotas: map[string]int{
		"default": 5,
		"vip":     20,
		"premium": 50,
	},
	RoleQuotas: map[string]int{
		"10":  -1,
		"100": -1,
	},
}

var invitationCodePolicy = sanitizeInvitationCodePolicy(defaultInvitationCodePolicy)

func sanitizeInvitationCodePolicy(policy InvitationCodePolicy) InvitationCodePolicy {
	if policy.MinAccountAgeDays < 0 {
		policy.MinAccountAgeDays = 0
	}
	if policy.DefaultGenerateQuota < -1 {
		policy.DefaultGenerateQuota = 0
	}
	if policy.DefaultCodeMaxUses < 0 {
		policy.DefaultCodeMaxUses = 0
	}
	if policy.DefaultCodeValidDays < 0 {
		policy.DefaultCodeValidDays = 0
	}
	if policy.GroupQuotas == nil {
		policy.GroupQuotas = map[string]int{}
	}
	if policy.RoleQuotas == nil {
		policy.RoleQuotas = map[string]int{}
	}
	return policy
}

func ValidateInvitationCodePolicy(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var policy InvitationCodePolicy
	if err := common.UnmarshalJsonStr(raw, &policy); err != nil {
		return err
	}
	_ = sanitizeInvitationCodePolicy(policy)
	return nil
}

func UpdateInvitationCodePolicy(raw string) error {
	if strings.TrimSpace(raw) == "" {
		invitationCodePolicy = sanitizeInvitationCodePolicy(defaultInvitationCodePolicy)
		return nil
	}
	var policy InvitationCodePolicy
	if err := common.UnmarshalJsonStr(raw, &policy); err != nil {
		return err
	}
	invitationCodePolicy = sanitizeInvitationCodePolicy(policy)
	return nil
}

func GetInvitationCodePolicy() InvitationCodePolicy {
	return invitationCodePolicy
}

func GetDefaultInvitationCodePolicyJSON() string {
	bytes, err := common.Marshal(defaultInvitationCodePolicy)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func GetInvitationCodePolicyJSON() string {
	bytes, err := common.Marshal(invitationCodePolicy)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}
