package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicErrorDoesNotChangeInternalDecisions(t *testing.T) {
	raw := OpenAIError{Message: "No available channel under group private-group", Type: "private-type", Code: "model_not_found", Param: "private-param", Metadata: []byte(`{"provider_name":"private-provider"}`)}
	err := WithOpenAIError(raw, 503, ErrOptionWithSkipRetry())
	before := err.Error()
	public := err.ToPublicOpenAIError("local-id")
	assert.Equal(t, "model_not_found", public.Code)
	assert.Equal(t, "", public.Param)
	assert.Empty(t, public.Metadata)
	assert.Contains(t, public.Message, "上游服务暂不可用，请稍后重试")
	assert.Contains(t, public.Message, "local-id")
	assert.NotContains(t, public.Message, "private-")
	assert.Equal(t, before, err.Error())
	assert.Equal(t, ErrorCode("model_not_found"), err.GetErrorCode())
	require.True(t, err.IsSkipRetry())
	assert.Contains(t, err.ToOpenAIError().Message, "private-group")
	assert.Equal(t, public.Message, err.ToPublicClaudeError("local-id").Message)
}
