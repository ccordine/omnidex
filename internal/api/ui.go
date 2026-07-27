package api

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
)

//go:embed web/dist/*
var uiDistFiles embed.FS

var (
	localizedUIShellsOnce sync.Once
	localizedUIShells     map[uiLocale]string
	localizedUIShellsErr  error
)

func (s *Server) registerUIRoutes() {
	webRoot, err := fs.Sub(uiDistFiles, "web/dist")
	if err != nil {
		panic(fmt.Sprintf("initialize embedded UI filesystem: %v", err))
	}
	shells, err := prepareLocalizedUIShells(webRoot)
	if err != nil {
		panic(fmt.Sprintf("prepare localized UI shells: %v", err))
	}
	if err := prepareLocalizedUIPanels(); err != nil {
		panic(fmt.Sprintf("prepare localized UI panels: %v", err))
	}
	fileServer := http.FileServer(http.FS(webRoot))
	s.mux.Handle("/ui/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ui/" || r.URL.Path == "/ui/index.html" {
			http.NotFound(w, r)
			return
		}
		http.StripPrefix("/ui/", fileServer).ServeHTTP(w, r)
	}))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/chat" {
			http.NotFound(w, r)
			return
		}
		s.handleUIShell(w, r, shells)
	})
}

func (s *Server) handleUIShell(w http.ResponseWriter, r *http.Request, shells map[uiLocale]string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessionID := s.ensureUISessionCookie(w, r)
	state, _, err := s.loadUIState(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err := mergeUIQueryState(state, r); err != nil {
		logUILocaleRejection(sessionID, "shell", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	locale, err := ensureUIStateLocale(state, r)
	if err != nil {
		logUILocaleRejection(sessionID, "shell", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.persistUIState(r.Context(), sessionID, state); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	body, exists := shells[locale]
	if !exists {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("localized UI shell %q is unavailable", locale))
		return
	}
	setUILocaleResponseHeaders(w, locale)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write([]byte(body))
}

func prepareLocalizedUIShells(webRoot fs.FS) (map[uiLocale]string, error) {
	localizedUIShellsOnce.Do(func() {
		raw, err := fs.ReadFile(webRoot, "index.html")
		if err != nil {
			localizedUIShellsErr = fmt.Errorf("embedded UI shell is missing: %w", err)
			return
		}
		localizedUIShells = make(map[uiLocale]string, len(supportedUILocaleOptions))
		for _, option := range supportedUILocaleOptions {
			rendered, err := renderLocalizedHTML(string(raw), option.Code)
			if err != nil {
				localizedUIShellsErr = fmt.Errorf("render UI shell locale %q: %w", option.Code, err)
				return
			}
			localizedUIShells[option.Code] = rendered
		}
	})
	return localizedUIShells, localizedUIShellsErr
}
