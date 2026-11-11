package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestUTF16Positions(t *testing.T) {
	text := []rune("a😀é\n界𝄞z\n")
	want := []Position{{0, 0}, {0, 1}, {0, 3}, {0, 4}, {1, 0}, {1, 1}, {1, 3}, {1, 4}, {2, 0}}
	for i, p := range want {
		if got := PositionAt(text, i); got != p {
			t.Fatalf("position at %d = %+v, want %+v", i, got, p)
		}
		if got := Offset(text, p); got != i {
			t.Fatalf("offset at %+v = %d, want %d", p, got, i)
		}
	}
	if Offset(text, Position{0, 2}) != 1 || Offset(text, Position{0, 100}) != 3 || Offset(text, Position{100, 0}) != len(text) {
		t.Fatal("invalid positions were not clamped")
	}
}

func TestURIsAndLocations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "space # café.go")
	uri := FileURI(path)
	if got, err := FilePath(uri); err != nil || got != path {
		t.Fatalf("URI round trip = %q, %v", got, err)
	}
	for _, uri := range []string{"https://example.com/a", "file://remote/a", "untitled:one"} {
		if _, err := FilePath(uri); err == nil {
			t.Fatal("accepted unsupported URI", uri)
		}
	}
	for _, raw := range []string{
		fmt.Sprintf(`{"uri":%q,"range":{"start":{"line":2,"character":4}}}`, uri),
		fmt.Sprintf(`[{"uri":%q,"range":{"start":{"line":2,"character":4}}}]`, uri),
		fmt.Sprintf(`[{"targetUri":%q,"targetSelectionRange":{"start":{"line":2,"character":4}}}]`, uri),
	} {
		got, err := locations(json.RawMessage(raw))
		if err != nil || len(got) != 1 || got[0] != (Location{path, Position{2, 4}}) {
			t.Fatalf("locations = %+v, %v", got, err)
		}
	}
}

func TestHoverContent(t *testing.T) {
	for _, tt := range []struct{ raw, want string }{
		{`"func F()"`, "func F()"},
		{`{"kind":"plaintext","value":"func F()"}`, "func F()"},
		{`[{"language":"go","value":"func F()"},"Documentation"]`, "func F()\n\nDocumentation"},
		{`null`, ""},
	} {
		if got := content(json.RawMessage(tt.raw)); got != tt.want {
			t.Fatalf("content = %q, want %q", got, tt.want)
		}
	}
}

func TestReadMessages(t *testing.T) {
	data := `{"jsonrpc":"2.0","id":1,"result":"café"}`
	framed := fmt.Sprintf("Content-Type: application/vscode-jsonrpc; charset=utf-8\r\nContent-Length: %d\r\n\r\n%s", len(data), data)
	r := bufio.NewReader(strings.NewReader(framed + framed))
	for i := 0; i < 2; i++ {
		m, err := readMessage(r)
		if err != nil || string(m.Result) != `"café"` {
			t.Fatalf("read message = %+v, %v", m, err)
		}
	}
	for _, bad := range []string{"\r\n", "Content-Length: -1\r\n\r\n", "Content-Length: 999999999\r\n\r\n", "Content-Length: nope\r\n\r\n", "Content-Length: 4\r\n\r\n{}"} {
		if _, err := readMessage(bufio.NewReader(strings.NewReader(bad))); err == nil {
			t.Fatal("accepted malformed frame", bad)
		}
	}
}
