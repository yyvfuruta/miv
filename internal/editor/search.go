package editor

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

func (e *Editor) find(pattern string, dir int) {
	if pattern == "" {
		return
	}
	b := e.Buffer()
	needle := []rune(pattern)
	if len(needle) > len(b.Text) {
		e.Message = "Não encontrado: " + pattern
		return
	}
	if dir == 0 {
		dir = 1
	}
	for step := 1; step <= len(b.Text); step++ {
		p := (b.Cursor + dir*step) % len(b.Text)
		if p < 0 {
			p += len(b.Text)
		}
		if p+len(needle) <= len(b.Text) && string(b.Text[p:p+len(needle)]) == pattern {
			b.Cursor = p
			e.Message = ""
			return
		}
	}
	e.Message = "Não encontrado: " + pattern
}
func (e *Editor) promptKey(key string) {
	switch key {
	case "<Esc>":
		if e.Mode == Search {
			e.Buffer().Cursor = e.searchOrigin
		}
		e.Mode = Normal
		e.Prompt = ""
		e.reset()
	case "<Enter>":
		mode, prompt := e.Mode, e.Prompt
		e.Mode = Normal
		e.Prompt = ""
		e.reset()
		switch mode {
		case Command:
			e.Execute(prompt)
		case Search:
			if prompt != "" {
				e.search = prompt
			}
			e.Buffer().Cursor = e.searchOrigin
			e.find(e.search, e.searchDir)
		case Picker:
			if len(e.Matches) > 0 {
				e.openPath(filepath.Join(e.Root, e.Matches[e.PickIndex]))
			}
		}
	case "<Backspace>":
		r := []rune(e.Prompt)
		if len(r) > 0 {
			e.Prompt = string(r[:len(r)-1])
		}
		if e.Mode == Picker {
			e.filterFiles()
		}
	case "<Up>", "<C-k>":
		if e.Mode == Picker {
			e.PickIndex = max(0, e.PickIndex-1)
		}
	case "<Down>", "<C-j>", "<Tab>":
		if e.Mode == Picker {
			e.PickIndex = min(max(0, len(e.Matches)-1), e.PickIndex+1)
		}
	default:
		if len([]rune(key)) == 1 {
			e.Prompt += key
			if e.Mode == Picker {
				e.filterFiles()
			}
		}
	}
}
func (e *Editor) StartPicker() {
	e.Files = nil
	e.Prompt = ""
	e.PickIndex = 0
	skipped := map[string]bool{".git": true, "node_modules": true, "target": true, "vendor": true, ".cache": true}
	err := filepath.WalkDir(e.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == e.Root {
				return err
			}
			return nil
		}
		if d.IsDir() {
			if skipped[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(e.Root, path)
		if err == nil {
			e.Files = append(e.Files, rel)
		}
		if len(e.Files) >= 20000 {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		e.Message = err.Error()
		return
	}
	e.Mode = Picker
	e.filterFiles()
}

// Fuzzy ranking favors contiguous matches and shorter paths.
func fuzzy(path, query string) (int, bool) {
	p, q := []rune(strings.ToLower(path)), []rune(strings.ToLower(query))
	at, score, last := 0, 0, -2
	for _, r := range q {
		found := false
		for at < len(p) {
			i := at
			at++
			if p[i] == r {
				score += i
				if i == last+1 {
					score -= 8
				}
				last = i
				found = true
				break
			}
		}
		if !found {
			return 0, false
		}
	}
	return score + len(p), true
}
func (e *Editor) filterFiles() {
	type match struct {
		path  string
		score int
	}
	var matches []match
	for _, p := range e.Files {
		if score, ok := fuzzy(p, e.Prompt); ok {
			matches = append(matches, match{p, score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score == matches[j].score {
			return matches[i].path < matches[j].path
		}
		return matches[i].score < matches[j].score
	})
	e.Matches = nil
	for _, m := range matches {
		e.Matches = append(e.Matches, m.path)
	}
	e.PickIndex = 0
}
