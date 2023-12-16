// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pdf implements reading of PDF files.
//
// # Overview
//
// PDF is Adobe's Portable Document Format, ubiquitous on the internet.
// A PDF document is a complex data format built on a fairly simple structure.
// This package exposes the simple structure along with some wrappers to
// extract basic information. If more complex information is needed, it is
// possible to extract that information by interpreting the structure exposed
// by this package.
//
// Specifically, a PDF is a data structure built from Values, each of which has
// one of the following Kinds:
//
//	Null, for the null object.
//	Integer, for an integer.
//	Real, for a floating-point number.
//	Bool, for a boolean value.
//	Name, for a name constant (as in /Helvetica).
//	String, for a string constant.
//	Dict, for a dictionary of name-value pairs.
//	Array, for an array of values.
//	Stream, for an opaque data stream and associated header dictionary.
//
// The accessors on Value—Int64, Float64, Bool, Name, and so on—return
// a view of the data as the given type. When there is no appropriate view,
// the accessor returns a zero result. For example, the Name accessor returns
// the empty string if called on a Value v for which v.Kind() != Name.
// Returning zero values this way, especially from the Dict and Array accessors,
// which themselves return Values, makes it possible to traverse a PDF quickly
// without writing any error checking. On the other hand, it means that mistakes
// can go unreported.
//
// The basic structure of the PDF file is exposed as the graph of Values.
//
// Most richer data structures in a PDF file are dictionaries with specific interpretations
// of the name-value pairs. The Font and Page wrappers make the interpretation
// of a specific Value as the corresponding type easier. They are only helpers, though:
// they are implemented only in terms of the Value API and could be moved outside
// the package. Equally important, traversal of other PDF data structures can be implemented
// in other packages as needed.
package pdf

// BUG(rsc): The library makes no attempt at efficiency. A value cache maintained in the Reader
// would probably help significantly.

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/njupg/pdf/internal/decrypter"
	"github.com/njupg/pdf/internal/types"
	"github.com/njupg/pdf/text"
)

// A Reader is a single PDF file open for reading.
type Reader struct {
	f          io.ReaderAt
	end        int64
	xref       []types.Xref
	trailer    types.Dict
	trailerptr types.Objptr
	decrypter  *decrypter.Decrypter
}

// Open opens a file for reading.
// Reader.Close should be called when done with the Reader.
func Open(file string) (*Reader, error) {
	f, err := os.Open(file)
	if err != nil {
		f.Close()
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	reader, err := NewReader(f, fi.Size())
	return reader, err
}

// NewReader opens a file for reading, using the data in f with the given total size.
func NewReader(f io.ReaderAt, size int64) (*Reader, error) {
	return NewReaderEncrypted(f, size, "")
}

// NewReaderEncrypted opens a file for reading, using the data in f with the given total size.
// If the PDF is encrypted, NewReaderEncrypted calls pw repeatedly to obtain passwords
// to try. If pw returns the empty string, NewReaderEncrypted stops trying to decrypt
// the file and returns an error.
func NewReaderEncrypted(f io.ReaderAt, size int64, pw string) (*Reader, error) {
	buf := make([]byte, 10)
	f.ReadAt(buf, 0)
	if !bytes.HasPrefix(buf, []byte("%PDF-1.")) || buf[7] < '0' || buf[7] > '7' || buf[8] != '\r' && buf[8] != '\n' {
		return nil, fmt.Errorf("not a PDF file: invalid header")
	}
	end := size
	const endChunk = 100
	buf = make([]byte, endChunk)
	f.ReadAt(buf, end-endChunk)
	for len(buf) > 0 && buf[len(buf)-1] == '\n' || buf[len(buf)-1] == '\r' {
		buf = buf[:len(buf)-1]
	}
	buf = bytes.TrimRight(buf, "\r\n\t ")
	if !bytes.HasSuffix(buf, []byte("%%EOF")) {
		return nil, fmt.Errorf("not a PDF file: missing %%%%EOF")
	}
	i := findLastLine(buf, "startxref")
	if i < 0 {
		return nil, fmt.Errorf("malformed PDF file: missing final startxref")
	}

	r := &Reader{
		f:   f,
		end: end,
	}
	pos := end - endChunk + int64(i)
	b := newBuffer(io.NewSectionReader(f, pos, end-pos), pos)
	if b.readToken() != keyword("startxref") {
		return nil, fmt.Errorf("malformed PDF file: missing startxref")
	}
	startxref, ok := b.readToken().(int64)
	if !ok {
		return nil, fmt.Errorf("malformed PDF file: startxref not followed by integer")
	}
	b = newBuffer(io.NewSectionReader(r.f, startxref, r.end-startxref), startxref)
	xref, trailerptr, trailer, err := readXref(r, b)
	if err != nil {
		return nil, err
	}
	r.xref = xref
	r.trailer = trailer
	r.trailerptr = trailerptr
	if trailer["Encrypt"] == nil {
		return r, nil
	}
	err = r.initEncrypt("")
	if err == nil {
		return r, nil
	}
	if pw == "" || err != decrypter.ErrInvalidPassword {
		return nil, err
	}

	if r.initEncrypt(pw) == nil {
		return r, nil
	}
	return nil, err
}

// Close closes the underlying Reader if it is an io.Closer.
func (r *Reader) Close() error {
	if c, ok := r.f.(io.Closer); ok {
		return c.Close()
	}

	return nil
}

func (r *Reader) trailerValue() value {
	return value{r: r, ptr: r.trailerptr, data: r.trailer}
}

// Text returns an array of structured Texts, one for each page.
func (r *Reader) Text() ([]text.Text, error) {
	var tt []text.Text
	for i := 1; i <= r.NPages(); i++ {
		p := r.Page(i)
		t, err := p.Text()
		if err != nil {
			return nil, fmt.Errorf("failed to read page text: %w", err)
		}
		tt = append(tt, t)
	}

	return tt, nil
}

func readXref(r *Reader, b *buffer) ([]types.Xref, types.Objptr, types.Dict, error) {
	tok := b.readToken()
	if tok == keyword("xref") {
		return readXrefTable(r, b)
	}
	if _, ok := tok.(int64); ok {
		b.unreadToken(tok)
		return readXrefStream(r, b)
	}
	return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: cross-reference table not found: %v", tok)
}

func readXrefStream(r *Reader, b *buffer) ([]types.Xref, types.Objptr, types.Dict, error) {
	obj1 := b.readObject()
	obj, ok := obj1.(types.Objdef)
	if !ok {
		return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: cross-reference table not found: %v", objfmt(obj1))
	}
	strmptr := obj.Ptr
	strm, ok := obj.Obj.(types.Stream)
	if !ok {
		return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: cross-reference table not found: %v", objfmt(obj))
	}
	if strm.Hdr["Type"] != types.Name("XRef") {
		return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref stream does not have type XRef")
	}
	size, ok := strm.Hdr["Size"].(int64)
	if !ok {
		return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref stream missing Size")
	}
	table := make([]types.Xref, size)

	table, err := readXrefStreamData(r, strm, table, size)
	if err != nil {
		return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: %v", err)
	}

	for prevoff := strm.Hdr["Prev"]; prevoff != nil; {
		off, ok := prevoff.(int64)
		if !ok {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev is not integer: %v", prevoff)
		}
		b := newBuffer(io.NewSectionReader(r.f, off, r.end-off), off)
		obj1 := b.readObject()
		obj, ok := obj1.(types.Objdef)
		if !ok {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref prev stream not found: %v", objfmt(obj1))
		}
		prevstrm, ok := obj.Obj.(types.Stream)
		if !ok {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref prev stream not found: %v", objfmt(obj))
		}
		prevoff = prevstrm.Hdr["Prev"]
		prev := value{r: r, data: prevstrm}
		if prev.Kind() != streamKind {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref prev stream is not stream: %v", prev)
		}
		if prev.Key("Type").Name() != "XRef" {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref prev stream does not have type XRef")
		}
		psize := prev.Key("Size").Int64()
		if psize > size {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref prev stream larger than last stream")
		}
		if table, err = readXrefStreamData(r, prev.data.(types.Stream), table, psize); err != nil {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: reading xref prev stream: %v", err)
		}
	}

	return table, strmptr, strm.Hdr, nil
}

func readXrefStreamData(r *Reader, strm types.Stream, table []types.Xref, size int64) ([]types.Xref, error) {
	index, _ := strm.Hdr["Index"].(types.Array)
	if index == nil {
		index = types.Array{int64(0), size}
	}
	if len(index)%2 != 0 {
		return nil, fmt.Errorf("invalid Index array %v", objfmt(index))
	}
	ww, ok := strm.Hdr["W"].(types.Array)
	if !ok {
		return nil, fmt.Errorf("xref stream missing W array")
	}

	var w []int
	for _, x := range ww {
		i, ok := x.(int64)
		if !ok || int64(int(i)) != i {
			return nil, fmt.Errorf("invalid W array %v", objfmt(ww))
		}
		w = append(w, int(i))
	}
	if len(w) < 3 {
		return nil, fmt.Errorf("invalid W array %v", objfmt(ww))
	}

	v := value{r: r, data: strm}
	wtotal := 0
	for _, wid := range w {
		wtotal += wid
	}
	buf := make([]byte, wtotal)
	data := v.Reader()
	for len(index) > 0 {
		start, ok1 := index[0].(int64)
		n, ok2 := index[1].(int64)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("malformed Index pair %v %v %T %T", objfmt(index[0]), objfmt(index[1]), index[0], index[1])
		}
		index = index[2:]
		for i := 0; i < int(n); i++ {
			_, err := io.ReadFull(data, buf)
			if err != nil {
				return nil, fmt.Errorf("error reading xref stream: %v", err)
			}
			v1 := decodeInt(buf[0:w[0]])
			if w[0] == 0 {
				v1 = 1
			}
			v2 := decodeInt(buf[w[0] : w[0]+w[1]])
			v3 := decodeInt(buf[w[0]+w[1] : w[0]+w[1]+w[2]])
			x := int(start) + i
			for cap(table) <= x {
				table = append(table[:cap(table)], types.Xref{})
			}
			if table[x].Ptr != (types.Objptr{}) {
				continue
			}
			switch v1 {
			case 0:
				table[x] = types.Xref{Ptr: types.Objptr{Gen: 65535}}
			case 1:
				table[x] = types.Xref{Ptr: types.Objptr{ID: uint32(x), Gen: uint16(v3)}, Offset: int64(v2)}
			case 2:
				table[x] = types.Xref{Ptr: types.Objptr{ID: uint32(x)}, InStream: true, Stream: types.Objptr{ID: uint32(v2)}, Offset: int64(v3)}
			default:
				slog.Debug("invalid xref stream type", slog.Int("v1", v1), slog.Any("buf", buf))
			}
		}
	}
	return table, nil
}

func decodeInt(b []byte) int {
	x := 0
	for _, c := range b {
		x = x<<8 | int(c)
	}
	return x
}

func readXrefTable(r *Reader, b *buffer) ([]types.Xref, types.Objptr, types.Dict, error) {
	var table []types.Xref

	table, err := readXrefTableData(b, table)
	if err != nil {
		return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: %v", err)
	}

	trailer, ok := b.readObject().(types.Dict)
	if !ok {
		return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref table not followed by trailer dictionary")
	}

	for prevoff := trailer["Prev"]; prevoff != nil; {
		off, ok := prevoff.(int64)
		if !ok {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev is not integer: %v", prevoff)
		}
		b := newBuffer(io.NewSectionReader(r.f, off, r.end-off), off)
		tok := b.readToken()
		if tok != keyword("xref") {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev does not point to xref")
		}
		table, err = readXrefTableData(b, table)
		if err != nil {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: %v", err)
		}

		trailer, ok := b.readObject().(types.Dict)
		if !ok {
			return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: xref Prev table not followed by trailer dictionary")
		}
		prevoff = trailer["Prev"]
	}

	size, ok := trailer[types.Name("Size")].(int64)
	if !ok {
		return nil, types.Objptr{}, nil, fmt.Errorf("malformed PDF: trailer missing /Size entry")
	}

	if size < int64(len(table)) {
		table = table[:size]
	}

	return table, types.Objptr{}, trailer, nil
}

func readXrefTableData(b *buffer, table []types.Xref) ([]types.Xref, error) {
	for {
		tok := b.readToken()
		if tok == keyword("trailer") {
			break
		}
		start, ok1 := tok.(int64)
		n, ok2 := b.readToken().(int64)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("malformed xref table")
		}
		for i := 0; i < int(n); i++ {
			off, ok1 := b.readToken().(int64)
			gen, ok2 := b.readToken().(int64)
			alloc, ok3 := b.readToken().(keyword)
			if !ok1 || !ok2 || !ok3 || alloc != keyword("f") && alloc != keyword("n") {
				return nil, fmt.Errorf("malformed xref table")
			}
			x := int(start) + i
			for cap(table) <= x {
				table = append(table[:cap(table)], types.Xref{})
			}
			if len(table) <= x {
				table = table[:x+1]
			}
			if alloc == "n" && table[x].Offset == 0 {
				table[x] = types.Xref{Ptr: types.Objptr{ID: uint32(x), Gen: uint16(gen)}, Offset: int64(off)}
			}
		}
	}
	return table, nil
}

func findLastLine(buf []byte, s string) int {
	bs := []byte(s)
	max := len(buf)
	for {
		i := bytes.LastIndex(buf[:max], bs)
		if i <= 0 || i+len(bs) >= len(buf) {
			return -1
		}
		if (buf[i-1] == '\n' || buf[i-1] == '\r') && (buf[i+len(bs)] == '\n' || buf[i+len(bs)] == '\r') {
			return i
		}
		max = i
	}
}

func (r *Reader) resolve(parent types.Objptr, x interface{}) value {
	if ptr, ok := x.(types.Objptr); ok {
		if ptr.ID >= uint32(len(r.xref)) {
			return value{}
		}
		xref := r.xref[ptr.ID]
		if xref.Ptr != ptr || !xref.InStream && xref.Offset == 0 {
			return value{}
		}
		var obj types.Object
		if xref.InStream {
			strm := r.resolve(parent, xref.Stream)
		Search:
			for {
				if strm.Kind() != streamKind {
					panic("not a stream")
				}
				if strm.Key("Type").Name() != "ObjStm" {
					panic("not an object stream")
				}
				n := int(strm.Key("N").Int64())
				first := strm.Key("First").Int64()
				if first == 0 {
					panic("missing First")
				}
				b := newBuffer(strm.Reader(), 0)
				b.allowEOF = true
				for i := 0; i < n; i++ {
					id, _ := b.readToken().(int64)
					off, _ := b.readToken().(int64)
					if uint32(id) == ptr.ID {
						b.seekForward(first + off)
						x = b.readObject()
						break Search
					}
				}
				ext := strm.Key("Extends")
				if ext.Kind() != streamKind {
					panic("cannot find object in stream")
				}
				strm = ext
			}
		} else {
			b := newBuffer(io.NewSectionReader(r.f, xref.Offset, r.end-xref.Offset), xref.Offset)
			b.decrypter = r.decrypter
			obj = b.readObject()
			def, ok := obj.(types.Objdef)
			if !ok {
				panic(fmt.Errorf("loading %v: found %T instead of types.Objdef", ptr, obj))
			}
			if def.Ptr != ptr {
				panic(fmt.Errorf("loading %v: found %v", ptr, def.Ptr))
			}
			x = def.Obj
		}
		parent = ptr
	}

	switch x := x.(type) {
	case nil, bool, int64, float64, types.Name, types.Dict, types.Array, types.Stream, string:
		return value{r: r, ptr: parent, data: x}
	default:
		panic(fmt.Errorf("unexpected value type %T in resolve", x))
	}
}

func (r *Reader) streamReader(s types.Stream, length int64) (io.Reader, error) {
	rd := io.NewSectionReader(r.f, s.Offset, length)
	return r.decrypter.Decrypt(s.Ptr, rd)
}

type errorReadCloser struct {
	err error
}

func (e *errorReadCloser) Read([]byte) (int, error) {
	return 0, e.err
}

func (e *errorReadCloser) Close() error {
	return e.err
}

// Reader returns the data contained in the stream v.
// If v.Kind() != Stream, Reader returns a ReadCloser that
// responds to all reads with a “stream not present” error.
func (v value) Reader() io.ReadCloser {
	x, ok := v.data.(types.Stream)
	if !ok {
		return &errorReadCloser{fmt.Errorf("stream not present")}
	}

	rd, err := v.r.streamReader(x, v.Key("Length").Int64())
	if err != nil {
		panic(fmt.Errorf("bad decryption: %w", err))
	}
	filter := v.Key("Filter")
	param := v.Key("DecodeParms")
	switch filter.Kind() {
	default:
		panic(fmt.Errorf("unsupported filter %v", filter))
	case nullKind:
		// ok
	case nameKind:
		rd = applyFilter(rd, filter.Name(), param)
	case arrayKind:
		for i := 0; i < filter.Len(); i++ {
			rd = applyFilter(rd, filter.Index(i).Name(), param.Index(i))
		}
	}

	if rc, ok := rd.(io.ReadCloser); ok {
		return rc
	}

	return io.NopCloser(rd)
}

func applyFilter(rd io.Reader, name string, param value) io.Reader {
	switch name {
	default:
		panic("unknown filter " + name)
	case "FlateDecode":
		zr, err := zlib.NewReader(rd)
		if err != nil {
			panic(err)
		}
		pred := param.Key("Predictor")
		if pred.Kind() == nullKind {
			return zr
		}
		columns := param.Key("Columns").Int64()
		switch pred.Int64() {
		default:
			slog.Debug("unknown predictor", slog.Any("pred", pred))
			panic("pred")
		case 12:
			return &pngUpReader{r: zr, hist: make([]byte, 1+columns), tmp: make([]byte, 1+columns)}
		}
	case "ASCII85Decode":
		cleanASCII85 := newAlphaReader(rd)
		decoder := ascii85.NewDecoder(cleanASCII85)

		switch param.Keys() {
		default:
			slog.Debug("unexpected ASCII85Decode param", slog.Any("param", param))
			panic("not expected DecodeParms for ascii85")
		case nil:
			return decoder
		}
	}
}

type pngUpReader struct {
	r    io.Reader
	hist []byte
	tmp  []byte
	pend []byte
}

func (r *pngUpReader) Read(b []byte) (int, error) {
	n := 0
	for len(b) > 0 {
		if len(r.pend) > 0 {
			m := copy(b, r.pend)
			n += m
			b = b[m:]
			r.pend = r.pend[m:]
			continue
		}
		_, err := io.ReadFull(r.r, r.tmp)
		if err != nil {
			return n, err
		}
		if r.tmp[0] != 2 {
			return n, fmt.Errorf("malformed PNG-Up encoding")
		}
		for i, b := range r.tmp {
			r.hist[i] += b
		}
		r.pend = r.hist[1:]
	}
	return n, nil
}

func (r *Reader) initEncrypt(password string) error {
	// See PDF 32000-1:2008, §7.6.
	encrypt, _ := r.resolve(types.Objptr{}, r.trailer["Encrypt"]).data.(types.Dict)
	if encrypt["Filter"] != types.Name("Standard") {
		return fmt.Errorf("unsupported PDF: encryption filter %v", objfmt(encrypt["Filter"]))
	}

	ids, ok := r.trailer["ID"].(types.Array)
	if !ok || len(ids) < 1 {
		return fmt.Errorf("malformed PDF: missing ID in trailer")
	}
	id, ok := ids[0].(string)
	if !ok {
		return fmt.Errorf("malformed PDF: missing ID in trailer")
	}

	dec, err := decrypter.New(password, encrypt, id)

	if err != nil {
		return err
	}

	r.decrypter = dec
	return nil
}
