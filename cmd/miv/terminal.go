package main

import (
	"fmt"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
)

// terminalPane owns a shell's virtual screen. Platform-specific PTY lifecycle
// is behind callbacks so drawing and input encoding remain independently testable.
type terminalPane struct {
	vt                    vt10x.Terminal
	focused, escapePrefix bool
	directory             string
	write                 func([]byte) (int, error)
	resize                func(int, int) error
	close                 func()
	done                  <-chan struct{}
	bracketedPaste        atomic.Bool
	exitMessage           string
}

func terminalGeometry(w, h int) (editorRows, header, rows int) {
	if h < 6 {
		return max(1, h-3), 0, max(1, h-3)
	}
	rows = max(1, (h-4)/2)
	editorRows = h - 4 - rows
	return editorRows, editorRows + 1, rows
}

func (v *view) closeTerminal() error {
	if v.terminal == nil {
		return fmt.Errorf("no terminal is open")
	}
	v.terminal.close()
	v.terminal = nil
	return nil
}

func terminalColor(c vt10x.Color) tcell.Color {
	if c == vt10x.DefaultFG || c == vt10x.DefaultBG || c == vt10x.DefaultCursor {
		return tcell.ColorDefault
	}
	if c < 256 {
		return tcell.PaletteColor(int(c))
	}
	return tcell.NewHexColor(int32(c))
}

func (v *view) drawTerminal(w, h int) {
	t := v.terminal
	_, header, rows := terminalGeometry(w, h)
	if oldW, oldH := t.vt.Size(); oldW != max(1, w) || oldH != rows {
		_ = t.resize(max(1, w), rows)
	}
	y0 := header + 1
	if h < 6 {
		y0 = 1
	} else {
		fill(v.screen, header, w, bar)
		focus := "EDITOR"
		if t.focused {
			focus = "TERMINAL"
		}
		state := ""
		select {
		case <-t.done:
			state = " [" + t.exitMessage + "]"
		default:
		}
		put(v.screen, 0, header, w, fmt.Sprintf(" %s%s • %s • Ctrl-\\ Ctrl-n: editor", focus, state, t.directory), accent)
	}
	t.vt.Lock()
	defer t.vt.Unlock()
	cols, lines := t.vt.Size()
	for y := 0; y < min(rows, lines) && y0+y < h-2; y++ {
		for x := 0; x < min(w, cols); x++ {
			cell := t.vt.Cell(x, y)
			r := cell.Char
			if r == 0 {
				r = ' '
			}
			style := base.Foreground(terminalColor(cell.FG)).Background(terminalColor(cell.BG))
			// vt10x's glyph attribute bits: reverse, underline, bold, graphics, italic, blink.
			style = style.Reverse(cell.Mode&1 != 0).Underline(cell.Mode&2 != 0).Bold(cell.Mode&4 != 0).Italic(cell.Mode&16 != 0).Blink(cell.Mode&32 != 0)
			v.screen.SetContent(x, y0+y, safeRune(r), nil, style)
		}
	}
	if t.focused {
		cursor := t.vt.Cursor()
		if t.vt.CursorVisible() && cursor.X < w && y0+cursor.Y < h-2 {
			v.screen.ShowCursor(cursor.X, y0+cursor.Y)
		} else {
			v.screen.HideCursor()
		}
	}
}

func (t *terminalPane) key(ev *tcell.EventKey) error {
	if t.escapePrefix {
		t.escapePrefix = false
		if ev.Key() == tcell.KeyCtrlN {
			t.focused = false
			return nil
		}
		if _, err := t.write([]byte{0x1c}); err != nil {
			return err
		}
	}
	if ev.Key() == tcell.KeyCtrlBackslash {
		t.escapePrefix = true
		return nil
	}
	data := terminalKey(ev, t.vt)
	if data == "" {
		return nil
	}
	_, err := t.write([]byte(data))
	return err
}

func terminalKey(ev *tcell.EventKey, vt vt10x.Terminal) string {
	key := ev.Key()
	if key == tcell.KeyRune {
		text := string(ev.Rune())
		if ev.Modifiers()&tcell.ModAlt != 0 {
			return "\x1b" + text
		}
		return text
	}
	if key >= tcell.KeyCtrlSpace && key <= tcell.KeyCtrlUnderscore {
		return string(rune(key))
	}
	vt.Lock()
	appCursor := vt.Mode()&vt10x.ModeAppCursor != 0
	vt.Unlock()
	prefix := "\x1b["
	if appCursor {
		prefix = "\x1bO"
	}
	keys := map[tcell.Key]string{
		tcell.KeyEscape: "\x1b", tcell.KeyEnter: "\r", tcell.KeyBackspace2: "\x7f", tcell.KeyTab: "\t", tcell.KeyBacktab: "\x1b[Z",
		tcell.KeyUp: prefix + "A", tcell.KeyDown: prefix + "B", tcell.KeyRight: prefix + "C", tcell.KeyLeft: prefix + "D",
		tcell.KeyHome: "\x1b[H", tcell.KeyEnd: "\x1b[F", tcell.KeyInsert: "\x1b[2~", tcell.KeyDelete: "\x1b[3~", tcell.KeyPgUp: "\x1b[5~", tcell.KeyPgDn: "\x1b[6~",
		tcell.KeyF1: "\x1bOP", tcell.KeyF2: "\x1bOQ", tcell.KeyF3: "\x1bOR", tcell.KeyF4: "\x1bOS", tcell.KeyF5: "\x1b[15~", tcell.KeyF6: "\x1b[17~", tcell.KeyF7: "\x1b[18~", tcell.KeyF8: "\x1b[19~", tcell.KeyF9: "\x1b[20~", tcell.KeyF10: "\x1b[21~", tcell.KeyF11: "\x1b[23~", tcell.KeyF12: "\x1b[24~",
	}
	return keys[key]
}

// Keep split UTF-8 code points between PTY reads; vt10x.Write consumes only
// complete runes. Escape parser state remains inside the emulator.
func feedTerminal(vt vt10x.Terminal, pending, chunk []byte) []byte {
	pending = append(pending, chunk...)
	complete := 0
	for complete < len(pending) && utf8.FullRune(pending[complete:]) {
		_, size := utf8.DecodeRune(pending[complete:])
		complete += size
	}
	if complete > 0 {
		_, _ = vt.Write(pending[:complete])
		pending = append([]byte(nil), pending[complete:]...)
	}
	return pending
}

func (t *terminalPane) paste(text string) error {
	// Use bracketed paste only when the child enabled it; plain shells receive
	// the same text as they would in a normal terminal.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if t.bracketedPaste.Load() {
		text = "\x1b[200~" + text + "\x1b[201~"
	}
	_, err := t.write([]byte(text))
	return err
}
