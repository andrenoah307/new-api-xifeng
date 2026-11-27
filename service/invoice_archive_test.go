package service

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestInvoiceArchiveRecursiveGrouping(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for i, name := range []string{"nested/invoices/123.png", "nested/123（2）.png", "other/456.png", "notes.txt"} {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		picture := image.NewRGBA(image.Rect(0, 0, 2, 2))
		picture.Set(0, 0, color.RGBA{R: uint8(i + 1), A: 255})
		require.NoError(t, png.Encode(entry, picture))
	}
	require.NoError(t, writer.Close())
	files, ignored, err := ExtractInvoiceArchive(bytes.NewReader(archive.Bytes()), int64(archive.Len()), t.TempDir())
	require.NoError(t, err)
	require.Len(t, files, 3)
	assert.Equal(t, 123, files[0].TicketID)
	assert.Equal(t, 123, files[1].TicketID)
	assert.Equal(t, 456, files[2].TicketID)
	assert.Equal(t, []string{"notes.txt"}, ignored)
	for _, file := range files {
		info, err := os.Stat(file.Path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}

func TestInvoiceArchiveRejectsUnsafeEntries(t *testing.T) {
	for _, name := range []string{"../123.png", "/123.png", "dir/../../123.png", "dir\\123.png", "C:/123.png", "123.png", "symlink"} {
		t.Run(name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := zip.NewWriter(&archive)
			header := &zip.FileHeader{Name: name, Method: zip.Store}
			if name == "symlink" {
				header.SetMode(os.ModeSymlink | 0777)
			}
			entry, err := writer.CreateHeader(header)
			require.NoError(t, err)
			_, err = entry.Write([]byte("not a PNG"))
			require.NoError(t, err)
			require.NoError(t, writer.Close())
			_, _, err = ExtractInvoiceArchive(bytes.NewReader(archive.Bytes()), int64(archive.Len()), t.TempDir())
			require.Error(t, err)
		})
	}
}

func TestInvoiceArchiveRejectsDuplicatePathsAndOversize(t *testing.T) {
	for _, duplicate := range []bool{true, false} {
		var archive bytes.Buffer
		writer := zip.NewWriter(&archive)
		entry, err := writer.Create("123.png")
		require.NoError(t, err)
		if duplicate {
			require.NoError(t, png.Encode(entry, image.NewRGBA(image.Rect(0, 0, 1, 1))))
			_, err = writer.Create("123.png")
			require.NoError(t, err)
		} else {
			_, err = entry.Write(bytes.Repeat([]byte{0}, invoiceImageMaxSize+1))
			require.NoError(t, err)
		}
		require.NoError(t, writer.Close())
		_, _, err = ExtractInvoiceArchive(bytes.NewReader(archive.Bytes()), int64(archive.Len()), t.TempDir())
		require.Error(t, err)
	}
}

func TestInvoiceArchivePreservesSeparatelyNamedIdenticalImages(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range []string{"123.png", "123（2）.png"} {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		require.NoError(t, png.Encode(entry, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	}
	require.NoError(t, writer.Close())
	files, ignored, err := ExtractInvoiceArchive(bytes.NewReader(archive.Bytes()), int64(archive.Len()), t.TempDir())
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Empty(t, ignored)
	assert.Equal(t, files[0].Digest, files[1].Digest)
}

func TestInvoiceArchiveWindowsChineseNames(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range []string{"发票/123.png", "发票/123（2）.png"} {
		encoded, err := simplifiedchinese.GB18030.NewEncoder().String(name)
		require.NoError(t, err)
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: encoded, NonUTF8: true, Method: zip.Store})
		require.NoError(t, err)
		require.NoError(t, png.Encode(entry, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	}
	require.NoError(t, writer.Close())
	files, ignored, err := ExtractInvoiceArchive(bytes.NewReader(archive.Bytes()), int64(archive.Len()), t.TempDir())
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Empty(t, ignored)
	assert.Equal(t, "123（2）.png", files[1].Name)
}
