package editor

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// shellEdit handles line filters and :read !command without interpreting plain
// :!command as a buffer edit. The command output is a single undo transaction.
func (e *Editor) shellEdit(command string) bool {
	for _, prefix := range []string{"read", "r"} {
		if rest, ok := strings.CutPrefix(command, prefix); ok {
			if shell, ok := strings.CutPrefix(strings.TrimSpace(rest), "!"); ok {
				e.applyShellEdit(strings.TrimSpace(shell), 0, 0, true)
				return true
			}
		}
	}
	address, shell, ok := strings.Cut(command, "!")
	if !ok {
		return false
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return false
	}
	b := e.Buffer()
	start, end := 0, 0
	switch address {
	case ".":
		start = b.LineStart(b.Cursor)
		end = b.LineEnd(b.Cursor)
	case "%":
		start = 0
		end = len(b.Text)
	case "'<,'>":
		start = b.LineStart(min(e.Anchor, b.Cursor))
		end = b.LineEnd(max(e.Anchor, b.Cursor))
	default:
		first, last, rangeGiven := strings.Cut(address, ",")
		a, err := strconv.Atoi(first)
		if err != nil {
			return false
		}
		z := a
		if rangeGiven {
			z, err = strconv.Atoi(last)
		}
		lineCount := strings.Count(string(b.Text), "\n") + 1
		if err != nil || a < 1 || z < a || z > lineCount {
			e.Message = "Invalid line range"
			return true
		}
		start = b.Offset(a-1, 0)
		end = b.LineEnd(b.Offset(z-1, 0))
	}
	if end < len(b.Text) {
		end++
	}
	e.applyShellEdit(strings.TrimSpace(shell), start, end, false)
	return true
}

func (e *Editor) applyShellEdit(command string, start, end int, insert bool) {
	if command == "" {
		e.Message = "A shell command is required"
		return
	}
	if e.CaptureShell == nil {
		e.Message = "Shell capture is unavailable"
		return
	}
	b := e.Buffer()
	input := ""
	if !insert {
		input = string(b.Text[start:end])
		if !strings.HasSuffix(input, "\n") {
			input += "\n"
		}
	}
	output, err := e.CaptureShell(command, input)
	if err != nil {
		e.Message = "Shell command failed: " + err.Error()
		return
	}
	if len(output) > maxFileSize || !utf8.ValidString(output) || strings.ContainsRune(output, 0) {
		e.Message = "Command output must be UTF-8 text up to 16 MiB"
		return
	}
	output = strings.ReplaceAll(output, "\r\n", "\n")
	cursor := start
	if insert {
		if output == "" {
			e.Message = "Command produced no output"
			return
		}
		start = b.LineEnd(b.Cursor)
		end = start
		cursor = start
		if start < len(b.Text) {
			start++
			end = start
			cursor = start
		} else if start > 0 {
			output = "\n" + output
			cursor = start + 1
		}
	}
	if end < len(b.Text) && output != "" && !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	b.End()
	b.Replace(start, end, output)
	b.Cursor = min(cursor, len(b.Text))
	b.NormalCursor()
	b.End()
	e.Message = fmt.Sprintf("Command output %s", map[bool]string{true: "inserted", false: "applied"}[insert])
}
