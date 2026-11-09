package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
	"github.com/yyvfuruta/vim/internal/editor"
)

func TestTerminalKeyEncodingAndFocus(t *testing.T) {
	vt := vt10x.New(vt10x.WithSize(80, 10))
	for _, tt := range []struct {
		key  tcell.Key
		want string
	}{{tcell.KeyCtrlC, "\x03"}, {tcell.KeyCtrlD, "\x04"}, {tcell.KeyEnter, "\r"}, {tcell.KeyEscape, "\x1b"}, {tcell.KeyUp, "\x1b[A"}, {tcell.KeyBackspace2, "\x7f"}} {
		if got := terminalKey(tcell.NewEventKey(tt.key, 0, tcell.ModNone), vt); got != tt.want {
			t.Fatalf("%v=%q, want=%q", tt.key, got, tt.want)
		}
	}
	var input bytes.Buffer
	pane := &terminalPane{vt: vt, focused: true, write: input.Write}
	_ = pane.key(tcell.NewEventKey(tcell.KeyCtrlBackslash, 0, tcell.ModNone))
	_ = pane.key(tcell.NewEventKey(tcell.KeyCtrlN, 0, tcell.ModNone))
	if pane.focused || input.Len() != 0 {
		t.Fatal("terminal escape was sent to the shell")
	}
	_ = pane.paste("echo hi\n")
	if input.String() != "echo hi\n" {
		t.Fatal("paste used unsupported bracket markers")
	}
	input.Reset()
	pane.bracketedPaste.Store(true)
	_ = pane.paste("echo hi\n")
	if input.String() != "\x1b[200~echo hi\n\x1b[201~" {
		t.Fatal("bracketed paste not forwarded")
	}
}

func TestTerminalSplitUTF8AndRendering(t *testing.T) {
	vt := vt10x.New(vt10x.WithSize(80, 10))
	pending := feedTerminal(vt, nil, []byte{'c', 'a', 'f', 0xc3})
	if len(pending) != 1 {
		t.Fatal("partial UTF-8 was dropped")
	}
	pending = feedTerminal(vt, pending, []byte{0xa9})
	if len(pending) != 0 || !strings.Contains(vt.String(), "café") {
		t.Fatal("UTF-8 output corrupted")
	}
	_, _ = vt.Write([]byte("\r\n\x1b[31mred\x1b[0m"))
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	e, _ := editor.New(nil)
	e.Buffer().Text = []rune("editor text")
	pane := &terminalPane{vt: vt, focused: true, done: make(chan struct{}), directory: "project", resize: func(w, h int) error { vt.Resize(w, h); return nil }}
	v := view{screen: screen, e: e, terminal: pane}
	v.draw()
	r, _, _, _ := screen.GetContent(3, 1)
	if r != 'e' {
		t.Fatal("terminal hid the editor")
	}
	_, header, _ := terminalGeometry(80, 24)
	r, _, _, _ = screen.GetContent(0, header+1)
	if r != 'c' {
		t.Fatal("terminal text not rendered")
	}
	_, _, style, _ := screen.GetContent(0, header+2)
	fg, _, _ := style.Decompose()
	if fg != tcell.PaletteColor(1) {
		t.Fatal("ANSI color lost")
	}
	for _, size := range [][2]int{{1, 1}, {2, 2}, {15, 6}, {100, 30}} {
		screen.SetSize(size[0], size[1])
		v.draw()
	}
}
