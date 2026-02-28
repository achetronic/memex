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

// Package globals holds the ApplicationContext, the single shared object that
// carries the parsed configuration and a structured logger throughout the
// entire lifetime of the process.
package globals

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"memex-mcp/api"
	"memex-mcp/internal/config"
)

// ApplicationContext is the central dependency carrier. Every package that
// needs configuration or logging receives a pointer to this struct rather than
// reaching into global variables or the environment directly.
type ApplicationContext struct {
	Context context.Context
	Logger  *slog.Logger
	Config  *api.Configuration
}

// NewApplicationContext parses the -config flag, loads and validates the YAML
// configuration file, and returns a ready-to-use ApplicationContext.
func NewApplicationContext() (*ApplicationContext, error) {
	appCtx := &ApplicationContext{
		Context: context.Background(),
		Logger:  slog.New(slog.NewJSONHandler(os.Stderr, nil)),
	}

	configFlag := flag.String("config", "config.yaml", "path to the config file")
	flag.Parse()

	cfg, err := config.ReadFile(*configFlag)
	if err != nil {
		return appCtx, err
	}
	appCtx.Config = &cfg

	return appCtx, nil
}
