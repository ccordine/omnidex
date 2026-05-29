package api

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type browseEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
}

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	target := strings.TrimSpace(r.URL.Query().Get("path"))
	if target == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			writeError(w, http.StatusBadRequest, "home directory unavailable")
			return
		}
		target = home
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	abs = filepath.Clean(abs)
	if err := s.ensureBrowseAllowed(r, abs); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	stat, err := os.Stat(abs)
	if err != nil {
		writeError(w, http.StatusBadRequest, "path does not exist")
		return
	}
	if !stat.IsDir() {
		writeError(w, http.StatusBadRequest, "path must be a directory")
		return
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]browseEntry, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		full := filepath.Join(abs, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, browseEntry{
			Name:  entry.Name(),
			Path:  full,
			IsDir: info.IsDir(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	parent := ""
	if parentPath := filepath.Dir(abs); parentPath != abs {
		parent = parentPath
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":    abs,
		"parent":  parent,
		"entries": items,
	})
}

func (s *Server) ensureBrowseAllowed(r *http.Request, abs string) error {
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		home = filepath.Clean(home)
		if abs == home || strings.HasPrefix(abs, home+string(os.PathSeparator)) {
			return nil
		}
	}
	if s.repo != nil {
		projects, err := s.repo.ListProjects(r.Context(), 500, 0)
		if err == nil {
			for _, project := range projects {
				root := filepath.Clean(strings.TrimSpace(project.Location))
				if root == "" {
					continue
				}
				if abs == root || strings.HasPrefix(abs, root+string(os.PathSeparator)) || strings.HasPrefix(root, abs+string(os.PathSeparator)) {
					return nil
				}
			}
		}
	}
	return errBrowsePathNotAllowed
}

var errBrowsePathNotAllowed = &browsePathError{}

type browsePathError struct{}

func (e *browsePathError) Error() string {
	return "path outside allowed browse roots"
}
