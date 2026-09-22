// Package ui embeds the DocViewer static admin console.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed static
var content embed.FS

// FS returns the embedded static files rooted at the "static" dir.
func FS() fs.FS {
	sub, err := fs.Sub(content, "static")
	if err != nil {
		panic(err)
	}
	return sub
}