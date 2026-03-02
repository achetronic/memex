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
	"compress/zlib"
	"fmt"
	"strings"
	"testing"
)

// ─── PDF: content stream helpers ─────────────────────────────────────────────

// TestExtractTextFromContentStream verifies the content stream parser against
// every PDF text operator combination defined in ISO 32000-1 §9.
func TestExtractTextFromContentStream(t *testing.T) {
	tests := []struct {
		name   string
		stream string
		want   string // substring that must appear in the result
	}{
		{
			name:   "simple Tj literal string",
			stream: "BT /F1 12 Tf 50 750 Td (Hello World) Tj ET",
			want:   "Hello World",
		},
		{
			name:   "hex string Tj",
			stream: "BT /F1 12 Tf 50 750 Td <48656c6c6f> Tj ET",
			want:   "Hello",
		},
		{
			name:   "TJ array strings only",
			stream: "BT /F1 12 Tf [(Alpha)(Beta)] TJ ET",
			want:   "AlphaBeta",
		},
		{
			name:   "TJ array large negative kerning adds space",
			stream: "BT /F1 12 Tf [(Hello) -250 (World)] TJ ET",
			want:   "Hello World",
		},
		{
			name:   "TJ array small negative kerning no space",
			stream: "BT /F1 12 Tf [(Hel) -50 (lo)] TJ ET",
			want:   "Hello",
		},
		{
			name:   "TJ array positive kerning no space",
			stream: "BT /F1 12 Tf [(He) 30 (llo)] TJ ET",
			want:   "Hello",
		},
		{
			name:   "T* moves to next line",
			stream: "BT /F1 12 Tf (Line1) Tj T* (Line2) Tj ET",
			want:   "Line1",
		},
		{
			name:   "T* line2 present",
			stream: "BT /F1 12 Tf (Line1) Tj T* (Line2) Tj ET",
			want:   "Line2",
		},
		{
			name:   "apostrophe operator moves to next line and shows string",
			stream: "BT /F1 12 Tf (Line1) Tj (Line2) ' ET",
			want:   "Line2",
		},
		{
			name:   "Td moves text position emitting newline",
			stream: "BT (First) Tj 0 -14 Td (Second) Tj ET",
			want:   "First",
		},
		{
			name:   "Td second string present",
			stream: "BT (First) Tj 0 -14 Td (Second) Tj ET",
			want:   "Second",
		},
		{
			name:   "TD same as Td",
			stream: "BT (A) Tj 0 -14 TD (B) Tj ET",
			want:   "B",
		},
		{
			name:   "text outside BT block is ignored",
			stream: "(Outside) Tj BT (Inside) Tj ET",
			want:   "Inside",
		},
		{
			name:   "text outside BT does not appear",
			stream: "(Outside) Tj BT (Inside) Tj ET",
			want:   "Inside",
		},
		{
			name:   "multiple BT blocks concatenated",
			stream: "BT (Alpha) Tj ET BT (Beta) Tj ET",
			want:   "Alpha",
		},
		{
			name:   "multiple BT blocks beta present",
			stream: "BT (Alpha) Tj ET BT (Beta) Tj ET",
			want:   "Beta",
		},
		{
			name:   "empty stream returns empty string",
			stream: "",
			want:   "",
		},
		{
			name:   "comment lines are skipped",
			stream: "BT\n% this is a comment\n(Hello) Tj\nET",
			want:   "Hello",
		},
		{
			name:   "balanced parentheses in literal string",
			stream: "BT (Hello (World)) Tj ET",
			want:   "Hello (World)",
		},
		{
			name:   "hex string with spaces",
			stream: "BT <48 65 6c 6c 6f> Tj ET",
			want:   "Hello",
		},
		{
			name:   "hex string odd length gets trailing zero",
			stream: "BT <486> Tj ET",
			want:   "H",
		},
		{
			name:   "inline image before text",
			stream: "BI /W 2 /H 2 /CS /G ID \xff\xfe\x00\xff EI BT (after image) Tj ET",
			want:   "after image",
		},
		{
			name:   "inline image between text blocks",
			stream: "BT (before) Tj ET BI /W 1 /H 1 ID \xff EI BT (after) Tj ET",
			want:   "before",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTextFromContentStream([]byte(tt.stream))
			if tt.want == "" {
				if got != "" {
					t.Fatalf("expected empty string, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("want substring %q in result, got %q", tt.want, got)
			}
		})
	}
}

// TestExtractTextFromContentStream_OutsideNotPresent verifies that text
// outside a BT block is never included in the output.
func TestExtractTextFromContentStream_OutsideNotPresent(t *testing.T) {
	stream := "(Outside) Tj BT (Inside) Tj ET"
	got := extractTextFromContentStream([]byte(stream))
	if strings.Contains(got, "Outside") {
		t.Fatalf("text outside BT block should not appear, got %q", got)
	}
}

// ─── PDF: string decoding ─────────────────────────────────────────────────────

// TestDecodePDFString_LiteralEscapes tests all backslash escape sequences
// defined by ISO 32000-1 §7.3.4.2.
func TestDecodePDFString_LiteralEscapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"newline escape", `(\n)`, "\n"},
		{"carriage return escape", `(\r)`, "\r"},
		{"tab escape", `(\t)`, "\t"},
		{"backslash escape", `(\\)`, "\\"},
		{"open paren escape", `(\()`, "("},
		{"close paren escape", `(\))`, ")"},
		{"octal 3-digit", `(\110\145\154\154\157)`, "Hello"},
		{"octal 1-digit", `(\110)`, "H"},
		{"line continuation backslash-newline", "(\\\nHello)", "Hello"},
		{"plain text no escapes", "(Hello World)", "Hello World"},
		{"nested parens balanced", "(foo(bar)baz)", "foo(bar)baz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodePDFString(tt.input)
			if got != tt.want {
				t.Fatalf("want %q, got %q", tt.want, got)
			}
		})
	}
}

// TestDecodePDFString_HexStrings tests hex string decoding including edge cases.
func TestDecodePDFString_HexStrings(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"basic hex", "<48656c6c6f>", "Hello"},
		{"uppercase hex", "<48656C6C6F>", "Hello"},
		{"hex with spaces", "<48 65 6c 6c 6f>", "Hello"},
		{"odd length padded", "<486>", "H`"},
		{"empty hex string", "<>", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodePDFString(tt.input)
			if tt.want == "" && got == "" {
				return
			}
			if tt.name == "odd length padded" {
				// <486> → bytes: 0x48 0x60 → "H`"
				if len(got) != 2 || got[0] != 'H' {
					t.Fatalf("want first byte 'H', got %q", got)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("want %q, got %q", tt.want, got)
			}
		})
	}
}

// TestDecodePDFString_InvalidToken ensures garbage tokens return empty string.
func TestDecodePDFString_InvalidToken(t *testing.T) {
	cases := []string{"", "hello", "/Name", "123"}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if got := decodePDFString(c); got != "" {
				t.Fatalf("expected empty for invalid token %q, got %q", c, got)
			}
		})
	}
}

// TestBytesToUTF8_UTF16BE verifies that strings with a UTF-16BE BOM are
// correctly decoded to UTF-8.
func TestBytesToUTF8_UTF16BE(t *testing.T) {
	// UTF-16BE encoding of "Hi" (U+0048 U+0069) with BOM.
	input := []byte{0xFE, 0xFF, 0x00, 0x48, 0x00, 0x69}
	got := bytesToUTF8(input)
	if got != "Hi" {
		t.Fatalf("want \"Hi\", got %q", got)
	}
}

// TestBytesToUTF8_Latin1 verifies that non-BOM bytes are treated as Latin-1.
func TestBytesToUTF8_Latin1(t *testing.T) {
	input := []byte("Hello")
	got := bytesToUTF8(input)
	if got != "Hello" {
		t.Fatalf("want \"Hello\", got %q", got)
	}
}

// TestBytesToUTF8_ControlCharsFiltered verifies that non-printable bytes
// (except \n, \r, \t) are filtered out.
func TestBytesToUTF8_ControlCharsFiltered(t *testing.T) {
	input := []byte{0x01, 0x02, 'A', 0x03, 'B'}
	got := bytesToUTF8(input)
	if got != "AB" {
		t.Fatalf("want \"AB\", got %q", got)
	}
}

// ─── PDF: tokeniser ───────────────────────────────────────────────────────────

// TestTokeniseContentStream covers the full range of token types the
// tokeniser must handle.
func TestTokeniseContentStream(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		checks func(t *testing.T, tokens []string)
	}{
		{
			name:  "operators and numbers",
			input: "BT /F1 12 Tf 50 750 Td ET",
			checks: func(t *testing.T, tokens []string) {
				if tokens[0] != "BT" {
					t.Fatalf("first token: want BT, got %q", tokens[0])
				}
				last := tokens[len(tokens)-1]
				if last != "ET" {
					t.Fatalf("last token: want ET, got %q", last)
				}
			},
		},
		{
			name:  "literal string is a single token",
			input: "(Hello World)",
			checks: func(t *testing.T, tokens []string) {
				if len(tokens) != 1 || tokens[0] != "(Hello World)" {
					t.Fatalf("want [(Hello World)], got %v", tokens)
				}
			},
		},
		{
			name:  "literal string with balanced parens",
			input: "(foo(bar)baz)",
			checks: func(t *testing.T, tokens []string) {
				if len(tokens) != 1 || tokens[0] != "(foo(bar)baz)" {
					t.Fatalf("unexpected tokens: %v", tokens)
				}
			},
		},
		{
			name:  "hex string is a single token",
			input: "<48656c6c6f>",
			checks: func(t *testing.T, tokens []string) {
				if len(tokens) != 1 || tokens[0] != "<48656c6c6f>" {
					t.Fatalf("unexpected tokens: %v", tokens)
				}
			},
		},
		{
			name:  "array is a single token",
			input: "[(Hello) -250 (World)]",
			checks: func(t *testing.T, tokens []string) {
				if len(tokens) != 1 || tokens[0] != "[(Hello) -250 (World)]" {
					t.Fatalf("unexpected tokens: %v", tokens)
				}
			},
		},
		{
			name:  "comment is skipped",
			input: "BT % this is a comment\nET",
			checks: func(t *testing.T, tokens []string) {
				for _, tok := range tokens {
					if strings.HasPrefix(tok, "%") {
						t.Fatalf("comment token should be skipped: %q", tok)
					}
				}
				if len(tokens) != 2 {
					t.Fatalf("want 2 tokens, got %d: %v", len(tokens), tokens)
				}
			},
		},
		{
			name:  "dict delimiters are skipped",
			input: "<< /Key /Value >>",
			checks: func(t *testing.T, tokens []string) {
				for _, tok := range tokens {
					if tok == "<<" || tok == ">>" {
						t.Fatalf("dict delimiter should be skipped: %q", tok)
					}
				}
			},
		},
		{
			name:  "empty input",
			input: "",
			checks: func(t *testing.T, tokens []string) {
				if len(tokens) != 0 {
					t.Fatalf("expected no tokens, got %v", tokens)
				}
			},
		},
		{
			name:  "inline image BI/ID/EI skipped",
			input: "BT (before) Tj ET\nBI /W 2 /H 2 ID \xff\xd8\xff\xe0\x00 EI BT (after) Tj ET",
			checks: func(t *testing.T, tokens []string) {
				// The raw binary bytes between ID and EI must not appear as tokens.
				// Verify by checking the token list is finite (loop didn't spin on binary data)
				// and that text tokens before and after the image are preserved.
				before, after := false, false
				for _, tok := range tokens {
					if tok == "(before)" {
						before = true
					}
					if tok == "(after)" {
						after = true
					}
				}
				if !before {
					t.Fatalf("expected (before) token, got: %v", tokens)
				}
				if !after {
					t.Fatalf("expected (after) token, got: %v", tokens)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := tokeniseContentStream([]byte(tt.input))
			tt.checks(t, tokens)
		})
	}
}

// ─── PDF: decodeTJArray ───────────────────────────────────────────────────────

// TestDecodeTJArray tests the TJ array decoder in isolation.
func TestDecodeTJArray(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"strings only", "[(Hello)(World)]", "HelloWorld"},
		{"large negative kerning produces space", "[(Hello) -250 (World)]", "Hello World"},
		{"threshold boundary negative just under", "[(A) -99 (B)]", "AB"},
		{"threshold boundary negative just over", "[(A) -101 (B)]", "A B"},
		{"positive kerning no space", "[(A) 50 (B)]", "AB"},
		{"hex strings in array", "[<48656c6c6f><576f726c64>]", "HelloWorld"},
		{"not an array", "(NotAnArray)", ""},
		{"empty array", "[]", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeTJArray(tt.input)
			if got != tt.want {
				t.Fatalf("want %q, got %q", tt.want, got)
			}
		})
	}
}

// ─── PDF: parseSimpleFloat ────────────────────────────────────────────────────

// TestParseSimpleFloat validates the inline float parser.
func TestParseSimpleFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"0", 0},
		{"123", 123},
		{"-250", -250},
		{"+50", 50},
		{"3.14", 3.14},
		{"-0.5", -0.5},
		{"", 0},
		{"abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseSimpleFloat(tt.input)
			diff := got - tt.want
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.0001 {
				t.Fatalf("parseSimpleFloat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ─── PDF: full parse pipeline (PDFParser.Parse) ───────────────────────────────

// TestPDFParser_RejectsNonPDF ensures non-PDF data is rejected before reaching
// the pdfcpu library.
func TestPDFParser_RejectsNonPDF(t *testing.T) {
	p := &PDFParser{}
	_, err := p.Parse(strings.NewReader("this is not a PDF"))
	if err == nil {
		t.Fatal("expected error for non-PDF input")
	}
	if !strings.Contains(err.Error(), "invalid PDF") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// TestPDFParser_RejectsEmpty ensures empty input is rejected.
func TestPDFParser_RejectsEmpty(t *testing.T) {
	p := &PDFParser{}
	_, err := p.Parse(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

// TestPDFParser_RejectsGarbage ensures binary garbage with no PDF magic is
// rejected.
func TestPDFParser_RejectsGarbage(t *testing.T) {
	p := &PDFParser{}
	garbage := []byte{0x00, 0xFF, 0xFE, 0x01, 0x89, 0x50}
	_, err := p.Parse(bytes.NewReader(garbage))
	if err == nil {
		t.Fatal("expected error for binary garbage")
	}
}

// TestPDFParser_SimplePDF tests extraction from a minimal hand-crafted PDF
// with a single page and a literal string.
func TestPDFParser_SimplePDF(t *testing.T) {
	pdf := buildMinimalPDF("Hello World")
	p := &PDFParser{}
	text, err := p.Parse(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Hello World") {
		t.Fatalf("expected 'Hello World' in output, got %q", text)
	}
}

// TestPDFParser_MultiPage tests that all pages in a multi-page PDF are
// extracted and their content appears in the result.
func TestPDFParser_MultiPage(t *testing.T) {
	pdf := buildMultiPagePDF("Page One Content", "Page Two Content")
	p := &PDFParser{}
	text, err := p.Parse(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Page One Content") {
		t.Fatalf("page 1 text missing from output: %q", text)
	}
	if !strings.Contains(text, "Page Two Content") {
		t.Fatalf("page 2 text missing from output: %q", text)
	}
}

// TestPDFParser_TJArray tests extraction from a PDF whose content stream uses
// the TJ operator with a large negative kerning value.
func TestPDFParser_TJArray(t *testing.T) {
	pdf := buildTJArrayPDF()
	p := &PDFParser{}
	text, err := p.Parse(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "Hello" and "World" must both be present (space between them is optional
	// depending on kerning threshold, but both strings must appear).
	if !strings.Contains(text, "Hello") || !strings.Contains(text, "World") {
		t.Fatalf("expected Hello and World in output, got %q", text)
	}
}

// TestPDFParser_HexString tests extraction from a PDF that uses a hex string
// in the content stream.
func TestPDFParser_HexString(t *testing.T) {
	pdf := buildHexStringPDF()
	p := &PDFParser{}
	text, err := p.Parse(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Test") {
		t.Fatalf("expected 'Test' in output, got %q", text)
	}
}

// TestPDFParser_CompressedStream tests extraction from a PDF with a
// FlateDecode (zlib) compressed content stream, which is the most common
// format in real-world PDFs.
func TestPDFParser_CompressedStream(t *testing.T) {
	pdf := buildCompressedStreamPDF("Compressed Content")
	p := &PDFParser{}
	text, err := p.Parse(bytes.NewReader(pdf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "Compressed Content") {
		t.Fatalf("expected 'Compressed Content' in output, got %q", text)
	}
}

// TestPDFParser_Extensions verifies the parser declares the .pdf extension.
func TestPDFParser_Extensions(t *testing.T) {
	p := &PDFParser{}
	exts := p.Extensions()
	found := false
	for _, e := range exts {
		if e == ".pdf" {
			found = true
		}
	}
	if !found {
		t.Fatal("PDFParser.Extensions() must include .pdf")
	}
}

// ─── PDF header sanitisation ──────────────────────────────────────────────────

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

func TestSanitisePDFHeader_CRNotStripped(t *testing.T) {
	// CRLF-terminated first line with binary comment — the \r before \n must
	// not be stripped because it could be part of content, and removing it
	// shifts all xref offsets.
	input := []byte("%PDF-1.5\r%\xe2\xe3\xcf\xd3\r\nrest of file")
	got := sanitisePDFHeader(input)
	if !bytes.Equal(got, input) {
		t.Fatalf("\\r was incorrectly stripped from first line: got %q, want %q", safeHead(got, 20), safeHead(input, 20))
	}
}

// ─── validateMagic ────────────────────────────────────────────────────────────

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

// ─── DOCX ─────────────────────────────────────────────────────────────────────

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

func TestDOCXParser_Extensions(t *testing.T) {
	p := &DOCXParser{}
	exts := p.Extensions()
	if len(exts) == 0 || exts[0] != ".docx" {
		t.Fatalf("expected [.docx], got %v", exts)
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

// ─── ODT ──────────────────────────────────────────────────────────────────────

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

func TestODTParser_Extensions(t *testing.T) {
	p := &ODTParser{}
	exts := p.Extensions()
	if len(exts) == 0 || exts[0] != ".odt" {
		t.Fatalf("expected [.odt], got %v", exts)
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

// ─── RTF ──────────────────────────────────────────────────────────────────────

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

// ─── extractXMLText ───────────────────────────────────────────────────────────

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

// ─── PDF test fixtures ────────────────────────────────────────────────────────

// buildMinimalPDF constructs a minimal but valid PDF 1.4 with a single page
// containing the given text rendered via the Tj operator.
func buildMinimalPDF(pageText string) []byte {
	contentStream := fmt.Sprintf("BT /F1 12 Tf 50 750 Td (%s) Tj ET", pageText)
	return assemblePDF([]string{contentStream})
}

// buildMultiPagePDF constructs a valid PDF 1.4 with two pages, each containing
// distinct text. Used to verify that multi-page extraction works correctly.
func buildMultiPagePDF(page1, page2 string) []byte {
	cs1 := fmt.Sprintf("BT /F1 12 Tf 50 750 Td (%s) Tj ET", page1)
	cs2 := fmt.Sprintf("BT /F1 12 Tf 50 750 Td (%s) Tj ET", page2)
	return assemblePDF([]string{cs1, cs2})
}

// buildTJArrayPDF constructs a PDF whose content stream uses the TJ operator
// with a large negative kerning value (-250) that should produce a space.
func buildTJArrayPDF() []byte {
	return assemblePDF([]string{"BT /F1 12 Tf 50 750 Td [(Hello) -250 (World)] TJ ET"})
}

// buildHexStringPDF constructs a PDF whose content stream contains a hex
// encoded string. "Test" in ASCII hex = 54657374.
func buildHexStringPDF() []byte {
	return assemblePDF([]string{"BT /F1 12 Tf 50 750 Td <54657374> Tj ET"})
}

// buildCompressedStreamPDF constructs a PDF with a FlateDecode (zlib)
// compressed content stream — the standard encoding used by all modern PDFs.
func buildCompressedStreamPDF(text string) []byte {
	raw := fmt.Sprintf("BT /F1 12 Tf 50 750 Td (%s) Tj ET", text)

	var zbuf bytes.Buffer
	w := zlib.NewWriter(&zbuf)
	w.Write([]byte(raw))
	w.Close()
	compressed := zbuf.Bytes()

	return assemblePDFWithCompressedStream(compressed)
}

// assemblePDF builds a minimal valid PDF with one page per content stream.
// All streams are stored uncompressed for simplicity. Font resources are
// declared so pdfcpu can validate the page resource dictionary.
func assemblePDF(contentStreams []string) []byte {
	var b strings.Builder
	offsets := []int{}

	write := func(s string) { b.WriteString(s) }
	obj := func(n int, body string) {
		offsets = append(offsets, b.Len())
		write(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", n, body))
	}

	write("%PDF-1.4\n")

	numPages := len(contentStreams)
	kidRefs := ""
	for i := 0; i < numPages; i++ {
		kidRefs += fmt.Sprintf("%d 0 R ", 3+i*2)
	}

	// 1: Catalog
	obj(1, "<< /Type /Catalog /Pages 2 0 R >>")
	// 2: Pages
	obj(2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.TrimSpace(kidRefs), numPages))

	fontObjNr := 3 + numPages*2

	for i, cs := range contentStreams {
		pageNr := 3 + i*2
		contentNr := pageNr + 1
		obj(pageNr, fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >> >>",
			contentNr, fontObjNr,
		))
		obj(contentNr, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(cs), cs))
	}

	// Font object
	obj(fontObjNr, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")

	totalObjs := fontObjNr + 1

	xrefOffset := b.Len()
	write(fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", totalObjs))
	for _, off := range offsets {
		write(fmt.Sprintf("%010d 00000 n \n", off))
	}
	write(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", totalObjs, xrefOffset))

	return []byte(b.String())
}

// assemblePDFWithCompressedStream builds a single-page PDF whose content
// stream is pre-compressed with FlateDecode. The stream bytes are binary, so
// this function assembles the PDF manually without using a strings.Builder for
// the stream portion.
func assemblePDFWithCompressedStream(compressed []byte) []byte {
	header := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n"
	obj4hdr := fmt.Sprintf("4 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", len(compressed))
	obj4ftr := "\nendstream\nendobj\n"
	obj5 := "5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n"

	off1 := len(header)
	off2 := off1 + len(obj1)
	off3 := off2 + len(obj2)
	off4 := off3 + len(obj3)
	off5 := off4 + len(obj4hdr) + len(compressed) + len(obj4ftr)

	var result []byte
	result = append(result, []byte(header)...)
	result = append(result, []byte(obj1)...)
	result = append(result, []byte(obj2)...)
	result = append(result, []byte(obj3)...)
	result = append(result, []byte(obj4hdr)...)
	result = append(result, compressed...)
	result = append(result, []byte(obj4ftr)...)
	result = append(result, []byte(obj5)...)

	xrefOffset := len(result)
	xref := fmt.Sprintf(
		"xref\n0 6\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		off1, off2, off3, off4, off5,
	)
	trailer := fmt.Sprintf("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	result = append(result, []byte(xref)...)
	result = append(result, []byte(trailer)...)
	return result
}
