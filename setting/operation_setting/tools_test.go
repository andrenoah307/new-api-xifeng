package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetToolPrice_AlphaSearch(t *testing.T) {
	original := toolPriceSetting
	t.Cleanup(func() {
		toolPriceSetting = original
		RebuildToolPriceIndex()
	})

	toolPriceSetting = ToolPriceSetting{Prices: map[string]float64{}}
	RebuildToolPriceIndex()
	assert.Equal(t, 10.0, GetToolPrice("alpha_search"))

	toolPriceSetting = ToolPriceSetting{Prices: map[string]float64{"alpha_search": 17.5}}
	RebuildToolPriceIndex()
	assert.Equal(t, 17.5, GetToolPrice("alpha_search"))
}
