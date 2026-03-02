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
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// CSVParser converts CSV files into line-by-line text where each row is
// joined by tabs, preserving tabular structure for embedding.
type CSVParser struct{}

func (p *CSVParser) Extensions() []string { return []string{".csv"} }

// Parse reads all CSV records from r and returns them as tab-separated lines.
func (p *CSVParser) Parse(r io.Reader) (string, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return "", fmt.Errorf("reading CSV: %w", err)
	}

	var buf strings.Builder
	for _, row := range records {
		buf.WriteString(strings.Join(row, "\t"))
		buf.WriteRune('\n')
	}
	return buf.String(), nil
}
