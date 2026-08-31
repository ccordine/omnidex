package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/hostbridge"
	"github.com/gryph/omnidex/internal/queue"
)

func splitProjectPath(path string) (id int64, action string) {
	const prefix = "/v1/projects/"
	if !strings.HasPrefix(path, prefix) {
		return 0, ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || strings.HasPrefix(rest, "/") || strings.HasSuffix(rest, "/") {
		return 0, ""
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || len(parts) > 3 || len(parts[0]) > 20 ||
		parts[0] != strings.TrimSpace(parts[0]) || !utf8.ValidString(parts[0]) ||
		strings.ContainsRune(parts[0], '\x00') {
		return 0, ""
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != parts[0] {
		return 0, ""
	}
	if len(parts) > 1 {
		for _, part := range parts[1:] {
			if part == "" || part != strings.TrimSpace(part) || !utf8.ValidString(part) ||
				strings.ContainsRune(part, '\x00') {
				return 0, ""
			}
		}
		action = strings.Join(parts[1:], "/")
		if len(action) > 64 {
			return 0, ""
		}
	}
	return id, action
}

func writeProjectError(w http.ResponseWriter, err error) {
	if errors.Is(err, queue.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, queue.ErrProjectVersionConflict) || errors.Is(err, queue.ErrProjectActiveWork) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

func (s *Server) validateProjectLocation(ctx context.Context, raw string) (string, error) {
	location, err := queue.NormalizeProjectLocation(raw)
	if err != nil {
		return "", err
	}
	if stat, err := os.Stat(location); err == nil {
		if stat.IsDir() {
			return location, nil
		}
		return "", fmt.Errorf("location must be an existing directory")
	}
	client := s.hostBridgeClient()
	if client == nil {
		return "", fmt.Errorf("location must be an existing directory")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := client.Browse(ctx, location, hostbridge.BrowseOptions{Limit: 1})
	if err != nil {
		return "", fmt.Errorf("location must be an existing directory")
	}
	if result == nil || strings.TrimSpace(result.Path) == "" {
		return "", fmt.Errorf("location must be an existing directory")
	}
	return filepath.Clean(result.Path), nil
}
