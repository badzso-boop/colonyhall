package httpserver

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// appHandler serves the PWA build (web/dist) under /app/. Hashed files in
// assets/ are cached for a year; everything else, and index.html above all,
// is revalidated so a deploy shows up on the next load. Paths that are not
// files fall back to index.html (client-side routing), except paths that
// look like files (assets/ or an extension), which get a real 404.
func appHandler(dist fs.FS) http.Handler {
	files := http.FileServerFS(dist)
	return http.StripPrefix("/app", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "index.html" {
			serveAppIndex(w, dist)
			return
		}
		if st, err := fs.Stat(dist, name); err == nil && !st.IsDir() && name != ".keep" {
			if strings.HasPrefix(name, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			files.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(name, "assets/") || path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		serveAppIndex(w, dist)
	}))
}

func serveAppIndex(w http.ResponseWriter, dist fs.FS) {
	page, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		http.Error(w, "The PWA is not built: run `make web`.", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(page)
}
