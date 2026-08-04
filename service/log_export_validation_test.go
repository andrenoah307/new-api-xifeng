package service

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogExportWorkerRejectsInvalidTimeRange(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(exportDir, 0o755))

	tests := []struct {
		name        string
		filtersJSON string
	}{
		{name: "空字符串", filtersJSON: ""},
		{name: "空 JSON", filtersJSON: `{}`},
		{name: "start 为零", filtersJSON: `{"start_timestamp":0,"end_timestamp":200}`},
		{name: "end 为零", filtersJSON: `{"start_timestamp":100,"end_timestamp":0}`},
		{name: "反向区间", filtersJSON: `{"start_timestamp":201,"end_timestamp":200}`},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := &model.LogExportTask{Id: index + 1, Filters: test.filtersJSON}
			_, _, err := (&logExportCenter{}).generateExport(task)

			require.Error(t, err)
			assert.Equal(t, "invalid export time range", err.Error())
		})
	}
}

func TestExportFiltersAllowValidLongRange(t *testing.T) {
	filters, err := parseExportFilters(`{"start_timestamp":100,"end_timestamp":7776100}`)
	require.NoError(t, err)

	assert.NoError(t, validateExportFilters(filters))
}
