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

// CompiledPolicy holds a precompiled CEL program alongside the tool and
// namespace constraints it enforces when the expression evaluates to true.
// Empty slices mean "no restriction on that dimension".
type CompiledPolicy struct {
	Program           cel.Program
	AllowedTools      []string
	AllowedNamespaces []string
}

// ToolPolicyMiddlewareDependencies carries the dependencies required to build
// a ToolPolicyMiddleware.
type ToolPolicyMiddlewareDependencies struct {
	AppCtx *globals.ApplicationContext
}

// ToolPolicyMiddleware enforces access control based on JWT claims evaluated
// against CEL expressions configured in the policies.rules section of the
// YAML config. Each rule matches a set of tools AND a set of namespaces; both
// dimensions must be satisfied for the request to proceed.
//
// Evaluation logic:
//   - Rules are evaluated in order; the first whose CEL expression matches wins.
//   - Within the matching rule, the tool must be in allowed_tools (if set) AND
//     the namespace must be in allowed_namespaces (if set).
//   - An empty allowed_tools or allowed_namespaces means no restriction on that
//     dimension.
//   - If no rule matches, the request is denied.
type ToolPolicyMiddleware struct {
	dependencies    ToolPolicyMiddlewareDependencies
	compiledPolicies []CompiledPolicy
	toolPrefix       string
}

// NewToolPolicyMiddleware compiles all CEL expressions from the configuration
// and returns a ready-to-use middleware. Returns an error if any expression
// fails to compile.
func NewToolPolicyMiddleware(deps ToolPolicyMiddlewareDependencies) (*ToolPolicyMiddleware, error) {
	mw := &ToolPolicyMiddleware{
		dependencies: deps,
		toolPrefix:   deps.AppCtx.ToolPrefix,
	}

	env, err := cel.NewEnv(
		cel.Variable("payload", cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("CEL environment creation error: %w", err)
	}

	for _, rule := range deps.AppCtx.Config.Policies.Rules {
		ast, issues := env.Compile(rule.Expression)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("CEL policy compilation error for %q: %w", rule.Expression, issues.Err())
		}
		prg, err := env.Program(ast)
		if err != nil {
			return nil, fmt.Errorf("CEL policy program construction error: %w", err)
		}
		mw.compiledPolicies = append(mw.compiledPolicies, CompiledPolicy{
			Program:           prg,
			AllowedTools:      rule.AllowedTools,
			AllowedNamespaces: rule.AllowedNamespaces,
		})
	}

	return mw, nil
}

// Middleware wraps a tool handler and enforces policy rules before delegating
// to the next handler. The first matching rule's allowed_tools and
// allowed_namespaces are both checked with AND semantics.
func (mw *ToolPolicyMiddleware) Middleware(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// When no rules are configured, allow everything.
		if len(mw.compiledPolicies) == 0 {
			return next(ctx, request)
		}

		payload, err := mw.extractJWTPayloadFromContext(ctx)
		if err != nil {
			mw.dependencies.AppCtx.Logger.Warn("could not extract JWT payload for policy check", "error", err.Error())
			return mcp.NewToolResultError("access denied: unable to verify permissions"), nil
		}

		toolName := strings.TrimPrefix(request.Params.Name, mw.toolPrefix)
		namespace := mw.extractNamespace(request)

		for _, policy := range mw.compiledPolicies {
			out, _, err := policy.Program.Eval(map[string]interface{}{"payload": payload})
			if err != nil {
				mw.dependencies.AppCtx.Logger.Error("CEL policy evaluation error", "error", err.Error())
				continue
			}
			if out.Value() != true {
				continue
			}

			// Expression matched — check both dimensions with AND.
			toolOK := len(policy.AllowedTools) == 0 || mw.isToolAllowed(toolName, policy.AllowedTools)
			nsOK := namespace == "" || len(policy.AllowedNamespaces) == 0 || mw.isNamespaceAllowed(namespace, policy.AllowedNamespaces)

			if !toolOK {
				mw.dependencies.AppCtx.Logger.Warn("tool access denied by policy", "tool", toolName)
				return mcp.NewToolResultError(fmt.Sprintf("access denied: no permission to use %q", toolName)), nil
			}
			if !nsOK {
				mw.dependencies.AppCtx.Logger.Warn("namespace access denied by policy", "tool", toolName, "namespace", namespace)
				return mcp.NewToolResultError(fmt.Sprintf("access denied: no permission to access namespace %q", namespace)), nil
			}

			// Both dimensions passed — allow the request.
			return next(ctx, request)
		}

		// No rule matched at all.
		mw.dependencies.AppCtx.Logger.Warn("access denied: no matching policy", "tool", toolName)
		return mcp.NewToolResultError("access denied: no matching policy"), nil
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
