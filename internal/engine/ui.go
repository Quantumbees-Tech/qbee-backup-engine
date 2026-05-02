package engine

import (
	"embed"
	"net/http"
)

//go:embed web/*.html
var uiFS embed.FS

func serveEmbeddedHTML(w http.ResponseWriter, file string) {
	b, err := uiFS.ReadFile(file)
	if err != nil {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
