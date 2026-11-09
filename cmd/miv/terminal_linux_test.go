package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
	"github.com/yyvfuruta/vim/internal/editor"
)

func terminalText(vt vt10x.Terminal) string { return vt.String() }
func waitTerminal(t *testing.T, pane *terminalPane, needle string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(terminalText(pane.vt), needle) {
		if time.Now().After(deadline) {
			t.Fatalf("missing %q in %q", needle, terminalText(pane.vt))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEmbeddedTerminalProcessAndResize(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	e, _ := editor.New(nil)
	v := view{screen: screen, e: e}
	if err := v.openTerminal("stty size; read value; stty size; printf 'received:%s' \"$value\"", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if v.terminal != nil {
			_ = v.closeTerminal()
		}
	}()
	waitTerminal(t, v.terminal, "10 80")
	screen.SetSize(100, 30)
	v.draw()
	for _, r := range "hello" {
		if err := v.terminal.key(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)); err != nil {
			t.Fatal(err)
		}
	}
	_ = v.terminal.key(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	select {
	case <-v.terminal.done:
	case <-time.After(3 * time.Second):
		t.Fatal("terminal did not exit")
	}
	text := terminalText(v.terminal.vt)
	if !strings.Contains(text, "13 100") || !strings.Contains(text, "received:hello") {
		t.Fatalf("terminal I/O or resize failed: %q", text)
	}
	v.draw()
	if v.terminal.focused {
		t.Fatal("exited terminal kept keyboard focus")
	}
	if err := v.openTerminal("sleep 10", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := v.terminal.key(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-v.terminal.done:
	case <-time.After(3 * time.Second):
		t.Fatal("Ctrl-C failed to interrupt terminal command")
	}
}

func TestCaptureShellInputErrorsAndCancellation(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	text, err := captureShell(screen, "tr a-z A-Z", "hello\n", t.TempDir(), nil)
	if err != nil || text != "HELLO\n" {
		t.Fatalf("capture: %q %v", text, err)
	}
	_, err = captureShell(screen, "printf error >&2; exit 7", "", t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "error") {
		t.Fatalf("stderr not reported: %v", err)
	}
	time.AfterFunc(50*time.Millisecond, func() { _ = screen.PostEvent(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone)) })
	start := time.Now()
	_, err = captureShell(screen, "sleep 10", "", t.TempDir(), nil)
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatalf("capture was not canceled: %v", err)
	}
}

func TestCapturePreservesTypeAhead(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	keys := []*tcell.EventKey{
		tcell.NewEventKey(tcell.KeyRune, ':', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyRune, 'w', tcell.ModNone),
		tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone),
	}
	for _, key := range keys {
		if err := screen.PostEvent(key); err != nil {
			t.Fatal(err)
		}
	}
	var pending []tcell.Event
	text, err := captureShell(screen, "printf done", "", t.TempDir(), func(event tcell.Event) { pending = append(pending, event) })
	if err != nil || text != "done" {
		t.Fatalf("capture: %q %v", text, err)
	}
	// Also collect any keys still in tcell's queue: both locations preserve input.
	for screen.HasPendingEvent() {
		if event := screen.PollEvent(); event != nil {
			if _, ok := event.(*tcell.EventKey); ok {
				pending = append(pending, event)
			}
		}
	}
	if len(pending) != len(keys) {
		t.Fatalf("lost type-ahead: %d events, want %d", len(pending), len(keys))
	}
	for i, event := range pending {
		if event != keys[i] {
			t.Fatalf("type-ahead order changed at %d", i)
		}
	}
}
