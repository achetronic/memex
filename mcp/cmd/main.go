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
	"log"
	"net/http"
	"time"

	"memex-mcp/internal/globals"
	"memex-mcp/internal/memex"
	"memex-mcp/internal/middlewares"
	"memex-mcp/internal/tools"

	"github.com/mark3labs/mcp-go/server"
)

func main() {

	// 0. Load configuration
	appCtx, err := globals.NewApplicationContext()
	if err != nil {
		log.Fatalf("failed creating application context: %v", err)
	}

	// 1. Build Memex API client
	memexClient := memex.NewClient(
		appCtx.Config.Memex.BaseURL,
		appCtx.Config.Memex.DefaultNamespace,
	)

	// 2. Initialise middlewares
	accessLogsMw := middlewares.NewAccessLogsMiddleware(middlewares.AccessLogsMiddlewareDependencies{
		AppCtx: appCtx,
	})

	jwtValidationMw, err := middlewares.NewJWTValidationMiddleware(middlewares.JWTValidationMiddlewareDependencies{
		AppCtx: appCtx,
	})
	if err != nil {
		appCtx.Logger.Info("failed starting JWT validation middleware", "error", err.Error())
	}

	toolPolicyMw, err := middlewares.NewToolPolicyMiddleware(middlewares.ToolPolicyMiddlewareDependencies{
		AppCtx: appCtx,
	})
	if err != nil {
		appCtx.Logger.Info("failed starting tool policy middleware", "error", err.Error())
	}

	var toolMiddlewares []middlewares.ToolMiddleware
	if toolPolicyMw != nil && (len(appCtx.Config.Policies.Tools) > 0 || len(appCtx.Config.Policies.Namespaces) > 0) {
		toolMiddlewares = append(toolMiddlewares, toolPolicyMw)
	}

	// 3. Create MCP server
	mcpServer := server.NewMCPServer(
		appCtx.Config.Server.Name,
		appCtx.Config.Server.Version,
		server.WithToolCapabilities(true),
	)

	// 4. Register tools
	tm := tools.NewToolsManager(tools.ToolsManagerDependencies{
		AppCtx:      appCtx,
		McpServer:   mcpServer,
		Middlewares: toolMiddlewares,
		MemexClient: memexClient,
	})
	tm.AddTools()

	// 5. Start selected transport
	switch appCtx.Config.Server.Transport.Type {
	case "http":
		httpServer := server.NewStreamableHTTPServer(mcpServer,
			server.WithHeartbeatInterval(30*time.Second),
			server.WithStateLess(false),
		)

		mux := http.NewServeMux()
		mux.Handle("/mcp", accessLogsMw.Middleware(jwtValidationMw.Middleware(httpServer)))

		if appCtx.Config.OAuthAuthorizationServer.Enabled {
			mux.Handle(
				"/.well-known/oauth-authorization-server"+appCtx.Config.OAuthAuthorizationServer.UrlSuffix,
				accessLogsMw.Middleware(http.HandlerFunc(handleOAuthAuthorizationServer(appCtx))),
			)
		}

		if appCtx.Config.OAuthProtectedResource.Enabled {
			mux.Handle(
				"/.well-known/oauth-protected-resource"+appCtx.Config.OAuthProtectedResource.UrlSuffix,
				accessLogsMw.Middleware(http.HandlerFunc(handleOAuthProtectedResource(appCtx))),
			)
		}

		srv := &http.Server{
			Addr:              appCtx.Config.Server.Transport.HTTP.Host,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       0, // disabled for SSE/streaming connections
		}

		appCtx.Logger.Info("starting StreamableHTTP server", "host", appCtx.Config.Server.Transport.HTTP.Host)
		if err := srv.ListenAndServe(); err != nil {
			log.Fatal(err)
		}

	default:
		appCtx.Logger.Info("starting stdio server")
		if err := server.ServeStdio(mcpServer); err != nil {
			log.Fatal(err)
		}
	}
}
