package dashboard

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:web/dist
var staticFS embed.FS

// staticHandler serves the embedded frontend bundle. Until the React
// bundle exists (B3) this serves the placeholder index.html that lists
// the available API endpoints.
func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "web/dist")
	if err != nil {
		// Should never happen — the embed is compile-time verified.
		panic(err)
	}
	return http.FileServer(http.FS(sub))
}
