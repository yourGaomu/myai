package relay

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web/*
var webAssets embed.FS

func (s *Server) webHandler() http.Handler {
	assets, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(assets))
}
