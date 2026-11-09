package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/gdamore/tcell/v2"
	"golang.org/x/sys/unix"
)

var errResumeScreen = errors.New("could not restore the editor terminal")

// Commands get the real terminal, including job control and interactive input.
// The editor keeps its buffers in memory while tcell is suspended.
func runShell(screen tcell.Screen, command string, interactive bool, directory string) (result error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer tty.Close()
	fd := int(tty.Fd())
	group, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP)
	if err != nil {
		return err
	}
	if err := screen.Suspend(); err != nil {
		return err
	}
	defer func() {
		if err := screen.Resume(); err != nil {
			result = errors.Join(result, fmt.Errorf("%w: %v", errResumeScreen, err))
		} else {
			screen.EnablePaste()
			screen.Sync()
		}
	}()
	settings, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return err
	}
	// Keep the editor alive if interrupted while displaying command output.
	// Notify leaves default signal handling in the child.
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGQUIT)
	defer signal.Stop(signals)
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	args := []string{"-c", command}
	if interactive && command == "" {
		args = []string{"-i"}
	}
	cmd := exec.Command(shell, args...)
	cmd.Dir = directory
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Foreground: true, Ctty: fd}
	result = cmd.Run()
	// Reclaiming the terminal from a background/orphaned editor process group
	// requires SIGTTOU to be ignored or blocked, rather than merely handled.
	ignored := signal.Ignored(syscall.SIGTTOU)
	signal.Ignore(syscall.SIGTTOU)
	if !ignored {
		defer signal.Reset(syscall.SIGTTOU)
	}
	if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPGRP, group); err != nil {
		return errors.Join(result, fmt.Errorf("%w: %v", errResumeScreen, err))
	}
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, settings); err != nil {
		return errors.Join(result, err)
	}
	if !interactive {
		if result != nil {
			fmt.Fprintln(tty, "\nCommand failed:", result)
		}
		fmt.Fprint(tty, "\nPress Enter to return to miv...")
		var input [1]byte
		for {
			n, err := tty.Read(input[:])
			if err != nil {
				return errors.Join(result, err)
			}
			if n == 0 || input[0] == '\n' || input[0] == '\r' {
				break
			}
		}
	}
	return result
}
