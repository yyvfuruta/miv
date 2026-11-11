package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Set MIV_TEST_GOPLS to an installed gopls executable to run the real-server test.
func TestGoplsIntegration(t *testing.T) {
	server := os.Getenv("MIV_TEST_GOPLS")
	if server == "" {
		t.Skip("MIV_TEST_GOPLS is not set")
	}
	t.Setenv("MIV_LSP_GO", server)
	t.Setenv("GOPLSCACHE", t.TempDir())
	root := t.TempDir()
	source := "package sample\n\nfunc Example() { _ = Greet(\"world\") }\n"
	target := "package sample\n\n// Greet returns a greeting.\nfunc Greet(name string) string { return name }\n"
	for path, text := range map[string]string{"go.mod": "module example.com/sample\n\ngo 1.23.0\n", "source.go": source, "target.go": target} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	q := Query{Action: "definition", Path: filepath.Join(root, "source.go"),
		Position:  PositionAt([]rune(source), strings.Index(source, "Greet")+1),
		Documents: []Document{{filepath.Join(root, "source.go"), source}, {filepath.Join(root, "target.go"), target}},
	}
	var m Manager
	defer m.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := m.Query(ctx, q)
	if err != nil || len(result.Locations) != 1 || result.Locations[0] != (Location{filepath.Join(root, "target.go"), Position{3, 5}}) {
		t.Fatalf("gopls definition = %+v, %v", result, err)
	}
	q.Action = "hover"
	result, err = m.Query(ctx, q)
	if err != nil || !strings.Contains(result.Text, "Greet(name string) string") || !strings.Contains(result.Text, "returns a greeting") {
		t.Fatalf("gopls hover = %+v, %v", result, err)
	}
	// Neither change is saved: the server must use both open-buffer overlays.
	q.Documents[0].Text = strings.ReplaceAll(source, `"world"`, "42")
	q.Documents[1].Text = strings.ReplaceAll(strings.ReplaceAll(target, "name string", "count int"), "string { return name }", "int { return count }")
	result, err = m.Query(ctx, q)
	if err != nil || !strings.Contains(result.Text, "Greet(count int) int") {
		t.Fatalf("gopls unsaved hover = %+v, %v", result, err)
	}
	q.Action = "signatureHelp"
	q.Position = PositionAt([]rune(q.Documents[0].Text), strings.Index(q.Documents[0].Text, "Greet(")+len("Greet("))
	result, err = m.Query(ctx, q)
	if err != nil || !strings.Contains(result.Text, "Greet(count int) int") {
		t.Fatalf("gopls signature = %+v, %v", result, err)
	}
	q.Action = "references"
	q.Position = PositionAt([]rune(source), strings.Index(source, "Greet")+1)
	result, err = m.Query(ctx, q)
	if err != nil || len(result.Locations) < 2 {
		t.Fatalf("gopls references = %+v, %v", result, err)
	}
}
