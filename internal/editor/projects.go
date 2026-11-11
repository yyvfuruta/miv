package editor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func gitRoot(path string) string {
	path, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	for {
		if info, err := os.Stat(filepath.Join(path, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		path = parent
	}
}

// Discover nearby Git repositories without recursively scanning the home directory.
// One level of owner directories also covers layouts such as github.com/owner/repo.
func (e *Editor) StartProjects() {
	e.PickingLocations, e.locationEntries = false, nil
	roots := make(map[string]bool)
	add := func(path string) {
		if root := gitRoot(path); root != "" {
			roots[root] = true
		}
	}
	add(e.Root)
	if e.Buffer().Path != "" {
		add(filepath.Dir(e.Buffer().Path))
	}
	base, err := filepath.Abs(e.Root)
	if err != nil {
		e.Message = err.Error()
		return
	}
	if root := gitRoot(base); root != "" {
		base = root
	}
	roots[base] = true
	for _, w := range e.workspaces {
		add(w.root)
	}
	budget := 2000
	for _, parent := range []string{filepath.Dir(base), filepath.Dir(filepath.Dir(base))} {
		entries, _ := os.ReadDir(parent)
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if budget <= 0 {
				break
			}
			budget--
			candidate := filepath.Join(parent, entry.Name())
			if root := gitRoot(candidate); root == candidate {
				roots[root] = true
				continue
			}
			children, _ := os.ReadDir(candidate)
			for _, child := range children {
				if !child.IsDir() || strings.HasPrefix(child.Name(), ".") {
					continue
				}
				if budget <= 0 {
					break
				}
				budget--
				path := filepath.Join(candidate, child.Name())
				if root := gitRoot(path); root == path {
					roots[root] = true
				}
			}
		}
	}
	e.Files = nil
	for root := range roots {
		e.Files = append(e.Files, root)
	}
	sort.Strings(e.Files)
	e.Mode, e.Prompt = Picker, ""
	e.PickingProjects, e.PickingWorkspaces = true, false
	e.workspaceEntries = nil
	e.filterFiles()
}
