package web

import "embed"

// distFS carries the built SPA. The dist directory is committed so plain
// `go build` / `go install` ship the real UI; `make build-web` (or the
// goreleaser before-hook) refreshes it from web/.
//
//go:embed all:dist
var distFS embed.FS
