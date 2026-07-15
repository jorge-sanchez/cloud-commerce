// Package web embeds the built admin SPA. dist/ holds a committed
// placeholder so Go builds work without Node; the Dockerfile replaces it
// with the real Vite build before compiling (ADR-007).
package web

import "embed"

// Dist is the built SPA.
//
//go:embed all:dist
var Dist embed.FS
