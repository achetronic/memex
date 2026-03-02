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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const realPDFRootEnv = "PDF_CORPUS_DIR"

// TestPDFParser_RealWorldCorpus runs the PDF parser against every .pdf file
// found under realPDFRoot. The test fails if any file returns a parse error.
// Files that parse successfully but yield no text are reported as warnings —
// they are image-only or encrypted PDFs where empty output is expected.
//
// This test requires the PDF_CORPUS_DIR environment variable to point to a
// directory tree of .pdf files. It is skipped automatically in CI when the
// variable is not set. Run locally with a sufficient timeout:
//
//	PDF_CORPUS_DIR=/path/to/pdfs go test ./internal/parser/... -run TestPDFParser_RealWorldCorpus -v -timeout 600s
func TestPDFParser_RealWorldCorpus(t *testing.T) {
	realPDFRoot := os.Getenv(realPDFRootEnv)
	if realPDFRoot == "" {
		t.Skipf("%s not set, skipping real-world corpus test", realPDFRootEnv)
	}

	var files []string
	err := filepath.Walk(realPDFRoot, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.ToLower(filepath.Ext(p)) == ".pdf" {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking corpus directory: %v", err)
	}
	sort.Strings(files)

	if len(files) == 0 {
		t.Skip("no PDF files found in corpus directory")
	}

	p := &PDFParser{}

	type outcome struct {
		rel      string
		err      error
		empty    bool
		duration time.Duration
	}

	var failures []outcome
	var empties []outcome
	var slow []outcome
	ok := 0

	const slowThreshold = 5 * time.Second

	for i, f := range files {
		rel, _ := filepath.Rel(realPDFRoot, f)

		data, err := os.ReadFile(f)
		if err != nil {
			t.Logf("[%d/%d] ERROR reading %s: %v", i+1, len(files), rel, err)
			failures = append(failures, outcome{rel, fmt.Errorf("reading file: %w", err), false, 0})
			continue
		}

		start := time.Now()
		t.Logf("[%d/%d] parsing %s (%dKB)...", i+1, len(files), rel, len(data)/1024)
		text, err := p.Parse(bytes.NewReader(data))
		dur := time.Since(start)

		if err != nil {
			t.Logf("[%d/%d] FAIL %s in %v: %v", i+1, len(files), rel, dur, err)
			failures = append(failures, outcome{rel, err, false, dur})
			continue
		}

		if dur > slowThreshold {
			t.Logf("[%d/%d] SLOW %s took %v", i+1, len(files), rel, dur)
			slow = append(slow, outcome{rel, nil, false, dur})
		}

		if strings.TrimSpace(text) == "" {
			t.Logf("[%d/%d] EMPTY %s in %v", i+1, len(files), rel, dur)
			empties = append(empties, outcome{rel, nil, true, dur})
			continue
		}

		ok++
	}

	// Report slow files (informational).
	if len(slow) > 0 {
		t.Logf("── Slow files (>%v): %d ──", slowThreshold, len(slow))
		for _, o := range slow {
			t.Logf("  SLOW  %s  (%v)", o.rel, o.duration)
		}
	}

	// Report empties as informational (not failures) — these are likely
	// scanned image PDFs or PDFs with no text layer.
	if len(empties) > 0 {
		t.Logf("── Empty output (image-only or no text layer): %d files ──", len(empties))
		for _, o := range empties {
			t.Logf("  EMPTY  %s", o.rel)
		}
	}

	// Report summary.
	t.Logf("── Summary: total=%d ok=%d empty=%d slow=%d failed=%d ──",
		len(files), ok, len(empties), len(slow), len(failures))

	// Fail the test for each file that returned a parse error.
	for _, o := range failures {
		t.Errorf("FAIL  %s\n      %v", o.rel, o.err)
	}
}
