package main

import (
	"path/filepath"
	"sort"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/gdamore/tcell/v2"
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
