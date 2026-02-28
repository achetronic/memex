// Copyright 2024 Alby Hernández
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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"memex-mcp/internal/globals"

	"github.com/google/cel-go/cel"
)

// JWTContextKey is the key used to store the JWT payload in context
type contextKey string

const JWTContextKey contextKey = "jwt_payload"

type JWTValidationMiddlewareDependencies struct {
	AppCtx *globals.ApplicationContext
}

type JWTValidationMiddleware struct {
	dependencies JWTValidationMiddlewareDependencies

	// Carried stuff
	jwks  *JWKS
	mutex sync.Mutex

	//
	celPrograms []*cel.Program
}

func NewJWTValidationMiddleware(deps JWTValidationMiddlewareDependencies) (*JWTValidationMiddleware, error) {

	mw := &JWTValidationMiddleware{
		dependencies: deps,
	}

	// Launch JWKS cache worker only when JWT middleware is enabled
	if mw.dependencies.AppCtx.Config.Middleware.JWT.Enabled {
		go mw.cacheJWKS()
	}

	// Precompile and check CEL expressions to fail-fast and safe resources.
	// They will be truly used later.
	allowConditionsEnv, err := cel.NewEnv(
		cel.Variable("payload", cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("CEL environment creation error: %s", err.Error())
	}

	for _, allowCondition := range mw.dependencies.AppCtx.Config.Middleware.JWT.AllowConditions {

		// Compile and execute the code
		ast, issues := allowConditionsEnv.Compile(allowCondition.Expression)
		if issues != nil && issues.Err() != nil {
			return nil, fmt.Errorf("CEL expression compilation exited with error: %s", issues.Err())
		}

		prg, err := allowConditionsEnv.Program(ast)
		if err != nil {
			return nil, fmt.Errorf("CEL program construction error: %s", err.Error())
		}
		mw.celPrograms = append(mw.celPrograms, &prg)
	}

	return mw, nil
}

func (mw *JWTValidationMiddleware) Middleware(next http.Handler) http.Handler {

	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {

		var wwwAuthResourceMetadataUrl string
		var wwwAuthScope string

		if !mw.dependencies.AppCtx.Config.Middleware.JWT.Enabled {
			goto nextStage
		}

		// Add WWW-Authenticate header just in case is needed.
		// Will be cleared for authorized requests later.
		// Ref: https://modelcontextprotocol.io/specification/draft/basic/authorization
		wwwAuthResourceMetadataUrl = fmt.Sprintf("%s://%s/.well-known/oauth-protected-resource%s",
			getRequestScheme(req), req.Host, mw.dependencies.AppCtx.Config.OAuthProtectedResource.UrlSuffix)
		wwwAuthScope = strings.Join(mw.dependencies.AppCtx.Config.OAuthProtectedResource.ScopesSupported, " ")

		rw.Header().Set("WWW-Authenticate",
			`Bearer error="invalid_token", 
					  resource_metadata="`+wwwAuthResourceMetadataUrl+`", 
					  scope="`+wwwAuthScope+`"`)

		{
			// 1. Extract token from Authorization header
			authHeader := req.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(rw, "RBAC: Access Denied: Authorization header not found", http.StatusUnauthorized)
				return
			}
			tokenString := strings.Replace(authHeader, "Bearer ", "", 1)

			// 2. Validate token signature and expiry against JWKS
			_, err := mw.isTokenValid(tokenString)
			if err != nil {
				http.Error(rw, fmt.Sprintf("RBAC: Access Denied: Invalid token: %v", err.Error()), http.StatusUnauthorized)
				return
			}

			// 3. Decode the JWT payload
			tokenStringParts := strings.Split(tokenString, ".")
			tokenPayloadBytes, err := base64.RawURLEncoding.DecodeString(tokenStringParts[1])
			if err != nil {
				mw.dependencies.AppCtx.Logger.Error("error decoding JWT payload from base64", "error", err.Error())
				http.Error(rw, "RBAC: Access Denied: JWT Payload can not be decoded", http.StatusUnauthorized)
				return
			}

			tokenPayload := map[string]any{}
			err = json.Unmarshal(tokenPayloadBytes, &tokenPayload)
			if err != nil {
				mw.dependencies.AppCtx.Logger.Error("error decoding JWT payload from JSON", "error", err.Error())
				http.Error(rw, "RBAC: Access Denied: Internal Issue", http.StatusUnauthorized)
				return
			}

			// 4. Check allow conditions
			for _, celProgram := range mw.celPrograms {
				out, _, err := (*celProgram).Eval(map[string]interface{}{
					"payload": tokenPayload,
				})

				if err != nil {
					mw.dependencies.AppCtx.Logger.Error("CEL program evaluation error", "error", err.Error())
					http.Error(rw, "RBAC: Access Denied: Internal Issue", http.StatusUnauthorized)
					return
				}

				if out.Value() != true {
					http.Error(rw, "RBAC: Access Denied: JWT does not meet conditions", http.StatusUnauthorized)
					return
				}
			}

			// 5. Store the decoded payload in context for downstream use (tool policies, etc.)
			ctx := context.WithValue(req.Context(), JWTContextKey, tokenPayload)
			req = req.WithContext(ctx)
		}

	nextStage:
		rw.Header().Del("WWW-Authenticate")
		next.ServeHTTP(rw, req)
	})
}
