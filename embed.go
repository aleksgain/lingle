// Package lingle holds the embedded assets. Go's embed directive cannot reach
// above its own directory, so the web client and the language packs are
// declared here at the module root and used from cmd/lingle.
package lingle

import "embed"

// WebFS is the browser client: HTML, CSS, JS, icon, manifest.
//
//go:embed all:web
var WebFS embed.FS

// PacksFS is the language packs, including the word lists. These are read by
// the server and never served to a browser.
//
//go:embed packs
var PacksFS embed.FS
