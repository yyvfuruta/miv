package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yyvfuruta/vim/internal/lsp"
)

func TestLanguageBindingsAndDocumentation(t *testing.T) {
	for _, tt := range []struct{ keys, action string }{
		{"gd", "definition"}, {"K", "hover"}, {"gi", "implementation"},
		{"gr", "references"}, {"gy", "typeDefinition"}, {"gs", "signatureHelp"},
	} {
		e := fixture("function()")
		e.Buffer().Path = filepath.Join(t.TempDir(), "main.go")
		var got string
		e.LanguageAction = func(action string) { got = action }
		keys(e, tt.keys)
		if got != tt.action || e.pending != "" {
			t.Fatalf("%s requested %q, want %q", tt.keys, got, tt.action)
		}
	}
	e := fixture("original text")
	keys(e, "K")
	if !strings.Contains(e.Message, "unavailable") {
		t.Fatal("unavailable language services were not reported")
	}
	e.ShowLanguageResult("hover", lsp.Result{Text: "func F()\n\nDocumentation"})
	if e.Mode != Info || e.InfoTitle != "Documentation" {
		t.Fatal("documentation was not opened")
	}
	keys(e, "jjki")
	assertText(t, e, "original text")
	if e.InfoScroll != 1 {
		t.Fatal("documentation did not scroll")
	}
	e.Handle("<Esc>")
	if e.Mode != Normal || e.InfoText != "" {
		t.Fatal("documentation did not close")
	}
	keys(e, "u")
	assertText(t, e, "original text")
	called := false
	e.LanguageAction = func(string) { called = true }
	keys(e, "K")
	if called || !strings.Contains(e.Message, "Save the file") {
		t.Fatal("unnamed file was not rejected")
	}
}

func TestLanguageNavigationAndBack(t *testing.T) {
	dir := t.TempDir()
	source, target := filepath.Join(dir, "source.go"), filepath.Join(dir, "target.go")
	if err := os.WriteFile(target, []byte("package p\n// 😀func\n"), 0600); err != nil {
		t.Fatal(err)
	}
	e := fixture("unsaved source")
	e.Buffer().Path, e.Buffer().Cursor = source, 3
	origin := e.Buffer()
	e.SplitWindow(true)
	e.ShowLanguageResult("definition", lsp.Result{Locations: []lsp.Location{{Path: target, Position: lsp.Position{Line: 1, Character: 5}}}})
	if e.Buffer().Path != target || e.Buffer().Cursor != 14 {
		t.Fatalf("wrong navigation: %s at %d", e.Buffer().Path, e.Buffer().Cursor)
	}
	e.Handle("<C-o>")
	if e.Buffer() != origin || e.Buffer().Cursor != 3 || len(e.jumps) != 0 {
		t.Fatal("jump back did not restore the source buffer and position")
	}
	assertText(t, e, "unsaved source")
	// An existing target buffer keeps unsaved content rather than reloading disk.
	keys(e, "gt")
	e.Buffer().Text = []rune("modified target")
	keys(e, "gt")
	e.ShowLanguageResult("definition", lsp.Result{Locations: []lsp.Location{{Path: target}}})
	assertText(t, e, "modified target")
	e.Handle("<C-o>")
	e.ShowLanguageResult("definition", lsp.Result{Locations: []lsp.Location{{Path: dir}}})
	if e.Buffer() != origin || len(e.jumps) != 0 {
		t.Fatal("failed navigation changed buffers or jump history")
	}
}

func TestReferencePicker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "name with spaces.go")
	e := fixture("first\nsecond\nthird")
	e.Root, e.Buffer().Path = dir, path
	locations := []lsp.Location{{Path: path, Position: lsp.Position{Line: 1}}, {Path: path, Position: lsp.Position{Line: 2}}}
	e.ShowLanguageResult("references", lsp.Result{Locations: locations})
	if e.Mode != Picker || !e.PickingLocations || len(e.Matches) != 2 {
		t.Fatal("references did not open the location picker")
	}
	keys(e, ":3:")
	if len(e.Matches) != 1 {
		t.Fatal("reference filtering failed")
	}
	e.Handle("<Enter>")
	if e.Mode != Normal || e.PickingLocations || e.Buffer().Cursor != 13 {
		t.Fatalf("reference picker did not jump: mode %s, cursor %d", e.Mode, e.Buffer().Cursor)
	}
	e.Handle("<C-o>")
	if e.Buffer().Cursor != 0 {
		t.Fatal("reference jump did not preserve origin")
	}
	e.ShowLanguageResult("references", lsp.Result{Locations: locations})
	e.Handle("<Esc>")
	if e.PickingLocations || e.locationEntries != nil || e.Buffer().Cursor != 0 {
		t.Fatal("reference picker cancel changed the cursor or kept stale locations")
	}
}
