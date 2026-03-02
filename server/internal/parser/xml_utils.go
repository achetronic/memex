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
)

// zipMagic is the magic byte sequence at the start of ZIP-based formats
// (DOCX, ODT, EPUB, …).
var zipMagic = []byte("PK")

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

// safeHead returns up to n bytes from b for use in diagnostic messages.
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
