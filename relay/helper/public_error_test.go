package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestPublicErrorFramesPreserveSuccessfulData(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Set(common.RequestIdKey, "local-stream-id")
	for _, frame := range []string{`{"choices":[{"delta":{"content":"group secret"}}],"usage":{"total_tokens":9007199254740993}}`, `[DONE]`, `{"type":"response.output_text.delta","delta":"channel secret"}`} {
		assert.Equal(t, frame, PublicErrorData(c, frame))
	}
	frame := `{"type":"error","error":{"message":"group secret unavailable","metadata":{"key":"credential"}}}`
	public := PublicErrorData(c, frame)
	assert.NotContains(t, public, "secret")
	assert.NotContains(t, public, "credential")
	assert.Contains(t, public, "上游服务暂不可用，请稍后重试")
	assert.Contains(t, public, "local-stream-id")
}
