# miv

A small modal terminal editor in Go, with Vim-style editing and Doom-style
project and workspace shortcuts.

## Getting started

Download a binary from [GitHub Releases](https://github.com/yyvfuruta/miv/releases),
or build with Go and Make. The required Go version is declared in [go.mod](go.mod).

```sh
make build
./bin/miv file.txt
./bin/miv .                 # Browse the current directory
./bin/miv path/to/project   # Browse another directory
./bin/miv main.go README.md # Open multiple files
```

Without arguments, miv opens an empty buffer. Use `:w filename` to save it.
A directory argument sets the workspace's project directory and opens its
file picker. Use a terminal with UTF-8 support.

## Editing

`Esc` returns to normal mode. Commands accept counts, such as `5j` and `2dd`.

| Action | Keys |
| --- | --- |
| Move | `h j k l`, arrows, `w b e`, `0 ^ $`, `gg G` |
| Insert | `i a I A o O` |
| Select characters / lines | `v` / `V` |
| Reflow selected lines to 80 columns | `gq` / `gw` in visual mode |
| Delete / change / yank | `d{motion}` / `c{motion}` / `y{motion}` |
| Delete / change / yank a line | `dd` / `cc` / `yy` |
| Paste after / before | `p` / `P` |
| Undo / redo / repeat | `u` / `Ctrl-r` / `.` |
| Literal search | `/text`, `?text`, then `n` / `N` |
| Save / save and close / quit | `:w` / `:wq` / `:q` |
| Quit all buffers and workspaces | `:qa` |

Text objects include words, quotes, and brackets: `ciw`, `ci"`, `di(`.
`Y` yanks to the end of the line. Syntax highlighting follows the file type;
colors and the font follow the terminal. Enter preserves indentation, with
YAML-aware indentation. Use `Ctrl-d` or `Shift-Tab` in insert mode to unindent.
Select text with `v` or whole lines with `V`, then press `gq` to reflow the
selected lines. Blank lines separate paragraphs; each paragraph keeps its
first line's indentation. Words are kept intact. `gw` formats the same way
and keeps the cursor on its original text; `gq` moves it to the first nonblank
character of the last formatted line. Use `u` to undo the entire formatting.

## Code navigation

Language commands use a language server installed separately and available on
`PATH`. The first command starts the server for the file's project. Save new
files once to give them a path; later queries include unsaved changes in open
project buffers. Queries run in the background with a 20-second timeout.
Typing another key cancels the pending query; late replies never change your
current file or cursor.

| Action | Normal-mode keys |
| --- | --- |
| Go to definition | `gd` |
| Show signature and documentation | `K` |
| Go to implementation | `gi` |
| Find references | `gr` |
| Go to type definition | `gy` |
| Show call signature at the cursor | `gs` or `:signature` |
| Return to the previous location | `Ctrl-o` |

`K` opens a read-only documentation view. Use `j`/`k`, arrows, Page Up/Down,
or `Ctrl-u`/`Ctrl-d` to scroll; `g`/`G` go to the top/bottom. Close it with
Esc or `q`. Signatures and documentation depend on what the server provides
for the symbol under the cursor; `gs` is useful inside a function call.
Multiple definitions or references open a filterable location picker. Enter
jumps to the selected result; Esc cancels. Navigation keeps existing buffers
and their unsaved edits.

| Language | Default server command | Override variable |
| --- | --- | --- |
| Go | `gopls` | `MIV_LSP_GO` |
| Rust | `rust-analyzer` | `MIV_LSP_RUST` |
| Python | `pyright-langserver --stdio` | `MIV_LSP_PYTHON` |
| JavaScript / JSX | `typescript-language-server --stdio` | `MIV_LSP_JAVASCRIPT` / `MIV_LSP_JAVASCRIPTREACT` |
| TypeScript / TSX | `typescript-language-server --stdio` | `MIV_LSP_TYPESCRIPT` / `MIV_LSP_TYPESCRIPTREACT` |
| C / C++ | `clangd` | `MIV_LSP_C` / `MIV_LSP_CPP` |

For Go, install [gopls](https://go.dev/gopls/#installation):

```sh
go install golang.org/x/tools/gopls@latest
export PATH="$(go env GOPATH)/bin:$PATH"
```

Overrides contain an executable followed by whitespace-separated arguments;
they do not interpret shell quoting or expansions. For example:

```sh
MIV_LSP_PYTHON='basedpyright-langserver --stdio' miv project/
```

Other file extensions can use `MIV_LSP_<EXTENSION>` (uppercase, without the
dot), for example `MIV_LSP_LUA='lua-language-server'`. The language ID for
these files is the lowercase extension. The server must support standard
stdio LSP, UTF-16 positions, and full or incremental document synchronization.
Unavailable servers and unsupported features produce a status message.
Completion, rename, code actions, and automatic diagnostics are not implemented.

## Files, projects, and workspaces

`SPC` means Space; press leader sequences in normal mode.

| Action | Keys or command |
| --- | --- |
| Save | `SPC f s` |
| Find a project file | `SPC p f`, `SPC f f`, `Ctrl-p` |
| Choose a nearby Git project | `SPC p p` |
| Open or create a file | `:e path` |
| Switch buffers | `gt`, `gT`, `:bn`, `:bp`, `:b N` |
| List buffers | `:ls` |
| Split below / right | `SPC w s` / `SPC w v`, `:split` / `:vsplit` |
| Focus left / down / up / right window | `SPC w h` / `SPC w j` / `SPC w k` / `SPC w l` |
| Close the current file window | `SPC w q`, `:close` |
| Switch workspace | `SPC TAB 1` through `SPC TAB 9` |
| List and select workspaces | `:workspaces` or `:ws` |
| Create a workspace | `SPC TAB n` or `:wsnew [N]` |
| Delete a workspace | `SPC TAB d` or `:wsdelete [N]` |

Pickers support fuzzy filtering, arrows or `Ctrl-j`/`Ctrl-k`, Enter to select,
and Esc to cancel. Workspace lists mark the current workspace with `*` and
unsaved edits with `+`. Relative file paths use the current project directory.
Workspaces keep their buffers, split layout, and project directory for the
current session. Each file window keeps its own cursor and scroll position.
Closing a split keeps its buffer available; `:q` closes the current split
when multiple file windows are open.

## Shell commands

On Linux, `:!command` runs a shell command in the project directory. Press
Enter afterward to return. `:.!command` replaces the current line with command
output; `:%!command` filters the whole buffer. `:r !command` inserts output
below the current line. These edits can be undone with `u`.

`:term` opens or focuses an embedded terminal pane; `:term command` runs a
command in it. Press `Ctrl-\` then `Ctrl-n` to return focus to the editor.
From the bottom file window, `SPC w j` also focuses the terminal. Leader
bindings apply in normal mode; use `Ctrl-\` then `Ctrl-n` when leaving shell
input. Use `exit` or Ctrl-D to end the shell, or `:termclose` to close the pane.
Save first if the command needs your latest edits.

## Notes

miv implements a subset of Vim and does not load Vim or Neovim configuration.
There is no autosave or crash recovery. Unsaved changes prevent closing or
deleting a workspace; `:q!`, `:qa!`, and `:wsdelete!` explicitly discard edits.

System clipboard support uses `wl-copy`/`wl-paste`, `xclip`/`xsel`, or
`pbcopy`/`pbpaste`. Without them, copying and pasting still work within miv.

## Development

```sh
make test
make check
make smoke # Terminal integration checks on Linux
```

The LSP tests use a local test server. To also test definition, documentation,
references, signatures, and unsaved edits with a real Go server, run:

```sh
MIV_TEST_GOPLS="$(command -v gopls)" go test ./internal/lsp -run TestGoplsIntegration -v
```
