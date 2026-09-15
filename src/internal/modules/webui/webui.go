// Package webui serves the compiled Vue frontend from the Go binary.
//
// The embedded assets are supplied by the root package's `embed` build-tagged
// file, which sets Assets during init. In a default (dev) build that file is
// excluded, Assets stays nil, and Enabled() reports false — so the backend
// behaves exactly as it did before, leaving the frontend to the Vite dev server.
package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// Assets is the root of the embedded frontend build (the contents of
// web_src/dist). It is nil unless the binary was built with `-tags embed`.
var Assets fs.FS

// Enabled reports whether an embedded frontend is available to serve.
func Enabled() bool { return Assets != nil }

// Handler serves the embedded single-page app: real files are served directly,
// and any other (non-API) path falls back to index.html so Vue Router history
// mode resolves on deep links and refreshes.
func Handler() gin.HandlerFunc {
	fileServer := http.FileServer(http.FS(Assets))
	index, err := fs.ReadFile(Assets, "index.html")
	if err != nil {
		panic("webui: embedded index.html missing: " + err.Error())
	}

	return func(c *gin.Context) {
		reqPath := c.Request.URL.Path
		// API/docs routes are owned by other handlers; never shadow them.
		if strings.HasPrefix(reqPath, "/api/") {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		name := strings.TrimPrefix(path.Clean("/"+reqPath), "/")
		if name != "" {
			if f, openErr := Assets.Open(name); openErr == nil {
				info, statErr := f.Stat()
				_ = f.Close()
				if statErr == nil && !info.IsDir() {
					fileServer.ServeHTTP(c.Writer, c.Request)
					return
				}
			}
		}

		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	}
}
