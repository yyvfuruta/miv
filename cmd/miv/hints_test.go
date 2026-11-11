package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/yyvfuruta/vim/internal/editor"
)

func renderedText(s tcell.SimulationScreen) string {
	cells, _, _ := s.GetContents()
	var text strings.Builder
	for _, cell := range cells {
		text.WriteString(string(cell.Runes))
	}
	return text.String()
}

func TestWorkspaceBarAndPrefixHints(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	s.SetSize(100, 28)
	e, _ := editor.New(nil)
	e.Buffer().Path = filepath.Join(t.TempDir(), "first.txt")
	e.Execute("e second.txt")
	e.CreateWorkspace(2)
	e.Execute("wsrename code")
	e.SwitchWorkspace(1)
	v := view{screen: s, e: e}
	v.draw()
	cells, _, _ := s.GetContents()
	var top strings.Builder
	for _, cell := range cells[:100] {
		top.WriteString(string(cell.Runes))
	}
	if !strings.Contains(top.String(), "[W1]") || !strings.Contains(top.String(), "W2:code") || strings.Contains(top.String(), " 1 ") || strings.Contains(top.String(), " 2 ") {
		t.Fatalf("top bar still showed file tabs or lost workspaces: %q", top.String())
	}
	e.Handle(" ")
	v.draw()
	if text := renderedText(s); !strings.Contains(text, "Workspaces") || !strings.Contains(text, "Buffers") {
		t.Fatal("leader menu missing")
	}
	e.Handle("w")
	v.draw()
	if text := renderedText(s); !strings.Contains(text, "SPC w") || !strings.Contains(text, "Split right") || strings.Contains(text, "Workspaces") {
		t.Fatal("prefix menu did not update")
	}
	for _, size := range [][2]int{{1, 1}, {2, 2}, {8, 4}, {20, 6}, {120, 40}} {
		s.SetSize(size[0], size[1])
		v.draw()
	}
	e.Handle("<Esc>")
	s.SetSize(100, 28)
	v.draw()
	if strings.Contains(renderedText(s), "Esc cancels") {
		t.Fatal("canceled prefix menu stayed visible")
	}
}
