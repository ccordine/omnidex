package cognitionreplay

import (
	"bytes"
	"fmt"
)

func buildPages[T any](
	prefix string,
	kind EntryKind,
	values []T,
	sequence func(T) uint64,
) ([]containerFile, []ContainerEntry, error) {
	files := make([]containerFile, 0, (len(values)+maxPageItems-1)/maxPageItems)
	entries := make([]ContainerEntry, 0, cap(files))
	for offset, page := 0, 0; offset < len(values); page++ {
		end := offset + maxPageItems
		if end > len(values) {
			end = len(values)
		}
		body, err := marshalJSONLines(values[offset:end])
		if err != nil {
			return nil, nil, err
		}
		if len(body) == 0 || len(body) > maxPageBytes {
			return nil, nil, fmt.Errorf("replay %s page exceeds its hard byte limit", kind)
		}
		name := fmt.Sprintf("%s/page-%06d.jsonl", prefix, page)
		files = append(files, containerFile{name: name, body: body})
		entries = append(entries, ContainerEntry{
			Path: name, Kind: kind, SHA256: digestBytes(body), ByteCount: int64(len(body)),
			First: sequence(values[offset]), Last: sequence(values[end-1]), RecordCount: end - offset,
		})
		offset = end
	}
	return files, entries, nil
}

func marshalJSONLines[T any](values []T) ([]byte, error) {
	var result bytes.Buffer
	for index, value := range values {
		raw, err := marshalCanonical(value)
		if err != nil {
			return nil, fmt.Errorf("encode replay JSONL record %d: %w", index+1, err)
		}
		result.Write(raw)
	}
	return result.Bytes(), nil
}

func decodeJSONLines[T any](raw []byte, label string) ([]T, error) {
	if len(raw) == 0 || len(raw) > maxPageBytes || raw[len(raw)-1] != '\n' {
		return nil, fmt.Errorf("%s page framing is invalid", label)
	}
	lines := bytes.Split(raw[:len(raw)-1], []byte{'\n'})
	if len(lines) == 0 || len(lines) > maxPageItems {
		return nil, fmt.Errorf("%s page record count is invalid", label)
	}
	result := make([]T, len(lines))
	for index, line := range lines {
		if len(line) == 0 {
			return nil, fmt.Errorf("%s record %d is empty", label, index+1)
		}
		if err := decodeCanonical(append(bytes.Clone(line), '\n'), &result[index], label); err != nil {
			return nil, err
		}
	}
	return result, nil
}
