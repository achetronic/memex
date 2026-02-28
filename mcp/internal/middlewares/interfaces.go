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

// Package middlewares defines the ToolMiddleware and HttpMiddleware interfaces
// and all their concrete implementations.
package middlewares

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
)

// ToolMiddleware wraps a tool handler function, enabling pre/post processing
// of every tool call (auth checks, logging, rate limiting, etc.).
type ToolMiddleware interface {
	Middleware(next server.ToolHandlerFunc) server.ToolHandlerFunc
}

// HttpMiddleware wraps an http.Handler, enabling pre/post processing of every
// HTTP request before it reaches the MCP transport layer.
type HttpMiddleware interface {
	Middleware(next http.Handler) http.Handler
}
