package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPath2RelayMode_AlphaSearch(t *testing.T) {
	assert.Equal(t, RelayModeAlphaSearch, Path2RelayMode("/v1/alpha/search"))
}
