package httpapi

import (
	"embed"
	"net/http"
)

//go:embed ui/index.html ui/app.js ui/style.css
var uiFiles embed.FS

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	serveUI(w, "index.html", "text/html; charset=utf-8")
}

func serveApplicationJS(w http.ResponseWriter, _ *http.Request) {
	serveUI(w, "app.js", "application/javascript; charset=utf-8")
}

func serveStyles(w http.ResponseWriter, _ *http.Request) {
	serveUI(w, "style.css", "text/css; charset=utf-8")
}

func serveUI(w http.ResponseWriter, name, mediaType string) {
	data, err := uiFiles.ReadFile("ui/" + name)
	if err != nil {
		http.Error(w, "UI unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'self'")
	_, _ = w.Write(data)
}
