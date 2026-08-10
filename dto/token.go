package dto

// TokenRequest is the client-writable token contract. Period counters and the
// persisted anchor are deliberately absent: they are server-owned state.
type TokenRequest struct {
	Id                 int     `json:"id"`
	Status             int     `json:"status"`
	Name               string  `json:"name"`
	ExpiredTime        int64   `json:"expired_time"`
	RemainQuota        int     `json:"remain_quota"`
	UnlimitedQuota     bool    `json:"unlimited_quota"`
	ModelLimitsEnabled bool    `json:"model_limits_enabled"`
	ModelLimits        string  `json:"model_limits"`
	AllowIps           *string `json:"allow_ips"`
	Group              string  `json:"group"`
	CrossGroupRetry    bool    `json:"cross_group_retry"`

	PeriodType       string `json:"period_type"`
	PeriodDays       int    `json:"period_days"`
	PeriodLimitUnit  string `json:"period_limit_unit"`
	PeriodLimitValue string `json:"period_limit_value"`
}

// TokenUpdateRequest keeps update-only presence information for period policy
// fields without changing TokenRequest's create-token contract.
type TokenUpdateRequest struct {
	TokenRequest

	PeriodType       *string `json:"period_type"`
	PeriodDays       *int    `json:"period_days"`
	PeriodLimitUnit  *string `json:"period_limit_unit"`
	PeriodLimitValue *string `json:"period_limit_value"`
}
