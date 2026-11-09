package editor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func workspaceKeys(e *Editor, id string) { keys(e, " "); e.Handle("<Tab>"); keys(e, id) }

func TestWorkspaceSwitchAndQuit(t *testing.T) {
	e := fixture("one")
	e.Root = t.TempDir()
	first := e.Buffer()
	keys(e, "Ax")
	e.Handle("<Esc>")
	workspaceKeys(e, "2")
	if e.Workspace != 2 || e.Buffer() == first || len(e.Buffer().Text) != 0 {
		t.Fatal("workspace did not start empty")
	}
	e.Root = t.TempDir()
	root2 := e.Root
	keys(e, "itwo")
	e.Handle("<Esc>")
	second := e.Buffer()
	workspaceKeys(e, "1")
	if e.Buffer() != first || !first.Dirty() {
		t.Fatal("workspace lost its edited buffer")
	}
	assertText(t, e, "onex")
	e.Execute("qa")
	if e.Quit {
		t.Fatal("quit discarded dirty workspaces")
	}
	e.Execute("q!")
	if e.Quit || e.Workspace != 2 || e.Buffer() != second || e.Root != root2 {
		t.Fatal("closing a workspace discarded another workspace")
	}
	assertText(t, e, "two")
	e.Execute("qa!")
	if !e.Quit {
		t.Fatal("forced quit failed")
	}
}

func TestWorkspaceHiddenChangesAndActiveBuffer(t *testing.T) {
	e := fixture("")
	keys(e, "ihidden")
	e.Handle("<Esc>")
	workspaceKeys(e, "9")
	e.Execute("qa")
	if e.Quit {
		t.Fatal("hidden dirty buffer was ignored")
	}
	e.Execute("q")
	if e.Quit || e.Workspace != 1 {
		t.Fatal("clean workspace closed over dirty workspace")
	}
	e.Buffers = append(e.Buffers, NewBuffer())
	e.Active = 1
	e.Buffer().Text = []rune("second")
	e.Buffer().Cursor = 3
	workspaceKeys(e, "2")
	workspaceKeys(e, "1")
	if e.Active != 1 || e.Buffer().Cursor != 3 {
		t.Fatal("active buffer or cursor lost")
	}
	workspaceKeys(e, "0")
	if e.Workspace != 1 {
		t.Fatal("invalid workspace accepted")
	}
	keys(e, " ")
	e.Handle("<Tab>")
	e.Handle("<Esc>")
	keys(e, "x")
	assertText(t, e, "secnd")
}

func TestDoomSaveAndProjectPicker(t *testing.T) {
	base := t.TempDir()
	project1 := filepath.Join(base, "owner", "alpha")
	project2 := filepath.Join(base, "owner", "beta")
	project3 := filepath.Join(base, "other", "gamma")
	for _, p := range []string{project1, project2, project3} {
		if err := os.MkdirAll(filepath.Join(p, ".git"), 0755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(p, "main.go"), []byte("package main"), 0644)
	}
	e := fixture("edited")
	e.Root = project1
	e.Buffer().Path = filepath.Join(project1, "new.txt")
	keys(e, " f")
	if e.Mode != Normal {
		t.Fatal("file prefix opened picker before save key")
	}
	keys(e, "s")
	data, err := os.ReadFile(e.Buffer().Path)
	if err != nil || string(data) != "edited" {
		t.Fatalf("leader save failed: %q %v", data, err)
	}
	keys(e, " pp")
	if e.Mode != Picker || !e.PickingProjects {
		t.Fatal("project picker did not open")
	}
	for _, p := range []string{project1, project2, project3} {
		found := false
		for _, match := range e.Matches {
			if match == p {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing project %s: %v", p, e.Matches)
		}
	}
	keys(e, "beta")
	if len(e.Matches) != 1 {
		t.Fatalf("project filter: %v", e.Matches)
	}
	e.Handle("<Enter>")
	if e.Root != project2 || e.Mode != Picker || e.PickingProjects {
		t.Fatal("project selection did not open file picker")
	}
	keys(e, "main.go")
	e.Handle("<Enter>")
	if e.Buffer().Path != filepath.Join(project2, "main.go") {
		t.Fatal("wrong project file opened")
	}
	e.Execute("e another.txt")
	if e.Buffer().Path != filepath.Join(project2, "another.txt") {
		t.Fatal("relative open did not use project directory")
	}
	keys(e, "ihello")
	e.Handle("<Esc>")
	e.Execute("w saved.txt")
	data, err = os.ReadFile(filepath.Join(project2, "saved.txt"))
	if err != nil || string(data) != "hello" {
		t.Fatal("relative save did not use project directory")
	}
	keys(e, " ff")
	if e.Mode != Picker || e.PickingProjects {
		t.Fatal("leader find file failed")
	}
	e.Handle("<Esc>")
	e.Active = 0
	assertText(t, e, "edited")
}

func TestShellCommandDispatch(t *testing.T) {
	for _, tt := range []struct {
		input, command string
		interactive    bool
	}{
		{`!printf '%s' "a b" | cat > out`, `printf '%s' "a b" | cat > out`, false},
		{"! false", "false", false},
		{"term", "", true}, {"terminal", "", true},
		{"term printf 'hello world'", "printf 'hello world'", true},
		{"term\tprintf hello", "printf hello", true},
	} {
		t.Run(tt.input, func(t *testing.T) {
			e := fixture("unsaved")
			keys(e, "Ax")
			e.Handle("<Esc>")
			called := false
			e.RunShell = func(command string, interactive bool) error {
				called = true
				if command != tt.command || interactive != tt.interactive {
					t.Fatalf("shell request = %q %v", command, interactive)
				}
				return nil
			}
			keys(e, ":"+tt.input)
			e.Handle("<Enter>")
			if !called || e.Mode != Normal || e.Quit {
				t.Fatal("shell request not dispatched from command mode")
			}
			assertText(t, e, "unsavedx")
			keys(e, "u")
			assertText(t, e, "unsaved")
		})
	}
	e := fixture("")
	e.Execute("!")
	if e.Message != "Use :!command" {
		t.Fatal("empty command accepted")
	}
	e.Execute("!true")
	if !strings.Contains(e.Message, "unavailable") {
		t.Fatal("missing shell handler")
	}
	e.RunShell = func(string, bool) error { return errors.New("exit status 7") }
	e.Execute("!false")
	if !strings.Contains(e.Message, "exit status 7") {
		t.Fatal("shell error not reported")
	}
}
