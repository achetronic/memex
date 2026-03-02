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
	"archive/zip"
	"bytes"
	"fmt"
	"io"
)

// ODTParser extracts text from .odt files (OpenDocument Text format).
type ODTParser struct{}

// Extensions returns the file extensions handled by ODTParser.
func (p *ODTParser) Extensions() []string { return []string{".odt"} }

// Parse reads an ODT from r and returns all extracted plain text.
func (p *ODTParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading ODT bytes: %w", err)
	}

	if err := validateMagic(b, zipMagic); err != nil {
		return "", fmt.Errorf("invalid ODT: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("opening ODT ZIP: %w", err)
	}

	return extractXMLText(zr, "content.xml", odtBreakElements)
}

// odtBreakElements are ODF elements that represent paragraph/line boundaries.
var odtBreakElements = map[string]bool{
	"p":          true, // <text:p>
	"h":          true, // <text:h> — heading
	"line-break": true, // <text:line-break>
	"table-row":  true, // <table:table-row>
}
