package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/dist/*
var uiDistFiles embed.FS

//go:embed web/styles.css
var uiStylesCSS []byte

func (s *Server) registerUIRoutes() {
	webRoot, err := fs.Sub(uiDistFiles, "web/dist")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(webRoot))
	s.mux.Handle("/ui/", http.StripPrefix("/ui/", fileServer))
	s.mux.HandleFunc("/ui/styles.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		_, _ = w.Write(uiStylesCSS)
	})
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/chat" {
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, webRoot, "index.html")
	})
}
