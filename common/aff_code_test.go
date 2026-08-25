package common

import (
	"bytes"
	crand "crypto/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAffCodeUsesApprovedUpperBase32Alphabet(t *testing.T) {
	const alphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

	originalReader := crand.Reader
	crand.Reader = bytes.NewReader([]byte{
		0, 1, 2, 3, 4, 5, 6, 7,
		8, 9, 10, 11, 12, 13, 14, 15,
		16, 17, 18, 19, 20, 21, 22, 23,
		24, 25, 26, 27, 28, 29, 30, 31,
	})
	t.Cleanup(func() { crand.Reader = originalReader })

	var generated strings.Builder
	for range 4 {
		code, err := GenerateAffCode()
		require.NoError(t, err)
		assert.Len(t, code, 8)
		generated.WriteString(code)
		for _, char := range code {
			assert.Contains(t, alphabet, string(char))
		}
		assert.NotContains(t, code, "0")
		assert.NotContains(t, code, "1")
		assert.NotContains(t, code, "I")
		assert.NotContains(t, code, "O")
	}

	assert.Equal(t, alphabet, generated.String())
}

func TestGenerateAffCodeReturnsEntropyFailure(t *testing.T) {
	originalReader := crand.Reader
	crand.Reader = bytes.NewReader(nil)
	t.Cleanup(func() { crand.Reader = originalReader })

	code, err := GenerateAffCode()
	require.Error(t, err)
	assert.Empty(t, code)
}
