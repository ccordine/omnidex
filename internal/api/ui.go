package api

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
)

//go:embed web/dist/*
var uiDistFiles embed.FS

func (s *Server) registerUIRoutes() {
	s.mux.Handle("/ui/", http.HandlerFunc(serveUIAsset))
	s.mux.HandleFunc("/", s.handleUIShell)
}

func serveUIAsset(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/ui/" || r.URL.Path == "/ui/index.html" {
		http.NotFound(w, r)
		return
	}
	webRoot, err := fs.Sub(uiDistFiles, "web/dist")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("initialize embedded UI filesystem: %v", err))
		return
	}
	fileServer := http.FileServer(http.FS(webRoot))
	http.StripPrefix("/ui/", fileServer).ServeHTTP(w, r)
}

func (s *Server) handleUIShell(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/chat" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessionID, err := s.ensureUISessionCookie(w, r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	body, err := loadLocalizedUIShell(locale)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

func loadLocalizedUIShell(locale uiLocale) (string, error) {
	webRoot, err := fs.Sub(uiDistFiles, "web/dist")
	if err != nil {
		return "", fmt.Errorf("initialize embedded UI filesystem: %w", err)
	}
	raw, err := fs.ReadFile(webRoot, "index.html")
	if err != nil {
		return "", fmt.Errorf("embedded UI shell is missing: %w", err)
	}
	rendered, err := renderLocalizedHTML(string(raw), locale)
	if err != nil {
		return "", fmt.Errorf("render UI shell locale %q: %w", locale, err)
	}
	return rendered, nil
}
