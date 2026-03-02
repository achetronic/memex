# Memex — PDF Parser: Context & Decisions

This file is the single source of truth for the PDF parser (`server/internal/parser/pdf.go`).
Read it entirely before touching the parser. It records every design decision, known issue,
fix rationale, and open work item agreed with the project owner.

---

## Library choice

**pdfcpu** (`github.com/pdfcpu/pdfcpu`) is the only well-maintained pure-Go PDF library.
No CGo, no external binaries, no system dependencies. This is non-negotiable.

Alternatives explicitly rejected:
- `go-fitz` / MuPDF — CGo wrapper, requires `libmupdf-dev` in the Docker image
- `pdftotext` / poppler — external CLI binary, requires `exec.Command`, not a library
- Custom xref repair — too fragile, reinventing pdfcpu internals

---

## Architecture

```
Parse(r io.Reader)
  └─ sanitisePDFHeader(b)          // fix quirky generators
  └─ validateMagic(b, "%PDF-")
  └─ safeParsePDF(b)               // panic boundary
       └─ openPDFContext(b)        // 3-strategy open (see below)
       └─ for each page:
            └─ buildPageToUnicodeMaps(ctx, pageNr)   // font→CMap per page
            └─ ExtractPageContent                     // pdfcpu: decode compressed streams
            └─ extractTextFromContentStream(raw, toUnicode)
            └─ if empty → extractTextFromPageXObjects(ctx, pageNr, nil)
                 └─ extractTextFromXObjectDict(ctx, xobjDict, visited)  // recursive
                      └─ buildDictToUnicodeMaps(ctx, sd.Dict)
                      └─ extractTextFromContentStream(sd.Content, toUnicode)
                      └─ recurse into /Resources /XObject (cycle-guarded)
```

---

## openPDFContext: 3-strategy cascade

This is the most important fix in the parser. A single open strategy leaves too many
real-world PDFs unreadable. The three strategies are tried in order, returning on the
first that yields `PageCount > 0`.

### Strategy 1 — `ReadValidateAndOptimize` (20s timeout)
The full pdfcpu pipeline. Handles the vast majority of PDFs. The optimise pass improves
extraction quality. Wrapped in a goroutine with a 20-second timeout because very large
PDFs (>50 MB) can stall the optimise stage indefinitely.

### Strategy 2 — `pdfcpu.Read` + `validate.XRefTable`, error ignored
The key insight: `validate.XRefTable` calls `validateRootObject` → `validatePages` first,
which is where `ctx.PageCount` gets set. The errors that break APPCC and R1169 fire later,
in optional catalog entries (StructTree, OCProperties). By calling `validate.XRefTable`
and discarding the error, we get a fully populated `PageCount` even when pdfcpu's higher-
level functions refuse to return a context.

This recovers three classes of real-world PDFs:
- **Linearized PDFs with broken trailer chains**: the last trailer lacks `/Root`, pdfcpu
  cannot follow the `/Prev` chain to find it, `ReadContext` returns `PageCount: 0`. Strategy 2
  reads the xref correctly at the lower level.
- **PDFs with invalid StructTree** (`processStructTreeClassMapDict: unsupported PDF object`):
  StructTree is optional and irrelevant for text extraction. R1169_2011 (EU regulation).
- **PDFs with invalid OCProperties** (`optContentPropertiesDict required entry=D missing`):
  Optional content is irrelevant for text extraction. APPCC_pasteleria_Comunidad_Madrid.

### Strategy 3 — `ReadContext` (no validation)
Raw read with no validation pass at all. Last resort. `PageCount` stays 0 for most broken
PDFs since it only gets set during validation, but this catches edge cases where even
`validate.XRefTable` panics or errors at the read stage.

---

## Form XObject fallback (recursive)

Some PDFs (notably legacy Codex Alimentarius documents generated with Mac OS X Quartz
PDF workflows) wrap every page's content in a Form XObject. The page content stream
contains only `/Fm1 Do` (invoke XObject). `extractTextFromContentStream` sees an empty
stream and returns "".

Fix: when the content stream yields no text, `safeParsePDF` calls
`extractTextFromPageXObjects(ctx, pageNr, nil)`, which delegates to
`extractTextFromXObjectDict`. That function:
1. Reads the page's (or XObject's) `/Resources /XObject` dictionary
2. Dereferences each entry, skips non-`/Subtype /Form` XObjects
3. Builds a local ToUnicode map from the XObject's own font resources
4. Decodes the stream and passes it through `extractTextFromContentStream`
5. Recursively processes any `/Resources /XObject` in the XObject's own resource dict

A `visited map[int]bool` (keyed on `IndirectRef.ObjectNumber`) is passed through all
recursive calls as a cycle guard. Infinite loops from self-referencing XObjects are
impossible.

---

## Content stream interpreter

`extractTextFromContentStream(stream []byte, toUnicode toUnicodeMaps) string`

The interpreter scans tokens sequentially and handles the following PDF operators:

| Operator | Action |
|----------|--------|
| `BT`     | Begin text block — resets `lastTmY` tracking |
| `ET`     | End text block — appends newline separator |
| `Tj`     | Show string — decoded with `decodePDFStringWithFont` |
| `TJ`     | Show array with kerning — decoded with `decodeTJArrayWithFont` |
| `'`      | Move to next line, show string |
| `"`      | Set word/char spacing, move to next line, show string |
| `Tf`     | Set font — records current font name for ToUnicode lookup |
| `Tm`     | Set text matrix — tracks Y-coordinate for line detection |

### Tm operator and line detection

`Tm` takes 6 operands: `a b c d e f` where `e` is the X position and `f` is the Y
position in user-space coordinates. On each `Tm`, the interpreter reads `tokens[i-1]`
(the `f` component).

If `|y - lastTmY| > tmNewlineThreshold` (currently `1.0`), `ensureNewline()` is called
before the next text token. This prevents text from different lines being concatenated
without separators — a very common issue in modern PDFs generated by Word/LibreOffice
where every text run has an explicit `Tm` rather than using `Td`/`T*` relative moves.

`lastTmY` and `tmYSet` are reset on `BT` (begin text block).

---

## Text encoding

### bytesToUTF8
PDF strings are not inherently Unicode. The decoder applies the following logic:
1. If bytes start with BOM `0xFE 0xFF` → decode as UTF-16BE (handles Word/LibreOffice output)
2. Otherwise → treat as Latin-1 / ISO-8859-1 (the PDF spec default for simple fonts)

Control characters below 0x20 are filtered out, except `\n`, `\r`, `\t`.

Both paths call `mapPUALigatures` as a post-processing step before returning.

### ToUnicode CMap support

CID fonts (fonts with `/Encoding /Identity-H`) encode text as 2-byte CID values.
When the font dict includes a `/ToUnicode` CMap stream, the mapping is fully deterministic.

**Build phase** — `buildPageToUnicodeMaps(ctx, pageNr) toUnicodeMaps`:
- Reads the page's `/Resources /Font` dictionary
- For each font that has a `/ToUnicode` stream entry, decodes and parses it
- Returns `map[fontName]map[uint32]rune`
- `buildDictToUnicodeMaps(ctx, d types.Dict)` is the shared implementation used by both
  the page path and the XObject path

**Parse phase** — `parseCMap(data []byte) map[uint32]rune`:
- Handles `beginbfchar` blocks (single CID→Unicode mappings)
- Handles `beginbfrange` blocks (contiguous CID→Unicode ranges)
- `parseHexToken(s string) int` parses `<XX>`, `<XXXX>`, `<XXXXXX>`, `<XXXXXXXX>` tokens

**Decode phase** — `decodePDFStringCID(tok string, m map[uint32]rune) string`:
1. Attempts 2-byte CID decoding: reads pairs of bytes as `uint32`, looks up each in the map.
   If all pairs map successfully, returns the result.
2. Falls back to 1-byte CID decoding if any 2-byte pair fails.
3. Final fallback: `bytesToUTF8` (Latin-1 / UTF-16BE).

**Font-aware string decoding** — `decodePDFStringWithFont(tok, fontName string, toUnicode toUnicodeMaps) string`:
- Looks up `toUnicode[fontName]`. If a map is found, delegates to `decodePDFStringCID`.
- Otherwise falls back to `bytesToUTF8`.

**Font-aware TJ decoding** — `decodeTJArrayWithFont(tok, fontName string, toUnicode toUnicodeMaps) string`:
- Same font-aware dispatch, applied to each string element within the TJ array.
- `decodeTJArray(tok)` is kept as a convenience wrapper that calls `decodeTJArrayWithFont(tok, "", nil)`.

### What this does NOT handle
Fonts with `/Encoding /Identity-H` that have **no** `/ToUnicode` stream — the information
simply does not exist in the file. See the Impossible Issues section.

---

## TJ kerning threshold

`decodeTJArray` / `decodeTJArrayWithFont` inserts a space when a kerning value (in
thousandths of a text unit) is below `tjSpaceThreshold = -50`.

The previous value was `-100`. Real-world corpus testing showed that tight word spaces
in many EU regulatory documents use values between `-50` and `-100` — treating them as
kerning produced concatenated words. Lowering to `-50` was validated against all 394
corpus PDFs with no regressions.

Values above `-50` are pure intra-glyph kerning adjustments and never represent word
spaces in standard typesetting practice.

---

## PUA ligature mapping

Some PDF generators (notably older Quartz / Core Text workflows) encode ligatures using
Unicode Private Use Area codepoints instead of the canonical Unicode characters:

| PUA codepoint | Ligature | Unicode canonical |
|---------------|----------|-------------------|
| U+FB00        | ff       | U+FB00 (standard) |
| U+FB01        | fi       | U+FB01 (standard) |
| U+FB02        | fl       | U+FB02 (standard) |
| U+FB03        | ffi      | U+FB03 (standard) |
| U+FB04        | ffl      | U+FB04 (standard) |
| U+FB05        | ſt       | U+FB05 (standard) |
| U+FB06        | st       | U+FB06 (standard) |
| U+E001        | fi       | private block      |
| U+E002        | fl       | private block      |
| U+E003        | ff       | private block      |
| U+E004        | ffi      | private block      |
| U+E005        | ffl      | private block      |

`mapPUALigatures(s string) string` replaces these using a package-level
`puaLigatureReplacer` (`strings.NewReplacer` — trie-based, single O(n) scan).

Fast-path: if the string contains no rune in the PUA range (U+E000–U+F8FF), it is
returned unchanged without allocating. This makes the call cost negligible for the
vast majority of strings that don't use PUA codepoints.

Called at the end of both `bytesToUTF8` and `utf16BEToString`.

---

## Header sanitisation

`sanitisePDFHeader` handles two real-world quirks:

1. **Leading whitespace before `%PDF-`**: some generators (e.g. email attachments) prepend
   whitespace. Stripped with `bytes.TrimLeft`.
2. **Trailing spaces on the header line**: `libtiff/tiff2pdf` emits `%PDF-1.4 \n`.
   Only ASCII space (0x20) and tab (0x09) are stripped — carriage-return bytes are
   intentionally preserved because they may be part of the binary comment marker on
   line 2 (`%\xe2\xe3\xcf\xd3\r`), and stripping them would corrupt all xref offsets.

---

## Real-world corpus test

`server/internal/parser/realworld_test.go` — `TestPDFParser_RealWorldCorpus`

Run against `/home/albyhernandez/Documents/Github/achetronic/normativa` (394 PDFs):

```
PDF_CORPUS_DIR=/path/to/normativa go test ./internal/parser/... -run TestPDFParser_RealWorldCorpus -v -timeout 600s
```

**Current results** (after all fixes): `total=394 ok=392 empty=2 failed=0`

The 2 permanently empty files are genuinely scanned image PDFs with no text layer:
- `espana/guias_canarias/Manual_manipuladores_alimentos_Canarias.pdf` (42 MB, 27 pages, rotated 90°, GPL Ghostscript from a scanned Word doc)
- `espana/guias_canarias/Manual_manipuladores_quesos_Canarias.pdf` (same origin)

`pdftotext` also returns empty for both — confirmed image-only, not a parser bug.

---

## Open work items (resoluble)

All previously tracked items have been resolved. No open items remain.

---

## Impossible issues (will not fix)

These cannot be resolved without fundamentally changing the architecture or adding
external dependencies that are out of scope.

### CID / Identity-H fonts without ToUnicode

Fonts with `/Encoding /Identity-H` encode text as 2-byte CID values. Without a
`/ToUnicode` CMap stream in the font dictionary, there is no mapping from CID to
Unicode — the information simply does not exist in the file. The bytes are decoded
as pairs of Latin-1 characters, producing garbage.

This is not a bug in the parser. It is a property of the PDF: the author embedded
a font without including the reverse mapping. Common in older government documents
and some Asian-language PDFs. The only solutions — heuristic glyph-name matching or
OCR — are either unreliable or require external dependencies (Tesseract).

### Multi-column reading order

The parser processes text operators in stream order, which is rendering order (not
reading order). In multi-column layouts, text from the left and right columns is
interleaved. Correct reconstruction requires knowing the absolute X/Y position of
every glyph, clustering glyphs into text lines, sorting columns left-to-right —
effectively a full layout engine.

This is what PDF viewers implement. It is out of scope for a text extraction parser
that intentionally avoids CGo and external binaries. The extracted text is still
complete and semantically correct for RAG purposes; the word order within a page
may be scrambled in multi-column documents.

### Scanned / image-only PDFs

PDFs that contain only raster images (no text layer) cannot yield any text without
OCR. The ingestion pipeline marks these as `failed` with `parser returned empty text`.
The only solution is Tesseract or a cloud OCR API — both out of scope for v1.
