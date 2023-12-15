package text

import (
	"fmt"
	"strings"
	"unicode"
)

// Text represents minimally structured text extracted from a PDF.
type Text []Part

// Part is a part of Text with the same size and font weight.
type Part struct {
	Size float64
	// bitmask of styles, currently just 1 for bold.
	Weight  int
	Content string
}

// String renders the Text without sizing information.
func (t Text) String() string {
	var b strings.Builder
	for _, p := range t {
		b.WriteString(p.Content)
	}

	return b.String()
}

// DebugString renders the Text as a string with annotation at each change of
// text size.
func (t Text) DebugString() string {
	var b strings.Builder
	for _, p := range t {
		fmt.Fprintf(&b, "[%.1f|%b]", p.Size, p.Weight)
		b.WriteString(p.Content)
	}

	return b.String()
}

// Size is calculated to be the maximum size of any segment in the string.
func (t Text) Size() float64 {
	var ms float64

	for _, p := range t {
		v := p.Size + float64(p.Weight)/100
		ms = max(ms, v)
	}

	return ms
}

// TrimSpace trims whitespace from both ends of the Text.
func (t Text) TrimSpace() Text {
	l := len(t)
	if l == 0 {
		return t
	}

	var trimmed Text
	for i, p := range t {
		if i == 0 {
			p.Content = strings.TrimLeftFunc(p.Content, unicode.IsSpace)
		}
		if i == l-1 {
			p.Content = strings.TrimRightFunc(p.Content, unicode.IsSpace)
		}

		if len(p.Content) > 0 {
			trimmed = append(trimmed, p)
		}
	}

	return trimmed
}

// Split splits the Text by the separator.
func (s Text) Split(sep string) []Text {
	var (
		parts   []Text
		current Builder
	)

	for _, p := range s {
		lines := strings.Split(p.Content, sep)
		for i, line := range lines {
			if i > 0 {
				parts = append(parts, current.text)
				current = Builder{}
			}
			current.add(p.Size, p.Weight, line, noWhitespace)
		}
	}

	if len(current.text) > 0 {
		parts = append(parts, current.text)
	}

	return parts
}
