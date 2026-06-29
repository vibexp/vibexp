//go:build !embedfrontend

// Package server, default build: the frontend SPA is NOT embedded.
//
// This is the build used by local development (`make backend-run-dev`), the test
// suite, and CI (`make backend-build` / `backend-test`): the backend compiles
// and runs with no built frontend/dist present, and the SPA is served by the
// Vite dev server. The combined release build replaces this with spa_embed.go
// via `-tags embedfrontend`.
package server

import "io/fs"

// embeddedSPAFS returns nil: no frontend is embedded in the default build, so
// handleSPA serves no static assets (config.js is still served from env vars).
func embeddedSPAFS() fs.FS { return nil }
