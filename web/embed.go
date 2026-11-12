// Package web holds the browser UI. The files are embedded into the binary,
// so the server ships as a single executable.
package web

import "embed"

//go:embed *.html *.css *.js *.svg
var FS embed.FS
