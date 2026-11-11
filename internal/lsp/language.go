package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type language struct {
	id      string
	command []string
	markers []string
}

func languageFor(path string) (language, error) {
	ext := strings.ToLower(filepath.Ext(path))
	var lang language
	switch ext {
	case ".go":
		lang = language{"go", []string{"gopls"}, []string{"go.work", "go.mod"}}
	case ".rs":
		lang = language{"rust", []string{"rust-analyzer"}, []string{"Cargo.toml"}}
	case ".py", ".pyi":
		lang = language{"python", []string{"pyright-langserver", "--stdio"}, []string{"pyproject.toml", "pyrightconfig.json", "setup.py"}}
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".mts", ".cts":
		id := "javascript"
		if ext == ".ts" || ext == ".tsx" || ext == ".mts" || ext == ".cts" {
			id = "typescript"
		}
		if ext == ".jsx" || ext == ".tsx" {
			id += "react"
		}
		lang = language{id, []string{"typescript-language-server", "--stdio"}, []string{"tsconfig.json", "jsconfig.json", "package.json"}}
	case ".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hxx":
		id := "cpp"
		if ext == ".c" || ext == ".h" {
			id = "c"
		}
		lang = language{id, []string{"clangd"}, []string{"compile_commands.json", ".clangd", "CMakeLists.txt"}}
	default:
		lang.id = strings.TrimPrefix(ext, ".")
	}
	variable := "MIV_LSP_" + strings.ToUpper(lang.id)
	if override := os.Getenv(variable); override != "" {
		lang.command = strings.Fields(override)
	}
	if len(lang.command) == 0 {
		return lang, fmt.Errorf("no language server configured for %s; set %s", ext, variable)
	}
	return lang, nil
}

func projectRoot(path string, lang language) string {
	dir := filepath.Dir(path)
	for current := dir; ; current = filepath.Dir(current) {
		for _, marker := range lang.markers {
			if _, err := os.Stat(filepath.Join(current, marker)); err == nil {
				return current
			}
		}
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current
		}
		if filepath.Dir(current) == current {
			return dir
		}
	}
}
