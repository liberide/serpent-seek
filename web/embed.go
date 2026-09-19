// Package web embeds the compiled SvelteKit SPA bundle.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:build
var buildFS embed.FS

// FS returns the embedded SPA rooted at the build directory.
func FS() fs.FS {
	sub, err := fs.Sub(buildFS, "build")
	if err != nil {
		return buildFS
	}
	return sub
}
