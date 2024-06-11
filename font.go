package pdf

import (
	"log/slog"

	"github.com/ScriptRock/pdf/internal/encoding"
)

func newFont(v value) *font {
	return &font{
		name:    v.Key("BaseFont").Name(),
		decoder: getDecoder(v),
	}
}

// A font represent a font in a PDF file.
// The methods interpret a font dictionary stored in V.
type font struct {
	decoder
	name string
}

// BaseFont returns the font's name (BaseFont property).
func (f font) Name() string { return f.name }

func getWidths(v value) widths {
	switch v.Key("Subtype").String() {
	case "/Type0":
		return getWidths(v.Key("DescendantFonts").Index(0))
	case "/CIDFontType0", "/CIDFontType2":
		dw := v.Key("DW").Float64()

		ww := v.Key("W")

		var spans []span
		i := 1
		for i < ww.Len() {
			span := span{
				first: int(ww.Index(i - 1).Int64()),
			}
			switch ww.Index(i).Kind() {
			case integerKind:
				span.last = int(ww.Index(i).Int64())
				span.fixed = ww.Index(i + 1).Float64()
				i += 3
			case arrayKind:
				values := ww.Index(i)
				span.last = span.first + values.Len() - 1
				span.linear = make([]float64, values.Len())
				for j := 0; j < values.Len(); j++ {
					span.linear[j] = values.Index(j).Float64()
				}
				i += 2
			default:
				panic("bad w:" + ww.String())
			}
			spans = append(spans, span)
		}

		return widths{defaultW: dw, spans: spans}
	default:
		dw := v.Key("FontDescriptor").Key("MissingWidth").Float64()

		ww := v.Key("Widths")
		s := span{
			first:  int(v.Key("FirstChar").Int64()),
			last:   int(v.Key("LastChar").Int64()),
			linear: make([]float64, ww.Len()),
		}
		for i := 0; i < ww.Len(); i++ {
			s.linear[i] = ww.Index(i).Float64()
		}

		return widths{defaultW: dw, spans: []span{s}}
	}
}

// See Table 112: Entries in an encoding dictionary.
func getDifferences(v value) map[byte]string {
	dd := map[byte]string{}
	diffs := v.Key("Differences")

	var c int = -1
	for i := range diffs.Len() {
		switch e := diffs.Index(i); e.Kind() {
		case integerKind:
			c = int(e.Int64())
		case nameKind:
			if c < 0 || c > 255 {
				panic("bad differences array:" + v.String())
			}
			dd[byte(c)] = e.String()[1:]
			c++
		default:
			panic("bad differences array:" + v.String())
		}
	}

	return dd
}

func getDecoder(v value) decoder {
	widths := getWidths(v)

	switch enc := v.Key("Encoding"); enc.Kind() {
	case nameKind:
		switch enc.Name() {
		case "WinAnsiEncoding":
			return encoding.WinANSI(widths, nil)
		case "MacRomanEncoding":
			return encoding.MacRoman(widths, nil)
		}
	case dictKind:
		// See 9.6.5 Character encoding.
		diffs := getDifferences(enc)
		switch enc.Key("BaseEncoding").Name() {
		case "WinAnsiEncoding":
			return encoding.WinANSI(widths, diffs)
		case "MacRomanEncoding":
			return encoding.MacRoman(widths, diffs)
		case "Identity-H":
			return charmapEncoding(v, widths)
		}
	}

	if toUnicode := v.Key("ToUnicode"); !toUnicode.IsNull() {
		return charmapEncoding(toUnicode, widths)
	}

	// See 9.6.2.2, Type 1 standard fonts.
	return encoding.PDFDoc(widths)
}

func charmapEncoding(toUnicode value, widths widths) decoder {
	if toUnicode.Kind() != streamKind {
		return encoding.PDFDoc(widths)
	}

	n := -1
	m := encoding.CMap{Widths: widths}
	ok := true
	interpret(toUnicode.Reader(), func(stk *stack, op string) {
		if !ok {
			return
		}
		switch op {
		case "findresource":
			stk.Pop() // category
			stk.Pop() // key
			stk.Push(newDict())
		case "begincmap":
			stk.Push(newDict())
		case "endcmap":
			stk.Pop()
		case "begincodespacerange":
			n = int(stk.Pop().Int64())
		case "endcodespacerange":
			if n < 0 {
				slog.Debug("missing begincodespacerange")
				ok = false
				return
			}
			for i := 0; i < n; i++ {
				hi, lo := stk.Pop().RawString(), stk.Pop().RawString()
				if len(lo) == 0 || len(lo) != len(hi) {
					slog.Debug("bad codespace range", slog.String("lo", lo), slog.String("hi", hi))
					ok = false
					return
				}
				m.Space[len(lo)-1] = append(m.Space[len(lo)-1], encoding.ByteRange{Lo: lo, Hi: hi})
			}
			n = -1
		case "beginbfchar":
			n = int(stk.Pop().Int64())
		case "endbfchar":
			if n < 0 {
				panic("missing beginbfchar")
			}
			for i := 0; i < n; i++ {
				repl, orig := stk.Pop().RawString(), stk.Pop().RawString()
				m.BFChars = append(m.BFChars, encoding.BFChar{Orig: orig, Repl: repl})
			}
		case "beginbfrange":
			n = int(stk.Pop().Int64())
		case "endbfrange":
			if n < 0 {
				panic("missing beginbfrange")
			}
			for i := 0; i < n; i++ {
				dst, srcHi, srcLo := stk.Pop(), stk.Pop().RawString(), stk.Pop().RawString()
				bfr := encoding.BFRange{Lo: srcLo, Hi: srcHi}
				switch dst.Kind() {
				case stringKind:
					bfr.DstS = dst.RawString()
				case arrayKind:
					bfr.DstA = dst.RawElements(stringKind)
				}
				m.BFRanges = append(m.BFRanges, bfr)
			}
		case "defineresource":
			stk.Pop().Name() // category
			value := stk.Pop()
			stk.Pop().Name() // key
			stk.Push(value)
		default:
			slog.Debug("unhandled op", slog.String("op", op))
		}
	})
	if !ok {
		panic("bad ToUnicode stream: " + toUnicode.String())
	}
	return &m
}

type widths struct {
	defaultW float64
	spans    []span
}

type span struct {
	first, last int
	fixed       float64
	linear      []float64
}

func (w widths) CodeWidth(code int) float64 {
	for _, s := range w.spans {
		if code >= s.first && code <= s.last {
			switch {
			case len(s.linear) > 0:
				return s.linear[code-s.first]
			default:
				return s.fixed
			}
		}
	}
	return w.defaultW
}
