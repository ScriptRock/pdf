package text

import (
	"strconv"
	"strings"
)

// Sectioned attempts to process the text into a structured hierarchy of sections.
// This does not work via metadata encoded in the PDF as that is found to be unreliable,
// but via text size and positioning.
func (t Text) Sectioned() Content {
	var (
		content Content
		sized   Builder
	)

	parts := t.Split("\n")
	for i, line := range parts {
		line = line.TrimSpace()
		// Drop page numbers.
		if _, err := strconv.Atoi(line.String()); err == nil {
			continue
		}

		if isHeading(parts, i) {
			content.writeText(sized.Text())
			sized = Builder{}

			content.writeSection(line)
		} else {
			sized.Add(line)
		}

		sized.Add(Text{{Content: "\n"}})
	}

	content.writeText(sized.Text())

	return content
}

type Content []interface {
	String() string
	DebugString() string
}

func (c *Content) writeText(content Text) {
	for i := len(*c) - 1; i >= 0; i-- {
		if s, ok := (*c)[i].(*Section); ok {
			// Write the text to the last section.
			s.Content.writeText(content)
			return
		}
	}

	// No sections, append to the content directly.
	*c = append(*c, content.TrimSpace())
}

func (c *Content) writeSection(title Text) {
	n := title.Size()

	for i := len(*c) - 1; i >= 0; i-- {
		if s, ok := (*c)[i].(*Section); ok {
			if n < s.Title.Size() {
				// Write the text to the last section.
				s.Content.writeSection(title)
				return
			}
		}
	}

	*c = append(*c, &Section{Title: title})
}

func (c Content) String() string {
	var buf strings.Builder
	for _, s := range c {
		buf.WriteString(s.String())
	}
	return buf.String()
}

func (c Content) DebugString() string {
	var buf strings.Builder
	for _, s := range c {
		buf.WriteString(s.DebugString())
	}
	return buf.String()
}

func (c Content) Headings() []string { return c.headings(0) }

func (c Content) headings(depth int) []string {
	var hh []string

	prefix := strings.Repeat("\t", depth)

	for _, v := range c {
		if s, ok := v.(*Section); ok {
			hh = append(hh, prefix+s.Title.String())
			hh = append(hh, s.Content.headings(depth+1)...)
		}
	}

	return hh
}

func (c Content) Sections(names []string) Content {
	want := map[string]bool{}

	for _, line := range names {
		want[line] = true
	}
	return c.sections(want)
}

func (c Content) sections(names map[string]bool) Content {
	var cc Content

	for _, v := range c {
		if s, ok := v.(*Section); ok {
			if names[s.Title.String()] {
				cc = append(cc, s)
			} else if inner := s.Content.sections(names); len(inner) > 0 {
				cc = append(cc, &Section{Title: s.Title, Content: inner})
			}
		}
	}

	return cc
}

type Section struct {
	Title   Text
	Content Content
}

func (s Section) DebugString() string { return s.debugString(0) }

func (s Section) debugString(depth int) string {
	var b strings.Builder

	t := s.Title.DebugString()
	if len(t) > 0 {
		b.WriteString("\n\n")
		b.WriteString(strings.Repeat("#", depth+1))
		b.WriteRune(' ')
		b.WriteString(t)
		b.WriteString("\n\n")
	}

	for _, c := range s.Content {
		if d, ok := c.(interface{ debugString(int) string }); ok {
			b.WriteString(d.debugString(depth + 1))
		} else {
			b.WriteString(c.DebugString())
		}
	}

	return b.String()
}

func (s Section) String() string {
	var b strings.Builder

	t := s.Title.String()
	if len(t) > 0 {
		b.WriteString("\n\n")
		b.WriteString(t)
		b.WriteString("\n\n")
	}

	for _, c := range s.Content {
		b.WriteString(c.String())
	}

	return b.String()
}

// many leet hax in here.
func isHeading(lines []Text, i int) bool {
	line := lines[i].TrimSpace()
	content := line.String()
	if content == "" {
		return false
	}

	if i > 0 && lines[i-1].TrimSpace().String() != "" {
		// Stuff on previous line.
		return false
	}

	var nextLineWithContent Text
	for j := i + 1; j < len(lines); j++ {
		if lines[j].TrimSpace().String() != "" {
			nextLineWithContent = lines[j]
			break
		}
	}

	if nextLineWithContent.String() == "" {
		// Last line.
		return false
	}

	// Short false matches.
	if len(content) < 4 {
		return false
	}

	// Denylist.
	switch strings.ToLower(content) {
	case "table of contents":
		return false
	}

	return line.Size() > nextLineWithContent.Size()
}
