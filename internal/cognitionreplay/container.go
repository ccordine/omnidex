package cognitionreplay

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"
)

const manifestPath = "manifest.json"

type containerFile struct {
	name string
	body []byte
}

func encodeContainer(files []containerFile) ([]byte, error) {
	if len(files) == 0 || files[0].name != manifestPath {
		return nil, fmt.Errorf("replay container must begin with its manifest")
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		if err := validateContainerPath(file.name); err != nil {
			_ = writer.Close()
			return nil, err
		}
		if _, duplicate := seen[file.name]; duplicate {
			_ = writer.Close()
			return nil, fmt.Errorf("replay container entry %q is duplicated", file.name)
		}
		seen[file.name] = struct{}{}
		header := &zip.FileHeader{Name: file.name, Method: zip.Store}
		header.SetMode(0o600)
		header.ModifiedDate = 0x21 // 1980-01-01, the earliest DOS date.
		handle, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("create replay entry %q: %w", file.name, err)
		}
		if _, err := handle.Write(file.body); err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("write replay entry %q: %w", file.name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close replay container: %w", err)
	}
	if buffer.Len() <= 0 || buffer.Len() > maxContainerBytes {
		return nil, fmt.Errorf("replay container exceeds its hard byte limit")
	}
	return buffer.Bytes(), nil
}

func decodeContainer(raw []byte) ([]containerFile, error) {
	if len(raw) == 0 || len(raw) > maxContainerBytes {
		return nil, fmt.Errorf("replay container byte count is invalid")
	}
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("open replay container: %w", err)
	}
	if len(reader.File) == 0 || len(reader.File) > maxBlobs+maxSources+maxEvents+maxCheckpoints {
		return nil, fmt.Errorf("replay container entry count is invalid")
	}
	files := make([]containerFile, len(reader.File))
	seen := make(map[string]struct{}, len(reader.File))
	for index, file := range reader.File {
		if err := validateContainerFile(file); err != nil {
			return nil, fmt.Errorf("replay container entry %d: %w", index+1, err)
		}
		if _, duplicate := seen[file.Name]; duplicate {
			return nil, fmt.Errorf("replay container entry %q is duplicated", file.Name)
		}
		seen[file.Name] = struct{}{}
		handle, err := file.Open()
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(handle, maxBlobBytes+maxPageBytes+1))
		closeErr := handle.Close()
		if readErr != nil || closeErr != nil {
			return nil, fmt.Errorf("read replay entry %q: %v / %v", file.Name, readErr, closeErr)
		}
		if uint64(len(body)) != file.UncompressedSize64 || len(body) == 0 ||
			len(body) > maxBlobBytes+maxPageBytes {
			return nil, fmt.Errorf("replay entry %q byte count is invalid", file.Name)
		}
		files[index] = containerFile{name: file.Name, body: body}
	}
	if files[0].name != manifestPath {
		return nil, fmt.Errorf("replay container manifest is not first")
	}
	rebuilt, err := encodeContainer(files)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(raw, rebuilt) {
		return nil, fmt.Errorf("replay container metadata or byte encoding is not canonical")
	}
	return files, nil
}

func validateContainerFile(file *zip.File) error {
	if file == nil || validateContainerPath(file.Name) != nil || file.Method != zip.Store ||
		file.Comment != "" || file.NonUTF8 || file.ModifiedTime != 0 || file.ModifiedDate != 0x21 ||
		file.Mode().Perm() != 0o600 || file.Mode()&^0o777 != 0 || len(file.Extra) != 0 {
		return fmt.Errorf("entry metadata is not canonical")
	}
	return nil
}

func validateContainerPath(value string) error {
	if value == "" || len(value) > 512 || value != path.Clean(value) ||
		strings.HasPrefix(value, "/") || strings.HasPrefix(value, "../") ||
		strings.Contains(value, "\\") || strings.ContainsRune(value, 0) {
		return fmt.Errorf("replay container path %q is unsafe", value)
	}
	return nil
}
