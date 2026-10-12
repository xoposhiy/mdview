// Package tabular turns CSV and TSV files into a table the Markdown renderer
// displays like any table written in Markdown.
package tabular

import (
	"bytes"
	"encoding/csv"
	"html"
	"io"
	"path/filepath"
	"strings"
)

// IsTable reports whether ext (as returned by filepath.Ext) names a CSV or
// TSV file. ext is compared case-insensitively.
func IsTable(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == ".csv" || ext == ".tsv"
}

// Prepare returns the Markdown source to render for the file at path. For a
// CSV or TSV file that is an HTML table; for any other file it is content
// unchanged.
//
// The table goes through the Markdown renderer instead of beside it, so it
// gets everything a Markdown table gets: the theme, the sortable headers, the
// scroller for wide tables, --export and "Save as HTML".
func Prepare(path string, content []byte) ([]byte, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if !IsTable(ext) {
		return content, nil
	}
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf")) // UTF-8 BOM, as Excel writes it
	var rows [][]string
	if ext == ".tsv" {
		rows = parseTSV(content)
	} else {
		var err error
		rows, err = parseCSV(content)
		if err != nil {
			return nil, err
		}
	}
	return toHTML(rows), nil
}

// parseCSV reads RFC 4180 CSV. The separator is a comma or, when the first
// line holds more of them, a semicolon: Excel writes semicolons in locales
// where the comma is the decimal separator.
func parseCSV(content []byte) ([][]string, error) {
	r := csv.NewReader(bytes.NewReader(content))
	r.Comma = sniffSeparator(content)
	r.FieldsPerRecord = -1 // rows may have different lengths
	r.LazyQuotes = true
	var rows [][]string
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, rec)
	}
}

// sniffSeparator counts commas and semicolons in the first line, outside
// quotes, and returns the one that occurs more often. A tie is a comma.
func sniffSeparator(content []byte) rune {
	commas, semicolons := 0, 0
	inQuotes := false
	for _, b := range content {
		switch b {
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				commas++
			}
		case ';':
			if !inQuotes {
				semicolons++
			}
		case '\n':
			if !inQuotes {
				if semicolons > commas {
					return ';'
				}
				return ','
			}
		}
	}
	if semicolons > commas {
		return ';'
	}
	return ','
}

// parseTSV splits content into lines and each line at tabs. TSV has no
// quoting, so a quote is an ordinary character. Empty lines are skipped.
func parseTSV(content []byte) [][]string {
	var rows [][]string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		rows = append(rows, strings.Split(line, "\t"))
	}
	return rows
}

// toHTML writes rows as a table. The first row is the header. Short rows are
// padded with empty cells, so every row has as many cells as the longest one.
//
// The output must reach the page as it is, so it keeps to what goldmark
// passes through as one raw HTML block: it starts with <table> and has no
// blank line in it. A line break inside a cell becomes <br>.
//
// A backslash becomes &#92;. The page shows the same character, but
// needsMathJax no longer finds `\(` or `\[` in the HTML, so a cell that holds
// one does not load MathJax, which would then typeset the data as a formula.
func toHTML(rows [][]string) []byte {
	if len(rows) == 0 {
		return nil
	}
	width := 0
	for _, row := range rows {
		width = max(width, len(row))
	}

	var b bytes.Buffer
	writeRow := func(row []string, tag string) {
		b.WriteString("<tr>")
		for i := 0; i < width; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			b.WriteString("<" + tag + ">")
			b.WriteString(escapeCell(cell))
			b.WriteString("</" + tag + ">")
		}
		b.WriteString("</tr>\n")
	}

	b.WriteString("<table>\n<thead>\n")
	writeRow(rows[0], "th")
	b.WriteString("</thead>\n<tbody>\n")
	for _, row := range rows[1:] {
		writeRow(row, "td")
	}
	b.WriteString("</tbody>\n</table>\n")
	return b.Bytes()
}

var cellReplacer = strings.NewReplacer(
	`\`, "&#92;",
	"\r\n", "<br>",
	"\n", "<br>",
	"\r", "<br>",
)

func escapeCell(s string) string {
	return cellReplacer.Replace(html.EscapeString(s))
}
