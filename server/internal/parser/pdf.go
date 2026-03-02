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
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/validate"
)

// PDFParser extracts plain text from PDF files using pdfcpu as the underlying
// PDF parser. It supports all modern PDF structures including xref streams
// (PDF 1.5+), compressed object streams, encrypted documents, linearized PDFs
// with broken trailer chains, and pages whose content is delegated to Form
// XObjects (ISO 32000 §8.10).
//
// Text is extracted by reading the raw content stream of each page and
// interpreting the PDF text operators (BT/ET, Tj, TJ, Tf, Tm, etc.) defined
// in the PDF specification (ISO 32000).
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
// extract content stream (with Form XObject fallback) → parse text operators.
//
// It accepts the raw PDF bytes b and returns the concatenated plain text of
// all pages. Pages that yield no text from their direct content stream are
// retried via extractTextFromPageXObjects, which handles documents that
// delegate page content to /Form XObjects (ISO 32000 §8.10).
// Any panic raised by the pdfcpu library is caught and returned as an error.
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

		// Build the ToUnicode map for the fonts declared on this page so that
		// CID-encoded strings can be decoded correctly (ISO 32000 §9.10.3).
		toUnicode := buildPageToUnicodeMaps(ctx, pageNr)

		// When a page delegates all its content to Form XObjects (e.g. linearized
		// or legacy PDFs that wrap every page in a /Form stream), the content
		// stream only contains operator sequences like "/Fm1 Do". In that case
		// we fall back to extracting text directly from those XObject streams.
		pageText := extractTextFromContentStream(raw, toUnicode)
		if pageText == "" {
			pageText = extractTextFromPageXObjects(ctx, pageNr, nil)
		}
		if pageText != "" {
			buf.WriteString(pageText)
			buf.WriteRune('\n')
		}
	}

	return strings.TrimSpace(buf.String()), nil
}

// extractTextFromPageXObjects extracts text from the Form XObjects referenced
// by the given page's Resources /XObject dictionary. This handles PDFs where
// each page's content stream is a thin wrapper that invokes a Form XObject
// with the Do operator instead of embedding text operators directly.
//
// ctx is the open PDF context and pageNr is the 1-based page number.
// visited tracks object numbers already processed to prevent infinite recursion
// through mutually-referencing XObjects; pass nil on the first call.
//
// Returns the concatenated plain text decoded from all Form XObjects on the
// page, or an empty string if the page has no XObject resources, if none of
// the XObjects are of /Subtype /Form, or if any error occurs during lookup.
//
// Only /Subtype /Form XObjects are processed; image XObjects are ignored.
func extractTextFromPageXObjects(ctx *model.Context, pageNr int, visited map[int]bool) string {
	pageDict, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil {
		return ""
	}

	resObj, found := pageDict.Find("Resources")
	if !found {
		return ""
	}
	resDict, err := ctx.DereferenceDict(resObj)
	if err != nil || resDict == nil {
		return ""
	}

	xobjObj, found := resDict.Find("XObject")
	if !found {
		return ""
	}
	xobjDict, err := ctx.DereferenceDict(xobjObj)
	if err != nil || xobjDict == nil {
		return ""
	}

	if visited == nil {
		visited = make(map[int]bool)
	}
	return extractTextFromXObjectDict(ctx, xobjDict, visited)
}

// extractTextFromXObjectDict iterates over an XObject resource dictionary and
// extracts plain text from every entry of /Subtype /Form. It recurses into
// nested Form XObjects that declare their own /Resources /XObject dictionaries.
//
// visited is a set of already-processed indirect object numbers used to break
// cycles. ctx is the open PDF context.
func extractTextFromXObjectDict(ctx *model.Context, xobjDict types.Dict, visited map[int]bool) string {
	var buf strings.Builder
	for _, v := range xobjDict {
		// Resolve the indirect reference number for cycle detection.
		if ir, ok := v.(types.IndirectRef); ok {
			objNr := ir.ObjectNumber.Value()
			if visited[objNr] {
				continue
			}
			visited[objNr] = true
		}

		sd, _, err := ctx.DereferenceStreamDict(v)
		if err != nil || sd == nil {
			continue
		}
		subtypeObj, found := sd.Dict.Find("Subtype")
		if !found || fmt.Sprintf("%v", subtypeObj) != "Form" {
			continue
		}
		if err := sd.Decode(); err != nil {
			continue
		}

		// Build ToUnicode maps from the XObject's own font resources, if any.
		toUnicode := buildDictToUnicodeMaps(ctx, sd.Dict)

		if t := extractTextFromContentStream(sd.Content, toUnicode); t != "" {
			buf.WriteString(t)
		}

		// Recurse into nested XObjects declared in this Form's /Resources.
		resObj, found := sd.Dict.Find("Resources")
		if !found {
			continue
		}
		resDict, err := ctx.DereferenceDict(resObj)
		if err != nil || resDict == nil {
			continue
		}
		nestedObj, found := resDict.Find("XObject")
		if !found {
			continue
		}
		nestedDict, err := ctx.DereferenceDict(nestedObj)
		if err != nil || nestedDict == nil {
			continue
		}
		if t := extractTextFromXObjectDict(ctx, nestedDict, visited); t != "" {
			buf.WriteString(t)
		}
	}
	return buf.String()
}

// openPDFContextTimeout is the maximum time allowed for the
// ReadValidateAndOptimize pass. Very large PDFs (>50 MB) can stall this pass
// indefinitely. If the timeout is exceeded we fall through to the next strategy.
const openPDFContextTimeout = 20 * time.Second

// openPDFContext attempts to open a PDF using three increasingly permissive
// strategies. It returns the first context with a non-zero page count, or an
// error if all strategies fail.
//
// Strategy 1: ReadValidateAndOptimize with ValidationRelaxed and a 20-second
// timeout. Handles most PDFs; the optimise pass improves extraction quality.
//
// Strategy 2: Read + validate.XRefTable, ignoring validation errors that occur
// after the page tree has already been processed. This recovers PDFs that fail
// on optional catalog entries (e.g. StructTree, OCProperties) but whose page
// count is correctly set by the time the error fires — including linearized
// PDFs with broken trailer chains and PDFs with non-standard optional content.
//
// Strategy 3: raw Read with no validation. Last resort for documents that
// cannot survive any validation pass at all.
func openPDFContext(b []byte) (*model.Context, error) {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	// Strategy 1: full pipeline with timeout.
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
		if r.err == nil && r.ctx.PageCount > 0 {
			return r.ctx, nil
		}
	case <-time.After(openPDFContextTimeout):
	}

	// Strategy 2: read + validate, ignoring errors that fire after page count
	// is already set (broken optional catalog entries, non-standard structures).
	ctx, err := pdfcpu.Read(bytes.NewReader(b), conf)
	if err == nil {
		_ = validate.XRefTable(ctx)
		if ctx.PageCount > 0 {
			return ctx, nil
		}
	}

	// Strategy 3: raw read, no validation.
	ctx, err = api.ReadContext(bytes.NewReader(b), conf)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

// ── ToUnicode CMap support ────────────────────────────────────────────────────

// toUnicodeMaps maps a PDF font resource name (e.g. "F1") to its CID→Unicode
// lookup table, parsed from the font's /ToUnicode CMap stream.
type toUnicodeMaps map[string]map[uint32]rune

// buildPageToUnicodeMaps extracts ToUnicode CMap data for all fonts declared
// in the Resources dictionary of the given page. The returned map is keyed by
// the font resource name as it appears in Tf operators (e.g. "F1", "TT2").
//
// Fonts without a /ToUnicode stream are omitted; text using those fonts falls
// back to the standard Latin-1 / UTF-16BE decoding path.
func buildPageToUnicodeMaps(ctx *model.Context, pageNr int) toUnicodeMaps {
	pageDict, _, _, err := ctx.PageDict(pageNr, false)
	if err != nil {
		return nil
	}
	return buildDictToUnicodeMaps(ctx, pageDict)
}

// buildDictToUnicodeMaps extracts ToUnicode CMap data for all fonts declared
// in the /Resources /Font dictionary of the given PDF dict (page dict or Form
// XObject dict). It is the shared implementation used by both
// buildPageToUnicodeMaps and the recursive XObject extractor.
func buildDictToUnicodeMaps(ctx *model.Context, d types.Dict) toUnicodeMaps {
	resObj, found := d.Find("Resources")
	if !found {
		return nil
	}
	resDict, err := ctx.DereferenceDict(resObj)
	if err != nil || resDict == nil {
		return nil
	}

	fontObj, found := resDict.Find("Font")
	if !found {
		return nil
	}
	fontDict, err := ctx.DereferenceDict(fontObj)
	if err != nil || fontDict == nil {
		return nil
	}

	maps := make(toUnicodeMaps)
	for fontName, fontRef := range fontDict {
		fontEntryDict, err := ctx.DereferenceDict(fontRef)
		if err != nil || fontEntryDict == nil {
			continue
		}
		tuObj, found := fontEntryDict.Find("ToUnicode")
		if !found {
			continue
		}
		sd, _, err := ctx.DereferenceStreamDict(tuObj)
		if err != nil || sd == nil {
			continue
		}
		if err := sd.Decode(); err != nil {
			continue
		}
		m := parseCMap(sd.Content)
		if len(m) > 0 {
			maps[fontName] = m
		}
	}
	if len(maps) == 0 {
		return nil
	}
	return maps
}

// parseCMap parses a PDF ToUnicode CMap stream and returns a map from CID
// (as uint32) to Unicode rune. It handles both beginbfchar and beginbfrange
// sections as defined in ISO 32000-1 §9.10.3.
//
// Only the most common 2-byte CID encoding is fully supported. 1-byte CIDs
// are also handled. Unicode values are expected as 2-byte or 4-byte hex
// sequences in the CMap destination fields.
func parseCMap(data []byte) map[uint32]rune {
	m := make(map[uint32]rune)
	s := string(data)
	lines := strings.Split(s, "\n")

	inChar := false
	inRange := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		switch {
		case line == "beginbfchar":
			inChar = true
			continue
		case line == "endbfchar":
			inChar = false
			continue
		case line == "beginbfrange":
			inRange = true
			continue
		case line == "endbfrange":
			inRange = false
			continue
		}

		if inChar {
			// Format: <srcCID> <dstUnicode>
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			src := parseHexToken(parts[0])
			dst := parseHexToken(parts[1])
			if src >= 0 && dst >= 0 {
				m[uint32(src)] = rune(dst)
			}
		}

		if inRange {
			// Format: <startCID> <endCID> <startUnicode>
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			start := parseHexToken(parts[0])
			end := parseHexToken(parts[1])
			dst := parseHexToken(parts[2])
			if start < 0 || end < 0 || dst < 0 {
				continue
			}
			for cid := start; cid <= end; cid++ {
				m[uint32(cid)] = rune(dst + (cid - start))
			}
		}
	}
	return m
}

// parseHexToken parses a CMap hex token of the form <XXXX> and returns its
// integer value. Returns -1 if the token is not a valid hex token.
func parseHexToken(s string) int {
	if len(s) < 2 || s[0] != '<' || s[len(s)-1] != '>' {
		return -1
	}
	inner := s[1 : len(s)-1]
	inner = strings.ReplaceAll(inner, " ", "")
	if len(inner) == 0 {
		return -1
	}
	if len(inner)%2 != 0 {
		inner = "0" + inner
	}
	b, err := hex.DecodeString(inner)
	if err != nil {
		return -1
	}
	switch len(b) {
	case 1:
		return int(b[0])
	case 2:
		return int(binary.BigEndian.Uint16(b))
	case 3:
		return int(uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2]))
	case 4:
		return int(binary.BigEndian.Uint32(b))
	}
	return -1
}

// ── Content stream interpreter ────────────────────────────────────────────────

// tmNewlineThreshold is the minimum absolute change in the Y component of the
// Tm text matrix that triggers a newline. The Tm matrix is specified in text
// space units (typically points); a change above this value means the text
// cursor has moved to a different line.
const tmNewlineThreshold = 1.0

// extractTextFromContentStream parses raw PDF content stream bytes and
// returns the plain text contained within text objects (BT…ET blocks).
//
// It implements a subset of the PDF content stream grammar (ISO 32000-1 §9)
// sufficient for text extraction:
//
//   - BT / ET   — begin/end text object
//   - Tj        — show literal string
//   - TJ        — show array of strings/kerning adjustments
//   - '  (apostrophe) — move to next line and show string
//   - "  (quote)      — set spacing, move to next line and show string
//   - Tm        — set text matrix; emits newline when Y position changes
//   - T*        — move to start of next line (emits newline)
//   - Td / TD   — move text position (emits newline)
//   - Tf        — set current font name (used for ToUnicode lookup)
//
// toUnicode maps PDF font resource names to their CID→Unicode tables. Pass
// nil when no ToUnicode data is available; affected strings fall back to the
// standard Latin-1 / UTF-16BE decoding path.
func extractTextFromContentStream(stream []byte, toUnicode toUnicodeMaps) string {
	var buf strings.Builder
	tokens := tokeniseContentStream(stream)

	inText := false
	currentFont := ""
	var lastTmY float64
	tmYSet := false

	for i, tok := range tokens {
		switch tok {
		case "BT":
			inText = true
			tmYSet = false

		case "ET":
			inText = false
			ensureNewline(&buf)

		case "Tf":
			// Tf syntax: /FontName size Tf — font name is two tokens back.
			if i >= 2 && strings.HasPrefix(tokens[i-2], "/") {
				currentFont = tokens[i-2][1:] // strip leading /
			}

		case "Tm":
			// Tm syntax: a b c d e f Tm — 6 numeric operands.
			// The f component (index i-1) is the Y translation.
			if inText && i >= 6 {
				y := parseSimpleFloat(tokens[i-1])
				if tmYSet {
					diff := y - lastTmY
					if diff < 0 {
						diff = -diff
					}
					if diff > tmNewlineThreshold {
						ensureNewline(&buf)
					}
				}
				lastTmY = y
				tmYSet = true
			}

		case "Tj":
			if inText && i > 0 {
				buf.WriteString(decodePDFStringWithFont(tokens[i-1], currentFont, toUnicode))
			}

		case "TJ":
			if inText && i > 0 {
				buf.WriteString(decodeTJArrayWithFont(tokens[i-1], currentFont, toUnicode))
			}

		case "'", `"`:
			if inText && i > 0 {
				buf.WriteRune('\n')
				buf.WriteString(decodePDFStringWithFont(tokens[i-1], currentFont, toUnicode))
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

// ── String decoding ───────────────────────────────────────────────────────────

// decodePDFStringWithFont decodes a PDF string token using the ToUnicode CMap
// of the currently active font when available, falling back to the standard
// decodePDFString path for fonts without ToUnicode data.
//
// tok is the raw PDF string token (literal or hex). fontName is the current
// font resource name as set by the most recent Tf operator. toUnicode is the
// per-page map of font names to CID→Unicode tables; it may be nil.
func decodePDFStringWithFont(tok, fontName string, toUnicode toUnicodeMaps) string {
	if toUnicode != nil {
		if m, ok := toUnicode[fontName]; ok {
			return decodePDFStringCID(tok, m)
		}
	}
	return decodePDFString(tok)
}

// decodePDFStringCID decodes a PDF string token using a CID→Unicode lookup
// table extracted from the font's ToUnicode CMap stream. Each pair of bytes
// in the raw string is treated as a 2-byte CID; unmapped CIDs are skipped.
//
// tok must be a literal string token (parentheses) or a hex string token
// (angle brackets). Returns an empty string for invalid tokens.
func decodePDFStringCID(tok string, m map[uint32]rune) string {
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

	var sb strings.Builder
	// Try 2-byte CID decoding first.
	if len(raw)%2 == 0 {
		allMapped := true
		for j := 0; j+1 < len(raw); j += 2 {
			cid := uint32(raw[j])<<8 | uint32(raw[j+1])
			if r, ok := m[cid]; ok {
				sb.WriteRune(r)
			} else {
				allMapped = false
				break
			}
		}
		if allMapped && sb.Len() > 0 {
			return sb.String()
		}
	}

	// Fall back to 1-byte CID decoding.
	sb.Reset()
	for _, b := range raw {
		if r, ok := m[uint32(b)]; ok {
			sb.WriteRune(r)
		}
	}
	if sb.Len() > 0 {
		return sb.String()
	}

	// If no CID mapping matched, decode as Latin-1 / UTF-16BE.
	return bytesToUTF8(raw)
}

// decodeTJArrayWithFont decodes a TJ operand token using ToUnicode CMap data
// for the currently active font. It is the font-aware counterpart of
// decodeTJArray, delegating each string element to decodePDFStringWithFont.
//
// tok is the raw TJ array token (e.g. "[(Hello) -250 (World)]"). fontName is
// the current font resource name. toUnicode is the per-page font CMap map.
func decodeTJArrayWithFont(tok, fontName string, toUnicode toUnicodeMaps) string {
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
			buf.WriteString(decodePDFStringWithFont(s[start:i], fontName, toUnicode))

		case s[i] == '<':
			start := i
			i++
			for i < len(s) && s[i] != '>' {
				i++
			}
			if i < len(s) {
				i++
			}
			buf.WriteString(decodePDFStringWithFont(s[start:i], fontName, toUnicode))

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

// decodePDFString decodes a single PDF string token — either a literal string
// enclosed in parentheses or a hex string enclosed in angle brackets — into a
// plain UTF-8 Go string. It does not use ToUnicode data; use
// decodePDFStringWithFont when a font context is available.
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
// as UTF-16BE. Otherwise it is treated as Latin-1 (ISO-8859-1), with common
// Unicode Private Use Area (PUA) ligature codepoints replaced by their
// canonical Unicode equivalents before output.
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
	return mapPUALigatures(sb.String())
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
	return mapPUALigatures(string(utf16.Decode(u16)))
}

// puaLigatureReplacer replaces Unicode Private Use Area codepoints commonly
// used by PDF generators to encode typographic ligatures with their canonical
// Unicode equivalents. Applied as a post-processing step in bytesToUTF8 and
// utf16BEToString.
var puaLigatureReplacer = strings.NewReplacer(
	// Adobe standard PUA ligatures (used by most Type1 / OpenType PS fonts)
	"\uFB00", "ff",
	"\uFB01", "fi",
	"\uFB02", "fl",
	"\uFB03", "ffi",
	"\uFB04", "ffl",
	"\uFB05", "ſt", // long-s + t
	"\uFB06", "st",
	// Some generators use the PUA block starting at U+E000
	"\uE001", "fi",
	"\uE002", "fl",
	"\uE003", "ff",
	"\uE004", "ffi",
	"\uE005", "ffl",
)

// mapPUALigatures replaces known PUA ligature codepoints in s with their
// canonical Unicode equivalents. It is a no-op for strings that contain no
// PUA characters, keeping the common path allocation-free.
func mapPUALigatures(s string) string {
	for _, r := range s {
		if (r >= 0xE000 && r <= 0xF8FF) || (r >= 0xFB00 && r <= 0xFB06) {
			return puaLigatureReplacer.Replace(s)
		}
	}
	return s
}

// ── TJ array ─────────────────────────────────────────────────────────────────

// tjSpaceThreshold is the kerning value (in thousandths of a text unit) below
// which a space is inserted between TJ array elements. The value -50 captures
// inter-word spacing in tight-kerning documents while avoiding false spaces
// within ligatures and closely-set glyphs.
const tjSpaceThreshold = -50

// decodeTJArray decodes a TJ operand token — a PDF array of the form
// [ string number string … ] — and returns the concatenated text.
// Negative kerning adjustments below tjSpaceThreshold emit a space.
// Use decodeTJArrayWithFont when a font context is available.
func decodeTJArray(tok string) string {
	return decodeTJArrayWithFont(tok, "", nil)
}

// ── Float parser ──────────────────────────────────────────────────────────────

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
