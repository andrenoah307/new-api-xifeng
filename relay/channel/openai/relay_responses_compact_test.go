package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOaiResponsesCompactionHandlerMapsAllInputTokenDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	responseBody := `{"id":"resp_compact","usage":{"input_tokens":100,"output_tokens":7,"total_tokens":107,"input_tokens_details":{"cached_tokens":40,"cached_creation_tokens":10,"cache_write_tokens":11,"text_tokens":30,"audio_tokens":4,"image_tokens":5}}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}

	usage, apiErr := OaiResponsesCompactionHandler(c, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, 7, usage.CompletionTokens)
	assert.Equal(t, 107, usage.TotalTokens)
	assert.Equal(t, dto.InputTokenDetails{
		CachedTokens:         40,
		CachedCreationTokens: 10,
		CacheWriteTokens:     11,
		TextTokens:           30,
		AudioTokens:          4,
		ImageTokens:          5,
	}, usage.PromptTokensDetails)
	assert.Nil(t, usage.InputTokensDetails)
}
