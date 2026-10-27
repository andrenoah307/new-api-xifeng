package model

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type importStartedReader struct {
	io.Reader
	started   chan struct{}
	announced bool
}

func TestClickHouseImportReadbackDoesNotBlockIndependentPublication(t *testing.T) {
	db := setupClickHouseLogTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	type readbackContextKey struct{}
	blocked, release := make(chan struct{}), make(chan struct{})
	releaseReadback := sync.OnceFunc(func() { close(release) })
	defer releaseReadback()
	var reads atomic.Int32
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("test:block_import_readback", func(tx *gorm.DB) {
		if tx.Statement.Context.Value(readbackContextKey{}) != true || tx.Statement.Table != "logs" || reads.Add(1) != 2 {
			return
		}
		close(blocked)
		select {
		case <-release:
		case <-ctx.Done():
			tx.AddError(ctx.Err())
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Row().Remove("test:block_import_readback") })
	stamp := int64(1700000000)
	first, err := common.Marshal(ClickHouseLogArchiveRow{ID: 11, CreatedAt: &stamp})
	require.NoError(t, err)
	first = append(first, '\n')
	h := sha256.Sum256(first)
	manifest := ClickHouseLogImportManifest{Source: "readback", AfterID: 10, ThroughID: 11, Month: 202311, Rows: 1, SHA256: hex.EncodeToString(h[:])}
	done := make(chan error, 1)
	go func() {
		_, err := ImportClickHouseLogs(context.WithValue(ctx, readbackContextKey{}, true), db, manifest, bytes.NewReader(first))
		done <- err
	}()
	select {
	case <-blocked:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	second, err := common.Marshal(ClickHouseLogArchiveRow{ID: 12, CreatedAt: &stamp})
	require.NoError(t, err)
	second = append(second, '\n')
	other := manifest
	other.AfterID, other.ThroughID = 11, 12
	h = sha256.Sum256(second)
	other.SHA256 = hex.EncodeToString(h[:])
	_, err = ImportClickHouseLogs(ctx, db, other, bytes.NewReader(second))
	require.NoError(t, err)
	select {
	case <-done:
		t.Fatal("first import completed before its target verification")
	default:
	}
	releaseReadback()
	require.NoError(t, <-done)
	var count int64
	require.NoError(t, db.Table("logs").Count(&count).Error)
	assert.Equal(t, int64(2), count)
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
