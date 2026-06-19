package builtinutil

import (
	"strings"
	"unicode/utf8"
)

// Align takes a sequence of rows (each row is a sequence of cells)
// and renders them as a column-aligned table. Cells in the same column
// are padded with sep so their right edges align. The trailing column
// is NOT padded.
//
// Empty rows produce an empty line. nil rows is a zero-length result.
// The separator must be non-empty.
//
// Used by the `column -t` built-in (Wave C) and is shared with Wave B's
// `ls` long-form / multi-column output.
func Align(rows [][]string, sep string) string {
	if sep == "" {
		sep = "  "
	}
	if len(rows) == 0 {
		return ""
	}
	// Compute column widths in display-runes (NOT bytes — multi-byte
	// chars must occupy their visible width).
	widths := make([]int, 0)
	for _, row := range rows {
		for i, cell := range row {
			w := utf8.RuneCountInString(cell)
			if i >= len(widths) {
				widths = append(widths, w)
			} else if w > widths[i] {
				widths[i] = w
			}
		}
	}
	var b strings.Builder
	for ri, row := range rows {
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				// Pad to column width then add sep.
				pad := widths[i] - utf8.RuneCountInString(cell)
				if pad > 0 {
					b.WriteString(strings.Repeat(" ", pad))
				}
				b.WriteString(sep)
			}
		}
		if ri < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
