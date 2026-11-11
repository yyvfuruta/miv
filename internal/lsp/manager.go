package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Document struct {
	Path string
	Text string
}

type Query struct {
	Action    string
	Path      string
	Position  Position
	Documents []Document
}

type Result struct {
	Text      string
	Locations []Location
}

type documentState struct {
	text    string
	version int
}

type session struct {
	client    *client
	root      string
	lang      language
	syncKind  int
	providers map[string]json.RawMessage
	documents map[string]documentState
}

// Manager is safe for concurrent queries. Each server session serializes sync
// and requests so a query always sees its own document snapshot.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*session
	closed   bool
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	var wg sync.WaitGroup
	for _, s := range m.sessions {
		wg.Add(1)
		go func(s *session) { defer wg.Done(); s.client.close() }(s)
	}
	wg.Wait()
	m.sessions = nil
}

func newSession(ctx context.Context, lang language, root string) (*session, error) {
	c, err := start(lang.command, root)
	if err != nil {
		return nil, err
	}
	s := &session{client: c, root: root, lang: lang, documents: make(map[string]documentState)}
	folders := []map[string]string{{"uri": FileURI(root), "name": filepath.Base(root)}}
	raw, err := c.call(ctx, "initialize", map[string]any{
		"processId": os.Getpid(), "clientInfo": map[string]string{"name": "miv"},
		"rootUri": FileURI(root), "workspaceFolders": folders,
		"capabilities": map[string]any{
			"general": map[string]any{"positionEncodings": []string{"utf-16"}},
			"textDocument": map[string]any{
				"hover":          map[string]any{"contentFormat": []string{"plaintext", "markdown"}},
				"definition":     map[string]bool{"linkSupport": true},
				"typeDefinition": map[string]bool{"linkSupport": true},
				"implementation": map[string]bool{"linkSupport": true},
			},
		},
	})
	if err == nil {
		var response struct{ Capabilities map[string]json.RawMessage }
		err = json.Unmarshal(raw, &response)
		s.providers = response.Capabilities
		var encoding string
		_ = json.Unmarshal(s.providers["positionEncoding"], &encoding)
		if encoding != "" && encoding != "utf-16" {
			err = fmt.Errorf("language server selected unsupported position encoding: %s", encoding)
		}
		sync := s.providers["textDocumentSync"]
		if json.Unmarshal(sync, &s.syncKind) != nil {
			var options struct{ Change int }
			_ = json.Unmarshal(sync, &options)
			s.syncKind = options.Change
		}
		if s.syncKind != 1 && s.syncKind != 2 {
			err = fmt.Errorf("language server must support document synchronization")
		}
	}
	if err == nil {
		err = c.notify(ctx, "initialized", map[string]any{})
	}
	if err != nil {
		c.kill()
		return nil, err
	}
	return s, nil
}

func (m *Manager) Query(ctx context.Context, q Query) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Result{}, fmt.Errorf("language services are closed")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if q.Path == "" || !filepath.IsAbs(q.Path) {
		return Result{}, fmt.Errorf("save the file before using language commands")
	}
	lang, err := languageFor(q.Path)
	if err != nil {
		return Result{}, err
	}
	root := projectRoot(q.Path, lang)
	key := root + "\x00" + strings.Join(lang.command, "\x00")
	if m.sessions == nil {
		m.sessions = make(map[string]*session)
	}
	s := m.sessions[key]
	if s != nil {
		select {
		case <-s.client.done:
			s.client.kill()
			s = nil
		default:
		}
	}
	if s == nil {
		s, err = newSession(ctx, lang, root)
		if err != nil {
			return Result{}, err
		}
		m.sessions[key] = s
	}
	if err := s.sync(ctx, q.Documents); err != nil {
		return Result{}, err
	}
	if _, ok := s.documents[q.Path]; !ok {
		return Result{}, fmt.Errorf("current file was not synchronized")
	}
	return s.query(ctx, q)
}

func (s *session) sync(ctx context.Context, documents []Document) error {
	present := make(map[string]bool)
	for _, d := range documents {
		lang, err := languageFor(d.Path)
		if err != nil || strings.Join(lang.command, "\x00") != strings.Join(s.lang.command, "\x00") {
			continue
		}
		// Include unsaved files from the project and external files already open
		// in this session, without adding unrelated workspaces to the server.
		rel, err := filepath.Rel(s.root, d.Path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if _, exists := s.documents[d.Path]; !exists {
				continue
			}
		}
		present[d.Path] = true
		old, opened := s.documents[d.Path]
		version := old.version + 1
		var syncErr error
		if !opened {
			syncErr = s.client.notify(ctx, "textDocument/didOpen", map[string]any{"textDocument": map[string]any{
				"uri": FileURI(d.Path), "languageId": lang.id, "version": version, "text": d.Text,
			}})
		} else if old.text != d.Text {
			change := map[string]any{"text": d.Text}
			if s.syncKind == 2 {
				change["range"] = Range{End: PositionAt([]rune(old.text), len([]rune(old.text)))}
			}
			syncErr = s.client.notify(ctx, "textDocument/didChange", map[string]any{
				"textDocument":   map[string]any{"uri": FileURI(d.Path), "version": version},
				"contentChanges": []any{change},
			})
		} else {
			continue
		}
		if syncErr != nil {
			return syncErr
		}
		s.documents[d.Path] = documentState{d.Text, version}
	}
	for path := range s.documents {
		if !present[path] {
			if err := s.client.notify(ctx, "textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": FileURI(path)}}); err != nil {
				return err
			}
			delete(s.documents, path)
		}
	}
	return nil
}

func (s *session) supports(provider string) bool {
	raw := s.providers[provider]
	return len(raw) > 0 && string(raw) != "null" && string(raw) != "false"
}

func (s *session) query(ctx context.Context, q Query) (Result, error) {
	method, provider := "textDocument/"+q.Action, q.Action+"Provider"
	if !s.supports(provider) {
		return Result{}, fmt.Errorf("language server does not support %s", q.Action)
	}
	params := map[string]any{
		"textDocument": map[string]string{"uri": FileURI(q.Path)}, "position": q.Position,
	}
	if q.Action == "references" {
		params["context"] = map[string]bool{"includeDeclaration": true}
	}
	raw, err := s.client.call(ctx, method, params)
	if err != nil {
		return Result{}, err
	}
	switch q.Action {
	case "hover":
		var hover struct{ Contents json.RawMessage }
		if err := json.Unmarshal(raw, &hover); err != nil {
			return Result{}, err
		}
		return Result{Text: strings.TrimSpace(content(hover.Contents))}, nil
	case "signatureHelp":
		var help struct {
			Signatures []struct {
				Label         string
				Documentation json.RawMessage
			}
			ActiveSignature int
		}
		if err := json.Unmarshal(raw, &help); err != nil {
			return Result{}, err
		}
		if len(help.Signatures) == 0 {
			return Result{}, nil
		}
		sig := help.Signatures[min(max(0, help.ActiveSignature), len(help.Signatures)-1)]
		return Result{Text: strings.TrimSpace(sig.Label + "\n\n" + content(sig.Documentation))}, nil
	default:
		locations, err := locations(raw)
		return Result{Locations: locations}, err
	}
}
