package editor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yyvfuruta/vim/internal/lsp"
)

type languageLocation = lsp.Location

type languageJump struct {
	workspace int
	buffer    *Buffer
	cursor    int
}

func (e *Editor) requestLanguage(action string) {
	if e.Mode != Normal {
		return
	}
	if e.LanguageAction == nil {
		e.Message = "Language services are unavailable"
		return
	}
	if e.Buffer().Path == "" {
		e.Message = "Save the file before using language commands"
		return
	}
	e.LanguageAction(action)
}

func (e *Editor) LanguageQuery(action string) lsp.Query {
	b := e.Buffer()
	q := lsp.Query{Action: action, Path: b.Path, Position: lsp.PositionAt(b.Text, b.Cursor)}
	seen := make(map[string]bool)
	for _, buffer := range e.Buffers {
		if buffer.Path != "" && !seen[buffer.Path] {
			q.Documents = append(q.Documents, lsp.Document{Path: buffer.Path, Text: string(buffer.Text)})
			seen[buffer.Path] = true
		}
	}
	return q
}

func (e *Editor) ShowLanguageResult(action string, result lsp.Result) {
	if action == "hover" || action == "signatureHelp" {
		if strings.TrimSpace(result.Text) == "" {
			e.Message = "No documentation or signature found"
			return
		}
		e.InfoTitle, e.InfoText, e.InfoScroll = "Documentation", result.Text, 0
		if action == "signatureHelp" {
			e.InfoTitle = "Signature"
		}
		e.Mode = Info
		e.Message = "j/k: scroll  Esc/q: close"
		return
	}
	if len(result.Locations) == 0 {
		e.Message = "No locations found"
		return
	}
	if len(result.Locations) == 1 {
		e.goToLocation(result.Locations[0])
		return
	}
	e.PickingProjects, e.PickingWorkspaces, e.PickingLocations = false, false, true
	e.workspaceEntries = nil
	e.locationEntries = make(map[string]languageLocation)
	e.Files = nil
	for _, location := range result.Locations {
		path := location.Path
		if rel, err := filepath.Rel(e.Root, path); err == nil {
			path = rel
		}
		entry := fmt.Sprintf("%s:%d:%d", path, location.Position.Line+1, location.Position.Character+1)
		if _, exists := e.locationEntries[entry]; !exists {
			e.Files = append(e.Files, entry)
			e.locationEntries[entry] = location
		}
	}
	e.Mode, e.Prompt = Picker, ""
	e.filterFiles()
	e.Message = "Select a location"
}

func (e *Editor) goToLocation(location lsp.Location) {
	origin := languageJump{e.Workspace, e.Buffer(), e.Buffer().Cursor}
	e.openPath(location.Path)
	if e.Buffer().Path != location.Path {
		return
	}
	e.jumps = append(e.jumps, origin)
	if len(e.jumps) > 100 {
		e.jumps = e.jumps[1:]
	}
	e.Buffer().Cursor = lsp.Offset(e.Buffer().Text, location.Position)
	e.Buffer().NormalCursor()
	e.desiredCol = -1
	e.syncWindow()
	e.Message = "Ctrl-o: jump back"
}

func (e *Editor) jumpBack() {
	if len(e.jumps) == 0 {
		e.Message = "No previous location"
		return
	}
	i := len(e.jumps) - 1
	j := e.jumps[i]
	e.jumps = e.jumps[:i]
	if !e.workspaceExists(j.workspace) {
		e.Message = "Previous workspace no longer exists"
		return
	}
	e.SwitchWorkspace(j.workspace)
	index := -1
	for i, b := range e.Buffers {
		if b == j.buffer {
			index = i
			break
		}
	}
	if index < 0 {
		e.openPath(j.buffer.Path)
		if e.Buffer().Path != j.buffer.Path {
			return
		}
	} else {
		e.Active = index
	}
	e.Buffer().Cursor = min(j.cursor, len(e.Buffer().Text))
	e.Buffer().NormalCursor()
	e.desiredCol = -1
	e.syncWindow()
	e.Message = ""
}

func (e *Editor) infoKey(key string) {
	switch key {
	case "<Esc>", "q", "K":
		e.Mode = Normal
		e.InfoText, e.InfoTitle, e.Message = "", "", ""
		e.reset()
	case "j", "<Down>":
		e.InfoScroll++
	case "k", "<Up>":
		e.InfoScroll = max(0, e.InfoScroll-1)
	case "<C-d>", "<PageDown>":
		e.InfoScroll += 15
	case "<C-u>", "<PageUp>":
		e.InfoScroll = max(0, e.InfoScroll-15)
	case "g":
		e.InfoScroll = 0
	case "G":
		e.InfoScroll = len([]rune(e.InfoText))
	}
}
