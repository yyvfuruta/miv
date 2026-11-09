package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
	"golang.org/x/sys/unix"
)

var errOutputLimit = errors.New("command output exceeds the size limit")

type boundedOutput struct {
	bytes.Buffer
	limit  int
	cancel context.CancelFunc
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		b.cancel()
		return 0, errOutputLimit
	}
	return b.Buffer.Write(p)
}

func captureShell(screen tcell.Screen, command, input, directory string, deferEvent func(tcell.Event)) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", command)
	cmd.Dir = directory
	cmd.Stdin = strings.NewReader(input)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
		if err == unix.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 500 * time.Millisecond
	output := &boundedOutput{limit: 16 << 20, cancel: cancel}
	diagnostic := &boundedOutput{limit: 64 << 10, cancel: cancel}
	cmd.Stdout, cmd.Stderr = output, diagnostic
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		err := cmd.Run()
		if err != nil {
			if detail := strings.TrimSpace(diagnostic.String()); detail != "" {
				err = fmt.Errorf("%w: %s", err, detail)
			}
			if ctx.Err() != nil {
				err = errors.New("command canceled or output limit reached")
			}
		}
		done <- result{output.String(), err}
		_ = screen.PostEvent(tcell.NewEventInterrupt(nil))
	}()
	w, h := screen.Size()
	if h > 0 {
		fill(screen, h-1, w, base)
		put(screen, 0, h-1, w, "Running command... Ctrl-C or Esc cancels", muted)
		screen.Show()
	}
	for {
		select {
		case r := <-done:
			return r.text, r.err
		default:
		}
		switch ev := screen.PollEvent().(type) {
		case *tcell.EventKey:
			if ev.Key() == tcell.KeyCtrlC || ev.Key() == tcell.KeyEscape {
				cancel()
			} else if deferEvent != nil {
				deferEvent(ev)
			}
		case *tcell.EventPaste:
			if deferEvent != nil {
				deferEvent(ev)
			}
		case nil:
			cancel()
		case *tcell.EventResize:
			screen.Sync()
		}
	}
}
