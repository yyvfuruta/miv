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

	"golang.org/x/sys/unix"
)

type capture struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data.Write(p)
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
	master  *os.File
	output  capture
	done    chan struct{}
	exitErr error // Read only after done closes.
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

func (t *terminalTest) send(text string, delay time.Duration) error {
	if _, err := io.WriteString(t.master, text); err != nil {
		return err
	}
	time.Sleep(delay)
	return nil
}
func (t *terminalTest) waitFor(marker string, start int) error {
	deadline := time.Now().Add(8 * time.Second)
	for !strings.Contains(t.output.from(start), marker) {
		select {
		case <-t.done:
			return fmt.Errorf("editor exited: %v; output=%q", t.exitErr, t.output.tail())
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("missing output %q; output=%q", marker, t.output.tail())
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}
func (t *terminalTest) shell(command, reply string, interrupt bool) error {
	start := t.output.length()
	if err := t.send(":!"+command+"\r", 200*time.Millisecond); err != nil {
		return err
	}
	if reply != "" {
		if err := t.send(reply+"\n", 100*time.Millisecond); err != nil {
			return err
		}
	}
	if interrupt {
		if err := t.send("\x03", 100*time.Millisecond); err != nil {
			return err
		}
	}
	if err := t.waitFor("Press Enter to return to miv...", start); err != nil {
		return err
	}
	return t.send("\r", 200*time.Millisecond)
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
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "WAYLAND_DISPLAY=", "DISPLAY=", "SHELL="+shell)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	t := &terminalTest{master: master, done: make(chan struct{})}
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
	time.Sleep(500 * time.Millisecond)
	// Directory startup must open a picker; choose a file before editing.
	for _, step := range []struct {
		text  string
		delay time.Duration
	}{
		{"smoke.txt\r", 200 * time.Millisecond},
		{"iHello, café!", 200 * time.Millisecond},
		{"\x1b[200~\npasted without indentation\x1b[201~", 200 * time.Millisecond},
		{"\x1b", 150 * time.Millisecond},
	} {
		if err = t.send(step.text, step.delay); err != nil {
			return err
		}
	}
	if err = t.shell("printf 'hello\\n' | tr a-z A-Z > shell-output.txt", "", false); err != nil {
		return err
	}
	if err = checkFile(filepath.Join(directory, "shell-output.txt"), "HELLO\n"); err != nil {
		return err
	}
	if err = t.shell(`read answer; printf "%s" "$answer" > input.txt`, "typed answer", false); err != nil {
		return err
	}
	if err = checkFile(filepath.Join(directory, "input.txt"), "typed answer"); err != nil {
		return err
	}
	if err = t.shell("exit 7", "", false); err != nil {
		return err
	}
	if err = t.shell("sleep 10", "", true); err != nil {
		return err
	}
	terminalStart := t.output.length()
	if err = t.send(":term\r", 300*time.Millisecond); err != nil {
		return err
	}
	if strings.Contains(t.output.from(terminalStart), "\x1b[?1049l") {
		return fmt.Errorf(":term left the editor screen")
	}
	if err = t.send("printf interactive > term-output.txt\n", 200*time.Millisecond); err != nil {
		return err
	}
	if err = checkFile(filepath.Join(directory, "term-output.txt"), "interactive"); err != nil {
		return err
	}
	if err = t.send("\x1c\x0e", 150*time.Millisecond); err != nil {
		return err
	}
	if err = t.send(":.!tr a-z A-Z\r", 200*time.Millisecond); err != nil {
		return err
	}
	if err = t.send(":w\r", 100*time.Millisecond); err != nil {
		return err
	}
	if err = checkFile(path, "Hello, café!\nPASTED WITHOUT INDENTATION\n"); err != nil {
		return fmt.Errorf("line filter: %w", err)
	}
	if err = t.send("u", 100*time.Millisecond); err != nil {
		return err
	}
	if err = t.send(":r !printf 'added\\n'\r", 200*time.Millisecond); err != nil {
		return err
	}
	if err = t.send(":w\r", 100*time.Millisecond); err != nil {
		return err
	}
	if err = checkFile(path, "Hello, café!\npasted without indentation\nadded\n"); err != nil {
		return fmt.Errorf("read shell output: %w", err)
	}
	if err = t.send("u:term\r", 200*time.Millisecond); err != nil {
		return err
	}
	if err = t.send("exit\n", 300*time.Millisecond); err != nil {
		return err
	}
	if err = t.send(":term printf command > term-command.txt\r", 300*time.Millisecond); err != nil {
		return err
	}
	if err = checkFile(filepath.Join(directory, "term-command.txt"), "command"); err != nil {
		return err
	}
	// Keep a dirty W2, verify deletion refusal, and create/delete a clean W3.
	for _, step := range []struct {
		text  string
		delay time.Duration
	}{
		{"A!", 100 * time.Millisecond}, {"\x1b", 150 * time.Millisecond},
		{" \tn", 100 * time.Millisecond}, {"iworkspace two", 100 * time.Millisecond}, {"\x1b", 150 * time.Millisecond},
		{":wsdelete\r", 100 * time.Millisecond},
		{" \tn", 100 * time.Millisecond}, {" \td", 100 * time.Millisecond},
		{" \t1 fs", 200 * time.Millisecond}, {":qa\r", 200 * time.Millisecond},
	} {
		if err = t.send(step.text, step.delay); err != nil {
			return err
		}
	}
	select {
	case <-t.done:
		return fmt.Errorf("quit discarded a hidden dirty workspace: %v", t.exitErr)
	default:
	}
	if err = t.send(":qa!\r", 0); err != nil {
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
