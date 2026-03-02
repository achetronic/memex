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
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ─── PDF ─────────────────────────────────────────────────────────────────────

// PDFParser extracts text from PDF files using the ledongthuc/pdf library.
// It validates the PDF header, sanitises known quirks, and recovers from
// library panics so a malformed file never crashes the worker.
type PDFParser struct{}

func (p *PDFParser) Extensions() []string { return []string{".pdf"} }

func (p *PDFParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading PDF bytes: %w", err)
	}

	b = sanitisePDFHeader(b)

	if err := validateMagic(b, []byte("%PDF-")); err != nil {
		return "", fmt.Errorf("invalid PDF: %w", err)
	}

	text, err := safeParsePDF(b)
	if err != nil {
		return "", err
	}
	return text, nil
}

// sanitisePDFHeader trims leading whitespace and strips trailing spaces on
// the first line. Some PDF generators (e.g. libtiff/tiff2pdf) emit headers
// like "%PDF-1.4 \n" which are valid per viewers but rejected by strict
// parsers.
func sanitisePDFHeader(b []byte) []byte {
	b = bytes.TrimLeft(b, " \t\r\n")

	if idx := bytes.IndexByte(b, '\n'); idx != -1 && idx < 20 {
		header := bytes.TrimRight(b[:idx], " \t\r")
		if !bytes.Equal(header, b[:idx]) {
			clean := make([]byte, 0, len(b))
			clean = append(clean, header...)
			clean = append(clean, b[idx:]...)
			return clean
		}
	}
	return b
}

// safeParsePDF wraps the PDF library calls in a panic-recovery boundary.
// ledongthuc/pdf is known to panic on malformed images and certain CJK
// encodings instead of returning errors.
func safeParsePDF(b []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("PDF library panic: %v", r)
		}
	}()

	reader, rerr := pdf.NewReader(bytes.NewReader(b), int64(len(b)))
	if rerr != nil {
		return "", fmt.Errorf("opening PDF: %w", rerr)
	}

	var buf strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		pageText, perr := page.GetPlainText(nil)
		if perr != nil {
			continue
		}
		buf.WriteString(pageText)
		buf.WriteRune('\n')
	}
	return buf.String(), nil
}

// ─── DOCX ────────────────────────────────────────────────────────────────────

// DOCXParser extracts text from .docx files (Office Open XML format).
type DOCXParser struct{}

func (p *DOCXParser) Extensions() []string { return []string{".docx"} }

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

// ─── ODT ─────────────────────────────────────────────────────────────────────

// ODTParser extracts text from .odt files (OpenDocument Text format).
type ODTParser struct{}

func (p *ODTParser) Extensions() []string { return []string{".odt"} }

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

// ─── shared helpers ──────────────────────────────────────────────────────────

var zipMagic = []byte("PK")

// docxBreakElements are OOXML elements that represent paragraph/line
// boundaries. When the decoder encounters a closing tag for any of these,
// a newline is emitted.
var docxBreakElements = map[string]bool{
	"p":  true, // <w:p> — paragraph
	"br": true, // <w:br> — explicit break
	"tr": true, // <w:tr> — table row
}

// odtBreakElements are ODF elements that represent paragraph/line boundaries.
var odtBreakElements = map[string]bool{
	"p":         true, // <text:p>
	"h":         true, // <text:h> — heading
	"line-break": true, // <text:line-break>
	"table-row": true, // <table:table-row>
}

// validateMagic checks that b starts with the expected magic bytes.
func validateMagic(b, magic []byte) error {
	if len(b) < len(magic) {
		return fmt.Errorf("file too small (%d bytes)", len(b))
	}
	if !bytes.HasPrefix(b, magic) {
		return fmt.Errorf("unexpected file signature: got %q, want %q", safeHead(b, len(magic)), magic)
	}
	return nil
}

// safeHead returns up to n bytes from b for diagnostic messages.
func safeHead(b []byte, n int) []byte {
	if len(b) < n {
		return b
	}
	return b[:n]
}

// extractXMLText opens the named file inside a ZIP archive, walks its XML
// tree, and collects all text content. Elements listed in breakTags cause a
// newline to be emitted when closed, preserving paragraph structure.
func extractXMLText(zr *zip.Reader, target string, breakTags map[string]bool) (string, error) {
	for _, f := range zr.File {
		if f.Name != target {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("opening %s in archive: %w", target, err)
		}
		defer rc.Close()

		decoder := xml.NewDecoder(rc)
		var buf strings.Builder

		for {
			tok, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", fmt.Errorf("decoding XML in %s: %w", target, err)
			}

			switch t := tok.(type) {
			case xml.CharData:
				text := strings.TrimSpace(string(t))
				if text != "" {
					buf.WriteString(text)
					buf.WriteRune(' ')
				}
			case xml.EndElement:
				if breakTags[t.Name.Local] {
					if buf.Len() > 0 {
						trimTrailingSpace(&buf)
						buf.WriteRune('\n')
					}
				}
			}
		}

		return strings.TrimSpace(buf.String()), nil
	}

	return "", fmt.Errorf("file %q not found in archive", target)
}

// trimTrailingSpace removes a single trailing space from a Builder, used to
// clean up the space added after each CharData token before a line break.
func trimTrailingSpace(buf *strings.Builder) {
	s := buf.String()
	if strings.HasSuffix(s, " ") {
		buf.Reset()
		buf.WriteString(s[:len(s)-1])
	}
}
