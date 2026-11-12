package types

import "github.com/QuantumNous/new-api/common"

// Public conversions are copies. Routing filters, retries and billing must keep
// using the original error and the existing internal conversions.
func (e *NewAPIError) ToPublicOpenAIError(requestID string) OpenAIError {
	result, _ := e.publicErrorProjection(requestID)
	return result
}

func (e *NewAPIError) publicErrorProjection(requestID string) (OpenAIError, bool) {
	result := e.ToOpenAIError()
	// Inspect the unmasked message too: URL masking must not hide a match.
	probe := result
	probe.Message = e.Error()
	probe.Metadata = e.Metadata
	data, err := common.Marshal(map[string]OpenAIError{"error": probe})
	if err != nil {
		return OpenAIError{Message: common.PublicErrorMessage(e.StatusCode, requestID), Type: "upstream_error", Code: "upstream_error"}, true
	}
	if clean, changed := common.RedactPublicErrorJSON(data, e.StatusCode, requestID); changed {
		var envelope struct {
			Error OpenAIError `json:"error"`
		}
		if common.Unmarshal(clean, &envelope) == nil {
			return envelope.Error, true
		}
	}
	return result, false
}

func (e *NewAPIError) ToPublicClaudeError(requestID string) ClaudeError {
	result := e.ToClaudeError()
	public, changed := e.publicErrorProjection(requestID)
	if changed {
		return ClaudeError{Type: "api_error", Message: public.Message}
	}
	return result
}
