package common

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type bodyStorageErrorReader struct {
	err error
}

func (r bodyStorageErrorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestCreateBodyStorageFromReaderMemoryBoundaries(t *testing.T) {
	previousDiskConfig := GetDiskCacheConfig()
	SetDiskCacheConfig(DiskCacheConfig{Enabled: false})
	t.Cleanup(func() {
		SetDiskCacheConfig(previousDiskConfig)
	})

	const maxBodyBytes = int64(128 << 20)
	largeBody := bytes.Repeat([]byte("l"), 10<<20+1)
	sevenMegabyteBody := bytes.Repeat([]byte("s"), 7<<20)
	lyingLargeBody := bytes.Repeat([]byte("h"), 4<<10)
	readerErr := errors.New("reader failed")
	tests := []struct {
		name          string
		contentLength int64
		reader        io.Reader
		wantBody      []byte
		maxBytes      int64
		wantErr       error
		wantSameError bool
	}{
		{name: "empty unknown length", contentLength: 0, reader: bytes.NewReader(nil), wantBody: []byte{}, maxBytes: maxBodyBytes},
		{name: "normal unknown length", contentLength: 0, reader: bytes.NewReader([]byte("normal body")), wantBody: []byte("normal body"), maxBytes: maxBodyBytes},
		{name: "accurate length below cap", contentLength: 12, reader: bytes.NewReader([]byte("known length")), wantBody: []byte("known length"), maxBytes: maxBodyBytes},
		{name: "accurate length above fallback cap", contentLength: int64(len(largeBody)), reader: bytes.NewReader(largeBody), wantBody: largeBody, maxBytes: maxBodyBytes},
		{name: "large declared length with small body", contentLength: 128 << 20, reader: bytes.NewReader(lyingLargeBody), wantBody: lyingLargeBody, maxBytes: maxBodyBytes},
		{name: "small declared length with large body", contentLength: 4 << 10, reader: bytes.NewReader(sevenMegabyteBody), wantBody: sevenMegabyteBody, maxBytes: maxBodyBytes},
		{name: "exactly max bytes", contentLength: 99, reader: bytes.NewReader([]byte("12345")), wantBody: []byte("12345"), maxBytes: 5},
		{name: "one byte above max", contentLength: 99, reader: bytes.NewReader([]byte("123456")), maxBytes: 5, wantErr: ErrRequestBodyTooLarge},
		{name: "large declared length one byte above max", contentLength: 128 << 20, reader: bytes.NewReader([]byte("123456")), maxBytes: 5, wantErr: ErrRequestBodyTooLarge},
		{name: "non EOF reader error", contentLength: 32, reader: bodyStorageErrorReader{err: readerErr}, maxBytes: 32, wantErr: readerErr, wantSameError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			storage, err := CreateBodyStorageFromReader(test.reader, test.contentLength, test.maxBytes)
			if test.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, test.wantErr)
				if test.wantSameError {
					assert.Same(t, test.wantErr, err)
				}
				assert.Nil(t, storage)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, storage)
			t.Cleanup(func() {
				require.NoError(t, storage.Close())
			})
			body, err := storage.Bytes()
			require.NoError(t, err)
			assert.NotNil(t, body)
			assert.Equal(t, test.wantBody, body)
			assert.EqualValues(t, len(test.wantBody), storage.Size())
		})
	}
}

func TestRelayBodyPreallocCap(t *testing.T) {
	previousBudget := RelayMaxActiveBodyBytes
	previousConcurrency := RelayMaxConcurrentRequests
	t.Cleanup(func() {
		RelayMaxActiveBodyBytes = previousBudget
		RelayMaxConcurrentRequests = previousConcurrency
	})

	tests := []struct {
		name        string
		budget      int64
		concurrency int
		want        int64
	}{
		{name: "zero budget", budget: 0, concurrency: 10, want: 10 << 20},
		{name: "zero concurrency", budget: 10 << 20, concurrency: 0, want: 10 << 20},
		{name: "both unset", budget: 0, concurrency: 0, want: 10 << 20},
		{name: "production fair share", budget: 10 << 30, concurrency: 5000, want: 2147483},
		{name: "share below minimum", budget: 1, concurrency: 2, want: 64 << 10},
		{name: "share above maximum", budget: 100 << 30, concurrency: 1, want: 10 << 20},
		{name: "negative budget", budget: -1, concurrency: 1, want: 10 << 20},
		{name: "negative concurrency", budget: 1, concurrency: -1, want: 10 << 20},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			RelayMaxActiveBodyBytes = test.budget
			RelayMaxConcurrentRequests = test.concurrency
			assert.Equal(t, test.want, relayBodyPreallocCap())
		})
	}
}

func TestCreateBodyStorageFromReaderDiskPathPreservesErrorsAndLimits(t *testing.T) {
	previousConfig := GetDiskCacheConfig()
	SetDiskCacheConfig(DiskCacheConfig{
		Enabled:     true,
		ThresholdMB: 1,
		MaxSizeMB:   10,
		Path:        t.TempDir(),
	})
	t.Cleanup(func() {
		SetDiskCacheConfig(previousConfig)
	})
	diskReaderErr := errors.New("disk reader failed")

	tests := []struct {
		name          string
		body          []byte
		contentLength int64
		maxBytes      int64
		reader        io.Reader
		wantErr       error
	}{
		{
			name:          "successful disk storage",
			body:          bytes.Repeat([]byte("d"), 1<<20),
			contentLength: 1 << 20,
			maxBytes:      2 << 20,
		},
		{
			name:          "memory read selected disk storage after read",
			body:          bytes.Repeat([]byte("m"), 1<<20),
			contentLength: 0,
			maxBytes:      2 << 20,
		},
		{
			name:          "disk request too large",
			body:          bytes.Repeat([]byte("x"), 1<<20+1),
			contentLength: 1<<20 + 1,
			maxBytes:      1 << 20,
			wantErr:       ErrRequestBodyTooLarge,
		},
		{
			name:          "disk reader error",
			contentLength: 1 << 20,
			maxBytes:      2 << 20,
			reader:        bodyStorageErrorReader{err: diskReaderErr},
			wantErr:       diskReaderErr,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := test.reader
			if reader == nil {
				reader = bytes.NewReader(test.body)
			}
			storage, err := CreateBodyStorageFromReader(reader, test.contentLength, test.maxBytes)
			if test.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, test.wantErr)
				assert.Nil(t, storage)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, storage)
			t.Cleanup(func() {
				require.NoError(t, storage.Close())
			})
			assert.True(t, storage.IsDisk())
			stored, err := storage.Bytes()
			require.NoError(t, err)
			assert.Equal(t, test.body, stored)
		})
	}
}

func TestCreateBodyStorageFromReaderClampsNegativePreallocation(t *testing.T) {
	previousConfig := GetDiskCacheConfig()
	SetDiskCacheConfig(DiskCacheConfig{Enabled: false})
	t.Cleanup(func() {
		SetDiskCacheConfig(previousConfig)
	})

	storage, err := CreateBodyStorageFromReader(bytes.NewReader(nil), 0, -1)
	require.ErrorIs(t, err, ErrRequestBodyTooLarge)
	assert.Nil(t, storage)
}
