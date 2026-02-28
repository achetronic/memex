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

package middleware

import "context"

type namespaceKeyType struct{}

// WithNamespace stores the validated namespace in the context so handlers can
// retrieve it without parsing headers again.
func WithNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, namespaceKeyType{}, namespace)
}

// NamespaceFromContext retrieves the namespace stored by WithNamespace.
// Returns an empty string if no namespace is in context.
func NamespaceFromContext(ctx context.Context) string {
	ns, _ := ctx.Value(namespaceKeyType{}).(string)
	return ns
}
