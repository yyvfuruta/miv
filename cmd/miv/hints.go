package main

import (
	"fmt"

	"github.com/mattn/go-runewidth"
)

func (v *view) drawWorkspaceBar(right int) int {
	list := v.e.WorkspaceList()
	labels := make([]string, len(list))
	total := 0
	active := 0
	for i, workspace := range list {
		label := fmt.Sprintf("W%d", workspace.ID)
		if workspace.Name != "" {
			label += ":" + workspace.Name
		}
		if workspace.Dirty {
			label += "+"
		}
		if workspace.Active {
			active = i
			label = "[" + label + "]"
		}
		labels[i] = " " + label + " "
		total += runewidth.StringWidth(labels[i])
	}
	// Keep the active workspace visible when the terminal cannot fit every tab.
	limit := max(0, right/2)
	if total > limit && len(list) > 0 {
		return put(v.screen, 0, 0, limit, labels[active], accent)
	}
	x := 0
	for i, workspace := range list {
		style := inactive
		if workspace.Active {
			style = accent
		}
		x = put(v.screen, x, 0, limit, labels[i], style)
	}
	return x
}

func (v *view) drawKeyHints(width, height int) {
	prefix, hints := v.e.KeyHints()
	if len(hints) == 0 || width < 2 || height < 4 {
		return
	}
	cellWidth := 1
	for _, hint := range hints {
		cellWidth = max(cellWidth, runewidth.StringWidth(hint.Key+"  "+hint.Description)+3)
	}
	columns := max(1, width/cellWidth)
	rows := (len(hints) + columns - 1) / columns
	bottom := height - 2
	top := max(1, bottom-rows-1)
	for y := top; y < bottom; y++ {
		fill(v.screen, y, width, accent)
	}
	put(v.screen, 1, top, width, prefix+"  • Esc cancels", accent)
	for i, hint := range hints {
		y := top + 1 + i/columns
		if y >= bottom {
			break
		}
		x := 1 + (i%columns)*cellWidth
		end := min(width, x+cellWidth-1)
		x = put(v.screen, x, y, end, hint.Key, accent.Bold(true))
		put(v.screen, x+2, y, end, hint.Description, accent)
	}
	v.screen.HideCursor()
}
