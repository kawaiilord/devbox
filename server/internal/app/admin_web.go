package app

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed admin_web/*
var adminWebFiles embed.FS

func (s *Server) adminWebHandler() http.Handler {
	sub, err := fs.Sub(adminWebFiles, "admin_web")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/admin/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		files.ServeHTTP(w, r)
	})
}
