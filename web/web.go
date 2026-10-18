// Package web embeds the browser UI into the binary.
package web

import "embed"

//go:embed index.html app.js style.css
var Files embed.FS
