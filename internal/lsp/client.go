package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type message struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type client struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int
	pending map[int]chan message
	done    chan struct{}
	waited  chan struct{}
	endOnce sync.Once
	err     error
}

func start(command []string, root string) (*client, error) {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = root
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, err
	}
	// A server's logs must never write over the editor's terminal.
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		in.Close()
		out.Close()
		return nil, fmt.Errorf("start language server %s: %w", command[0], err)
	}
	c := &client{cmd: cmd, in: in, pending: make(map[int]chan message), done: make(chan struct{}), waited: make(chan struct{})}
	go c.read(out)
	go func() {
		err := cmd.Wait()
		if err == nil {
			err = io.EOF
		}
		c.finish(err)
		close(c.waited)
	}()
	return c, nil
}

func readMessage(r *bufio.Reader) (message, error) {
	length, headerBytes := -1, 0
	for {
		lineBytes, err := r.ReadSlice('\n')
		line := string(lineBytes)
		headerBytes += len(line)
		if err != nil {
			return message{}, err
		}
		if headerBytes > 8192 {
			return message{}, fmt.Errorf("language server header is too large")
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(name, "Content-Length") {
			length, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return message{}, fmt.Errorf("invalid language server content length")
			}
		}
	}
	if length < 0 || length > 32<<20 {
		return message{}, fmt.Errorf("invalid language server message size")
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return message{}, err
	}
	var m message
	err := json.Unmarshal(data, &m)
	return m, err
}

func (c *client) finish(err error) {
	c.endOnce.Do(func() {
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
		close(c.done)
	})
}

func (c *client) read(out io.Reader) {
	r := bufio.NewReader(out)
	for {
		m, err := readMessage(r)
		if err != nil {
			c.finish(err)
			return
		}
		if m.Method != "" {
			if len(m.ID) != 0 {
				go c.serverRequest(m)
			}
			continue
		}
		var id int
		if json.Unmarshal(m.ID, &id) != nil {
			continue
		}
		c.mu.Lock()
		ch := c.pending[id]
		c.mu.Unlock()
		if ch != nil {
			select {
			case ch <- m:
			default:
			}
		}
	}
}

func (c *client) serverRequest(m message) {
	response := map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": nil}
	switch m.Method {
	case "workspace/configuration":
		var params struct{ Items []json.RawMessage }
		_ = json.Unmarshal(m.Params, &params)
		response["result"] = make([]any, len(params.Items))
	case "window/workDoneProgress/create":
	default:
		delete(response, "result")
		response["error"] = rpcError{-32601, "Client method is not supported"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = c.send(ctx, response)
}

func (c *client) send(ctx context.Context, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	result := make(chan error, 1)
	go func() {
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		_, err := fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n%s", len(data), data)
		result <- err
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		// Killing a server that stopped reading also releases a blocked writer.
		c.kill()
		return ctx.Err()
	case <-c.done:
		return c.failure()
	}
}

func (c *client) failure() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Errorf("language server disconnected: %v", c.err)
}

func (c *client) notify(ctx context.Context, method string, params any) error {
	return c.send(ctx, map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan message, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	if err := c.send(ctx, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	select {
	case m := <-ch:
		if m.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		return m.Result, nil
	case <-ctx.Done():
		c.kill()
		return nil, ctx.Err()
	case <-c.done:
		return nil, c.failure()
	}
}

func (c *client) kill() {
	_ = c.cmd.Process.Kill()
	_ = c.in.Close()
	c.finish(fmt.Errorf("language server stopped"))
}

func (c *client) close() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = c.call(ctx, "shutdown", nil)
	_ = c.notify(ctx, "exit", nil)
	select {
	case <-c.waited:
	case <-ctx.Done():
		c.kill()
	}
}
