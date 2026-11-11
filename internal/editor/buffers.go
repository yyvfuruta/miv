package editor

import (
	"fmt"
	"path/filepath"
)

// StartBuffers lists only this workspace's buffers, including hidden edits.
func (e *Editor) StartBuffers() {
	e.PickingProjects, e.PickingWorkspaces, e.PickingLocations = false, false, false
	e.workspaceEntries, e.locationEntries = nil, nil
	e.PickingBuffers = true
	e.bufferEntries = make(map[string]*Buffer)
	e.Files = nil
	for i, b := range e.Buffers {
		name := b.Path
		if name == "" {
			name = "[new]"
		} else if root, err := filepath.Abs(e.Root); err == nil {
			if rel, err := filepath.Rel(root, name); err == nil {
				name = rel
			}
		}
		mark := ""
		if i == e.Active {
			mark = "*"
		}
		if b.Dirty() {
			mark += "+"
		}
		entry := fmt.Sprintf("%d %-2s %s", i+1, mark, name)
		e.Files = append(e.Files, entry)
		e.bufferEntries[entry] = b
	}
	e.Mode, e.Prompt = Picker, ""
	e.filterFiles()
}

func (e *Editor) selectBuffer(b *Buffer) {
	for i, candidate := range e.Buffers {
		if candidate == b {
			e.Buffer().End()
			e.Active = i
			e.desiredCol = -1
			e.syncWindow()
			return
		}
	}
}

// Deleting a buffer keeps its workspace and windows alive, even for the last file.
func (e *Editor) deleteBuffer(force bool) {
	old := e.Buffer()
	if old.Dirty() && !force {
		e.Message = "Unsaved changes; use :w to save or :bd! to discard them"
		return
	}
	old.End()
	if len(e.Buffers) == 1 {
		e.Buffers[0] = NewBuffer()
	} else {
		e.Buffers = append(e.Buffers[:e.Active], e.Buffers[e.Active+1:]...)
		e.Active = min(e.Active, len(e.Buffers)-1)
	}
	e.replaceWindowBuffer(old)
	e.desiredCol = -1
	e.syncWindow()
	e.Message = "Buffer closed"
}
