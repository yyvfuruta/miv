package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/yyvfuruta/vim/internal/editor"
)

func foregroundAt(c *syntaxCache, offset int) tcell.Color {
	index := c.firstSpan(offset)
	fg, _, _ := c.styleAt(offset, &index).Decompose()
	return fg
}

func TestSyntaxLanguages(t *testing.T) {
	for _, tt := range []struct {
		path, text, needle string
		color              int
	}{
		{"main.go", "package main\nconst msg = \"ação\"\n", "const", 5},
		{"main.go", "package main\nconst msg = \"ação\"\n", "ação", 2},
		{"main.go", "/* primeira\nsegunda */\nvar x = 42", "segunda", 8},
		{"main.rs", "fn main() { let x = 42; }", "fn", 5},
		{"main.py", "def hello():\n    return 42", "def", 5},
		{"main.js", "const x = 42;", "const", 5},
		{"main.ts", "const x: number = 42;", "42", 3},
		{"run.sh", "echo \"ação\"", "ação", 2},
		{"config.json", "{\"count\": 42}", "42", 3},
		{"config.jsonc", "// comentário\n{\"count\": 42}", "comentário", 8},
		{"config.yaml", "# comentário\nvalue: 42", "comentário", 8},
		{"main.lua", "-- comentário\nreturn 42", "comentário", 8},
		{"main.c", "/* comentário */\nint main() { return 42; }", "return", 5},
		{"Makefile", "# comentário\nbuild:\n\techo ok", "comentário", 8},
	} {
		t.Run(tt.path+"/"+tt.needle, func(t *testing.T) {
			var c syntaxCache
			c.update(tt.path, tt.text)
			offset := utf8.RuneCountInString(tt.text[:strings.Index(tt.text, tt.needle)])
			if got := foregroundAt(&c, offset); got != tcell.PaletteColor(tt.color) {
				t.Fatalf("color = %v, want palette %d", got, tt.color)
			}
		})
	}
}

func TestSyntaxCacheAndFallback(t *testing.T) {
	var c syntaxCache
	text := "/* ação */\r\nconst x = 42"
	c.update("main.go", text)
	offset := utf8.RuneCountInString(text[:strings.Index(text, "const")])
	if foregroundAt(&c, offset) != tcell.PaletteColor(5) {
		t.Fatal("CRLF/Unicode offsets shifted")
	}
	spans := &c.spans[0]
	c.update("main.go", text)
	if spans != &c.spans[0] {
		t.Fatal("unchanged document was retokenized")
	}
	c.update("main.go", "\"hello\"")
	if foregroundAt(&c, 1) != tcell.PaletteColor(2) {
		t.Fatal("edited text kept stale colors")
	}
	for _, path := range []string{"plain.txt", "", "unknown.extension-does-not-exist"} {
		c.update(path, text)
		if len(c.spans) != 0 {
			t.Fatalf("unexpected highlighting for %q", path)
		}
	}
	c.update("main.go", strings.Repeat("x", maxHighlightBytes+1))
	if len(c.spans) != 0 {
		t.Fatal("large file highlighting should be disabled")
	}
	c.update("main.go", "const x = 1")
	if foregroundAt(&c, 0) != tcell.PaletteColor(5) {
		t.Fatal("highlighting did not return after path/size change")
	}
}

func TestDrawSyntaxAndSelection(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	e, _ := editor.New(nil)
	b := e.Buffer()
	b.Path = "main.go"
	b.Text = []rune("const msg = \"ação\"\n/* comment\nstill comment */")
	v := view{screen: screen, e: e}
	v.draw()
	_, _, style, _ := screen.GetContent(3, 1)
	fg, bg, _ := style.Decompose()
	if fg != tcell.PaletteColor(5) || bg != tcell.ColorDefault {
		t.Fatal("keyword color or terminal background lost")
	}
	e.Mode = editor.Visual
	e.Anchor = 0
	v.draw()
	_, _, style, _ = screen.GetContent(3, 1)
	if style != selected {
		t.Fatal("syntax overrode visual selection")
	}
	e.Mode = editor.Normal
	e.Handle("i")
	e.Handle("x")
	e.Handle("<Esc>")
	v.draw()
	_, _, style, _ = screen.GetContent(3, 1)
	fg, _, _ = style.Decompose()
	if fg != tcell.ColorDefault {
		t.Fatal("edited identifier retained keyword color")
	}
}
