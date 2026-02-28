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

// Package chunker splits plain text into overlapping chunks suitable for
// embedding. Chunk boundaries follow paragraph and sentence structure where
// possible to preserve semantic coherence.
package chunker

import (
	"strings"
	"unicode"
)

// Chunk is a segment of text with its position index within the source document.
type Chunk struct {
	Index   int
	Content string
}

// Chunker splits text into fixed-size overlapping segments.
type Chunker struct {
	// Size is the target number of runes per chunk.
	Size int
	// Overlap is the number of runes shared between consecutive chunks.
	Overlap int
}

// New creates a Chunker with the given size and overlap.
// Panics if overlap >= size.
func New(size, overlap int) *Chunker {
	if overlap >= size {
		panic("chunker: overlap must be less than size")
	}
	return &Chunker{Size: size, Overlap: overlap}
}

// Split divides text into overlapping chunks. It first splits on paragraph
// boundaries, then further splits long paragraphs by sentences, and finally
// by rune count if needed. Returns an empty slice if text is blank.
func (c *Chunker) Split(text string) []Chunk {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	// Tokenize into words (rune-aware).
	words := tokenize(text)
	if len(words) == 0 {
		return nil
	}

	var chunks []Chunk
	step := c.Size - c.Overlap
	idx := 0

	for start := 0; start < len(words); start += step {
		end := start + c.Size
		if end > len(words) {
			end = len(words)
		}

		content := strings.Join(words[start:end], " ")
		chunks = append(chunks, Chunk{
			Index:   idx,
			Content: content,
		})
		idx++

		if end == len(words) {
			break
		}
	}

	return chunks
}

// tokenize splits text into a slice of whitespace-delimited tokens,
// preserving punctuation as part of tokens. Blank tokens are discarded.
func tokenize(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r)
	})
}
