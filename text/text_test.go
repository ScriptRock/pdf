package text

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func Test_Text_Split(t *testing.T) {
	testCases := map[string]struct {
		input Text
		want  []Text
	}{
		"nil string": {},
		"empty string": {
			input: Text{{Size: 1, Weight: 1, Content: ""}},
			want:  []Text{{{Size: 1, Weight: 1, Content: ""}}},
		},
		"single size without sep": {
			input: Text{{Size: 1, Weight: 1, Content: "abc"}},
			want:  []Text{{{Size: 1, Weight: 1, Content: "abc"}}},
		},
		"multiple sizes without sep": {
			input: Text{{Size: 1, Weight: 1, Content: "a"}, {Size: 2, Weight: 2, Content: "bc"}},
			want:  []Text{{{Size: 1, Weight: 1, Content: "a"}, {Size: 2, Weight: 2, Content: "bc"}}},
		},
		"sep in one part": {
			input: Text{{Size: 1, Weight: 1, Content: "a\nb"}, {Size: 2, Weight: 2, Content: "c"}},
			want: []Text{
				{{Size: 1, Weight: 1, Content: "a"}},
				{{Size: 1, Weight: 1, Content: "b"}, {Size: 2, Weight: 2, Content: "c"}},
			},
		},
		"sep in multiple parts": {
			input: Text{{Size: 1, Weight: 1, Content: "a\nb"}, {Size: 2, Weight: 2, Content: "c\nd"}},
			want: []Text{
				{{Size: 1, Weight: 1, Content: "a"}},
				{{Size: 1, Weight: 1, Content: "b"}, {Size: 2, Weight: 2, Content: "c"}},
				{{Size: 2, Weight: 2, Content: "d"}},
			},
		},
	}

	opt := cmp.AllowUnexported(Builder{})
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := tc.input.Split("\n")

			if diff := cmp.Diff(got, tc.want, opt); diff != "" {
				t.Error("split Strings did not match expectations:", diff)
			}
		})
	}
}

func Test_Text_TrimSpace(t *testing.T) {
	testCases := map[string]struct {
		input Text
		want  Text
	}{
		"whitespace only": {
			input: Text{{Size: 1, Weight: 1, Content: " \n\t "}},
			want:  nil,
		},
		"single piece": {
			input: Text{{Size: 1, Weight: 1, Content: " a "}},
			want:  Text{{Size: 1, Weight: 1, Content: "a"}},
		},
		"multi pieces": {
			input: Text{{Size: 1, Weight: 1, Content: " a "}, {Size: 2, Weight: 2, Content: " b "}, {Size: 3, Weight: 3, Content: " c "}},
			want:  Text{{Size: 1, Weight: 1, Content: "a "}, {Size: 2, Weight: 2, Content: " b "}, {Size: 3, Weight: 3, Content: " c"}},
		},
	}

	opt := cmp.AllowUnexported(Builder{})
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got := tc.input.TrimSpace()

			if diff := cmp.Diff(got, tc.want, opt); diff != "" {
				t.Error("trimmed string did not match expectation:", diff)
			}
		})
	}
}
