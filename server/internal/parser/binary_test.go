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
	"strings"
	"testing"
)

// ─── PDF ─────────────────────────────────────────────────────────────────────

func TestPDFParser_RejectsNonPDF(t *testing.T) {
	p := &PDFParser{}
	_, err := p.Parse(strings.NewReader("this is not a PDF"))
	if err == nil {
		t.Fatal("expected error for non-PDF input")
	}
	if !strings.Contains(err.Error(), "invalid PDF") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPDFParser_RejectsEmpty(t *testing.T) {
	p := &PDFParser{}
	_, err := p.Parse(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestPDFParser_RejectsGarbageWithPDFExtension(t *testing.T) {
	p := &PDFParser{}
	garbage := []byte{0x00, 0xFF, 0xFE, 0x01, 0x89, 0x50}
	_, err := p.Parse(bytes.NewReader(garbage))
	if err == nil {
		t.Fatal("expected error for binary garbage")
	}
}

func TestSanitisePDFHeader_TrailingSpace(t *testing.T) {
	input := []byte("%PDF-1.4 \nrest of file")
	got := sanitisePDFHeader(input)
	if !bytes.HasPrefix(got, []byte("%PDF-1.4\n")) {
		t.Fatalf("trailing space not stripped: %q", got[:20])
	}
}

func TestSanitisePDFHeader_LeadingWhitespace(t *testing.T) {
	input := []byte("  \n%PDF-1.7\nrest")
	got := sanitisePDFHeader(input)
	if !bytes.HasPrefix(got, []byte("%PDF-1.7")) {
		t.Fatalf("leading whitespace not stripped: %q", safeHead(got, 15))
	}
}

func TestSanitisePDFHeader_CleanHeader(t *testing.T) {
	input := []byte("%PDF-1.5\nrest of file")
	got := sanitisePDFHeader(input)
	if !bytes.Equal(got, input) {
		t.Fatalf("clean header was modified: %q", got)
	}
}

func TestValidateMagic(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		magic   []byte
		wantErr bool
	}{
		{"valid PDF", []byte("%PDF-1.4\n"), []byte("%PDF-"), false},
		{"valid ZIP", []byte("PK\x03\x04rest"), []byte("PK"), false},
		{"wrong magic", []byte("NOTPDF"), []byte("%PDF-"), true},
		{"too short", []byte("PK"), []byte("%PDF-"), true},
		{"empty", []byte{}, []byte("PK"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMagic(tt.data, tt.magic)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateMagic() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// ─── DOCX ────────────────────────────────────────────────────────────────────

func TestDOCXParser_RejectsNonZIP(t *testing.T) {
	p := &DOCXParser{}
	_, err := p.Parse(strings.NewReader("not a zip file"))
	if err == nil {
		t.Fatal("expected error for non-ZIP input")
	}
	if !strings.Contains(err.Error(), "invalid DOCX") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDOCXParser_ValidFile(t *testing.T) {
	body := buildDOCXZip(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
		<w:body>
			<w:p><w:r><w:t>Hello</w:t></w:r></w:p>
			<w:p><w:r><w:t>World</w:t></w:r></w:p>
		</w:body>
	</w:document>`)

	p := &DOCXParser{}
	text, err := p.Parse(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Hello") || !strings.Contains(text, "World") {
		t.Fatalf("missing expected text: %q", text)
	}
	if !strings.Contains(text, "\n") {
		t.Fatalf("paragraphs should be separated by newlines: %q", text)
	}
}

func buildDOCXZip(t *testing.T, documentXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(documentXML)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// ─── ODT ─────────────────────────────────────────────────────────────────────

func TestODTParser_RejectsNonZIP(t *testing.T) {
	p := &ODTParser{}
	_, err := p.Parse(strings.NewReader("not a zip file"))
	if err == nil {
		t.Fatal("expected error for non-ZIP input")
	}
	if !strings.Contains(err.Error(), "invalid ODT") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestODTParser_ValidFile(t *testing.T) {
	body := buildODTZip(t, `<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
		xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
		<office:body>
			<office:text>
				<text:p>First paragraph</text:p>
				<text:p>Second paragraph</text:p>
			</office:text>
		</office:body>
	</office:document-content>`)

	p := &ODTParser{}
	text, err := p.Parse(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "First paragraph") || !strings.Contains(text, "Second paragraph") {
		t.Fatalf("missing expected text: %q", text)
	}
}

func buildODTZip(t *testing.T, contentXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("content.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(contentXML)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// ─── RTF ─────────────────────────────────────────────────────────────────────

func TestRTFParser_BasicText(t *testing.T) {
	p := &RTFParser{}
	text, err := p.Parse(strings.NewReader(`{\rtf1\ansi Hello World}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Hello World") {
		t.Fatalf("expected 'Hello World', got %q", text)
	}
}

func TestRTFParser_SkipsMetadata(t *testing.T) {
	p := &RTFParser{}
	text, err := p.Parse(strings.NewReader(`{\rtf1\ansi{\fonttbl{\f0 Arial;}}{\info{\title Secret}}Visible text}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(text, "Arial") {
		t.Fatalf("font table should be skipped: %q", text)
	}
	if strings.Contains(text, "Secret") {
		t.Fatalf("info group should be skipped: %q", text)
	}
	if !strings.Contains(text, "Visible text") {
		t.Fatalf("body text missing: %q", text)
	}
}

func TestRTFParser_ParagraphBreaks(t *testing.T) {
	p := &RTFParser{}
	text, err := p.Parse(strings.NewReader(`{\rtf1\ansi First\par Second\par Third}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), text)
	}
}

func TestRTFParser_EscapedChars(t *testing.T) {
	p := &RTFParser{}
	text, err := p.Parse(strings.NewReader(`{\rtf1\ansi Price: \{100\}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "{100}") {
		t.Fatalf("escaped braces missing: %q", text)
	}
}

// ─── extractXMLText ──────────────────────────────────────────────────────────

func TestExtractXMLText_MissingTarget(t *testing.T) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, _ := w.Create("other.xml")
	f.Write([]byte("<root>text</root>"))
	w.Close()

	zr, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	_, err := extractXMLText(zr, "missing.xml", docxBreakElements)
	if err == nil {
		t.Fatal("expected error for missing target file")
	}
}
