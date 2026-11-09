package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(text string) *Editor {
	e, _ := New(nil)
	e.Buffer().Text = []rune(text)
	e.Buffer().saved = text
	return e
}
func keys(e *Editor, s string) {
	for _, r := range s {
		e.Handle(string(r))
	}
}
func assertText(t *testing.T, e *Editor, want string) {
	t.Helper()
	if got := string(e.Buffer().Text); got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
}
func TestEditing(t *testing.T) {
	tests := []struct {
		name, text, keys, want string
		escape                 bool
	}{
		{"delete word", "one two three", "dw", "two three", false},
		{"delete count", "one two three four", "2dw", "three four", false},
		{"operator count", "one two three four", "d2w", "three four", false},
		{"composed count", "1 2 3 4 5 6 7", "2d2w", "5 6 7", false},
		{"change word", "one two", "cwnew", "new two", true},
		{"inner word", "hello world", "lciwHello", "Hello world", true},
		{"inner quotes", `x = "old text"`, `f"ci"new`, `x = "new"`, true},
		{"inner nested brackets", "(a (b) c)", "ci(new", "(new)", true},
		{"delete lines", "a\nb\nc\n", "2dd", "c\n", false},
		{"delete last line", "a\nb", "Gdd", "a", false},
		{"yank paste", "first\nsecond", "yyp", "first\nfirst\nsecond", false},
		{"Y to end", "hello world", "wYp", "hello wworldorld", false},
		{"visual delete", "abcdef", "vlld", "def", false},
		{"visual line delete", "a\nb\nc", "Vjd", "c", false},
		{"delete to end", "one two", "wD", "one ", false},
		{"append", "one", "A two", "one two", true},
		{"join", "one\n  two", "J", "one two", false},
		{"unicode delete", "café", "lx", "cfé", false},
		{"indent", "one\ntwo", ">>", "    one\ntwo", false},
		{"repeat delete", "abcdef", "x.", "cdef", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := fixture(tt.text)
			keys(e, tt.keys)
			if tt.escape {
				e.Handle("<Esc>")
			}
			assertText(t, e, tt.want)
		})
	}
}
func TestInsertUndoRedoRepeat(t *testing.T) {
	e := fixture("world")
	keys(e, "ihello ")
	e.Handle("<Esc>")
	assertText(t, e, "hello world")
	keys(e, "u")
	assertText(t, e, "world")
	e.Handle("<C-r>")
	assertText(t, e, "hello world")
	keys(e, "A!")
	e.Handle("<Esc>")
	keys(e, ".")
	assertText(t, e, "hello world!!")
	keys(e, "u")
	assertText(t, e, "hello world!")
}
func TestMovementAndSearch(t *testing.T) {
	e := fixture("abcdef\nx\nabcdef\n")
	keys(e, "5ljj")
	row, col := e.Buffer().RowCol()
	if row != 2 || col != 5 {
		t.Fatalf("position = %d:%d", row, col)
	}
	keys(e, "gg/abcdef")
	e.Handle("<Enter>")
	row, _ = e.Buffer().RowCol()
	if row != 2 {
		t.Fatal("search did not advance")
	}
	keys(e, "n")
	if e.Buffer().Cursor != 0 {
		t.Fatal("search did not wrap")
	}
	keys(e, "/missing")
	e.Handle("<Esc>")
	if e.Buffer().Cursor != 0 {
		t.Fatal("cancel changed cursor")
	}
}
func TestModesAndCommands(t *testing.T) {
	e := fixture("abc")
	e.Execute("q")
	if !e.Quit {
		t.Fatal("clean buffer should close")
	}
	e = fixture("abc")
	keys(e, "x")
	e.Execute("q")
	if e.Quit {
		t.Fatal("dirty buffer closed")
	}
	e.Execute("q!")
	if !e.Quit {
		t.Fatal("forced quit failed")
	}
	e = fixture("abc")
	keys(e, "iX")
	e.Handle("<Esc>")
	keys(e, "u")
	if e.Buffer().Dirty() {
		t.Fatal("undo to original should be clean")
	}
	e = fixture("a")
	e.Buffers = append(e.Buffers, NewBuffer())
	e.Execute("bn")
	if e.Active != 1 {
		t.Fatal("buffer switch")
	}
	e.Execute("q")
	if e.Quit || len(e.Buffers) != 1 {
		t.Fatal("close buffer")
	}
}
func TestSavePreservesFormatAndDetectsConflict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	original := []byte("café\r\nsecond")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b.Text); got != "café\nsecond" {
		t.Fatalf("text = %q", got)
	}
	b.Replace(0, 0, "X")
	if err = b.Save("", false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "Xcafé\r\nsecond" {
		t.Fatalf("saved = %q", data)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("permissions changed")
	}
	os.WriteFile(path, []byte("external"), 0600)
	if err = b.Save("", false); err == nil {
		t.Fatal("external modification overwritten")
	}
	data, _ = os.ReadFile(path)
	if string(data) != "external" {
		t.Fatal("conflict modified file")
	}
	if err = b.Save("", true); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, ".miv-*"))
	if len(files) > 0 {
		t.Fatal("temporary files leaked")
	}
}
func TestFileSafety(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new")
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	b.Replace(0, 0, "new")
	os.WriteFile(path, []byte("other"), 0644)
	if b.Save("", false) == nil {
		t.Fatal("new file collision overwritten")
	}
	if _, err = Open(dir); err == nil {
		t.Fatal("directory accepted")
	}
	for _, data := range [][]byte{{0xff}, {0}} {
		os.WriteFile(path, data, 0644)
		if _, err = Open(path); err == nil {
			t.Fatal("binary accepted")
		}
	}
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	os.WriteFile(target, []byte("x\n"), 0644)
	if err = os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	b, err = Open(link)
	if err != nil {
		t.Fatal(err)
	}
	b.Replace(0, 1, "y")
	if err = b.Save("", false); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(link)
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "y\n" {
		t.Fatal("symlink target not saved")
	}
}
func TestPicker(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	for _, p := range []string{".git/config", "src/main.go", "README.md", ".hidden"} {
		os.WriteFile(filepath.Join(dir, p), []byte("x"), 0644)
	}
	e := fixture("")
	e.Root = dir
	e.StartPicker()
	if len(e.Files) != 3 {
		t.Fatalf("files = %v", e.Files)
	}
	keys(e, "smg")
	if len(e.Matches) != 1 || e.Matches[0] != "src/main.go" {
		t.Fatalf("matches = %v", e.Matches)
	}
	e.Handle("<Enter>")
	if !strings.HasSuffix(e.Buffer().Path, "src/main.go") {
		t.Fatal("picker did not open selected file")
	}
}
func TestEmptyBufferAndPaste(t *testing.T) {
	e := fixture("")
	for _, s := range []string{"dd", "dw", "d$", "b", "e", "%", "p", "u", "n"} {
		keys(e, s)
		if e.Buffer().Cursor != 0 {
			t.Fatal("invalid empty cursor")
		}
	}
	keys(e, "i")
	e.InsertPaste("hello\r\nworld")
	e.Handle("<Esc>")
	assertText(t, e, "hello\nworld")
	keys(e, "u")
	assertText(t, e, "")
}
func FuzzKeys(f *testing.F) {
	for _, s := range []string{"ihello\x1bu", "dggdG", "ciw", "Vjd", "(a (b))"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 500 {
			return
		}
		e := fixture("first (nested [text])\nsecond\n")
		// Keep fuzzing inside the pure editing state machine; commands/picker access the filesystem.
		for _, r := range s {
			if strings.ContainsRune(":/? \x10", r) {
				continue
			}
			key := string(r)
			if r == 27 {
				key = "<Esc>"
			}
			e.Handle(key)
			b := e.Buffer()
			if b.Cursor < 0 || b.Cursor > len(b.Text) {
				t.Fatalf("cursor %d out of bounds %d", b.Cursor, len(b.Text))
			}
		}
	})
}

func TestRepeatSurvivesUndoAndMultipleDots(t *testing.T) {
	e := fixture("abcdef")
	keys(e, "x..")
	assertText(t, e, "def")
	keys(e, "u.")
	assertText(t, e, "def")
	e = fixture("x")
	keys(e, "i")
	e.InsertPaste("a\nb")
	e.Handle("<Esc>")
	keys(e, ".")
	assertText(t, e, "a\na\nbbx")
}
func TestChangeLines(t *testing.T) {
	for _, tt := range []struct{ text, keys, want string }{{"one\ntwo", "Gccnew", "one\nnew"}, {"one\ntwo\nthree", "jccnew", "one\nnew\nthree"}, {"one\ntwo\n", "2ccnew", "new\n"}, {"one\n", "Gccnew", "one\nnew"}, {"one", "ccnew", "new"}} {
		e := fixture(tt.text)
		keys(e, tt.keys)
		e.Handle("<Esc>")
		assertText(t, e, tt.want)
		keys(e, "u")
		assertText(t, e, tt.text)
	}
}

func TestNumericTargetsAndEmptyLineMotion(t *testing.T) {
	e := fixture("abc")
	keys(e, "r1")
	assertText(t, e, "1bc")
	e = fixture("x2y")
	keys(e, "f2")
	if e.Buffer().Cursor != 1 {
		t.Fatal("digit search failed")
	}
	e = fixture("\nnext")
	keys(e, "d$")
	assertText(t, e, "\nnext")
	e = fixture("a")
	e.Buffers = append(e.Buffers, NewBuffer())
	keys(e, " 2")
	if e.Active != 1 {
		t.Fatal("leader buffer selection failed")
	}
}
