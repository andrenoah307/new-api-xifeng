package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogQueriesPropagateCanceledContext(t *testing.T) {
	tests := []struct {
		name  string
		query func(context.Context) error
	}{
		{
			name: "all logs count",
			query: func(ctx context.Context) error {
				_, _, err := GetAllLogs(ctx, LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "", 0)
				return err
			},
		},
		{
			name: "all logs list",
			query: func(ctx context.Context) error {
				_, _, err := GetAllLogs(ctx, LogTypeUnknown, 0, 0, "", "", "", 0, 10, 0, "", "", "", 1)
				return err
			},
		},
		{
			name: "user logs count",
			query: func(ctx context.Context) error {
				_, _, err := GetUserLogs(ctx, 1, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "", 0)
				return err
			},
		},
		{
			name: "user logs list",
			query: func(ctx context.Context) error {
				_, _, err := GetUserLogs(ctx, 1, LogTypeUnknown, 0, 0, "", "", 0, 10, "", "", "", 1)
				return err
			},
		},
		{
			name: "log statistics",
			query: func(ctx context.Context) error {
				_, err := SumUsedQuota(ctx, 0, LogTypeConsume, 0, 0, "", "", "", 0, "", "", "")
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			require.ErrorIs(t, test.query(ctx), context.Canceled)
		})
	}
}
