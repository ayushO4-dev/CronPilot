package server

import (
	"net/http"
	"strings"
)

// spaHandler serves the embedded frontend. Real files are served directly;
// unknown paths fall back to index.html so client-side routing works. If the
// frontend has not been built (only the .gitkeep is embedded), it returns a
// helpful message instead of a blank 404.
func (s *Server) spaHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.webFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := s.webFS.Open(p); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback to index.html for client-side routes.
		if f, err := s.webFS.Open("index.html"); err == nil {
			f.Close()
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		http.Error(w, "frontend not built — run `make web` (see README)", http.StatusServiceUnavailable)
	})
}
