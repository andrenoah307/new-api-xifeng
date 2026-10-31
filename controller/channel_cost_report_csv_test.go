package controller

import (
	"encoding/csv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostReportCSVExactDecimalAndFormulaSafety(t *testing.T) {
	row, err := projectChannelCostReportRow(model.ChannelCostReportRow{ConsumeCount: 1, OriginalCount: 1, ConsumeAmount: "9007199254740993.123456789012345678", KnownRevenue: "9007199254740993.123456789012345678", RefundAmount: "0", OriginalCost: "9007199254740994", FallbackCost: "0"})
	require.NoError(t, err)
	row.ChannelName = "\u0001@formula,\"quoted\""
	job := channelCostReportJobDTO{ChannelCostReportJob: &model.ChannelCostReportJob{ID: "task", Status: "completed", Cursor: 200}, Start: 100, End: 200}
	raw, err := channelCostReportCSV(job, []channelCostReportRowDTO{row})
	require.NoError(t, err)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(raw), "\ufeff"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)
	assert.Equal(t, "'\u0001@formula,\"quoted\"", records[1][6])
	assert.Equal(t, "9007199254740993.123456789012345678", records[1][7])
	assert.Equal(t, "-0.876543210987654322", records[1][14], "negative numeric profit must not be treated as text injection")
}
