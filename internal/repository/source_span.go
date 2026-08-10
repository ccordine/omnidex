package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type SourceSpan struct {
	SubjectID    string `json:"subject_id"`
	FileID       string `json:"file_id"`
	SourceSHA256 string `json:"source_sha256"`
	StartByte    int64  `json:"start_byte"`
	EndByte      int64  `json:"end_byte"`
	Content      string `json:"content"`
}

func ReadExactSymbolSpan(snapshot Snapshot, symbol Symbol, maxBytes int) (SourceSpan, error) {
	if maxBytes < 1 {
		return SourceSpan{}, fmt.Errorf("repository source span requires a positive byte limit")
	}
	if err := snapshot.Validate(); err != nil {
		return SourceSpan{}, err
	}
	file, err := exactSymbolFile(snapshot, symbol)
	if err != nil {
		return SourceSpan{}, err
	}
	length := symbol.EndByte - symbol.StartByte
	if length > int64(maxBytes) {
		return SourceSpan{}, fmt.Errorf("repository source span for %q exceeds %d bytes", symbol.ID, maxBytes)
	}
	absolute := filepath.Join(snapshot.Root, filepath.FromSlash(file.Path))
	handle, err := os.Open(absolute)
	if err != nil {
		return SourceSpan{}, fmt.Errorf("open repository source for %q: %w", symbol.ID, err)
	}
	defer handle.Close()
	before, err := handle.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return SourceSpan{}, fmt.Errorf("repository source for %q is not an exact regular file", symbol.ID)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, handle); err != nil {
		return SourceSpan{}, fmt.Errorf("hash repository source for %q: %w", symbol.ID, err)
	}
	if digest := hex.EncodeToString(hash.Sum(nil)); digest != file.SHA256 {
		return SourceSpan{}, fmt.Errorf("repository source for %q is stale; refresh the snapshot", symbol.ID)
	}
	content := make([]byte, length)
	if length > 0 {
		if _, err := handle.ReadAt(content, symbol.StartByte); err != nil {
			return SourceSpan{}, fmt.Errorf("read repository source span for %q: %w", symbol.ID, err)
		}
	}
	after, err := handle.Stat()
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return SourceSpan{}, fmt.Errorf("repository source for %q changed while it was read", symbol.ID)
	}
	return SourceSpan{
		SubjectID: symbol.ID, FileID: file.ID, SourceSHA256: file.SHA256,
		StartByte: symbol.StartByte, EndByte: symbol.EndByte, Content: string(content),
	}, nil
}

func exactSymbolFile(snapshot Snapshot, symbol Symbol) (File, error) {
	for _, file := range snapshot.Files {
		if file.ID != symbol.FileID {
			continue
		}
		if file.Kind != EntryRegular || symbol.SourceSHA256 != file.SHA256 ||
			symbol.StartByte < 0 || symbol.EndByte < symbol.StartByte || symbol.EndByte > file.Size {
			return File{}, fmt.Errorf("repository symbol %q is not bound to an exact regular file span", symbol.ID)
		}
		return file, nil
	}
	return File{}, fmt.Errorf("repository symbol %q references an unknown file", symbol.ID)
}
