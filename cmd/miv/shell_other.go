//go:build !linux

package main

import (
	"errors"

	"github.com/gdamore/tcell/v2"
)

var errResumeScreen = errors.New("could not restore the editor terminal")

func runShell(tcell.Screen, string, bool, string) error {
	return errors.New("shell commands currently require Linux")
}
