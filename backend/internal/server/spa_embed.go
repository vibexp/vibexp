//go:build embedfrontend

// Package server, embedfrontend build: embeds the built frontend SPA.
//
// This file is compiled ONLY with `-tags embedfrontend` (the combined release
// build — see backend/Dockerfile and `make build-combined`). It embeds the
// frontend build that the release pipeline copies into ./dist (this package's
// directory) before `go build`. The default build excludes this file and uses
// spa_noembed.go instead, so the backend compiles and runs without a built
// frontend (local dev + CI).
package server

import (
	"embed"
	"io/fs"
)

// embeddedDist holds the built frontend (the contents of frontend/dist copied
// to internal/server/dist by the release build). `all:` includes files whose
// names begin with `.` or `_` so nothing in the build is silently dropped.
//
//go:embed all:dist
var embeddedDist embed.FS

// embeddedSPAFS returns the embedded frontend rooted at the build output, or nil
// if the embed is unexpectedly empty/malformed.
func embeddedSPAFS() fs.FS {
	sub, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		return nil
	}
	return sub
}
