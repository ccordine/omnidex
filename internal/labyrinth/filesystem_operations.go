package labyrinth

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const MaxFilesystemEntries = 64

type filesystemEntryResult struct {
	ID     EntityID `json:"id"`
	SHA256 string   `json:"sha256"`
}

func (surface *filesystemSurface) observe(
	root string,
	state *filesystemState,
	location EntityID,
) (any, error) {
	directory := filepath.Join(root, string(location))
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("%w: observe stage: %v", ErrSurfaceOperation, err)
	}
	result := struct {
		Location  EntityID                `json:"location"`
		Entries   []filesystemEntryResult `json:"entries"`
		Truncated bool                    `json:"truncated"`
	}{Location: location}
	for _, entry := range entries {
		if len(result.Entries) == MaxFilesystemEntries {
			result.Truncated = true
			break
		}
		if entry.IsDir() {
			return nil, fmt.Errorf("%w: nested directory is forbidden", ErrSurfaceOperation)
		}
		if !strings.HasSuffix(entry.Name(), ".txt") {
			return nil, fmt.Errorf("%w: projected entry has an invalid extension", ErrSurfaceOperation)
		}
		id := EntityID(entry.Name()[:len(entry.Name())-len(".txt")])
		document := findDocument(state, id)
		if document == nil {
			return nil, fmt.Errorf("%w: projected entry has no state identity", ErrSurfaceOperation)
		}
		result.Entries = append(result.Entries, filesystemEntryResult{id, document.ContentSHA256})
	}
	return result, nil
}

func (surface *filesystemSurface) read(
	root string,
	state *filesystemState,
	artifact EntityID,
) (any, error) {
	document := findDocument(state, artifact)
	if document == nil {
		return nil, ErrSurfacePrecondition
	}
	type entry struct {
		ID       EntityID `json:"id"`
		Location EntityID `json:"location"`
		Content  string   `json:"content"`
		SHA256   string   `json:"sha256"`
	}
	result := struct {
		Records []entry `json:"records"`
	}{Records: make([]entry, 1)}
	raw, err := readBoundedRegularFile(filesystemDocumentPath(root, *document), MaxFilesystemReadBytes)
	if err != nil {
		return nil, err
	}
	if textSHA256(string(raw)) != document.ContentSHA256 {
		return nil, fmt.Errorf("%w: read content hash does not match state", ErrSurfaceOperation)
	}
	result.Records[0] = entry{document.ID, document.Location, string(raw), document.ContentSHA256}
	return result, nil
}

func (surface *filesystemSurface) navigate(root string, from, to EntityID) (any, error) {
	for _, location := range []EntityID{from, to} {
		info, err := os.Stat(filepath.Join(root, string(location)))
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("%w: navigation endpoint is absent", ErrSurfaceOperation)
		}
	}
	return struct {
		From EntityID `json:"from"`
		To   EntityID `json:"to"`
	}{from, to}, nil
}

func (surface *filesystemSurface) take(
	root string,
	state *filesystemState,
	location EntityID,
	object EntityID,
) (any, error) {
	document := findDocument(state, object)
	if document == nil || document.Location != location || containsEntityID(state.Inventory, object) {
		return nil, ErrSurfacePrecondition
	}
	if _, err := readBoundedRegularFile(filesystemDocumentPath(root, *document), MaxFilesystemReadBytes); err != nil {
		return nil, err
	}
	state.Inventory = append(state.Inventory, document.ID)
	sort.Slice(state.Inventory, func(left, right int) bool { return state.Inventory[left] < state.Inventory[right] })
	return filesystemEntryResult{document.ID, document.ContentSHA256}, nil
}

func (surface *filesystemSurface) use(
	state *filesystemState,
	item EntityID,
	target EntityID,
	current EntityID,
) (any, error) {
	if target != current || !containsEntityID(state.Inventory, item) || containsEntityID(state.Used, item) {
		return nil, ErrSurfacePrecondition
	}
	state.Used = append(state.Used, item)
	sort.Slice(state.Used, func(left, right int) bool { return state.Used[left] < state.Used[right] })
	return struct {
		Item   EntityID `json:"item"`
		Target EntityID `json:"target"`
	}{item, target}, nil
}

func (surface *filesystemSurface) write(
	root string,
	state *filesystemState,
	location EntityID,
	target EntityID,
	expected string,
	value string,
) (any, error) {
	document := findDocument(state, target)
	if document == nil || document.Location != location || !validSymbol(value) {
		return nil, ErrSurfacePrecondition
	}
	path := filesystemDocumentPath(root, *document)
	current, err := readBoundedRegularFile(path, MaxFilesystemReadBytes)
	if err != nil || textSHA256(string(current)) != document.ContentSHA256 || expected != document.ContentSHA256 {
		return nil, fmt.Errorf("%w: write expected content hash is stale", ErrSurfacePrecondition)
	}
	previous := document.ContentSHA256
	content := value
	if len(content) > MaxFilesystemReadBytes {
		return nil, ErrSurfaceLimit
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return nil, fmt.Errorf("%w: write bounded content: %v", ErrSurfaceOperation, err)
	}
	written, err := readBoundedRegularFile(path, MaxFilesystemReadBytes)
	if err != nil || string(written) != content {
		return nil, fmt.Errorf("%w: verify bounded write", ErrSurfaceOperation)
	}
	document.Content = content
	document.ContentSHA256 = textSHA256(content)
	return struct {
		ID             EntityID `json:"id"`
		PreviousSHA256 string   `json:"previous_sha256"`
		CurrentSHA256  string   `json:"current_sha256"`
	}{document.ID, previous, document.ContentSHA256}, nil
}

func findDocument(state *filesystemState, id EntityID) *filesystemDocument {
	for index := range state.Documents {
		if state.Documents[index].ID == id {
			return &state.Documents[index]
		}
	}
	return nil
}

func documentsAt(state *filesystemState, location EntityID) []*filesystemDocument {
	result := make([]*filesystemDocument, 0)
	for index := range state.Documents {
		if state.Documents[index].Location == location {
			result = append(result, &state.Documents[index])
		}
	}
	return result
}

func containsEntityID(values []EntityID, expected EntityID) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
