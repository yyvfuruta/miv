package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/yyvfuruta/vim/internal/editor"
	"github.com/yyvfuruta/vim/internal/lsp"
)

func TestLanguageResultsRejectStaleState(t *testing.T) {
	for _, change := range []string{"none", "edit", "cursor", "workspace", "mode", "cancel"} {
		t.Run(change, func(t *testing.T) {
			e, _ := editor.New(nil)
			e.Buffer().Path = filepath.Join(t.TempDir(), "main.go")
			e.Buffer().Text = []rune("text")
			c := newLanguageController(e, func() {})
			defer c.close()
			c.id, c.busy = 1, true
			reply := languageReply{id: 1, query: e.LanguageQuery("hover"), buffer: e.Buffer(), workspace: e.Workspace, result: lsp.Result{Text: "Documentation"}}
			switch change {
			case "edit":
				e.Buffer().Text = []rune("changed")
			case "cursor":
				e.Buffer().Cursor = 1
			case "workspace":
				e.SwitchWorkspace(2)
			case "mode":
				e.Mode = editor.Insert
			case "cancel":
				c.cancelRequest()
			}
			c.results <- reply
			c.apply()
			if (e.Mode == editor.Info) != (change == "none") {
				t.Fatal("stale language result applied or valid result discarded")
			}
		})
	}
}

func TestDocumentationRendering(t *testing.T) {
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	defer s.Fini()
	e, _ := editor.New(nil)
	e.ShowLanguageResult("hover", lsp.Result{Text: "func Example(value int)\n\nDocumentation\n" + strings.Repeat("wide 界 words ", 50) + "\x1b[31m"})
	v := view{screen: s, e: e}
	for _, size := range [][2]int{{80, 24}, {10, 5}, {1, 1}, {2, 2}} {
		s.SetSize(size[0], size[1])
		v.draw()
		e.Handle("G")
		v.draw()
		e.Handle("g")
	}
	s.SetSize(80, 24)
	v.draw()
	content, _, _ := s.GetContents()
	var text strings.Builder
	for _, c := range content {
		text.WriteString(string(c.Runes))
	}
	if !strings.Contains(text.String(), "func Example(value int)") || strings.ContainsRune(text.String(), '\x1b') {
		t.Fatal("documentation was clipped or leaked control characters")
	}
	if keyName(tcell.NewEventKey(tcell.KeyCtrlO, 0, tcell.ModNone)) != "<C-o>" {
		t.Fatal("jump back key mapping missing")
	}
}
