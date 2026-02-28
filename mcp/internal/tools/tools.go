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

// Package tools registers all MCP tools and their handlers.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"memex-mcp/internal/globals"
	"memex-mcp/internal/memex"
	"memex-mcp/internal/middlewares"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolsManagerDependencies groups everything the ToolsManager needs.
type ToolsManagerDependencies struct {
	AppCtx       *globals.ApplicationContext
	McpServer    *server.MCPServer
	Middlewares  []middlewares.ToolMiddleware
	MemexClient  *memex.Client
}

// ToolsManager registers and owns all MCP tools.
type ToolsManager struct {
	dependencies ToolsManagerDependencies
}

// NewToolsManager constructs a ToolsManager.
func NewToolsManager(deps ToolsManagerDependencies) *ToolsManager {
	return &ToolsManager{dependencies: deps}
}

// wrapWithMiddlewares applies all configured middlewares to a tool handler,
// with the first middleware in the slice being the outermost wrapper.
func (tm *ToolsManager) wrapWithMiddlewares(handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	for i := len(tm.dependencies.Middlewares) - 1; i >= 0; i-- {
		handler = tm.dependencies.Middlewares[i].Middleware(handler)
	}
	return handler
}

// strArg extracts a named string argument from a CallToolRequest.
// Returns an empty string if the argument is absent or not a string.
func strArg(request mcp.CallToolRequest, name string) string {
	v, _ := request.Params.Arguments.(map[string]interface{})[name].(string)
	return v
}

// intArg extracts a named integer argument (stored as float64 by JSON) from a
// CallToolRequest. Returns the fallback value if the argument is absent.
func intArg(request mcp.CallToolRequest, name string, fallback int) int {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return fallback
	}
	v, ok := args[name].(float64)
	if !ok {
		return fallback
	}
	return int(v)
}

// jsonResult serialises v as indented JSON and returns a text tool result.
func jsonResult(v interface{}) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("serialising result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// AddTools registers every Memex MCP tool on the server.
func (tm *ToolsManager) AddTools() {

	// ── Documents ────────────────────────────────────────────────────────────

	// list_documents — list documents in a namespace
	tm.dependencies.McpServer.AddTool(
		mcp.NewTool("list_documents",
			mcp.WithDescription("List all documents in the given namespace. Optionally filter by ingestion status."),
			mcp.WithString("namespace",
				mcp.Description("Namespace to scope the request to. Sent as X-Memex-Namespace. Falls back to the default configured namespace."),
			),
			mcp.WithString("status",
				mcp.Description("Filter by status: pending, processing, completed, failed. Leave empty for all."),
			),
		),
		tm.wrapWithMiddlewares(tm.HandleListDocuments),
	)

	// get_document — get detail and status of a single document
	tm.dependencies.McpServer.AddTool(
		mcp.NewTool("get_document",
			mcp.WithDescription("Get the detail and current ingestion status of a single document."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Document ID."),
			),
			mcp.WithString("namespace",
				mcp.Description("Namespace to scope the request to."),
			),
		),
		tm.wrapWithMiddlewares(tm.HandleGetDocument),
	)

	// upload_document — upload a file from a local path
	tm.dependencies.McpServer.AddTool(
		mcp.NewTool("upload_document",
			mcp.WithDescription("Upload a local file to Memex for ingestion. The file is read from the given path on the filesystem where memex-mcp is running."),
			mcp.WithString("path",
				mcp.Required(),
				mcp.Description("Absolute path to the file to upload."),
			),
			mcp.WithString("namespace",
				mcp.Description("Namespace to scope the request to."),
			),
		),
		tm.wrapWithMiddlewares(tm.HandleUploadDocument),
	)

	// delete_document — delete a document and all its chunks
	tm.dependencies.McpServer.AddTool(
		mcp.NewTool("delete_document",
			mcp.WithDescription("Delete a document and all its associated chunks from Memex."),
			mcp.WithString("id",
				mcp.Required(),
				mcp.Description("Document ID to delete."),
			),
			mcp.WithString("namespace",
				mcp.Description("Namespace to scope the request to."),
			),
		),
		tm.wrapWithMiddlewares(tm.HandleDeleteDocument),
	)

	// ── Search ───────────────────────────────────────────────────────────────

	// search — semantic search over a namespace
	tm.dependencies.McpServer.AddTool(
		mcp.NewTool("search",
			mcp.WithDescription("Perform a semantic search over the documents in a namespace. Returns the most relevant chunks with their source document and similarity score."),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("Natural language query."),
			),
			mcp.WithString("namespace",
				mcp.Description("Namespace to search in."),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum number of results to return (default: 5)."),
			),
		),
		tm.wrapWithMiddlewares(tm.HandleSearch),
	)

	// ── Health ───────────────────────────────────────────────────────────────

	// health — check the upstream Memex instance
	tm.dependencies.McpServer.AddTool(
		mcp.NewTool("health",
			mcp.WithDescription("Check the health of the upstream Memex instance (database and embeddings API connectivity)."),
		),
		tm.wrapWithMiddlewares(tm.HandleHealth),
	)
}

// ─── Handlers ────────────────────────────────────────────────────────────────

// HandleListDocuments handles the list_documents tool call.
func (tm *ToolsManager) HandleListDocuments(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	namespace := strArg(request, "namespace")
	status := strArg(request, "status")

	docs, err := tm.dependencies.MemexClient.ListDocuments(namespace, status)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("listing documents: %v", err)), nil
	}
	return jsonResult(docs)
}

// HandleGetDocument handles the get_document tool call.
func (tm *ToolsManager) HandleGetDocument(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := strArg(request, "id")
	namespace := strArg(request, "namespace")

	doc, err := tm.dependencies.MemexClient.GetDocument(namespace, id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("getting document: %v", err)), nil
	}
	return jsonResult(doc)
}

// HandleUploadDocument handles the upload_document tool call.
// It reads the file from the local filesystem and streams it to the Memex API.
func (tm *ToolsManager) HandleUploadDocument(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := strArg(request, "path")
	namespace := strArg(request, "namespace")

	if path == "" {
		return mcp.NewToolResultError("path is required"), nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("reading file %q: %v", path, err)), nil
	}

	// Use the base filename as the document name in Memex
	filename := fmt.Sprintf("%s", filepath(path))

	doc, err := tm.dependencies.MemexClient.UploadDocument(namespace, filename, content)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("uploading document: %v", err)), nil
	}
	return jsonResult(doc)
}

// HandleDeleteDocument handles the delete_document tool call.
func (tm *ToolsManager) HandleDeleteDocument(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := strArg(request, "id")
	namespace := strArg(request, "namespace")

	if err := tm.dependencies.MemexClient.DeleteDocument(namespace, id); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("deleting document: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("document %q deleted successfully", id)), nil
}

// HandleSearch handles the search tool call.
func (tm *ToolsManager) HandleSearch(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := strArg(request, "query")
	namespace := strArg(request, "namespace")
	limit := intArg(request, "limit", 5)

	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	results, err := tm.dependencies.MemexClient.Search(namespace, query, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("searching: %v", err)), nil
	}
	return jsonResult(results)
}

// HandleHealth handles the health tool call.
func (tm *ToolsManager) HandleHealth(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h, err := tm.dependencies.MemexClient.Health()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("health check: %v", err)), nil
	}
	return jsonResult(h)
}

// filepath returns the base name of a file path (last segment after the last slash).
func filepath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}
