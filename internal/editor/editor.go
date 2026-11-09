package editor

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

type Mode int

const (
	Normal Mode = iota
	Insert
	Visual
	VisualLine
	Command
	Search
	Picker
)

func (m Mode) String() string {
	return [...]string{"NORMAL", "INSERT", "VISUAL", "VISUAL LINE", "COMMAND", "SEARCH", "FILES"}[m]
}

type Editor struct {
	Buffers                     []*Buffer
	Active                      int
	Mode                        Mode
	Anchor                      int
	Prompt, Message             string
	Quit                        bool
	pending, count, motionCount string
	opCount                     int
	search                      string
	searchDir                   int
	searchOrigin                int
	register                    string
	lineRegister                bool
	desiredCol                  int
	record, repeat              []string
	replaying                   bool
	Files, Matches              []string
	PickIndex                   int
	Root                        string
	ClipboardRead               func() (string, error)
	ClipboardWrite              func(string) error
}

func New(paths []string) (*Editor, error) {
	e := &Editor{desiredCol: -1, Root: "."}
	for _, p := range paths {
		b, err := Open(p)
		if err != nil {
			return nil, err
		}
		e.Buffers = append(e.Buffers, b)
	}
	if len(e.Buffers) == 0 {
		e.Buffers = []*Buffer{NewBuffer()}
	}
	return e, nil
}
func (e *Editor) Buffer() *Buffer { return e.Buffers[e.Active] }
func (e *Editor) reset()          { e.pending = ""; e.count = ""; e.motionCount = ""; e.opCount = 0 }
func number(s string) int         { n, _ := strconv.Atoi(s); return min(max(n, 1), 10000) }
func (e *Editor) Handle(key string) {
	b := e.Buffer()
	old := string(b.Text)
	if !e.replaying && e.Mode == Normal && e.pending == "" && e.count == "" {
		e.record = nil
	}
	if !e.replaying {
		e.record = append(e.record, key)
	}
	e.handle(key)
	if e.Mode == Normal || e.Mode == Visual || e.Mode == VisualLine {
		e.Buffer().NormalCursor()
	}
	if !e.replaying && e.Mode == Normal && e.pending == "" && e.count == "" {
		// Insert sessions are committed on Escape, even when the last key only moves the cursor.
		if b.transaction && (old != string(b.Text) || key == "<Esc>") {
			e.repeat = append([]string(nil), e.record...)
		}
		b.End()
	}
}
func (e *Editor) handle(key string) {
	b := e.Buffer()
	if e.Mode == Command || e.Mode == Search || e.Mode == Picker {
		e.promptKey(key)
		return
	}
	if key == "<Esc>" {
		if e.Mode == Insert && b.Cursor > b.LineStart(b.Cursor) {
			b.Cursor--
		}
		e.Mode = Normal
		e.reset()
		return
	}
	if e.Mode == Insert {
		e.insertKey(key)
		return
	}
	if key == "<C-r>" {
		b.Undo(true)
		return
	}
	if key == "<C-p>" {
		e.StartPicker()
		return
	}
	if key == "<C-d>" || key == "<C-u>" || key == "<PageDown>" || key == "<PageUp>" {
		dir := 1
		if key == "<C-u>" || key == "<PageUp>" {
			dir = -1
		}
		e.vertical(15 * dir)
		return
	}
	visual := e.Mode == Visual || e.Mode == VisualLine
	if visual && strings.Contains("dycx><", key) && len(key) == 1 {
		if key == ">" || key == "<" {
			e.indent(key == ">")
			return
		}
		start, end := e.selection()
		op := key
		if op == "x" {
			op = "d"
		}
		line := e.Mode == VisualLine
		e.Mode = Normal
		e.operate(op, start, end, line)
		return
	}
	if key == "<Left>" {
		key = "h"
	}
	if key == "<Right>" {
		key = "l"
	}
	if key == "<Up>" {
		key = "k"
	}
	if key == "<Down>" {
		key = "j"
	}
	if key == "<Home>" {
		key = "0"
	}
	if key == "<End>" {
		key = "$"
	}
	if (e.pending == "" || e.pending == "d" || e.pending == "c" || e.pending == "y") && len(key) == 1 && key[0] >= '0' && key[0] <= '9' && (key != "0" || e.count != "" || e.motionCount != "") {
		if e.pending != "" && strings.Contains("dcy", e.pending) {
			e.motionCount += key
		} else {
			e.count += key
		}
		return
	}
	n := number(e.count)
	if e.pending != "" {
		e.pendingKey(key)
		return
	}
	if key == "g" || key == "f" || key == "F" || key == "t" || key == "T" || key == "r" || key == " " {
		e.pending = key
		return
	}
	if key == "d" || key == "c" || key == "y" || key == ">" || key == "<" {
		e.pending = key
		e.opCount = n
		e.count = ""
		return
	}
	if e.move(key, n) {
		e.count = ""
		return
	}
	e.count = ""
	e.desiredCol = -1
	switch key {
	case "i":
		e.Mode = Insert
	case "a":
		if b.Cursor < b.LineEnd(b.Cursor) {
			b.Cursor++
		}
		e.Mode = Insert
	case "I":
		b.Cursor = e.firstNonblank(b.Cursor)
		e.Mode = Insert
	case "A":
		b.Cursor = b.LineEnd(b.Cursor)
		e.Mode = Insert
	case "o", "O":
		p := b.LineEnd(b.Cursor)
		indent := e.indentation(b.Cursor)
		if key == "O" {
			p = b.LineStart(b.Cursor)
			b.Replace(p, p, indent+"\n")
			b.Cursor = p + len([]rune(indent))
		} else {
			b.Replace(p, p, "\n"+indent)
		}
		e.Mode = Insert
	case "v":
		if visual {
			e.Mode = Normal
		} else {
			e.Anchor = b.Cursor
			e.Mode = Visual
		}
	case "V":
		if e.Mode == VisualLine {
			e.Mode = Normal
		} else {
			if !visual {
				e.Anchor = b.Cursor
			}
			e.Mode = VisualLine
		}
	case "x", "s":
		e.operate("d", b.Cursor, min(b.Cursor+n, b.LineEnd(b.Cursor)), false)
		if key == "s" {
			e.Mode = Insert
		}
	case "D", "C", "Y":
		op := map[string]string{"D": "d", "C": "c", "Y": "y"}[key]
		e.operate(op, b.Cursor, b.LineEnd(b.Cursor), false)
	case "p", "P":
		e.paste(key == "p", n)
	case "u":
		for i := 0; i < n; i++ {
			b.Undo(false)
		}
	case "J":
		for i := 0; i < n; i++ {
			end := b.LineEnd(b.Cursor)
			if end >= len(b.Text) {
				break
			}
			next := e.firstNonblank(end + 1)
			b.Replace(end, next, " ")
			b.Cursor = end
		}
	case "~":
		start := b.Cursor
		end := min(start+n, b.LineEnd(start))
		r := append([]rune(nil), b.Text[start:end]...)
		for i, c := range r {
			if unicode.IsUpper(c) {
				r[i] = unicode.ToLower(c)
			} else {
				r[i] = unicode.ToUpper(c)
			}
		}
		b.Replace(start, end, string(r))
	case ":":
		e.Mode = Command
		e.Prompt = ""
	case "/", "?":
		e.Mode = Search
		e.Prompt = ""
		e.searchOrigin = b.Cursor
		e.searchDir = 1
		if key == "?" {
			e.searchDir = -1
		}
	case "n", "N":
		dir := e.searchDir
		if key == "N" {
			dir = -dir
		}
		for i := 0; i < n; i++ {
			e.find(e.search, dir)
		}
	case "*":
		start, end := e.wordObject(false)
		e.search = string(b.Text[start:end])
		e.searchDir = 1
		e.find(e.search, 1)
	case ".":
		if !e.replaying && len(e.repeat) > 0 {
			keys := append([]string(nil), e.repeat...)
			e.replaying = true
			for i := 0; i < n; i++ {
				for _, k := range keys {
					e.Handle(k)
				}
				b.End()
			}
			e.replaying = false
		}
	}
}
func (e *Editor) pendingKey(key string) {
	b := e.Buffer()
	pending := e.pending
	n := number(e.count)
	if pending == " " {
		if key >= "0" && key <= "9" && len(key) == 1 {
			i, _ := strconv.Atoi(key)
			if i == 0 {
				i = len(e.Buffers)
			}
			if i <= len(e.Buffers) {
				e.Active = i - 1
				e.desiredCol = -1
			}
			e.reset()
			return
		}
		if key == "f" {
			e.StartPicker()
			e.reset()
			return
		}
		if key == "s" {
			e.Execute("w")
		}
		if key == "b" {
			e.nextBuffer(1)
		}
		e.reset()
		return
	}
	if pending == "g" {
		if key == "g" {
			b.Cursor = b.Offset(n-1, 0)
		}
		if key == "t" {
			e.nextBuffer(1)
		}
		if key == "T" {
			e.nextBuffer(-1)
		}
		e.reset()
		return
	}
	if pending == "r" {
		if len([]rune(key)) == 1 {
			end := min(b.Cursor+n, b.LineEnd(b.Cursor))
			start := b.Cursor
			b.Replace(start, end, strings.Repeat(key, end-start))
			b.Cursor = start
		}
		e.reset()
		return
	}
	if strings.Contains("fFtT", pending) && len(pending) == 1 {
		e.findChar(key, pending, n)
		e.reset()
		return
	}
	if pending == ">" || pending == "<" {
		if key == pending {
			e.indent(pending == ">")
		}
		e.reset()
		return
	}
	op := pending[:1]
	total := max(e.opCount, 1) * number(e.motionCount)
	if len(pending) == 2 && (pending[1] == 'i' || pending[1] == 'a') {
		start, end, ok := e.textObject(key, pending[1] == 'a')
		if ok {
			e.operate(op, start, end, false)
		}
		e.reset()
		return
	}
	if key == "i" || key == "a" {
		e.pending += key
		return
	}
	if key == op {
		start := b.LineStart(b.Cursor)
		end := start
		for i := 0; i < total; i++ {
			end = b.LineEnd(end)
			if end < len(b.Text) {
				end++
			}
		}
		e.operate(op, start, end, true)
		e.reset()
		return
	}
	if pending == op+"g" {
		if key == "g" {
			e.operate(op, 0, b.LineEnd(b.Cursor)+boolInt(b.LineEnd(b.Cursor) < len(b.Text)), true)
		}
		e.reset()
		return
	}
	if key == "g" {
		e.pending += "g"
		return
	}
	start := b.Cursor
	if op == "c" && key == "w" && start < len(b.Text) && !unicode.IsSpace(b.Text[start]) {
		key = "e"
	}
	if e.move(key, total) {
		end := b.Cursor
		b.Cursor = start
		line := key == "j" || key == "k" || key == "G"
		lo, hi := min(start, end), max(start, end)
		if line {
			lo = b.LineStart(lo)
			hi = b.LineEnd(hi)
			if hi < len(b.Text) {
				hi++
			}
		} else if (key == "e" || key == "$" || key == "%") && b.LineStart(hi) < b.LineEnd(hi) {
			hi = min(hi+1, len(b.Text))
		}
		e.operate(op, lo, hi, line)
	}
	e.reset()
}
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
func (e *Editor) firstNonblank(p int) int {
	b := e.Buffer()
	p = b.LineStart(p)
	for p < b.LineEnd(p) && (b.Text[p] == ' ' || b.Text[p] == '\t') {
		p++
	}
	return p
}
func (e *Editor) indentation(p int) string {
	b := e.Buffer()
	start := b.LineStart(p)
	return string(b.Text[start:e.firstNonblank(p)])
}
func (e *Editor) vertical(n int) {
	b := e.Buffer()
	row, col := b.RowCol()
	if e.desiredCol < 0 {
		e.desiredCol = col
	}
	b.Cursor = b.Offset(max(0, row+n), e.desiredCol)
}
func class(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
		return 1
	}
	return 2
}
func (e *Editor) move(key string, n int) bool {
	b := e.Buffer()
	p := b.Cursor
	eol := b.LineEnd(p)
	if key == "j" || key == "k" {
		if key == "k" {
			n = -n
		}
		e.vertical(n)
		return true
	}
	e.desiredCol = -1
	switch key {
	case "h":
		p = max(b.LineStart(p), p-n)
	case "l":
		p = min(max(b.LineStart(p), eol-1), p+n)
	case "0":
		p = b.LineStart(p)
	case "^":
		p = e.firstNonblank(p)
	case "$":
		p = max(b.LineStart(p), eol-1)
	case "G":
		if e.count != "" || e.motionCount != "" {
			p = b.Offset(n-1, 0)
		} else {
			p = b.LineStart(len(b.Text))
		}
	case "w", "b", "e":
		for i := 0; i < n; i++ {
			if key == "w" {
				if p < len(b.Text) {
					c := class(b.Text[p])
					for p < len(b.Text) && class(b.Text[p]) == c {
						p++
					}
				}
				for p < len(b.Text) && class(b.Text[p]) == 0 {
					p++
				}
			}
			if key == "b" {
				if p > 0 {
					p--
				}
				for p > 0 && class(b.Text[p]) == 0 {
					p--
				}
				for p > 0 && class(b.Text[p-1]) == class(b.Text[p]) {
					p--
				}
			}
			if key == "e" {
				if p < len(b.Text)-1 {
					p++
				}
				for p < len(b.Text)-1 && class(b.Text[p]) == 0 {
					p++
				}
				for p < len(b.Text)-1 && class(b.Text[p+1]) == class(b.Text[p]) {
					p++
				}
			}
		}
	case "%":
		if p >= len(b.Text) {
			return true
		}
		pairs := "()[]{}"
		index := strings.IndexRune(pairs, b.Text[p])
		if index < 0 {
			return true
		}
		dir := 1
		if index%2 == 1 {
			dir = -1
		}
		own, other := b.Text[p], []rune(pairs)[index+dir]
		depth := 1
		for q := p + dir; q >= 0 && q < len(b.Text); q += dir {
			if b.Text[q] == own {
				depth++
			}
			if b.Text[q] == other {
				depth--
			}
			if depth == 0 {
				p = q
				break
			}
		}
	default:
		return false
	}
	b.Cursor = min(max(0, p), len(b.Text))
	return true
}
func (e *Editor) findChar(key, kind string, n int) {
	r := []rune(key)
	if len(r) != 1 {
		return
	}
	b := e.Buffer()
	p := b.Cursor
	dir := 1
	if kind == "F" || kind == "T" {
		dir = -1
	}
	for i := 0; i < n; i++ {
		found := false
		for q := p + dir; q >= b.LineStart(p) && q < b.LineEnd(p); q += dir {
			if b.Text[q] == r[0] {
				p = q
				found = true
				break
			}
		}
		if !found {
			return
		}
	}
	if kind == "t" || kind == "T" {
		p -= dir
	}
	b.Cursor = p
}
func (e *Editor) wordObject(around bool) (int, int) {
	b := e.Buffer()
	if len(b.Text) == 0 {
		return 0, 0
	}
	p := min(b.Cursor, len(b.Text)-1)
	start, end := p, p+1
	c := class(b.Text[p])
	for start > 0 && class(b.Text[start-1]) == c {
		start--
	}
	for end < len(b.Text) && class(b.Text[end]) == c {
		end++
	}
	if around {
		old := end
		for end < len(b.Text) && (b.Text[end] == ' ' || b.Text[end] == '\t') {
			end++
		}
		if old == end {
			for start > 0 && (b.Text[start-1] == ' ' || b.Text[start-1] == '\t') {
				start--
			}
		}
	}
	return start, end
}
func (e *Editor) textObject(key string, around bool) (int, int, bool) {
	if key == "w" {
		a, b := e.wordObject(around)
		return a, b, true
	}
	b := e.Buffer()
	pairs := map[string][2]rune{"(": {'(', ')'}, ")": {'(', ')'}, "b": {'(', ')'}, "[": {'[', ']'}, "]": {'[', ']'}, "{": {'{', '}'}, "}": {'{', '}'}, "B": {'{', '}'}, "\"": {'"', '"'}, "'": {'\'', '\''}, "`": {'`', '`'}}
	pair, ok := pairs[key]
	if !ok {
		return 0, 0, false
	}
	start := -1
	end := -1
	if pair[0] == pair[1] {
		for p := b.LineStart(b.Cursor); p < b.LineEnd(b.Cursor); p++ {
			if b.Text[p] != pair[0] || (p > 0 && b.Text[p-1] == '\\') {
				continue
			}
			if start < 0 {
				start = p
			} else {
				end = p
				if b.Cursor <= end {
					break
				}
				start = -1
				end = -1
			}
		}
	} else {
		depth := 0
		for p := b.Cursor; p >= 0; p-- {
			if b.TextAt(p) == pair[0] {
				if depth == 0 {
					start = p
					break
				}
				depth--
			} else if b.TextAt(p) == pair[1] && p != b.Cursor {
				depth++
			}
		}
		if start >= 0 {
			depth = 1
			for p := start + 1; p < len(b.Text); p++ {
				if b.Text[p] == pair[0] {
					depth++
				}
				if b.Text[p] == pair[1] {
					depth--
				}
				if depth == 0 {
					end = p
					break
				}
			}
		}
	}
	if start < 0 || end < 0 {
		return 0, 0, false
	}
	if around {
		return start, end + 1, true
	}
	return start + 1, end, true
}
func (b *Buffer) TextAt(p int) rune {
	if p < 0 || p >= len(b.Text) {
		return 0
	}
	return b.Text[p]
}
func (e *Editor) selection() (int, int) {
	b := e.Buffer()
	start, end := min(e.Anchor, b.Cursor), max(e.Anchor, b.Cursor)
	if e.Mode == VisualLine {
		start = b.LineStart(start)
		end = b.LineEnd(end)
		if end < len(b.Text) {
			end++
		}
	} else {
		end = min(end+1, len(b.Text))
	}
	return start, end
}
func (e *Editor) operate(op string, start, end int, line bool) {
	b := e.Buffer()
	if start == end && op != "c" {
		return
	}
	e.register = string(b.Text[start:end])
	e.lineRegister = line
	if e.ClipboardWrite != nil {
		if err := e.ClipboardWrite(e.register); err != nil {
			e.Message = "Clipboard unavailable; copy kept in the editor"
		}
	}
	if op == "y" {
		b.Cursor = start
		return
	}
	if op == "c" && line {
		if end > start && b.Text[end-1] == '\n' {
			end--
		}
		b.Replace(start, end, "")
		b.Cursor = start
		e.Mode = Insert
		return
	}
	// Removing the final line also removes its preceding separator.
	cutStart := start
	if line && end == len(b.Text) && start > 0 && (end == start || b.Text[end-1] != '\n') {
		cutStart--
	}
	b.Replace(cutStart, end, "")
	b.Cursor = cutStart
	if op == "c" {
		e.Mode = Insert
	}
}
func (e *Editor) paste(after bool, n int) {
	b := e.Buffer()
	text := e.register
	line := e.lineRegister
	if e.ClipboardRead != nil {
		if s, err := e.ClipboardRead(); err == nil && s != text {
			text = s
			line = strings.HasSuffix(s, "\n")
		}
	}
	if text == "" {
		return
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if len(text)*n > maxFileSize {
		e.Message = "Paste exceeds the 16 MiB limit"
		return
	}
	p := b.Cursor
	if line {
		text = strings.TrimSuffix(text, "\n") + "\n"
		text = strings.Repeat(text, n)
		if after {
			p = b.LineEnd(p)
			if p < len(b.Text) {
				p++
			} else {
				text = "\n" + strings.TrimSuffix(text, "\n")
			}
		} else {
			p = b.LineStart(p)
		}
	} else {
		text = strings.Repeat(text, n)
		if after && p < b.LineEnd(p) {
			p++
		}
	}
	b.Replace(p, p, text)
	b.Cursor = p
	if line && strings.HasPrefix(text, "\n") {
		b.Cursor++
	}
}
func (e *Editor) tabSize() int {
	switch strings.ToLower(filepath.Ext(e.Buffer().Path)) {
	case ".yaml", ".yml", ".sh", ".bash", ".json", ".jsonc":
		return 2
	}
	return 4
}
func (e *Editor) indent(right bool) {
	b := e.Buffer()
	start, end := b.LineStart(b.Cursor), b.LineEnd(b.Cursor)
	if e.Mode == Visual || e.Mode == VisualLine {
		start, end = e.selection()
		start = b.LineStart(start)
		if end > start && b.Text[end-1] == '\n' {
			end--
		}
		end = b.LineEnd(end)
	} else {
		for i := 1; i < max(e.opCount, 1) && end < len(b.Text); i++ {
			end = b.LineEnd(end + 1)
		}
	}
	lines := strings.Split(string(b.Text[start:end]), "\n")
	for i, s := range lines {
		if right {
			lines[i] = strings.Repeat(" ", e.tabSize()) + s
		} else {
			r := []rune(s)
			n := 0
			for n < len(r) && n < e.tabSize() && r[n] == ' ' {
				n++
			}
			if n == 0 && len(r) > 0 && r[0] == '\t' {
				n = 1
			}
			lines[i] = string(r[n:])
		}
	}
	b.Replace(start, end, strings.Join(lines, "\n"))
	b.Cursor = start
	e.Mode = Normal
}
func (e *Editor) insertKey(key string) {
	b := e.Buffer()
	p := b.Cursor
	switch key {
	case "<Backspace>":
		if p > 0 {
			b.Replace(p-1, p, "")
		}
	case "<Delete>":
		if p < len(b.Text) {
			b.Replace(p, p+1, "")
			b.Cursor = p
		}
	case "<Enter>", "<C-j>":
		indent := e.indentation(p)
		b.Replace(p, p, "\n"+indent)
	case "<Tab>":
		_, col := b.RowCol()
		b.Replace(p, p, strings.Repeat(" ", e.tabSize()-col%e.tabSize()))
	case "<Left>":
		b.Cursor = max(0, p-1)
	case "<Right>":
		b.Cursor = min(len(b.Text), p+1)
	case "<Up>":
		e.vertical(-1)
	case "<Down>":
		e.vertical(1)
	case "<Home>":
		b.Cursor = b.LineStart(p)
	case "<End>":
		b.Cursor = b.LineEnd(p)
	case "<C-w>":
		start := p
		for start > 0 && unicode.IsSpace(b.Text[start-1]) && b.Text[start-1] != '\n' {
			start--
		}
		if start > 0 {
			c := class(b.Text[start-1])
			for start > 0 && class(b.Text[start-1]) == c {
				start--
			}
		}
		b.Replace(start, p, "")
	default:
		if text, ok := strings.CutPrefix(key, "<Paste>"); ok {
			b.Replace(p, p, text)
		} else if len([]rune(key)) == 1 {
			b.Replace(p, p, key)
		}
	}
	if key != "<Up>" && key != "<Down>" {
		e.desiredCol = -1
	}
}
func (e *Editor) InsertPaste(text string) {
	if e.Mode != Insert {
		e.Message = "Enter INSERT mode to paste from the terminal"
		return
	}
	e.Handle("<Paste>" + strings.ReplaceAll(text, "\r\n", "\n"))
}
func (e *Editor) nextBuffer(dir int) {
	e.Buffer().End()
	e.Active = (e.Active + dir + len(e.Buffers)) % len(e.Buffers)
	e.desiredCol = -1
}
func (e *Editor) openPath(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		e.Message = err.Error()
		return
	}
	for i, b := range e.Buffers {
		if b.Path == abs {
			e.Active = i
			return
		}
	}
	b, err := Open(path)
	if err != nil {
		e.Message = err.Error()
		return
	}
	e.Buffers = append(e.Buffers, b)
	e.Active = len(e.Buffers) - 1
	e.desiredCol = -1
}
func (e *Editor) Execute(cmd string) {
	cmd = strings.TrimSpace(cmd)
	name, arg, _ := strings.Cut(cmd, " ")
	arg = strings.TrimSpace(arg)
	force := strings.HasSuffix(name, "!")
	name = strings.TrimSuffix(name, "!")
	b := e.Buffer()
	switch name {
	case "w", "write", "wq", "x":
		if err := b.Save(arg, force); err != nil {
			e.Message = err.Error()
			return
		}
		e.Message = fmt.Sprintf("Saved: %s", b.Path)
		if name == "wq" || name == "x" {
			e.closeBuffer(force)
		}
	case "q", "quit", "bd":
		e.closeBuffer(force)
	case "qa", "qall":
		for _, b := range e.Buffers {
			if b.Dirty() && !force {
				e.Message = "Unsaved changes; use :qa! to discard them"
				return
			}
		}
		e.Quit = true
	case "e", "edit", "tabnew":
		if arg != "" {
			e.openPath(arg)
		} else {
			e.Message = "Use :e path"
		}
	case "bn", "bnext", "tabnext":
		e.nextBuffer(1)
	case "bp", "bprevious", "tabprevious":
		e.nextBuffer(-1)
	case "b":
		n, err := strconv.Atoi(arg)
		if err == nil && n > 0 && n <= len(e.Buffers) {
			e.Active = n - 1
		} else {
			e.Message = "Invalid buffer"
		}
	case "ls", "buffers":
		var names []string
		for i, b := range e.Buffers {
			mark := ""
			if b.Dirty() {
				mark = "+"
			}
			names = append(names, fmt.Sprintf("%d:%s%s", i+1, filepath.Base(b.Path), mark))
		}
		e.Message = strings.Join(names, "  ")
	case "files":
		e.StartPicker()
	case "help", "h":
		e.Message = "hjkl wbe  i/a/o  v/V  d/c/y+motion  p  u/Ctrl-r  / n/N  :w :q  Ctrl-p  gt/gT"
	default:
		if n, err := strconv.Atoi(name); err == nil && n > 0 {
			b.Cursor = b.Offset(n-1, 0)
		} else {
			e.Message = "Unsupported command: " + cmd
		}
	}
}
func (e *Editor) closeBuffer(force bool) {
	if e.Buffer().Dirty() && !force {
		e.Message = "Unsaved changes; use :w to save or :q! to discard them"
		return
	}
	if len(e.Buffers) == 1 {
		e.Quit = true
		return
	}
	e.Buffers = append(e.Buffers[:e.Active], e.Buffers[e.Active+1:]...)
	e.Active = min(e.Active, len(e.Buffers)-1)
}
