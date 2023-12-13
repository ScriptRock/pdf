// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pdf

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/njupg/pdf/internal/state"
	"github.com/njupg/pdf/text"
)

// A Page represent a single page in a PDF file.
// The methods interpret a Page dictionary stored in V.
type Page struct {
	V Value
}

// Page returns the page for the given page number.
// Page numbers are indexed starting at 1, not 0.
// If the page is not found, Page returns a Page with p.V.IsNull().
func (r *Reader) Page(num int) Page {
	num-- // now 0-indexed
	page := r.trailerValue().Key("Root").Key("Pages")
Search:
	for page.Key("Type").Name() == "Pages" {
		count := int(page.Key("Count").Int64())
		if count < num {
			return Page{}
		}
		kids := page.Key("Kids")
		for i := 0; i < kids.Len(); i++ {
			kid := kids.Index(i)
			if kid.Key("Type").Name() == "Pages" {
				c := int(kid.Key("Count").Int64())
				if num < c {
					page = kid
					continue Search
				}
				num -= c
				continue
			}
			if kid.Key("Type").Name() == "Page" {
				if num == 0 {
					return Page{kid}
				}
				num--
			}
		}
	}
	return Page{}
}

// NumPage returns the number of pages in the PDF file.
func (r *Reader) NumPage() int {
	return int(r.trailerValue().Key("Root").Key("Pages").Key("Count").Int64())
}

func (p Page) findInherited(key string) Value {
	for v := p.V; !v.IsNull(); v = v.Key("Parent") {
		if r := v.Key(key); !r.IsNull() {
			return r
		}
	}
	return Value{}
}

// Resources returns the resources dictionary associated with the page.
func (p Page) Resources() Value {
	return p.findInherited("Resources")
}

// Fonts returns a list of the fonts associated with the page.
func (p Page) Fonts() []string {
	return p.Resources().Key("Font").Keys()
}

// Font returns the font with the given name associated with the page.
func (p Page) Font(name string) *Font {
	return NewFont(p.Resources().Key("Font").Key(name))
}

func (p *Page) GetText() (result text.Text, err error) {
	// TODO: return errors everywhere.
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("failed to read page text: %v\n%s", r, debug.Stack())
		}
	}()

	decoders := make(map[string]*Font)
	for _, f := range p.Fonts() {
		decoders[f] = p.Font(f)
	}

	var (
		out    text.Builder
		gState state.Graphics
	)

	forEachStream(p, func(stk *Stack, op string) {
		n := stk.Len()
		args := make([]Value, n)
		for i := n - 1; i >= 0; i-- {
			args[i] = stk.Pop()
		}

		switch op {
		case "q":
			gState.Push()
		case "Q":
			gState.Pop()
		case "cm":
			gState.CM(args[0].Float64(), args[1].Float64(), args[2].Float64(), args[3].Float64(), args[4].Float64(), args[5].Float64())

		case "Tc":
			gState.Tc(args[0].Float64())
		case "Tw":
			gState.Tw(args[0].Float64())
		case "Tz":
			gState.Tz(args[0].Float64())
		case "TL":
			gState.TL(args[0].Float64())
		case "BT":
			gState.BT()
		case "ET":
			gState.ET()
		case "Td":
			gState.Td(args[0].Float64(), args[1].Float64())
		case "TD":
			gState.TD(args[0].Float64(), args[1].Float64())
		case "Tm":
			gState.Tm(args[0].Float64(), args[1].Float64(), args[2].Float64(), args[3].Float64(), args[4].Float64(), args[5].Float64())
		case "T*":
			gState.Tstar()
		case "Tf":
			gState.Tf(decoders[args[0].Name()], args[1].Float64())

		case `"`:
			gState.Tw(args[0].Float64())
			gState.Tc(args[1].Float64())
			args = args[2:]
			fallthrough
		case `'`:
			gState.Tstar()
			fallthrough
		case "Tj":
			gState.Tj(&out, args[0].RawString())
		case "TJ":
			arr := args[0]
			for i := 0; i < arr.Len(); i++ {
				e := arr.Index(i)
				switch e.Kind() {
				case StringKind:
					gState.Tj(&out, e.RawString())
				case RealKind:
					gState.TJDisplace(e.Float64())
				}
			}
		}
	})

	return out.Text(), nil
}

// forEachStream interprets each stream in the reader as a PostScript stream,
// running `do` against every PostScript operation.
func forEachStream(p *Page, do func(stk *Stack, op string)) {
	v := p.V.Key("Contents")
	if v.Kind() == StreamKind {
		Interpret(v.Reader(), do)
		return
	}

	var rr []io.Reader
	for i := 0; i < v.Len(); i++ {
		v := v.Index(i)
		if v.Kind() == StreamKind {
			rr = append(rr, v.Reader())
		}
	}

	Interpret(io.MultiReader(rr...), do)

}
