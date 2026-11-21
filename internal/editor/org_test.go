package editor

import (
	"reflect"
	"strings"
	"testing"
)

func orgFixture(text string) *Editor {
	e := fixture(text)
	e.Buffer().Path = "notes.org"
	return e
}

func TestOrgEditing(t *testing.T) {
	for _, tt := range []struct{ text, keys, want string }{
		{"* café", " mt", "* TODO café"},
		{"* TODO café :work:", " mt", "* DONE café :work:"},
		{"* DONE café", " mt", "* café"},
		{"- [ ] café", " mx", "- [X] café"},
		{"  1. [X] café", " mx", "  1. [ ] café"},
		{"+ [-] café", " mx", "+ [X] café"},
		{"* Parent\nbody\n** Child\n*** Deep\n* Next", " ml", "** Parent\nbody\n*** Child\n**** Deep\n* Next"},
		{"** Parent\n*** Child\n* Next", " mh", "* Parent\n** Child\n* Next"},
	} {
		t.Run(tt.keys+tt.text, func(t *testing.T) {
			e := orgFixture(tt.text)
			keys(e, tt.keys)
			assertText(t, e, tt.want)
			keys(e, "u")
			assertText(t, e, tt.text)
			e.Handle("<C-r>")
			assertText(t, e, tt.want)
		})
	}
	e := orgFixture("* Top")
	keys(e, " mh")
	assertText(t, e, "* Top")
	e = fixture("* Plain")
	keys(e, " mt")
	assertText(t, e, "* Plain")
	e = orgFixture("A link [ ] is not a checkbox")
	keys(e, " mx")
	assertText(t, e, "A link [ ] is not a checkbox")
}

func TestOrgInsertHeading(t *testing.T) {
	for _, text := range []string{"* Parent\n** Child\nbody\n* Next", "* Parent\nbody", "* Parent\nbody\n"} {
		e := orgFixture(text)
		keys(e, " miNew")
		e.Handle("<Esc>")
		want := text + "\n* New"
		if strings.HasSuffix(text, "\n") {
			want = text + "* New\n"
		}
		if strings.Contains(text, "* Next") {
			want = "* Parent\n** Child\nbody\n* New\n* Next"
		}
		assertText(t, e, want)
		keys(e, "u")
		assertText(t, e, text)
	}
}

func TestOrgFoldingAndNavigation(t *testing.T) {
	e := orgFixture("* First\nbody\n** Child\nchild body\n* Last\nlast body")
	b := e.Buffer()
	e.Handle("<Tab>")
	if got := b.OrgVisibleRows(); !reflect.DeepEqual(got, []int{0, 4, 5}) {
		t.Fatal(got)
	}
	keys(e, "j")
	row, _ := b.RowCol()
	if row != 4 {
		t.Fatal(row)
	}
	keys(e, "k")
	row, _ = b.RowCol()
	if row != 0 {
		t.Fatal(row)
	}
	if b.Dirty() {
		t.Fatal("folding changed the document")
	}
	keys(e, " m]")
	row, _ = b.RowCol()
	if row != 2 {
		t.Fatal(row)
	}
	if b.OrgFolded(0) {
		t.Fatal("heading navigation did not reveal destination")
	}
	keys(e, " m[")
	row, _ = b.RowCol()
	if row != 0 {
		t.Fatal(row)
	}
	e.Handle("<S-Tab>")
	if got := b.OrgVisibleRows(); !reflect.DeepEqual(got, []int{0, 4}) {
		t.Fatal(got)
	}
	e.Handle("<S-Tab>")
	if len(b.OrgVisibleRows()) != 6 {
		t.Fatal("global unfold failed")
	}
	e.Handle("<Tab>")
	keys(e, "iX")
	e.Handle("<Esc>")
	if len(b.orgFolds) != 0 {
		t.Fatal("edit retained stale folds")
	}
	keys(e, "u")
	if len(b.orgFolds) != 0 {
		t.Fatal("undo retained stale folds")
	}
}

func TestOrgLiteralBlocksAndFileDetection(t *testing.T) {
	e := orgFixture("#+begin_src text\n* Not a heading\n#+end_src\n* Real\n#+BEGIN_EXAMPLE\n** Also literal\n#+END_EXAMPLE\n** Child")
	h := e.Buffer().OrgHeadings()
	if len(h) != 2 || h[0].Row != 3 || h[1].Row != 7 {
		t.Fatal(h)
	}
	e.Buffer().Path = "NOTES.ORG"
	if !e.Buffer().IsOrg() {
		t.Fatal("uppercase extension")
	}
	e.Buffer().Path = "notes.txt"
	if len(e.Buffer().OrgHeadings()) != 0 {
		t.Fatal("plain file treated as Org")
	}
	e = orgFixture("#+BEGIN_\n* Real")
	if len(e.Buffer().OrgHeadings()) != 1 {
		t.Fatal("malformed directive")
	}
}

func TestOrgFoldAtLastHeadingAndEOF(t *testing.T) {
	for _, text := range []string{"* First\nbody\n* Last", "* First\nbody\n* Last\n"} {
		e := orgFixture(text)
		e.Handle("<Tab>")
		rows := e.Buffer().OrgVisibleRows()
		if len(rows) < 2 || rows[1] != 2 {
			t.Fatalf("last heading hidden: %v", rows)
		}
	}
	e := orgFixture("* First\nbody")
	e.Handle("<Tab>")
	if rows := e.Buffer().OrgVisibleRows(); !reflect.DeepEqual(rows, []int{0}) {
		t.Fatal(rows)
	}
	e.Handle("<Tab>")
	e.Handle("<S-Tab>")
	if !e.Buffer().OrgFolded(0) {
		t.Fatal("global fold after local unfold failed")
	}
}
