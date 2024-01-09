package pdf

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	"github.com/njupg/pdf/internal/encoding"
	"github.com/njupg/pdf/internal/types"
)

// A value is a single PDF value, such as an integer, dictionary, or array.
// The zero value is a PDF null (Kind() == Null, IsNull() = true).
type value struct {
	r    *Reader
	ptr  types.Objptr
	data any
}

// IsNull reports whether the value is a null. It is equivalent to Kind() == Null.
func (v value) IsNull() bool {
	return v.data == nil
}

// A valueKind specifies the kind of data underlying a Value.
type valueKind int

// The PDF value kinds.
const (
	nullKind valueKind = iota
	boolKind
	integerKind
	realKind
	stringKind
	nameKind
	dictKind
	arrayKind
	streamKind
)

// Kind reports the kind of value underlying v.
func (v value) Kind() valueKind {
	switch v.data.(type) {
	default:
		return nullKind
	case bool:
		return boolKind
	case int64:
		return integerKind
	case float64:
		return realKind
	case string:
		return stringKind
	case types.Name:
		return nameKind
	case types.Dict:
		return dictKind
	case types.Array:
		return arrayKind
	case types.Stream:
		return streamKind
	}
}

// String returns a textual representation of the value v.
// Note that String is not the accessor for values with Kind() == String.
// To access such values, see RawString, Text, and TextFromUTF16.
func (v value) String() string {
	return objfmt(v.data)
}

func objfmt(x any) string {
	switch x := x.(type) {
	default:
		return fmt.Sprint(x)
	case string:
		if encoding.IsPDFDocEncoded(x) {
			return strconv.Quote(encoding.PDFDocDecode(x))
		}
		if encoding.IsUTF16(x) {
			return strconv.Quote(encoding.UTF16Decode(x[2:]))
		}
		return strconv.Quote(x)
	case types.Name:
		return "/" + string(x)
	case types.Dict:
		var keys []string
		for k := range x {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)
		var buf bytes.Buffer
		buf.WriteString("<<")
		for i, k := range keys {
			elem := x[types.Name(k)]
			if i > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString("/")
			buf.WriteString(k)
			buf.WriteString(" ")
			buf.WriteString(objfmt(elem))
		}
		buf.WriteString(">>")
		return buf.String()

	case types.Array:
		var buf bytes.Buffer
		buf.WriteString("[")
		for i, elem := range x {
			if i > 0 {
				buf.WriteString(" ")
			}
			buf.WriteString(objfmt(elem))
		}
		buf.WriteString("]")
		return buf.String()

	case types.Stream:
		return fmt.Sprintf("%v@%d", objfmt(x.Hdr), x.Offset)

	case types.Objptr:
		return fmt.Sprintf("%d %d R", x.ID, x.Gen)

	case types.Objdef:
		return fmt.Sprintf("{%d %d obj}%v", x.Ptr.ID, x.Ptr.Gen, objfmt(x.Obj))
	}
}

// Bool returns v's boolean value.
// If v.Kind() != Bool, Bool returns false.
func (v value) Bool() bool {
	x, ok := v.data.(bool)
	if !ok {
		return false
	}
	return x
}

// Int64 returns v's int64 value.
// If v.Kind() != Int64, Int64 returns 0.
func (v value) Int64() int64 {
	x, ok := v.data.(int64)
	if !ok {
		return 0
	}
	return x
}

// Float64 returns v's float64 value, converting from integer if necessary.
// If v.Kind() != Float64 and v.Kind() != Int64, Float64 returns 0.
func (v value) Float64() float64 {
	x, ok := v.data.(float64)
	if !ok {
		x, ok := v.data.(int64)
		if ok {
			return float64(x)
		}
		return 0
	}
	return x
}

// RawString returns v's string value.
// If v.Kind() != String, RawString returns the empty string.
func (v value) RawString() string {
	x, ok := v.data.(string)
	if !ok {
		return ""
	}
	return x
}

// Text returns v's string value interpreted as a “text string” (defined in the PDF spec)
// and converted to UTF-8.
// If v.Kind() != String, Text returns the empty string.
func (v value) Text() string {
	x, ok := v.data.(string)
	if !ok {
		return ""
	}
	if encoding.IsPDFDocEncoded(x) {
		return encoding.PDFDocDecode(x)
	}
	if encoding.IsUTF16(x) {
		return encoding.UTF16Decode(x[2:])
	}
	return x
}

// TextFromUTF16 returns v's string value interpreted as big-endian UTF-16
// and then converted to UTF-8.
// If v.Kind() != String or if the data is not valid UTF-16, TextFromUTF16 returns
// the empty string.
func (v value) TextFromUTF16() string {
	x, ok := v.data.(string)
	if !ok {
		return ""
	}
	if len(x)%2 == 1 {
		return ""
	}
	if x == "" {
		return ""
	}
	return encoding.UTF16Decode(x)
}

// Name returns v's name value.
// If v.Kind() != Name, Name returns the empty string.
// The returned name does not include the leading slash:
// if v corresponds to the name written using the syntax /Helvetica,
// Name() == "Helvetica".
func (v value) Name() string {
	x, ok := v.data.(types.Name)
	if !ok {
		return ""
	}
	return string(x)
}

// Key returns the value associated with the given name key in the dictionary v.
// Like the result of the Name method, the key should not include a leading slash.
// If v is a stream, Key applies to the stream's header dictionary.
// If v.Kind() != Dict and v.Kind() != Stream, Key returns a null Value.
func (v value) Key(key string) value {
	x, ok := v.data.(types.Dict)
	if !ok {
		strm, ok := v.data.(types.Stream)
		if !ok {
			return value{}
		}
		x = strm.Hdr
	}
	return v.r.resolve(v.ptr, x[types.Name(key)])
}

// Keys returns a sorted list of the keys in the dictionary v.
// If v is a stream, Keys applies to the stream's header dictionary.
// If v.Kind() != Dict and v.Kind() != Stream, Keys returns nil.
func (v value) Keys() []string {
	x, ok := v.data.(types.Dict)
	if !ok {
		strm, ok := v.data.(types.Stream)
		if !ok {
			return nil
		}
		x = strm.Hdr
	}
	keys := []string{} // not nil
	for k := range x {
		keys = append(keys, string(k))
	}
	sort.Strings(keys)
	return keys
}

// Index returns the i'th element in the array v.
// If v.Kind() != Array or if i is outside the array bounds,
// Index returns a null Value.
func (v value) Index(i int) value {
	x, ok := v.data.(types.Array)
	if !ok || i < 0 || i >= len(x) {
		return value{}
	}
	return v.r.resolve(v.ptr, x[i])
}

// Len returns the length of the array v.
// If v.Kind() != Array, Len returns 0.
func (v value) Len() int {
	x, ok := v.data.(types.Array)
	if !ok {
		return 0
	}
	return len(x)
}

// RawElements returns the elements in the array.
// If v.Kind() != Array, RawElements returns nil.
// RawElements only returns values with kinds matching those given.
func (v value) RawElements(kinds ...valueKind) []any {
	var ee []any

	kk := map[valueKind]bool{}
	for _, k := range kinds {
		kk[k] = true
	}

	for i := 0; i < v.Len(); i++ {
		e := v.Index(i)
		if !kk[e.Kind()] {
			continue
		}

		switch e.Kind() {
		case boolKind:
			ee = append(ee, e.Bool())
		case integerKind:
			ee = append(ee, e.Int64())
		case realKind:
			ee = append(ee, e.Float64())
		case stringKind:
			ee = append(ee, e.RawString())
		case nameKind:
			ee = append(ee, e.Name())
		}
	}
	return ee
}
