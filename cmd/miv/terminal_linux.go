package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
	"golang.org/x/sys/unix"
)

func (v *view) openTerminal(command, directory string) error {
	if old := v.terminal; old != nil {
		select {
		case <-old.done:
			_ = v.closeTerminal()
		default:
			if command != "" {
				return errors.New("a terminal is already running; use :termclose first")
			}
			old.focused = true
			old.escapePrefix = false
			return nil
		}
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	args := []string{"-i"}
	if command != "" {
		args = []string{"-c", command}
	}
	cmd := exec.Command(shell, args...)
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	width, height := v.screen.Size()
	_, _, rows := terminalGeometry(width, height)
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(max(1, width))})
	if err != nil {
		return err
	}
	fd := int(master.Fd())
	t := &terminalPane{directory: directory, focused: true}
	t.vt = vt10x.New(vt10x.WithSize(max(1, width), rows), vt10x.WithWriter(master))
	t.write = func(p []byte) (int, error) {
		select {
		case <-t.done:
			return 0, errors.New("terminal has exited; use :term to start a new shell")
		default:
		}
		return master.Write(p)
	}
	t.resize = func(cols, rows int) error {
		t.vt.Resize(cols, rows)
		return pty.Setsize(master, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	}
	done, childDone, readerDone, stopReader := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.done = done
	notify := func() { _ = v.screen.PostEvent(tcell.NewEventInterrupt(nil)) }
	go func() {
		defer close(readerDone)
		var buffer [32768]byte
		var pending []byte
		var modeTail string
		var exitedAt time.Time
		for {
			select {
			case <-stopReader:
				return
			default:
			}
			select {
			case <-childDone:
				if exitedAt.IsZero() {
					exitedAt = time.Now()
				}
				if time.Since(exitedAt) > 500*time.Millisecond {
					return
				}
			default:
			}
			ready := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			n, err := unix.Poll(ready, 100)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				return
			}
			if n == 0 {
				if !exitedAt.IsZero() {
					return
				}
				continue
			}
			count, err := unix.Read(fd, buffer[:])
			if count > 0 {
				modes := modeTail + string(buffer[:count])
				enabled, disabled := strings.LastIndex(modes, "\x1b[?2004h"), strings.LastIndex(modes, "\x1b[?2004l")
				if enabled >= 0 || disabled >= 0 {
					t.bracketedPaste.Store(enabled > disabled)
				}
				modeTail = modes[max(0, len(modes)-16):]
				pending = feedTerminal(t.vt, pending, buffer[:count])
				notify()
			}
			if err != nil || count == 0 {
				return
			}
		}
	}()
	go func() {
		err := cmd.Wait()
		close(childDone)
		<-readerDone
		if err != nil {
			t.exitMessage = err.Error()
		} else {
			t.exitMessage = "exited"
		}
		close(done)
		notify()
	}()
	var closeOnce sync.Once
	t.close = func() {
		closeOnce.Do(func() {
			select {
			case <-childDone:
			default:
				_ = unix.Kill(-cmd.Process.Pid, unix.SIGTERM)
				select {
				case <-childDone:
				case <-time.After(500 * time.Millisecond):
					_ = unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
					_ = cmd.Process.Kill()
				}
			}
			close(stopReader)
			<-readerDone
			_ = master.Close()
			<-done
		})
	}
	v.terminal = t
	return nil
}
