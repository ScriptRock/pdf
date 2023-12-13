package encoding

import (
	"log/slog"
	"strings"
)

type ByteRange struct {
	Lo string
	Hi string
}

type BFChar struct {
	Orig string
	Repl string
}

type BFRange struct {
	Lo   string
	Hi   string
	DstS string
	DstA []any
}

type CMap struct {
	Widths   Sizer
	Space    [4][]ByteRange // codespace range
	BFRanges []BFRange
	BFChars  []BFChar
}

func (m *CMap) Decode(raw string) (string, float64) {
	var w float64
	var r strings.Builder
Parse:
	for len(raw) > 0 {
		var code int
		for n := 1; n <= 4 && n <= len(raw); n++ { // number of digits in character replacement (1-4 possible)
			code = (code << 8) | int(raw[n-1])
			for _, space := range m.Space[n-1] { // find matching codespace Ranges for number of digits
				if space.Lo <= raw[:n] && raw[:n] <= space.Hi { // see if value is in range
					text := raw[:n]
					raw = raw[n:]
					for _, bfchar := range m.BFChars { // check for matching bfchar
						if len(bfchar.Orig) == n && bfchar.Orig == text {
							r.WriteString(UTF16Decode(bfchar.Repl))
							w += m.Widths.CodeWidth(code)
							continue Parse
						}
					}
					for _, bfrange := range m.BFRanges { // check for matching bfrange
						if len(bfrange.Lo) == n && bfrange.Lo <= text && text <= bfrange.Hi {
							switch {
							case len(bfrange.DstS) > 0:
								s := bfrange.DstS
								if bfrange.Lo != text { // value isn't at the beginning of the range so scale result
									b := []byte(s)
									b[len(b)-1] += text[len(text)-1] - bfrange.Lo[len(bfrange.Lo)-1] // increment last byte by difference
									s = string(b)
								}
								r.WriteString(UTF16Decode(s))
								w += m.Widths.CodeWidth(code)
								continue Parse
							case len(bfrange.DstA) > 0:
								n := text[len(text)-1] - bfrange.Lo[len(bfrange.Lo)-1]
								s := bfrange.DstA[int(n)].(string)
								r.WriteString(UTF16Decode(s))
								w += m.Widths.CodeWidth(code)
								continue Parse
							default:
								slog.Debug("unknown dst", slog.Any("dst", bfrange.DstA))
							}
							r.WriteRune(NoRune)
							continue Parse
						}
					}
					r.WriteRune(NoRune)
					continue Parse
				}
			}
		}
		slog.Debug("no code space found")
		r.WriteRune(NoRune)
		raw = raw[1:]
	}
	return r.String(), w
}
