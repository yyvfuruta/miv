package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectionalSplitsAndIndependentCursors(t *testing.T) {
	e := fixture("abcdef\nsecond")
	e.Buffer().Cursor = 1
	e.WindowLayout(100, 30)
	first := e.windows.active
	keys(e, " wv")
	second := e.windows.active
	if first == second || len(e.WindowLayout(100, 30)) != 2 {
		t.Fatal("vertical split not created")
	}
	keys(e, "3l")
	if e.Buffer().Cursor != 4 {
		t.Fatal("movement in new window failed")
	}
	keys(e, " wh")
	if e.windows.active != first || e.Buffer().Cursor != 1 {
		t.Fatal("left window cursor was overwritten")
	}
	keys(e, " wl")
	if e.windows.active != second || e.Buffer().Cursor != 4 {
		t.Fatal("right window cursor was lost")
	}
	keys(e, " ws")
	third := e.windows.active
	if len(e.WindowLayout(100, 30)) != 3 {
		t.Fatal("horizontal split not created")
	}
	keys(e, " wk")
	if e.windows.active != second {
		t.Fatal("up did not return to upper split")
	}
	keys(e, " wj")
	if e.windows.active != third {
		t.Fatal("down did not select lower split")
	}
	keys(e, " wq")
	if len(e.WindowLayout(100, 30)) != 2 || e.WindowExists(third) {
		t.Fatal("closed window is still present")
	}
}

func TestSplitFileSelectionAndHiddenChanges(t *testing.T) {
	root := t.TempDir()
	one, two := filepath.Join(root, "one.txt"), filepath.Join(root, "two.txt")
	os.WriteFile(one, []byte("one"), 0644)
	os.WriteFile(two, []byte("two"), 0644)
	e, err := New([]string{one})
	if err != nil {
		t.Fatal(err)
	}
	e.Execute("vsplit " + two)
	if len(e.WindowLayout(80, 24)) != 2 || e.Buffer().Path != two {
		t.Fatal("vsplit did not open requested file")
	}
	keys(e, " whiX")
	e.Handle("<Esc>")
	assertText(t, e, "Xone")
	e.Execute("q")
	if e.Quit || len(e.WindowLayout(80, 24)) != 1 || e.Buffer().Path != two {
		t.Fatal("quit closed the editor instead of the window")
	}
	e.Execute("qa")
	if e.Quit {
		t.Fatal("hidden modified buffer was discarded")
	}
	e.Execute("close")
	if e.Quit {
		t.Fatal("closing last window quit the editor")
	}
}

func TestWindowLayoutsBelongToWorkspaces(t *testing.T) {
	e := fixture("abcdef")
	e.WindowLayout(100, 30)
	keys(e, " wv3l")
	active := e.windows.active
	cursor := e.Buffer().Cursor
	e.CreateWorkspace(2)
	if len(e.WindowLayout(100, 30)) != 1 {
		t.Fatal("new workspace inherited split layout")
	}
	e.SplitWindow(false)
	oldSecond := e.windows.active
	e.SwitchWorkspace(1)
	if len(e.WindowLayout(100, 30)) != 2 || e.windows.active != active || e.Buffer().Cursor != cursor {
		t.Fatal("workspace lost layout or active cursor")
	}
	e.DeleteWorkspace(2, false)
	if e.WindowExists(oldSecond) {
		t.Fatal("deleted workspace kept its windows")
	}
	e.CreateWorkspace(2)
	if len(e.WindowLayout(100, 30)) != 1 {
		t.Fatal("recreated workspace kept deleted layout")
	}
}

func TestDeletingBufferRetargetsAllWindows(t *testing.T) {
	e := fixture("one")
	first := e.Buffer()
	second := NewBuffer()
	second.Text = []rune("two")
	second.saved = "two"
	e.Buffers = append(e.Buffers, second)
	e.SplitWindow(true)
	e.Execute("bd")
	for _, w := range e.WindowLayout(80, 24) {
		if w.Window.Buffer == first {
			t.Fatal("window retained a deleted buffer")
		}
	}
	keys(e, " wh")
	assertText(t, e, "two")
	for _, size := range [][2]int{{1, 1}, {2, 2}, {5, 3}, {80, 24}} {
		e.WindowLayout(size[0], size[1])
	}
}
