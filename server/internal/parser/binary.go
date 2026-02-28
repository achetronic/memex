package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDFParser extracts text from PDF files using the ledongthuc/pdf library.
type PDFParser struct{}

func (p *PDFParser) Extensions() []string { return []string{".pdf"} }

// Parse reads a PDF from r, extracts text from all pages, and returns
// the concatenated content with page breaks as newlines.
func (p *PDFParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading PDF bytes: %w", err)
	}

	reader, err := pdf.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("opening PDF: %w", err)
	}

	var buf strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			// Non-fatal: skip unreadable pages and continue.
			continue
		}
		buf.WriteString(text)
		buf.WriteRune('\n')
	}

	return buf.String(), nil
}

// DOCXParser extracts text from .docx files (Office Open XML format).
// A .docx is a ZIP archive containing word/document.xml with the body text.
type DOCXParser struct{}

func (p *DOCXParser) Extensions() []string { return []string{".docx"} }

// Parse reads a .docx from r, extracts word/document.xml from the ZIP archive,
// and returns the concatenated paragraph text.
func (p *DOCXParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading DOCX bytes: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("opening DOCX ZIP: %w", err)
	}

	return extractOOXMLText(zr, "word/document.xml")
}

// ODTParser extracts text from .odt files (OpenDocument Text format).
// An .odt is a ZIP archive containing content.xml with the document body.
type ODTParser struct{}

func (p *ODTParser) Extensions() []string { return []string{".odt"} }

// Parse reads a .odt from r, extracts content.xml from the ZIP archive,
// and returns the concatenated text content.
func (p *ODTParser) Parse(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("reading ODT bytes: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", fmt.Errorf("opening ODT ZIP: %w", err)
	}

	return extractOOXMLText(zr, "content.xml")
}

// extractOOXMLText opens the named file inside a ZIP reader, decodes its XML,
// and returns all CharData (text) content concatenated as plain text.
// This works for both DOCX (word/document.xml) and ODT (content.xml).
func extractOOXMLText(zr *zip.Reader, target string) (string, error) {
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
			if cd, ok := tok.(xml.CharData); ok {
				text := strings.TrimSpace(string(cd))
				if text != "" {
					buf.WriteString(text)
					buf.WriteRune('\n')
				}
			}
		}
		return buf.String(), nil
	}

	return "", fmt.Errorf("file %q not found in archive", target)
}
