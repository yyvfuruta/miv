package main

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
	"github.com/yyvfuruta/vim/internal/editor"
)

func TestFileWindowRenderingAndTerminalNavigation(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(80, 24)
	e, _ := editor.New(nil)
	e.Buffer().Path = "one.go"
	e.Buffer().Text = []rune("left content")
	e.SplitWindow(true)
	other := editor.NewBuffer()
	other.Path = "two.go"
	other.Text = []rune("right content")
	e.Buffers = append(e.Buffers, other)
	e.Active = 1
	v := view{screen: s, e: e}
	v.draw()
	cells, _, _ := s.GetContents()
	var text strings.Builder
	for _, cell := range cells {
		text.WriteString(string(cell.Runes))
	}
	for _, marker := range []string{"left content", "right content", "one.go", "two.go"} {
		if !strings.Contains(text.String(), marker) {
			t.Fatalf("missing %q", marker)
		}
	}
	v.focusWindow("h")
	if e.Buffer().Path != "one.go" {
		t.Fatal("left navigation selected wrong file")
	}
	e.SplitWindow(false)
	v.draw()
	v.focusWindow("k")
	if e.Buffer().Path != "one.go" {
		t.Fatal("up navigation failed")
	}
	vt := vt10x.New(vt10x.WithSize(80, 10))
	v.terminal = &terminalPane{vt: vt, done: make(chan struct{}), resize: func(w, h int) error { vt.Resize(w, h); return nil }}
	v.draw()
	v.focusWindow("j")
	v.focusWindow("j")
	if !v.terminal.focused {
		t.Fatal("down navigation did not focus terminal below file windows")
	}
	v.terminal.focused = false
	for _, size := range [][2]int{{1, 1}, {2, 2}, {15, 6}, {100, 30}} {
		s.SetSize(size[0], size[1])
		v.draw()
	}
}
