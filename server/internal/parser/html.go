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
	"strings"

	"golang.org/x/net/html"
)

// HTMLParser extracts visible text from HTML documents, stripping all tags.
type HTMLParser struct{}

func (p *HTMLParser) Extensions() []string { return []string{".html", ".htm"} }

// Parse parses the HTML in r and returns the concatenated visible text content.
func (p *HTMLParser) Parse(r io.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", fmt.Errorf("parsing HTML: %w", err)
	}

	var buf strings.Builder
	extractHTMLText(doc, &buf)
	return strings.TrimSpace(buf.String()), nil
}

// extractHTMLText recursively walks an HTML node tree and writes visible text
// to buf, skipping script and style elements.
func extractHTMLText(n *html.Node, buf *strings.Builder) {
	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			buf.WriteString(text)
			buf.WriteRune('\n')
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && (c.Data == "script" || c.Data == "style") {
			continue
		}
		extractHTMLText(c, buf)
	}
}
