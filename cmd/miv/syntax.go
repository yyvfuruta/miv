package main

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/gdamore/tcell/v2"
	"github.com/yyvfuruta/vim/internal/editor"
)

// Keep full-document lexing bounded. Larger files remain editable without colors.
const maxHighlightBytes = 1 << 20

type syntaxSpan struct {
	start, end int // Code-point offsets, matching the editing buffer.
	color      tcell.Color
}

type syntaxCache struct {
	path, text string
	spans      []syntaxSpan
}

func (c *syntaxCache) update(path, text string) {
	if c.path == path && c.text == text {
		return
	}
	c.path, c.text, c.spans = path, text, nil
	if path == "" || len(text) > maxHighlightBytes {
		return
	}
	if strings.EqualFold(filepath.Ext(path), ".org") {
		c.highlightOrg()
		return
	}
	lexer := lexers.Match(filepath.Base(path))
	if lexer == nil {
		return
	}
	// Preserve input offsets even for mixed line endings and standalone CRs.
	tokens, err := chroma.Coalesce(lexer).Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, text)
	if err != nil {
		return
	}
	offset := 0
	for token := tokens(); token != chroma.EOF; token = tokens() {
		end := offset + utf8.RuneCountInString(token.Value)
		if color := syntaxColor(token.Type); color != tcell.ColorDefault {
			c.spans = append(c.spans, syntaxSpan{offset, end, color})
		}
		offset = end
	}
}

// ANSI palette entries follow the user's terminal theme instead of hardcoded RGBs.
func syntaxColor(token chroma.TokenType) tcell.Color {
	switch {
	case token.InCategory(chroma.Comment):
		return tcell.PaletteColor(8)
	case token == chroma.KeywordType:
		return tcell.PaletteColor(6)
	case token.InCategory(chroma.Keyword):
		return tcell.PaletteColor(5)
	case token.InSubCategory(chroma.LiteralString):
		return tcell.PaletteColor(2)
	case token.InSubCategory(chroma.LiteralNumber):
		return tcell.PaletteColor(3)
	case token == chroma.NameFunction || token == chroma.NameFunctionMagic || token == chroma.NameBuiltin:
		return tcell.PaletteColor(4)
	case token == chroma.NameClass || token == chroma.NameTag || token == chroma.NameAttribute || token == chroma.NameProperty:
		return tcell.PaletteColor(6)
	case token == chroma.GenericHeading || token == chroma.GenericSubheading:
		return tcell.PaletteColor(4)
	case token == chroma.GenericInserted:
		return tcell.PaletteColor(2)
	case token == chroma.GenericDeleted:
		return tcell.PaletteColor(1)
	default:
		return tcell.ColorDefault
	}
}

func (c *syntaxCache) firstSpan(offset int) int {
	return sort.Search(len(c.spans), func(i int) bool { return c.spans[i].end > offset })
}

// Advance once per span as visible characters are drawn; moving the cursor reuses the cache.
func (c *syntaxCache) styleAt(offset int, index *int) tcell.Style {
	for *index < len(c.spans) && c.spans[*index].end <= offset {
		*index++
	}
	if *index < len(c.spans) && c.spans[*index].start <= offset {
		return base.Foreground(c.spans[*index].color)
	}
	return base
}

var orgInline = regexp.MustCompile(`\[\[[^\n]+?\]\]|\[[ Xx-]\]`)

func (c *syntaxCache) highlightOrg() {
	b := &editor.Buffer{Path: c.path, Text: []rune(c.text)}
	headings := make(map[int]int)
	for _, h := range b.OrgHeadings() {
		headings[h.Start] = h.Level
	}
	offset := 0
	add := func(start, end, color int) {
		if end > start {
			c.spans = append(c.spans, syntaxSpan{start, end, tcell.PaletteColor(color)})
		}
	}
	for _, line := range strings.Split(c.text, "\n") {
		runes := []rune(line)
		if level, ok := headings[offset]; ok {
			start := level
			for start < len(runes) && (runes[start] == ' ' || runes[start] == '\t') {
				start++
			}
			end := start
			for end < len(runes) && runes[end] != ' ' && runes[end] != '\t' {
				end++
			}
			state := string(runes[start:end])
			if state == "TODO" || state == "DONE" {
				add(offset, offset+start, 4)
				color := 3
				if state == "DONE" {
					color = 2
				}
				add(offset+start, offset+end, color)
				add(offset+end, offset+len(runes), 4)
			} else {
				add(offset, offset+len(runes), 4)
			}
		} else if strings.HasPrefix(strings.TrimSpace(line), "#") {
			color := 8
			if strings.HasPrefix(strings.TrimSpace(line), "#+") {
				color = 5
			}
			add(offset, offset+len(runes), color)
		} else {
			for _, match := range orgInline.FindAllStringIndex(line, -1) {
				start := offset + utf8.RuneCountInString(line[:match[0]])
				end := start + utf8.RuneCountInString(line[match[0]:match[1]])
				color := 6
				if line[match[0]:match[1]] == "[X]" || line[match[0]:match[1]] == "[x]" {
					color = 2
				}
				add(start, end, color)
			}
		}
		offset += len(runes) + 1
	}
}
