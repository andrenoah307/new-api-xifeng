package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type importStartedReader struct {
	io.Reader
	started   chan struct{}
	announced bool
}

func (r *importStartedReader) Read(p []byte) (int, error) {
	if !r.announced {
		close(r.started)
		r.announced = true
	}
	return r.Reader.Read(p)
}

func TestClickHouseImportStagesIndependentlyAndRejectsOverlap(t *testing.T) {
	db := setupClickHouseLogTest(t)
	stamp := int64(1700000000)
	input, err := common.Marshal(ClickHouseLogArchiveRow{ID: 11, CreatedAt: &stamp})
	require.NoError(t, err)
	input = append(input, '\n')
	h := sha256.Sum256(input)
	m := ClickHouseLogImportManifest{Source: "concurrent", AfterID: 10, ThroughID: 11, Month: 202311, Rows: 1, SHA256: hex.EncodeToString(h[:])}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		_, err := ImportClickHouseLogsWithOptions(ctx, db, m, &importStartedReader{Reader: r, started: started}, ClickHouseLogImportOptions{20000, 32 * 1024 * 1024})
		done <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// The pipe blocks the first batch while another independently stages and publishes.
	other := m
	other.AfterID = 0
	_, err = ImportClickHouseLogs(ctx, db, other, bytes.NewReader(input))
	require.NoError(t, err)
	_, writeErr := w.Write(input)
	require.NoError(t, writeErr)
	require.NoError(t, w.Close())
	require.ErrorContains(t, <-done, "overlaps")
	var count int64
	require.NoError(t, db.Table("logs").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
