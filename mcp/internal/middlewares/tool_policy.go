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
	"fmt"
	"strings"

	"memex-mcp/internal/globals"

	"github.com/google/cel-go/cel"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// CompiledToolPolicy holds a precompiled CEL program and the list of tools it
// grants access to when the expression evaluates to true.
type CompiledToolPolicy struct {
	Program      cel.Program
	AllowedTools []string
}

// CompiledNamespacePolicy holds a precompiled CEL program and the list of
// namespaces it grants access to when the expression evaluates to true.
type CompiledNamespacePolicy struct {
	Program            cel.Program
	AllowedNamespaces  []string
}

// ToolPolicyMiddlewareDependencies carries the dependencies required to build
// a ToolPolicyMiddleware.
type ToolPolicyMiddlewareDependencies struct {
	AppCtx *globals.ApplicationContext
}

// ToolPolicyMiddleware enforces tool-level and namespace-level access control
// based on JWT claims evaluated against CEL expressions configured in the
// policies section of the YAML config. It also validates that the namespace
// parameter supplied by the caller is allowed by at least one namespace policy.
type ToolPolicyMiddleware struct {
	dependencies         ToolPolicyMiddlewareDependencies
	compiledToolPolicies []CompiledToolPolicy
	compiledNsPolicies   []CompiledNamespacePolicy
}

// NewToolPolicyMiddleware compiles all CEL expressions from the configuration
// and returns a ready-to-use middleware. Returns an error if any expression
// fails to compile.
func NewToolPolicyMiddleware(deps ToolPolicyMiddlewareDependencies) (*ToolPolicyMiddleware, error) {
	mw := &ToolPolicyMiddleware{dependencies: deps}

	env, err := cel.NewEnv(
		cel.Variable("payload", cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("CEL environment creation error: %w", err)
	}

	// Compile tool policies
	for _, policy := range deps.AppCtx.Config.Policies.Tools {
		ast, issues := env.Compile(policy.Expression)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("CEL tool policy compilation error for %q: %w", policy.Expression, issues.Err())
		}
		prg, err := env.Program(ast)
		if err != nil {
			return nil, fmt.Errorf("CEL tool program construction error: %w", err)
		}
		mw.compiledToolPolicies = append(mw.compiledToolPolicies, CompiledToolPolicy{
			Program:      prg,
			AllowedTools: policy.AllowedTools,
		})
	}

	// Compile namespace policies
	for _, policy := range deps.AppCtx.Config.Policies.Namespaces {
		ast, issues := env.Compile(policy.Expression)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("CEL namespace policy compilation error for %q: %w", policy.Expression, issues.Err())
		}
		prg, err := env.Program(ast)
		if err != nil {
			return nil, fmt.Errorf("CEL namespace program construction error: %w", err)
		}
		mw.compiledNsPolicies = append(mw.compiledNsPolicies, CompiledNamespacePolicy{
			Program:           prg,
			AllowedNamespaces: policy.AllowedNamespaces,
		})
	}

	return mw, nil
}

// Middleware wraps a tool handler and enforces both tool and namespace policies
// before delegating to the next handler.
func (mw *ToolPolicyMiddleware) Middleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// When no policies are configured, allow everything.
		if len(mw.compiledToolPolicies) == 0 && len(mw.compiledNsPolicies) == 0 {
			return next(ctx, request)
		}

		payload, err := mw.extractJWTPayloadFromContext(ctx)
		if err != nil {
			mw.dependencies.AppCtx.Logger.Warn("could not extract JWT payload for policy check", "error", err.Error())
			return mcp.NewToolResultError("access denied: unable to verify permissions"), nil
		}

		toolName := request.Params.Name

		// --- Tool policy check ---
		if len(mw.compiledToolPolicies) > 0 {
			allowed := false
			for _, policy := range mw.compiledToolPolicies {
				out, _, err := policy.Program.Eval(map[string]interface{}{"payload": payload})
				if err != nil {
					mw.dependencies.AppCtx.Logger.Error("CEL tool policy evaluation error", "error", err.Error())
					continue
				}
				if out.Value() == true {
					if mw.isToolAllowed(toolName, policy.AllowedTools) {
						allowed = true
						break
					}
				}
			}
			if !allowed {
				mw.dependencies.AppCtx.Logger.Warn("tool access denied by policy", "tool", toolName)
				return mcp.NewToolResultError(fmt.Sprintf("access denied: no permission to use %q", toolName)), nil
			}
		}

		// --- Namespace policy check ---
		if len(mw.compiledNsPolicies) > 0 {
			namespace := mw.extractNamespace(request)
			if namespace != "" {
				nsAllowed := false
				for _, policy := range mw.compiledNsPolicies {
					out, _, err := policy.Program.Eval(map[string]interface{}{"payload": payload})
					if err != nil {
						mw.dependencies.AppCtx.Logger.Error("CEL namespace policy evaluation error", "error", err.Error())
						continue
					}
					if out.Value() == true {
						if mw.isNamespaceAllowed(namespace, policy.AllowedNamespaces) {
							nsAllowed = true
							break
						}
					}
				}
				if !nsAllowed {
					mw.dependencies.AppCtx.Logger.Warn("namespace access denied by policy",
						"tool", toolName,
						"namespace", namespace,
					)
					return mcp.NewToolResultError(fmt.Sprintf("access denied: no permission to access namespace %q", namespace)), nil
				}
			}
		}

		return next(ctx, request)
	}
}

// extractNamespace reads the "namespace" parameter from the tool call arguments.
// Returns an empty string if the parameter is absent or not a string.
func (mw *ToolPolicyMiddleware) extractNamespace(request mcp.CallToolRequest) string {
	if request.Params.Arguments == nil {
		return ""
	}
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return ""
	}
	ns, _ := args["namespace"].(string)
	return ns
}

// isToolAllowed checks whether toolName is covered by the allowedTools list.
// Supports exact matches and prefix wildcards (e.g. "search_*").
func (mw *ToolPolicyMiddleware) isToolAllowed(toolName string, allowedTools []string) bool {
	for _, allowed := range allowedTools {
		if allowed == "*" || allowed == toolName {
			return true
		}
		if strings.HasSuffix(allowed, "*") {
			if strings.HasPrefix(toolName, strings.TrimSuffix(allowed, "*")) {
				return true
			}
		}
	}
	return false
}

// isNamespaceAllowed checks whether namespace is covered by the allowedNamespaces list.
// Supports exact matches and the catch-all "*".
func (mw *ToolPolicyMiddleware) isNamespaceAllowed(namespace string, allowedNamespaces []string) bool {
	for _, allowed := range allowedNamespaces {
		if allowed == "*" || allowed == namespace {
			return true
		}
	}
	return false
}

// extractJWTPayloadFromContext retrieves the JWT claims map stored in the
// context by the JWT validation middleware.
func (mw *ToolPolicyMiddleware) extractJWTPayloadFromContext(ctx context.Context) (map[string]interface{}, error) {
	payload, ok := ctx.Value(JWTContextKey).(map[string]interface{})
	if !ok || payload == nil {
		return nil, fmt.Errorf("no JWT payload in context")
	}
	return payload, nil
}
