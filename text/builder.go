package text

import (
	"strings"
)

// Builder builds Text
type Builder struct {
	y    float64 // y-location on the page of the last text rendered.
	text Text
}

// Add adds the Text content to the buffer, merging text parts if possible.
func (s *Builder) Add(t Text) {
	for _, part := range t {
		s.add(part.Size, part.Weight, part.Content)
	}
}

// Render adds the content with the given dimensions and font to the text builder.
// Text blocks are sectioned into lines and paragraphs based on their relative location
// on the page.
// TODO: segment horizontally segmented text blocks.
func (s *Builder) Render(x, y, w, h float64, font, content string) {
	if len(content) == 0 {
		return
	}

	switch {
	case len(s.text) == 0:
	case y > s.y, y < s.y-2*h:
		// Next paragraph.
		content = "\n\n" + content
	case y < s.y:
		// Next line.
		content = "\n" + content
	}
	s.y = y

	var weight int
	if strings.HasSuffix(font, "-Bold") {
		weight = 1
	}

	s.add(h, weight, content)
}

func (b *Builder) add(size float64, weight int, content string) {
	isWhitespace := len(strings.TrimSpace(content)) == 0
	var lastPiece *Part
	if l := len(b.text); l > 0 {
		lastPiece = &b.text[l-1]
	}

	if lastPiece != nil && (isWhitespace || (lastPiece.Size == size && lastPiece.Weight == weight)) {
		lastPiece.Content += content
		return
	}

	b.text = append(b.text, Part{Size: size, Weight: weight, Content: content})
}

func (b Builder) Text() Text { return b.text }
