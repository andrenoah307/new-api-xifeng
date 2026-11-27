package service

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image/png"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

const InvoiceArchiveMaxSize = 25 << 20
const invoiceImageMaxSize = 10 << 20
const invoiceArchiveExpandedMax = 100 << 20

var invoiceFilePattern = regexp.MustCompile(`^([1-9][0-9]*)(?: ?\([1-9][0-9]*\)| ?（[1-9][0-9]*）)?(?i:\.png)$`)
var ErrInvoiceArchive = errors.New("invalid invoice archive")

type InvoiceArchiveFile struct {
	TicketID int
	Name     string
	Path     string
	Digest   string
	Size     int64
}

// ExtractInvoiceArchive uses a private directory owned by the caller. No archive
// path is used before checking traversal, links, duplicate paths and size limits.
func ExtractInvoiceArchive(reader io.ReaderAt, size int64, directory string) ([]InvoiceArchiveFile, []string, error) {
	if size <= 0 || size > InvoiceArchiveMaxSize {
		return nil, nil, ErrInvoiceArchive
	}
	zr, err := zip.NewReader(reader, size)
	if err != nil || len(zr.File) > 1000 {
		return nil, nil, ErrInvoiceArchive
	}
	var total int64
	seen := make(map[string]bool)
	for _, entry := range zr.File {
		name := strings.TrimSuffix(entry.Name, "/")
		if !utf8.ValidString(name) {
			// Windows invoice exports commonly use GBK instead of ZIP's UTF-8
			// flag. Decode before path validation and filename matching.
			decoded, err := simplifiedchinese.GB18030.NewDecoder().String(name)
			if err != nil || strings.ContainsRune(decoded, utf8.RuneError) {
				return nil, nil, ErrInvoiceArchive
			}
			name = decoded
		}
		if name == "" || len(name) > 512 || strings.ContainsAny(name, "\\\x00:") || strings.HasPrefix(name, "/") || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || entry.Mode()&os.ModeSymlink != 0 || (!entry.FileInfo().IsDir() && !entry.Mode().IsRegular()) || seen[name] {
			return nil, nil, ErrInvoiceArchive
		}
		seen[name] = true
		if entry.FileInfo().IsDir() {
			continue
		}
		if entry.UncompressedSize64 > invoiceImageMaxSize || entry.UncompressedSize64 > uint64(invoiceArchiveExpandedMax-total) {
			return nil, nil, ErrInvoiceArchive
		}
		target := filepath.Join(directory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, nil, err
		}
		src, err := entry.Open()
		if err != nil {
			return nil, nil, ErrInvoiceArchive
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			src.Close()
			return nil, nil, err
		}
		n, copyErr := io.Copy(dst, io.LimitReader(src, invoiceImageMaxSize+1))
		closeErr := dst.Close()
		src.Close()
		if copyErr != nil || closeErr != nil || n > invoiceImageMaxSize || uint64(n) != entry.UncompressedSize64 {
			return nil, nil, ErrInvoiceArchive
		}
		total += n
	}
	files := []InvoiceArchiveFile{}
	ignored := []string{}
	counts, sizes := make(map[int]int), make(map[int]int64)
	err = filepath.WalkDir(directory, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, _ := filepath.Rel(directory, filename)
		match := invoiceFilePattern.FindStringSubmatch(entry.Name())
		if match == nil || strings.HasPrefix(filepath.ToSlash(rel), "__MACOSX/") {
			ignored = append(ignored, filepath.ToSlash(rel))
			return nil
		}
		id, err := strconv.Atoi(match[1])
		if err != nil || id <= 0 {
			return ErrInvoiceArchive
		}
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer file.Close()
		cfg, err := png.DecodeConfig(file)
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 16000000 {
			return ErrInvoiceArchive
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		if _, err = png.Decode(file); err != nil {
			return ErrInvoiceArchive
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		hash := sha256.New()
		n, err := io.Copy(hash, file)
		if err != nil {
			return err
		}
		digest := hex.EncodeToString(hash.Sum(nil))
		counts[id]++
		sizes[id] += n
		if len(counts) > 200 || counts[id] > 20 || sizes[id] > invoiceImageMaxSize {
			return ErrInvoiceArchive
		}
		files = append(files, InvoiceArchiveFile{TicketID: id, Name: entry.Name(), Path: filename, Digest: digest, Size: n})
		return nil
	})
	if err != nil || len(files) == 0 {
		return nil, nil, ErrInvoiceArchive
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].TicketID == files[j].TicketID {
			if files[i].Digest == files[j].Digest {
				return files[i].Name < files[j].Name
			}
			return files[i].Digest < files[j].Digest
		}
		return files[i].TicketID < files[j].TicketID
	})
	return files, ignored, nil
}
