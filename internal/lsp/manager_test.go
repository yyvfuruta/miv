package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The subprocess exercises real stdio framing and lifecycle without requiring
// language servers on contributors' machines or in CI.
func TestLanguageServerProcess(t *testing.T) {
	if os.Getenv("MIV_TEST_LSP") != "1" {
		return
	}
	if err := serveTestLanguage(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func serveTestLanguage() error {
	r := bufio.NewReader(os.Stdin)
	documents := make(map[string]string)
	initialized := false
	syncKind := 2
	if os.Getenv("MIV_TEST_LSP_MODE") == "full" {
		syncKind = 1
	}
	send := func(value any) error {
		data, err := json.Marshal(value)
		if err == nil {
			_, err = fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(data), data)
		}
		return err
	}
	for {
		m, err := readMessage(r)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var result any
		switch m.Method {
		case "initialize":
			result = map[string]any{"capabilities": map[string]any{
				"textDocumentSync": syncKind, "definitionProvider": true,
				"hoverProvider": true, "referencesProvider": true, "signatureHelpProvider": map[string]any{},
			}}
		case "initialized":
			initialized = true
			continue
		case "textDocument/didOpen":
			var p struct{ TextDocument struct{ URI, Text string } }
			_ = json.Unmarshal(m.Params, &p)
			if !initialized {
				return fmt.Errorf("didOpen before initialized")
			}
			documents[p.TextDocument.URI] = p.TextDocument.Text
			continue
		case "textDocument/didChange":
			var p struct {
				TextDocument   struct{ URI string }
				ContentChanges []struct {
					Text  string
					Range *Range
				}
			}
			_ = json.Unmarshal(m.Params, &p)
			change := p.ContentChanges[0]
			old := []rune(documents[p.TextDocument.URI])
			if syncKind == 2 && (change.Range == nil || *change.Range != (Range{End: PositionAt(old, len(old))})) {
				return fmt.Errorf("incorrect incremental replacement range")
			}
			if syncKind == 1 && change.Range != nil {
				return fmt.Errorf("full sync included a range")
			}
			documents[p.TextDocument.URI] = change.Text
			continue
		case "textDocument/didClose":
			var p struct{ TextDocument struct{ URI string } }
			_ = json.Unmarshal(m.Params, &p)
			delete(documents, p.TextDocument.URI)
			continue
		case "textDocument/hover", "textDocument/definition", "textDocument/references", "textDocument/signatureHelp":
			if os.Getenv("MIV_TEST_LSP_MODE") == "hang" {
				continue
			}
			if os.Getenv("MIV_TEST_LSP_MODE") == "crash" {
				return fmt.Errorf("simulated server crash")
			}
			var p struct {
				TextDocument struct{ URI string }
				Position     Position
			}
			_ = json.Unmarshal(m.Params, &p)
			text, ok := documents[p.TextDocument.URI]
			if !ok || !initialized {
				return fmt.Errorf("request before document sync")
			}
			switch m.Method {
			case "textDocument/hover":
				// Exercise a server-to-client request with a string ID while the
				// client is waiting for its hover response.
				_ = send(map[string]any{"jsonrpc": "2.0", "id": "configuration", "method": "workspace/configuration", "params": map[string]any{"items": []any{map[string]string{"section": "test"}}}})
				response, err := readMessage(r)
				if err != nil || string(response.Result) != "[null]" {
					return fmt.Errorf("incorrect configuration reply: %s, %v", response.Result, err)
				}
				result = map[string]any{"contents": map[string]string{"kind": "plaintext", "value": text}}
			case "textDocument/definition":
				result = []any{map[string]any{"targetUri": p.TextDocument.URI, "targetSelectionRange": Range{Start: p.Position}}}
			case "textDocument/references":
				result = []any{
					map[string]any{"uri": p.TextDocument.URI, "range": Range{Start: p.Position}},
					map[string]any{"uri": p.TextDocument.URI, "range": Range{Start: Position{1, 2}}},
				}
			case "textDocument/signatureHelp":
				result = map[string]any{"signatures": []any{map[string]any{"label": "func F(value int)", "documentation": "Function documentation"}}}
			}
		case "shutdown":
		case "exit":
			return nil
		default:
			return fmt.Errorf("unexpected method: %s", m.Method)
		}
		if len(m.ID) > 0 {
			if err := send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result}); err != nil {
				return err
			}
		}
	}
}

func fakeServer(t *testing.T, mode string) Query {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MIV_TEST_LSP", "1")
	t.Setenv("MIV_TEST_LSP_MODE", mode)
	t.Setenv("MIV_LSP_GO", executable+" -test.run=^TestLanguageServerProcess$")
	path := filepath.Join(t.TempDir(), "main.go")
	return Query{Action: "hover", Path: path, Position: Position{0, 3}, Documents: []Document{{path, "a😀bc"}}}
}

func TestManagerSyncAndQueries(t *testing.T) {
	for _, mode := range []string{"incremental", "full"} {
		t.Run(mode, func(t *testing.T) {
			q := fakeServer(t, mode)
			var m Manager
			defer m.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for _, text := range []string{"a😀bc", "unsaved😀\nnew text"} {
				q.Documents[0].Text = text
				q.Action = "hover"
				result, err := m.Query(ctx, q)
				if err != nil || result.Text != text {
					t.Fatalf("hover = %+v, %v", result, err)
				}
			}
			q.Action = "definition"
			result, err := m.Query(ctx, q)
			if err != nil || len(result.Locations) != 1 || result.Locations[0] != (Location{q.Path, q.Position}) {
				t.Fatalf("definition = %+v, %v", result, err)
			}
			q.Action = "references"
			result, err = m.Query(ctx, q)
			if err != nil || len(result.Locations) != 2 {
				t.Fatalf("references = %+v, %v", result, err)
			}
			q.Action = "signatureHelp"
			result, err = m.Query(ctx, q)
			if err != nil || result.Text != "func F(value int)\n\nFunction documentation" {
				t.Fatalf("signature = %+v, %v", result, err)
			}
			q.Action = "implementation"
			if _, err := m.Query(ctx, q); err == nil || !strings.Contains(err.Error(), "does not support") {
				t.Fatal("unsupported method did not report an error", err)
			}
		})
	}
}

func TestManagerFailureAndTimeout(t *testing.T) {
	for _, mode := range []string{"hang", "crash"} {
		t.Run(mode, func(t *testing.T) {
			q := fakeServer(t, mode)
			var m Manager
			defer m.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			if _, err := m.Query(ctx, q); err == nil {
				t.Fatal("failed server returned no error")
			}
			// The next query must start a fresh server after cancellation or a crash.
			t.Setenv("MIV_TEST_LSP_MODE", "incremental")
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if result, err := m.Query(ctx, q); err != nil || result.Text != q.Documents[0].Text {
				t.Fatalf("server did not recover: %+v, %v", result, err)
			}
		})
	}
}

func TestMissingServerAndUnnamedFile(t *testing.T) {
	t.Setenv("MIV_LSP_GO", filepath.Join(t.TempDir(), "missing-server"))
	var m Manager
	defer m.Close()
	if _, err := m.Query(context.Background(), Query{}); err == nil || !strings.Contains(err.Error(), "save the file") {
		t.Fatal("unnamed file was not rejected", err)
	}
	path := filepath.Join(t.TempDir(), "main.go")
	if _, err := m.Query(context.Background(), Query{Path: path}); err == nil || !strings.Contains(err.Error(), "start language server") {
		t.Fatal("missing server error was not actionable", err)
	}
}
