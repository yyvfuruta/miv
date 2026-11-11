package editor

import (
	"fmt"
	"path/filepath"
)

type workspace struct {
	buffers []*Buffer
	active  int
	root    string
	windows windowState
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
	e.syncWindow()
	e.workspaces[e.Workspace] = workspace{buffers: e.Buffers, active: e.Active, root: e.Root, windows: e.windows}
	next, ok := e.workspaces[id]
	if !ok {
		next = workspace{buffers: []*Buffer{NewBuffer()}, root: e.Root}
	}
	e.Buffers, e.Active, e.Root = next.buffers, next.active, next.root
	e.windows = next.windows
	if e.windows.active != nil {
		e.activateWindow(e.windows.active)
	}
	e.Workspace = id
	e.Mode, e.Prompt = Normal, ""
	e.PickingProjects, e.PickingWorkspaces = false, false
	e.workspaceEntries = nil
	e.PickingLocations, e.locationEntries = false, nil
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
	e.PickingLocations, e.locationEntries = false, nil
	e.Files = nil
	e.workspaceEntries = make(map[string]int)
	for id := 1; id <= 9; id++ {
		w, exists := e.workspaces[id]
		mark := ""
		if id == e.Workspace {
			w = workspace{buffers: e.Buffers, active: e.Active, root: e.Root, windows: e.windows}
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

func (e *Editor) workspaceExists(id int) bool {
	if id == e.Workspace {
		return true
	}
	_, exists := e.workspaces[id]
	return exists
}

// CreateWorkspace chooses the first unused number when id is zero.
func (e *Editor) CreateWorkspace(id int) {
	if id == 0 {
		for candidate := 1; candidate <= 9; candidate++ {
			if !e.workspaceExists(candidate) {
				id = candidate
				break
			}
		}
		if id == 0 {
			e.Message = "All 9 workspaces are in use"
			return
		}
	}
	if id < 1 || id > 9 {
		e.Message = "Workspace number must be between 1 and 9"
		return
	}
	if e.workspaceExists(id) {
		e.Message = fmt.Sprintf("Workspace %d already exists", id)
		return
	}
	e.SwitchWorkspace(id)
	e.Message = fmt.Sprintf("Created workspace %d", id)
}

func (e *Editor) DeleteWorkspace(id int, force bool) {
	if id == 0 {
		id = e.Workspace
	}
	if id < 1 || id > 9 || !e.workspaceExists(id) {
		e.Message = fmt.Sprintf("Workspace %d does not exist", id)
		return
	}
	next := 0
	for candidate := 1; candidate <= 9; candidate++ {
		if candidate != id && e.workspaceExists(candidate) {
			next = candidate
			break
		}
	}
	if next == 0 {
		e.Message = "Cannot delete the last workspace"
		return
	}
	buffers := e.workspaces[id].buffers
	if id == e.Workspace {
		buffers = e.Buffers
	}
	if !force {
		for _, b := range buffers {
			if b.Dirty() {
				e.Message = fmt.Sprintf("Workspace %d has unsaved changes; use :wsdelete! %d to discard them", id, id)
				return
			}
		}
	}
	if id == e.Workspace {
		e.SwitchWorkspace(next)
	}
	delete(e.workspaces, id)
	e.Message = fmt.Sprintf("Deleted workspace %d", id)
}
