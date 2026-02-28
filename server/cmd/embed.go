// Copyright 2025 Alby Hernández
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
