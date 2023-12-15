package state

import (
	"math"
)

type Font interface {
	Name() string
	Decode(string) (string, float64)
}

// Text holds most state defined in:
// PDF_ISO_32000-2: Table 102: Text state parameters
//
// Methods on Text implement the operators from:
// PDF_ISO_32000-2: Table 103: Text state operators
// and
// PDF_ISO_32000-2: Table 106: Text-positioning operators
type Text struct {
	tc    float64
	tw    float64
	logTh float64 // Log so that zero value is correct, Th = 1.
	tl    float64
	tf    Font
	tfs   float64
	tm    *matrix
	tlm   *matrix
}

func (t *Text) Tc(v float64) { t.tc = v }

func (t *Text) Tw(v float64) { t.tw = v }

func (t *Text) Tz(v float64) { t.logTh = math.Log(v / 100) }

func (t *Text) TL(v float64) { t.tl = v }

func (t *Text) Tf(font Font, size float64) {
	t.tf = font
	t.tfs = size
}

func (t *Text) BT() {
	t.tlm = identity()
	t.tm = t.tlm
}

func (t *Text) ET() {
	t.tlm = nil
	t.tm = nil
}

func (t *Text) Td(tx, ty float64) {
	m := matrix{
		{1, 0, 0},
		{0, 1, 0},
		{tx, ty, 1},
	}
	t.tlm = m.Mul(t.tlm)
	t.tm = t.tlm
}

func (t *Text) TD(tx, ty float64) {
	t.TL(-ty)
	t.Td(tx, ty)
}

func (t *Text) Tm(a, b, c, d, e, f float64) {
	t.tlm = &matrix{
		{a, b, 0},
		{c, d, 0},
		{e, f, 1},
	}
	t.tm = t.tlm
}

func (t *Text) Tstar() {
	t.TD(0, -t.tl)
}

type Renderer interface {
	Render(x, y, w, h float64, font, s string)
}

func (t *Text) Tj(ctm *matrix, r Renderer, raw string) {
	fn := t.tf.Name()
	s, w0 := t.tf.Decode(raw)
	x, y, w, h := t.textDims(ctm, s, w0)

	r.Render(x, y, w, h, fn, s)
}

// TJDisplace handles that part of a TJ operator when one of the array elements is a glyph displacement.
func (t *Text) TJDisplace(v float64) {
	t.displace(-v, 0, 0)
}

// displace update the text matrix (cursor), but not the text line matrix (representing the beginning of the line),
// in response to a glyph render or TJ glyph displacement.
func (t *Text) displace(v, nc, nw float64) {
	tx := (v/1000*t.tfs + nc*t.tc + nw*t.tw) * math.Exp(t.logTh)
	t.tm = (&matrix{
		{1, 0, 0},
		{0, 1, 0},
		{tx, 0, 1},
	}).Mul(t.tm)
}

// See PDF_ISO_32000-2: 9.4.4 Text space details.
func (t *Text) textDims(ctm *matrix, s string, w0 float64) (x, y, w, h float64) {
	rm := t.trm(ctm)

	var nc, nw float64
	for _, r := range s {
		if r == ' ' {
			nw++
		} else {
			nc++
		}
	}

	t.displace(w0, nc, nw)

	x = rm[2][0]
	y = rm[2][1]
	w = t.trm(ctm)[2][0] - rm[2][0]
	h = rm[1][1]
	return
}

// trm calculates the text rendering matrix,
// see PDF_ISO_32000-2: 9.4.4 Text space details.
func (t *Text) trm(ctm *matrix) *matrix {
	m := &matrix{
		{t.tfs * math.Exp(t.logTh), 0, 0},
		{0, t.tfs, 0},
		{0, 0, 1},
	}

	return m.Mul(t.tm).Mul(ctm)
}
