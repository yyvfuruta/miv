package editor

import (
	"path/filepath"
	"strings"
)

func (e *Editor) isYAML() bool {
	ext := strings.ToLower(filepath.Ext(e.Buffer().Path))
	return ext == ".yaml" || ext == ".yml"
}

// newLineIndent uses only text before the split, so Enter in the middle of
// a line does not treat characters after the cursor as a completed YAML key.
func (e *Editor) newLineIndent(p int) string {
	b := e.Buffer()
	start := b.LineStart(p)
	prefix := string(b.Text[start:p])
	indent := prefix[:len(prefix)-len(strings.TrimLeft(prefix, " \t"))]
	if !e.isYAML() || e.inYAMLScalar(start, len(indent)) {
		return indent
	}
	code, colon, complete := yamlCode(strings.TrimSpace(prefix))
	if !complete || code == "" {
		return indent
	}
	listIndent := 0
	content := code
	for strings.HasPrefix(content, "- ") {
		listIndent += 2
		content = strings.TrimLeft(content[2:], " \t")
	}
	if code == "-" {
		return indent + "  "
	}
	if colon >= 0 {
		// List mappings align subsequent fields after the sequence marker.
		indent += strings.Repeat(" ", listIndent)
		value := strings.TrimSpace(string([]rune(code)[colon+1:]))
		if value == "" {
			return indent + "  "
		}
		if width, ok := yamlBlockWidth(value); ok {
			return indent + strings.Repeat(" ", width)
		}
	} else if width, ok := yamlBlockWidth(content); ok {
		// A scalar sequence item ('- |') nests one level below its marker.
		if listIndent > 0 {
			return indent + strings.Repeat(" ", max(listIndent, width))
		}
	}
	return indent
}

// yamlCode strips comments outside quotes and finds mapping separators outside
// flow collections. This is deliberately an indentation heuristic, not a parser.
func yamlCode(line string) (string, int, bool) {
	r := []rune(line)
	var quote rune
	depth, colon := 0, -1
	for i := 0; i < len(r); i++ {
		c := r[i]
		if quote != 0 {
			if quote == '"' && c == '\\' {
				i++
				continue
			}
			if c == quote {
				if quote == '\'' && i+1 < len(r) && r[i+1] == '\'' {
					i++
					continue
				}
				quote = 0
			}
			continue
		}
		switch c {
		case '#':
			if i == 0 || r[i-1] == ' ' || r[i-1] == '\t' {
				return strings.TrimRight(string(r[:i]), " \t"), colon, depth == 0
			}
		case '\'', '"':
			if i == 0 || strings.ContainsRune(" \t:[{,-?", r[i-1]) {
				quote = c
			}
		case '[', '{':
			depth++
		case ']', '}':
			depth = max(0, depth-1)
		case ':':
			if depth == 0 && (i+1 == len(r) || r[i+1] == ' ' || r[i+1] == '\t') {
				colon = i
			}
		}
	}
	return strings.TrimRight(line, " \t"), colon, quote == 0 && depth == 0
}

func yamlBlockWidth(value string) (int, bool) {
	if value == "" || (value[0] != '|' && value[0] != '>') {
		return 0, false
	}
	width, sign := 2, false
	explicit := false
	for _, r := range value[1:] {
		if r == '+' || r == '-' {
			if sign {
				return 0, false
			}
			sign = true
		} else if r >= '1' && r <= '9' {
			if explicit {
				return 0, false
			}
			width = int(r - '0')
			explicit = true
		} else {
			return 0, false
		}
	}
	return width, true
}

// Scalar text can end with ':' or contain '#'; it must not be reinterpreted
// as a YAML mapping. Walk preceding ancestor indentation levels, skipping
// earlier siblings, to find an enclosing scalar header.
func (e *Editor) inYAMLScalar(start, indentation int) bool {
	b := e.Buffer()
	ceiling := indentation
	for start > 0 {
		end := start - 1
		start = b.LineStart(end)
		line := string(b.Text[start:end])
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parentIndent := len(line) - len(trimmed)
		if parentIndent >= ceiling {
			continue
		}
		ceiling = parentIndent
		code, colon, complete := yamlCode(trimmed)
		if !complete {
			continue
		}
		listIndent := 0
		content := code
		for strings.HasPrefix(content, "- ") {
			listIndent += 2
			content = strings.TrimLeft(content[2:], " \t")
		}
		if colon >= 0 {
			width, ok := yamlBlockWidth(strings.TrimSpace(string([]rune(code)[colon+1:])))
			if ok {
				return indentation >= parentIndent+listIndent+width
			}
		} else if width, ok := yamlBlockWidth(content); ok {
			return indentation >= parentIndent+max(listIndent, width)
		}
	}
	return false
}

func (e *Editor) unindentInsert() {
	b := e.Buffer()
	start := b.LineStart(b.Cursor)
	end := e.firstNonblank(b.Cursor)
	if start == end {
		return
	}
	remove := 0
	if b.Text[start] == '\t' {
		remove = 1
	} else {
		for start+remove < end && remove < e.tabSize() && b.Text[start+remove] == ' ' {
			remove++
		}
	}
	cursor := b.Cursor
	b.Replace(start, start+remove, "")
	b.Cursor = max(start, cursor-remove)
}
