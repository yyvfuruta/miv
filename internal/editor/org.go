package editor

import (
	"path/filepath"
	"regexp"
	"strings"
)

// OrgHeading uses rune offsets, like Buffer, and includes the entire subtree.
type OrgHeading struct {
	Start, End, Row, Level int
}

func (b *Buffer) IsOrg() bool { return strings.EqualFold(filepath.Ext(b.Path), ".org") }

// OrgHeadings ignores literal blocks so example text cannot become an outline.
func (b *Buffer) OrgHeadings() []OrgHeading {
	var headings []OrgHeading
	if !b.IsOrg() {
		return headings
	}
	offset, block := 0, ""
	for row, line := range strings.Split(string(b.Text), "\n") {
		upper := strings.ToUpper(strings.TrimSpace(line))
		if block != "" {
			if strings.HasPrefix(upper, "#+END_"+block) {
				block = ""
			}
		} else if strings.HasPrefix(upper, "#+BEGIN_") {
			fields := strings.Fields(strings.TrimPrefix(upper, "#+BEGIN_"))
			if len(fields) > 0 {
				block = fields[0]
			}
		} else {
			level := 0
			for level < len(line) && line[level] == '*' {
				level++
			}
			if level > 0 && level < len(line) && (line[level] == ' ' || line[level] == '\t') {
				headings = append(headings, OrgHeading{offset, len(b.Text), row, level})
			}
		}
		offset += len([]rune(line)) + 1
	}
	var stack []int
	for i := range headings {
		for len(stack) > 0 && headings[stack[len(stack)-1]].Level >= headings[i].Level {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			headings[last].End = headings[i].Start
		}
		stack = append(stack, i)
	}
	return headings
}

func (b *Buffer) orgHeading() (OrgHeading, bool) {
	headings := b.OrgHeadings()
	for i := len(headings) - 1; i >= 0; i-- {
		if headings[i].Start <= b.Cursor {
			return headings[i], true
		}
	}
	return OrgHeading{}, false
}

// OrgVisibleRows is shared by movement and rendering. Fold state is buffer-local.
func (b *Buffer) OrgVisibleRows() []int {
	count := 1
	var endRows map[int]int
	if len(b.orgFolds) > 0 && b.IsOrg() {
		endRows = make(map[int]int)
	}
	for i, r := range b.Text {
		if r == '\n' {
			if endRows != nil {
				endRows[i+1] = count
			}
			count++
		}
	}
	hidden := make(map[int]int)
	if len(b.orgFolds) > 0 && b.IsOrg() {
		for _, h := range b.OrgHeadings() {
			if b.orgFolds[h.Start] {
				end := count
				if h.End < len(b.Text) {
					end = endRows[h.End]
				}
				hidden[h.Row] = end
			}
		}
	}
	rows := make([]int, 0, count)
	for row := 0; row < count; row++ {
		rows = append(rows, row)
		if end, ok := hidden[row]; ok {
			row = max(row, end-1)
		}
	}
	return rows
}

func (b *Buffer) OrgFolded(offset int) bool { return b.IsOrg() && b.orgFolds[offset] }

func (b *Buffer) revealOrgCursor() {
	if len(b.orgFolds) == 0 {
		return
	}
	for _, h := range b.OrgHeadings() {
		if b.Cursor > b.LineEnd(h.Start) && b.Cursor < h.End {
			delete(b.orgFolds, h.Start)
		}
	}
}

func (e *Editor) orgAction(key string) {
	b := e.Buffer()
	if !b.IsOrg() {
		e.Message = "Org commands require an .org file"
		return
	}
	headings := b.OrgHeadings()
	if key == "[" || key == "]" {
		if key == "]" {
			for _, h := range headings {
				if h.Start > b.Cursor {
					b.Cursor = h.Start
					return
				}
			}
		} else {
			for i := len(headings) - 1; i >= 0; i-- {
				if headings[i].Start < b.LineStart(b.Cursor) {
					b.Cursor = headings[i].Start
					return
				}
			}
		}
		e.Message = "No more Org headings"
		return
	}
	if key == "x" {
		start, end := b.LineStart(b.Cursor), b.LineEnd(b.Cursor)
		line := string(b.Text[start:end])
		match := orgCheckbox.FindStringSubmatchIndex(line)
		if match != nil && !strings.HasPrefix(line, "*") {
			i := start + len([]rune(line[:match[4]]))
			state := "X"
			if b.Text[i] == 'X' || b.Text[i] == 'x' {
				state = " "
			}
			cursor := b.Cursor
			b.Replace(i, i+1, state)
			b.Cursor = cursor
			return
		}
		e.Message = "No Org checkbox on this line"
		return
	}
	if key == "<S-Tab>" {
		if len(b.orgFolds) > 0 {
			b.orgFolds = nil
		} else {
			b.orgFolds = make(map[int]bool)
			for _, h := range headings {
				if h.Level == 1 {
					b.orgFolds[h.Start] = true
				}
			}
			if h, ok := b.orgHeading(); ok {
				for _, parent := range headings {
					if parent.Start <= h.Start && parent.End > h.Start && parent.Level == 1 {
						b.Cursor = parent.Start
						break
					}
				}
			}
		}
		return
	}
	h, ok := b.orgHeading()
	if !ok {
		e.Message = "No Org heading at the cursor"
		return
	}
	switch key {
	case "<Tab>":
		if b.orgFolds == nil {
			b.orgFolds = make(map[int]bool)
		}
		if b.orgFolds[h.Start] {
			delete(b.orgFolds, h.Start)
		} else {
			b.orgFolds[h.Start] = true
		}
		b.Cursor = h.Start
	case "t":
		start := h.Start + h.Level
		for start < b.LineEnd(h.Start) && (b.Text[start] == ' ' || b.Text[start] == '\t') {
			start++
		}
		end := start
		for end < b.LineEnd(h.Start) && b.Text[end] != ' ' && b.Text[end] != '\t' {
			end++
		}
		switch string(b.Text[start:end]) {
		case "TODO":
			b.Replace(start, end, "DONE")
		case "DONE":
			for end < b.LineEnd(h.Start) && (b.Text[end] == ' ' || b.Text[end] == '\t') {
				end++
			}
			b.Replace(start, end, "")
		default:
			b.Replace(start, start, "TODO ")
		}
		b.Cursor = h.Start
	case "h", "l":
		if key == "h" && h.Level == 1 {
			e.Message = "Cannot promote a top-level heading"
			return
		}
		cursor := b.Cursor
		for i := len(headings) - 1; i >= 0; i-- {
			child := headings[i]
			if child.Start < h.Start || child.Start >= h.End {
				continue
			}
			if key == "l" {
				b.Replace(child.Start, child.Start, "*")
				if child.Start <= cursor {
					cursor++
				}
			} else {
				b.Replace(child.Start, child.Start+1, "")
				if child.Start < cursor {
					cursor--
				}
			}
		}
		b.Cursor = cursor
	case "i":
		prefix := "\n" + strings.Repeat("*", h.Level) + " "
		pos := h.End
		if pos > 0 && b.Text[pos-1] == '\n' {
			prefix = strings.TrimPrefix(prefix, "\n")
			prefix += "\n"
		}
		b.Replace(pos, pos, prefix)
		b.Cursor = pos + len([]rune(strings.TrimSuffix(prefix, "\n")))
		e.Mode = Insert
	}
	e.desiredCol = -1
}

var orgCheckbox = regexp.MustCompile(`^([ \t]*(?:[-+*]|[0-9]+[.)])[ \t]+)\[([ Xx-])\]`)
