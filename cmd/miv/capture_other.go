//go:build !linux

package main

import (
	"errors"

	"github.com/gdamore/tcell/v2"
)

func captureShell(tcell.Screen, string, string, string) (string, error) {
	return "", errors.New("shell commands currently require Linux")
}
