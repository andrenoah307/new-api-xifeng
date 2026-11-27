package common

import (
	"context"
	"errors"
	"strings"
)

// Reports retain their own durable delivery ledger while sharing the SMTP
// connection, account-wide pacing and authentication cooldown with other mail.
func SendRevenueEmail(ctx context.Context, subject, receiver, body string) (state string, err error) {
	if strings.Contains(receiver, ";") {
		return "rejected", errors.New("invalid recipient")
	}
	if err := ValidateEmailMessage(subject, receiver, body); err != nil {
		return "rejected", err
	}
	uncertain, err := sendQueuedSMTP(ctx, subject, receiver, body, nil)
	if err == nil {
		return "accepted", nil
	}
	if uncertain {
		return "unknown", err
	}
	var failure *SMTPDeliveryError
	if errors.As(err, &failure) && SMTPRetryDelay(err, 1) == 0 {
		return "rejected", err
	}
	return "retryable", err
}
