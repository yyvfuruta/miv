package main

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/yyvfuruta/vim/internal/editor"
)

func TestDrawModesAndResize(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	e, _ := editor.New(nil)
	e.Handle("i")
	e.InsertPaste("ação\t世界\ne\u0301\x1b[31m")
	e.Handle("<Esc>")
	v := view{screen: screen, e: e}
	for _, size := range [][2]int{{80, 24}, {15, 5}, {1, 1}, {2, 2}, {120, 40}} {
		screen.SetSize(size[0], size[1])
		for _, mode := range []editor.Mode{editor.Normal, editor.Insert, editor.Visual, editor.VisualLine, editor.Command, editor.Search, editor.Picker} {
			e.Mode = mode
			e.Prompt = "texto"
			e.Matches = []string{"a.go", "pasta/b.go"}
			v.draw()
		}
	}
	e.Mode = editor.Normal
	screen.SetSize(80, 24)
	v.draw()
	cells, _, _ := screen.GetContents()
	var text strings.Builder
	for _, c := range cells {
		text.WriteString(string(c.Runes))
	}
	if !strings.Contains(text.String(), "NORMAL") {
		t.Fatal("status line missing")
	}
	if strings.ContainsRune(text.String(), '\x1b') {
		t.Fatal("file control byte reached terminal")
	}
}
func TestTerminalKeyMapping(t *testing.T) {
	for _, test := range []struct {
		key  tcell.Key
		want string
	}{{tcell.KeyEscape, "<Esc>"}, {tcell.KeyBackspace2, "<Backspace>"}, {tcell.KeyCtrlR, "<C-r>"}, {tcell.KeyCtrlP, "<C-p>"}, {tcell.KeyLeft, "<Left>"}} {
		if got := keyName(tcell.NewEventKey(test.key, 0, tcell.ModNone)); got != test.want {
			t.Fatalf("key = %q, want %q", got, test.want)
		}
	}
	if keyName(tcell.NewEventKey(tcell.KeyRune, 'ç', tcell.ModNone)) != "ç" {
		t.Fatal("unicode key lost")
	}
}
