package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceCyclingNamesAndLast(t *testing.T) {
	e := fixture("one")
	e.Execute("wsrename first")
	e.SwitchWorkspace(5)
	e.Execute("wsrename fifth")
	e.SwitchWorkspace(9)
	e.SwitchWorkspace(1)
	workspaceKeys(e, "]")
	if e.Workspace != 5 || e.WorkspaceName != "fifth" {
		t.Fatal("next workspace did not skip unused numbers or restore its name")
	}
	workspaceKeys(e, "`")
	if e.Workspace != 1 || e.WorkspaceName != "first" {
		t.Fatal("last workspace was not restored")
	}
	workspaceKeys(e, "[")
	if e.Workspace != 9 {
		t.Fatal("previous workspace did not wrap")
	}
	workspaceKeys(e, "]")
	if e.Workspace != 1 {
		t.Fatal("next workspace did not wrap")
	}
	workspaceKeys(e, "0")
	if e.Workspace != 9 {
		t.Fatal("zero did not select the final workspace")
	}
	workspaceKeys(e, "r")
	if e.Mode != Command || e.Prompt != "wsrename " {
		t.Fatal("rename did not open a command prompt")
	}
	keys(e, "last")
	e.Handle("<Enter>")
	if e.WorkspaceName != "last" {
		t.Fatal("workspace rename failed")
	}
	workspaceKeys(e, ".")
	if !e.PickingWorkspaces || len(e.Matches) != 3 {
		t.Fatal("workspace selector did not open")
	}
}

func TestProjectSwitchKeepsSeparateWorkspaceAndReusesIt(t *testing.T) {
	base := t.TempDir()
	alpha, beta := filepath.Join(base, "alpha"), filepath.Join(base, "beta")
	for _, root := range []string{alpha, beta} {
		if err := os.Mkdir(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "main.txt"), []byte("text"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e := fixture("unsaved alpha")
	e.Root = alpha
	e.Buffer().Path = filepath.Join(alpha, "other.txt")
	e.Buffer().saved = "old alpha"
	e.SplitWindow(true)
	original := e.Buffer()
	windows := e.windows.root
	e.switchProject(beta)
	if e.Workspace != 2 || e.Root != beta || e.WorkspaceName != "beta" || e.Mode != Picker {
		t.Fatal("project switch did not start a separate workspace")
	}
	keys(e, "main")
	e.Handle("<Enter>")
	if len(e.Buffers) != 1 {
		t.Fatal("project workspace kept an extra blank buffer")
	}
	e.switchProject(alpha)
	if e.Workspace != 1 || e.Buffer() != original || e.windows.root != windows || !original.Dirty() {
		t.Fatal("returning to a project lost buffers, splits, or unsaved changes")
	}
	e.Handle("<Esc>")
	list := e.WorkspaceList()
	if len(list) != 2 || !list[0].Dirty || !list[0].Active {
		t.Fatal("workspace bar state did not include hidden edits")
	}
}
