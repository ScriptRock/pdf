package text

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSections_writeText(t *testing.T) {
	text1 := Text{{Size: 1, Content: "this is first content"}}
	var c Content
	c.writeText(text1)

	want := Content{text1}
	if diff := cmp.Diff(c, want); diff != "" {
		t.Error("Sections didn't match expectation:", diff)
	}

	text2 := Text{{Size: 1, Content: "this is the second content"}}
	c.writeText(text2)

	want = Content{text1, text2}
	if diff := cmp.Diff(c, want); diff != "" {
		t.Error("Sections didn't match expectation:", diff)
	}
}
