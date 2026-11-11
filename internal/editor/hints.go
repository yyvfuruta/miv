package editor

import "strings"

type KeyHint struct {
	Key, Description string
}

// KeyHints describes valid continuations without consuming keys or changing modes.
func (e *Editor) KeyHints() (string, []KeyHint) {
	if e.Mode != Normal {
		return "", nil
	}
	var hints []KeyHint
	switch e.pending {
	case " ":
		hints = []KeyHint{{"f", "Files"}, {"p", "Projects"}, {"b", "Buffers"}, {"w", "Windows"},
			{"TAB", "Workspaces"}, {".", "Find file"}, {"SPC", "Find project file"}, {",", "Switch buffer"},
			{"s", "Save file"}, {"0-9", "Switch buffer"}}
	case " f":
		hints = []KeyHint{{"f", "Find file"}, {"s", "Save file"}}
	case " p":
		hints = []KeyHint{{"p", "Switch project"}, {"f", "Find project file"}}
	case " b":
		hints = []KeyHint{{"b", "Switch buffer"}, {"n / ]", "Next buffer"}, {"p / [", "Previous buffer"},
			{"d / k", "Close buffer"}, {"s", "Save buffer"}}
	case " w":
		hints = []KeyHint{{"h", "Focus left"}, {"j", "Focus down"}, {"k", "Focus up"}, {"l", "Focus right"},
			{"s", "Split below"}, {"v", "Split right"}, {"d / q", "Close window"}}
	case " <Tab>":
		hints = []KeyHint{{"n", "New workspace"}, {"d", "Delete workspace"}, {"r", "Rename workspace"},
			{". / TAB", "Choose workspace"}, {"[", "Previous workspace"}, {"]", "Next workspace"},
			{"`", "Last workspace"}, {"1-9 / 0", "Select workspace"}}
	default:
		return "", nil
	}
	prefix := "SPC"
	if len(e.pending) > 1 {
		prefix += " " + strings.ReplaceAll(e.pending[1:], "<Tab>", "TAB")
	}
	return prefix, hints
}
