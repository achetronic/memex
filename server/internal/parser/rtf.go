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
)

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
