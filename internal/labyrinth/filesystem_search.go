package labyrinth

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	MaxFilesystemSearchMatches = 16
	MaxFilesystemRGLineBytes   = 8 * 1024
)

type filesystemSearchMatch struct {
	ID            EntityID `json:"id"`
	Location      EntityID `json:"location"`
	Line          int      `json:"line"`
	Content       string   `json:"content"`
	ContentSHA256 string   `json:"content_sha256"`
}

func (surface *filesystemSurface) search(
	ctx context.Context,
	root string,
	state *filesystemState,
	query string,
) (any, error) {
	command := exec.CommandContext(
		ctx, surface.rgPath, "--json", "--fixed-strings", "--sort", "path", "--no-config", "--no-messages",
		"--", query, root,
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%w: create rg output: %v", ErrSurfaceOperation, err)
	}
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("%w: start rg --json: %v", ErrSurfaceOperation, err)
	}
	matches := make([]filesystemSearchMatch, 0, MaxFilesystemSearchMatches)
	truncated := false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), MaxFilesystemRGLineBytes)
	for scanner.Scan() {
		match, matched, decodeErr := decodeFilesystemRGMatch(root, state, scanner.Bytes())
		if decodeErr != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return nil, decodeErr
		}
		if !matched {
			continue
		}
		matches = append(matches, match)
		if len(matches) == MaxFilesystemSearchMatches {
			truncated = true
			_ = command.Process.Kill()
			break
		}
	}
	if err := scanner.Err(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, fmt.Errorf("%w: read bounded rg --json output: %v", ErrSurfaceLimit, err)
	}
	waitErr := command.Wait()
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if !truncated && waitErr != nil {
		var exitError *exec.ExitError
		if !errors.As(waitErr, &exitError) || exitError.ExitCode() != 1 {
			return nil, fmt.Errorf("%w: rg --json failed: %v", ErrSurfaceOperation, waitErr)
		}
	}
	return struct {
		Query     string                  `json:"query"`
		Matches   []filesystemSearchMatch `json:"matches"`
		Truncated bool                    `json:"truncated"`
	}{query, matches, truncated}, nil
}

func decodeFilesystemRGMatch(
	directory string,
	state *filesystemState,
	raw []byte,
) (filesystemSearchMatch, bool, error) {
	var event struct {
		Type string `json:"type"`
		Data struct {
			Path struct {
				Text string `json:"text"`
			} `json:"path"`
			Lines struct {
				Text string `json:"text"`
			} `json:"lines"`
			LineNumber int `json:"line_number"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return filesystemSearchMatch{}, false, fmt.Errorf("%w: decode rg --json event: %v", ErrSurfaceOperation, err)
	}
	if event.Type != "match" {
		return filesystemSearchMatch{}, false, nil
	}
	relative, err := filepath.Rel(directory, event.Data.Path.Text)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filesystemSearchMatch{}, false, fmt.Errorf("%w: rg returned a path outside its root", ErrSurfaceOperation)
	}
	location, name := filepath.Dir(relative), filepath.Base(relative)
	if location == "." || !safeSurfaceSegment(location) || !strings.HasSuffix(name, ".txt") {
		return filesystemSearchMatch{}, false, fmt.Errorf("%w: rg returned an unregistered file", ErrSurfaceOperation)
	}
	id := EntityID(strings.TrimSuffix(name, ".txt"))
	document := findDocument(state, id)
	content := strings.TrimSuffix(event.Data.Lines.Text, "\n")
	if document == nil || document.Location != EntityID(location) ||
		content != document.Content || event.Data.LineNumber < 1 {
		return filesystemSearchMatch{}, false, fmt.Errorf("%w: rg result does not match exact document state", ErrSurfaceOperation)
	}
	return filesystemSearchMatch{id, document.Location, event.Data.LineNumber, content, document.ContentSHA256}, true, nil
}
