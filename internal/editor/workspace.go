package editor

import (
	"fmt"
	"path/filepath"
)

type workspace struct {
	buffers []*Buffer
	active  int
	root    string
}

func (w workspace) hasContent() bool {
	for _, b := range w.buffers {
		if b.Path != "" || len(b.Text) > 0 || b.Dirty() {
			return true
		}
	}
	return false
}

// Workspaces keep buffers, active positions and project directories in memory.
func (e *Editor) SwitchWorkspace(id int) {
	if id < 1 || id > 9 || id == e.Workspace {
		return
	}
	e.Buffer().End()
	e.workspaces[e.Workspace] = workspace{e.Buffers, e.Active, e.Root}
	next, ok := e.workspaces[id]
	if !ok {
		next = workspace{buffers: []*Buffer{NewBuffer()}, root: e.Root}
	}
	e.Buffers, e.Active, e.Root = next.buffers, next.active, next.root
	e.Workspace = id
	e.Mode, e.Prompt = Normal, ""
	e.PickingProjects, e.PickingWorkspaces = false, false
	e.workspaceEntries = nil
	e.desiredCol = -1
	e.reset()
}

func (e *Editor) allBuffers() []*Buffer {
	buffers := append([]*Buffer(nil), e.Buffers...)
	for id, w := range e.workspaces {
		if id != e.Workspace {
			buffers = append(buffers, w.buffers...)
		}
	}
	return buffers
}

func (e *Editor) projectPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(e.Root, path)
}

// StartWorkspaces lists only workspaces created in this session. Read the active
// workspace directly: its saved entry may still refer to an older buffer list.
func (e *Editor) StartWorkspaces() {
	e.Files = nil
	e.workspaceEntries = make(map[string]int)
	for id := 1; id <= 9; id++ {
		w, exists := e.workspaces[id]
		mark := ""
		if id == e.Workspace {
			w = workspace{e.Buffers, e.Active, e.Root}
			exists, mark = true, "*"
		}
		if !exists {
			continue
		}
		for _, b := range w.buffers {
			if b.Dirty() {
				mark += "+"
				break
			}
		}
		root, err := filepath.Abs(w.root)
		if err != nil {
			root = w.root
		}
		unit := "buffers"
		if len(w.buffers) == 1 {
			unit = "buffer"
		}
		entry := fmt.Sprintf("W%d %-2s | %d %s | %s", id, mark, len(w.buffers), unit, root)
		e.Files = append(e.Files, entry)
		e.workspaceEntries[entry] = id
	}
	e.Mode, e.Prompt = Picker, ""
	e.PickingProjects, e.PickingWorkspaces = false, true
	e.filterFiles()
}
