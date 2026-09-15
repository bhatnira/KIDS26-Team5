//go:build embed

package main

import (
	"embed"
	"io/fs"

	"antelope/internal/modules/webui"
)

// embeddedUI holds the compiled Vue frontend. Build the frontend first
// (`cd web_src && pnpm run build`) so web_src/dist exists, then build the
// backend with `-tags embed`. Without the tag this file is excluded and the
// binary serves no frontend.
//
//go:embed all:web_src/dist
var embeddedUI embed.FS

func init() {
	sub, err := fs.Sub(embeddedUI, "web_src/dist")
	if err != nil {
		panic("webui: failed to open embedded web_src/dist: " + err.Error())
	}
	webui.Assets = sub
}
