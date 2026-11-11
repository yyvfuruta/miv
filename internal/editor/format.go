package editor

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

const formatWidth = 80

// Formatting is linewise, including lines touched by a character selection.
func (e *Editor) formatSelection(keepCursor bool) {
	b := e.Buffer()
	start, end := e.selection()
	start = b.LineStart(start)
	end = b.LineEnd(max(start, end-1))
	old := b.Text[start:end]
	cursor := b.Cursor
	// Track the cursor by its non-whitespace offset so wrapping keeps it on
	// the same character even when preceding line breaks change.
	offset := 0
	for _, r := range b.Text[start:min(cursor, end)] {
		if !unicode.IsSpace(r) {
			offset++
		}
	}
	formatted := reflow(string(old))
	b.Replace(start, end, formatted)
	e.Mode = Normal
	e.desiredCol = -1
	if keepCursor {
		b.Cursor = start
		for i, r := range []rune(formatted) {
			if !unicode.IsSpace(r) {
				if offset == 0 {
					b.Cursor = start + i
					return
				}
				offset--
			}
		}
		b.Cursor = min(cursor, start+len([]rune(formatted)))
		return
	}
	b.Cursor = e.firstNonblank(start + len([]rune(formatted)))
}

func reflow(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) == "" {
			result = append(result, lines[i])
			i++
			continue
		}
		indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
		var words []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			words = append(words, strings.Fields(lines[i])...)
			i++
		}
		indentWidth := 0
		for _, r := range indent {
			if r == '\t' {
				indentWidth += 8 - indentWidth%8
			} else {
				indentWidth++
			}
		}
		line, width := indent, indentWidth
		for _, word := range words {
			wordWidth := runewidth.StringWidth(word)
			if width > indentWidth {
				if width+1+wordWidth > formatWidth {
					result = append(result, line)
					line, width = indent, indentWidth
				} else {
					line += " "
					width++
				}
			}
			line += word
			width += wordWidth
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}
