package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartupDirectoryPicker(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	e, err := New([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if e.Mode != Picker || e.Root != root || len(e.Matches) != 1 || e.Matches[0] != filepath.Join("nested", "hello.txt") {
		t.Fatalf("directory startup: root=%q mode=%v matches=%v", e.Root, e.Mode, e.Matches)
	}
	keys(e, "hello")
	e.Handle("<Enter>")
	if e.Buffer().Path != filepath.Join(root, "nested", "hello.txt") {
		t.Fatal("selected file opened outside requested directory")
	}
	assertText(t, e, "hello")
	e.Execute("e new.txt")
	if e.Buffer().Path != filepath.Join(root, "new.txt") {
		t.Fatal("directory did not become workspace project root")
	}
}

func TestStartupCurrentAndEmptyDirectories(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	e, err := New([]string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if e.Mode != Picker || e.Root != cwd {
		t.Fatalf("dot directory: %q %v", e.Root, e.Mode)
	}
	e, err = New([]string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if e.Mode != Picker || len(e.Matches) != 0 {
		t.Fatal("empty directory did not open an empty picker")
	}
	e.Handle("<Enter>")
	if e.Mode != Normal || len(e.Buffers) != 1 {
		t.Fatal("empty directory picker could not return to buffer")
	}
}

func TestStartupMixedFilesAndDirectories(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(t.TempDir(), "existing.txt")
	missing := filepath.Join(t.TempDir(), "new.txt")
	if err := os.WriteFile(file, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, paths := range [][]string{{file, root, missing}, {root, file, missing}} {
		e, err := New(paths)
		if err != nil {
			t.Fatal(err)
		}
		if e.Mode != Picker || len(e.Buffers) != 2 || e.Buffers[0].Path != file || e.Buffers[1].Path != missing {
			t.Fatal("file arguments changed when a directory was supplied")
		}
		e.Handle("<Esc>")
		assertText(t, e, "existing")
	}
	e, err := New([]string{missing})
	if err != nil {
		t.Fatal(err)
	}
	if e.Mode != Normal || e.Buffer().Path != missing {
		t.Fatal("new file path stopped working")
	}
	if _, err = New([]string{root, t.TempDir()}); err == nil {
		t.Fatal("multiple directories were silently ignored")
	}
}

func TestStartupSymlinkDirectory(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(t.TempDir(), "project")
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("text"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, link); err != nil {
		t.Skip(err)
	}
	e, err := New([]string{link})
	if err != nil {
		t.Fatal(err)
	}
	if e.Mode != Picker || e.Root != root || len(e.Matches) != 1 {
		t.Fatal("symlinked directory was not traversed")
	}
}
