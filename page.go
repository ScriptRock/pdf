// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pdf

import (
	"fmt"
	"io"
	"runtime/debug"

	"github.com/ScriptRock/pdf/internal/state"
	"github.com/ScriptRock/pdf/text"
)

// A Page represent a single Page in a PDF file.
// The methods interpret a Page dictionary stored in V.
type Page struct {
	v value
}

// Page returns the page for the given page number.
// Page numbers are indexed starting at 1, not 0.
// If the page is not found, Page returns an error.
func (r *Reader) Page(i int) (text.Text, error) {
	num := i - 1 // now 0-indexed
	page := r.trailerValue().Key("Root").Key("Pages")
Search:
	for page.Key("Type").Name() == "Pages" {
		count := int(page.Key("Count").Int64())
		if count < num {
			break
		}
		kids := page.Key("Kids")
		for i := range kids.Len() {
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
					p := Page{kid}
					return p.Text()
				}
				num--
			}
		}
	}

	return nil, fmt.Errorf("page %d not found", i)
}

// NPages returns the number of pages in the PDF file.
func (r *Reader) NPages() int {
	return int(r.trailerValue().Key("Root").Key("Pages").Key("Count").Int64())
}

func (p Page) findInherited(key string) value {
	for v := p.v; !v.IsNull(); v = v.Key("Parent") {
		if r := v.Key(key); !r.IsNull() {
			return r
		}
	}
	return value{}
}

// resources returns the resources dictionary associated with the page.
func (p Page) resources() value {
	return p.findInherited("Resources")
}

// fonts returns a list of the fonts associated with the page.
func (p Page) fonts() []string {
	return p.resources().Key("Font").Keys()
}

// font returns the font with the given name associated with the page.
func (p Page) font(name string) *font {
	return newFont(p.resources().Key("Font").Key(name))
}

// Text returns the structured text on the page.
func (p *Page) Text() (result text.Text, err error) {
	// TODO: return errors everywhere.
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = fmt.Errorf("failed to read page text: %v\n%s", r, debug.Stack())
		}
	}()

	decoders := make(map[string]*font)
	for _, f := range p.fonts() {
		decoders[f] = p.font(f)
	}

	var (
		out    text.Builder
		gState state.Graphics
	)

	forEachStream(p, func(stk *stack, op string) {
		n := stk.Len()
		args := make([]value, n)
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
				case stringKind:
					gState.Tj(&out, e.RawString())
				case integerKind:
					gState.TJDisplace(float64(e.Int64()))
				case realKind:
					gState.TJDisplace(e.Float64())
				}
			}
		}
	})

	return out.Text(), nil
}

// forEachStream interprets each stream in the reader as a PostScript stream,
// running `do` against every PostScript operation.
func forEachStream(p *Page, do func(stk *stack, op string)) {
	v := p.v.Key("Contents")
	if v.Kind() == streamKind {
		interpret(v.Reader(), do)
		return
	}

	var rr []io.Reader
	for i := 0; i < v.Len(); i++ {
		v := v.Index(i)
		if v.Kind() == streamKind {
			rr = append(rr, v.Reader())
		}
	}

	interpret(io.MultiReader(rr...), do)

}
