package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
)

func TestShouldRecordChannelError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *types.NewAPIError
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{
			name: "local skip retry error",
			err: types.NewErrorWithStatusCode(
				errors.New("invalid request"),
				types.ErrorCodeInvalidRequest,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			),
			want: false,
		},
		{name: "upstream 5xx", err: types.NewErrorWithStatusCode(errors.New("upstream"), types.ErrorCodeBadResponse, http.StatusBadGateway), want: true},
		{name: "upstream 429", err: types.NewErrorWithStatusCode(errors.New("upstream"), types.ErrorCodeBadResponse, http.StatusTooManyRequests), want: true},
		{name: "upstream 403", err: types.NewErrorWithStatusCode(errors.New("upstream"), types.ErrorCodeBadResponse, http.StatusForbidden), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldRecordChannelError(tt.err))
		})
	}
}

func TestShouldRecordTaskChannelError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  *dto.TaskError
		want bool
	}{
		{name: "nil error", err: nil, want: false},
		{name: "local task error", err: &dto.TaskError{LocalError: true}, want: false},
		{name: "upstream task error", err: &dto.TaskError{LocalError: false}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldRecordTaskChannelError(tt.err))
		})
	}
}
