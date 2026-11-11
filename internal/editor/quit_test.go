package editor

import "testing"

func TestForcedQuitExitsAllWindowsBuffersAndWorkspaces(t *testing.T) {
	for _, command := range []string{"q!", "quit!"} {
		t.Run(command, func(t *testing.T) {
			e := fixture("first")
			keys(e, "Ax")
			e.Handle("<Esc>")
			e.SwitchWorkspace(2)
			keys(e, "iunsaved second workspace")
			e.Handle("<Esc>")
			e.SwitchWorkspace(1)
			other := NewBuffer()
			other.Text = []rune("unsaved hidden buffer")
			e.Buffers = append(e.Buffers, other)
			e.SplitWindow(true)
			e.SplitWindow(false)
			keys(e, ":"+command)
			e.Handle("<Enter>")
			if !e.Quit {
				t.Fatal("forced quit only closed a window or buffer")
			}
		})
	}
}

func TestUnforcedQuitStillClosesOnlyCurrentSplit(t *testing.T) {
	e := fixture("text")
	e.SplitWindow(true)
	e.Execute("q")
	if e.Quit || len(e.WindowLayout(80, 24)) != 1 {
		t.Fatal("unforced quit no longer closes the current split")
	}
}
