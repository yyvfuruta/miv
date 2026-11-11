package editor

import "testing"

func TestLeaderHintsFollowPrefixesAndCancel(t *testing.T) {
	for _, tt := range []struct{ key, prefix, option string }{
		{"", "SPC", "Workspaces"}, {"b", "SPC b", "Switch buffer"}, {"w", "SPC w", "Split right"},
		{"f", "SPC f", "Find file"}, {"p", "SPC p", "Switch project"}, {"<Tab>", "SPC TAB", "Rename workspace"},
	} {
		e := fixture("unchanged")
		keys(e, " ")
		if tt.key != "" {
			e.Handle(tt.key)
		}
		prefix, hints := e.KeyHints()
		found := false
		for _, hint := range hints {
			found = found || hint.Description == tt.option
		}
		if prefix != tt.prefix || !found || e.Mode != Normal {
			t.Fatalf("hints for %q: %q %+v", tt.key, prefix, hints)
		}
		e.Handle("<Esc>")
		if _, hints := e.KeyHints(); len(hints) != 0 {
			t.Fatal("Escape did not hide prefix hints")
		}
		assertText(t, e, "unchanged")
	}
	e := fixture("text")
	keys(e, " bz")
	if _, hints := e.KeyHints(); len(hints) != 0 {
		t.Fatal("invalid sequence kept its hints")
	}
	keys(e, " b")
	if e.Active != 0 || len(e.Buffers) != 1 {
		t.Fatal("buffer prefix changed buffers before command completion")
	}
}
