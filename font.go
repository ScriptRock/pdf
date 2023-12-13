package pdf

import (
	"log/slog"

	"github.com/njupg/pdf/internal/encoding"
)

func NewFont(v Value) *Font {
	return &Font{
		name:    v.Key("BaseFont").Name(),
		Decoder: getDecoder(v),
		widths:  getWidths(v),
	}
}

// A Font represent a font in a PDF file.
// The methods interpret a Font dictionary stored in V.
type Font struct {
	Decoder
	name   string
	widths widths
}

// BaseFont returns the font's name (BaseFont property).
func (f Font) Name() string { return f.name }

// Width returns the width of the given code point.
func (f Font) Width(code int) float64 { return f.widths.lookup(code) }

func getWidths(v Value) widths {
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
			case IntegerKind:
				span.last = int(ww.Index(i).Int64())
				span.fixed = ww.Index(i + 1).Float64()
				i += 3
			case ArrayKind:
				values := ww.Index(i)
				span.last = span.first + values.Len()
				span.linear = make([]float64, values.Len())
				for j := 0; j < values.Len(); j++ {
					span.linear[j] = values.Index(j).Float64()
				}
				i += 2
			default:
				panic("bad w:" + ww.String())
			}
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

func getDecoder(v Value) Decoder {
	enc := v.Key("Encoding")
	switch enc.Kind() {
	case NameKind:
		switch enc.Name() {
		case "WinAnsiEncoding":
			return encoding.WinANSI()
		case "MacRomanEncoding":
			return encoding.MacRoman()
		case "Identity-H":
			return charmapEncoding(v)
		default:
			slog.Debug("unknown encoding", slog.String("name", enc.Name()))
			return encoding.None{}
		}
	case DictKind:
		return &encoding.Dict{Elements: enc.Key("Differences").RawElements(NameKind, IntegerKind)}
	case NullKind:
		return charmapEncoding(v)
	default:
		slog.Debug("unexpected encoding", slog.String("encoding", enc.String()))
		return encoding.None{}
	}
}

func charmapEncoding(v Value) Decoder {
	toUnicode := v.Key("ToUnicode")
	if toUnicode.Kind() != StreamKind {
		return encoding.PDFDoc()
	}

	n := -1
	var m encoding.CMap
	ok := true
	Interpret(toUnicode.Reader(), func(stk *Stack, op string) {
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
				case StringKind:
					bfr.DstS = dst.RawString()
				case ArrayKind:
					bfr.DstA = dst.RawElements(StringKind)
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
		return encoding.None{}
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

func (w widths) lookup(code int) float64 {
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
