package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/yyvfuruta/vim/internal/editor"
)

type fileViewport struct {
	top, left int
	last      *editor.Buffer
	syntax    syntaxCache
}

func (v *view) focusWindow(direction string) {
	if v.e.FocusWindow(direction) {
		if v.terminal != nil {
			v.terminal.focused = false
		}
		return
	}
	if direction == "j" && v.terminal != nil {
		select {
		case <-v.terminal.done:
			v.e.Message = "Terminal has exited; use :term to start a shell"
			return
		default:
		}
		v.terminal.focused = true
		v.terminal.escapePrefix = false
	}
}

func (v *view) drawWindows(width, height int) {
	if v.panes == nil {
		v.panes = make(map[*editor.Window]*fileViewport)
	}
	layout := v.e.WindowLayout(width, height)
	for window := range v.panes {
		if !v.e.WindowExists(window) {
			delete(v.panes, window)
		}
	}
	v.screen.HideCursor()
	for _, region := range layout {
		if region.X > 0 {
			for y := region.Y + 1; y <= region.Y+region.Height; y++ {
				v.screen.SetContent(region.X-1, y, '│', nil, muted)
			}
		}
		if region.Y > 0 {
			for x := region.X; x < region.X+region.Width; x++ {
				v.screen.SetContent(x, region.Y, '─', nil, muted)
			}
		}
		state := v.panes[region.Window]
		if state == nil {
			state = &fileViewport{}
			v.panes[region.Window] = state
		}
		v.drawFileWindow(region, state, len(layout) > 1)
	}
}

func (v *view) drawFileWindow(region editor.WindowView, state *fileViewport, titled bool) {
	s, e := v.screen, v.e
	original := region.Window.Buffer
	display := *original
	display.Cursor = min(max(0, region.Window.Cursor), len(display.Text))
	b := &display
	if state.last != original {
		state.top, state.left = 0, 0
		state.last = original
	}
	row, col := b.RowCol()
	text := string(b.Text)
	state.syntax.update(b.Path, text)
	lines := strings.Split(text, "\n")
	width, height := region.Width, region.Height
	if titled && height > 1 {
		style := bar
		if region.Active {
			style = accent
		}
		y := region.Y + height
		for x := region.X; x < region.X+width; x++ {
			s.SetContent(x, y, ' ', nil, style)
		}
		name := filepath.Base(b.Path)
		if b.Path == "" {
			name = "[new]"
		}
		if b.Dirty() {
			name += " +"
		}
		put(s, region.X, y, region.X+width, " "+name, style)
		height--
	}
	margin := min(5, height/3)
	if row < state.top+margin {
		state.top = max(0, row-margin)
	}
	if row >= state.top+height-margin {
		state.top = max(0, row-height+margin+1)
	}
	gutter := len(fmt.Sprint(len(lines))) + 2
	contentW := max(1, width-gutter)
	tab := 4
	switch strings.ToLower(filepath.Ext(b.Path)) {
	case ".yaml", ".yml", ".sh", ".bash", ".json", ".jsonc":
		tab = 2
	}
	displayCol := cellCol([]rune(lines[row]), col, tab)
	if displayCol < state.left {
		state.left = displayCol
	}
	if displayCol >= state.left+contentW {
		state.left = displayCol - contentW + 1
	}
	start, end := -1, -1
	if region.Active && (e.Mode == editor.Visual || e.Mode == editor.VisualLine) {
		start, end = min(e.Anchor, b.Cursor), max(e.Anchor, b.Cursor)+1
		if e.Mode == editor.VisualLine {
			start = b.LineStart(start)
			end = b.LineEnd(end-1) + 1
		}
	}
	offset := 0
	for i := 0; i < state.top && i < len(lines); i++ {
		offset += len([]rune(lines[i])) + 1
	}
	tokenIndex := state.syntax.firstSpan(offset)
	right := region.X + width
	for localY := 0; localY < height; localY++ {
		y := region.Y + localY + 1
		index := state.top + localY
		if index >= len(lines) {
			put(s, region.X, y, right, "~", muted)
			continue
		}
		put(s, region.X, y, right, fmt.Sprintf("%*d ", gutter-1, index+1), muted)
		cells := 0
		runes := []rune(lines[index])
		lastX := -1
		for j, r := range runes {
			style := state.syntax.styleAt(offset+j, &tokenIndex)
			if offset+j >= start && offset+j < end {
				style = selected
			}
			cellWidth := runewidth.RuneWidth(safeRune(r))
			if r == '\t' {
				cellWidth = tab - cells%tab
			}
			x := region.X + gutter + cells - state.left
			if cellWidth == 0 && lastX >= region.X+gutter {
				main, comb, st, _ := s.GetContent(lastX, y)
				s.SetContent(lastX, y, main, append(comb, r), st)
				continue
			}
			if x >= region.X+gutter && x+cellWidth <= right {
				if r == '\t' {
					for k := 0; k < cellWidth; k++ {
						s.SetContent(x+k, y, ' ', nil, style)
					}
				} else {
					s.SetContent(x, y, safeRune(r), nil, style)
					lastX = x
				}
			}
			cells += cellWidth
		}
		if offset+len(runes) >= start && offset+len(runes) < end {
			x := region.X + gutter + cells - state.left
			if x >= region.X+gutter && x < right {
				s.SetContent(x, y, ' ', nil, selected)
			}
		}
		offset += len(runes) + 1
	}
	if region.Active {
		x, y := region.X+gutter+displayCol-state.left, region.Y+row-state.top+1
		if x < right && y >= region.Y+1 && y <= region.Y+height {
			s.ShowCursor(x, y)
		}
	}
}
