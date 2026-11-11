package editor

import (
	"strings"
	"testing"
)

func TestVisualFormat(t *testing.T) {
	long := strings.Repeat("word ", 19) + "last"
	wrapped := strings.TrimSpace(strings.Repeat("word ", 16)) + "\n" + strings.Repeat("word ", 3) + "last"
	for _, command := range []string{"Vgq", "vgq", "Vgw", "vgw"} {
		t.Run(command, func(t *testing.T) {
			e := fixture("before\n" + long + "\nafter\n")
			keys(e, "j"+command)
			assertText(t, e, "before\n"+wrapped+"\nafter\n")
			if e.Mode != Normal || e.pending != "" {
				t.Fatal("format did not finish in normal mode")
			}
			wantCursor := len([]rune("before\n"))
			if strings.HasSuffix(command, "q") {
				wantCursor += len([]rune(strings.TrimSpace(strings.Repeat("word ", 16)))) + 1
			}
			if e.Buffer().Cursor != wantCursor {
				t.Fatalf("cursor = %d, want %d", e.Buffer().Cursor, wantCursor)
			}
			keys(e, "u")
			assertText(t, e, "before\n"+long+"\nafter\n")
			e.Handle("<C-r>")
			assertText(t, e, "before\n"+wrapped+"\nafter\n")
		})
	}
}

func TestFormatParagraphs(t *testing.T) {
	for _, tt := range []struct{ name, text, want string }{
		{"join short lines", "one\ntwo\nthree", "one two three"},
		{"paragraphs", "  one\n  two\n\n\tthree\n\tfour\n", "  one two\n\n\tthree four\n"},
		{"whitespace", "one  two\tthree ", "one two three"},
		{"blank", "\n  \n", "\n  \n"},
		{"empty", "", ""},
		{"long word", strings.Repeat("x", 90) + " end", strings.Repeat("x", 90) + "\nend"},
		{"wide characters", strings.Repeat("界 ", 27) + "end", strings.TrimSpace(strings.Repeat("界 ", 27)) + "\nend"},
		{"tabs", "\t" + strings.Repeat("word ", 15) + "end", "\t" + strings.TrimSpace(strings.Repeat("word ", 14)) + "\n\tword end"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := fixture(tt.text)
			keys(e, "VGgq")
			assertText(t, e, tt.want)
		})
	}
}

func TestFormatReverseSelectionAndCursor(t *testing.T) {
	e := fixture("before\none\ntwo\nafter")
	keys(e, "2jVkgq")
	assertText(t, e, "before\none two\nafter")
	e = fixture("one\ntwo\nthree")
	keys(e, "vGgw")
	assertText(t, e, "one two three")
	if e.Buffer().Cursor != 8 {
		t.Fatalf("gw cursor = %d, want 8", e.Buffer().Cursor)
	}
	e = fixture("one\ntwo")
	keys(e, "vGg")
	e.Handle("<Esc>")
	keys(e, "q")
	assertText(t, e, "one\ntwo")
}
