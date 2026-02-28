package main

import (
	"embed"
	"io/fs"
)

// frontendFiles holds the embedded Vue dist/ directory.
// Path is relative to this file, which lives at server/cmd/.
// Before compiling, the build process copies frontend/dist to
// server/cmd/frontend_dist (make build or Dockerfile stage 2).
//
//go:embed frontend_dist
var frontendFiles embed.FS

// init sets the package-level frontend variable to the embedded dist directory
// so the router serves files from / without any prefix.
func init() {
	sub, err := fs.Sub(frontendFiles, "frontend_dist")
	if err != nil {
		panic("failed to sub frontend FS: " + err.Error())
	}
	frontend = sub
}
