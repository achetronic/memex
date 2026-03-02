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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPDFParser_Testdata parses every PDF in testdata/ and verifies expected
// outcomes. The corpus was chosen to cover edge cases found in real-world
// documents:
//
//   - text_pdf12_trailing_space_header.pdf  — PDF 1.2, header ends with a
//     space before \r ("%PDF-1.2 \r"), text-bearing; exercises the trailing-
//     space sanitisation path in sanitisePDFHeader.
//
//   - text_pdf15_linearised.pdf  — PDF 1.5, CRLF+binary-comment header, linearised
//     PDF; contains a text layer (the "image-only" label in earlier session
//     notes was incorrect — the parser successfully extracts text).
//
//   - text_pdf15_crlf_header.pdf  — PDF 1.5, CRLF-terminated header with a
//     binary comment on line 2 ("%PDF-1.5\r%\xe2\xe3\xcf\xd3\r\n"); was one
//     of 13 files that failed before the sanitisePDFHeader \r-stripping bug
//     was fixed.
//
//   - text_pdf15_crlf_binary_comment.pdf  — PDF 1.5, same CRLF+binary-comment
//     header pattern; additional text-bearing document exercising xref-stream
//     parsing.
func TestPDFParser_Testdata(t *testing.T) {
	cases := []struct {
		file      string
		wantEmpty bool
		wantText  string
	}{
		{
			file:      "text_pdf12_trailing_space_header.pdf",
			wantEmpty: false,
			wantText:  "",
		},
		{
			file:      "text_pdf15_linearised.pdf",
			wantEmpty: false,
			wantText:  "CODEX",
		},
		{
			file:      "text_pdf15_crlf_header.pdf",
			wantEmpty: false,
			wantText:  "",
		},
		{
			file:      "text_pdf15_crlf_binary_comment.pdf",
			wantEmpty: false,
			wantText:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join("testdata", tc.file)
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("open %s: %v", path, err)
			}
			defer f.Close()

			p := &PDFParser{}
			text, err := p.Parse(f)
			if err != nil {
				t.Fatalf("Parse(%s) error: %v", tc.file, err)
			}

			if tc.wantEmpty {
				if strings.TrimSpace(text) != "" {
					t.Fatalf("expected empty text for %s, got %q (first 200 chars)",
						tc.file, truncate(text, 200))
				}
				return
			}

			if strings.TrimSpace(text) == "" {
				t.Fatalf("expected non-empty text for %s but got empty string", tc.file)
			}

			if tc.wantText != "" && !strings.Contains(text, tc.wantText) {
				t.Fatalf("expected %q in output of %s, got %q (first 200 chars)",
					tc.wantText, tc.file, truncate(text, 200))
			}
		})
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
