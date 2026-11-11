// Package lsp provides on-demand language queries over the Language Server Protocol.
package lsp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	Path     string
	Position Position
}

// PositionAt and Offset use UTF-16 code units, the protocol's default encoding.
func PositionAt(text []rune, cursor int) Position {
	var p Position
	for _, r := range text[:min(max(0, cursor), len(text))] {
		if r == '\n' {
			p.Line++
			p.Character = 0
		} else {
			p.Character++
			if r > 0xffff {
				p.Character++
			}
		}
	}
	return p
}

func Offset(text []rune, p Position) int {
	i := 0
	for row := 0; row < p.Line && i < len(text); i++ {
		if text[i] == '\n' {
			row++
		}
	}
	col := 0
	for i < len(text) && text[i] != '\n' && col < p.Character {
		width := 1
		if text[i] > 0xffff {
			width = 2
		}
		if col+width > p.Character {
			break
		}
		col += width
		i++
	}
	return i
}

func FileURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func FilePath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return "", fmt.Errorf("unsupported definition URI: %s", uri)
	}
	path := u.Path
	if runtime.GOOS == "windows" && len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	if !filepath.IsAbs(filepath.FromSlash(path)) {
		return "", fmt.Errorf("definition path must be absolute")
	}
	return filepath.FromSlash(path), nil
}

func locations(raw json.RawMessage) ([]Location, error) {
	if string(raw) == "null" || len(raw) == 0 {
		return nil, nil
	}
	type target struct {
		URI                  string `json:"uri"`
		Range                Range  `json:"range"`
		TargetURI            string `json:"targetUri"`
		TargetSelectionRange Range  `json:"targetSelectionRange"`
	}
	var targets []target
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &targets); err != nil {
			return nil, err
		}
	} else {
		var t target
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	var result []Location
	seen := make(map[Location]bool)
	for _, t := range targets {
		uri, p := t.URI, t.Range.Start
		if t.TargetURI != "" {
			uri, p = t.TargetURI, t.TargetSelectionRange.Start
		}
		path, err := FilePath(uri)
		if err != nil {
			return nil, err
		}
		if p.Line < 0 || p.Character < 0 {
			return nil, fmt.Errorf("invalid definition position")
		}
		location := Location{path, p}
		if !seen[location] {
			result = append(result, location)
			seen[location] = true
		}
	}
	return result, nil
}

func content(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		var parts []string
		for _, item := range list {
			if text := content(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n\n")
	}
	var markup struct{ Value string }
	_ = json.Unmarshal(raw, &markup)
	return markup.Value
}
