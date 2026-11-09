//go:build !linux

package main

import "errors"

func (v *view) openTerminal(string, string) error {
	return errors.New("embedded terminals currently require Linux")
}
