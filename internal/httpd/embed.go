package httpd

import (
	"io/fs"

	"github.com/liberide/serpent-seek/web"
)

// FS returns the embedded SvelteKit SPA bundle. The actual embed directive
// lives in package web (web/embed.go) because go:embed cannot reference files
// outside the package directory; this wrapper keeps the httpd-facing API.
func FS() fs.FS { return web.FS() }
