package editor

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxFileSize = 16 << 20

type snapshot struct {
	text   string
	cursor int
}

// Buffer stores Unicode code points. History records editing transactions, not keystrokes.
type Buffer struct {
	Path        string
	Text        []rune
	Cursor      int
	saved       string
	disk        []byte
	exists      bool
	crlf        bool
	undo, redo  []snapshot
	transaction bool
}

func NewBuffer() *Buffer { return &Buffer{} }

func Open(path string) (*Buffer, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	b := &Buffer{Path: abs}
	info, err := os.Stat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file: %s", path)
	}
	if info.Size() > maxFileSize {
		return nil, fmt.Errorf("file exceeds the 16 MiB limit")
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("only UTF-8 text files are supported")
	}
	b.disk = data
	b.exists = true
	b.crlf = bytes.Contains(data, []byte("\r\n")) && !bytes.Contains(bytes.ReplaceAll(data, []byte("\r\n"), nil), []byte("\n"))
	text := string(data)
	if b.crlf {
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	b.Text = []rune(text)
	b.saved = text
	return b, nil
}

func (b *Buffer) Dirty() bool { return string(b.Text) != b.saved }
func (b *Buffer) checkpoint() {
	if b.transaction {
		return
	}
	b.undo = append(b.undo, snapshot{string(b.Text), b.Cursor})
	if len(b.undo) > 200 {
		b.undo = b.undo[1:]
	}
	b.redo = nil
	b.transaction = true
}
func (b *Buffer) End() { b.transaction = false }
func (b *Buffer) Replace(start, end int, text string) {
	start = min(max(start, 0), len(b.Text))
	end = min(max(end, start), len(b.Text))
	if string(b.Text[start:end]) == text {
		return
	}
	b.checkpoint()
	next := make([]rune, 0, len(b.Text)-(end-start)+len([]rune(text)))
	next = append(next, b.Text[:start]...)
	next = append(next, []rune(text)...)
	next = append(next, b.Text[end:]...)
	b.Text = next
	b.Cursor = start + len([]rune(text))
}
func (b *Buffer) Undo(redo bool) {
	b.End()
	from, to := &b.undo, &b.redo
	if redo {
		from, to = to, from
	}
	if len(*from) == 0 {
		return
	}
	*to = append(*to, snapshot{string(b.Text), b.Cursor})
	s := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	b.Text, b.Cursor = []rune(s.text), s.cursor
}
func (b *Buffer) LineStart(p int) int {
	p = min(max(0, p), len(b.Text))
	for p > 0 && b.Text[p-1] != '\n' {
		p--
	}
	return p
}
func (b *Buffer) LineEnd(p int) int {
	p = min(max(0, p), len(b.Text))
	for p < len(b.Text) && b.Text[p] != '\n' {
		p++
	}
	return p
}
func (b *Buffer) RowCol() (int, int) {
	row, col := 0, 0
	for _, r := range b.Text[:min(b.Cursor, len(b.Text))] {
		if r == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return row, col
}
func (b *Buffer) Offset(row, col int) int {
	p := 0
	for row > 0 && p < len(b.Text) {
		if b.Text[p] == '\n' {
			row--
		}
		p++
	}
	return min(p+max(0, col), b.LineEnd(p))
}
func (b *Buffer) NormalCursor() {
	b.Cursor = min(max(0, b.Cursor), len(b.Text))
	start, end := b.LineStart(b.Cursor), b.LineEnd(b.Cursor)
	if b.Cursor == end && end > start {
		b.Cursor--
	}
}

// Save uses a temporary sibling and rename so a failed write leaves the old file intact.
// A changed on-disk file or an unrelated existing destination requires explicit force.
func (b *Buffer) Save(path string, force bool) error {
	if path == "" {
		path = b.Path
	}
	if path == "" {
		return errors.New("use :w filename")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	target := abs
	if resolved, e := filepath.EvalSymlinks(abs); e == nil {
		target = resolved
	}
	mode := os.FileMode(0644)
	info, statErr := os.Stat(target)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if statErr == nil {
		if !info.Mode().IsRegular() {
			return errors.New("destination is not a regular file")
		}
		mode = info.Mode().Perm()
		if !force {
			if abs != b.Path || !b.exists {
				return errors.New("file already exists; use :w! to overwrite")
			}
			current, e := os.ReadFile(target)
			if e != nil {
				return e
			}
			if !bytes.Equal(current, b.disk) {
				return errors.New("file changed on disk; use :w! to overwrite")
			}
		}
	} else if !force && abs == b.Path && b.exists {
		return errors.New("file was removed from disk; use :w! to recreate it")
	}
	text := string(b.Text)
	if b.crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	f, err := os.CreateTemp(filepath.Dir(target), ".miv-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.WriteString(text)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), target); err != nil {
		return err
	}
	b.Path = abs
	b.saved = string(b.Text)
	b.disk = []byte(text)
	b.exists = true
	return nil
}
