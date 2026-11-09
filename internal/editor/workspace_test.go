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

func TestWorkspaceListUsesLiveState(t *testing.T) {
	e := fixture("")
	firstRoot := t.TempDir()
	e.Root = firstRoot
	workspaceKeys(e, "9")
	e.Root = t.TempDir()
	workspaceKeys(e, "1")
	// The saved W1 entry still has one buffer; the active workspace now has two.
	e.Buffers = append(e.Buffers, NewBuffer())
	e.Active = 1
	keys(e, "iedited")
	e.Handle("<Esc>")
	current := e.Buffer()
	keys(e, ":workspaces")
	e.Handle("<Enter>")
	if e.Mode != Picker || !e.PickingWorkspaces || e.PickingProjects {
		t.Fatal("workspace list did not open")
	}
	if len(e.Matches) != 2 || !strings.HasPrefix(e.Matches[0], "W1 *+") || !strings.HasPrefix(e.Matches[1], "W9 ") {
		t.Fatalf("workspace order/markers: %v", e.Matches)
	}
	if !strings.Contains(e.Matches[0], "2 buffers") || !strings.Contains(e.Matches[0], firstRoot) {
		t.Fatalf("list uses stale workspace state: %v", e.Matches)
	}
	e.Handle("<Esc>")
	if e.Workspace != 1 || e.Buffer() != current || e.PickingWorkspaces {
		t.Fatal("cancel changed workspace")
	}
	assertText(t, e, "edited")
	e.Execute("ws")
	keys(e, "W9")
	if len(e.Matches) == 0 || !strings.HasPrefix(e.Matches[0], "W9 ") {
		t.Fatalf("workspace filter ranking: %v", e.Matches)
	}
	e.Handle("<Enter>")
	if e.Workspace != 9 || e.Mode != Normal || e.PickingWorkspaces {
		t.Fatal("selection did not switch workspace")
	}
	e.Execute("workspaces")
	if !strings.HasPrefix(e.Matches[0], "W1 +") || !strings.HasPrefix(e.Matches[1], "W9 *") {
		t.Fatalf("saved/current markers: %v", e.Matches)
	}
	e.Handle("<Enter>")
	if e.Buffer() != current {
		t.Fatal("selection lost original buffer")
	}
	assertText(t, e, "edited")
}

func TestWorkspaceListCurrentAndNavigation(t *testing.T) {
	e := fixture("")
	e.Execute("ws")
	if len(e.Matches) != 1 || !strings.Contains(e.Matches[0], "1 buffer") {
		t.Fatalf("initial list: %v", e.Matches)
	}
	e.Handle("<Enter>")
	if e.Mode != Normal || e.Workspace != 1 || e.PickingWorkspaces {
		t.Fatal("selecting current workspace left picker active")
	}
	workspaceKeys(e, "3")
	workspaceKeys(e, "1")
	e.Execute("workspaces")
	e.Handle("<Down>")
	e.Handle("<Enter>")
	if e.Workspace != 3 {
		t.Fatal("workspace keyboard selection failed")
	}
	e.Execute("ws")
	keys(e, "no such workspace")
	if len(e.Matches) != 0 {
		t.Fatal("unexpected matches")
	}
	e.Handle("<Enter>")
	if e.Mode != Normal || e.Workspace != 3 || e.PickingWorkspaces {
		t.Fatal("empty result changed workspace or left the picker active")
	}
}

func TestProjectFindBindingAcrossWorkspaces(t *testing.T) {
	root1, root2 := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(root1, "first.txt"), []byte("first"), 0644)
	os.WriteFile(filepath.Join(root2, "second.txt"), []byte("second"), 0644)
	e := fixture("unsaved")
	e.Root = root1
	workspaceKeys(e, "2")
	e.Root = root2
	// Ensure opening a file picker clears a previous workspace-picker state.
	e.Execute("ws")
	e.Handle("<Esc>")
	keys(e, " pf")
	if e.Mode != Picker || e.PickingProjects || e.PickingWorkspaces || len(e.Matches) != 1 || e.Matches[0] != "second.txt" {
		t.Fatalf("wrong project files: %v", e.Matches)
	}
	e.Handle("<Enter>")
	if e.Buffer().Path != filepath.Join(root2, "second.txt") {
		t.Fatal("wrong project file opened")
	}
	workspaceKeys(e, "1")
	keys(e, " pf")
	if len(e.Matches) != 1 || e.Matches[0] != "first.txt" {
		t.Fatalf("workspace project was not restored: %v", e.Matches)
	}
	e.Handle("<Esc>")
	assertText(t, e, "unsaved")
}

func TestCreateDeleteWorkspaces(t *testing.T) {
	e := fixture("first")
	e.Root = t.TempDir()
	root := e.Root
	first := e.Buffer()
	workspaceKeys(e, "n")
	if e.Workspace != 2 || e.Root != root || e.Buffer() == first {
		t.Fatal("leader creation did not create a fresh workspace")
	}
	e.Execute("wsnew 5")
	if e.Workspace != 5 {
		t.Fatal("explicit creation failed")
	}
	e.Execute("wsnew 2")
	if e.Workspace != 5 || !strings.Contains(e.Message, "already exists") {
		t.Fatal("existing workspace was replaced")
	}
	workspaceKeys(e, "d")
	if e.Workspace != 1 || e.workspaceExists(5) || e.Buffer() != first {
		t.Fatal("active deletion did not restore another workspace")
	}
	e.Execute("wsdelete 2")
	if e.Workspace != 1 || e.workspaceExists(2) {
		t.Fatal("inactive deletion changed the active workspace")
	}
	e.Execute("wsdelete!")
	if !e.workspaceExists(1) || !strings.Contains(e.Message, "last workspace") {
		t.Fatal("last workspace was deleted")
	}
	e.Execute("wsnew")
	if e.Workspace != 2 {
		t.Fatal("deleted workspace number was not reused")
	}
	e.Execute("workspaces")
	if len(e.Matches) != 2 {
		t.Fatal("workspace list includes deleted workspaces")
	}
	e.Handle("<Esc>")
}

func TestWorkspaceDeletionProtectsAllBuffers(t *testing.T) {
	e := fixture("clean")
	first := e.Buffer()
	e.CreateWorkspace(2)
	e.Buffers = append(e.Buffers, NewBuffer())
	e.Active = 1
	keys(e, "ichanged")
	e.Handle("<Esc>")
	e.Active = 0
	e.Execute("wsdelete")
	if e.Workspace != 2 || !strings.Contains(e.Message, "unsaved changes") {
		t.Fatal("dirty inactive buffer was discarded")
	}
	e.SwitchWorkspace(1)
	e.Execute("wsdelete 2")
	if !e.workspaceExists(2) {
		t.Fatal("dirty hidden workspace was discarded")
	}
	e.Execute("wsdelete! 2")
	if e.workspaceExists(2) || e.Buffer() != first || e.Quit {
		t.Fatal("forced deletion affected another workspace")
	}
	e.CreateWorkspace(2)
	keys(e, "inew edits")
	e.Handle("<Esc>")
	e.Execute("wsdelete!")
	if e.Workspace != 1 || e.workspaceExists(2) {
		t.Fatal("forced active deletion failed")
	}
	assertText(t, e, "clean")
}

func TestWorkspaceCreationLimitsAndInvalidIDs(t *testing.T) {
	e := fixture("")
	for id := 2; id <= 9; id++ {
		e.CreateWorkspace(id)
	}
	e.Execute("wsnew")
	if e.Workspace != 9 || !strings.Contains(e.Message, "All 9") {
		t.Fatal("workspace capacity was not enforced")
	}
	for _, command := range []string{"wsnew 0", "wsnew 10", "wsnew nope", "wsdelete -1", "wsdelete 10", "wsdelete 1 2"} {
		e.Execute(command)
		if e.Workspace != 9 || !strings.Contains(e.Message, "between 1 and 9") {
			t.Fatalf("invalid command accepted: %s", command)
		}
	}
	e.Execute("wsdelete 3")
	e.Execute("wsdelete 3")
	if !strings.Contains(e.Message, "does not exist") {
		t.Fatal("missing workspace deletion was not reported")
	}
	e.CreateWorkspace(0)
	if e.Workspace != 3 {
		t.Fatal("creation did not use first available slot")
	}
}
