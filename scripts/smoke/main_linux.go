// Command smoke exercises the compiled editor in a real Linux pseudoterminal.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/hinshun/vt10x"

	"golang.org/x/sys/unix"
)

type capture struct {
	mu      sync.Mutex
	data    bytes.Buffer
	screen  vt10x.Terminal
	pending []byte
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n, err := c.data.Write(p)
	c.pending = append(c.pending, p...)
	complete := 0
	for complete < len(c.pending) && utf8.FullRune(c.pending[complete:]) {
		_, size := utf8.DecodeRune(c.pending[complete:])
		complete += size
	}
	if complete > 0 {
		_, _ = c.screen.Write(c.pending[:complete])
		c.pending = append([]byte(nil), c.pending[complete:]...)
	}
	return n, err
}
func (c *capture) length() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data.Len()
}
func (c *capture) from(start int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data.String()[start:]
}
func (c *capture) tail() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	data := c.data.String()
	return data[max(0, len(data)-1500):]
}

type terminalTest struct {
	master       *os.File
	output       capture
	done         chan struct{}
	exitErr      error // Read only after done closes.
	terminalOpen bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "smoke:", err)
		os.Exit(1)
	}
	fmt.Println("PTY smoke passed: editing, paste, embedded terminal, shell filters, Ctrl-C, directory picker, workspaces, terminal restoration")
}

func openPTY() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	fd := int(master.Fd())
	if err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
		master.Close()
		return nil, nil, err
	}
	number, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err != nil {
		master.Close()
		return nil, nil, err
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}

func (t *terminalTest) send(text string) error {
	_, err := io.WriteString(t.master, text)
	return err
}
func (t *terminalTest) until(description string, condition func() bool) error {
	deadline := time.Now().Add(8 * time.Second)
	for !condition() {
		select {
		case <-t.done:
			return fmt.Errorf("editor exited waiting for %s: %v; screen=%q", description, t.exitErr, t.output.screen.String())
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for %s; screen=%q; output=%q", description, t.output.screen.String(), t.output.tail())
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}
func (t *terminalTest) waitFor(marker string, start int) error {
	return t.until("output "+marker, func() bool { return strings.Contains(t.output.from(start), marker) })
}
func (t *terminalTest) screenContains(markers ...string) error {
	return t.until("screen "+strings.Join(markers, ", "), func() bool {
		text := t.output.screen.String()
		for _, marker := range markers {
			if !strings.Contains(text, marker) {
				return false
			}
		}
		return true
	})
}
func (t *terminalTest) editorReady() error {
	return t.until("normal-mode editor focus", func() bool {
		text := t.output.screen.String()
		return strings.Contains(text, " NORMAL ") && (!t.terminalOpen || strings.Contains(text, " EDITOR "))
	})
}
func (t *terminalTest) file(path, want string) error {
	return t.until("file "+filepath.Base(path)+" = "+fmt.Sprintf("%q", want), func() bool { return checkFile(path, want) == nil })
}
func shellReadyCommand(command string) string {
	var escaped strings.Builder
	for _, r := range "MIV_SMOKE_SHELL_READY\n" {
		fmt.Fprintf(&escaped, "\\%03o", r)
	}
	return "printf '" + escaped.String() + "'; " + command
}
func (t *terminalTest) shell(command, reply string, interrupt bool) error {
	start := t.output.length()
	if reply != "" || interrupt {
		command = shellReadyCommand(command)
	}
	if err := t.send(":!" + command + "\r"); err != nil {
		return err
	}
	if reply != "" || interrupt {
		if err := t.waitFor("MIV_SMOKE_SHELL_READY", start); err != nil {
			return err
		}
	}
	if reply != "" {
		if err := t.send(reply + "\n"); err != nil {
			return err
		}
	}
	if interrupt {
		if err := t.send("\x03"); err != nil {
			return err
		}
	}
	if err := t.waitFor("Press Enter to return to miv...", start); err != nil {
		return err
	}
	if err := t.send("\r"); err != nil {
		return err
	}
	return t.editorReady()
}
func checkFile(path, want string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if string(data) != want {
		return fmt.Errorf("%s: content=%q, want=%q", filepath.Base(path), data, want)
	}
	return nil
}

func run() error {
	binary, err := filepath.Abs("bin/miv")
	if err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "miv-smoke-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	// Bash's system startup file may override PS1; make its final prompt stable.
	if err = os.WriteFile(filepath.Join(directory, ".bashrc"), []byte("PS1='MIV_SMOKE_PROMPT> '\n"), 0600); err != nil {
		return err
	}
	path := filepath.Join(directory, "smoke.txt")
	if err = os.WriteFile(path, nil, 0644); err != nil {
		return err
	}
	master, slave, err := openPTY()
	if err != nil {
		return err
	}
	defer master.Close()
	defer slave.Close()
	if err = unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 80}); err != nil {
		return err
	}
	before, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		return err
	}
	shell := os.Getenv("MIV_TEST_SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(binary, ".")
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "WAYLAND_DISPLAY=", "DISPLAY=", "SHELL="+shell, "HOME="+directory, "PS1=MIV_SMOKE_PROMPT> ")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	t := &terminalTest{master: master, done: make(chan struct{}), output: capture{screen: vt10x.New(vt10x.WithSize(80, 24))}}
	if err = cmd.Start(); err != nil {
		return err
	}
	go func() { t.exitErr = cmd.Wait(); close(t.done) }()
	copied := make(chan struct{})
	stopReader := make(chan struct{})
	masterFD := int(master.Fd())
	go func() {
		defer close(copied)
		var buffer [65536]byte
		for {
			select {
			case <-stopReader:
				return
			default:
			}
			ready := []unix.PollFd{{Fd: int32(masterFD), Events: unix.POLLIN}}
			n, err := unix.Poll(ready, 100)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				return
			}
			if n == 0 {
				continue
			}
			count, err := unix.Read(masterFD, buffer[:])
			if count > 0 {
				_, _ = t.output.Write(buffer[:count])
			}
			if err != nil || count == 0 {
				return
			}
		}
	}()
	defer func() {
		select {
		case <-t.done:
		default:
			_ = cmd.Process.Kill()
			<-t.done
		}
		close(stopReader)
		<-copied
		_ = master.Close()
	}()

	// Wait for rendered states and file contents, not host-speed-dependent sleeps.
	if err = t.screenContains("Files ("); err != nil {
		return err
	}
	if err = t.send("smoke.txt\r"); err != nil {
		return err
	}
	if err = t.editorReady(); err != nil {
		return err
	}
	if err = t.send("iHello, café!"); err != nil {
		return err
	}
	if err = t.screenContains(" INSERT ", "Hello, café!"); err != nil {
		return err
	}
	if err = t.send("\x1b[200~\npasted without indentation\x1b[201~"); err != nil {
		return err
	}
	if err = t.screenContains("pasted without indentation"); err != nil {
		return err
	}
	if err = t.send("\x1b"); err != nil {
		return err
	}
	if err = t.editorReady(); err != nil {
		return err
	}
	if err = t.shell("printf 'hello\\n' | tr a-z A-Z > shell-output.txt", "", false); err != nil {
		return err
	}
	if err = t.file(filepath.Join(directory, "shell-output.txt"), "HELLO\n"); err != nil {
		return err
	}
	if err = t.shell(`read answer; printf "%s" "$answer" > input.txt`, "typed answer", false); err != nil {
		return err
	}
	if err = t.file(filepath.Join(directory, "input.txt"), "typed answer"); err != nil {
		return err
	}
	if err = t.shell("exit 7", "", false); err != nil {
		return err
	}
	if err = t.shell("sleep 10", "", true); err != nil {
		return err
	}
	terminalStart := t.output.length()
	t.terminalOpen = true
	if err = t.send(":term\r"); err != nil {
		return err
	}
	if err = t.screenContains(" TERMINAL ", "MIV_SMOKE_PROMPT>"); err != nil {
		return err
	}
	if strings.Contains(t.output.from(terminalStart), "\x1b[?1049l") {
		return fmt.Errorf(":term left the editor screen")
	}
	if err = t.send("printf interactive > term-output.txt\n"); err != nil {
		return err
	}
	if err = t.file(filepath.Join(directory, "term-output.txt"), "interactive"); err != nil {
		return err
	}
	if err = t.send("\x1c\x0e"); err != nil {
		return err
	}
	if err = t.editorReady(); err != nil {
		return err
	}
	// Deliberately type :w before the slow filter finishes: input must be preserved.
	if err = t.send(":.!sleep 0.35; tr a-z A-Z\r:w\r"); err != nil {
		return err
	}
	if err = t.file(path, "Hello, café!\nPASTED WITHOUT INDENTATION\n"); err != nil {
		return fmt.Errorf("line filter: %w", err)
	}
	if err = t.send("u"); err != nil {
		return err
	}
	if err = t.screenContains("pasted without indentation"); err != nil {
		return err
	}
	if err = t.send(":r !sleep 0.35; printf 'added\\n'\r:w\r"); err != nil {
		return err
	}
	if err = t.file(path, "Hello, café!\npasted without indentation\nadded\n"); err != nil {
		return fmt.Errorf("read shell output: %w", err)
	}
	if err = t.send("u:term\r"); err != nil {
		return err
	}
	if err = t.screenContains(" TERMINAL ", "MIV_SMOKE_PROMPT>"); err != nil {
		return err
	}
	if err = t.send("exit\n"); err != nil {
		return err
	}
	if err = t.editorReady(); err != nil {
		return err
	}
	if err = t.send(":term printf command > term-command.txt\r"); err != nil {
		return err
	}
	if err = t.file(filepath.Join(directory, "term-command.txt"), "command"); err != nil {
		return err
	}
	if err = t.editorReady(); err != nil {
		return err
	}
	if err = t.send("A!"); err != nil {
		return err
	}
	if err = t.screenContains(" INSERT ", "pasted without indentation!"); err != nil {
		return err
	}
	if err = t.send("\x1b"); err != nil {
		return err
	}
	if err = t.editorReady(); err != nil {
		return err
	}
	if err = t.send(" \tn"); err != nil {
		return err
	}
	if err = t.screenContains("W2"); err != nil {
		return err
	}
	if err = t.send("iworkspace two"); err != nil {
		return err
	}
	if err = t.screenContains(" INSERT ", "workspace two"); err != nil {
		return err
	}
	if err = t.send("\x1b"); err != nil {
		return err
	}
	if err = t.editorReady(); err != nil {
		return err
	}
	if err = t.send(":wsdelete\r"); err != nil {
		return err
	}
	if err = t.screenContains("Workspace 2 has unsaved changes"); err != nil {
		return err
	}
	if err = t.send(" \tn"); err != nil {
		return err
	}
	if err = t.screenContains("W3"); err != nil {
		return err
	}
	if err = t.send(" \td"); err != nil {
		return err
	}
	if err = t.screenContains("W1"); err != nil {
		return err
	}
	if err = t.send(" fs"); err != nil {
		return err
	}
	if err = t.file(path, "Hello, café!\npasted without indentation!"); err != nil {
		return err
	}
	if err = t.send(":qa\r"); err != nil {
		return err
	}
	if err = t.screenContains("Unsaved changes; use :qa!"); err != nil {
		return err
	}
	if err = t.send(":qa!\r"); err != nil {
		return err
	}
	select {
	case <-t.done:
	case <-time.After(8 * time.Second):
		return fmt.Errorf("editor did not exit; output=%q", t.output.tail())
	}
	if t.exitErr != nil {
		return fmt.Errorf("editor failed: %w; output=%q", t.exitErr, t.output.tail())
	}
	if err = checkFile(path, "Hello, café!\npasted without indentation!"); err != nil {
		return err
	}
	after, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		return err
	}
	if *after != *before {
		return fmt.Errorf("terminal settings were not restored")
	}
	return nil
}
