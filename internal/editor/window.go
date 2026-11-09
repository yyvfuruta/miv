package editor

// Window keeps a buffer and cursor independent of other views of the same file.
type Window struct {
	Buffer *Buffer
	Cursor int
}
type windowNode struct {
	window        *Window
	first, second *windowNode
	vertical      bool
}
type windowState struct {
	root          *windowNode
	active        *Window
	width, height int
}
type WindowView struct {
	Window              *Window
	X, Y, Width, Height int
	Active              bool
}

func (e *Editor) ensureWindows() {
	if e.windows.root == nil {
		w := &Window{Buffer: e.Buffer(), Cursor: e.Buffer().Cursor}
		e.windows.root = &windowNode{window: w}
		e.windows.active = w
	}
	e.syncWindow()
}
func (e *Editor) syncWindow() {
	if w := e.windows.active; w != nil {
		w.Buffer = e.Buffer()
		w.Cursor = e.Buffer().Cursor
	}
}
func (e *Editor) activateWindow(w *Window) {
	for i, b := range e.Buffers {
		if b == w.Buffer {
			e.Active = i
			break
		}
	}
	e.windows.active = w
	e.Buffer().Cursor = min(w.Cursor, len(e.Buffer().Text))
	e.Buffer().NormalCursor()
	e.desiredCol = -1
}
func (e *Editor) WindowLayout(width, height int) []WindowView {
	e.ensureWindows()
	e.windows.width, e.windows.height = max(1, width), max(1, height)
	var result []WindowView
	var walk func(*windowNode, int, int, int, int)
	walk = func(n *windowNode, x, y, w, h int) {
		if n == nil || w <= 0 || h <= 0 {
			return
		}
		if n.window != nil {
			result = append(result, WindowView{n.window, x, y, w, h, n.window == e.windows.active})
			return
		}
		if n.vertical {
			a := (w - 1) / 2
			walk(n.first, x, y, a, h)
			walk(n.second, x+a+1, y, w-a-1, h)
		} else {
			a := (h - 1) / 2
			walk(n.first, x, y, w, a)
			walk(n.second, x, y+a+1, w, h-a-1)
		}
	}
	walk(e.windows.root, 0, 0, max(1, width), max(1, height))
	return result
}
func (e *Editor) windowLayout() []WindowView {
	width, height := e.windows.width, e.windows.height
	if width == 0 {
		width = 80
	}
	if height == 0 {
		height = 21
	}
	return e.WindowLayout(width, height)
}
func (e *Editor) SplitWindow(vertical bool) bool {
	e.ensureWindows()
	visible := false
	for _, r := range e.windowLayout() {
		if r.Active {
			visible = true
		}
		if r.Active && ((vertical && r.Width < 8) || (!vertical && r.Height < 4)) {
			e.Message = "Window is too small to split"
			return false
		}
	}
	if !visible {
		e.Message = "Window is too small to split"
		return false
	}
	old := e.windows.active
	next := &Window{Buffer: old.Buffer, Cursor: old.Cursor}
	var split func(*windowNode)
	split = func(n *windowNode) {
		if n.window == old {
			n.window = nil
			n.vertical = vertical
			n.first = &windowNode{window: old}
			n.second = &windowNode{window: next}
			return
		}
		if n.first != nil {
			split(n.first)
		}
		if n.second != nil {
			split(n.second)
		}
	}
	split(e.windows.root)
	e.activateWindow(next)
	e.Message = "Split created"
	return true
}

func absDistance(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func (e *Editor) FocusWindow(direction string) bool {
	layout := e.windowLayout()
	var current WindowView
	for _, r := range layout {
		if r.Active {
			current = r
			break
		}
	}
	var best *Window
	bestScore := int(^uint(0) >> 1)
	for _, r := range layout {
		if r.Active {
			continue
		}
		primary, secondary := 0, 0
		switch direction {
		case "h":
			if r.X+r.Width > current.X {
				continue
			}
			primary = current.X - r.X - r.Width
			secondary = absDistance(2*r.Y + r.Height - 2*current.Y - current.Height)
		case "l":
			if r.X < current.X+current.Width {
				continue
			}
			primary = r.X - current.X - current.Width
			secondary = absDistance(2*r.Y + r.Height - 2*current.Y - current.Height)
		case "k":
			if r.Y+r.Height > current.Y {
				continue
			}
			primary = current.Y - r.Y - r.Height
			secondary = absDistance(2*r.X + r.Width - 2*current.X - current.Width)
		case "j":
			if r.Y < current.Y+current.Height {
				continue
			}
			primary = r.Y - current.Y - current.Height
			secondary = absDistance(2*r.X + r.Width - 2*current.X - current.Width)
		default:
			return false
		}
		score := primary*10000 + secondary
		if score < bestScore {
			bestScore = score
			best = r.Window
		}
	}
	if best == nil {
		return false
	}
	e.activateWindow(best)
	return true
}

func firstWindow(n *windowNode) *Window {
	if n.window != nil {
		return n.window
	}
	return firstWindow(n.first)
}
func (e *Editor) CloseWindow() bool {
	e.ensureWindows()
	if e.windows.root.window != nil {
		return false
	}
	target := e.windows.active
	var remove func(*windowNode) *windowNode
	remove = func(n *windowNode) *windowNode {
		if n.window == target {
			return nil
		}
		if n.window != nil {
			return n
		}
		n.first = remove(n.first)
		n.second = remove(n.second)
		if n.first == nil {
			return n.second
		}
		if n.second == nil {
			return n.first
		}
		return n
	}
	e.windows.root = remove(e.windows.root)
	e.activateWindow(firstWindow(e.windows.root))
	e.Message = "Window closed"
	return true
}
func windowIn(n *windowNode, w *Window) bool {
	return n != nil && (n.window == w || windowIn(n.first, w) || windowIn(n.second, w))
}
func (e *Editor) WindowExists(w *Window) bool {
	if windowIn(e.windows.root, w) {
		return true
	}
	for id, state := range e.workspaces {
		if id != e.Workspace && windowIn(state.windows.root, w) {
			return true
		}
	}
	return false
}
func (e *Editor) replaceWindowBuffer(removed *Buffer) {
	var walk func(*windowNode)
	walk = func(n *windowNode) {
		if n == nil {
			return
		}
		if n.window != nil && n.window.Buffer == removed {
			n.window.Buffer = e.Buffer()
			n.window.Cursor = min(n.window.Cursor, len(e.Buffer().Text))
		}
		walk(n.first)
		walk(n.second)
	}
	walk(e.windows.root)
}
