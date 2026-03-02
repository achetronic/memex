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
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/mail"
	"strings"

	"github.com/BurntSushi/toml"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
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
		// Skip script and style nodes entirely.
		if c.Type == html.ElementNode && (c.Data == "script" || c.Data == "style") {
			continue
		}
		extractHTMLText(c, buf)
	}
}

// CSVParser converts CSV files into line-by-line text where each row is
// joined by tabs, preserving tabular structure for embedding.
type CSVParser struct{}

func (p *CSVParser) Extensions() []string { return []string{".csv"} }

// Parse reads all CSV records from r and returns them as tab-separated lines.
func (p *CSVParser) Parse(r io.Reader) (string, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("reading CSV: %w", err)
	}

	var buf strings.Builder
	for _, row := range records {
		buf.WriteString(strings.Join(row, "\t"))
		buf.WriteRune('\n')
	}
	return buf.String(), nil
}

// JSONParser converts JSON into indented text. This preserves keys and values
// in a human-readable form that embeds well.
type JSONParser struct{}

func (p *JSONParser) Extensions() []string { return []string{".json"} }

// Parse reads JSON from r and returns a pretty-printed representation.
func (p *JSONParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading JSON file: %w", err)
	}

	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return "", fmt.Errorf("parsing JSON: %w", err)
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("re-marshalling JSON: %w", err)
	}
	return string(out), nil
}

// YAMLParser converts YAML into JSON text for consistent representation.
type YAMLParser struct{}

func (p *YAMLParser) Extensions() []string { return []string{".yaml", ".yml"} }

// Parse reads YAML from r, unmarshals it, and returns a JSON representation.
func (p *YAMLParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading YAML file: %w", err)
	}

	var v any
	if err := yaml.Unmarshal(b, &v); err != nil {
		return "", fmt.Errorf("parsing YAML: %w", err)
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("converting YAML to JSON: %w", err)
	}
	return string(out), nil
}

// TOMLParser converts TOML into JSON text for consistent representation.
type TOMLParser struct{}

func (p *TOMLParser) Extensions() []string { return []string{".toml"} }

// Parse reads TOML from r, decodes it, and returns a JSON representation.
func (p *TOMLParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading TOML file: %w", err)
	}

	var v any
	if _, err := toml.Decode(string(b), &v); err != nil {
		return "", fmt.Errorf("parsing TOML: %w", err)
	}

	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("converting TOML to JSON: %w", err)
	}
	return string(out), nil
}

// XMLParser extracts text content from XML documents, stripping all tags.
type XMLParser struct{}

func (p *XMLParser) Extensions() []string { return []string{".xml"} }

// Parse reads XML from r and returns concatenated text content of all elements.
func (p *XMLParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading XML file: %w", err)
	}

	decoder := xml.NewDecoder(bytes.NewReader(b))
	var buf strings.Builder

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("decoding XML token: %w", err)
		}
		if cd, ok := tok.(xml.CharData); ok {
			text := strings.TrimSpace(string(cd))
			if text != "" {
				buf.WriteString(text)
				buf.WriteRune('\n')
			}
		}
	}
	return buf.String(), nil
}

// RTFParser extracts plain text from RTF files by stripping RTF control words
// and sequences. This is a lightweight implementation covering standard RTF.
type RTFParser struct{}

func (p *RTFParser) Extensions() []string { return []string{".rtf"} }

// Parse reads RTF content from r and returns plain text with control sequences removed.
func (p *RTFParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading RTF file: %w", err)
	}
	return stripRTF(string(b)), nil
}

// rtfSkipGroups lists RTF destination groups whose content is metadata,
// not visible document text. When the parser enters one of these groups
// it suppresses output until the matching closing brace.
var rtfSkipGroups = map[string]bool{
	"fonttbl": true, "colortbl": true, "stylesheet": true,
	"info": true, "pict": true, "header": true, "footer": true,
	"headerl": true, "headerr": true, "footerl": true, "footerr": true,
	"footnote": true, "object": true, "themedata": true,
	"colorschememapping": true, "datastore": true, "latentstyles": true,
	"fldinst": true,
}

// stripRTF removes RTF control words, groups, and special characters,
// returning the underlying plain text content. It tracks a stack of
// "skip" depths so metadata groups are silenced while body text inside
// the root group is emitted.
func stripRTF(s string) string {
	var buf strings.Builder
	i := 0
	depth := 0
	skipDepth := 0

	for i < len(s) {
		ch := s[i]
		switch {
		case ch == '{':
			depth++
			i++
		case ch == '}':
			if depth > 0 {
				if skipDepth >= depth {
					skipDepth = 0
				}
				depth--
			}
			i++
		case ch == '\\' && i+1 < len(s):
			i++
			if s[i] == '\\' || s[i] == '{' || s[i] == '}' {
				if skipDepth == 0 {
					buf.WriteByte(s[i])
				}
				i++
			} else if s[i] == '\n' || s[i] == '\r' {
				i++
			} else {
				start := i
				for i < len(s) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z')) {
					i++
				}
				word := s[start:i]
				for i < len(s) && (s[i] == '-' || (s[i] >= '0' && s[i] <= '9')) {
					i++
				}
				if i < len(s) && s[i] == ' ' {
					i++
				}

				if skipDepth > 0 {
					continue
				}

				if rtfSkipGroups[word] {
					skipDepth = depth
					continue
				}

				switch word {
				case "par", "line":
					buf.WriteRune('\n')
				case "tab":
					buf.WriteRune('\t')
				}
			}
		default:
			if skipDepth == 0 {
				buf.WriteByte(ch)
			}
			i++
		}
	}

	return strings.TrimSpace(buf.String())
}

// EMLParser extracts subject, from, to and body text from email (.eml) files.
type EMLParser struct{}

func (p *EMLParser) Extensions() []string { return []string{".eml"} }

// Parse reads an EML file from r and returns a text representation containing
// the main headers and the message body.
func (p *EMLParser) Parse(r io.Reader) (string, error) {
	msg, err := mail.ReadMessage(r)
	if err != nil {
		return "", fmt.Errorf("parsing EML: %w", err)
	}

	var buf strings.Builder

	// Write key headers.
	for _, header := range []string{"Subject", "From", "To", "Date"} {
		if v := msg.Header.Get(header); v != "" {
			buf.WriteString(header)
			buf.WriteString(": ")
			buf.WriteString(v)
			buf.WriteRune('\n')
		}
	}
	buf.WriteRune('\n')

	// Write body.
	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return "", fmt.Errorf("reading EML body: %w", err)
	}
	buf.Write(body)

	return buf.String(), nil
}
