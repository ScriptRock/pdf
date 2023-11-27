package state

import (
	"math"

	"github.com/njupg/pdf/internal/matrix"
)

type Font interface {
	Name() string
	Width(c int) float64
	Decode(string) string
}

// Text holds most state defined in:
// PDF_ISO_32000-2: Table 102: Text state parameters
//
// Methods on Text implement the operators from:
// PDF_ISO_32000-2: Table 103: Text state operators
// and
// PDF_ISO_32000-2: Table 106: Text-positioning operators
type Text struct {
	tc    unscaled
	tw    unscaled
	logTh float64 // Log so that zero value is correct, Th = 1.
	tl    unscaled
	tf    Font
	tfs   float64
	tm    *matrix.Matrix
	tlm   *matrix.Matrix
}

type unscaled float64

func (t *Text) Tc(v float64) { t.tc = unscaled(v) }

func (t *Text) Tw(v float64) { t.tw = unscaled(v) }

func (t *Text) Tz(v float64) { t.logTh = math.Log(v / 100) }

func (t *Text) TL(v float64) { t.tl = unscaled(v) }

func (t *Text) Tf(font Font, size float64) {
	t.tf = font
	t.tfs = size
}

func (t *Text) BT() {
	t.tm = matrix.Identity()
	t.tlm = matrix.Identity()
}

func (t *Text) ET() {
	t.tm = nil
	t.tlm = nil
}

func (t *Text) Td(tx, ty float64) {
	m := matrix.Matrix{
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
	t.tlm = &matrix.Matrix{
		{a, b, 0},
		{c, d, 0},
		{e, f, 1},
	}
	t.tm = t.tlm
}

func (t *Text) Tstar() {
	t.TD(0, -float64(t.tl))
}

type Renderer interface {
	Render(x, y, w, h float64, font, s string)
}

func (t *Text) Tj(g Graphics, r Renderer, s string) {
	var fn string
	if t.tf != nil {
		fn = t.tf.Name()
		s = t.tf.Decode(s)
	}
	x, y, w, h := t.textDims(g, s)

	r.Render(x, y, w, h, fn, s)
}

func (t *Text) TJAdjust(v float64) {
	tx := -v / 1000 * t.tfs * math.Exp(t.logTh)
	t.Td(tx, 0)
}

// See PDF_ISO_32000-2: 9.4.4 Text space details.
func (t *Text) textDims(g Graphics, s string) (x, y, w, h float64) {
	m := &matrix.Matrix{
		{t.tfs * math.Exp(t.logTh), 0, 0},
		{0, t.tfs, 0},
		{0, 0, 1},
	}
	pre := m.Mul(t.tm).Mul(g.ctm)

	var tx, w0 float64
	for _, b := range s {
		w0 += t.tf.Width(int(b))
	}
	tx = (w0*t.tfs/1000 + float64(len(s))*float64(t.tc+t.tw)) * math.Exp(t.logTh)
	t.Td(tx, 0)

	post := m.Mul(t.tm).Mul(g.ctm)
	w = post[2][0] - pre[2][0]

	return pre[2][0], pre[2][1], w, pre[1][1]

}
