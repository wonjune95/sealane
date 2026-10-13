// Package assets embeds the built React SPA (Vite outputs to ./dist) and serves
// it with client-side-routing fallback to index.html.
package assets

import (
	"io"
	"io/fs"
	"net/http"
	"strings"

	"embed"
)

//go:embed all:dist
var distFS embed.FS

// Handler serves embedded static files; unknown paths fall back to index.html
// so React Router deep links (e.g. /routes/HTTPRoute/default/app) work on reload.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // build-time guarantee: dist exists
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		f, err := sub.Open(p)
		if err != nil {
			p = "index.html" // SPA fallback
			f, err = sub.Open(p)
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
		}
		defer f.Close() //nolint:errcheck // read-only embedded file
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			_ = f.Close()
			p = "index.html"
			if f, err = sub.Open(p); err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			defer f.Close() //nolint:errcheck // read-only embedded file
			st, _ = f.Stat()
		}
		rs, ok := f.(io.ReadSeeker)
		if !ok {
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, p, st.ModTime(), rs)
	})
}
