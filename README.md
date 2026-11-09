# miv

A small modal editor in Go, inspired by essential Vim commands.
This is an independent implementation: it does not run or embed the C Vim
source in `../../vim/vim`. The original code served as a reference for
organization and behavior; this project does not aim for full compatibility.

## Running

Requires Go 1.23 or later and a terminal with UTF-8 support.

```sh
make build
./bin/miv path/to/file
# Open multiple files:
./bin/miv main.go README.md
# Or run without creating a binary in bin/:
go run ./cmd/miv path/to/file
```

Without arguments, the editor starts with an empty buffer. Use `:w filename`
to save. The compiled binary runs without Go installed. In this environment,
the compiler was downloaded to `/tmp/go`: use
`PATH=/tmp/go/bin:$PATH make build` until Go is on your PATH.

## Appearance

The editing area inherits the terminal's default colors, following the foot
theme and background settings. The font also comes from the terminal.
The bars follow your Waybar configuration: background `#222222`, inactive
buffers in `#bbbbbb`, and an active buffer with background `#333333` and
text `#eeeeee`, without bold. The top bar shows buffers on the left, the
file name in the center, and the cursor position on the right. Selection
uses the same gray as the active buffer.

## Syntax highlighting

Enabled automatically by file name or extension, using Chroma.
Supports Go, Rust, C/C++, Python, JavaScript/TypeScript, shell, JSON/JSONC,
YAML, Lua, HTML/CSS, Markdown, Makefile, and other languages recognized
by the library. New files gain highlighting when saved with an extension.

Comments, strings, numbers, keywords, types, and functions use the
terminal's ANSI palette; the background still comes from foot. Visual
selection keeps its gray highlight. Multiline comments and strings are
recognized from the entire document, with caching while the content stays
unchanged. Files with no recognized language or larger than 1 MiB use
plain text. No LSP, plugins, or external programs are required.

## Commands

`Esc` returns to normal mode; `Ctrl-c` also cancels the current operation.
Operations accept counts, such as `5j`, `3dw`, `d2w`, and `2dd`.

| Action | Keys |
| --- | --- |
| Move | `h j k l` or arrow keys |
| Words | `w b e` |
| Line start, first nonblank character, line end | `0 ^ $` |
| First/last line; line N | `gg`, `G`, `Ngg`, `NG`, `:N` |
| Scroll 15 lines | `Ctrl-d`, `Ctrl-u`, PageDown, PageUp |
| Find a character on the current line | `f{character}`, `F{character}`, `t{character}`, `T{character}` |
| Matching parenthesis, bracket, or brace | `%` |
| Insert | `i a I A o O` |
| Delete, change, yank | `d{motion}`, `c{motion}`, `y{motion}` |
| Entire line | `dd cc yy` |
| To the end of the line | `D C Y` |
| Words and text objects | `diw`, `daw`, `ciw`, `ci"`, `ci'`, `ci(`, `di[`, `da{`, etc. |
| Delete, replace, or change a character | `x`, `r{character}`, `s` |
| Character/line selection | `v`, `V`; then `d`, `c`, `y`, `>`, `<` |
| Paste after/before | `p`, `P` |
| Undo/redo | `u`, `Ctrl-r` |
| Repeat the last edit | `.` |
| Join lines; toggle case | `J`, `~` |
| Indent/unindent | `>>`, `<<` |
| Literal search forward/backward | `/text`, `?text`, Enter |
| Next/previous occurrence | `n`, `N` |
| Search for the word under the cursor | `*` |
| Find a file | `Ctrl-p`, `Space f f`, `Space p f`, `:files` |
| Switch projects | `Space p p`, `:projects` |
| Switch workspace | `Space Tab 1` through `Space Tab 9` |
| List/select existing workspaces | `:workspaces`, `:ws` |
| Switch buffers | `gt`, `gT`, `:bn`, `:bp`, `:b N` |
| Go to buffer N/last | `Space 1` through `Space 9`, `Space 0` |
| List buffers | `:ls` |
| Open/create a file in another buffer | `:e path` |
| Save | `:w`, `:w path`, `Space f s`, `Space s` |
| Save and close the buffer | `:wq` |
| Close the buffer; quit all buffers | `:q`, `:qa` |
| Discard changes when closing | `:q!`, `:qa!` |
| Run a shell command | `:!command` |
| Start an interactive shell | `:term`, `:terminal` |
| Run a command with direct return | `:term command` |

The file picker uses subsequence matching: `smg` matches `src/main.go`.
Use arrow keys or `Ctrl-j`/`Ctrl-k`, Enter to open, and Esc to cancel.
It searches from the current workspace project directory (initially the
directory where the editor was started), includes hidden files, skips `.git`, `node_modules`, `target`, `vendor`, and `.cache`, and
limits the listing to 20,000 files. It does not follow symlinks.

In insert mode: Backspace, Delete, Enter with automatic indentation,
Tab with spaces, arrow keys, and `Ctrl-w` to delete the previous word.
`Ctrl-d` or `Shift-Tab` removes one indentation level in insert mode.
Terminal paste uses bracketed paste to preserve content without interpreting
commands or adding indentation.

## Shell commands

On Linux, `:!command` runs a command through `$SHELL` (falling back to
`/bin/sh`). Shell quoting, environment variables, pipes, redirection, and
interactive input work normally. For example:

```vim
:!go test ./...
:!git status
:!printf 'hello\n' | tr a-z A-Z
```

The command takes over the terminal. Press Enter afterward to return to the
editor; failures also report the exit status. Ctrl-C interrupts the command
while keeping the editor alive.

`:term` (or `:terminal`) starts an interactive shell in the same terminal.
Type `exit` or press Ctrl-D at an empty shell prompt to return. With an
argument, `:term command` runs that command and returns immediately when it
finishes, without the extra Enter prompt.

Both forms use the current workspace's project directory. Open buffers and
unsaved edits stay in memory. Save first if the command needs your latest
file contents. The shell's `cd` applies to that shell only.

## Projects and workspaces

The leader is Space. Press these keys in sequence in normal mode:

| Binding | Action |
| --- | --- |
| `SPC f s` | Save the current file |
| `SPC f f` or `SPC p f` | Find a file in the current project |
| `SPC p p` | Choose a project, then find a file in it |
| `SPC TAB 1`–`SPC TAB 9` | Switch to workspace 1–9 |

`SPC f` is now a prefix; press a second `f` to open the file picker.
`Ctrl-p` still opens the picker directly. Esc cancels an unfinished sequence.

The project picker discovers Git repositories near the current project,
including sibling repositories and repositories under nearby owner
directories such as `github.com/owner/repo`. Discovery checks at most 2,000
directories, includes project roots from visited workspaces, and always
includes the current root. Use the same fuzzy filter and navigation keys as
the file picker. Selecting a project changes the current workspace's project
directory; relative `:e` and `:w path` commands then use that directory.

Workspaces are created on demand and keep their buffers, active buffer,
cursor positions, undo history, and project directory for this session.
The `W1`–`W9` badge in the top bar identifies the active workspace; numbered
buffer tags follow it. Existing `Space 1`–`Space 9` bindings still select
buffers within that workspace.

`:workspaces` (or `:ws`) opens a list of workspaces created in this session,
sorted by number. Each row shows the workspace number, buffer count, and
project directory. `*` marks the current workspace and `+` marks unsaved
changes in any of its buffers. Type to filter, use arrow keys or
`Ctrl-j`/`Ctrl-k` to select a row, and press Enter to switch; Esc cancels.

`SPC p f` searches within the current workspace's project directory, using
the same file picker as `SPC f f` and `Ctrl-p`.

Switching workspaces keeps unsaved changes. Closing the last buffer with
`:q` moves to another workspace that still has files; `:qa` checks all
workspaces before quitting. `:qa!` discards unsaved changes in every
workspace. Workspaces and project choices are kept in memory for this
session and are not persisted after exit.

## Automatic indentation

Enter and `o` copy the current indentation. For `.yaml` and `.yml` files,
they also add two spaces after an empty mapping key and align fields in
list mappings. For example, Enter after each colon prepares the indentation
shown here:

```yaml
services:
  api:
    port: 8080
```

After `- name: api`, the next line is aligned for another field:

```yaml
- name: api
  port: 8080
```

Literal/folded block headers (`|`, `>`, and their modifiers) also indent
content. Colons inside quotes, comments, and existing block scalar content
do not create extra indentation. `O` copies the current indentation when
opening a line above. Use `Ctrl-d` or `Shift-Tab` in insert mode to move
back a level, or `>>`/`<<` in normal mode to adjust existing lines.

This is a typing aid based on common YAML patterns, not a YAML formatter
or parser. It does not insert list markers or reformat pasted text.

## Workflow preferences

- `Y` yanks to the end of the line, matching your `.vimrc`.
- YAML, shell, and JSON use 2-space indentation; other files use 4 spaces.
- System clipboard support uses `wl-copy`/`wl-paste` on Wayland,
  `xclip` or `xsel` on X11, or `pbcopy`/`pbpaste` on macOS.
  Without these tools, copy and paste still work inside the editor.
- Open files stay in buffers, with tags at the top, keeping changes when switching.
- Long lines scroll horizontally; there is no visual line wrapping.

The editor has fixed preferences and does not read `.vimrc` or Neovim
configuration. Doom-style leader bindings are listed above; `Space s`
remains a shortcut for saving.

## Files and limits

Opens UTF-8 text files up to 16 MiB. Preserves uniform LF/CRLF line endings
and the presence or absence of a final newline. Each edit stores an
in-memory snapshot, with up to 200 undo entries per buffer; the focus is
editing small and medium files.

Saving uses a temporary file in the same directory followed by rename.
Preserves existing file permissions and valid symlink targets. Detects
external changes and collisions with existing files; `:w!` explicitly
overrides these checks. Because saving uses rename, it does not preserve
hardlinks, another user's ownership, ACLs, or extended attributes.
There are no swap files, autosave, or recovery after an unexpected exit.

Not included in this version: LSP, plugins/Vimscript/Lua, debugging,
splits, block selection, macros, multiple registers, regex/substitution,
and complex graphemes as editing units. Movements and positions use Unicode
code points. Command behavior is a simplified subset of Vim, including
handling of the final line.

## Development

```sh
make test
make check
make smoke  # Python 3 and a Unix pseudoterminal
```

The core in `internal/editor` does not depend on the terminal. `cmd/miv`
connects events, draws the screen with tcell, and accesses the clipboard.
Tests cover editing, history, files, search, the picker, rendering, resizing,
syntax highlighting, workspaces, project discovery, and shell dispatch.
The smoke test opens the binary in a PTY, edits UTF-8 text, pastes multiple
lines, runs shell commands and an interactive shell, interrupts a command
with Ctrl-C, saves, closes, and verifies that terminal settings are restored.
