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
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

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
