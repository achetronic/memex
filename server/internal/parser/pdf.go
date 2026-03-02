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
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// PDFParser extracts plain text from PDF files using pdfcpu as the underlying
// PDF parser. It supports all modern PDF structures including xref streams
// (PDF 1.5+), compressed object streams, and encrypted documents.
//
// Text is extracted by reading the raw content stream of each page and
// interpreting the PDF text operators (BT/ET, Tj, TJ, Tf, etc.) defined in
// the PDF specification (ISO 32000).
type PDFParser struct{}

// Extensions returns the file extensions handled by PDFParser.
func (p *PDFParser) Extensions() []string { return []string{".pdf"} }

// Parse reads a PDF from r and returns all extracted plain text.
// It validates the PDF magic bytes before parsing. The function is safe to
// call concurrently and recovers from panics in the underlying library.
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

// sanitisePDFHeader trims leading whitespace-only bytes before %PDF and strips
// trailing space characters on the first line. Some PDF generators
// (e.g. libtiff/tiff2pdf) emit headers like "%PDF-1.4 \n" which are valid per
// viewers but rejected by strict parsers.
//
// Only ASCII space (0x20) and horizontal tab (0x09) are stripped from the
// first line — carriage-return bytes must not be removed because they may be
// part of the line content (e.g. a binary comment marker %\xe2\xe3\xcf\xd3\r)
// and removing them would shift all subsequent xref offsets.
func sanitisePDFHeader(b []byte) []byte {
	b = bytes.TrimLeft(b, " \t\r\n")

	if idx := bytes.IndexByte(b, '\n'); idx != -1 && idx < 20 {
		header := bytes.TrimRight(b[:idx], " \t")
		if !bytes.Equal(header, b[:idx]) {
			clean := make([]byte, 0, len(b))
			clean = append(clean, header...)
			clean = append(clean, b[idx:]...)
			return clean
		}
	}
	return b
}

// safeParsePDF wraps the pdfcpu calls in a panic-recovery boundary and
// coordinates the full extraction pipeline: read context → iterate pages →
// extract content stream → parse text operators.
func safeParsePDF(b []byte) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("PDF library panic: %v", r)
		}
	}()

	ctx, err := openPDFContext(b)
	if err != nil {
		return "", fmt.Errorf("opening PDF: %w", err)
	}

	var buf strings.Builder

	for pageNr := 1; pageNr <= ctx.PageCount; pageNr++ {
		pageReader, err := pdfcpu.ExtractPageContent(ctx, pageNr)
		if err != nil {
			continue
		}

		raw, err := io.ReadAll(pageReader)
		if err != nil {
			continue
		}

		pageText := extractTextFromContentStream(raw)
		if pageText != "" {
			buf.WriteString(pageText)
			buf.WriteRune('\n')
		}
	}

	return strings.TrimSpace(buf.String()), nil
}

// openPDFContextTimeout is the maximum time allowed for the
// ReadValidateAndOptimize pass. Very large PDFs (>50 MB) can stall this pass
// indefinitely. If the timeout is exceeded we fall through to ReadContext.
const openPDFContextTimeout = 20 * time.Second

// openPDFContext attempts to open a PDF using two increasingly permissive
// strategies. It returns the first context that succeeds, or the last error
// if all strategies fail.
//
// Strategy 1: ReadValidateAndOptimize with ValidationRelaxed, with a 20-second
// timeout. Handles most PDFs; the optimise pass improves extraction quality.
// Strategy 2: ReadContext with ValidationRelaxed (skips optimise). Faster for
// large PDFs and avoids optimisation-stage errors on non-standard documents.
func openPDFContext(b []byte) (*model.Context, error) {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	type ctxResult struct {
		ctx *model.Context
		err error
	}

	ch := make(chan ctxResult, 1)
	go func() {
		ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(b), conf)
		ch <- ctxResult{ctx, err}
	}()

	select {
	case r := <-ch:
		if r.err == nil {
			return r.ctx, nil
		}
	case <-time.After(openPDFContextTimeout):
	}

	ctx, err := api.ReadContext(bytes.NewReader(b), conf)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

// extractTextFromContentStream parses raw PDF content stream bytes and
// returns the plain text contained within text objects (BT…ET blocks).
//
// It implements a minimal subset of the PDF content stream grammar
// (ISO 32000-1 §9) sufficient for text extraction:
//
//   - BT / ET   — begin/end text object
//   - Tj        — show literal string
//   - TJ        — show array of strings/kerning adjustments
//   - '  (apostrophe) — move to next line and show string
//   - "  (quote)      — set spacing, move to next line and show string
//   - T*        — move to start of next line (emits newline)
//   - Td / TD   — move text position (emits newline)
//   - Tf        — set font (no text output, tracked for future encoding use)
func extractTextFromContentStream(stream []byte) string {
	var buf strings.Builder
	tokens := tokeniseContentStream(stream)

	inText := false

	for i, tok := range tokens {
		switch tok {
		case "BT":
			inText = true

		case "ET":
			inText = false
			ensureNewline(&buf)

		case "Tj":
			if inText && i > 0 {
				buf.WriteString(decodePDFString(tokens[i-1]))
			}

		case "TJ":
			if inText && i > 0 {
				buf.WriteString(decodeTJArray(tokens[i-1]))
			}

		case "'", `"`:
			if inText && i > 0 {
				buf.WriteRune('\n')
				buf.WriteString(decodePDFString(tokens[i-1]))
			}

		case "Td", "TD":
			if inText {
				ensureNewline(&buf)
			}

		case "T*":
			if inText {
				buf.WriteRune('\n')
			}
		}
	}

	return buf.String()
}

// ensureNewline appends a newline to buf only if buf is non-empty and does not
// already end with one.
func ensureNewline(buf *strings.Builder) {
	if buf.Len() > 0 {
		s := buf.String()
		if s[len(s)-1] != '\n' {
			buf.WriteRune('\n')
		}
	}
}

// tokeniseContentStream splits a PDF content stream into a flat slice of
// string tokens. Handles literal strings, hex strings, arrays, operators, and
// inline image data blocks (BI/ID/EI).
func tokeniseContentStream(b []byte) []string {
	var tokens []string
	i := 0
	n := len(b)

	for i < n {
		c := b[i]

		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			i++
			continue
		}

		if c == '%' {
			for i < n && b[i] != '\n' {
				i++
			}
			continue
		}

		if c == '(' {
			start := i
			depth := 0
			i++
			for i < n {
				ch := b[i]
				if ch == '\\' {
					i += 2
					continue
				}
				if ch == '(' {
					depth++
				} else if ch == ')' {
					if depth == 0 {
						i++
						break
					}
					depth--
				}
				i++
			}
			tokens = append(tokens, string(b[start:i]))
			continue
		}

		if c == '<' && i+1 < n && b[i+1] != '<' {
			start := i
			i++
			for i < n && b[i] != '>' {
				i++
			}
			if i < n {
				i++
			}
			tokens = append(tokens, string(b[start:i]))
			continue
		}

		if c == '[' {
			start := i
			depth := 0
			for i < n {
				ch := b[i]
				if ch == '[' {
					depth++
				} else if ch == ']' {
					depth--
					if depth == 0 {
						i++
						break
					}
				} else if ch == '(' {
					i++
					inner := 0
					for i < n {
						ic := b[i]
						if ic == '\\' {
							i += 2
							continue
						}
						if ic == '(' {
							inner++
						} else if ic == ')' {
							if inner == 0 {
								break
							}
							inner--
						}
						i++
					}
				}
				i++
			}
			tokens = append(tokens, string(b[start:i]))
			continue
		}

		if c == '<' && i+1 < n && b[i+1] == '<' {
			i += 2
			continue
		}
		if c == '>' && i+1 < n && b[i+1] == '>' {
			i += 2
			continue
		}
		if c == '>' {
			i++
			continue
		}

		start := i
		for i < n {
			ch := b[i]
			if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' ||
				ch == '(' || ch == ')' || ch == '<' || ch == '>' ||
				ch == '[' || ch == ']' || ch == '%' {
				break
			}
			i++
		}
		if i > start {
			word := string(b[start:i])
			tokens = append(tokens, word)
			if word == "ID" {
				i = skipInlineImageData(b, i)
			}
		}
	}

	return tokens
}

// skipInlineImageData scans forward from pos (immediately after the ID
// operator) looking for the EI (end-image) token that terminates an inline
// image. Per ISO 32000-1 §8.9.7, EI must appear as a token boundary.
func skipInlineImageData(b []byte, pos int) int {
	n := len(b)
	if pos < n && (b[pos] == ' ' || b[pos] == '\t' || b[pos] == '\r' || b[pos] == '\n') {
		pos++
	}
	for pos < n-2 {
		if b[pos] == 'E' && b[pos+1] == 'I' {
			if pos == 0 || b[pos-1] == ' ' || b[pos-1] == '\t' || b[pos-1] == '\r' || b[pos-1] == '\n' {
				after := pos + 2
				if after >= n || b[after] == ' ' || b[after] == '\t' || b[after] == '\r' || b[after] == '\n' {
					return after
				}
			}
		}
		pos++
	}
	return n
}

// decodePDFString decodes a single PDF string token — either a literal string
// enclosed in parentheses or a hex string enclosed in angle brackets — into a
// plain UTF-8 Go string.
func decodePDFString(tok string) string {
	if len(tok) < 2 {
		return ""
	}

	var raw []byte

	switch {
	case tok[0] == '(' && tok[len(tok)-1] == ')':
		raw = decodeLiteralString(tok[1 : len(tok)-1])
	case tok[0] == '<' && tok[len(tok)-1] == '>':
		raw = decodeHexString(tok[1 : len(tok)-1])
	default:
		return ""
	}

	return bytesToUTF8(raw)
}

// decodeLiteralString processes the interior of a PDF literal string
// (without surrounding parentheses), expanding backslash escape sequences.
func decodeLiteralString(inner string) []byte {
	var out []byte
	i := 0
	for i < len(inner) {
		c := inner[i]
		if c != '\\' {
			out = append(out, c)
			i++
			continue
		}
		i++
		if i >= len(inner) {
			break
		}
		switch inner[i] {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case '(':
			out = append(out, '(')
		case ')':
			out = append(out, ')')
		case '\\':
			out = append(out, '\\')
		case '\n', '\r':
			if inner[i] == '\r' && i+1 < len(inner) && inner[i+1] == '\n' {
				i++
			}
		default:
			if inner[i] >= '0' && inner[i] <= '7' {
				octal := 0
				for j := 0; j < 3 && i < len(inner) && inner[i] >= '0' && inner[i] <= '7'; j++ {
					octal = octal*8 + int(inner[i]-'0')
					i++
				}
				out = append(out, byte(octal))
				continue
			}
			out = append(out, inner[i])
		}
		i++
	}
	return out
}

// decodeHexString converts a hex-encoded PDF string (interior without angle
// brackets) to raw bytes. An odd-length hex string has an implicit trailing
// zero per the PDF spec.
func decodeHexString(inner string) []byte {
	s := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, inner)
	if len(s)%2 != 0 {
		s += "0"
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}

// bytesToUTF8 converts a raw PDF string byte slice to a UTF-8 Go string.
// If the bytes start with a UTF-16BE BOM (0xFE 0xFF), the content is decoded
// as UTF-16BE. Otherwise it is treated as Latin-1 (ISO-8859-1).
func bytesToUTF8(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF {
		return utf16BEToString(b[2:])
	}
	var sb strings.Builder
	sb.Grow(len(b))
	for _, ch := range b {
		if ch >= 0x20 || ch == '\n' || ch == '\r' || ch == '\t' {
			sb.WriteRune(rune(ch))
		}
	}
	return sb.String()
}

// utf16BEToString decodes a big-endian UTF-16 byte slice (without BOM) to a
// UTF-8 string. Surrogate pairs are handled correctly by utf16.Decode.
func utf16BEToString(b []byte) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = uint16(b[i*2])<<8 | uint16(b[i*2+1])
	}
	return string(utf16.Decode(u16))
}

// tjSpaceThreshold is the kerning value (in thousandths of a text unit) below
// which a space is inserted between TJ array elements.
const tjSpaceThreshold = -100

// decodeTJArray decodes a TJ operand token — a PDF array of the form
// [ string number string … ] — and returns the concatenated text.
// Negative kerning adjustments larger than tjSpaceThreshold emit a space.
func decodeTJArray(tok string) string {
	if len(tok) < 2 || tok[0] != '[' || tok[len(tok)-1] != ']' {
		return ""
	}

	var buf strings.Builder
	s := tok[1 : len(tok)-1]
	i := 0

	for i < len(s) {
		switch {
		case s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n':
			i++

		case s[i] == '(':
			start, depth := i, 0
			i++
			for i < len(s) {
				if s[i] == '\\' {
					i += 2
					continue
				}
				if s[i] == '(' {
					depth++
				} else if s[i] == ')' {
					if depth == 0 {
						i++
						break
					}
					depth--
				}
				i++
			}
			buf.WriteString(decodePDFString(s[start:i]))

		case s[i] == '<':
			start := i
			i++
			for i < len(s) && s[i] != '>' {
				i++
			}
			if i < len(s) {
				i++
			}
			buf.WriteString(decodePDFString(s[start:i]))

		default:
			start := i
			for i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '\r' && s[i] != '\n' && s[i] != '(' && s[i] != '<' {
				i++
			}
			if parseSimpleFloat(s[start:i]) < tjSpaceThreshold {
				buf.WriteRune(' ')
			}
		}
	}

	return buf.String()
}

// parseSimpleFloat parses a simple ASCII float/integer string without
// importing strconv, to keep the hot path allocation-free for small numbers.
func parseSimpleFloat(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	sign := 1.0
	i := 0
	if s[i] == '-' {
		sign = -1
		i++
	} else if s[i] == '+' {
		i++
	}
	intPart := 0.0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		intPart = intPart*10 + float64(s[i]-'0')
		i++
	}
	fracPart := 0.0
	if i < len(s) && s[i] == '.' {
		i++
		factor := 0.1
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			fracPart += float64(s[i]-'0') * factor
			factor *= 0.1
			i++
		}
	}
	return sign * (intPart + fracPart)
}
