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

// DOCXParser extracts text from .docx files (Office Open XML format).
type DOCXParser struct{}

// Extensions returns the file extensions handled by DOCXParser.
func (p *DOCXParser) Extensions() []string { return []string{".docx"} }

// Parse reads a DOCX from r and returns all extracted plain text.
func (p *DOCXParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading DOCX bytes: %w", err)
	}

	if err := validateMagic(b, zipMagic); err != nil {
		return "", fmt.Errorf("invalid DOCX: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("opening DOCX ZIP: %w", err)
	}

	return extractXMLText(zr, "word/document.xml", docxBreakElements)
}

// docxBreakElements are OOXML elements that represent paragraph/line
// boundaries. When the decoder encounters a closing tag for any of these,
// a newline is emitted.
var docxBreakElements = map[string]bool{
	"p":  true, // <w:p> — paragraph
	"br": true, // <w:br> — explicit break
	"tr": true, // <w:tr> — table row
}
