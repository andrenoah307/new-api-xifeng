package service

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
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

func TestLogExportWorkerAppliesTypeFilter(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.MkdirAll(exportDir, 0o755))
	require.NoError(t, model.DB.Exec("DELETE FROM logs").Error)
	t.Cleanup(func() {
		_ = model.DB.Exec("DELETE FROM logs").Error
	})

	const userID = 62001
	logs := []*model.Log{
		{
			UserId:    userID,
			CreatedAt: 150,
			Type:      model.LogTypeManage,
			Content:   "included-manage-row",
		},
		{
			UserId:    userID,
			CreatedAt: 150,
			Type:      model.LogTypeConsume,
			Content:   "excluded-consume-row",
		},
	}
	require.NoError(t, model.DB.Create(&logs).Error)

	task := &model.LogExportTask{
		Id:      62002,
		UserId:  userID,
		Filters: `{"type":3,"start_timestamp":100,"end_timestamp":200}`,
	}
	rowCount, _, err := (&logExportCenter{}).generateExport(task)
	require.NoError(t, err)
	assert.Equal(t, 1, rowCount)

	file, err := os.Open(filepath.Join(exportDir, "62002.csv.gz"))
	require.NoError(t, err)
	defer file.Close()
	reader, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer reader.Close()
	csvData, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(csvData), "included-manage-row")
	assert.NotContains(t, string(csvData), "excluded-consume-row")
}
