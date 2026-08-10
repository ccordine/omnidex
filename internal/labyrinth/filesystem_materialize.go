package labyrinth

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const MaxFilesystemReadBytes = MaxPublicRecordContentBytes

func (surface *filesystemSurface) materialize(
	state *filesystemState,
) (string, func(), error) {
	root, err := os.MkdirTemp(surface.root, "operation-")
	if err != nil {
		return "", nil, fmt.Errorf("%w: create operation root: %v", ErrSurfaceOperation, err)
	}
	cleanup := func() { _ = os.RemoveAll(root) }
	for _, stage := range surface.stages {
		if err := os.Mkdir(filepath.Join(root, string(stage)), 0o700); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("%w: create stage projection: %v", ErrSurfaceOperation, err)
		}
	}
	for _, document := range state.Documents {
		path := filepath.Join(root, string(document.Location), string(document.ID)+".txt")
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("%w: create document projection: %v", ErrSurfaceOperation, err)
		}
		_, writeErr := io.WriteString(file, document.Content)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			cleanup()
			return "", nil, fmt.Errorf("%w: write document projection", ErrSurfaceOperation)
		}
	}
	return root, cleanup, nil
}

func readBoundedRegularFile(path string, maximum int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open bounded file: %v", ErrSurfaceOperation, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: bounded file is not regular", ErrSurfaceOperation)
	}
	raw, err := io.ReadAll(io.LimitReader(file, int64(maximum+1)))
	if err != nil {
		return nil, fmt.Errorf("%w: read bounded file: %v", ErrSurfaceOperation, err)
	}
	if len(raw) > maximum {
		return nil, fmt.Errorf("%w: file exceeds %d bytes", ErrSurfaceLimit, maximum)
	}
	return raw, nil
}

func filesystemDocumentPath(root string, document filesystemDocument) string {
	return filepath.Join(root, string(document.Location), string(document.ID)+".txt")
}

func firstDocumentAt(state *filesystemState, location EntityID) (*filesystemDocument, error) {
	for index := range state.Documents {
		if state.Documents[index].Location == location {
			return &state.Documents[index], nil
		}
	}
	return nil, ErrSurfacePrecondition
}
