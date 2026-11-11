package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/yyvfuruta/vim/internal/editor"
	"github.com/yyvfuruta/vim/internal/lsp"
)

type languageReply struct {
	id        int
	query     lsp.Query
	buffer    *editor.Buffer
	cursor    int
	workspace int
	result    lsp.Result
	err       error
}

type languageController struct {
	e       *editor.Editor
	manager lsp.Manager
	wake    func()
	results chan languageReply
	life    context.Context
	stop    context.CancelFunc
	cancel  context.CancelFunc
	id      int
	busy    bool
}

func newLanguageController(e *editor.Editor, wake func()) *languageController {
	ctx, stop := context.WithCancel(context.Background())
	return &languageController{e: e, wake: wake, results: make(chan languageReply, 1), life: ctx, stop: stop}
}

func (c *languageController) cancelRequest() {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if c.busy {
		c.id++
		c.busy = false
		c.e.Message = "Language request canceled"
	}
}

func (c *languageController) close() {
	c.cancelRequest()
	c.stop()
	c.manager.Close()
}

func (c *languageController) request(action string) {
	c.cancelRequest()
	c.id++
	c.busy = true
	ctx, cancel := context.WithTimeout(c.life, 20*time.Second)
	c.cancel = cancel
	b := c.e.Buffer()
	reply := languageReply{id: c.id, query: c.e.LanguageQuery(action), buffer: b, cursor: b.Cursor, workspace: c.e.Workspace}
	c.e.Message = "Language: " + action + " (Esc cancels)"
	go func() {
		defer cancel()
		reply.result, reply.err = c.manager.Query(ctx, reply.query)
		if ctx.Err() == context.Canceled {
			return
		}
		select {
		case c.results <- reply:
			c.wake()
		case <-c.life.Done():
		}
	}()
}

func (c *languageController) apply() {
	for {
		select {
		case reply := <-c.results:
			if reply.id != c.id {
				continue
			}
			c.busy = false
			if c.e.Mode != editor.Normal || c.e.Workspace != reply.workspace || c.e.Buffer() != reply.buffer || c.e.Buffer().Cursor != reply.cursor {
				continue
			}
			// Also reject changes to another open file while the server was working.
			current := c.e.LanguageQuery(reply.query.Action)
			if !sameDocuments(current.Documents, reply.query.Documents) {
				continue
			}
			if reply.err != nil {
				c.e.Message = "Language: " + reply.err.Error()
				continue
			}
			c.e.ShowLanguageResult(reply.query.Action, reply.result)
		default:
			return
		}
	}
}

func sameDocuments(a, b []lsp.Document) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func infoLines(text string, width int) []string {
	var lines []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		var part strings.Builder
		used := 0
		for _, r := range strings.ReplaceAll(line, "\t", "    ") {
			r = safeRune(r)
			cells := runewidth.RuneWidth(r)
			if cells > 0 && used+cells > width && used > 0 {
				lines = append(lines, part.String())
				part.Reset()
				used = 0
			}
			part.WriteRune(r)
			used += cells
		}
		lines = append(lines, part.String())
	}
	return lines
}

func (v *view) drawInfo(w, h int) {
	e, s := v.e, v.screen
	lines := infoLines(e.InfoText, max(1, w-2))
	rows := max(0, h-2)
	e.InfoScroll = min(max(0, e.InfoScroll), max(0, len(lines)-rows))
	for y := 0; y < h; y++ {
		fill(s, y, w, base)
	}
	fill(s, 0, w, bar)
	put(s, 0, 0, w, " "+e.InfoTitle, bar)
	for y := 0; y < rows && y+e.InfoScroll < len(lines); y++ {
		put(s, 1, y+1, w, lines[y+e.InfoScroll], base)
	}
	if h > 1 {
		put(s, 0, h-1, w, fmt.Sprintf("j/k: scroll  Esc/q: close  %d/%d", e.InfoScroll+1, len(lines)), muted)
	}
	s.HideCursor()
}
