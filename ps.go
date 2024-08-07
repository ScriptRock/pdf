// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pdf

import (
	"io"

	"github.com/ScriptRock/pdf/internal/types"
)

// A stack represents a stack of values.
type stack struct {
	stack []value
}

func (stk *stack) Len() int {
	return len(stk.stack)
}

func (stk *stack) Push(v value) {
	stk.stack = append(stk.stack, v)
}

func (stk *stack) Pop() value {
	n := len(stk.stack)
	if n == 0 {
		return value{}
	}
	v := stk.stack[n-1]
	stk.stack[n-1] = value{}
	stk.stack = stk.stack[:n-1]
	return v
}

func newDict() value {
	return value{data: make(types.Dict)}
}

// interpret interprets the content in a stream as a basic PostScript program,
// pushing values onto a stack and then calling the do function to execute
// operators. The do function may push or pop values from the stack as needed
// to implement op.
//
// interpret handles the operators "dict", "currentdict", "begin", "end", "def", and "pop" itself.
//
// interpret is not a full-blown PostScript interpreter. Its job is to handle the
// very limited PostScript found in certain supporting file formats embedded
// in PDF files, such as cmap files that describe the mapping from font code
// points to Unicode code points.
//
// There is no support for executable blocks, among other limitations.
func interpret(rd io.Reader, do func(stk *stack, op string)) {
	b := newBuffer(rd, 0)
	b.allowEOF = true
	b.allowObjptr = false
	b.allowStream = false
	var stk stack
	var dicts []types.Dict
Reading:
	for {
		tok := b.readToken()
		if tok == io.EOF {
			break
		}
		if kw, ok := tok.(keyword); ok {
			switch kw {
			default:
				for i := len(dicts) - 1; i >= 0; i-- {
					if v, ok := dicts[i][types.Name(kw)]; ok {
						stk.Push(value{data: v})
						continue Reading
					}
				}
				do(&stk, string(kw))
				continue
			case "null", "[", "]", "<<", ">>":
				break
			case "dict":
				stk.Pop()
				stk.Push(value{data: make(types.Dict)})
				continue
			case "currentdict":
				if len(dicts) == 0 {
					panic("no current dictionary")
				}
				stk.Push(value{data: dicts[len(dicts)-1]})
				continue
			case "begin":
				d := stk.Pop()
				if d.Kind() != dictKind {
					panic("cannot begin non-dict")
				}
				dicts = append(dicts, d.data.(types.Dict))
				continue
			case "end":
				if len(dicts) <= 0 {
					panic("mismatched begin/end")
				}
				dicts = dicts[:len(dicts)-1]
				continue
			case "def":
				if len(dicts) <= 0 {
					panic("def without open dict")
				}
				val := stk.Pop()
				key, ok := stk.Pop().data.(types.Name)
				if !ok {
					panic("def of non-name")
				}
				dicts[len(dicts)-1][key] = val.data
				continue
			case "pop":
				stk.Pop()
				continue
			case "dup":
				// See Section 8.2 of Postscript Language Reference, https://www.adobe.com/jp/print/postscript/pdfs/PLRM.pdf.
				val := stk.Pop()
				stk.Push(val)
				stk.Push(val)
				continue
			}
		}
		b.unreadToken(tok)
		obj := b.readObject()
		stk.Push(value{data: obj})
	}
}
