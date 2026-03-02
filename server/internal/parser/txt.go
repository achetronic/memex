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

package parser

import (
	"fmt"
	"io"
)

// TXTParser handles plain text files.
type TXTParser struct{}

func (p *TXTParser) Extensions() []string { return []string{".txt"} }

// Parse reads r and returns its content as-is.
func (p *TXTParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading text file: %w", err)
	}
	return string(b), nil
}

// MarkdownParser handles Markdown files. It returns raw Markdown text, which
// contains enough semantic signal for embedding without stripping syntax.
type MarkdownParser struct{}

func (p *MarkdownParser) Extensions() []string { return []string{".md", ".markdown"} }

// Parse reads r and returns raw Markdown content.
func (p *MarkdownParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading markdown file: %w", err)
	}
	return string(b), nil
}
