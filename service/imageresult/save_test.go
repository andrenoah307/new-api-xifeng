package imageresult

import (
	"context"
	"encoding/base64"
	"io"
	"testing"

	"github.com/QuantumNous/new-api/service/attachment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 1x1 PNG，最小合法图片，用于走通解码→嗅探→落盘全链路。
var tinyPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, 0x89, 0x00, 0x00, 0x00,
	0x0A, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82,
}

func newTestStore(t *testing.T) attachment.Storage {
	t.Helper()
	store, err := attachment.NewLocalStorage(t.TempDir())
	require.NoError(t, err)
	return store
}

func readStoredFile(t *testing.T, store attachment.Storage, key string) []byte {
	t.Helper()
	reader, err := store.Get(context.Background(), key)
	require.NoError(t, err)
	defer reader.Close()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	return data
}

func TestSaveBase64ImageValid(t *testing.T) {
	store := newTestStore(t)
	b64 := base64.StdEncoding.EncodeToString(tinyPNG)

	file, err := saveBase64Image(store, b64, 1024*1024)
	require.NoError(t, err)
	assert.Equal(t, "image/png", file.Mime)
	assert.Equal(t, "b64", file.Source)
	assert.EqualValues(t, len(tinyPNG), file.Size)
	assert.Equal(t, tinyPNG, readStoredFile(t, store, file.Key))
}

func TestSaveBase64ImageDataURIPrefix(t *testing.T) {
	store := newTestStore(t)
	b64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG)

	file, err := saveBase64Image(store, b64, 1024*1024)
	require.NoError(t, err)
	assert.Equal(t, tinyPNG, readStoredFile(t, store, file.Key))
}

func TestSaveBase64ImageInvalid(t *testing.T) {
	store := newTestStore(t)

	_, err := saveBase64Image(store, "!!not-base64!!", 1024*1024)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode base64")
}

func TestSaveBase64ImageOverLimit(t *testing.T) {
	store := newTestStore(t)
	b64 := base64.StdEncoding.EncodeToString(tinyPNG)

	_, err := saveBase64Image(store, b64, 10) // 上限 10 字节，图片必超
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds limit")
}

func TestPutImageExtByMime(t *testing.T) {
	store := newTestStore(t)

	file, err := putImage(store, tinyPNG, "image/jpeg", "url")
	require.NoError(t, err)
	assert.Regexp(t, `^\d{4}/\d{2}/[0-9a-f-]+\.jpg$`, file.Key)
	assert.Equal(t, "url", file.Source)
}
