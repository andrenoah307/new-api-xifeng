package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestConvertOpenAIRequestRecordsReasoningEffortForNonGPT5Model(t *testing.T) {
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-4.6"}}
	request := &dto.GeneralOpenAIRequest{
		Model:           "grok-4.6",
		ReasoningEffort: "high",
	}

	_, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, request)
	require.NoError(t, err)
	require.Equal(t, "high", info.ReasoningEffort)
}
