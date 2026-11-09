package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/yyvfuruta/vim/internal/editor"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "miv:", err)
		os.Exit(1)
	}
}
func run() error {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: miv [file ...]\nMinimal modal editor. Ctrl-p: files; :help: shortcuts; :q: quit.")
	}
	flag.Parse()
	e, err := editor.New(flag.Args())
	if err != nil {
		return err
	}
	e.ClipboardRead = readClipboard
	e.ClipboardWrite = writeClipboard
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err = screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.EnablePaste()
	v := view{screen: screen, e: e}
	pasting := false
	var paste strings.Builder
	for !e.Quit {
		v.draw()
		event := screen.PollEvent()
		if event == nil {
			break
		}
		switch ev := event.(type) {
		case *tcell.EventResize:
			screen.Sync()
		case *tcell.EventPaste:
			pasting = ev.Start()
			if pasting {
				paste.Reset()
			} else {
				e.InsertPaste(paste.String())
			}
		case *tcell.EventKey:
			if pasting {
				switch ev.Key() {
				case tcell.KeyRune:
					paste.WriteRune(ev.Rune())
				case tcell.KeyEnter, tcell.KeyCtrlJ:
					paste.WriteByte('\n')
				case tcell.KeyTab:
					paste.WriteByte('\t')
				}
				continue
			}
			e.Handle(keyName(ev))
		}
	}
	return nil
}
func keyName(ev *tcell.EventKey) string {
	if ev.Key() == tcell.KeyCtrlC {
		return "<Esc>"
	}
	if ev.Key() == tcell.KeyCtrlJ {
		return "<C-j>"
	}
	if ev.Key() == tcell.KeyRune {
		return string(ev.Rune())
	}
	keys := map[tcell.Key]string{tcell.KeyEscape: "<Esc>", tcell.KeyEnter: "<Enter>", tcell.KeyBackspace: "<Backspace>", tcell.KeyBackspace2: "<Backspace>", tcell.KeyDelete: "<Delete>", tcell.KeyTab: "<Tab>", tcell.KeyBacktab: "<S-Tab>", tcell.KeyLeft: "<Left>", tcell.KeyRight: "<Right>", tcell.KeyUp: "<Up>", tcell.KeyDown: "<Down>", tcell.KeyHome: "<Home>", tcell.KeyEnd: "<End>", tcell.KeyPgUp: "<PageUp>", tcell.KeyPgDn: "<PageDown>", tcell.KeyCtrlR: "<C-r>", tcell.KeyCtrlP: "<C-p>", tcell.KeyCtrlD: "<C-d>", tcell.KeyCtrlU: "<C-u>", tcell.KeyCtrlW: "<C-w>", tcell.KeyCtrlK: "<C-k>"}
	if k, ok := keys[ev.Key()]; ok {
		return k
	}
	return "<Unknown>"
}

type view struct {
	screen    tcell.Screen
	e         *editor.Editor
	top, left int
	last      *editor.Buffer
	syntax    syntaxCache
}

var (
	// Keep the editing area on the terminal palette (including its background).
	base     = tcell.StyleDefault
	muted    = base.Foreground(tcell.NewHexColor(0xbbbbbb))
	bar      = base.Foreground(tcell.NewHexColor(0xffffff)).Background(tcell.NewHexColor(0x222222))
	inactive = bar.Foreground(tcell.NewHexColor(0xbbbbbb))
	accent   = bar.Foreground(tcell.NewHexColor(0xeeeeee)).Background(tcell.NewHexColor(0x333333))
	selected = base.Foreground(tcell.NewHexColor(0xeeeeee)).Background(tcell.NewHexColor(0x333333))
)

// Printable substitutes keep control bytes in files from reaching the terminal.
func safeRune(r rune) rune {
	if unicode.IsControl(r) {
		return '�'
	}
	return r
}
func put(s tcell.Screen, x, y, limit int, text string, style tcell.Style) int {
	for _, r := range text {
		r = safeRune(r)
		width := runewidth.RuneWidth(r)
		if width == 0 {
			continue
		}
		if x+width > limit {
			break
		}
		s.SetContent(x, y, r, nil, style)
		x += width
	}
	return x
}
func fill(s tcell.Screen, y, w int, style tcell.Style) {
	for x := 0; x < w; x++ {
		s.SetContent(x, y, ' ', nil, style)
	}
}
func cellCol(text []rune, col, tab int) int {
	width := 0
	for _, r := range text[:min(col, len(text))] {
		if r == '\t' {
			width += tab - width%tab
		} else {
			width += runewidth.RuneWidth(safeRune(r))
		}
	}
	return width
}
func (v *view) draw() {
	s, e := v.screen, v.e
	w, h := s.Size()
	if w < 1 || h < 1 {
		return
	}
	s.SetStyle(base)
	s.Clear()
	b := e.Buffer()
	if v.last != b {
		v.top = 0
		v.left = 0
		v.last = b
	}
	row, col := b.RowCol()
	text := string(b.Text)
	v.syntax.update(b.Path, text)
	lines := strings.Split(text, "\n")
	height := max(1, h-3)
	margin := min(5, height/3)
	if row < v.top+margin {
		v.top = max(0, row-margin)
	}
	if row >= v.top+height-margin {
		v.top = max(0, row-height+margin+1)
	}
	gutter := len(fmt.Sprint(len(lines))) + 2
	contentW := max(1, w-gutter)
	tab := 4
	switch strings.ToLower(filepath.Ext(b.Path)) {
	case ".yaml", ".yml", ".sh", ".bash", ".json", ".jsonc":
		tab = 2
	}
	displayCol := cellCol([]rune(lines[row]), col, tab)
	if displayCol < v.left {
		v.left = displayCol
	}
	if displayCol >= v.left+contentW {
		v.left = displayCol - contentW + 1
	}
	// Waybar layout: buffer tags on the left, file title centered, position on the right.
	fill(s, 0, w, bar)
	position := fmt.Sprintf(" %d:%d ", row+1, col+1)
	right := max(0, w-runewidth.StringWidth(position))
	x := 0
	for i, buf := range e.Buffers {
		label := fmt.Sprintf(" %d ", i+1)
		if buf.Dirty() {
			label = fmt.Sprintf(" %d+ ", i+1)
		}
		style := inactive
		if i == e.Active {
			style = accent
		}
		x = put(s, x, 0, right, label, style)
	}
	name := filepath.Base(b.Path)
	if b.Path == "" {
		name = "[new]"
	}
	titleX := max(x+1, (w-runewidth.StringWidth(name))/2)
	if titleX < right-1 {
		put(s, titleX, 0, right-1, name, bar)
	}
	put(s, right, 0, w, position, bar)
	start, end := -1, -1
	if e.Mode == editor.Visual || e.Mode == editor.VisualLine {
		start, end = min(e.Anchor, b.Cursor), max(e.Anchor, b.Cursor)+1
		if e.Mode == editor.VisualLine {
			start = b.LineStart(start)
			end = b.LineEnd(end-1) + 1
		}
	}
	offset := 0
	for i := 0; i < v.top && i < len(lines); i++ {
		offset += len([]rune(lines[i])) + 1
	}
	tokenIndex := v.syntax.firstSpan(offset)
	for y := 1; y <= height && y < h; y++ {
		lineIndex := v.top + y - 1
		if lineIndex >= len(lines) {
			put(s, 0, y, w, "~", muted)
			continue
		}
		put(s, 0, y, w, fmt.Sprintf("%*d ", gutter-1, lineIndex+1), muted)
		cells := 0
		runes := []rune(lines[lineIndex])
		lastX := -1
		for j, r := range runes {
			style := v.syntax.styleAt(offset+j, &tokenIndex)
			if offset+j >= start && offset+j < end {
				style = selected
			}
			width := runewidth.RuneWidth(safeRune(r))
			if r == '\t' {
				width = tab - cells%tab
			}
			x := gutter + cells - v.left
			if width == 0 && lastX >= gutter {
				main, comb, st, _ := s.GetContent(lastX, y)
				s.SetContent(lastX, y, main, append(comb, r), st)
				continue
			}
			if x >= gutter && x+width <= w {
				if r == '\t' {
					for k := 0; k < width; k++ {
						s.SetContent(x+k, y, ' ', nil, style)
					}
				} else {
					s.SetContent(x, y, safeRune(r), nil, style)
					lastX = x
				}
			}
			cells += width
		}
		if offset+len(runes) >= start && offset+len(runes) < end {
			x := gutter + cells - v.left
			if x >= gutter && x < w {
				s.SetContent(x, y, ' ', nil, selected)
			}
		}
		offset += len(runes) + 1
	}
	if h >= 2 {
		fill(s, h-2, w, bar)
		put(s, 0, h-2, w, fmt.Sprintf(" %s  %s", e.Mode, b.Path), bar)
	}
	bottom := e.Message
	if e.Mode == editor.Command {
		bottom = ":" + e.Prompt
	}
	if e.Mode == editor.Search {
		bottom = "/" + e.Prompt
	}
	if e.Mode == editor.Picker {
		bottom = "Files > " + e.Prompt
		fill(s, 0, w, bar)
		put(s, 0, 0, w, fmt.Sprintf(" Files (%d) • Enter opens • Esc cancels", len(e.Matches)), accent)
		top := max(0, e.PickIndex-height+1)
		for y := 1; y <= height && y < h; y++ {
			fill(s, y, w, base)
			i := top + y - 1
			if i < len(e.Matches) {
				style := base
				if i == e.PickIndex {
					style = selected
				}
				fill(s, y, w, style)
				put(s, 1, y, w, e.Matches[i], style)
			}
		}
	}
	fill(s, h-1, w, base)
	put(s, 0, h-1, w, bottom, muted)
	if e.Mode == editor.Command || e.Mode == editor.Search || e.Mode == editor.Picker {
		s.ShowCursor(min(w-1, runewidth.StringWidth(bottom)), h-1)
	} else {
		x, y := gutter+displayCol-v.left, row-v.top+1
		if x < w && y < h-2 {
			s.ShowCursor(x, y)
		} else {
			s.HideCursor()
		}
	}
	s.Show()
}

func clipboardCommand(read bool) (string, []string) {
	type candidate struct {
		name      string
		args      []string
		available bool
	}
	candidates := []candidate{
		{"wl-copy", nil, os.Getenv("WAYLAND_DISPLAY") != ""},
		{"xclip", []string{"-selection", "clipboard"}, os.Getenv("DISPLAY") != ""},
		{"xsel", []string{"--clipboard", "--input"}, os.Getenv("DISPLAY") != ""},
		{"pbcopy", nil, true},
	}
	if read {
		candidates = []candidate{{"wl-paste", []string{"--no-newline"}, os.Getenv("WAYLAND_DISPLAY") != ""}, {"xclip", []string{"-selection", "clipboard", "-o"}, os.Getenv("DISPLAY") != ""}, {"xsel", []string{"--clipboard", "--output"}, os.Getenv("DISPLAY") != ""}, {"pbpaste", nil, true}}
	}
	for _, c := range candidates {
		if c.available {
			if path, err := exec.LookPath(c.name); err == nil {
				return path, c.args
			}
		}
	}
	return "", nil
}
func readClipboard() (string, error) {
	name, args := clipboardCommand(true)
	if name == "" {
		return "", fmt.Errorf("clipboard unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, name, args...).Output()
	return string(data), err
}
func writeClipboard(text string) error {
	name, args := clipboardCommand(false)
	if name == "" {
		return fmt.Errorf("clipboard unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
