package editor

import (
	"errors"
	"strings"
	"testing"
)

func TestShellFiltersAndUndo(t *testing.T) {
	for _, tt := range []struct {
		command, text, output, want, input string
		cursor                             int
	}{
		{".!ls", "before\nreplace me\nafter", "a.go\nb.go\n", "before\na.go\nb.go\nafter", "replace me\n", 9},
		{".!cat", "last", "changed", "changed", "last\n", 0},
		{"%!sort", "c\nb\na\n", "a\nb\nc\n", "a\nb\nc\n", "c\nb\na\n", 0},
		{"2,3!sort", "head\nc\nb\ntail", "b\nc", "head\nb\nc\ntail", "c\nb\n", 0},
		{".!true", "a\nb\nc", "", "a\nc", "b\n", 2},
		{".!echo", "", "new\n", "new\n", "\n", 0},
	} {
		t.Run(tt.command+tt.text, func(t *testing.T) {
			e := fixture(tt.text)
			e.Buffer().Cursor = tt.cursor
			e.CaptureShell = func(command, input string) (string, error) {
				if input != tt.input {
					t.Fatalf("stdin=%q, want %q", input, tt.input)
				}
				return tt.output, nil
			}
			keys(e, ":"+tt.command)
			e.Handle("<Enter>")
			assertText(t, e, tt.want)
			if !e.Buffer().Dirty() {
				t.Fatal("filter did not mark buffer dirty")
			}
			keys(e, "u")
			assertText(t, e, tt.text)
			e.Handle("<C-r>")
			assertText(t, e, tt.want)
		})
	}
}

func TestReadShellOutput(t *testing.T) {
	for _, tt := range []struct{ text, output, want string }{
		{"first\nnext", "one\ntwo\n", "first\none\ntwo\nnext"},
		{"first\nnext", "one", "first\none\nnext"},
		{"last", "one\n", "last\none\n"},
		{"", "one\n", "one\n"},
		{"last", "", "last"},
	} {
		for _, command := range []string{"r !printf output", "read !printf output", "r!printf output"} {
			e := fixture(tt.text)
			e.CaptureShell = func(command, input string) (string, error) {
				if input != "" {
					t.Fatal("read command received buffer contents")
				}
				return tt.output, nil
			}
			e.Execute(command)
			assertText(t, e, tt.want)
			if tt.output != "" {
				keys(e, "u")
				assertText(t, e, tt.text)
			}
		}
	}
}

func TestFilterFailureAndVisualRange(t *testing.T) {
	e := fixture("one\ntwo\nthree")
	calls := 0
	e.CaptureShell = func(command, input string) (string, error) { calls++; return "partial", errors.New("exit status 7") }
	e.Execute(".!false")
	assertText(t, e, "one\ntwo\nthree")
	if !strings.Contains(e.Message, "exit status 7") || e.Buffer().Dirty() {
		t.Fatal("failed filter changed buffer")
	}
	e.CaptureShell = func(command, input string) (string, error) {
		if input != "one\ntwo\n" {
			t.Fatalf("visual input: %q", input)
		}
		return "changed\n", nil
	}
	keys(e, "Vj:!cat")
	e.Handle("<Enter>")
	assertText(t, e, "changed\nthree")
	e = fixture("unchanged")
	e.CaptureShell = func(string, string) (string, error) { return string([]byte{0xff}), nil }
	e.Execute(".!binary")
	assertText(t, e, "unchanged")
	e.Execute("2,1!sort")
	if e.Message != "Invalid line range" {
		t.Fatal("invalid range accepted")
	}
	if calls != 1 {
		t.Fatal("command unexpectedly repeated")
	}
}
