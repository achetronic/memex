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
	"net/mail"
	"strings"
)

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

	for _, header := range []string{"Subject", "From", "To", "Date"} {
		if v := msg.Header.Get(header); v != "" {
			buf.WriteString(header)
			buf.WriteString(": ")
			buf.WriteString(v)
			buf.WriteRune('\n')
		}
	}
	buf.WriteRune('\n')

	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return "", fmt.Errorf("reading EML body: %w", err)
	}
	buf.Write(body)

	return buf.String(), nil
}
