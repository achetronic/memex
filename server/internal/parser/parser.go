// Package parser provides document text extraction for multiple file formats.
// Each format is implemented as a separate type satisfying the Parser interface.
// The registry maps file extensions to their corresponding parser.
package parser

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// Parser extracts plain text from a document of a specific format.
// Implementations must be stateless and safe for concurrent use.
type Parser interface {
	// Parse reads from r and returns the full extracted text.
	// It returns an error if the format is invalid or extraction fails.
	Parse(r io.Reader) (string, error)

	// Extensions returns the list of file extensions this parser handles,
	// e.g. []string{".pdf"} or []string{".yaml", ".yml"}.
	Extensions() []string
}

// registry maps lowercase file extensions (with dot) to their parser.
var registry = map[string]Parser{}

// init registers all built-in parsers.
func init() {
	register(
		&TXTParser{},
		&MarkdownParser{},
		&HTMLParser{},
		&CSVParser{},
		&JSONParser{},
		&YAMLParser{},
		&TOMLParser{},
		&XMLParser{},
		&RTFParser{},
		&EMLParser{},
		&ODTParser{},
		&DOCXParser{},
		&PDFParser{},
	)
}

// register adds a parser to the registry for all its declared extensions.
func register(parsers ...Parser) {
	for _, p := range parsers {
		for _, ext := range p.Extensions() {
			registry[strings.ToLower(ext)] = p
		}
	}
}

// ForFile returns the Parser appropriate for the given filename based on its
// extension. Returns an error if the format is not supported.
func ForFile(filename string) (Parser, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	p, ok := registry[ext]
	if !ok {
		return nil, fmt.Errorf("unsupported file format: %q", ext)
	}
	return p, nil
}

// SupportedExtensions returns a sorted list of all registered extensions.
func SupportedExtensions() []string {
	exts := make([]string, 0, len(registry))
	for ext := range registry {
		exts = append(exts, ext)
	}
	return exts
}
