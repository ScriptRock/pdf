package text

import (
	"strings"
	"unicode"
)

// Builder builds Text
type Builder struct {
	// location on the page of the last text rendered.
	x, y float64
	text Text
}

// Add adds the Text content to the buffer, merging text parts if possible.
func (b *Builder) Add(t Text) {
	for _, part := range t {
		b.add(part.Size, part.Weight, part.Content, noWhitespace)
	}
}

// Render adds the content with the given dimensions and font to the text builder.
// Text blocks are sectioned into lines and paragraphs based on their relative location
// on the page.
// TODO: segment horizontally segmented text blocks.
func (b *Builder) Render(x, y, w, h float64, font, content string) {
	if len(content) == 0 {
		return
	}

	var ws whitespace
	switch {
	case len(b.text) == 0:
	case y > b.y, // Above previous write.
		y < b.y-2*h: // More than 2 lines below previous write.
		// Next paragraph.
		ws = newParagraph
	case y < b.y: // Below previous write.
		// Next line.
		ws = newLine
	case x > b.x+h:
		ws = newWord
	}
	b.x = x + w
	b.y = y

	var weight int
	if strings.HasSuffix(font, "-Bold") {
		weight = 1
	}

	b.add(h, weight, content, ws)
}

type whitespace int

const (
	noWhitespace whitespace = iota
	newWord
	newLine
	newParagraph
)

func (b *Builder) add(size float64, weight int, content string, w whitespace) {
	isWhitespace := len(strings.TrimSpace(content)) == 0
	if l := len(b.text); l > 0 {
		last := &b.text[l-1]
		if isWhitespace || (last.Size == size && last.Weight == weight) {
			b.append(content, w)
			return
		}
	}

	b.text = append(b.text, Part{Size: size, Weight: weight})
	b.append(content, w)
}

// The Builder must be non-empty to call append, or else it will panic.
func (b *Builder) append(s string, w whitespace) {
	last := &b.text[len(b.text)-1]
	n := len(last.Content)
	m := len(s)

	switch w {
	case noWhitespace:
	case newWord:
		switch {
		case n > 0 && unicode.IsSpace(rune(last.Content[n-1])):
		case m > 0 && unicode.IsSpace(rune(s[0])):
		default:
			last.Content += " "
		}
	case newLine:
		switch {
		case n > 0 && last.Content[n-1] == '\n':
		case m > 0 && s[0] == '\n':
		default:
			last.Content += "\n"
		}
	case newParagraph:
		last.Content = strings.TrimRightFunc(last.Content, unicode.IsSpace)
		s = strings.TrimLeftFunc(s, unicode.IsSpace)
		last.Content += "\n\n"
	}
	last.Content += s
}

func (b Builder) Text() Text { return b.text }
