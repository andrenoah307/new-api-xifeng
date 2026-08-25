package model

import "errors"

// Common errors
var (
	ErrDatabase = errors.New("database error")
)

// User auth errors
var (
	ErrInvalidCredentials   = errors.New("invalid credentials")
	ErrUserEmptyCredentials = errors.New("empty credentials")
	ErrEmailAlreadyTaken    = errors.New("email already taken")
	ErrEmailNotFound        = errors.New("email not found")
	ErrEmailAmbiguous       = errors.New("email matches multiple users")
	ErrUserNotEnabled       = errors.New("user not enabled")
)

// ErrAffCodeGenerationExhausted indicates that every bounded candidate collided.
var ErrAffCodeGenerationExhausted = errors.New("affiliate code generation exhausted")

// Token auth errors
var (
	ErrTokenNotProvided = errors.New("token not provided")
	ErrTokenInvalid     = errors.New("token invalid")
)

// Redemption errors
var ErrRedeemFailed = errors.New("redeem.failed")

// 2FA errors
var ErrTwoFANotEnabled = errors.New("2fa not enabled")

// Invitation code errors
var (
	ErrInvitationCodeRequired             = errors.New("invitation_code.required")
	ErrInvitationCodeInvalid              = errors.New("invitation_code.invalid")
	ErrInvitationCodeExhausted            = errors.New("invitation_code.exhausted")
	ErrInvitationCodeExpired              = errors.New("invitation_code.expired")
	ErrInvitationCodeDisabled             = errors.New("invitation_code.disabled")
	ErrInvitationCodeQuotaExceeded        = errors.New("invitation_code.quota_exceeded")
	ErrInvitationCodeAccountTooYoung      = errors.New("invitation_code.account_too_young")
	ErrInvitationCodeUserGenerateDisabled = errors.New("invitation_code.user_generate_disabled")
)
