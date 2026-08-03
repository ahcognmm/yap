// Package markdown parses a Markdown document into an ordered list of
// prose and code blocks for the notebook-style document pane.
package markdown

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// BlockKind distinguishes prose from fenced code blocks.
type BlockKind int

const (
	// KindProse is a run of Markdown source rendered via Glamour.
	KindProse BlockKind = iota
	// KindCode is a fenced code block, optionally runnable.
	KindCode
)

// Block is one unit of the parsed document, in source order.
type Block struct {
	Kind BlockKind

	// Raw is the original Markdown source for this block (KindProse) or
	// the code fence's content (KindCode).
	Raw string

	// Lang is the fence's info string (e.g. "sh"), KindCode only.
	Lang string

	// Runnable is true when Lang identifies a shell code block.
	Runnable bool

	// Heading, when non-empty, is the nearest preceding heading text —
	// used for "/" filtering by block heading/content.
	Heading string
}

// runnableLangs are fence info strings treated as shell-executable.
var runnableLangs = map[string]bool{
	"sh":   true,
	"bash": true,
	"zsh":  true,
	"shell": true,
}

// Parse splits Markdown source into an ordered slice of Blocks.
func Parse(source []byte) []Block {
	md := goldmark.New()
	doc := md.Parser().Parse(text.NewReader(source))

	var blocks []Block
	var proseStart, proseEnd int
	proseOpen := false
	heading := ""

	flushProse := func() {
		if proseOpen && proseEnd > proseStart {
			raw := string(source[proseStart:proseEnd])
			blocks = append(blocks, Block{
				Kind:    KindProse,
				Raw:     raw,
				Heading: heading,
			})
		}
		proseOpen = false
	}

	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if fcb, ok := n.(*ast.FencedCodeBlock); ok {
			flushProse()

			lang := ""
			if fcb.Info != nil {
				lang = string(fcb.Info.Value(source))
			}

			var b strings.Builder
			lines := fcb.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				b.Write(seg.Value(source))
			}
			raw := b.String()

			blocks = append(blocks, Block{
				Kind:     KindCode,
				Raw:      raw,
				Lang:     lang,
				Runnable: runnableLangs[lang],
				Heading:  heading,
			})
			continue
		}

		start, end := nodeSourceRange(n)
		if start < 0 {
			continue
		}

		if _, ok := n.(*ast.Heading); ok {
			// ast.Heading.Lines() excludes the leading '#' marker(s) —
			// goldmark only keeps the inline title text. Expand back to
			// the full source line so the raw text still reads "# Hello"
			// and Glamour renders it as a heading, not a bare paragraph.
			start, end = lineBounds(source, start)
			heading = strings.TrimSpace(string(source[start:end]))
		}

		if !proseOpen {
			proseOpen = true
			proseStart = start
		}
		proseEnd = end
	}
	flushProse()

	return blocks
}

// nodeSourceRange returns the byte offsets in source spanned by n's lines,
// or (-1, -1) if n has no line info (e.g. an empty block).
func nodeSourceRange(n ast.Node) (int, int) {
	lines := n.Lines()
	if lines == nil || lines.Len() == 0 {
		return -1, -1
	}
	first := lines.At(0)
	last := lines.At(lines.Len() - 1)
	return first.Start, last.Stop
}

// lineBounds expands (start, start) back to the full source line containing
// start — used to recover an ATX heading's '#' marker(s), which goldmark
// strips from ast.Heading.Lines() since it only keeps the inline title text.
func lineBounds(source []byte, start int) (int, int) {
	lineStart := start
	for lineStart > 0 && source[lineStart-1] != '\n' {
		lineStart--
	}
	lineEnd := start
	for lineEnd < len(source) && source[lineEnd] != '\n' {
		lineEnd++
	}
	return lineStart, lineEnd
}
