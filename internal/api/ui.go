package api

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var uiFiles embed.FS

func (s *Server) registerUIRoutes() {
	webRoot, err := fs.Sub(uiFiles, "web")
	if err != nil {
		return
	}
	fileServer := http.FileServer(http.FS(webRoot))
	s.mux.Handle("/ui/", http.StripPrefix("/ui/", fileServer))
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/chat" {
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, webRoot, "index.html")
	})
}
