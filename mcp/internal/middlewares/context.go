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

package middlewares

import (
	"context"
	"net/http"
)

// forwardedHeadersKey is the context key used to store forwarded HTTP headers
// so tool handlers can retrieve them without depending on mcp-go internals.
type forwardedHeadersKey struct{}

// WithForwardedHeaders stores the given http.Header map into the context.
// The HTTP middleware calls this so tool handlers can read forwarded values.
func WithForwardedHeaders(ctx context.Context, headers http.Header) context.Context {
	return context.WithValue(ctx, forwardedHeadersKey{}, headers)
}

// ForwardedHeadersFromContext retrieves the forwarded headers stored by
// WithForwardedHeaders. Returns nil if no headers were stored.
func ForwardedHeadersFromContext(ctx context.Context) http.Header {
	h, _ := ctx.Value(forwardedHeadersKey{}).(http.Header)
	return h
}
