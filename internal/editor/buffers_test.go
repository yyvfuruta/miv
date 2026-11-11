package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindUsesCurrentWindowAndKeepsHiddenEdits(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{"one.txt": "one", "two.txt": "two"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e, err := New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	keys(e, "one")
	e.Handle("<Enter>")
	if len(e.Buffers) != 1 {
		t.Fatal("opening the first file left an extra empty buffer")
	}
	e.WindowLayout(80, 24)
	window := e.windows.active
	original := e.Buffer()
	keys(e, "iX")
	e.Handle("<Esc>")
	keys(e, " pf")
	keys(e, "two")
	e.Handle("<Enter>")
	if e.Buffer().Path != filepath.Join(root, "two.txt") || e.windows.active != window || len(e.WindowLayout(80, 24)) != 1 {
		t.Fatal("find opened another window")
	}
	if !original.Dirty() || string(original.Text) != "Xone" {
		t.Fatal("find discarded hidden edits")
	}
	keys(e, " bb")
	if !e.PickingBuffers || len(e.Matches) != 2 {
		t.Fatal("buffer picker did not list open files")
	}
	keys(e, "one.txt")
	e.Handle("<Enter>")
	if e.Buffer() != original || e.PickingBuffers || e.windows.active != window {
		t.Fatal("buffer picker did not reuse the same window and original buffer")
	}
	assertText(t, e, "Xone")
	keys(e, " pf")
	keys(e, "two")
	e.Handle("<Enter>")
	if len(e.Buffers) != 2 {
		t.Fatal("reopening a file duplicated its buffer")
	}
}

func TestBufferPickerIsWorkspaceLocalAndCancelable(t *testing.T) {
	e := fixture("one")
	one := e.Buffer()
	e.SwitchWorkspace(2)
	e.Buffer().Text = []rune("two")
	e.Buffer().Path = filepath.Join(t.TempDir(), "other.txt")
	e.SwitchWorkspace(1)
	keys(e, " ,")
	if len(e.Matches) != 1 || strings.Contains(e.Matches[0], "other.txt") {
		t.Fatal("buffer picker leaked another workspace")
	}
	e.Handle("<Esc>")
	if e.Mode != Normal || e.PickingBuffers || e.bufferEntries != nil || e.Buffer() != one {
		t.Fatal("canceling the buffer picker changed the active buffer")
	}
	e.StartBuffers()
	e.StartWorkspaces()
	if e.PickingBuffers || e.bufferEntries != nil {
		t.Fatal("workspace picker retained buffer-picker state")
	}
}

func TestBufferDeletionKeepsWorkspaceAndProtectsChanges(t *testing.T) {
	e := fixture("one")
	e.SplitWindow(true)
	keys(e, " bd")
	if e.Quit || len(e.Buffers) != 1 || len(e.WindowLayout(80, 24)) != 2 || e.Buffer().Path != "" {
		t.Fatal("deleting the last buffer closed its workspace or windows")
	}
	for _, region := range e.WindowLayout(80, 24) {
		if region.Window.Buffer != e.Buffer() {
			t.Fatal("window kept a deleted buffer")
		}
	}
	keys(e, "iunsaved")
	e.Handle("<Esc>")
	old := e.Buffer()
	keys(e, " bd")
	if e.Buffer() != old || !strings.Contains(e.Message, "Unsaved changes") {
		t.Fatal("buffer deletion discarded edits")
	}
	e.Execute("bd!")
	if e.Quit || e.Buffer() == old || e.Buffer().Dirty() {
		t.Fatal("forced buffer deletion did not leave an empty workspace")
	}
}

func TestFindInSplitLeavesOtherWindowAlone(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "two.txt")
	if err := os.WriteFile(path, []byte("two"), 0600); err != nil {
		t.Fatal(err)
	}
	e := fixture("one")
	e.Root = root
	e.SplitWindow(true)
	original := e.Buffer()
	active := e.windows.active
	keys(e, " fftwo")
	e.Handle("<Enter>")
	if e.Buffer().Path != path || e.windows.active != active {
		t.Fatal("find did not replace the active split's file")
	}
	keys(e, " wh")
	if e.Buffer() != original {
		t.Fatal("find changed the other split")
	}
}
