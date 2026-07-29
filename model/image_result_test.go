package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertImageResult(t *testing.T, userId int, createdTime int64, files []ImageResultFile) *ImageResult {
	t.Helper()
	r := &ImageResult{
		UserId:      userId,
		ModelName:   "gpt-image-1",
		Mode:        "generations",
		Prompt:      "a cat",
		RequestId:   "req-test",
		CreatedTime: createdTime,
	}
	require.NoError(t, r.SetFiles(files))
	require.NoError(t, r.Insert())
	return r
}

func TestImageResultFilesRoundTrip(t *testing.T) {
	truncateTables(t)

	files := []ImageResultFile{
		{Key: "2026/07/a.png", Mime: "image/png", Size: 123, Source: "b64"},
		{Key: "2026/07/b.jpg", Mime: "image/jpeg", Size: 456, Source: "url"},
	}
	r := insertImageResult(t, 1, 1000, files)

	loaded, err := GetImageResultById(r.Id)
	require.NoError(t, err)
	assert.Equal(t, files, loaded.GetFiles())
}

func TestImageResultFilesMalformedJSON(t *testing.T) {
	truncateTables(t)

	r := insertImageResult(t, 1, 1000, nil)
	require.NoError(t, DB.Model(&ImageResult{}).Where("id = ?", r.Id).Update("files", "{not json").Error)

	loaded, err := GetImageResultById(r.Id)
	require.NoError(t, err)
	assert.Nil(t, loaded.GetFiles())
}

func TestGetUserImageResultsPaginationAndIsolation(t *testing.T) {
	truncateTables(t)

	for i := 0; i < 3; i++ {
		insertImageResult(t, 1, int64(1000+i), nil)
	}
	insertImageResult(t, 2, 5000, nil)

	results, total, err := GetUserImageResults(1, 0, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, results, 2)
	// id 倒序：最新的在前
	assert.Greater(t, results[0].Id, results[1].Id)
	for _, r := range results {
		assert.Equal(t, 1, r.UserId)
	}

	// 第二页
	results, _, err = GetUserImageResults(1, 2, 2)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestGetAllImageResultsUserFilter(t *testing.T) {
	truncateTables(t)

	insertImageResult(t, 1, 1000, nil)
	insertImageResult(t, 2, 2000, nil)

	all, total, err := GetAllImageResults(0, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Len(t, all, 2)

	filtered, total, err := GetAllImageResults(2, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, filtered, 1)
	assert.Equal(t, 2, filtered[0].UserId)
}

func TestGetExpiredImageResultsCutoff(t *testing.T) {
	truncateTables(t)

	old := insertImageResult(t, 1, 100, nil)
	insertImageResult(t, 1, 9999, nil)

	expired, err := GetExpiredImageResults(500, 10)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	assert.Equal(t, old.Id, expired[0].Id)

	require.NoError(t, DeleteImageResultById(old.Id))
	expired, err = GetExpiredImageResults(500, 10)
	require.NoError(t, err)
	assert.Empty(t, expired)
}
